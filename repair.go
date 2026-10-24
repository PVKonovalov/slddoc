package slddoc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"sort"
)

// The repairs below are what the editor runs on a diagram it opens or imports. They mirror the frontend's
// diagramOps.ts (normalizeTopology and its helpers, ensureLastId, inferSizeSteps) rule for rule, in the same order,
// so a diagram repaired here is the same one the browser would have produced.

const (
	repairEpsilon      = 1e-6
	portMatchTolerance = 0.5
	// sizeStepTolerance is how far an imported port may sit from its terminal at an inferred size step.
	sizeStepTolerance = 6
)

// SizeSteps are the size steps the editor offers and InferSizeSteps tries.
var SizeSteps = []int{-2, -1, 0, 1, 2, 3, 4}

// legacyTerminals are the local terminal positions base.xml used before they were snapped to the 10-grid, by shape.
var legacyTerminals = map[string][]Point{
	"32":  {{X: 0, Y: -12}, {X: 0, Y: 14}},
	"154": {{X: 0, Y: -31}, {X: 0, Y: 31}},
	"385": {{X: 0, Y: -22}},
}

// Repair repairs d's topology in place, as the editor does on open and import: zero-length wires removed, ports at
// legacy terminal positions moved, every device given exactly its shape's fixed ports (terminals maps a symbol
// shape to its local terminals, the library's <terminals>), a busbar's shared Node split into one port Node per
// point where something meets the bus (splitBusbarNodes), stray port Nodes moved onto their terminals, wire ends
// lying on a port joined to it, wires split at junction dots and at wire ends lying on them, and unused Nodes
// dropped, pass after pass until one changes nothing. Existing Nodes and wires are otherwise never moved. Returns
// whether anything changed.
func Repair(d *Diagram, terminals map[string][]Point) bool {
	if len(terminals) == 0 {
		return false
	}
	// A pass can leave work for the next (a wire is tapped at one Node per pass, a Node one pass creates may lie on
	// another wire), so passes repeat until one changes nothing.
	changed := false
	for range maxRepairPasses {
		if !changes(d, func() { repairPass(d, terminals) }) {
			break
		}
		changed = true
	}
	return changed
}

// maxRepairPasses bounds Repair's passes; a real diagram settles in two.
const maxRepairPasses = 5

func repairPass(d *Diagram, terminals map[string][]Point) {
	r := repairer{d: d, terminals: terminals}
	r.removeDegenerateConnectors()
	r.moveLegacyTerminalNodes()
	ids := make([]int, len(d.Elements))
	for i, e := range d.Elements {
		ids[i] = e.ID
	}
	for _, id := range ids {
		r.fitElementPorts(id)
	}
	r.splitBusbarNodes()
	r.removeDegenerateConnectors()
	r.moveStrayPortNodes()
	r.mergeNodesOntoPorts()
	r.tapWiresThroughJunctions()
	r.tapWireNodesOntoWires()
	r.dropUnusedNodes()
}

// EnsureIDs backfills d.LastID from the highest id in use, gives every label with id 0 a fresh id, and renumbers
// colliding ids (elements, then connectors, digital devices and labels keep an id's first use; a colliding node or
// voltage class is renumbered with every reference to it). Layer ids are never reused. Returns whether anything
// changed.
func EnsureIDs(d *Diagram) bool {
	return changes(d, func() {
		if d.LastID == 0 {
			m := 0
			for _, e := range d.Elements {
				m = max(m, e.ID)
			}
			for _, n := range d.Nodes {
				m = max(m, n.ID)
			}
			for _, c := range d.Connectors {
				m = max(m, c.ID)
			}
			for _, vc := range d.VoltageClasses {
				m = max(m, vc.ID)
			}
			for _, l := range d.Labels {
				m = max(m, l.ID)
			}
			for _, dd := range d.DigitalDevices {
				m = max(m, dd.ID)
			}
			d.LastID = m
		}
		for i := range d.Labels {
			if d.Labels[i].ID == 0 {
				d.LastID++
				d.Labels[i].ID = d.LastID
			}
		}
		ensureUniqueIDs(d)
	})
}

func ensureUniqueIDs(d *Diagram) {
	used := map[int]bool{}
	for _, l := range d.Layers {
		used[l.ID] = true
	}
	collides := func(id int) bool { return id == 0 || used[id] }
	next := d.LastID
	take := func() int {
		next++
		for used[next] {
			next++
		}
		used[next] = true
		return next
	}
	keep := func(id *int) {
		if collides(*id) {
			*id = take()
			return
		}
		used[*id] = true
	}
	for i := range d.Elements {
		keep(&d.Elements[i].ID)
	}
	for i := range d.Connectors {
		keep(&d.Connectors[i].ID)
	}
	for i := range d.DigitalDevices {
		keep(&d.DigitalDevices[i].ID)
	}
	for i := range d.Labels {
		keep(&d.Labels[i].ID)
	}
	nodeID := map[int]int{}
	for i := range d.Nodes {
		if old := d.Nodes[i].ID; collides(old) {
			d.Nodes[i].ID = take()
			nodeID[old] = d.Nodes[i].ID
		} else {
			used[old] = true
		}
	}
	classID := map[int]int{}
	for i := range d.VoltageClasses {
		if old := d.VoltageClasses[i].ID; collides(old) {
			d.VoltageClasses[i].ID = take()
			classID[old] = d.VoltageClasses[i].ID
		} else {
			used[old] = true
		}
	}
	if next == d.LastID {
		return
	}
	d.LastID = next
	node := func(id *int) {
		if n, ok := nodeID[*id]; ok {
			*id = n
		}
	}
	volt := func(v *int) {
		if c, ok := classID[*v]; ok && *v != 0 {
			*v = c
		}
	}
	for i := range d.Elements {
		e := &d.Elements[i]
		volt(&e.Voltage)
		for j := range e.Ports {
			node(&e.Ports[j].Node)
		}
		for j := range e.Windings {
			volt(&e.Windings[j].Voltage)
		}
		for j := range e.Sectors {
			volt(&e.Sectors[j].Voltage)
		}
	}
	for i := range d.Connectors {
		node(&d.Connectors[i].From)
		node(&d.Connectors[i].To)
		volt(&d.Connectors[i].Voltage)
	}
	if d.Editor != nil {
		volt(&d.Editor.DefaultVoltage)
	}
}

// InferSizeSteps sets each extracted element's size step (Scale) when its port Nodes sit where its terminals are at
// one step but not at the library size: xsde2svg draws every element at its own step, and Extract reads the real
// port positions but not the step. Only elements whose class takes a size step, with two or more terminals and no
// step yet, are considered, except lead shapes (Extract reads their step from the drawn body); a port whose Node
// another element shares is skipped. Returns whether anything changed.
func InferSizeSteps(d *Diagram, terminals map[string][]Point) bool {
	nodes := nodeIndex(d)
	portUse := map[int]int{}
	for _, e := range d.Elements {
		for _, p := range e.Ports {
			portUse[p.Node]++
		}
	}
	changed := false
	for i := range d.Elements {
		el := &d.Elements[i]
		if !UsesSizeStep(el.Class) || len(terminals[el.Shape]) < 2 || el.Scale != 0 || HasLeads(el.Shape) {
			continue
		}
		var points []Point
		for _, p := range el.Ports {
			if n, ok := nodes[p.Node]; ok && portUse[p.Node] == 1 {
				points = append(points, Point{X: n.X, Y: n.Y})
			}
		}
		if len(points) == 0 {
			continue
		}
		worst := func(step int) float64 {
			probe := *el
			probe.Scale = step
			ts := symbolTerminals(&probe, terminals)
			w := math.Inf(-1)
			for _, p := range points {
				best := math.Inf(1)
				for _, t := range ts {
					best = math.Min(best, math.Hypot(t.X-p.X, t.Y-p.Y))
				}
				w = math.Max(w, best)
			}
			return w
		}
		best, bestError := 0, worst(0)
		for _, step := range SizeSteps {
			if e := worst(step); e < bestError-0.5 {
				best, bestError = step, e
			}
		}
		if best != 0 && bestError <= sizeStepTolerance {
			el.Scale = best
			changed = true
		}
	}
	return changed
}

// MostUsedVoltage is the voltage class id most elements (each transformer winding counted) and connectors
// reference, ties broken by the lower id; 0 when nothing references one.
func MostUsedVoltage(d *Diagram) int {
	counts := map[int]int{}
	for _, e := range d.Elements {
		if e.Voltage != 0 {
			counts[e.Voltage]++
		}
		for _, w := range e.Windings {
			if w.Voltage != 0 {
				counts[w.Voltage]++
			}
		}
	}
	for _, c := range d.Connectors {
		if c.Voltage != 0 {
			counts[c.Voltage]++
		}
	}
	best, bestCount := 0, 0
	for id, n := range counts {
		if n > bestCount || (n == bestCount && id < best) {
			best, bestCount = id, n
		}
	}
	return best
}

// changes runs f on d and reports whether d's JSON form changed.
func changes(d *Diagram, f func()) bool {
	before, err := json.Marshal(d)
	f()
	if err != nil {
		return true
	}
	after, err := json.Marshal(d)
	return err != nil || !bytes.Equal(before, after)
}

type repairer struct {
	d         *Diagram
	terminals map[string][]Point
	// nodes caches nodeIndex(d) for nodeMap until nodesChanged marks it stale.
	nodes map[int]Node
}

// nodeMap is nodeIndex(r.d), rebuilt only after nodesChanged.
func (r *repairer) nodeMap() map[int]Node {
	if r.nodes == nil {
		r.nodes = nodeIndex(r.d)
	}
	return r.nodes
}

// nodesChanged must follow every write to r.d.Nodes.
func (r *repairer) nodesChanged() {
	r.nodes = nil
}

func (r *repairer) takeID() int {
	r.d.LastID++
	return r.d.LastID
}

func samePt(a, b Point) bool {
	return math.Abs(a.X-b.X) < repairEpsilon && math.Abs(a.Y-b.Y) < repairEpsilon
}

// jsRound rounds half up, as JavaScript's Math.round does.
func jsRound(v float64) float64 {
	return math.Floor(v + 0.5)
}

func roundPt(p Point) Point {
	return Point{X: jsRound(p.X*1e6) / 1e6, Y: jsRound(p.Y*1e6) / 1e6}
}

func onSeg(p, a, b Point) bool {
	dx, dy := b.X-a.X, b.Y-a.Y
	len2 := dx*dx + dy*dy
	if len2 == 0 {
		return samePt(p, a)
	}
	cross := (p.X-a.X)*dy - (p.Y-a.Y)*dx
	if math.Abs(cross) > repairEpsilon*math.Sqrt(len2) {
		return false
	}
	dot := (p.X-a.X)*dx + (p.Y-a.Y)*dy
	return dot >= -repairEpsilon && dot <= len2+repairEpsilon
}

// segmentWith is the index of pts' first segment p lies on, or -1.
func segmentWith(pts []Point, p Point) int {
	for i := 0; i+1 < len(pts); i++ {
		if onSeg(p, pts[i], pts[i+1]) {
			return i
		}
	}
	return -1
}

func degenerate(pts []Point) bool {
	if len(pts) < 2 {
		return true
	}
	for _, q := range pts {
		if !samePt(q, pts[0]) {
			return false
		}
	}
	return true
}

func nodeIndex(d *Diagram) map[int]Node {
	m := make(map[int]Node, len(d.Nodes))
	for _, n := range d.Nodes {
		m[n.ID] = n
	}
	return m
}

func (r *repairer) elementIndex(id int) int {
	for i := range r.d.Elements {
		if r.d.Elements[i].ID == id {
			return i
		}
	}
	return -1
}

func (r *repairer) node(id int) (Node, bool) {
	for _, n := range r.d.Nodes {
		if n.ID == id {
			return n, true
		}
	}
	return Node{}, false
}

func (r *repairer) portNodes() map[int]bool {
	set := map[int]bool{}
	for _, e := range r.d.Elements {
		for _, p := range e.Ports {
			set[p.Node] = true
		}
	}
	return set
}

func defaultConnectorName(kind ConnectorKind, id int, fallback string) string {
	switch kind {
	case KindOverheadLine:
		return fmt.Sprintf("Overhead line-%d", id)
	case KindCableLine:
		return fmt.Sprintf("Cable line-%d", id)
	case KindLinkToObject:
		return fmt.Sprintf("Object link-%d", id)
	}
	return fallback
}

// simplifyPath drops repeated points and the middle of three collinear axis-aligned points, as the editor's
// simplifyOrthogonalPath does. Always returns a new slice.
func simplifyPath(points []Point) []Point {
	if len(points) <= 2 {
		return append([]Point(nil), points...)
	}
	out := []Point{points[0]}
	for i := 1; i < len(points)-1; i++ {
		prev, curr, next := out[len(out)-1], points[i], points[i+1]
		if prev.X == curr.X && curr.X == next.X {
			continue
		}
		if prev.Y == curr.Y && curr.Y == next.Y {
			continue
		}
		if prev.X == curr.X && prev.Y == curr.Y {
			continue
		}
		out = append(out, curr)
	}
	last := points[len(points)-1]
	if second := out[len(out)-1]; second.X != last.X || second.Y != last.Y {
		out = append(out, last)
	}
	return out
}

// joinPoints is a followed by b, in a new slice.
func joinPoints(a []Point, b ...Point) []Point {
	out := make([]Point, 0, len(a)+len(b))
	return append(append(out, a...), b...)
}

// rerouteEnd moves one end of a wire's path to p, keeping the segment touching it orthogonal. Repeated points and
// points inside a straight run are dropped first, so the vertex that slides is a real bend.
func rerouteEnd(points []Point, from bool, p Point) []Point {
	if len(points) < 2 {
		return points
	}
	if simple := simplifyPath(points); len(simple) >= 2 {
		points = simple
	}
	n := len(points)
	if from {
		old := points[0]
		if n == 2 {
			other := points[1]
			bend := Point{X: other.X, Y: p.Y}
			if old.Y == other.Y {
				bend = Point{X: p.X, Y: other.Y}
			}
			return simplifyPath([]Point{p, bend, other})
		}
		nb := points[1]
		if old.Y == nb.Y {
			nb.Y = p.Y
		} else if old.X == nb.X {
			nb.X = p.X
		}
		return simplifyPath(joinPoints([]Point{p, nb}, points[2:]...))
	}
	old := points[n-1]
	if n == 2 {
		other := points[0]
		bend := Point{X: other.X, Y: p.Y}
		if old.Y == other.Y {
			bend = Point{X: p.X, Y: other.Y}
		}
		return simplifyPath([]Point{other, bend, p})
	}
	nb := points[n-2]
	if old.Y == nb.Y {
		nb.Y = p.Y
	} else if old.X == nb.X {
		nb.X = p.X
	}
	return simplifyPath(joinPoints(points[:n-2], nb, p))
}

// placeLocal maps a local template point through el's mirror, rotation and anchor, as Render places a symbol.
func placeLocal(el *Element, p Point) Point {
	mx := p.X
	if el.Mirror {
		mx = -p.X
	}
	rad := float64(el.Orient) * math.Pi / 180
	cos, sin := math.Cos(rad), math.Sin(rad)
	return Point{X: el.X + mx*cos - p.Y*sin, Y: el.Y + mx*sin + p.Y*cos}
}

// symbolTerminals is el's terminal positions in diagram coordinates, from terminals (by shape) or, for a power
// transformer, from its windings; nil when it has none.
func symbolTerminals(el *Element, terminals map[string][]Point) []Point {
	if el.Class == ClassPowerTransformer {
		count := max(2, len(el.Windings))
		var out []Point
		for i := range count {
			ox, oy := transformerWindingOffset(count, i)
			dir := defaultTerminal(count, i)
			if i < len(el.Windings) && el.Windings[i].Terminal != "" {
				dir = el.Windings[i].Terminal
			}
			x, y := transformerLegEndpoint(ox, oy, dir, transformerLeadLength)
			out = append(out, placeLocal(el, Point{X: x, Y: y}))
		}
		if el.Autotransformer {
			out = append(out, placeLocal(el, Point{X: 0, Y: transformerTapOffsetY}))
		}
		return out
	}
	ts := terminals[el.Shape]
	if len(ts) == 0 {
		return nil
	}
	scale := 1.0
	if el.Class == ClassFork && el.Radius != 0 {
		scale = el.Radius / forkArmLength
	}
	step := 0
	if UsesSizeStep(el.Class) {
		step = el.Scale
	}
	half := 0.0
	if HasLeads(el.Shape) && el.Span != 0 {
		half = el.Span / 2
	}
	out := make([]Point, len(ts))
	for i, t := range ts {
		x, y := scaledLength(t.X*scale, step), scaledLength(t.Y*scale, step)
		if half > math.Abs(y) && y != 0 {
			y = math.Copysign(half, y)
		}
		out[i] = placeLocal(el, Point{X: x, Y: y})
	}
	return out
}

// fixedTerminals is where el's fixed ports must sit, rounded, or nil when its ports aren't fixed.
func fixedTerminals(el *Element, terminals map[string][]Point) []Point {
	if el.Class == ClassBusBarSection {
		return nil
	}
	ts := symbolTerminals(el, terminals)
	for i := range ts {
		ts[i] = roundPt(ts[i])
	}
	return ts
}

// mergeNode replaces every reference to Node drop with keep and removes drop.
func (r *repairer) mergeNode(drop, keep int) {
	if drop == keep {
		return
	}
	d := r.d
	nodes := d.Nodes[:0]
	for _, n := range d.Nodes {
		if n.ID != drop {
			nodes = append(nodes, n)
		}
	}
	d.Nodes = nodes
	r.nodesChanged()
	for i := range d.Elements {
		for j := range d.Elements[i].Ports {
			if d.Elements[i].Ports[j].Node == drop {
				d.Elements[i].Ports[j].Node = keep
			}
		}
	}
	for i := range d.Connectors {
		if d.Connectors[i].From == drop {
			d.Connectors[i].From = keep
		}
		if d.Connectors[i].To == drop {
			d.Connectors[i].To = keep
		}
	}
}

// removeDegenerateConnectors removes every wire with fewer than two distinct points, joining its two end Nodes
// (keeping the one a device port uses), then drops Nodes nothing uses.
func (r *repairer) removeDegenerateConnectors() {
	d := r.d
	var bad []Connector
	kept := d.Connectors[:0:0]
	for _, c := range d.Connectors {
		if degenerate(c.Points) {
			bad = append(bad, c)
		} else {
			kept = append(kept, c)
		}
	}
	if len(bad) == 0 {
		return
	}
	d.Connectors = kept
	for _, c := range bad {
		ports := r.portNodes()
		keep, drop := c.From, c.To
		if !ports[c.From] && ports[c.To] {
			keep, drop = c.To, c.From
		}
		r.mergeNode(drop, keep)
	}
	used := r.portNodes()
	for _, c := range d.Connectors {
		used[c.From], used[c.To] = true, true
	}
	nodes := d.Nodes[:0]
	for _, n := range d.Nodes {
		if used[n.ID] {
			nodes = append(nodes, n)
		}
	}
	d.Nodes = nodes
	r.nodesChanged()
}

// moveNodesTo moves each Node in target onto its point, rerouting the wire ends on it.
func (r *repairer) moveNodesTo(target map[int]Point) {
	d := r.d
	for i := range d.Nodes {
		if p, ok := target[d.Nodes[i].ID]; ok {
			d.Nodes[i].X, d.Nodes[i].Y = p.X, p.Y
		}
	}
	r.nodesChanged()
	for i := range d.Connectors {
		c := &d.Connectors[i]
		from, okFrom := target[c.From]
		to, okTo := target[c.To]
		if okFrom && len(c.Points) > 0 {
			c.Points = rerouteEnd(c.Points, true, from)
		}
		if okTo && len(c.Points) > 0 {
			c.Points = rerouteEnd(c.Points, false, to)
		}
	}
}

// moveLegacyTerminalNodes moves each port Node lying on a legacy terminal position onto the matching current
// terminal, unless another element's port uses it too.
func (r *repairer) moveLegacyTerminalNodes() {
	d := r.d
	users := map[int]int{}
	for _, e := range d.Elements {
		for _, p := range e.Ports {
			users[p.Node]++
		}
	}
	nodes := nodeIndex(d)
	target := map[int]Point{}
	for i := range d.Elements {
		el := &d.Elements[i]
		legacy := legacyTerminals[el.Shape]
		if legacy == nil || len(el.Ports) == 0 {
			continue
		}
		current := fixedTerminals(el, r.terminals)
		if _, ok := r.terminals[el.Shape]; !ok || len(current) == 0 {
			continue
		}
		old := fixedTerminals(el, map[string][]Point{el.Shape: legacy})
		for _, port := range el.Ports {
			n, ok := nodes[port.Node]
			if !ok || users[port.Node] != 1 {
				continue
			}
			for k, t := range old {
				if math.Hypot(t.X-n.X, t.Y-n.Y) <= portMatchTolerance {
					if k < len(current) && !samePt(Point{X: n.X, Y: n.Y}, current[k]) {
						target[port.Node] = current[k]
					}
					break
				}
			}
		}
	}
	if len(target) > 0 {
		r.moveNodesTo(target)
	}
}

// fitElementPorts gives element id exactly its shape's fixed ports, each on a Node at its terminal: a port whose
// Node sits at a terminal keeps it, duplicates on one terminal are merged, a leftover port goes to the nearest free
// terminal (or joins its nearest terminal when unambiguously that one's), and a terminal with no port gets a fresh
// Node joined to whatever lies there. Existing Nodes are never moved. A busbar instead collects every Node on its
// line as a port.
func (r *repairer) fitElementPorts(id int) {
	d := r.d
	idx := r.elementIndex(id)
	if idx < 0 {
		return
	}
	el := d.Elements[idx]
	el.Ports = append([]Port(nil), el.Ports...)
	if el.Class == ClassBusBarSection {
		r.joinBusbar(idx)
		return
	}
	terminals := fixedTerminals(&el, r.terminals)
	if len(terminals) == 0 {
		return
	}
	nodes := r.nodeMap()
	slots := make([][]int, len(terminals))
	var leftover []int
	for _, port := range el.Ports {
		n, ok := nodes[port.Node]
		if !ok {
			continue
		}
		i := -1
		for k, t := range terminals {
			if math.Hypot(t.X-n.X, t.Y-n.Y) <= portMatchTolerance {
				i = k
				break
			}
		}
		if i >= 0 {
			if !containsInt(slots[i], port.Node) {
				slots[i] = append(slots[i], port.Node)
			}
		} else if !containsInt(leftover, port.Node) {
			leftover = append(leftover, port.Node)
		}
	}
	dist := func(nodeID, i int) float64 {
		n := nodes[nodeID]
		return math.Hypot(terminals[i].X-n.X, terminals[i].Y-n.Y)
	}
	type pair struct {
		node, i int
		d       float64
	}
	var pairs []pair
	for _, nodeID := range leftover {
		for i := range terminals {
			pairs = append(pairs, pair{nodeID, i, dist(nodeID, i)})
		}
	}
	sort.SliceStable(pairs, func(a, b int) bool { return pairs[a].d < pairs[b].d })
	placed := map[int]bool{}
	taken := map[int]bool{}
	for i, s := range slots {
		if len(s) > 0 {
			taken[i] = true
		}
	}
	for _, p := range pairs {
		if placed[p.node] || taken[p.i] {
			continue
		}
		slots[p.i] = append(slots[p.i], p.node)
		placed[p.node], taken[p.i] = true, true
	}
	minGap := math.Inf(1)
	for i := range terminals {
		for j := i + 1; j < len(terminals); j++ {
			minGap = math.Min(minGap, math.Hypot(terminals[i].X-terminals[j].X, terminals[i].Y-terminals[j].Y))
		}
	}
	for _, nodeID := range leftover {
		if placed[nodeID] {
			continue
		}
		best := 0
		for i := 1; i < len(terminals); i++ {
			if dist(nodeID, i) < dist(nodeID, best) {
				best = i
			}
		}
		if dist(nodeID, best) <= minGap/2 {
			slots[best] = append(slots[best], nodeID)
		}
	}

	added := false
	portNodes := make([]int, len(slots))
	for i, s := range slots {
		if len(s) == 0 {
			n := Node{ID: r.takeID(), X: terminals[i].X, Y: terminals[i].Y}
			d.Nodes = append(d.Nodes, n)
			r.nodesChanged()
			portNodes[i] = n.ID
			added = true
			continue
		}
		for _, drop := range s[1:] {
			r.mergeNode(drop, s[0])
		}
		portNodes[i] = s[0]
	}
	ports := make([]Port, len(portNodes))
	for i, n := range portNodes {
		ports[i] = Port{Name: fmt.Sprint(i + 1), Node: n}
	}
	same := len(el.Ports) == len(ports)
	for i := 0; same && i < len(ports); i++ {
		same = el.Ports[i] == ports[i]
	}
	if !same {
		d.Elements[idx].Ports = ports
	}
	if added {
		r.joinPortNodes(id)
	}
}

func containsInt(s []int, v int) bool {
	return slices.Contains(s, v)
}

// joinBusbar makes every Node lying on busbar idx's line that isn't one of its ports yet a new port of it.
func (r *repairer) joinBusbar(idx int) {
	d := r.d
	bus := &d.Elements[idx]
	if len(bus.Points) < 2 {
		return
	}
	own := map[int]bool{}
	for _, p := range bus.Ports {
		own[p.Node] = true
	}
	for _, n := range d.Nodes {
		if !own[n.ID] && segmentWith(bus.Points, Point{X: n.X, Y: n.Y}) >= 0 {
			bus.Ports = append(bus.Ports, Port{Name: fmt.Sprint(len(bus.Ports) + 1), Node: n.ID})
		}
	}
}

// joinPortNodes joins each of element id's port Nodes to whatever lies exactly there: another Node is merged into
// it, a wire passing through it (not at an end, not an overhead line, not touching this element) is split there,
// and a busbar passing through it gets a port on it.
func (r *repairer) joinPortNodes(id int) {
	d := r.d
	idx := r.elementIndex(id)
	if idx < 0 || len(d.Elements[idx].Ports) == 0 {
		return
	}
	ports := append([]Port(nil), d.Elements[idx].Ports...)
	own := map[int]bool{}
	for _, p := range ports {
		own[p.Node] = true
	}
	r.cutWireThroughDevice(ports)

	for _, port := range ports {
		node, ok := r.node(port.Node)
		if !ok {
			continue
		}
		at := Point{X: node.X, Y: node.Y}
		var others []int
		for _, n := range d.Nodes {
			if !own[n.ID] && samePt(Point{X: n.X, Y: n.Y}, at) {
				others = append(others, n.ID)
			}
		}
		for _, o := range others {
			r.mergeNode(o, node.ID)
		}

		var split []Connector
		touched := map[int]bool{}
		for _, c := range d.Connectors {
			if own[c.From] || own[c.To] || c.Kind == KindOverheadLine || len(c.Points) < 2 {
				continue
			}
			if samePt(c.Points[0], at) || samePt(c.Points[len(c.Points)-1], at) {
				continue
			}
			k := segmentWith(c.Points, at)
			if k < 0 {
				continue
			}
			before := simplifyPath(joinPoints(c.Points[:k+1], at))
			after := simplifyPath(joinPoints([]Point{at}, c.Points[k+1:]...))
			if degenerate(before) || degenerate(after) {
				continue
			}
			second := r.takeID()
			first := c
			first.To, first.Points, first.ToMRID = node.ID, before, ""
			tail := c
			tail.ID, tail.Name, tail.From, tail.Points = second, defaultConnectorName(c.Kind, second, c.Name), node.ID, after
			tail.MRID, tail.FromMRID = "", ""
			split = append(split, first, tail)
			touched[c.ID] = true
		}
		if len(touched) > 0 {
			kept := make([]Connector, 0, len(d.Connectors)+len(split))
			for _, c := range d.Connectors {
				if !touched[c.ID] {
					kept = append(kept, c)
				}
			}
			d.Connectors = append(kept, split...)
		}

		for i := range d.Elements {
			bus := &d.Elements[i]
			if bus.ID == id || bus.Class != ClassBusBarSection || len(bus.Points) < 2 {
				continue
			}
			has := false
			for _, p := range bus.Ports {
				has = has || p.Node == node.ID
			}
			if !has && segmentWith(bus.Points, at) >= 0 {
				bus.Ports = append(bus.Ports, Port{Name: fmt.Sprint(len(bus.Ports) + 1), Node: node.ID})
			}
		}
	}
}

// cutWireThroughDevice cuts a wire that a two-port device lies in, both terminals on one straight stretch of it:
// the piece under the device is removed, the head ending on the nearer terminal's Node and the tail starting on the
// other's.
func (r *repairer) cutWireThroughDevice(ports []Port) {
	d := r.d
	if len(ports) != 2 || ports[0].Node == ports[1].Node {
		return
	}
	n1, ok1 := r.node(ports[0].Node)
	n2, ok2 := r.node(ports[1].Node)
	if !ok1 || !ok2 {
		return
	}
	p1, p2 := Point{X: n1.X, Y: n1.Y}, Point{X: n2.X, Y: n2.Y}
	along := func(pts []Point, p Point) (float64, bool) {
		for i := 0; i+1 < len(pts); i++ {
			if onSeg(p, pts[i], pts[i+1]) {
				l := math.Hypot(pts[i+1].X-pts[i].X, pts[i+1].Y-pts[i].Y)
				if l == 0 {
					return float64(i), true
				}
				return float64(i) + math.Hypot(p.X-pts[i].X, p.Y-pts[i].Y)/l, true
			}
		}
		return 0, false
	}
	for ci, c := range d.Connectors {
		if c.From == n1.ID || c.From == n2.ID || c.To == n1.ID || c.To == n2.ID || c.Kind == KindOverheadLine || len(c.Points) < 2 {
			continue
		}
		pts := c.Points
		last := len(pts) - 1
		if samePt(pts[0], p1) || samePt(pts[0], p2) || samePt(pts[last], p1) || samePt(pts[last], p2) {
			continue
		}
		s1, ok1 := along(pts, p1)
		s2, ok2 := along(pts, p2)
		if !ok1 || !ok2 || s1 == s2 {
			continue
		}
		first, second, a, b := n1, n2, s1, s2
		if s2 < s1 {
			first, second, a, b = n2, n1, s2, s1
		}
		fa, fb := math.Floor(a), math.Floor(b)
		if fb-fa > 1 || (fb != fa && math.Mod(b, 1) != 0) {
			continue
		}
		head := simplifyPath(joinPoints(pts[:int(fa)+1], Point{X: first.X, Y: first.Y}))
		tail := simplifyPath(joinPoints([]Point{{X: second.X, Y: second.Y}}, pts[int(fb)+1:]...))
		tailID := r.takeID()
		second2 := c
		second2.ID, second2.Name, second2.From, second2.Points = tailID, defaultConnectorName(c.Kind, tailID, c.Name), second.ID, tail
		d.Connectors[ci].To, d.Connectors[ci].Points = first.ID, head
		d.Connectors = append(d.Connectors, second2)
		return
	}
}

// splitBusbarNodes gives each point where a wire end or device terminal meets a busbar its own port Node there.
// Extract (as xsde2svg draws it) joins everything along a busbar on one Node at the busbar's first point, while
// the editor's convention is one port per connection point, each on its own Node: a wire end or terminal on one of
// the busbar's port Nodes but lying elsewhere on its line moves to a Node at its own point (a busbar port Node
// already there, or a new one added as a port). Ends at the same point share it. The busbar stays one electrical
// node either way, so the topology is unchanged; dropUnusedNodes then removes a shared Node nothing uses any more.
func (r *repairer) splitBusbarNodes() {
	d := r.d
	for b := range d.Elements {
		if d.Elements[b].Class != ClassBusBarSection || len(d.Elements[b].Points) < 2 {
			continue
		}
		busID := d.Elements[b].ID
		line := d.Elements[b].Points
		nodes := nodeIndex(d)
		atPoint := map[Point]int{}
		isPort := map[int]bool{}
		for _, p := range d.Elements[b].Ports {
			isPort[p.Node] = true
			if n, ok := nodes[p.Node]; ok {
				atPoint[Point{X: n.X, Y: n.Y}] = n.ID
			}
		}
		nodeAt := func(q Point) int {
			if id, ok := atPoint[q]; ok {
				return id
			}
			n := Node{ID: r.takeID(), X: q.X, Y: q.Y}
			d.Nodes = append(d.Nodes, n)
			r.nodesChanged()
			bus := &d.Elements[b]
			bus.Ports = append(bus.Ports, Port{Name: fmt.Sprint(len(bus.Ports) + 1), Node: n.ID})
			atPoint[q], isPort[n.ID] = n.ID, true
			return n.ID
		}
		// moved reports whether q, an attachment of Node id, belongs on its own Node.
		moved := func(id int, q Point) bool {
			n, ok := nodes[id]
			return ok && isPort[id] && !samePt(q, Point{X: n.X, Y: n.Y}) && segmentWith(line, q) >= 0
		}
		for i := range d.Connectors {
			c := &d.Connectors[i]
			if len(c.Points) == 0 {
				continue
			}
			if q := c.Points[0]; moved(c.From, q) {
				c.From = nodeAt(q)
			}
			if q := c.Points[len(c.Points)-1]; moved(c.To, q) {
				c.To = nodeAt(q)
			}
		}
		for i := range d.Elements {
			el := &d.Elements[i]
			if el.ID == busID {
				continue
			}
			terminals := fixedTerminals(el, r.terminals)
			for k := range el.Ports {
				if k < len(terminals) && moved(el.Ports[k].Node, terminals[k]) {
					el.Ports[k].Node = nodeAt(terminals[k])
				}
			}
		}
	}
	// A new Node can also lie on another busbar touching this one: it becomes a port of that one too.
	for b := range d.Elements {
		if d.Elements[b].Class == ClassBusBarSection {
			r.joinBusbar(b)
		}
	}
}

// moveStrayPortNodes moves each port Node nothing else uses (no wire, no other port, no other Node on it) that
// lies away from its terminal onto that terminal, when no other port of the element is already there.
func (r *repairer) moveStrayPortNodes() {
	d := r.d
	uses := map[int]int{}
	for _, c := range d.Connectors {
		uses[c.From]++
		uses[c.To]++
	}
	for _, e := range d.Elements {
		for _, p := range e.Ports {
			uses[p.Node]++
		}
	}
	atPoint := map[Point]int{}
	for _, n := range d.Nodes {
		atPoint[Point{X: n.X, Y: n.Y}]++
	}
	nodes := nodeIndex(d)
	moved := map[int]Point{}
	for i := range d.Elements {
		el := &d.Elements[i]
		terminals := fixedTerminals(el, r.terminals)
		if len(terminals) == 0 || len(el.Ports) == 0 {
			continue
		}
		pos := make([]*Point, len(el.Ports))
		for k, p := range el.Ports {
			if m, ok := moved[p.Node]; ok {
				pos[k] = &m
			} else if n, ok := nodes[p.Node]; ok {
				pos[k] = &Point{X: n.X, Y: n.Y}
			}
		}
		for k, port := range el.Ports {
			p := pos[k]
			if p == nil || k >= len(terminals) || uses[port.Node] != 1 || atPoint[*p] > 1 {
				continue
			}
			t := terminals[k]
			at := func(q Point) bool { return math.Hypot(q.X-t.X, q.Y-t.Y) <= portMatchTolerance }
			if at(*p) {
				continue
			}
			busy := false
			for j, q := range pos {
				busy = busy || (j != k && q != nil && at(*q))
			}
			if busy {
				continue
			}
			moved[port.Node] = t
			tc := t
			pos[k] = &tc
		}
	}
	for i := range d.Nodes {
		if p, ok := moved[d.Nodes[i].ID]; ok {
			d.Nodes[i].X, d.Nodes[i].Y = p.X, p.Y
		}
	}
	r.nodesChanged()
}

// mergeNodesOntoPorts merges every Node that isn't a port but sits exactly on a port Node into it, unless a wire
// runs between the two.
func (r *repairer) mergeNodesOntoPorts() {
	d := r.d
	ports := r.portNodes()
	byPoint := map[Point]int{}
	for _, n := range d.Nodes {
		if ports[n.ID] {
			byPoint[Point{X: n.X, Y: n.Y}] = n.ID
		}
	}
	snapshot := append([]Node(nil), d.Nodes...)
	for _, n := range snapshot {
		if ports[n.ID] {
			continue
		}
		keep, ok := byPoint[Point{X: n.X, Y: n.Y}]
		if !ok {
			continue
		}
		loop := false
		for _, c := range d.Connectors {
			loop = loop || (c.From == n.ID && c.To == keep) || (c.To == n.ID && c.From == keep)
		}
		if !loop {
			r.mergeNode(n.ID, keep)
		}
	}
}

// splitWireAt splits wire c at point at on its segment k into two wires meeting on Node nodeID; the first keeps
// c's id and electrical parameters, the second gets none.
func (r *repairer) splitWireAt(c Connector, k int, at Point, nodeID int) {
	d := r.d
	second := r.takeID()
	for i := range d.Connectors {
		if d.Connectors[i].ID == c.ID {
			first := c
			first.To, first.Points, first.ToMRID = nodeID, simplifyPath(joinPoints(c.Points[:k+1], at)), ""
			d.Connectors[i] = first
		}
	}
	tail := c
	tail.ID, tail.Name, tail.From, tail.Electrical = second, defaultConnectorName(c.Kind, second, c.Name), nodeID, nil
	tail.MRID, tail.FromMRID = "", ""
	tail.Points = simplifyPath(joinPoints([]Point{at}, c.Points[k+1:]...))
	d.Connectors = append(d.Connectors, tail)
}

// tapWiresThroughJunctions splits every wire running through a junction point's dot, away from its ends, onto
// the junction's port Node; a segment lying along a busbar there is not cut.
func (r *repairer) tapWiresThroughJunctions() {
	d := r.d
	var busLines [][2]Point
	for _, e := range d.Elements {
		if e.Class == ClassBusBarSection {
			for i := 1; i < len(e.Points); i++ {
				busLines = append(busLines, [2]Point{e.Points[i-1], e.Points[i]})
			}
		}
	}
	alongBus := func(a, b Point) bool {
		for _, l := range busLines {
			if onSeg(a, l[0], l[1]) && onSeg(b, l[0], l[1]) {
				return true
			}
		}
		return false
	}
	var junctions []Element
	for _, e := range d.Elements {
		if e.Class == ClassJunctionPoint && len(e.Ports) > 0 {
			junctions = append(junctions, e)
		}
	}
	for _, j := range junctions {
		nodeID := j.Ports[0].Node
		at := Point{X: j.X, Y: j.Y}
		snapshot := append([]Connector(nil), d.Connectors...)
		for _, c := range snapshot {
			if len(c.Points) < 2 || c.From == nodeID || c.To == nodeID {
				continue
			}
			if samePt(c.Points[0], at) || samePt(c.Points[len(c.Points)-1], at) {
				continue
			}
			if k := segmentWith(c.Points, at); k >= 0 && !alongBus(c.Points[k], c.Points[k+1]) {
				r.splitWireAt(c, k, at, nodeID)
			}
		}
	}
}

// tapWireNodesOntoWires joins each Node carrying a wire end that lies on another wire's line, away from its ends,
// to that wire. A busbar's Node, or one on a busbar's line, is left alone, and a wire ending on another port of a
// device at the Node is never tapped.
func (r *repairer) tapWireNodesOntoWires() {
	d := r.d
	var buses []Element
	busNodes := map[int]bool{}
	for _, e := range d.Elements {
		if e.Class == ClassBusBarSection {
			buses = append(buses, e)
			for _, p := range e.Ports {
				busNodes[p.Node] = true
			}
		}
	}
	nodes := nodeIndex(d)
	onBus := func(p Point) bool {
		for _, b := range buses {
			if segmentWith(b.Points, p) >= 0 {
				return true
			}
		}
		return false
	}
	first := map[int]Node{}
	for _, n := range d.Nodes {
		if _, ok := first[n.ID]; !ok {
			first[n.ID] = n
		}
	}
	owners := map[int][]int{}
	for i, e := range d.Elements {
		for _, p := range e.Ports {
			if o := owners[p.Node]; len(o) == 0 || o[len(o)-1] != i {
				owners[p.Node] = append(o, i)
			}
		}
	}
	var ends []int
	seen := map[int]bool{}
	for _, c := range d.Connectors {
		for _, n := range []int{c.From, c.To} {
			if !seen[n] {
				seen[n] = true
				ends = append(ends, n)
			}
		}
	}
	wires := newWireGrid(d.Connectors)
	for _, id := range ends {
		n, ok := nodes[id]
		if !ok || busNodes[id] || onBus(Point{X: n.X, Y: n.Y}) {
			continue
		}
		avoid := map[int]bool{}
		for _, i := range owners[id] {
			for _, p := range d.Elements[i].Ports {
				if p.Node != id {
					avoid[p.Node] = true
				}
			}
		}
		if at, ok := first[id]; ok {
			r.tapWireAt(Point{X: at.X, Y: at.Y}, id, avoid, wires)
		}
	}
}

// tapWireAt splits the first wire (not an overhead line, not ending on Node nodeID or on a Node in avoid) whose
// line passes through at, Node nodeID's point, away from its ends, into two wires meeting on that Node. wires
// indexes r.d.Connectors and is kept up to date.
func (r *repairer) tapWireAt(at Point, nodeID int, avoid map[int]bool, wires *wireGrid) {
	for _, i := range wires.near(at) {
		c := r.d.Connectors[i]
		if c.Kind == KindOverheadLine || len(c.Points) < 2 || c.From == nodeID || c.To == nodeID || avoid[c.From] || avoid[c.To] {
			continue
		}
		if samePt(c.Points[0], at) || samePt(c.Points[len(c.Points)-1], at) {
			continue
		}
		if k := segmentWith(c.Points, at); k >= 0 {
			r.splitWireAt(c, k, at, nodeID)
			wires.add(len(r.d.Connectors)-1, r.d.Connectors[len(r.d.Connectors)-1].Points)
			return
		}
	}
}

// wireGridCell is the side of a wireGrid cell, in diagram units.
const wireGridCell = 64

// wireGrid finds the wires that may pass through a point: each wire's index in the connector list is filed under
// every cell its bounding box (widened by twice repairEpsilon) overlaps. A wire split in place keeps its old,
// larger box, which only adds candidates.
type wireGrid struct {
	cells map[[2]int][]int
}

func newWireGrid(connectors []Connector) *wireGrid {
	g := &wireGrid{cells: map[[2]int][]int{}}
	for i, c := range connectors {
		g.add(i, c.Points)
	}
	return g
}

func wireGridKey(v float64) int {
	return int(math.Floor(v / wireGridCell))
}

// add files wire i, whose points are pts; i must be above every index added before.
func (g *wireGrid) add(i int, pts []Point) {
	if len(pts) == 0 {
		return
	}
	minX, minY, maxX, maxY := pts[0].X, pts[0].Y, pts[0].X, pts[0].Y
	for _, p := range pts[1:] {
		minX, minY = math.Min(minX, p.X), math.Min(minY, p.Y)
		maxX, maxY = math.Max(maxX, p.X), math.Max(maxY, p.Y)
	}
	const pad = 2 * repairEpsilon
	for x := wireGridKey(minX - pad); x <= wireGridKey(maxX+pad); x++ {
		for y := wireGridKey(minY - pad); y <= wireGridKey(maxY+pad); y++ {
			g.cells[[2]int{x, y}] = append(g.cells[[2]int{x, y}], i)
		}
	}
}

// near returns, in ascending order, the indices of the wires whose box may contain p.
func (g *wireGrid) near(p Point) []int {
	return g.cells[[2]int{wireGridKey(p.X), wireGridKey(p.Y)}]
}

// dropUnusedNodes drops a busbar's ports whose Node nothing else uses (renumbering the rest) and every Node nothing
// uses.
func (r *repairer) dropUnusedNodes() {
	d := r.d
	uses := map[int]int{}
	for _, e := range d.Elements {
		for _, p := range e.Ports {
			uses[p.Node]++
		}
	}
	for _, c := range d.Connectors {
		uses[c.From]++
		uses[c.To]++
	}
	for i := range d.Elements {
		e := &d.Elements[i]
		if e.Class != ClassBusBarSection {
			continue
		}
		empty := false
		for _, p := range e.Ports {
			empty = empty || uses[p.Node] == 1
		}
		if !empty {
			continue
		}
		var kept []Port
		for _, p := range e.Ports {
			if uses[p.Node] == 1 {
				delete(uses, p.Node)
			} else {
				kept = append(kept, p)
			}
		}
		for k := range kept {
			kept[k].Name = fmt.Sprint(k + 1)
		}
		e.Ports = kept
	}
	nodes := d.Nodes[:0]
	for _, n := range d.Nodes {
		if uses[n.ID] > 0 {
			nodes = append(nodes, n)
		}
	}
	d.Nodes = nodes
	r.nodesChanged()
}

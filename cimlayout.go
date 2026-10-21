package slddoc

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strconv"
)

// Auto-layout for a CIM model without a Diagram Layout: one page per Substation, plus one for equipment outside
// any substation. Each voltage level is a band, highest voltage at the top: its busbars on one row, and its bays
// (the chains of devices hanging off a busbar's node) as columns above or below it. A bay reaching a transformer of a
// higher voltage, or the top band's bays reaching a line, go up; the rest go down. Transformers sit on a row between
// the band of their highest winding and the next, loads and other end devices end the bays below. The layout is
// written as ordinary DiagramObjects of a generated Diagram, so page builds it like any other layout, wiring
// included. A voltage level without a busbar hangs its bays off its busiest node instead.

const (
	layoutColumns = 6 // grid steps between bay columns
	layoutLevel   = 8 // grid steps between devices down a bay
)

type layNode struct {
	eq       *cimObject
	from     string     // the node it hangs from
	line     bool       // an AC line segment drawn as a stub
	trans    *cimObject // a transformer this bay reaches (eq nil)
	busStop  string     // a busbar node this bay reaches (eq nil)
	loop     string     // a node already laid out that this bay reaches again, closing a ring (eq nil)
	children []*layNode
	width    int
	depth    int
	links    int // transformers and busbars its subtree reaches
}

// layLink is a connection the layout draws itself through a corridor: from at (a bay end) to a transformer winding
// on node cn, or to the busbar on node cn.
type layLink struct {
	cn       string
	at       Point
	trans    *cimObject
	loop     bool
	corridor float64
}

type cimLayout struct {
	im       *cimImporter
	grid     float64
	page     map[string]bool         // equipment ids on this page
	cnPage   map[string]bool         // ConnectivityNode ids on this page
	atCN     map[string][]*cimObject // ConnectivityNode → equipment with a terminal on it
	visited  map[string]bool
	busCN    map[string]bool
	vlKV     map[string]float64 // voltage level key → kV
	diagram  *cimObject
	transTop map[string]string // transformer id → voltage level key of its highest winding
	links    []layLink
	busLine  map[string][3]float64    // busbar node → its line's x1, x2 and y
	grown    map[string]bool          // nodes already expanded
	cnAt     map[string]Point         // node → the point it was first drawn at
	cnOut    map[string]float64       // node → the side (+1 below, -1 above) its own wires leave that point
	sideUsed map[float64][][2]float64 // x → the y spans of the vertical runs links take there
	margin   float64
}

// autoLayout returns a generated Diagram per page.
func (im *cimImporter) autoLayout() []*cimObject {
	atCN := map[string][]*cimObject{}
	var equipment []*cimObject
	for _, list := range im.byClass {
		for _, o := range list {
			if _, isEq := im.terminals[o.id]; !isEq {
				continue
			}
			if _, drawn := cimEquipmentShapes[o.class]; !drawn && o.class != "ACLineSegment" {
				continue
			}
			equipment = append(equipment, o)
			for _, t := range im.terminals[o.id] {
				if cn := t.get("ConnectivityNode"); cn != "" {
					atCN[cn] = append(atCN[cn], o)
				}
			}
		}
	}
	for _, o := range im.auxiliary() {
		equipment = append(equipment, o)
		if t := im.objs[o.get("Terminal")]; t != nil && t.get("ConnectivityNode") != "" {
			cn := t.get("ConnectivityNode")
			atCN[cn] = append(atCN[cn], o)
		}
	}
	byName := func(a, b *cimObject) int {
		return cmp.Or(cmp.Compare(a.get("name"), b.get("name")), cmp.Compare(a.id, b.id))
	}
	slices.SortFunc(equipment, byName)
	for _, list := range atCN {
		slices.SortFunc(list, byName)
	}

	// Pages: a substation's equipment; a line belongs to the substations at its ends, drawn there as a stub.
	pages := map[string]map[string]bool{}
	var order []string
	add := func(page, eq string) {
		if pages[page] == nil {
			pages[page] = map[string]bool{}
			order = append(order, page)
		}
		pages[page][eq] = true
	}
	for _, o := range equipment {
		subs := map[string]bool{}
		for _, cn := range im.equipmentCNs(o) {
			subs[im.substationOfCN(cn)] = true
		}
		if s := im.substationOf(o); s != "" && o.class != "ACLineSegment" {
			subs = map[string]bool{s: true}
		}
		if len(subs) == 0 {
			subs[""] = true
		}
		for s := range subs {
			add(s, o.id)
		}
	}
	slices.SortStableFunc(order, func(a, b string) int {
		return cmp.Compare(im.objs[a].get("name"), im.objs[b].get("name"))
	})
	var diagrams []*cimObject
	for _, sub := range order {
		title := "Network"
		if o := im.objs[sub]; o != nil && o.get("name") != "" {
			title = o.get("name")
		}
		l := &cimLayout{im: im, grid: im.opt.Grid, page: pages[sub], cnPage: map[string]bool{}, atCN: atCN,
			visited: map[string]bool{}, busCN: map[string]bool{}, vlKV: map[string]float64{},
			transTop: map[string]string{}, busLine: map[string][3]float64{}, grown: map[string]bool{}, cnAt: map[string]Point{}, cnOut: map[string]float64{},
			sideUsed: map[float64][][2]float64{}}
		for cn := range atCN {
			if im.substationOfCN(cn) == sub {
				l.cnPage[cn] = true
			}
		}
		l.diagram = im.synthetic("Diagram", map[string]string{"name": title, "synthetic": "true"})
		l.layout()
		diagrams = append(diagrams, l.diagram)
	}
	return diagrams
}

// auxiliary is every auxiliary device (attached to a terminal rather than having its own).
func (im *cimImporter) auxiliary() []*cimObject {
	var out []*cimObject
	for class := range cimAuxiliaryClasses {
		out = append(out, im.byClass[class]...)
	}
	return out
}

func (im *cimImporter) equipmentCNs(o *cimObject) []string {
	var out []string
	if t := im.objs[o.get("Terminal")]; t != nil && cimAuxiliaryClasses[o.class] {
		return []string{t.get("ConnectivityNode")}
	}
	for _, t := range im.terminals[o.id] {
		if cn := t.get("ConnectivityNode"); cn != "" {
			out = append(out, cn)
		}
	}
	return out
}

// containerSubstation follows a container (Bay → VoltageLevel → Substation) to its substation id.
func (im *cimImporter) containerSubstation(id string) string {
	for range 4 {
		o := im.objs[id]
		if o == nil {
			return ""
		}
		switch o.class {
		case "Substation":
			return o.id
		case "VoltageLevel":
			id = o.get("Substation")
		case "Bay":
			id = o.get("VoltageLevel")
		default:
			return ""
		}
	}
	return ""
}

func (im *cimImporter) substationOf(o *cimObject) string {
	return im.containerSubstation(o.get("EquipmentContainer"))
}

func (im *cimImporter) substationOfCN(cn string) string {
	if o := im.objs[cn]; o != nil {
		return im.containerSubstation(o.get("ConnectivityNodeContainer"))
	}
	return ""
}

// synthetic adds a generated object of class with literal properties.
func (im *cimImporter) synthetic(class string, props map[string]string) *cimObject {
	o := &cimObject{id: fmt.Sprintf("auto-%d", len(im.objs)), class: class, props: map[string][]string{}}
	for k, v := range props {
		o.props[k] = []string{v}
	}
	im.objs[o.id] = o
	im.byClass[class] = append(im.byClass[class], o)
	return o
}

// --- voltage levels

// vlOf is the key of o's voltage level: its container's (or its bay's), else its base voltage's, else that of its
// first node.
func (l *cimLayout) vlOf(o *cimObject) string {
	im := l.im
	if c := im.objs[o.get("EquipmentContainer")]; c != nil {
		if c.class == "Bay" {
			c = im.objs[c.get("VoltageLevel")]
		}
		if c != nil && c.class == "VoltageLevel" {
			return l.vlKey(c.id, c.get("BaseVoltage"))
		}
	}
	if bv := o.get("BaseVoltage"); bv != "" {
		return l.vlKey("bv:"+bv, bv)
	}
	for _, cn := range im.equipmentCNs(o) {
		if k := l.cnVL(cn); k != "" {
			return k
		}
	}
	return ""
}

func (l *cimLayout) cnVL(cn string) string {
	o := l.im.objs[cn]
	if o == nil {
		return ""
	}
	c := l.im.objs[o.get("ConnectivityNodeContainer")]
	if c != nil && c.class == "Bay" {
		c = l.im.objs[c.get("VoltageLevel")]
	}
	if c == nil || c.class != "VoltageLevel" {
		return ""
	}
	return l.vlKey(c.id, c.get("BaseVoltage"))
}

func (l *cimLayout) vlKey(key, bv string) string {
	if _, ok := l.vlKV[key]; !ok {
		kv, _ := l.im.objs[bv].num("nominalVoltage")
		l.vlKV[key] = kv
	}
	return key
}

// --- trees

// grow collects the bays hanging off node cn: each device there not yet placed, continued through its other
// terminals' nodes until a busbar's node, a transformer or the page's edge. A transformer comes first, so it stays
// in its parent's column.
func (l *cimLayout) grow(cn string) []*layNode {
	l.grown[cn] = true
	var out, trans []*layNode
	for _, eq := range l.atCN[cn] {
		if l.visited[eq.id] || !l.page[eq.id] || eq.class == "BusbarSection" {
			continue
		}
		if eq.class == "PowerTransformer" {
			trans = append(trans, &layNode{trans: eq, from: cn})
			continue
		}
		l.visited[eq.id] = true
		n := &layNode{eq: eq, from: cn}
		cns := l.im.equipmentCNs(eq)
		if eq.class == "ACLineSegment" && (len(cns) < 2 || !l.cnPage[cns[0]] || !l.cnPage[cns[1]]) {
			n.line = true
			out = append(out, n)
			continue
		}
		if !cimAuxiliaryClasses[eq.class] {
			for _, c := range cns {
				switch {
				case c == cn:
				case l.busCN[c]:
					n.children = append(n.children, &layNode{busStop: c, from: c})
				case l.grown[c]:
					n.children = append(n.children, &layNode{loop: c, from: c})
				default:
					n.children = append(n.children, l.grow(c)...)
				}
			}
		}
		out = append(out, n)
	}
	return append(trans, out...)
}

func measure(n *layNode) {
	n.width, n.depth, n.links = 0, 0, 0
	if n.trans != nil || n.busStop != "" || n.loop != "" {
		n.links = 1
	}
	for _, c := range n.children {
		measure(c)
		n.width += c.width
		n.depth = max(n.depth, c.depth)
		n.links += c.links
	}
	n.width = max(n.width, 1)
	if n.eq != nil {
		n.depth++
	}
}

// reaches reports whether n's subtree reaches a transformer whose highest winding is above vl, or a line.
func (l *cimLayout) reaches(n *layNode, vl string, line *bool) bool {
	if n.trans != nil {
		top := l.transTop[n.trans.id]
		return top != vl && l.vlKV[top] > l.vlKV[vl]
	}
	if n.line {
		*line = true
	}
	up := false
	for _, c := range n.children {
		up = l.reaches(c, vl, line) || up
	}
	return up
}

// --- the page

type layHub struct {
	bus  *cimObject // nil for a voltage level without a busbar
	cn   string     // the node a busbar-less hub hangs its bays from
	bays []*layNode
	up   []bool
	col  int // first column
	cols int
}

type layBand struct {
	vl                 string
	hubs               []*layHub
	upDepth, downDepth int
	upLinks, downLinks int
	top, busY, bottom  float64
	nextUp, nextDown   int // corridors taken
}

func (l *cimLayout) layout() {
	im := l.im
	vls := map[string]*layBand{}
	band := func(vl string) *layBand {
		if vls[vl] == nil {
			vls[vl] = &layBand{vl: vl}
		}
		return vls[vl]
	}
	var buses []*cimObject
	for _, eq := range im.byClass["BusbarSection"] {
		if l.page[eq.id] {
			buses = append(buses, eq)
			for _, cn := range im.equipmentCNs(eq) {
				l.busCN[cn] = true
			}
		}
	}
	slices.SortFunc(buses, func(a, b *cimObject) int { return cmp.Compare(a.get("name"), b.get("name")) })
	var trs []*cimObject
	for _, tr := range im.byClass["PowerTransformer"] {
		if !l.page[tr.id] {
			continue
		}
		trs = append(trs, tr)
		top, kv := "", math.Inf(-1)
		for _, cn := range im.equipmentCNs(tr) {
			if vl := l.cnVL(cn); vl != "" && l.vlKV[vl] > kv {
				top, kv = vl, l.vlKV[vl]
			}
		}
		if top == "" {
			top = l.vlOf(tr)
		}
		l.transTop[tr.id] = top
		band(top)
	}
	for _, bus := range buses {
		l.visited[bus.id] = true
		h := &layHub{bus: bus}
		for _, cn := range im.equipmentCNs(bus) {
			h.bays = append(h.bays, l.grow(cn)...)
		}
		band(l.vlOf(bus)).hubs = append(band(l.vlOf(bus)).hubs, h)
	}
	// Whatever no busbar reaches hangs off its busiest node, repeatedly.
	for {
		best, bestN := "", 0
		for cn, list := range l.atCN {
			if !l.cnPage[cn] && !l.anyOnPage(list) {
				continue
			}
			n := 0
			for _, eq := range list {
				if !l.visited[eq.id] && l.page[eq.id] && eq.class != "PowerTransformer" && eq.class != "BusbarSection" {
					n++
				}
			}
			if n > bestN || n == bestN && n > 0 && cn < best {
				best, bestN = cn, n
			}
		}
		if bestN == 0 {
			break
		}
		h := &layHub{cn: best, bays: l.grow(best)}
		vl := l.cnVL(best)
		if vl == "" {
			for _, n := range h.bays {
				if n.eq != nil {
					vl = l.vlOf(n.eq)
					break
				}
			}
		}
		band(vl).hubs = append(band(vl).hubs, h)
	}

	bands := make([]*layBand, 0, len(vls))
	for _, b := range vls {
		bands = append(bands, b)
	}
	slices.SortFunc(bands, func(a, b *layBand) int {
		return cmp.Or(cmp.Compare(l.vlKV[b.vl], l.vlKV[a.vl]), cmp.Compare(a.vl, b.vl))
	})

	g := l.grid
	colW, levelH := layoutColumns*g, layoutLevel*g
	margin := 6 * g
	l.margin = margin
	colX := func(col int) float64 { return margin + float64(col)*colW + colW/2 }

	// Columns, directions, depths and corridors.
	for bi, b := range bands {
		col := 0
		for _, h := range b.hubs {
			h.col = col
			for _, n := range h.bays {
				measure(n)
				line := false
				up := l.reaches(n, b.vl, &line) || bi == 0 && line && n.trans == nil
				h.up = append(h.up, up)
				h.cols += n.width
				if up {
					b.upDepth, b.upLinks = max(b.upDepth, n.depth), b.upLinks+n.links
				} else {
					b.downDepth, b.downLinks = max(b.downDepth, n.depth), b.downLinks+n.links
				}
			}
			h.cols = max(h.cols, 1)
			col += h.cols + 1
		}
	}

	// Bands top to bottom, each with a corridor row per link above and below it, and a transformer row after it.
	y := margin
	maxX := margin
	rows := map[string]float64{}
	for _, b := range bands {
		b.top = y + float64(b.upLinks+1)*g
		b.busY = b.top + float64(b.upDepth)*levelH + 2*g
		b.bottom = b.busY + float64(b.downDepth)*levelH + 2*g
		for _, h := range b.hubs {
			col := h.col
			for i, n := range h.bays {
				dir := 1.0
				if h.up[i] {
					dir = -1
				}
				hubCN := h.cn
				if h.bus != nil {
					hubCN = n.from
				}
				at := Point{X: colX(col), Y: b.busY}
				if _, seen := l.cnAt[hubCN]; !seen && hubCN != "" {
					l.cnAt[hubCN], l.cnOut[hubCN] = at, dir
				}
				if p, ok := l.place(n, b, col, 1, dir, at, colX); ok {
					l.wire(hubCN, []Point{at, p})
				}
				col += n.width
			}
			x1, x2 := colX(h.col)-colW/2+g, colX(h.col+h.cols-1)+colW/2-g
			switch {
			case h.bus != nil:
				l.object(h.bus, []Point{{X: x1, Y: b.busY}, {X: x2, Y: b.busY}}, 0)
				if name := h.bus.get("name"); name != "" {
					l.text(h.bus, name, Point{X: x1, Y: b.busY - g})
				}
				for _, cn := range im.equipmentCNs(h.bus) {
					l.busLine[cn] = [3]float64{x1, x2, b.busY}
				}
			case h.cols > 1 && im.objs[h.cn] != nil:
				// A drawn wire stands in for the busbar, so the bays drop straight onto it.
				l.object(im.objs[h.cn], []Point{{X: colX(h.col), Y: b.busY}, {X: colX(h.col + h.cols - 1), Y: b.busY}}, 0)
			}
			maxX = math.Max(maxX, colX(h.col+h.cols))
		}
		// Room for a transformer's leads and the wires coming in to them, clear of the corridors.
		rows[b.vl] = b.bottom + float64(b.downLinks+1)*g + 12*g
		y = rows[b.vl] + 13*g
	}

	// Transformers: on the row under the band of their highest winding, above the bays that reach them, turned so
	// winding 1 is on top.
	xs := map[string][]float64{}
	for _, k := range l.links {
		if k.trans != nil {
			xs[k.trans.id] = append(xs[k.trans.id], k.at.X)
		}
	}
	slices.SortFunc(trs, func(a, b *cimObject) int {
		return cmp.Or(cmp.Compare(meanOr(xs[a.id], math.Inf(1)), meanOr(xs[b.id], math.Inf(1))), cmp.Compare(a.get("name"), b.get("name")))
	})
	last := map[float64]float64{}
	terms := map[string][]Point{}  // transformer id → its winding terminals as placed
	centres := map[string]Point{}  // transformer id → its centre
	approaches := map[string]int{} // transformer id → links already brought in
	for _, tr := range trs {
		rowY, ok := rows[l.transTop[tr.id]]
		if !ok {
			rowY = y
		}
		probe := Element{Class: ClassPowerTransformer, Shape: "47", Y: rowY,
			Windings: make([]TransformerWinding, min(max(len(im.transformerEnds(tr)), 2), 4))}
		best, bestScore := 0, math.Inf(-1)
		for _, o := range []int{0, 90, 180, -90} {
			probe.Orient = o
			ts := symbolTerminals(&probe, l.im.opt.Terminals)
			score := 0.0
			for _, t := range ts[1:] {
				score += t.Y - ts[0].Y
			}
			if score > bestScore+1e-9 {
				best, bestScore = o, score
			}
		}
		probe.Orient = best
		// Its terminals' reach left and right of its centre, plus room for the wires coming in beside them.
		left, right := 0.0, 0.0
		for _, t := range symbolTerminals(&probe, l.im.opt.Terminals) {
			left, right = math.Min(left, t.X), math.Max(right, t.X)
		}
		room := 6 * g
		x := math.Round(meanOr(xs[tr.id], maxX+colW)/g) * g
		if prev, ok := last[rowY]; ok && x+left-room < prev {
			x = prev - left + room
		}
		last[rowY] = x + right + room
		maxX = math.Max(maxX, x+right+colW)
		probe.X = x
		terms[tr.id] = symbolTerminals(&probe, l.im.opt.Terminals)
		centres[tr.id] = Point{X: x, Y: rowY}
		l.object(tr, []Point{{X: x, Y: rowY}}, float64(best))
		if name := tr.get("name"); name != "" {
			l.text(tr, name, Point{X: x + 5*g, Y: rowY})
		}
	}

	// The links: down (or up) the bay's column into its corridor, along it, and into the winding or busbar.
	for _, k := range l.links {

		var end, turn Point
		switch {
		case k.loop:
			to, ok := l.cnAt[k.cn]
			if !ok {
				continue
			}
			// In from the side the node's own wires leave, where only they lie.
			ay := to.Y + l.cnOut[k.cn]*g
			side := l.side(to.X, k.corridor, ay, colW)
			l.wire(k.cn, []Point{k.at, {X: k.at.X, Y: k.corridor}, {X: side, Y: k.corridor}, {X: side, Y: ay}, {X: to.X, Y: ay}, to})
			continue
		case k.trans != nil:
			ts := terms[k.trans.id]
			w := -1
			for i, end := range im.transformerEnds(k.trans) {
				if t := im.objs[end.get("Terminal")]; t != nil && t.get("ConnectivityNode") == k.cn && i < len(ts) {
					w = i
				}
			}
			if w < 0 {
				continue
			}
			end = ts[w]
			c := centres[k.trans.id]
			if math.Abs(end.Y-c.Y) >= math.Abs(end.X-c.X) {
				// A lead up or down: come in vertically, from outside, at a height of its own.
				out := math.Copysign(1, end.Y-c.Y)
				approaches[k.trans.id]++
				ay := end.Y + out*float64(1+approaches[k.trans.id])*g
				side := l.side(end.X, k.corridor, ay, colW)
				l.wire(k.cn, []Point{k.at, {X: k.at.X, Y: k.corridor}, {X: side, Y: k.corridor}, {X: side, Y: ay}, {X: end.X, Y: ay}, end})
				continue
			}
			// A lead to the side: come down beyond it, at a distance of its own, and in horizontally.
			out := math.Copysign(1, end.X-c.X)
			ax := end.X
			for {
				approaches[k.trans.id]++
				ax = end.X + out*float64(1+approaches[k.trans.id])*g
				if !l.columnX(ax) && l.free(ax, k.corridor, end.Y) {
					break
				}
			}
			l.wire(k.cn, []Point{k.at, {X: k.at.X, Y: k.corridor}, {X: ax, Y: k.corridor}, {X: ax, Y: end.Y}, end})
			continue
		default:
			bl, ok := l.busLine[k.cn]
			if !ok {
				continue
			}
			x := bl[0]
			if k.at.X > bl[1] {
				x = bl[1]
			}
			end = Point{X: x, Y: bl[2]}
			turn = Point{X: x, Y: k.corridor}
			l.sideUsed[x] = append(l.sideUsed[x], [2]float64{math.Min(k.corridor, end.Y), math.Max(k.corridor, end.Y)})
		}
		l.wire(k.cn, []Point{k.at, {X: k.at.X, Y: k.corridor}, turn, end})
	}
}

// columnX reports whether x is a bay column's centre, where devices' terminals lie.
func (l *cimLayout) columnX(x float64) bool {
	colW := layoutColumns * l.grid
	return math.Mod(math.Abs(x-l.margin-colW/2), colW) == 0
}

// free reports whether a vertical run at x from lo to hi overlaps no other run there, and if so takes it.
func (l *cimLayout) free(x, y1, y2 float64) bool {
	lo, hi := math.Min(y1, y2), math.Max(y1, y2)
	for _, span := range l.sideUsed[x] {
		if span[0] <= hi && lo <= span[1] {
			return false
		}
	}
	l.sideUsed[x] = append(l.sideUsed[x], [2]float64{lo, hi})
	return true
}

// side picks an x within half a column of x, where no terminal lies, for a link's vertical run from y1 to y2 that
// no other run overlaps; the run then touches nothing on its way, nor does its short last leg to x.
func (l *cimLayout) side(x, y1, y2, colW float64) float64 {
	g := l.grid
	lo, hi := math.Min(y1, y2), math.Max(y1, y2)
	for _, off := range []float64{-3, 3, -2, 2, -4, 4, -1, 1, -5, 5} {
		cand := x + off*g
		if math.Abs(off*g) >= colW/2+g || l.columnX(cand) {
			continue
		}
		if l.free(cand, lo, hi) {
			return cand
		}
	}
	return x - colW/2
}

func (l *cimLayout) anyOnPage(list []*cimObject) bool {
	for _, eq := range list {
		if l.page[eq.id] {
			return true
		}
	}
	return false
}

func meanOr(xs []float64, or float64) float64 {
	if len(xs) == 0 {
		return or
	}
	s := 0.0
	for _, x := range xs {
		s += x
	}
	return s / float64(len(xs))
}

// place writes n's subtree at column col and level (1 = next to the bus), growing in direction dir (+1 down, -1
// up), and the wires within it; at is the point n's terminal toward the bus connects to. Returns where that terminal
// is drawn (false for a transformer, which is linked through a corridor instead).
func (l *cimLayout) place(n *layNode, b *layBand, col, level int, dir float64, at Point, colX func(int) float64) (Point, bool) {
	g := l.grid
	levelH := layoutLevel * g
	x := colX(col)
	corridor := func() float64 {
		if dir > 0 {
			b.nextDown++
			return b.bottom + float64(b.nextDown)*g
		}
		b.nextUp++
		return b.top - float64(b.nextUp)*g
	}
	if n.trans != nil {
		l.links = append(l.links, layLink{cn: n.from, at: at, trans: n.trans, corridor: corridor()})
		return Point{}, false
	}
	if n.busStop != "" || n.loop != "" {
		// Its own column, from just past the parent's terminal down to the corridor.
		start := Point{X: x, Y: at.Y + dir*g}
		l.links = append(l.links, layLink{cn: n.from, at: start, loop: n.loop != "", corridor: corridor()})
		return start, true
	}
	centre := b.busY + dir*(float64(level)-0.5)*levelH
	near := b.busY + dir*(float64(level-1)*levelH+g) // where the terminal facing the bus goes
	far := map[string]Point{}                        // the point each other node of n is drawn at
	facing := Point{X: x, Y: near}
	if n.line || n.eq.class == "ACLineSegment" {
		bottom := near + dir*(levelH-3*g)
		pts := []Point{{X: x, Y: near}, {X: x, Y: bottom}}
		if cns := l.im.equipmentCNs(n.eq); len(cns) > 1 && cns[1] == n.from {
			pts[0], pts[1] = pts[1], pts[0] // the points run from terminal 1 to terminal 2
		}
		l.object(n.eq, pts, 0)
		for _, c := range l.im.equipmentCNs(n.eq) {
			far[c] = Point{X: x, Y: bottom}
		}
	} else {
		ts, k := l.device(n, x, near, centre, dir)
		for i, c := range l.im.equipmentCNs(n.eq) {
			if i < len(ts) && i != k {
				far[c] = ts[i]
			}
		}
		if k >= 0 && k < len(ts) {
			facing = ts[k]
		}
	}
	if name := n.eq.get("name"); name != "" {
		l.text(n.eq, name, Point{X: x + 2*g, Y: centre})
	}
	c := col
	for _, ch := range n.children {
		p, ok := far[ch.from]
		if !ok {
			p = Point{X: x, Y: near + dir*(levelH-3*g)}
		}
		if _, seen := l.cnAt[ch.from]; !seen && ch.loop == "" && ch.busStop == "" {
			l.cnAt[ch.from], l.cnOut[ch.from] = p, dir
		}
		if cp, ok := l.place(ch, b, c, level+1, dir, p, colX); ok {
			// Straight down in the same column, else just past the parent's terminal and across.
			by := p.Y + dir*g
			l.wire(ch.from, []Point{p, {X: p.X, Y: by}, {X: cp.X, Y: by}, cp})
		}
		c += ch.width
	}
	return facing, true
}

// wire adds a drawn wire of node cn along pts.
func (l *cimLayout) wire(cn string, pts []Point) {

	pts = simplifyPath(pts)
	if o := l.im.objs[cn]; o != nil && len(pts) >= 2 {
		l.object(o, pts, 0)
	}
}

// device places a device of a bay: turned (0° or 180°) so the terminal facing the bus is on the bus side, and moved
// so that terminal sits at near; one without symbol terminals is centred at centre. Returns its terminals as placed
// and the facing one's index.
func (l *cimLayout) device(n *layNode, x, near, centre, dir float64) ([]Point, int) {
	def := cimEquipmentShapes[n.eq.class]
	ts := l.im.opt.Terminals[def.shape]
	facing := 0
	if !cimAuxiliaryClasses[n.eq.class] {
		facing = max(slices.Index(l.im.equipmentCNs(n.eq), n.from), 0)
	}
	if facing >= len(ts) {
		l.object(n.eq, []Point{{X: x, Y: centre}}, 0)
		return nil, -1
	}
	ty := ts[facing].Y
	if ty == 0 && len(ts) == 1 {
		ty = -1 // a terminal at the anchor: the body is below it
	}
	rot := 0
	if dir > 0 && ty > 0 || dir < 0 && ty < 0 {
		rot = 180
	}
	probe := Element{Class: def.class, Shape: def.shape, Orient: rot}
	placed := symbolTerminals(&probe, l.im.opt.Terminals)
	anchor := Point{X: x, Y: near - placed[facing].Y}
	l.object(n.eq, []Point{anchor}, float64(rot))
	for i := range placed {
		placed[i] = roundPt(Point{X: placed[i].X + anchor.X, Y: placed[i].Y + anchor.Y})
	}
	return placed, facing
}

// object adds a DiagramObject for eq with its points.
func (l *cimLayout) object(eq *cimObject, pts []Point, rotation float64) {
	do := l.im.synthetic("DiagramObject", map[string]string{
		"Diagram": l.diagram.id, "IdentifiedObject": eq.id, "rotation": strconv.FormatFloat(rotation, 'f', -1, 64)})
	l.im.dos[l.diagram.id] = append(l.im.dos[l.diagram.id], do)
	for i, p := range pts {
		pt := l.im.synthetic("DiagramObjectPoint", map[string]string{"DiagramObject": do.id, "sequenceNumber": strconv.Itoa(i + 1),
			"xPosition": strconv.FormatFloat(p.X, 'f', -1, 64), "yPosition": strconv.FormatFloat(p.Y, 'f', -1, 64)})
		l.im.points[do.id] = append(l.im.points[do.id], pt)
	}
}

func (l *cimLayout) text(eq *cimObject, text string, at Point) {
	do := l.im.synthetic("TextDiagramObject", map[string]string{"Diagram": l.diagram.id, "IdentifiedObject": eq.id, "text": text})
	l.im.dos[l.diagram.id] = append(l.im.dos[l.diagram.id], do)
	pt := l.im.synthetic("DiagramObjectPoint", map[string]string{"DiagramObject": do.id,
		"xPosition": strconv.FormatFloat(at.X, 'f', -1, 64), "yPosition": strconv.FormatFloat(at.Y, 'f', -1, 64)})
	l.im.points[do.id] = append(l.im.points[do.id], pt)
}

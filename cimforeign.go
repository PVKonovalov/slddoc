package slddoc

import (
	"fmt"
	"math"
	"slices"
)

// Layouts drawn by other tools (cimPageBuilder.foreign) may draw a busbar as the drawing of its ConnectivityNode, a
// terminal as a polyline from its device to its node, and a line only as the marker of the substation at its other
// end (a Substation DiagramObject), with device symbols of their own size.

// terminalWire takes terminal t, drawn by do as a polyline, as a drawn wire of its node; the polyline's end nearer
// the device's own drawing is where the layout draws the terminal.
func (b *cimPageBuilder) terminalWire(t, do *cimObject) {
	pts, cn := b.doPoints(do), t.get("ConnectivityNode")
	dev := b.doFor[t.get("ConductingEquipment")]
	if len(pts) < 2 || cn == "" || dev == nil {
		return
	}
	at := b.doPoints(dev)
	if len(at) == 0 {
		return
	}
	first := dist(pts[0], at[0]) <= dist(pts[len(pts)-1], at[0])
	if first {
		b.termDrawn[t.id] = pts[0]
	} else {
		b.termDrawn[t.id] = pts[len(pts)-1]
	}
	if _, ok := b.cnLayer[cn]; !ok {
		b.cnLayer[cn] = b.layer(do)
	}
	b.wires[cn] = append(b.wires[cn], pts)
	b.termWires = append(b.termWires, cimTermWire{cn: cn, idx: len(b.wires[cn]) - 1, first: first, term: t.id})
}

// junctionMarks returns whether drawing do of node cn has no end that another drawing of cn (the node's own, or a
// polyline of a terminal on it) ends at, in the layout's own coordinates: such a short bar only marks a junction.
func (b *cimPageBuilder) junctionMarks(dos []*cimObject) func(cn string, do *cimObject) bool {
	type end struct {
		cn string
		p  Point
	}
	raw := func(do *cimObject) []Point {
		var out []Point
		for _, p := range b.im.points[do.id] {
			x, _ := p.num("xPosition")
			y, _ := p.num("yPosition")
			out = append(out, Point{X: x, Y: y})
		}
		return out
	}
	ends := map[end]int{}
	for _, do := range dos {
		o := b.im.objs[do.get("IdentifiedObject")]
		cn := ""
		switch {
		case o == nil:
		case isNode(o):
			cn = o.id
		case o.class == "Terminal":
			cn = o.get("ConnectivityNode")
		}
		if pts := raw(do); cn != "" && len(pts) >= 2 {
			ends[end{cn, pts[0]}]++
			ends[end{cn, pts[len(pts)-1]}]++
		}
	}
	return func(cn string, do *cimObject) bool {
		pts := raw(do)
		return ends[end{cn, pts[0]}] < 2 && ends[end{cn, pts[len(pts)-1]}] < 2
	}
}

// undrawnBusbar is a BusbarSection on node cn that this page neither draws nor has placed yet, or nil.
func (b *cimPageBuilder) undrawnBusbar(cn string) *cimObject {
	for _, bb := range b.im.byClass["BusbarSection"] {
		if _, placed := b.itemOf[bb.id]; placed || b.doFor[bb.id] != nil {
			continue
		}
		if ts := b.im.terminals[bb.id]; len(ts) > 0 && ts[0].get("ConnectivityNode") == cn {
			return bb
		}
	}
	return nil
}

// face turns e by a quarter, half or three-quarter turn when that brings its terminals nearest targets (target i
// for terminal i; a zero one is ignored): another tool may draw a symbol on another axis. Returns whether any target
// was given.
func (b *cimPageBuilder) face(e *Element, targets []Point) bool {
	cost := func(probe *Element) (float64, bool) {
		w, any := 0.0, false
		for i, t := range symbolTerminals(probe, b.im.opt.Terminals) {
			if i < len(targets) && targets[i] != (Point{}) {
				w, any = w+dist(t, targets[i]), true
			}
		}
		return w, any
	}
	best, any := cost(e)
	if !any {
		return false
	}
	orig := *e
	for _, turn := range []int{180, 90, 270} {
		turned := orig
		turned.Orient = snapOrient(float64(orig.Orient + turn))
		if c, _ := cost(&turned); c < best-1e-6 {
			*e, best = turned, c
		}
	}
	return true
}

// faceConnections turns each device whose terminals the layout doesn't draw by 180° when that brings its terminals
// nearer what else is on their nodes, so no wire runs through it (a one-terminal device by faceAway); twice, as a
// turned neighbour moves the targets.
func (b *cimPageBuilder) faceConnections() {
	for range 2 {
		for i := range b.d.Elements {
			e := &b.d.Elements[i]
			pc := b.portCN[e.ID]
			if b.faced[e.ID] || e.Class == ClassBusBarSection || len(pc) == 0 || pc[0] == "" || b.portAt[e.ID] != nil {
				continue
			}
			if len(pc) == 1 {
				if b.faceAway(e, pc[0]) {
					b.reattach(e)
				}
				continue
			}
			cost := func(probe *Element) float64 {
				w := 0.0
				for k, t := range symbolTerminals(probe, b.im.opt.Terminals) {
					if k < len(pc) && pc[k] != "" {
						w += b.distToNode(pc[k], roundPt(t), e.ID)
					}
				}
				return w
			}
			turned := *e
			turned.Orient = snapOrient(float64(e.Orient + 180))
			if cost(&turned) >= cost(e)-1e-6 {
				continue
			}
			*e = turned
			b.reattach(e)
		}
	}
}

// faceAway turns one-terminal device e, whose symbol points at the nearest thing on its node cn so its wire would
// run over it, to the quarter, half or three-quarter turn pointing most away from it. Returns whether e turned.
func (b *cimPageBuilder) faceAway(e *Element, cn string) bool {
	local := b.im.opt.Terminals[e.Shape]
	if len(local) != 1 {
		return false
	}
	// toward is the cosine between the symbol's way from its terminal (to its anchor, or down its lead when the
	// terminal is the anchor) and the way to cn's nearest point.
	toward := func(probe *Element) float64 {
		t := roundPt(symbolTerminals(probe, b.im.opt.Terminals)[0])
		body := Point{X: probe.X, Y: probe.Y}
		if local[0] == (Point{}) {
			body = placeLocal(probe, Point{Y: 1})
		}
		q, ok := b.nearestOnNode(cn, t, probe.ID)
		bl, ql := dist(t, body), dist(t, q)
		if !ok || bl == 0 || ql == 0 {
			return 0
		}
		return ((body.X-t.X)*(q.X-t.X) + (body.Y-t.Y)*(q.Y-t.Y)) / (bl * ql)
	}
	best, bestCos := *e, toward(e)
	if bestCos <= 1e-6 {
		return false
	}
	for _, turn := range []int{180, 90, 270} {
		turned := *e
		turned.Orient = snapOrient(float64(e.Orient + turn))
		if c := toward(&turned); c < bestCos-1e-6 {
			best, bestCos = turned, c
		}
	}
	if best.Orient == e.Orient {
		return false
	}
	*e = best
	return true
}

// reattach moves what element e's ports must reach to where its terminals now are.
func (b *cimPageBuilder) reattach(e *Element) {
	pc := b.portCN[e.ID]
	for _, cn := range pc {
		b.attach[cn] = slices.DeleteFunc(b.attach[cn], func(a cimAttach) bool { return a.owner == e.ID })
	}
	for k, t := range symbolTerminals(e, b.im.opt.Terminals) {
		if k < len(pc) && pc[k] != "" {
			b.attach[pc[k]] = append(b.attach[pc[k]], cimAttach{at: roundPt(t), owner: e.ID})
		}
	}
}

// dropOverdrawn leaves to the routing each drawn terminal polyline that runs over another device or a terminal of
// another node, and turns each device that faced such a terminal again by what the layout still draws of it.
func (b *cimPageBuilder) dropOverdrawn() {
	refacing := map[int]*cimObject{}
	for _, tw := range b.termWires {
		t := b.im.objs[tw.term]
		if w := b.wires[tw.cn][tw.idx]; w == nil || !b.runsOverOthers(tw.cn, w, b.itemOf[t.get("ConductingEquipment")]) {
			continue
		}
		b.wires[tw.cn][tw.idx] = nil
		delete(b.termDrawn, t.id)
		if id, ok := b.itemOf[t.get("ConductingEquipment")]; ok {
			refacing[id] = b.im.objs[t.get("ConductingEquipment")]
		}
	}
	for i := range b.d.Elements {
		e := &b.d.Elements[i]
		if obj := refacing[e.ID]; obj != nil {
			e.Orient = b.baseOrient[e.ID]
			b.faced[e.ID] = b.face(e, b.terminalPoints(obj, e.Class))
			b.reattach(e)
		}
	}
}

// distToNode is how far p is from the nearest thing on node cn other than element self's ports (0 when there is
// none).
func (b *cimPageBuilder) distToNode(cn string, p Point, self int) float64 {
	q, ok := b.nearestOnNode(cn, p, self)
	if !ok {
		return 0
	}
	return dist(p, q)
}

// nearestOnNode is the point nearest p of what is on node cn other than element self's ports: a port or line end, a
// drawn wire or a busbar; false when there is none.
func (b *cimPageBuilder) nearestOnNode(cn string, p Point, self int) (Point, bool) {
	best, bd := Point{}, math.Inf(1)
	try := func(q Point) {
		if d := dist(p, q); d < bd {
			best, bd = q, d
		}
	}
	for _, a := range b.attach[cn] {
		if a.owner != self {
			try(a.at)
		}
	}
	for _, w := range b.wires[cn] {
		for k := 1; k < len(w); k++ {
			try(nearestOnSegment(p, w[k-1], w[k]))
		}
	}
	for _, s := range b.busbars[cn] {
		try(nearestOnSegment(p, s[0], s[1]))
	}
	return best, !math.IsInf(bd, 1)
}

// snapTerminalWires moves the device end of each drawn terminal polyline onto the terminal of the device's symbol,
// with a bend where the wire would otherwise turn oblique.
func (b *cimPageBuilder) snapTerminalWires() {
	for _, tw := range b.termWires {
		if b.wires[tw.cn][tw.idx] == nil {
			continue
		}
		t := b.im.objs[tw.term]
		obj := b.im.objs[t.get("ConductingEquipment")]
		id, ok := b.itemOf[obj.id]
		k := slices.IndexFunc(b.d.Elements, func(e Element) bool { return e.ID == id })
		if !ok || k < 0 {
			continue
		}
		e := &b.d.Elements[k]
		port := slices.Index(b.portTerminals(obj, e.Class), t)
		ts := b.portAt[e.ID]
		if ts == nil {
			ts = symbolTerminals(e, b.im.opt.Terminals)
		}
		if port < 0 || port >= len(ts) {
			continue
		}
		w := slices.Clone(b.wires[tw.cn][tw.idx])
		if !tw.first {
			slices.Reverse(w)
		}
		p, end, next := roundPt(ts[port]), w[0], w[1]
		path := []Point{p}
		if p.X != next.X && p.Y != next.Y {
			if end.X == next.X {
				path = append(path, Point{X: p.X, Y: next.Y})
			} else {
				path = append(path, Point{X: next.X, Y: p.Y})
			}
		}
		w = simplifyPath(append(path, w[1:]...))
		if !tw.first {
			slices.Reverse(w)
		}
		b.wires[tw.cn][tw.idx] = w
	}
}

// runsOverOthers reports whether wire w of node cn, drawn for a terminal of element self, passes another symbol or a
// port of another node, its own ends and self's own ports aside.
func (b *cimPageBuilder) runsOverOthers(cn string, w []Point, self int) bool {
	ends := []Point{w[0], w[len(w)-1]}
	for k := 1; k < len(w); k++ {
		a, z := w[k-1], w[k]
		for other, as := range b.attach {
			for _, at := range as {
				if other != cn && at.owner != self && !slices.Contains(ends, at.at) && onSeg(at.at, a, z) {
					return true
				}
			}
		}
		for _, e := range b.d.Elements {
			c := Point{X: e.X, Y: e.Y}
			if e.ID != self && e.Class != ClassBusBarSection && !slices.Contains(ends, c) && segDist(c, a, z) < b.im.opt.Grid/2 &&
				!slices.Contains(b.portCN[e.ID], cn) {
				return true
			}
		}
	}
	return false
}

// undrawnLines draws each AC line segment the layout leaves out whose end is on a device of this page: from that
// device's port to the marker of the substation at the line's other end when the layout draws one, else as a stub
// of four grid steps out of the device, labelled with the line's name.
func (b *cimPageBuilder) undrawnLines(dos []*cimObject) {
	markers := map[string][]Point{}
	for _, do := range dos {
		if o := b.im.objs[do.get("IdentifiedObject")]; o != nil && o.class == "Substation" {
			if pts := b.doPoints(do); len(pts) > 0 {
				markers[o.id] = append(markers[o.id], pts[0])
			}
		}
	}
	// marker takes the free marker of substation st nearest p.
	marker := func(st string, p Point) (Point, bool) {
		ms := markers[st]
		best := -1
		for i, m := range ms {
			if best < 0 || dist(m, p) < dist(ms[best], p) {
				best = i
			}
		}
		if best < 0 {
			return Point{}, false
		}
		m := ms[best]
		markers[st] = slices.Delete(ms, best, best+1)
		return m, true
	}
	drawn := 0
	for _, ln := range b.im.byClass["ACLineSegment"] {
		ts := b.im.terminals[ln.id]
		if _, placed := b.itemOf[ln.id]; placed || len(ts) < 2 {
			continue
		}
		for k, t := range ts[:2] {
			cn := t.get("ConnectivityNode")
			start, owner, ok := b.lineStart(cn)
			if !ok {
				continue
			}
			ei := slices.IndexFunc(b.d.Elements, func(e Element) bool { return e.ID == owner })
			el := b.d.Elements[ei]
			out := axis(Point{X: start.X - el.X, Y: start.Y - el.Y})
			m, toMarker := marker(b.im.substationOfCN(ts[1-k].get("ConnectivityNode")), start)
			pts := b.lineRoute(start, out, m, toMarker)
			c := b.lineConnector(ln, pts, el.Layer, KindOverheadLine)
			b.connCN[c.ID] = [2]string{cn, ""}
			b.attach[cn] = append(b.attach[cn], cimAttach{at: start})
			b.d.Connectors = append(b.d.Connectors, c)
			if _, ok := b.itemOf[ln.id]; !ok {
				b.itemOf[ln.id] = c.ID
			}
			b.report.Connectors++
			drawn++
			if name := ln.get("name"); name != "" {
				end := pts[len(pts)-1]
				b.lineLabel(c, name, end, axis(Point{X: end.X - pts[len(pts)-2].X, Y: end.Y - pts[len(pts)-2].Y}))
			}
		}
	}
	if drawn > 0 {
		b.messages = append(b.messages, fmt.Sprintf("%d line ends the layout doesn't draw were drawn from their device", drawn))
	}
}

// lineRoute is the path of a line leaving a device's port at start, out of the device along out: straight to the
// level of marker m when it lies ahead and nothing is in the way, else across to it and then to it; with no marker, a stub of four grid steps
// out, or to either side when that runs over nothing while out doesn't.
func (b *cimPageBuilder) lineRoute(start, out, m Point, toMarker bool) []Point {
	g := b.im.opt.Grid
	if toMarker {
		if along := (m.X-start.X)*out.X + (m.Y-start.Y)*out.Y; along > g {
			if end := (Point{X: start.X + along*out.X, Y: start.Y + along*out.Y}); b.runsClear(start, end) {
				return []Point{start, end}
			}
		}
		bend := Point{X: m.X, Y: start.Y}
		if out.Y == 0 {
			bend = Point{X: start.X, Y: m.Y}
		}
		return simplifyPath([]Point{start, bend, m})
	}
	side := Point{X: -out.Y, Y: out.X}
	for _, dir := range []Point{out, side, {X: -side.X, Y: -side.Y}} {
		end := Point{X: start.X + 4*g*dir.X, Y: start.Y + 4*g*dir.Y}
		if b.runsClear(start, end) {
			return []Point{start, end}
		}
	}
	return []Point{start, {X: start.X + 4*g*out.X, Y: start.Y + 4*g*out.Y}}
}

// runsClear reports whether segment a–z (a excluded) meets no wire, line, busbar or terminal and passes no symbol.
func (b *cimPageBuilder) runsClear(a, z Point) bool {
	hit := func(p Point) bool { return p != a && onSeg(p, a, z) }
	for _, c := range b.d.Connectors {
		for k := 1; k < len(c.Points); k++ {
			if hit(c.Points[k-1]) || hit(c.Points[k]) || segDist(c.Points[k-1], a, z) < 1 && segDist(c.Points[k], a, z) < 1 {
				return false
			}
			if _, ok := properCrossing(a, z, c.Points[k-1], c.Points[k]); ok {
				return false
			}
		}
	}
	for _, ws := range b.wires {
		for _, w := range ws {
			for k := 1; k < len(w); k++ {
				if hit(w[k-1]) || hit(w[k]) {
					return false
				}
				if _, ok := properCrossing(a, z, w[k-1], w[k]); ok {
					return false
				}
			}
		}
	}
	for _, segs := range b.busbars {
		for _, sg := range segs {
			if _, ok := properCrossing(a, z, sg[0], sg[1]); ok || hit(sg[0]) || hit(sg[1]) {
				return false
			}
		}
	}
	for _, as := range b.attach {
		for _, at := range as {
			if hit(at.at) {
				return false
			}
		}
	}
	for _, e := range b.d.Elements {
		if e.Class != ClassBusBarSection && segDist(Point{X: e.X, Y: e.Y}, a, z) < b.im.opt.Grid/2 {
			return false
		}
	}
	return true
}

// lineLabel names line c beyond its end, moved further out while it would overlap a label placed before.
func (b *cimPageBuilder) lineLabel(c Connector, name string, end, out Point) {
	const size = 10
	g := b.im.opt.Grid
	w, h := 0.6*size*float64(len([]rune(name))), float64(size)
	l := Label{ID: b.take(), For: c.ID, Layer: c.Layer, X: end.X, Y: end.Y - g/2, Size: size, Anchor: "middle", Text: name}
	if out.Y == 0 {
		l.X, l.Y, l.Anchor = end.X+out.X*g/2, end.Y+h/3, "start"
		if out.X < 0 {
			l.Anchor = "end"
		}
	} else if out.Y > 0 {
		l.Y = end.Y + g/2 + h
	}
	box := func(l Label) [4]float64 {
		x := l.X - w/2
		switch l.Anchor {
		case "start":
			x = l.X
		case "end":
			x = l.X - w
		}
		return [4]float64{x, l.Y - h, x + w, l.Y}
	}
	overlaps := func(l Label) bool {
		a := box(l)
		for _, o := range b.d.Labels {
			if o.Size != size {
				continue
			}
			ow := 0.6 * size * float64(len([]rune(o.Text)))
			ox := o.X - ow/2
			switch o.Anchor {
			case "start":
				ox = o.X
			case "end":
				ox = o.X - ow
			}
			if a[0] < ox+ow && ox < a[2] && a[1] < o.Y && o.Y-h < a[3] {
				return true
			}
		}
		return false
	}
	step := Point{Y: -(h + 2)}
	if out.Y > 0 {
		step.Y = h + 2
	}
	for i := 0; i < 8 && overlaps(l); i++ {
		l.X, l.Y = l.X+step.X, l.Y+step.Y
	}
	b.d.Labels = append(b.d.Labels, l)
	b.report.Labels++
}

// lineStart is the port of a device on node cn a line drawn into this page starts at, and that device.
func (b *cimPageBuilder) lineStart(cn string) (Point, int, bool) {
	if cn == "" {
		return Point{}, 0, false
	}
	for _, a := range b.attach[cn] {
		if a.owner != 0 && a.group == 0 {
			return a.at, a.owner, true
		}
	}
	return Point{}, 0, false
}

// axis is the unit vector along v's larger component (up for a zero v).
func axis(v Point) Point {
	switch {
	case v == Point{}:
		return Point{Y: -1}
	case math.Abs(v.X) > math.Abs(v.Y):
		return Point{X: math.Copysign(1, v.X)}
	default:
		return Point{Y: math.Copysign(1, v.Y)}
	}
}

// pathLength is the length of the polyline pts.
func pathLength(pts []Point) float64 {
	l := 0.0
	for k := 1; k < len(pts); k++ {
		l += dist(pts[k-1], pts[k])
	}
	return l
}

func dist(p, q Point) float64 {
	return math.Hypot(p.X-q.X, p.Y-q.Y)
}

// segDist is how far p is from segment a–z.
func segDist(p, a, z Point) float64 {
	dx, dy := z.X-a.X, z.Y-a.Y
	l2 := dx*dx + dy*dy
	if l2 == 0 {
		return dist(p, a)
	}
	t := math.Max(0, math.Min(1, ((p.X-a.X)*dx+(p.Y-a.Y)*dy)/l2))
	return dist(p, Point{X: a.X + t*dx, Y: a.Y + t*dy})
}

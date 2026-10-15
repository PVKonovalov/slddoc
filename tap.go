package slddoc

import "math"

// xsde2svg draws a T-tap, a device whose terminal meets a wire mid-span (a
// ground switch hanging off a feeder), by ending the device's lead on the
// wire's line, or a unit or two short of or past it, while the wire runs on
// unbroken: it has no end there. buildTopology only joins points to wire ends
// and busbars, so such a port came out connected to nothing. tapPortsOntoWires
// runs first and gives each one a wire end to join.

// lineEntranceReach is how far along an overhead line, from its nearer end, a
// grounding device may tap it: the line entrance at the substation (154 of the
// corpus's 156 such switches sit within 50). An overhead line is otherwise
// never tapped mid-span.
const lineEntranceReach = 50

// groundingClasses are the single-port grounding devices that may tap an
// overhead line at its substation end.
var groundingClasses = map[Class]bool{ClassGroundSwitch: true, ClassGround: true, ClassShortCircuiter: true}

// nearestOnSegment is the point of segment a-b nearest p.
func nearestOnSegment(p, a, b Point) Point {
	dx, dy := b.X-a.X, b.Y-a.Y
	l2 := dx*dx + dy*dy
	if l2 == 0 {
		return a
	}
	t := ((p.X-a.X)*dx + (p.Y-a.Y)*dy) / l2
	t = math.Max(0, math.Min(1, t))
	return Point{X: a.X + t*dx, Y: a.Y + t*dy}
}

// splitPath cuts pts at q, a point on segment i: the part up to q and the part
// from q on, each without a zero-length step at q.
func splitPath(pts []Point, i int, q Point) (head, tail []Point) {
	head = append(head, pts[:i+1]...)
	if distance(head[len(head)-1], q) > onSegmentEpsilon {
		head = append(head, q)
	} else {
		head[len(head)-1] = q
	}
	tail = []Point{q}
	rest := pts[i+1:]
	if len(rest) > 0 && distance(rest[0], q) <= onSegmentEpsilon {
		rest = rest[1:]
	}
	tail = append(tail, rest...)
	return head, tail
}

// tapPortsOntoWires joins each port that would otherwise connect to nothing to
// the wire or busbar passing within snapTolerance of it. A port is "free" when
// no other port and no wire end lies within snapTolerance of it and it isn't
// on a busbar. Its binding moves onto the nearest point of that line:
//   - a busbar: buildTopology then joins it to the busbar;
//   - a wire (any kind but an overhead line), touching none of the device's
//     other ports: the wire is split there into two connectors that both end
//     on the port. The second piece gets a fresh id (nextID, returned
//     advanced) and keeps the wire's kind, name, voltage, layer and style;
//   - an overhead line, only for a grounding device within lineEntranceReach
//     of the line's nearer end: the piece from that end to the tap becomes a
//     BusWork wire (fresh id) and the line, keeping its id and name, now
//     starts (or ends) at the tap, so the device sits on the line's
//     substation-end node and the line stays one object. At the end itself
//     the port just moves onto it.
//
// Only the nearest line is tapped, never every wire passing through the
// point, so a wire merely crossing there stays separate.
func tapPortsOntoWires(d *Diagram, bindings []portBinding, nextID int) int {
	near := func(a, b Point) bool { return distance(a, b) <= snapTolerance }
	var buses [][]Point
	for _, e := range d.Elements {
		if e.Class == ClassBusBarSection {
			buses = append(buses, e.Points)
		}
	}
	onBus := func(p Point) bool {
		for _, pts := range buses {
			for i := 0; i+1 < len(pts); i++ {
				if onSegment(p, pts[i], pts[i+1]) {
					return true
				}
			}
		}
		return false
	}
	// Port and wire-end points in snapTolerance-sized cells, so finding what
	// lies near a port looks at its own cell and the eight around it.
	type cellKey struct{ x, y int }
	cellOf := func(p Point) cellKey {
		return cellKey{int(math.Floor(p.X / snapTolerance)), int(math.Floor(p.Y / snapTolerance))}
	}
	type marked struct {
		p       Point
		binding int // -1 for a wire end
	}
	cells := map[cellKey][]marked{}
	for i, b := range bindings {
		k := cellOf(b.p)
		cells[k] = append(cells[k], marked{b.p, i})
	}
	for _, c := range d.Connectors {
		if n := len(c.Points); n >= 2 {
			for _, p := range []Point{c.Points[0], c.Points[n-1]} {
				k := cellOf(p)
				cells[k] = append(cells[k], marked{p, -1})
			}
		}
	}
	isFree := func(bi int) bool {
		p := bindings[bi].p
		k := cellOf(p)
		for dx := -1; dx <= 1; dx++ {
			for dy := -1; dy <= 1; dy++ {
				for _, m := range cells[cellKey{k.x + dx, k.y + dy}] {
					if m.binding != bi && near(m.p, p) {
						return false
					}
				}
			}
		}
		return !onBus(p)
	}
	// Wires touching one of an element's own ports are never tapped by it, so
	// a wire drawn past its own other terminal doesn't short it.
	own := map[int][]Point{}
	for _, b := range bindings {
		own[b.elemIdx] = append(own[b.elemIdx], b.p)
	}
	touchesOwn := func(elemIdx int, c Connector) bool {
		n := len(c.Points)
		for _, p := range own[elemIdx] {
			if near(c.Points[0], p) || near(c.Points[n-1], p) {
				return true
			}
		}
		return false
	}
	// A line's bounding box grown by snapTolerance: a port outside it can't
	// be near any of its segments, which skips most lines at once.
	type box struct{ minX, minY, maxX, maxY float64 }
	boxOf := func(pts []Point) box {
		b := box{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}
		for _, p := range pts {
			b.minX, b.minY = math.Min(b.minX, p.X), math.Min(b.minY, p.Y)
			b.maxX, b.maxY = math.Max(b.maxX, p.X), math.Max(b.maxY, p.Y)
		}
		return b
	}
	reaches := func(b box, p Point) bool {
		return p.X >= b.minX-snapTolerance && p.X <= b.maxX+snapTolerance && p.Y >= b.minY-snapTolerance && p.Y <= b.maxY+snapTolerance
	}
	var busPts [][]Point
	var busBoxes []box
	for _, e := range d.Elements {
		if e.Class == ClassBusBarSection && len(e.Points) >= 2 {
			busPts = append(busPts, e.Points)
			busBoxes = append(busBoxes, boxOf(e.Points))
		}
	}

	// Decided up front, from the drawing as it is: a tap made below adds wire
	// ends that would otherwise make the next port on the same spot look
	// taken.
	free := make([]bool, len(bindings))
	for bi := range bindings {
		free[bi] = isFree(bi)
	}
	for bi := range bindings {
		if !free[bi] {
			continue
		}
		b := bindings[bi]
		el := d.Elements[b.elemIdx]
		if el.Class == ClassBusBarSection {
			continue
		}
		grounding := groundingClasses[el.Class]

		best := snapTolerance + onSegmentEpsilon
		var at Point
		hitConn, hitSeg, hitBus := -1, -1, false
		consider := func(pts []Point, connIdx int, bus bool) {
			for i := 0; i+1 < len(pts); i++ {
				q := nearestOnSegment(b.p, pts[i], pts[i+1])
				if dist := distance(q, b.p); dist <= snapTolerance && dist < best {
					best, at, hitConn, hitSeg, hitBus = dist, q, connIdx, i, bus
				}
			}
		}
		for ci, c := range d.Connectors {
			if len(c.Points) < 2 || !reaches(boxOf(c.Points), b.p) || touchesOwn(b.elemIdx, c) {
				continue
			}
			if c.Kind == KindOverheadLine && !grounding {
				continue
			}
			consider(c.Points, ci, false)
		}
		for i, pts := range busPts {
			if reaches(busBoxes[i], b.p) {
				consider(pts, -1, true)
			}
		}
		if hitConn < 0 && !hitBus {
			continue
		}
		if hitBus {
			bindings[bi].p = at
			continue
		}

		c := d.Connectors[hitConn]
		if c.Kind == KindOverheadLine {
			total, along := 0.0, 0.0
			for i := 0; i+1 < len(c.Points); i++ {
				if i == hitSeg {
					along = total + distance(c.Points[i], at)
				}
				total += distance(c.Points[i], c.Points[i+1])
			}
			fromStart := along <= total-along
			reach := math.Min(along, total-along)
			if reach > lineEntranceReach {
				continue
			}
			if reach <= onSegmentEpsilon {
				if fromStart {
					bindings[bi].p = c.Points[0]
				} else {
					bindings[bi].p = c.Points[len(c.Points)-1]
				}
				continue
			}
			head, tail := splitPath(c.Points, hitSeg, at)
			stub := Connector{ID: nextID, Kind: KindBusWork, Voltage: c.Voltage, Layer: c.Layer}
			nextID++
			if fromStart {
				stub.Points, d.Connectors[hitConn].Points = head, tail
			} else {
				stub.Points, d.Connectors[hitConn].Points = tail, head
			}
			d.Connectors = append(d.Connectors, stub)
			bindings[bi].p = at
			continue
		}

		head, tail := splitPath(c.Points, hitSeg, at)
		if len(head) < 2 || len(tail) < 2 {
			// At (or within a hair of) the wire's own end: join that end.
			bindings[bi].p = at
			continue
		}
		second := c
		second.ID = nextID
		nextID++
		second.Points = tail
		d.Connectors[hitConn].Points = head
		d.Connectors = append(d.Connectors, second)
		bindings[bi].p = at
	}
	return nextID
}

// tapWireEndsOntoWires joins each connection point made of a wire end, plus
// any ports and other wire ends within snapTolerance of it, to the wire passing
// mid-span within snapTolerance of it: a T-junction in buswork, which xsde2svg
// draws as one wire (and maybe a device's lead) ending on another that runs on
// unbroken. The nearest such wire with no end there (any kind but an overhead
// line, never one of the point's own wires, and never one touching another
// port of a device at the point, so a device isn't shorted across its own
// leads) is split there into two connectors, the second with a fresh id
// (nextID, returned advanced) and the wire's kind, name, voltage, layer and
// style; every wire end and port of the point moves onto the split point, so
// buildTopology joins them all. A point on a busbar is left to buildTopology,
// and a wire merely crossing another, with no end on it, stays separate. Runs
// after tapPortsOntoWires.
func tapWireEndsOntoWires(d *Diagram, bindings []portBinding, nextID int) int {
	near := func(a, b Point) bool { return distance(a, b) <= snapTolerance }
	var buses [][]Point
	for _, e := range d.Elements {
		if e.Class == ClassBusBarSection {
			buses = append(buses, e.Points)
		}
	}
	onBus := func(p Point) bool {
		for _, pts := range buses {
			for i := 0; i+1 < len(pts); i++ {
				if onSegment(p, pts[i], pts[i+1]) {
					return true
				}
			}
		}
		return false
	}
	ownPorts := map[int][]int{}
	for bi, b := range bindings {
		ownPorts[b.elemIdx] = append(ownPorts[b.elemIdx], bi)
	}

	// Kept as points: a split made below may leave an end on another piece
	// of its wire.
	var candidates []Point
	for _, c := range d.Connectors {
		if n := len(c.Points); n >= 2 {
			for _, p := range []Point{c.Points[0], c.Points[n-1]} {
				if !onBus(p) {
					candidates = append(candidates, p)
				}
			}
		}
	}
	for _, p := range candidates {
		// The point's ports, and every port of their devices that isn't at it.
		var here []int
		var others []Point
		for bi, b := range bindings {
			if near(b.p, p) {
				here = append(here, bi)
			}
		}
		for _, bi := range here {
			for _, oi := range ownPorts[bindings[bi].elemIdx] {
				if !near(bindings[oi].p, p) {
					others = append(others, bindings[oi].p)
				}
			}
		}
		touches := func(c Connector) bool {
			n := len(c.Points)
			if near(c.Points[0], p) || near(c.Points[n-1], p) {
				return true
			}
			for _, q := range others {
				if near(c.Points[0], q) || near(c.Points[n-1], q) {
					return true
				}
			}
			return false
		}

		best := snapTolerance + onSegmentEpsilon
		var at Point
		hitConn, hitSeg := -1, -1
		for ci, c := range d.Connectors {
			if c.Kind == KindOverheadLine || len(c.Points) < 2 || touches(c) {
				continue
			}
			for i := 0; i+1 < len(c.Points); i++ {
				q := nearestOnSegment(p, c.Points[i], c.Points[i+1])
				if dist := distance(q, p); dist <= snapTolerance && dist < best {
					best, at, hitConn, hitSeg = dist, q, ci, i
				}
			}
		}
		if hitConn < 0 {
			continue
		}
		head, tail := splitPath(d.Connectors[hitConn].Points, hitSeg, at)
		if len(head) < 2 || len(tail) < 2 {
			continue
		}
		// Gather the point onto the split before adding the second piece, whose
		// first point is already at.
		for ci := range d.Connectors {
			pts := d.Connectors[ci].Points
			if ci == hitConn || len(pts) < 2 {
				continue
			}
			if near(pts[0], p) {
				pts[0] = at
			}
			if near(pts[len(pts)-1], p) {
				pts[len(pts)-1] = at
			}
		}
		for _, bi := range here {
			bindings[bi].p = at
		}
		second := d.Connectors[hitConn]
		second.ID = nextID
		nextID++
		second.Points = tail
		d.Connectors[hitConn].Points = head
		d.Connectors = append(d.Connectors, second)
	}
	return nextID
}

// tapWiresThroughJunctions splits every wire running through a junction point
// (shape 7, xsde2svg's dot marking that what crosses there is connected) at
// the dot: a wire whose line passes within snapTolerance of the junction's
// port, with neither end near it, is cut there into two connectors, the second
// with a fresh id (nextID, returned advanced) and the wire's kind, name,
// voltage, layer and style; buildTopology then joins both pieces to the
// junction. Every such wire is split, an overhead line included, since the dot
// is an explicit connection. Runs before tapPortsOntoWires, so the junction's
// port then counts as taken.
func tapWiresThroughJunctions(d *Diagram, bindings []portBinding, nextID int) int {
	near := func(a, b Point) bool { return distance(a, b) <= snapTolerance }
	for _, b := range bindings {
		if d.Elements[b.elemIdx].Class != ClassJunctionPoint {
			continue
		}
		for ci := 0; ci < len(d.Connectors); ci++ {
			pts := d.Connectors[ci].Points
			if len(pts) < 2 || near(pts[0], b.p) || near(pts[len(pts)-1], b.p) {
				continue
			}
			for i := 0; i+1 < len(pts); i++ {
				at := nearestOnSegment(b.p, pts[i], pts[i+1])
				if !near(at, b.p) {
					continue
				}
				head, tail := splitPath(pts, i, at)
				if len(head) < 2 || len(tail) < 2 {
					break
				}
				second := d.Connectors[ci]
				second.ID = nextID
				nextID++
				second.Points = tail
				d.Connectors[ci].Points = head
				d.Connectors = append(d.Connectors, second)
				break
			}
		}
	}
	return nextID
}

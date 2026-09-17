package slddoc

import (
	"fmt"
	"math"
	"sort"
)

// unionFind merges connection points into electrical nodes. Exact
// coincidence is free (two points at the same coordinate share the same
// pointKey and so are the same set from the start); union is only needed to
// additionally merge a point onto a BusBarSection's node when the point
// lies somewhere along the bus's drawn length, not just at its endpoints.
type unionFind struct {
	parent map[string]string
}

func newUnionFind() *unionFind {
	return &unionFind{parent: map[string]string{}}
}

func (uf *unionFind) find(k string) string {
	p, ok := uf.parent[k]
	if !ok {
		uf.parent[k] = k
		return k
	}
	if p != k {
		p = uf.find(p)
		uf.parent[k] = p
	}
	return p
}

func (uf *unionFind) union(a, b string) {
	ra, rb := uf.find(a), uf.find(b)
	if ra != rb {
		uf.parent[ra] = rb
	}
}

// pointKey rounds to the nearest integer before formatting: every legitimate
// coordinate in this format is an integer, so this both canonicalizes
// floating-point rotation noise and guarantees two geometrically-coincident
// points always produce an identical key.
func pointKey(p Point) string {
	return fmt.Sprintf("%.0f,%.0f", math.Round(p.X), math.Round(p.Y))
}

func busKey(elementID int) string {
	return fmt.Sprintf("bus:%d", elementID)
}

const onSegmentEpsilon = 0.5

// snapTolerance merges two otherwise-unconnected points within this many
// units of each other. xsde2svg's own rendering formulas sometimes leave a
// few pixels of deliberate visual gap between a symbol's drawn terminal and
// the wire that electrically continues it (observed on a PowerTransformer's
// leads specifically), so exact-coordinate matching alone under-connects.
const snapTolerance = 5.0

func distance(a, b Point) float64 {
	dx, dy := a.X-b.X, a.Y-b.Y
	return math.Sqrt(dx*dx + dy*dy)
}

// onSegment reports whether p lies on the axis-aligned segment a-b
// (inclusive of its endpoints). Non-axis-aligned bus segments are not
// supported in v1 and always report false, same as a point genuinely off
// the segment.
func onSegment(p, a, b Point) bool {
	if math.Abs(a.Y-b.Y) < onSegmentEpsilon { // horizontal
		if math.Abs(p.Y-a.Y) > onSegmentEpsilon {
			return false
		}
		lo, hi := math.Min(a.X, b.X), math.Max(a.X, b.X)
		return p.X >= lo-onSegmentEpsilon && p.X <= hi+onSegmentEpsilon
	}
	if math.Abs(a.X-b.X) < onSegmentEpsilon { // vertical
		if math.Abs(p.X-a.X) > onSegmentEpsilon {
			return false
		}
		lo, hi := math.Min(a.Y, b.Y), math.Max(a.Y, b.Y)
		return p.Y >= lo-onSegmentEpsilon && p.Y <= hi+onSegmentEpsilon
	}
	return false
}

// portBinding is one Element port whose global coordinate has been resolved
// (potentially by rotating a local port through the element's transform)
// but whose Node id is not yet known.
type portBinding struct {
	elemIdx, portIdx int
	p                Point
}

type connectorEnd struct {
	connIdx int
	isTo    bool
	p       Point
}

// buildTopology reconstructs the electrical node graph and writes the
// result back into d: every port binding's Node, every connector's
// From/To, and d.Nodes itself.
func buildTopology(d *Diagram, bindings []portBinding) {
	uf := newUnionFind()

	var ends []connectorEnd
	for ci, c := range d.Connectors {
		if len(c.Points) < 2 {
			continue
		}
		ends = append(ends, connectorEnd{ci, false, c.Points[0]})
		ends = append(ends, connectorEnd{ci, true, c.Points[len(c.Points)-1]})
	}

	type bus struct {
		key string
		pts []Point
	}
	var buses []bus
	for _, e := range d.Elements {
		if e.Class == ClassBusBarSection {
			buses = append(buses, bus{busKey(e.ID), e.Points})
		}
	}

	type keyedPoint struct {
		key string
		p   Point
	}
	var points []keyedPoint
	seenKey := map[string]bool{}
	add := func(p Point) {
		k := pointKey(p)
		if !seenKey[k] {
			seenKey[k] = true
			points = append(points, keyedPoint{k, p})
		}
	}
	for _, b := range bindings {
		add(b.p)
	}
	for _, e := range ends {
		add(e.p)
	}
	for i := 0; i < len(points); i++ {
		for j := i + 1; j < len(points); j++ {
			if distance(points[i].p, points[j].p) <= snapTolerance {
				uf.union(points[i].key, points[j].key)
			}
		}
	}

	unionOntoBuses := func(p Point) {
		k := pointKey(p)
		for _, b := range buses {
			for i := 0; i+1 < len(b.pts); i++ {
				if onSegment(p, b.pts[i], b.pts[i+1]) {
					uf.union(k, b.key)
					break
				}
			}
		}
	}
	for _, b := range bindings {
		unionOntoBuses(b.p)
	}
	for _, e := range ends {
		unionOntoBuses(e.p)
	}

	// Assign a representative coordinate to each root, preferring a bus's
	// own coordinate so a bus's node is shown at the bus, not at whichever
	// tap happened to be processed first.
	repCoord := map[string]Point{}
	claim := func(k string, p Point, overwrite bool) {
		r := uf.find(k)
		if _, ok := repCoord[r]; !ok || overwrite {
			repCoord[r] = p
		}
	}
	for _, b := range buses {
		if len(b.pts) > 0 {
			claim(b.key, b.pts[0], true)
		}
	}
	for _, b := range bindings {
		claim(pointKey(b.p), b.p, false)
	}
	for _, e := range ends {
		claim(pointKey(e.p), e.p, false)
	}

	roots := make([]string, 0, len(repCoord))
	for r := range repCoord {
		roots = append(roots, r)
	}
	sort.Slice(roots, func(i, j int) bool {
		pi, pj := repCoord[roots[i]], repCoord[roots[j]]
		if pi.X != pj.X {
			return pi.X < pj.X
		}
		return pi.Y < pj.Y
	})

	nodeID := make(map[string]int, len(roots))
	d.Nodes = make([]Node, 0, len(roots))
	for i, r := range roots {
		id := i + 1
		nodeID[r] = id
		p := repCoord[r]
		d.Nodes = append(d.Nodes, Node{ID: id, X: p.X, Y: p.Y})
	}

	for _, b := range bindings {
		d.Elements[b.elemIdx].Ports[b.portIdx].Node = nodeID[uf.find(pointKey(b.p))]
	}
	for _, e := range ends {
		id := nodeID[uf.find(pointKey(e.p))]
		if e.isTo {
			d.Connectors[e.connIdx].To = id
		} else {
			d.Connectors[e.connIdx].From = id
		}
	}
}

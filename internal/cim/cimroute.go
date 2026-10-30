package cim

import (
	"container/heap"
	"math"

	"github.com/PVKonovalov/slddoc/internal/model"
	"github.com/PVKonovalov/slddoc/internal/repair"
)

// A lead the simple routes can't take (through a corridor, then straight or around one band) is routed on the grid:
// the shortest orthogonal path, a bend costing as much as layoutBendCells steps, that never runs along anything
// drawn nor through a symbol or a name, and crosses a wire or busbar straight across only where it must (each
// crossing costing layoutCrossCells steps).

// layoutBendCells and layoutCrossCells are what a bend and a crossing cost in grid steps when routing a lead.
const (
	layoutBendCells  = 12
	layoutCrossCells = 200
)

// What lies on a grid cell: a horizontal or vertical wire, or something solid.
const (
	cellHWire = 1 << iota
	cellVWire
	cellSolid
)

type routeState struct {
	x, y, d int // grid cell and direction of travel (index into routeDirs)
}

var routeDirs = [4][2]int{{1, 0}, {0, 1}, {-1, 0}, {0, -1}}

type routeItem struct {
	s    routeState
	cost int
}

type routeHeap []routeItem

func (h routeHeap) Len() int { return len(h) }

func (h routeHeap) Less(i, j int) bool { return h[i].cost < h[j].cost }

func (h routeHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h *routeHeap) Push(x any) { *h = append(*h, x.(routeItem)) }

func (h *routeHeap) Pop() any {
	old := *h
	it := old[len(old)-1]
	*h = old[:len(old)-1]
	return it
}

// route finds a lead from `from`, leaving along fromDir, to `to`, arriving along toDir (both unit grid
// directions), through cells nothing drawn lies on but transformer self; node cn's own wires are solid, so the lead
// never crosses nor runs along them, and meets them only at its two ends. Returns its points, or false when there
// is no way.
func (l *cimLayout) route(from, fromDir, to, toDir model.Point, cn string, self *cimObject) ([]model.Point, bool) {
	g := l.grid
	cell := func(v float64) int { return int(math.Round(v / g)) }
	cells := map[[2]int]uint8{}
	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	mark := func(x1, y1, x2, y2 float64, kind uint8) {
		for x := cell(math.Min(x1, x2)); x <= cell(math.Max(x1, x2)); x++ {
			for y := cell(math.Min(y1, y2)); y <= cell(math.Max(y1, y2)); y++ {
				cells[[2]int{x, y}] |= kind
			}
		}
	}
	for _, do := range l.im.dos[l.diagram.id] {
		pts := l.im.points[do.id]
		for _, p := range pts {
			q := pointOf(p)
			minX, minY, maxX, maxY = math.Min(minX, q.X), math.Min(minY, q.Y), math.Max(maxX, q.X), math.Max(maxY, q.Y)
		}
		eq := do.get("IdentifiedObject")
		if self != nil && eq == self.id {
			continue
		}
		switch {
		case eq == cn:
			for i := 1; i < len(pts); i++ {
				p, q := pointOf(pts[i-1]), pointOf(pts[i])
				mark(p.X, p.Y, q.X, q.Y, cellSolid)
			}
		case do.class == "TextDiagramObject":
			if len(pts) > 0 {
				p := pointOf(pts[0])
				mark(p.X, p.Y-10, p.X+6*float64(len([]rune(do.get("text")))), p.Y, cellSolid)
			}
		case len(pts) == 1:
			b := l.symbolBox(do, pointOf(pts[0]), g)
			mark(b[0], b[1], b[2], b[3], cellSolid)
		default:
			for i := 1; i < len(pts); i++ {
				p, q := pointOf(pts[i-1]), pointOf(pts[i])
				switch {
				case p.Y == q.Y:
					mark(p.X, p.Y, q.X, q.Y, cellHWire)
				case p.X == q.X:
					mark(p.X, p.Y, q.X, q.Y, cellVWire)
				default:
					mark(p.X, p.Y, q.X, q.Y, cellSolid)
				}
				// A wire's ends and bends can't be crossed.
				mark(p.X, p.Y, p.X, p.Y, cellSolid)
				mark(q.X, q.Y, q.X, q.Y, cellSolid)
			}
		}
	}
	minX, minY = math.Min(minX, math.Min(from.X, to.X)), math.Min(minY, math.Min(from.Y, to.Y))
	maxX, maxY = math.Max(maxX, math.Max(from.X, to.X)), math.Max(maxY, math.Max(from.Y, to.Y))
	x0, y0, x1, y1 := cell(minX)-6, cell(minY)-6, cell(maxX)+6, cell(maxY)+6
	x0 = max(x0, cell(l.margin/2))
	dirOf := func(p model.Point) int {
		switch {
		case p.X > 0:
			return 0
		case p.Y > 0:
			return 1
		case p.X < 0:
			return 2
		}
		return 3
	}
	start := routeState{cell(from.X), cell(from.Y), dirOf(fromDir)}
	gx, gy, gd := cell(to.X), cell(to.Y), dirOf(toDir)
	dist := map[routeState]int{start: 0}
	prev := map[routeState]routeState{}
	h := &routeHeap{{start, 0}}
	for h.Len() > 0 {
		it := heap.Pop(h).(routeItem)
		s := it.s
		if it.cost > dist[s] {
			continue
		}
		if s.x == gx && s.y == gy && s.d == gd {
			var cells []routeState
			for c := s; ; c = prev[c] {
				cells = append(cells, c)
				if c == start {
					break
				}
			}
			pts := make([]model.Point, 0, len(cells)+2)
			pts = append(pts, from)
			for i := len(cells) - 1; i >= 0; i-- {
				pts = append(pts, model.Point{X: float64(cells[i].x) * g, Y: float64(cells[i].y) * g})
			}
			return repair.SimplifyPath(append(pts, to)), true
		}
		onWire := s != start && cells[[2]int{s.x, s.y}] != 0
		for nd := range routeDirs {
			if (nd+2)%4 == s.d || onWire && nd != s.d {
				continue // no turning back, and straight on across a wire
			}
			n := routeState{s.x + routeDirs[nd][0], s.y + routeDirs[nd][1], nd}
			if n.x < x0 || n.x > x1 || n.y < y0 || n.y > y1 {
				continue
			}
			cost := it.cost + 1
			if c := cells[[2]int{n.x, n.y}]; c != 0 && !(n.x == gx && n.y == gy) {
				along := uint8(cellHWire)
				if routeDirs[nd][0] == 0 {
					along = cellVWire
				}
				if c&cellSolid != 0 || c&along != 0 {
					continue
				}
				cost += layoutCrossCells
			}
			if nd != s.d {
				cost += layoutBendCells
			}
			if d, ok := dist[n]; !ok || cost < d {
				dist[n] = cost
				prev[n] = s
				heap.Push(h, routeItem{n, cost})
			}
		}
	}
	return nil, false
}

// symbolBox is the box the symbol drawn by DiagramObject do at p covers: reach around it for most symbols (half a
// symbol's size); for a transformer, out to its farthest terminals along its axis and, across it, half that plus
// half of reach (its windings' circles, with room beside them).
func (l *cimLayout) symbolBox(do *cimObject, p model.Point, reach float64) [4]float64 {
	rx, ry := reach, reach
	if o := l.im.objs[do.get("IdentifiedObject")]; o != nil && o.class == "PowerTransformer" {
		for _, t := range repair.SymbolTerminals(new(l.transformerProbe(o, p.X, p.Y)), l.im.opt.Terminals) {
			rx, ry = math.Max(rx, math.Abs(t.X-p.X)), math.Max(ry, math.Abs(t.Y-p.Y))
		}
		rx, ry = math.Max(rx, ry/2+reach/2), math.Max(ry, rx/2+reach/2)
	}
	return [4]float64{p.X - rx, p.Y - ry, p.X + rx, p.Y + ry}
}

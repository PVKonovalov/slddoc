package designate

import (
	"cmp"
	"math"
	"slices"

	"github.com/PVKonovalov/slddoc/internal/model"
)

type wire struct {
	id    int
	other int
	line  bool
}

// graph is a diagram's topology: elements and wires at each node.
type graph struct {
	d        *model.Diagram
	elements map[int]*model.Element
	at       map[int][]*model.Element
	wires    map[int][]wire
	nodes    map[int]model.Point
	busbarAt map[int][]int
	conns    map[int]*model.Connector
}

func isLine(k model.ConnectorKind) bool {
	return k == model.KindOverheadLine || k == model.KindCableLine || k == model.KindLinkToObject
}

func isTransformer(c model.Class) bool {
	return c == model.ClassPowerTransformer || c == model.ClassBooster
}

func newGraph(d *model.Diagram) *graph {
	g := &graph{
		d: d, elements: map[int]*model.Element{}, at: map[int][]*model.Element{}, wires: map[int][]wire{},
		nodes: map[int]model.Point{}, busbarAt: map[int][]int{}, conns: map[int]*model.Connector{},
	}
	for _, n := range d.Nodes {
		g.nodes[n.ID] = model.Point{X: n.X, Y: n.Y}
	}
	for i := range d.Elements {
		e := &d.Elements[i]
		g.elements[e.ID] = e
		for _, p := range e.Ports {
			if !slices.Contains(g.at[p.Node], e) {
				g.at[p.Node] = append(g.at[p.Node], e)
			}
			if e.Class == model.ClassBusBarSection && !slices.Contains(g.busbarAt[p.Node], e.ID) {
				g.busbarAt[p.Node] = append(g.busbarAt[p.Node], e.ID)
			}
		}
	}
	for i := range d.Connectors {
		c := &d.Connectors[i]
		g.conns[c.ID] = c
		line := isLine(c.Kind)
		g.wires[c.From] = append(g.wires[c.From], wire{c.ID, c.To, line})
		g.wires[c.To] = append(g.wires[c.To], wire{c.ID, c.From, line})
	}
	return g
}

// position is an element's reading-order point: a points-based element's top-left point, else its anchor.
func position(e *model.Element) model.Point {
	if len(e.Points) == 0 {
		return model.Point{X: e.X, Y: e.Y}
	}
	p := e.Points[0]
	for _, q := range e.Points[1:] {
		p.X, p.Y = math.Min(p.X, q.X), math.Min(p.Y, q.Y)
	}
	return p
}

// unionFind joins node ids.
type unionFind map[int]int

func (u unionFind) find(x int) int {
	root := x
	for {
		p, ok := u[root]
		if !ok || p == root {
			break
		}
		root = p
	}
	for x != root {
		x, u[x] = u[x], root
	}
	return root
}

func (u unionFind) union(a, b int) {
	if ra, rb := u.find(a), u.find(b); ra != rb {
		u[ra] = rb
	}
}

// components joins nodes connected by a wire other than a line, or by an element other than a transformer: each
// component is one voltage level's switchgear.
func (g *graph) components() unionFind {
	u := unionFind{}
	for _, c := range g.d.Connectors {
		if !isLine(c.Kind) {
			u.union(c.From, c.To)
		}
	}
	for _, e := range g.d.Elements {
		if isTransformer(e.Class) {
			continue
		}
		for i := 1; i < len(e.Ports); i++ {
			u.union(e.Ports[0].Node, e.Ports[i].Node)
		}
	}
	return u
}

// rowRanks returns each point's reading-order rank: rows by y (a new row starts more than tol below the first
// point of the current one), then x.
func rowRanks(ps []model.Point, tol float64) []int {
	idx := make([]int, len(ps))
	for i := range idx {
		idx[i] = i
	}
	slices.SortStableFunc(idx, func(a, b int) int { return cmp.Compare(ps[a].Y, ps[b].Y) })
	row := make([]int, len(ps))
	r, top := 0, math.Inf(-1)
	for _, i := range idx {
		if ps[i].Y-top > tol {
			r++
			top = ps[i].Y
		}
		row[i] = r
	}
	slices.SortStableFunc(idx, func(a, b int) int {
		return cmp.Or(cmp.Compare(row[a], row[b]), cmp.Compare(ps[a].X, ps[b].X))
	})
	rank := make([]int, len(ps))
	for k, i := range idx {
		rank[i] = k
	}
	return rank
}

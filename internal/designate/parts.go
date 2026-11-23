package designate

import (
	"slices"

	"github.com/PVKonovalov/slddoc/internal/model"
)

// part is one bay: what a trace from a busbar reaches without passing another busbar, a transformer or a line.
type part struct {
	elements     []int
	dist         map[int]int
	busbars      []int
	transformers []int
	lines        int
	sourceLinks  int
	at           model.Point
	hasAt        bool
	comp         int
}

type partBuilder struct {
	g        *graph
	comps    unionFind
	partOf   map[int]*part
	seenNode map[int]bool
	parts    []*part
}

type step struct {
	node, element, dist int
}

// buildParts traces from every busbar connection point outward, then from the equipment no such trace reached.
func buildParts(g *graph, comps unionFind, tol float64) []*part {
	b := &partBuilder{g: g, comps: comps, partOf: map[int]*part{}, seenNode: map[int]bool{}}
	var busbars []*model.Element
	for i := range g.d.Elements {
		if e := &g.d.Elements[i]; e.Class == model.ClassBusBarSection {
			busbars = append(busbars, e)
		}
	}
	sortByReading(busbars, tol)
	for _, bb := range busbars {
		for _, p := range bb.Ports {
			for _, w := range g.wires[p.Node] {
				if !w.line && !b.seenNode[w.other] && len(g.busbarAt[w.other]) == 0 {
					b.trace(step{node: w.other, dist: 1}, p.Node)
				}
			}
			for _, e := range g.at[p.Node] {
				if e.Class != model.ClassBusBarSection && b.partOf[e.ID] == nil {
					b.trace(step{element: e.ID, dist: 1}, p.Node)
				}
			}
		}
	}
	var rest []*model.Element
	for i := range g.d.Elements {
		e := &g.d.Elements[i]
		if len(e.Ports) > 0 && e.Class != model.ClassBusBarSection && !isTransformer(e.Class) && b.partOf[e.ID] == nil {
			rest = append(rest, e)
		}
	}
	sortByReading(rest, tol)
	for _, e := range rest {
		if b.partOf[e.ID] == nil {
			b.trace(step{element: e.ID}, 0)
		}
	}
	return b.parts
}

// trace collects one part, breadth first from start; from is the busbar node it leaves (0: none).
func (b *partBuilder) trace(start step, from int) {
	p := &part{dist: map[int]int{}, comp: -1}
	if from != 0 {
		p.at, p.hasAt = b.g.nodes[from], true
		p.comp = b.comps.find(from)
		for _, id := range b.g.busbarAt[from] {
			addOnce(&p.busbars, id)
		}
	}
	queue := []step{start}
	for len(queue) > 0 {
		s := queue[0]
		queue = queue[1:]
		if s.element != 0 {
			e := b.g.elements[s.element]
			if isTransformer(e.Class) {
				if !slices.Contains(p.transformers, e.ID) {
					p.transformers = append(p.transformers, e.ID)
					p.dist[e.ID] = s.dist
				}
				continue
			}
			if b.partOf[e.ID] != nil {
				continue
			}
			b.partOf[e.ID] = p
			p.elements = append(p.elements, e.ID)
			p.dist[e.ID] = s.dist
			if p.comp < 0 && len(e.Ports) > 0 {
				p.comp = b.comps.find(e.Ports[0].Node)
			}
			for _, port := range e.Ports {
				queue = append(queue, step{node: port.Node, dist: s.dist + 1})
			}
			continue
		}
		if ids := b.g.busbarAt[s.node]; len(ids) > 0 {
			for _, id := range ids {
				addOnce(&p.busbars, id)
			}
			continue
		}
		if b.seenNode[s.node] {
			continue
		}
		b.seenNode[s.node] = true
		if p.comp < 0 {
			p.comp = b.comps.find(s.node)
		}
		for _, w := range b.g.wires[s.node] {
			if w.line {
				p.lines++
				if c := b.g.conns[w.id]; c != nil && c.Kind == model.KindLinkToObject && c.CIM == "EnergySource" {
					p.sourceLinks++
				}
				continue
			}
			queue = append(queue, step{node: w.other, dist: s.dist})
		}
		for _, e := range b.g.at[s.node] {
			queue = append(queue, step{element: e.ID, dist: s.dist})
		}
	}
	if len(p.elements) == 0 && len(p.transformers) == 0 {
		return
	}
	if !p.hasAt && len(p.elements) > 0 {
		p.at = position(b.g.elements[p.elements[0]])
	}
	b.parts = append(b.parts, p)
}

func addOnce(s *[]int, id int) {
	if !slices.Contains(*s, id) {
		*s = append(*s, id)
	}
}

func sortByReading(es []*model.Element, tol float64) {
	ps := make([]model.Point, len(es))
	for i, e := range es {
		ps[i] = position(e)
	}
	rank := rowRanks(ps, tol)
	order := make([]*model.Element, len(es))
	for i, e := range es {
		order[rank[i]] = e
	}
	copy(es, order)
}

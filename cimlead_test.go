package slddoc

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
)

// transformerUnderBaysEQ builds a one-busbar substation whose transformer hangs below the bays and is fed from above on
// both windings: winding 2 from a busbar breaker, winding 1 from an infeed through a disconnector, beside four load
// bays.
func transformerUnderBaysEQ() string {
	var b strings.Builder
	obj := func(class, id string, props ...string) {
		fmt.Fprintf(&b, "<cim:%s rdf:ID=\"_%s\">", class, id)
		for i := 0; i+1 < len(props); i += 2 {
			if strings.HasPrefix(props[i+1], "#") {
				fmt.Fprintf(&b, "<cim:%s rdf:resource=\"#_%s\"/>", props[i], props[i+1][1:])
			} else {
				fmt.Fprintf(&b, "<cim:%s>%s</cim:%s>", props[i], props[i+1], props[i])
			}
		}
		fmt.Fprintf(&b, "</cim:%s>\n", class)
	}
	term := func(eq, cn string, seq int) {
		obj("Terminal", fmt.Sprintf("t-%s-%d", eq, seq), "Terminal.ConductingEquipment", "#"+eq,
			"Terminal.ConnectivityNode", "#"+cn, "ACDCTerminal.sequenceNumber", fmt.Sprint(seq))
	}
	b.WriteString(`<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#" xmlns:cim="http://iec.ch/TC57/CIM100#">` + "\n")
	obj("BaseVoltage", "bv", "BaseVoltage.nominalVoltage", "33")
	obj("Substation", "s", "IdentifiedObject.name", "S")
	obj("VoltageLevel", "v", "VoltageLevel.Substation", "#s", "VoltageLevel.BaseVoltage", "#bv")
	for _, cn := range []string{"bus", "in", "iso", "tx2", "f1", "f2", "f3", "f4"} {
		obj("ConnectivityNode", cn, "ConnectivityNode.ConnectivityNodeContainer", "#v")
	}
	obj("BusbarSection", "B", "IdentifiedObject.name", "B", "Equipment.EquipmentContainer", "#v")
	term("B", "bus", 1)
	breaker := func(id, a, z string) {
		obj("Breaker", id, "IdentifiedObject.name", id, "Equipment.EquipmentContainer", "#v", "Switch.normalOpen", "false")
		term(id, a, 1)
		term(id, z, 2)
	}
	breaker("Q1", "bus", "tx2")
	for i := 1; i <= 4; i++ {
		breaker(fmt.Sprintf("Q%d", i+1), "bus", fmt.Sprintf("f%d", i))
		obj("EnergyConsumer", fmt.Sprintf("L%d", i), "IdentifiedObject.name", fmt.Sprintf("L%d", i), "Equipment.EquipmentContainer", "#v")
		term(fmt.Sprintf("L%d", i), fmt.Sprintf("f%d", i), 1)
	}
	obj("EnergySource", "G", "IdentifiedObject.name", "G", "Equipment.EquipmentContainer", "#v")
	term("G", "in", 1)
	obj("Disconnector", "D", "IdentifiedObject.name", "D", "Equipment.EquipmentContainer", "#v", "Switch.normalOpen", "false")
	term("D", "in", 1)
	term("D", "iso", 2)
	obj("PowerTransformer", "T", "IdentifiedObject.name", "T", "Equipment.EquipmentContainer", "#s")
	term("T", "iso", 1)
	term("T", "tx2", 2)
	for i, bv := range []string{"bv", "bv"} {
		obj("PowerTransformerEnd", fmt.Sprintf("Te%d", i+1), "PowerTransformerEnd.PowerTransformer", "#T",
			"TransformerEnd.Terminal", fmt.Sprintf("#t-T-%d", i+1), "TransformerEnd.endNumber", fmt.Sprint(i+1),
			"TransformerEnd.BaseVoltage", "#"+bv)
	}
	b.WriteString("</rdf:RDF>\n")
	return b.String()
}

// TestAutoLayoutTransformerLeads checks the rules every lead into a transformer keeps: it never passes over the
// transformer (nor any other symbol) but in its last segment, coming into the winding from outside, and never
// crosses nor touches another wire of its own node or itself.
func TestAutoLayoutTransformerLeads(t *testing.T) {
	terms := map[string][]Point{"41": {{X: 0, Y: -10}, {X: 0, Y: 10}}, "162": {{X: 0, Y: -10}, {X: 0, Y: 10}},
		"330001": {{X: 0, Y: 0}}, "330002": {{X: 0, Y: 0}}}
	pages, err := ImportCIM([][]byte{[]byte(transformerUnderBaysEQ())}, testImport(CIMImportOptions{Terminals: terms, Grid: 10}))
	if err != nil {
		t.Fatal(err)
	}
	d := pages[0].Diagram
	if len(pages[0].Warnings) > 0 {
		t.Errorf("warnings = %v", pages[0].Warnings)
	}
	nodes := map[int]Node{}
	for _, n := range d.Nodes {
		nodes[n.ID] = n
	}
	var tr Element
	for _, e := range d.Elements {
		if e.Class == ClassPowerTransformer {
			tr = e
		}
	}
	if len(tr.Ports) != 2 {
		t.Fatalf("transformer = %+v", tr)
	}
	// The transformer's box: between its winding terminals along its axis, its circles' width across it.
	a, z := nodes[tr.Ports[0].Node], nodes[tr.Ports[1].Node]
	box := [4]float64{math.Min(a.X, z.X), math.Min(a.Y, z.Y), math.Max(a.X, z.X), math.Max(a.Y, z.Y)}
	if a.X == z.X {
		box[0], box[2] = tr.X-22, tr.X+22
	} else {
		box[1], box[3] = tr.Y-22, tr.Y+22
	}
	byNode := map[string][]Connector{}
	for _, c := range d.Connectors {
		for k := 1; k < len(c.Points); k++ {
			p, q := c.Points[k-1], c.Points[k]
			if overlaps([4]float64{math.Min(p.X, q.X), math.Min(p.Y, q.Y), math.Max(p.X, q.X), math.Max(p.Y, q.Y)}, box) {
				t.Errorf("wire %d runs over the transformer: %v", c.ID, c.Points)
			}
		}
		for i := 1; i < len(c.Points); i++ {
			for j := i + 2; j < len(c.Points); j++ {
				if segmentsMeet(c.Points[i-1], c.Points[i], c.Points[j-1], c.Points[j]) {
					t.Errorf("wire %d crosses itself: %v", c.ID, c.Points)
				}
			}
		}
		byNode[nodes[c.From].MRID] = append(byNode[nodes[c.From].MRID], c)
	}
	for _, cs := range byNode {
		for i, c := range cs {
			for _, o := range cs[i+1:] {
				shared := map[Point]bool{}
				for _, n := range []int{c.From, c.To} {
					if n == o.From || n == o.To {
						shared[Point{X: nodes[n].X, Y: nodes[n].Y}] = true
					}
				}
				for p := 1; p < len(c.Points); p++ {
					for q := 1; q < len(o.Points); q++ {
						s1, s2, s3, s4 := c.Points[p-1], c.Points[p], o.Points[q-1], o.Points[q]
						if !segmentsMeet(s1, s2, s3, s4) {
							continue
						}
						if collinearOverlap(s1, s2, s3, s4) || !slicesAny([]Point{s1, s2, s3, s4}, func(x Point) bool {
							return shared[x] && onSeg(x, s1, s2) && onSeg(x, s3, s4)
						}) {
							t.Errorf("wires %d and %d of one node meet away from their node: %v %v", c.ID, o.ID, c.Points, o.Points)
						}
					}
				}
			}
		}
	}
}

func slicesAny(pts []Point, f func(Point) bool) bool {
	return slices.ContainsFunc(pts, f)
}

// TestAutoLayoutSpacing checks the spacing hand-drawn diagrams use: bays 9 grid steps apart on a busbar up to 20 kV
// and 12 above, the busbar 2 steps past its outermost bays, a bay's first device terminal 2 steps from the busbar,
// devices 6 steps apart down a bay, and each device's name on its own row, right of it.
func TestAutoLayoutSpacing(t *testing.T) {
	terms := map[string][]Point{"41": {{X: 0, Y: -10}, {X: 0, Y: 10}}, "162": {{X: 0, Y: -10}, {X: 0, Y: 10}},
		"330001": {{X: 0, Y: 0}}, "330002": {{X: 0, Y: 0}}}
	for _, c := range []struct {
		kv    float64
		pitch float64
	}{{11, 90}, {33, 120}} {
		opt := testImport(CIMImportOptions{Terminals: terms, Grid: 10})
		if c.kv != 33 {
			opt.WindingVoltages = map[string]float64{"Te2": c.kv}
		}
		pages, err := ImportCIM([][]byte{[]byte(transformerUnderBaysEQ())}, opt)
		if err != nil {
			t.Fatal(err)
		}
		d := pages[0].Diagram
		at := map[string]Element{}
		var bus Element
		for _, e := range d.Elements {
			at[e.Name] = e
			if e.Class == ClassBusBarSection {
				bus = e
			}
		}
		var xs []float64
		for i := 2; i <= 5; i++ {
			xs = append(xs, at[fmt.Sprintf("Q%d", i)].X)
		}
		for i := 1; i < len(xs); i++ {
			if xs[i]-xs[i-1] != c.pitch {
				t.Errorf("%v kV: bays at %v, want %v apart", c.kv, xs, c.pitch)
			}
		}
		if len(bus.Points) != 2 {
			t.Fatalf("busbar = %+v", bus)
		}
		lo, hi := math.Min(bus.Points[0].X, bus.Points[1].X), math.Max(bus.Points[0].X, bus.Points[1].X)
		first, last := math.Inf(1), math.Inf(-1)
		for _, e := range d.Elements {
			if e.Class == ClassBreaker {
				first, last = math.Min(first, e.X), math.Max(last, e.X)
			}
		}
		if first-lo != 20 || hi-last != 20 {
			t.Errorf("%v kV: busbar %v–%v for bays %v–%v, want 20 past each", c.kv, lo, hi, first, last)
		}
		busY := bus.Points[0].Y
		q, l := at["Q2"], at["L1"]
		if math.Abs(q.Y-busY) != 30 {
			t.Errorf("%v kV: breaker centre %v from the busbar at %v, want its terminal 20 from it", c.kv, q.Y, busY)
		}
		if math.Abs(l.Y-q.Y) > 60 {
			t.Errorf("%v kV: load %v and breaker %v more than a level apart", c.kv, l.Y, q.Y)
		}
		for _, lb := range d.Labels {
			if lb.Text == "Q2" && (lb.X != q.X+20 || lb.Y != q.Y+5) {
				t.Errorf("%v kV: Q2's name at (%v,%v), device at (%v,%v)", c.kv, lb.X, lb.Y, q.X, q.Y)
			}
		}
	}
}

// TestAutoLayoutEarthingTaps: a bay whose nodes carry earthing switches (one behind its own disconnector) is one
// straight run of its switches, its earthing drawn sideways off it, each at its node and clear of the others.
func TestAutoLayoutEarthingTaps(t *testing.T) {
	var b strings.Builder
	obj := func(class, id string, props ...string) {
		fmt.Fprintf(&b, "<cim:%s rdf:ID=\"_%s\">", class, id)
		for i := 0; i+1 < len(props); i += 2 {
			if strings.HasPrefix(props[i+1], "#") {
				fmt.Fprintf(&b, "<cim:%s rdf:resource=\"#_%s\"/>", props[i], props[i+1][1:])
			} else {
				fmt.Fprintf(&b, "<cim:%s>%s</cim:%s>", props[i], props[i+1], props[i])
			}
		}
		fmt.Fprintf(&b, "</cim:%s>\n", class)
	}
	term := func(eq, cn string, seq int) {
		obj("Terminal", fmt.Sprintf("t-%s-%d", eq, seq), "Terminal.ConductingEquipment", "#"+eq,
			"Terminal.ConnectivityNode", "#"+cn, "ACDCTerminal.sequenceNumber", fmt.Sprint(seq))
	}
	b.WriteString(`<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#" xmlns:cim="http://iec.ch/TC57/CIM100#">` + "\n")
	obj("BaseVoltage", "bv", "BaseVoltage.nominalVoltage", "35")
	obj("Substation", "s", "IdentifiedObject.name", "S")
	obj("VoltageLevel", "v", "VoltageLevel.Substation", "#s", "VoltageLevel.BaseVoltage", "#bv")
	for _, cn := range []string{"bus", "a", "b", "c", "e"} {
		obj("ConnectivityNode", cn, "ConnectivityNode.ConnectivityNodeContainer", "#v")
	}
	obj("BusbarSection", "B", "IdentifiedObject.name", "B", "Equipment.EquipmentContainer", "#v")
	term("B", "bus", 1)
	two := func(class, id, x, y string) {
		obj(class, id, "IdentifiedObject.name", id, "Equipment.EquipmentContainer", "#v", "Switch.normalOpen", "false")
		term(id, x, 1)
		term(id, y, 2)
	}
	one := func(class, id, x string) {
		obj(class, id, "IdentifiedObject.name", id, "Equipment.EquipmentContainer", "#v", "Switch.normalOpen", "true")
		term(id, x, 1)
	}
	// Busbar, bus disconnector, breaker, line disconnector, a load beyond; earthing on each node between them, the
	// one on the line side behind a disconnector of its own.
	two("Disconnector", "DB", "bus", "a")
	two("Breaker", "Q", "a", "b")
	two("Disconnector", "DL", "b", "c")
	obj("EnergyConsumer", "L", "IdentifiedObject.name", "L", "Equipment.EquipmentContainer", "#v")
	term("L", "c", 1)
	one("GroundDisconnector", "EA", "a")
	one("GroundDisconnector", "EB", "b")
	two("Disconnector", "DE", "c", "e")
	one("GroundDisconnector", "EC", "e")
	b.WriteString("</rdf:RDF>\n")

	terms := map[string][]Point{"41": {{X: 0, Y: -10}, {X: 0, Y: 10}}, "162": {{X: 0, Y: -10}, {X: 0, Y: 10}},
		"54": {{X: 0, Y: 10}}, "330001": {{X: 0, Y: 0}}}
	pages, err := ImportCIM([][]byte{[]byte(b.String())}, testImport(CIMImportOptions{Terminals: terms, Grid: 10}))
	if err != nil {
		t.Fatal(err)
	}
	d := pages[0].Diagram
	at := map[string]Element{}
	for _, e := range d.Elements {
		at[e.Name] = e
	}
	axis := at["Q"].X
	for _, name := range []string{"DB", "Q", "DL", "L"} {
		if at[name].X != axis || at[name].Orient%180 != 0 {
			t.Errorf("%s at x %v orient %d, want on the bay's axis %v, upright", name, at[name].X, at[name].Orient, axis)
		}
	}
	nodes := map[int]Node{}
	for _, n := range d.Nodes {
		nodes[n.ID] = n
	}
	for _, name := range []string{"EA", "EB", "EC", "DE"} {
		e := at[name]
		if e.Orient%180 == 0 || e.X <= axis {
			t.Errorf("%s at (%v,%v) orient %d, want sideways right of the axis %v", name, e.X, e.Y, e.Orient, axis)
		}
	}
	if at["EC"].X <= at["DE"].X || at["EC"].Y != at["DE"].Y {
		t.Errorf("EC (%v,%v) should follow its disconnector DE (%v,%v) on its row", at["EC"].X, at["EC"].Y, at["DE"].X, at["DE"].Y)
	}
	for _, e := range d.Elements {
		if e.Class == ClassBusBarSection || e.Class == ClassJunctionPoint {
			continue
		}
		ts := symbolTerminals(&e, terms)
		box := [4]float64{e.X - 8, e.Y - 8, e.X + 8, e.Y + 8}
		for _, p := range ts {
			box = [4]float64{math.Min(box[0], p.X), math.Min(box[1], p.Y), math.Max(box[2], p.X), math.Max(box[3], p.Y)}
		}
		for _, c := range d.Connectors {
			for k := 1; k < len(c.Points); k++ {
				p, q := c.Points[k-1], c.Points[k]
				onTerm := slices.ContainsFunc(ts, func(x Point) bool { return samePt(x, p) || samePt(x, q) })
				if !onTerm && overlaps([4]float64{math.Min(p.X, q.X), math.Min(p.Y, q.Y), math.Max(p.X, q.X), math.Max(p.Y, q.Y)}, box) {
					t.Errorf("wire %d %v runs over %s", c.ID, c.Points, e.Name)
				}
			}
		}
	}
}

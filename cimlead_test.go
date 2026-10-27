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
	pages, err := ImportCIM([][]byte{[]byte(transformerUnderBaysEQ())}, CIMImportOptions{Terminals: terms, Grid: 10})
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

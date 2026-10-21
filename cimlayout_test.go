package slddoc

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// layoutEQ builds an equipment-only CIM document: substation S1 with a 110 kV busbar, a breaker to a 110/10 kV
// transformer, a breaker to a 10 kV busbar, a feeder breaker to a load and one to a line to S2, where the line
// lands on a breaker to S2's busbar with a load bay.
func layoutEQ() string {
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
	obj("BaseVoltage", "bv110", "BaseVoltage.nominalVoltage", "110")
	obj("BaseVoltage", "bv10", "BaseVoltage.nominalVoltage", "10")
	obj("Substation", "s1", "IdentifiedObject.name", "S1")
	obj("Substation", "s2", "IdentifiedObject.name", "S2")
	obj("VoltageLevel", "v1hv", "VoltageLevel.Substation", "#s1", "VoltageLevel.BaseVoltage", "#bv110")
	obj("VoltageLevel", "v1lv", "VoltageLevel.Substation", "#s1", "VoltageLevel.BaseVoltage", "#bv10")
	obj("VoltageLevel", "v2", "VoltageLevel.Substation", "#s2", "VoltageLevel.BaseVoltage", "#bv10")
	for cn, vl := range map[string]string{"h1": "v1hv", "c1": "v1hv", "c2": "v1lv", "h2": "v1lv", "c3": "v1lv", "c4": "v1lv",
		"c5": "v2", "h3": "v2", "c6": "v2"} {
		obj("ConnectivityNode", cn, "ConnectivityNode.ConnectivityNodeContainer", "#"+vl)
	}
	busbar := func(id, vl, cn string) {
		obj("BusbarSection", id, "IdentifiedObject.name", id, "Equipment.EquipmentContainer", "#"+vl)
		term(id, cn, 1)
	}
	breaker := func(id, vl, a, z string) {
		obj("Breaker", id, "IdentifiedObject.name", id, "Equipment.EquipmentContainer", "#"+vl, "Switch.normalOpen", "false")
		term(id, a, 1)
		term(id, z, 2)
	}
	load := func(id, vl, cn string) {
		obj("EnergyConsumer", id, "IdentifiedObject.name", id, "Equipment.EquipmentContainer", "#"+vl)
		term(id, cn, 1)
	}
	busbar("B110", "v1hv", "h1")
	breaker("Q1", "v1hv", "h1", "c1")
	obj("PowerTransformer", "T1", "IdentifiedObject.name", "T1", "Equipment.EquipmentContainer", "#s1")
	term("T1", "c1", 1)
	term("T1", "c2", 2)
	obj("PowerTransformerEnd", "T1e1", "PowerTransformerEnd.PowerTransformer", "#T1", "TransformerEnd.Terminal", "#t-T1-1",
		"TransformerEnd.endNumber", "1", "TransformerEnd.BaseVoltage", "#bv110")
	obj("PowerTransformerEnd", "T1e2", "PowerTransformerEnd.PowerTransformer", "#T1", "TransformerEnd.Terminal", "#t-T1-2",
		"TransformerEnd.endNumber", "2", "TransformerEnd.BaseVoltage", "#bv10")
	breaker("Q2", "v1lv", "c2", "h2")
	busbar("B10", "v1lv", "h2")
	breaker("Q3", "v1lv", "h2", "c3")
	load("L1", "v1lv", "c3")
	breaker("Q4", "v1lv", "h2", "c4")
	obj("ACLineSegment", "W1", "IdentifiedObject.name", "W1", "ConductingEquipment.BaseVoltage", "#bv10")
	term("W1", "c4", 1)
	term("W1", "c5", 2)
	breaker("Q5", "v2", "c5", "h3")
	busbar("B2", "v2", "h3")
	breaker("Q6", "v2", "h3", "c6")
	load("L2", "v2", "c6")
	b.WriteString("</rdf:RDF>\n")
	return b.String()
}

// terminalNodes maps "name/sequence" of every exported terminal to its ConnectivityNode.
func terminalNodes(t *testing.T, d *Diagram) map[string]string {
	t.Helper()
	eq := parseRDF(t, ExportCIM(d, CIMOptions{Name: "t", Created: time.Unix(0, 0)}).EQ)
	names := map[string]string{}
	for _, o := range eq {
		names["#"+o.id] = strings.Join(o.props["IdentifiedObject.name"], "")
	}
	out := map[string]string{}
	for _, o := range eq {
		if o.class == "Terminal" {
			key := names[o.props["Terminal.ConductingEquipment"][0]] + "/" + o.props["ACDCTerminal.sequenceNumber"][0]
			out[key] = o.props["Terminal.ConnectivityNode"][0]
		}
	}
	return out
}

func TestAutoLayout(t *testing.T) {
	terms := map[string][]Point{"41": {{X: 0, Y: -20}, {X: 0, Y: 20}}, "330001": {{X: 0, Y: 0}}}
	pages, err := ImportCIM([][]byte{[]byte(layoutEQ())}, CIMImportOptions{Terminals: terms, Grid: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 2 || pages[0].Title != "S1" || pages[1].Title != "S2" {
		t.Fatalf("pages = %d %v", len(pages), func() []string {
			var s []string
			for _, p := range pages {
				s = append(s, p.Title)
			}
			return s
		}())
	}
	if !strings.Contains(strings.Join(pages[0].Messages, ";"), "laid out automatically") {
		t.Errorf("messages = %v", pages[0].Messages)
	}

	d := pages[0].Diagram
	Repair(d, terms)
	at := map[string]Element{}
	for _, e := range d.Elements {
		at[e.Name] = e
	}
	// High voltage on top, the transformer between, the consumer at the bottom.
	if !(at["B110"].Y < at["T1"].Y && at["T1"].Y < at["B10"].Y && at["B10"].Y < at["L1"].Y) {
		t.Errorf("vertical order: B110 %v, T1 %v, B10 %v, L1 %v", at["B110"].Y, at["T1"].Y, at["B10"].Y, at["L1"].Y)
	}
	tr := at["T1"]
	if ts := symbolTerminals(&tr, terms); len(ts) != 2 || ts[0].Y >= ts[1].Y {
		t.Errorf("winding 1 should be on top: %v", ts)
	}
	line := false
	for _, c := range d.Connectors {
		line = line || c.Name == "W1"
	}
	if !line {
		t.Error("the line to S2 should be drawn as a stub")
	}
	n := terminalNodes(t, d)
	same := [][]string{{"B110/1", "Q1/1"}, {"Q1/2", "T1/1"}, {"T1/2", "Q2/1"}, {"Q2/2", "B10/1", "Q3/1", "Q4/1"}, {"Q3/2", "L1/1"},
		{"Q4/2", "W1/1"}}
	seen := map[string]bool{}
	for _, group := range same {
		for _, k := range group {
			if n[k] == "" || n[k] != n[group[0]] {
				t.Errorf("%s should share %s's node: %v", k, group[0], n)
			}
		}
		if seen[n[group[0]]] {
			t.Errorf("group %v merged with another", group)
		}
		seen[n[group[0]]] = true
	}

	d2 := pages[1].Diagram
	Repair(d2, terms)
	n2 := terminalNodes(t, d2)
	for _, group := range [][]string{{"W1/2", "Q5/1"}, {"Q5/2", "B2/1", "Q6/1"}, {"Q6/2", "L2/1"}} {
		for _, k := range group {
			if n2[k] == "" || n2[k] != n2[group[0]] {
				t.Errorf("S2: %s should share %s's node: %v", k, group[0], n2)
			}
		}
	}
}

package cim

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/PVKonovalov/slddoc/internal/model"
	"github.com/PVKonovalov/slddoc/internal/repair"
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
func terminalNodes(t *testing.T, d *model.Diagram) map[string]string {
	t.Helper()
	eq := parseRDF(t, ExportCIM(d, testExport(CIMOptions{Name: "t", Created: time.Unix(0, 0)})).EQ)
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
	terms := map[string][]model.Point{"41": {{X: 0, Y: -20}, {X: 0, Y: 20}}, "330001": {{X: 0, Y: 0}}}
	pages, err := ImportCIM([][]byte{[]byte(layoutEQ())}, testImport(CIMImportOptions{Terminals: terms, Grid: 10}))
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
	at := map[string]model.Element{}
	for _, e := range d.Elements {
		at[e.Name] = e
	}
	// Bands without a voltage level name are named after their voltage.
	names := map[string]bool{}
	for _, lb := range d.Labels {
		names[lb.Text] = true
	}
	if !names["110 kV"] || !names["10 kV"] {
		t.Errorf("bands should be named 110 kV and 10 kV: %+v", d.Labels)
	}
	// High voltage on top, the transformer between, the consumer at the bottom.
	if !(at["B110"].Y < at["T1"].Y && at["T1"].Y < at["B10"].Y && at["B10"].Y < at["L1"].Y) {
		t.Errorf("vertical order: B110 %v, T1 %v, B10 %v, L1 %v", at["B110"].Y, at["T1"].Y, at["B10"].Y, at["L1"].Y)
	}
	tr := at["T1"]
	if ts := repair.SymbolTerminals(&tr, terms); len(ts) != 2 || ts[0].Y >= ts[1].Y {
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
	n2 := terminalNodes(t, d2)
	for _, group := range [][]string{{"W1/2", "Q5/1"}, {"Q5/2", "B2/1", "Q6/1"}, {"Q6/2", "L2/1"}} {
		for _, k := range group {
			if n2[k] == "" || n2[k] != n2[group[0]] {
				t.Errorf("S2: %s should share %s's node: %v", k, group[0], n2)
			}
		}
	}
}

// doubleBusEQ is substation S1 with two 110 kV busbars, BA and BB, and one bay: disconnectors DA and DB to them,
// joined at a node, a breaker Q and line W1 leaving for S2; plus a coupler: DC1 from BA and DC2 from BB to breaker QC.
func doubleBusEQ() string {
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
	obj("BaseVoltage", "bv", "BaseVoltage.nominalVoltage", "110")
	obj("Substation", "s1", "IdentifiedObject.name", "S1")
	obj("Substation", "s2", "IdentifiedObject.name", "S2")
	obj("VoltageLevel", "v1", "IdentifiedObject.name", "S1_110", "VoltageLevel.Substation", "#s1", "VoltageLevel.BaseVoltage", "#bv")
	obj("VoltageLevel", "v2", "VoltageLevel.Substation", "#s2", "VoltageLevel.BaseVoltage", "#bv")
	for _, cn := range []string{"ha", "hb", "m", "n", "c1", "c2"} {
		obj("ConnectivityNode", cn, "ConnectivityNode.ConnectivityNodeContainer", "#v1")
	}
	obj("ConnectivityNode", "far", "ConnectivityNode.ConnectivityNodeContainer", "#v2")
	for id, cn := range map[string]string{"BA": "ha", "BB": "hb"} {
		obj("BusbarSection", id, "IdentifiedObject.name", id, "Equipment.EquipmentContainer", "#v1")
		term(id, cn, 1)
	}
	sw := func(class, id, a, z string) {
		obj(class, id, "IdentifiedObject.name", id, "Equipment.EquipmentContainer", "#v1", "Switch.normalOpen", "false")
		term(id, a, 1)
		term(id, z, 2)
	}
	sw("Disconnector", "DA", "ha", "m")
	sw("Disconnector", "DB", "hb", "m")
	sw("Breaker", "Q", "m", "n")
	obj("ACLineSegment", "W1", "IdentifiedObject.name", "W1", "ConductingEquipment.BaseVoltage", "#bv")
	term("W1", "n", 1)
	term("W1", "far", 2)
	sw("Disconnector", "DC1", "ha", "c1")
	sw("Breaker", "QC", "c1", "c2")
	sw("Disconnector", "DC2", "c2", "hb")
	b.WriteString("</rdf:RDF>\n")
	return b.String()
}

// TestAutoLayoutParallelBusbars: busbars sharing a bay are rows one under another over the same width; each bay
// device on a busbar has a column of its own and drops straight onto its row; the breaker stands over them, the line
// beyond it; the coupler comes first, above the busbars.
func TestAutoLayoutParallelBusbars(t *testing.T) {
	terms := map[string][]model.Point{"41": {{X: 0, Y: -10}, {X: 0, Y: 10}}, "162": {{X: 0, Y: -10}, {X: 0, Y: 10}}, "14": {{X: -10}, {X: 10}}}
	classes := map[string]model.Class{"41": model.ClassBreaker, "162": model.ClassDisconnector, "24": model.ClassBusBarSection, "14": model.ClassNonIntersection}
	pages, err := ImportCIM([][]byte{[]byte(doubleBusEQ())}, testImport(CIMImportOptions{Terminals: terms, Classes: classes, Grid: 10}))
	if err != nil {
		t.Fatal(err)
	}
	d := pages[0].Diagram
	at := map[string]model.Element{}
	for _, e := range d.Elements {
		at[e.Name] = e
		if e.Class == model.ClassNonIntersection {
			t.Errorf("no wire jump where a bay crosses a busbar of its own voltage class: %+v", e)
		}
	}
	ba, bb := at["BA"], at["BB"]
	if len(ba.Points) != 2 || len(bb.Points) != 2 || ba.Points[0].X != bb.Points[0].X || ba.Points[1].X != bb.Points[1].X ||
		ba.Points[0].Y >= bb.Points[0].Y {
		t.Fatalf("busbars should be parallel rows, BA above BB: %v %v", ba.Points, bb.Points)
	}
	da, db, q := at["DA"], at["DB"], at["Q"]
	if da.X == db.X || q.X <= min(da.X, db.X) || q.X >= max(da.X, db.X) {
		t.Errorf("DA %v and DB %v should have columns of their own with Q %v between them", da.X, db.X, q.X)
	}
	if !(q.Y < da.Y && q.Y < db.Y && da.Y < ba.Points[0].Y && db.Y < ba.Points[0].Y) {
		t.Errorf("the bay should stand above the busbars, Q over DA and DB: Q %v, DA %v, DB %v, BA %v", q.Y, da.Y, db.Y, ba.Points[0].Y)
	}
	if at["QC"].X >= min(da.X, db.X) || at["QC"].Y >= ba.Points[0].Y {
		t.Errorf("the coupler should come first, above the busbars: QC %v, DA %v", at["QC"], da.X)
	}
	for _, c := range d.Connectors {
		if c.Kind == model.KindOverheadLine && c.Name == "W1" {
			if top := min(c.Points[0].Y, c.Points[len(c.Points)-1].Y); top >= q.Y {
				t.Errorf("line W1 should leave above Q: %v", c.Points)
			}
		}
		if c.Kind == model.KindBusWork && len(c.Points) > 4 {
			t.Errorf("a bay needs no long runs: %v", c.Points)
		}
	}
	// The voltage level's name beside its busbars: left of them, between their rows.
	named := false
	for _, lb := range d.Labels {
		if lb.Text == "S1_110" {
			named = lb.X+6*float64(len(lb.Text)) <= ba.Points[0].X && lb.Y > ba.Points[0].Y && lb.Y < bb.Points[0].Y
		}
	}
	if !named {
		t.Errorf("voltage level S1_110 should be named left of its busbars, between their rows: %+v", d.Labels)
	}
	n := terminalNodes(t, d)
	for _, group := range [][]string{{"BA/1", "DA/1", "DC1/1"}, {"BB/1", "DB/1", "DC2/2"}, {"DA/2", "DB/2", "Q/1"}, {"Q/2", "W1/1"}} {
		for _, k := range group {
			if n[k] == "" || n[k] != n[group[0]] {
				t.Errorf("%s should share %s's node: %v", k, group[0], n)
			}
		}
	}
}

// offTerminals lists the device ports whose Node lies off its terminal (within 20) where the Node is that port's
// alone and only buswork reaches it: the editor's "connection points off their terminals" check.
func offTerminals(d *model.Diagram, terms map[string][]model.Point) []string {
	owners := map[int]int{}
	for _, e := range d.Elements {
		for _, p := range e.Ports {
			owners[p.Node]++
		}
	}
	busWorkOnly := map[int]bool{}
	other := map[int]bool{}
	for _, c := range d.Connectors {
		for _, n := range []int{c.From, c.To} {
			if c.Kind == model.KindBusWork {
				busWorkOnly[n] = true
			} else {
				other[n] = true
			}
		}
	}
	at := map[int]model.Point{}
	for _, n := range d.Nodes {
		at[n.ID] = model.Point{X: n.X, Y: n.Y}
	}
	var out []string
	for _, e := range d.Elements {
		if e.Class == model.ClassBusBarSection {
			continue
		}
		ts := repair.SymbolTerminals(&e, terms)
		for i, p := range e.Ports {
			if i >= len(ts) || owners[p.Node] != 1 || !busWorkOnly[p.Node] || other[p.Node] {
				continue
			}
			if dd := math.Hypot(at[p.Node].X-ts[i].X, at[p.Node].Y-ts[i].Y); dd > 1e-6 && dd <= 20 {
				out = append(out, fmt.Sprintf("%s #%d port %s by %g", e.Class, e.ID, p.Name, dd))
			}
		}
	}
	return out
}

// TestAutoLayoutArranged: the transformer stands under its 110 kV bay and the 10 kV region moves under it, so both of
// its leads are straight, and nothing crosses; also when a 110 kV load bay ahead of the transformer's puts that bay a
// column further right than the 10 kV bay to the transformer.
func TestAutoLayoutArranged(t *testing.T) {
	extra := `<cim:ConnectivityNode rdf:ID="_c0"><cim:ConnectivityNode.ConnectivityNodeContainer rdf:resource="#_v1hv"/></cim:ConnectivityNode>
<cim:Breaker rdf:ID="_A0"><cim:IdentifiedObject.name>A0</cim:IdentifiedObject.name><cim:Equipment.EquipmentContainer rdf:resource="#_v1hv"/></cim:Breaker>
<cim:Terminal rdf:ID="_t-A0-1"><cim:Terminal.ConductingEquipment rdf:resource="#_A0"/><cim:Terminal.ConnectivityNode rdf:resource="#_h1"/><cim:ACDCTerminal.sequenceNumber>1</cim:ACDCTerminal.sequenceNumber></cim:Terminal>
<cim:Terminal rdf:ID="_t-A0-2"><cim:Terminal.ConductingEquipment rdf:resource="#_A0"/><cim:Terminal.ConnectivityNode rdf:resource="#_c0"/><cim:ACDCTerminal.sequenceNumber>2</cim:ACDCTerminal.sequenceNumber></cim:Terminal>
<cim:EnergyConsumer rdf:ID="_A1"><cim:IdentifiedObject.name>A1</cim:IdentifiedObject.name><cim:Equipment.EquipmentContainer rdf:resource="#_v1hv"/></cim:EnergyConsumer>
<cim:Terminal rdf:ID="_t-A1-1"><cim:Terminal.ConductingEquipment rdf:resource="#_A1"/><cim:Terminal.ConnectivityNode rdf:resource="#_c0"/><cim:ACDCTerminal.sequenceNumber>1</cim:ACDCTerminal.sequenceNumber></cim:Terminal>
</rdf:RDF>`
	for _, eq := range []string{layoutEQ(), strings.Replace(layoutEQ(), "</rdf:RDF>", extra, 1)} {
		arranged(t, eq)
	}
}

func arranged(t *testing.T, eq string) {
	terms := map[string][]model.Point{"41": {{X: 0, Y: -20}, {X: 0, Y: 20}}, "330001": {{X: 0, Y: 0}}, "14": {{X: -10}, {X: 10}}}
	classes := map[string]model.Class{"41": model.ClassBreaker, "24": model.ClassBusBarSection, "330001": model.ClassLoad, "14": model.ClassNonIntersection}
	pages, err := ImportCIM([][]byte{[]byte(eq)}, testImport(CIMImportOptions{Terminals: terms, Classes: classes, Grid: 10}))
	if err != nil {
		t.Fatal(err)
	}
	d := pages[0].Diagram
	var tr model.Element
	for _, e := range d.Elements {
		if e.Name == "T1" {
			tr = e
		}
		if e.Class == model.ClassNonIntersection {
			t.Errorf("nothing should cross: wire jump %+v", e)
		}
	}
	for _, p := range tr.Ports {
		for _, c := range d.Connectors {
			if c.From != p.Node && c.To != p.Node {
				continue
			}
			if len(c.Points) != 2 || c.Points[0].X != c.Points[1].X {
				t.Errorf("T1 winding %s should have a straight lead: %v", p.Name, c.Points)
			}
		}
	}
}

// TestAutoLayoutGeneratorsUnderTransformers: three step-up transformers on adjacent 110 kV bays, each feeding a 10 kV
// voltage level of its own with a long name. The bays are narrower than the transformers, so these spread out and
// their leads from the bays turn in one jog each, without crossing; the 10 kV levels stay together on one row, each straight under its
// transformer.
func TestAutoLayoutGeneratorsUnderTransformers(t *testing.T) {
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
	obj("VoltageLevel", "hv", "IdentifiedObject.name", "S1_110", "VoltageLevel.Substation", "#s1", "VoltageLevel.BaseVoltage", "#bv110")
	obj("ConnectivityNode", "h", "ConnectivityNode.ConnectivityNodeContainer", "#hv")
	obj("BusbarSection", "B110", "IdentifiedObject.name", "B110", "Equipment.EquipmentContainer", "#hv")
	term("B110", "h", 1)
	for k := 1; k <= 3; k++ {
		n := fmt.Sprint(k)
		obj("VoltageLevel", "g"+n, "IdentifiedObject.name", "GENERATOR_"+n+"_BUS", "VoltageLevel.Substation", "#s1",
			"VoltageLevel.BaseVoltage", "#bv10")
		obj("ConnectivityNode", "c"+n, "ConnectivityNode.ConnectivityNodeContainer", "#hv")
		obj("ConnectivityNode", "n"+n, "ConnectivityNode.ConnectivityNodeContainer", "#g"+n)
		obj("Breaker", "Q"+n, "IdentifiedObject.name", "Q"+n, "Equipment.EquipmentContainer", "#hv", "Switch.normalOpen", "false")
		term("Q"+n, "h", 1)
		term("Q"+n, "c"+n, 2)
		obj("PowerTransformer", "T"+n, "IdentifiedObject.name", "T"+n+"_STEP_UP", "Equipment.EquipmentContainer", "#s1")
		term("T"+n, "c"+n, 1)
		term("T"+n, "n"+n, 2)
		obj("PowerTransformerEnd", "T"+n+"e1", "PowerTransformerEnd.PowerTransformer", "#T"+n, "TransformerEnd.Terminal",
			"#t-T"+n+"-1", "TransformerEnd.endNumber", "1", "TransformerEnd.BaseVoltage", "#bv110")
		obj("PowerTransformerEnd", "T"+n+"e2", "PowerTransformerEnd.PowerTransformer", "#T"+n, "TransformerEnd.Terminal",
			"#t-T"+n+"-2", "TransformerEnd.endNumber", "2", "TransformerEnd.BaseVoltage", "#bv10")
		obj("EnergyConsumer", "G"+n, "IdentifiedObject.name", "G"+n, "Equipment.EquipmentContainer", "#g"+n)
		term("G"+n, "n"+n, 1)
	}
	b.WriteString("</rdf:RDF>\n")

	terms := map[string][]model.Point{"41": {{X: 0, Y: -20}, {X: 0, Y: 20}}, "330001": {{X: 0, Y: 0}}, "14": {{X: -10}, {X: 10}}}
	classes := map[string]model.Class{"41": model.ClassBreaker, "24": model.ClassBusBarSection, "330001": model.ClassLoad, "14": model.ClassNonIntersection}
	pages, err := ImportCIM([][]byte{[]byte(b.String())}, testImport(CIMImportOptions{Terminals: terms, Classes: classes, Grid: 10}))
	if err != nil {
		t.Fatal(err)
	}
	d := pages[0].Diagram
	var gens []float64
	var up [][]model.Point
	for _, e := range d.Elements {
		if e.Class == model.ClassNonIntersection {
			t.Errorf("nothing should cross: wire jump %+v", e)
		}
		if e.Class == model.ClassLoad {
			gens = append(gens, e.X)
		}
		if e.Class != model.ClassPowerTransformer {
			continue
		}
		for _, p := range e.Ports {
			for _, c := range d.Connectors {
				if c.From != p.Node && c.To != p.Node {
					continue
				}
				if p.Name == "2" && (len(c.Points) != 2 || c.Points[0].X != c.Points[1].X) {
					t.Errorf("%s winding 2 should have a straight lead: %v", e.Name, c.Points)
				}
				if p.Name == "1" {
					up = append(up, c.Points)
					if len(c.Points) > 4 {
						t.Errorf("%s winding 1 should turn in one jog: %v", e.Name, c.Points)
					}
				}
			}
		}
	}
	for i, a := range up {
		for _, b := range up[i+1:] {
			for j := 1; j < len(a); j++ {
				for k := 1; k < len(b); k++ {
					if _, hit := properCrossing(a[j-1], a[j], b[k-1], b[k]); hit {
						t.Errorf("leads from the bays cross: %v and %v", a, b)
					}
				}
			}
		}
	}
	gy := map[float64]bool{}
	for _, e := range d.Elements {
		if e.Class == model.ClassLoad {
			gy[e.Y] = true
		}
	}
	if len(gy) != 1 {
		t.Errorf("the generators should be on one row: %v", gy)
	}
	slices.Sort(gens)
	if len(gens) != 3 || gens[2]-gens[0] > 600 {
		t.Errorf("the generators should stay together: %v", gens)
	}
}

// TestAutoLayoutThreeWindings: two 110/35/10 kV transformers, each with a bay in every level. The voltage levels'
// BaseVoltage is missing, so their names give the voltages (110 on top, 35 and 10 side by side under the
// transformers, every wire with its class), and the nodes are listed by their containers only (the inverse role). No lead crosses a busbar of another
// class or runs along a wire of another node.
func TestAutoLayoutThreeWindings(t *testing.T) {
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
	obj("Substation", "s", "IdentifiedObject.name", "S")
	// Each level: a busbar, feeders, the bay of T1, feeders, the bay of T2, feeders.
	bays := []string{"f1", "f2", "c1", "f3", "f4", "f5", "c2", "f6", "f7"}
	for _, v := range []struct{ id, name string }{{"v110", "HV-110 кВ"}, {"v35", "MV-35 кВ"}, {"v10", "LV-10 кВ"}} {
		var cns strings.Builder
		for _, cn := range append([]string{"h"}, bays...) {
			fmt.Fprintf(&cns, "<cim:ConnectivityNodeContainer.ConnectivityNodes rdf:resource=\"#_%s-%s\"/>", v.id, cn)
		}
		fmt.Fprintf(&b, `<cim:VoltageLevel rdf:ID="_%s"><cim:IdentifiedObject.name>%s</cim:IdentifiedObject.name><cim:VoltageLevel.Substation rdf:resource="#_s"/><cim:VoltageLevel.BaseVoltage rdf:resource="#_missing-%s"/>%s</cim:VoltageLevel>`+"\n",
			v.id, v.name, v.id, cns.String())
		for _, cn := range append([]string{"h"}, bays...) {
			fmt.Fprintf(&b, "<cim:ConnectivityNode rdf:ID=\"_%s-%s\"/>\n", v.id, cn)
		}
		obj("BusbarSection", "B-"+v.id, "IdentifiedObject.name", "B-"+v.id, "Equipment.EquipmentContainer", "#"+v.id)
		term("B-"+v.id, v.id+"-h", 1)
		for _, bay := range bays {
			id := "Q-" + v.id + "-" + bay
			obj("Breaker", id, "IdentifiedObject.name", id, "Equipment.EquipmentContainer", "#"+v.id)
			term(id, v.id+"-h", 1)
			term(id, v.id+"-"+bay, 2)
			if bay[0] == 'f' {
				obj("EnergyConsumer", "L-"+v.id+"-"+bay, "IdentifiedObject.name", "L-"+v.id+"-"+bay, "Equipment.EquipmentContainer", "#"+v.id)
				term("L-"+v.id+"-"+bay, v.id+"-"+bay, 1)
			}
		}
	}
	for _, tr := range []string{"1", "2"} {
		obj("PowerTransformer", "T"+tr, "IdentifiedObject.name", "T"+tr, "Equipment.EquipmentContainer", "#s")
		for i, v := range []string{"v110", "v35", "v10"} {
			term("T"+tr, v+"-c"+tr, i+1)
			obj("PowerTransformerEnd", fmt.Sprintf("T%se%d", tr, i+1), "PowerTransformerEnd.PowerTransformer", "#T"+tr,
				"TransformerEnd.Terminal", fmt.Sprintf("#t-T%s-%d", tr, i+1), "TransformerEnd.endNumber", fmt.Sprint(i+1))
		}
	}
	b.WriteString("</rdf:RDF>\n")

	terms := map[string][]model.Point{"41": {{X: 0, Y: -20}, {X: 0, Y: 20}}, "330001": {{X: 0, Y: 0}}, "14": {{X: -10}, {X: 10}}}
	classes := map[string]model.Class{"41": model.ClassBreaker, "24": model.ClassBusBarSection, "330001": model.ClassLoad, "14": model.ClassNonIntersection}
	pages, err := ImportCIM([][]byte{[]byte(b.String())}, testImport(CIMImportOptions{Terminals: terms, Classes: classes, Grid: 10}))
	if err != nil {
		t.Fatal(err)
	}
	d := pages[0].Diagram
	names := map[int]string{}
	for _, vc := range d.VoltageClasses {
		names[vc.ID] = vc.Name
	}
	busY := map[string]float64{}
	var busbars []model.Element
	for _, e := range d.Elements {
		if e.Class == model.ClassBusBarSection {
			busY[names[e.Voltage]] = e.Points[0].Y
			busbars = append(busbars, e)
		}
	}
	if !(busY["110 kV"] < busY["35 kV"] && busY["35 kV"] == busY["10 kV"]) {
		t.Fatalf("110 kV should be on top, 35 and 10 kV side by side under it: %v", busY)
	}
	var trPorts []model.Port
	for _, e := range d.Elements {
		if e.Class == model.ClassPowerTransformer {
			trPorts = append(trPorts, e.Ports...)
		}
	}
	for _, c := range d.Connectors {
		if c.Voltage == 0 {
			t.Errorf("wire %d has no voltage class", c.ID)
		}
		lead := slices.ContainsFunc(trPorts, func(p model.Port) bool { return p.Node == c.From || p.Node == c.To })
		if !lead {
			continue
		}
		for k := 1; k < len(c.Points); k++ {
			for _, bus := range busbars {
				if bus.Voltage == c.Voltage {
					continue
				}
				if _, hit := properCrossing(c.Points[k-1], c.Points[k], bus.Points[0], bus.Points[len(bus.Points)-1]); hit {
					t.Errorf("the %s lead crosses the %s busbar: %v", names[c.Voltage], names[bus.Voltage], c.Points)
				}
			}
			for _, o := range d.Connectors {
				if o.ID == c.ID || o.From == c.From || o.To == c.To || o.From == c.To || o.To == c.From {
					continue
				}
				for j := 1; j < len(o.Points); j++ {
					if alongSeg(c.Points[k-1], c.Points[k], o.Points[j-1], o.Points[j]) {
						t.Errorf("the %s lead runs along wire %d: %v %v", names[c.Voltage], o.ID, c.Points, o.Points)
					}
				}
			}
		}
	}
}

// TestRouteAroundBusbar: a lead whose way down is barred by a busbar goes around its end rather than across it, and
// never through a symbol.
func TestRouteAroundBusbar(t *testing.T) {
	im := &cimImporter{objs: map[string]*cimObject{}, byClass: map[string][]*cimObject{}, dos: map[string][]*cimObject{},
		points: map[string][]*cimObject{}, opt: CIMImportOptions{Grid: 10}}
	l := &cimLayout{im: im, grid: 10, diagram: &cimObject{id: "d"}, sideUsed: map[float64][][2]float64{}}
	draw := func(id, class string, pts ...model.Point) {
		im.objs[id] = &cimObject{id: id, class: class, props: map[string][]string{}}
		do := &cimObject{id: "do-" + id, class: "DiagramObject", props: map[string][]string{"IdentifiedObject": {id}}}
		im.dos["d"] = append(im.dos["d"], do)
		for _, p := range pts {
			im.points[do.id] = append(im.points[do.id], &cimObject{props: map[string][]string{
				"xPosition": {fmt.Sprint(p.X)}, "yPosition": {fmt.Sprint(p.Y)}}})
		}
	}
	draw("bus", "BusbarSection", model.Point{X: 0, Y: 100}, model.Point{X: 400, Y: 100})
	draw("q", "Breaker", model.Point{X: 200, Y: 160})
	pts, ok := l.route(model.Point{X: 200, Y: 0}, model.Point{Y: 1}, model.Point{X: 200, Y: 200}, model.Point{Y: 1}, "lead", nil)
	if !ok {
		t.Fatal("no route")
	}
	for k := 1; k < len(pts); k++ {
		if _, hit := properCrossing(pts[k-1], pts[k], model.Point{X: 0, Y: 100}, model.Point{X: 400, Y: 100}); hit {
			t.Errorf("the lead crosses the busbar: %v", pts)
		}
		if overlaps([4]float64{math.Min(pts[k-1].X, pts[k].X), math.Min(pts[k-1].Y, pts[k].Y), math.Max(pts[k-1].X, pts[k].X),
			math.Max(pts[k-1].Y, pts[k].Y)}, [4]float64{190, 150, 210, 170}) {
			t.Errorf("the lead runs through the breaker: %v", pts)
		}
	}
}

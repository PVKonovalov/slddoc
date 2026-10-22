package slddoc

import (
	"bytes"
	"encoding/xml"
	"io"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// cimDiagram: a 110 kV busbar, a buswork wire to a breaker, a CT on the wire, the breaker wired to a
// two-winding transformer whose 10 kV side feeds an overhead line, plus a knife switch and a label.
func cimDiagram() *Diagram {
	closed, open := 1, 0
	return &Diagram{
		Width: 400, Height: 400,
		Layers:         []Layer{{ID: 0, Name: "Base"}},
		VoltageClasses: []VoltageClass{{ID: 1, Name: "110 kV"}, {ID: 2, Name: "10 kV"}, {ID: 3, Name: "aux"}},
		Nodes: []Node{
			{ID: 10, X: 0, Y: 0}, {ID: 11, X: 100, Y: 0},
			{ID: 12, X: 50, Y: 20}, {ID: 13, X: 50, Y: 40}, {ID: 14, X: 50, Y: 60},
			{ID: 15, X: 50, Y: 100}, {ID: 16, X: 50, Y: 140},
			{ID: 17, X: 50, Y: 300},
			{ID: 18, X: 200, Y: 0}, {ID: 19, X: 190, Y: 20}, {ID: 20, X: 210, Y: 20},
		},
		Elements: []Element{
			{ID: 1, Class: ClassBusBarSection, Shape: "24", Name: "B1", Voltage: 1,
				Points: []Point{{0, 0}, {100, 0}}, Ports: []Port{{"1", 10}, {"2", 11}}},
			{ID: 2, Class: ClassCurrentTransformer, Shape: "34", Voltage: 1, X: 50, Y: 30, Ports: []Port{{"1", 12}, {"2", 13}}},
			{ID: 3, Class: ClassBreaker, Shape: "41", Name: "Q1", Voltage: 1, X: 50, Y: 80, Orient: -90, State: &open,
				Ports: []Port{{"1", 14}, {"2", 15}}, Electrical: &Electrical{RatedCurrent: new(1250.0)}},
			{ID: 4, Class: ClassPowerTransformer, Shape: "47", Name: "T1", X: 50, Y: 120,
				Ports: []Port{{"1", 15}, {"2", 16}},
				Windings: []TransformerWinding{
					{Voltage: 1, Scheme: SchemeWyeN, RatedU: new(115.0), RatedS: new(25.0), R: new(2.5), X: new(55.0), G: new(0.0), B: new(0.0)},
					{Voltage: 2, Scheme: SchemeDelta, PhaseAngleClock: new(11)},
				}},
			{ID: 5, Class: ClassKnifeSwitch3, Shape: "175", Name: "S1", Voltage: 1, X: 200, Y: 10, State: &closed,
				Ports: []Port{{"1", 18}, {"2", 19}, {"3", 20}}},
			{ID: 6, Class: ClassRectangle, Shape: "3", Points: []Point{{0, 0}, {10, 10}}},
			{ID: 7, Class: ClassThyristor, Shape: "157", Ports: []Port{{"1", 19}}},
		},
		Connectors: []Connector{
			{ID: 30, Kind: KindBusWork, Voltage: 1, From: 11, To: 12, Points: []Point{{100, 0}, {50, 20}}},
			{ID: 31, Kind: KindBusWork, Voltage: 1, From: 13, To: 14, Points: []Point{{50, 40}, {50, 60}}},
			{ID: 32, Kind: KindOverheadLine, Name: "L1", Voltage: 2, From: 16, To: 17,
				Points: []Point{{50, 140}, {50, 300}}, Electrical: &Electrical{Length: new(12.0), R: new(3.0), X: new(5.0)}},
			{ID: 33, Kind: KindBusWork, Voltage: 1, From: 10, To: 18, Points: []Point{{0, 0}, {200, 0}}},
		},
		Labels: []Label{{ID: 40, For: 3, X: 60, Y: 80, Size: 10, Text: "Q1 <main>"}},
	}
}

type cimObj struct {
	class string
	id    string
	props map[string][]string
}

// parseRDF reads every object of an RDF/XML document; reference values keep their "#".
func parseRDF(t *testing.T, doc []byte) []cimObj {
	t.Helper()
	dec := xml.NewDecoder(bytes.NewReader(doc))
	var objs []cimObj
	depth := 0
	var prop string
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("not well-formed XML: %v", err)
		}
		switch tk := tok.(type) {
		case xml.StartElement:
			depth++
			if depth == 2 && tk.Name.Space == cimNS {
				o := cimObj{class: tk.Name.Local, props: map[string][]string{}}
				for _, a := range tk.Attr {
					if a.Name.Local == "ID" {
						o.id = a.Value
					}
				}
				objs = append(objs, o)
			}
			if depth == 3 && len(objs) > 0 && tk.Name.Space == cimNS {
				prop = tk.Name.Local
				for _, a := range tk.Attr {
					if a.Name.Local == "resource" {
						objs[len(objs)-1].props[prop] = append(objs[len(objs)-1].props[prop], a.Value)
					}
				}
			}
		case xml.CharData:
			if depth == 3 && len(objs) > 0 && strings.TrimSpace(string(tk)) != "" {
				objs[len(objs)-1].props[prop] = append(objs[len(objs)-1].props[prop], string(tk))
			}
		case xml.EndElement:
			depth--
		}
	}
	return objs
}

func TestExportCIM(t *testing.T) {
	when := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	out := ExportCIM(cimDiagram(), CIMOptions{Name: "test/Sub", Created: when})
	eq, dl := parseRDF(t, out.EQ), parseRDF(t, out.DL)

	defined := map[string]cimObj{}
	for _, o := range append(slices.Clone(eq), dl...) {
		if o.id == "" {
			t.Fatalf("%s without rdf:ID", o.class)
		}
		if _, dup := defined[o.id]; dup {
			t.Fatalf("duplicate rdf:ID %s", o.id)
		}
		defined[o.id] = o
	}
	for _, o := range append(slices.Clone(eq), dl...) {
		for name, vals := range o.props {
			for _, v := range vals {
				if strings.HasPrefix(v, "#") {
					if _, ok := defined[v[1:]]; !ok {
						t.Errorf("%s.%s refers to undefined %s", o.class, name, v)
					}
				}
			}
		}
	}
	for _, o := range eq {
		if strings.HasPrefix(o.class, "Diagram") {
			t.Errorf("DL class %s in EQ", o.class)
		}
	}

	count := func(objs []cimObj, class string) int {
		n := 0
		for _, o := range objs {
			if o.class == class {
				n++
			}
		}
		return n
	}
	byName := func(class, name string) cimObj {
		for _, o := range eq {
			if o.class == class && slices.Contains(o.props["IdentifiedObject.name"], name) {
				return o
			}
		}
		t.Fatalf("no %s %q", class, name)
		return cimObj{}
	}

	// Busbar ports 10/11, wires 30/31/33, the CT (12, 13) and the breaker's port 1 (14) and knife pivot (18)
	// are one node; then 15 (Q1–T1), 16 (T1–L1), 17 (line end), 19 and 20 (knife contacts).
	if n := count(eq, "ConnectivityNode"); n != 6 {
		t.Errorf("ConnectivityNodes = %d, want 6", n)
	}
	bus := byName("BusbarSection", "B1")
	q1 := byName("Breaker", "Q1")
	if got := q1.props["Switch.normalOpen"]; !slices.Equal(got, []string{"true"}) {
		t.Errorf("Q1 normalOpen = %v, want true", got)
	}
	if got := q1.props["Switch.ratedCurrent"]; !slices.Equal(got, []string{"1250"}) {
		t.Errorf("Q1 ratedCurrent = %v", got)
	}
	terminalNode := func(eqID string, seq string) string {
		for _, o := range eq {
			if o.class == "Terminal" && o.props["Terminal.ConductingEquipment"][0] == "#"+eqID && o.props["ACDCTerminal.sequenceNumber"][0] == seq {
				return o.props["Terminal.ConnectivityNode"][0]
			}
		}
		t.Fatalf("no terminal %s of %s", seq, eqID)
		return ""
	}
	if terminalNode(bus.id, "1") != terminalNode(q1.id, "1") {
		t.Error("the busbar and Q1's port 1 should share a ConnectivityNode through the wires and the CT")
	}
	ct := byName("CurrentTransformer", "CurrentTransformer 2")
	if len(ct.props["AuxiliaryEquipment.Terminal"]) != 1 {
		t.Error("the CT should be attached to a terminal")
	}
	if n := count(eq, "PowerTransformerEnd"); n != 2 {
		t.Errorf("PowerTransformerEnds = %d, want 2", n)
	}
	lv := byName("PowerTransformerEnd", "T1 W2")
	if got := lv.props["PowerTransformerEnd.ratedU"]; !slices.Equal(got, []string{"10"}) {
		t.Errorf("T1 W2 ratedU = %v, want the voltage class's 10", got)
	}
	if got := lv.props["PowerTransformerEnd.connectionKind"]; !slices.Equal(got, []string{cimNS + "WindingConnection.D"}) {
		t.Errorf("T1 W2 connectionKind = %v", got)
	}
	line := byName("ACLineSegment", "L1")
	if got := line.props["Conductor.length"]; !slices.Equal(got, []string{"12"}) {
		t.Errorf("L1 length = %v", got)
	}
	if left, right := byName("Disconnector", "S1 L"), byName("Disconnector", "S1 R"); left.props["Switch.normalOpen"][0] != "true" || right.props["Switch.normalOpen"][0] != "true" {
		t.Error("a 3-position knife switch in the middle has both sides open")
	}
	if count(eq, "Thyristor") != 0 || count(dl, "TextDiagramObject") != 1 {
		t.Error("unexpected thyristor or label count")
	}
	// Whatever a device other than a line joins is one substation; the line's far end has no equipment, so it
	// belongs to this one too.
	if n := count(eq, "Substation"); n != 1 {
		t.Errorf("Substations = %d, want 1", n)
	}
	if n := count(eq, "VoltageLevel"); n != 2 {
		t.Errorf("VoltageLevels = %d, want 2: 110 and 10 kV (the unused class left out)", n)
	}
	if !bytes.Contains(out.DL, []byte("Q1 &lt;main&gt;")) {
		t.Error("label text not escaped")
	}
	if count(dl, "DiagramObjectGluePoint") == 0 {
		t.Error("no glue points")
	}

	want := map[string][]string{
		"4/missing":     {"2:ratedU", "2:ratedS", "2:r", "2:x", "2:g", "2:b"},
		"32/missing":    {"bch"},
		"7/notExported": nil,
	}
	got := map[string][]string{}
	for _, w := range out.Warnings {
		got[strconv.Itoa(w.ID)+"/"+w.Kind] = w.Fields
	}
	for k, fields := range want {
		if g, ok := got[k]; !ok || !slices.Equal(g, fields) {
			t.Errorf("warning %s = %v, want %v (all: %+v)", k, g, fields, out.Warnings)
		}
	}

	again := ExportCIM(cimDiagram(), CIMOptions{Name: "test/Sub", Created: when.Add(time.Hour)})
	if string(bytes.SplitN(again.EQ, []byte("</md:FullModel>"), 2)[1]) != string(bytes.SplitN(out.EQ, []byte("</md:FullModel>"), 2)[1]) {
		t.Error("re-exporting the same diagram should give the same EQ objects")
	}
}

func TestParseVoltage(t *testing.T) {
	for name, want := range map[string]float64{"110 kV": 110, "6,3 \u043a\u0412": 6.3, "0.4kV": 0.4, "400 V": 0.4, "35": 35, "1,15 MV": 1150} {
		if got, ok := ParseVoltage(name); !ok || got != want {
			t.Errorf("ParseVoltage(%q) = %v, %v; want %v", name, got, ok, want)
		}
	}
	for _, bad := range []string{"#00cc00", "0 kV", "coral"} {
		if _, ok := ParseVoltage(bad); ok {
			t.Errorf("ParseVoltage(%q) should fail", bad)
		}
	}
	if _, ok := ParseVoltage("aux"); ok {
		t.Error(`ParseVoltage("aux") should fail`)
	}
}

func TestExportCIMClassChoice(t *testing.T) {
	d := &Diagram{
		VoltageClasses: []VoltageClass{{ID: 1, Name: "10 kV"}},
		Nodes:          []Node{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}, {ID: 5}},
		Elements: []Element{
			{ID: 10, Class: ClassLoad, Shape: "330001", Voltage: 1, CIM: "NonConformLoad", Ports: []Port{{"1", 1}},
				Electrical: &Electrical{P: new(1.5)}},
			{ID: 11, Class: ClassSynchronousMotor, Shape: "39", Voltage: 1, CIM: "Breaker", Ports: []Port{{"1", 2}}},
			{ID: 12, Class: ClassStaticVarCompensator, Shape: "330003", Voltage: 1, Ports: []Port{{"1", 3}}},
			{ID: 13, Class: ClassReactorShunt, Shape: "397", Voltage: 1, CIM: "PetersenCoil", Ports: []Port{{"1", 4}}},
			{ID: 14, Class: ClassNetworkInfeed, Shape: "330002", Voltage: 1, Ports: []Port{{"1", 5}}},
		},
	}
	out := ExportCIM(d, CIMOptions{Name: "t", Created: time.Unix(0, 0)})
	classes := map[string]bool{}
	for _, o := range parseRDF(t, out.EQ) {
		classes[o.class] = true
	}
	for _, c := range []string{"NonConformLoad", "SynchronousMachine", "StaticVarCompensator", "PetersenCoil", "ExternalNetworkInjection"} {
		if !classes[c] {
			t.Errorf("no %s in EQ", c)
		}
	}
	if classes["Breaker"] {
		t.Error("a CIM class the element's class doesn't allow must fall back to the default")
	}
	if !bytes.Contains(out.EQ, []byte("<cim:EnergyConsumer.pfixed>1.5</cim:EnergyConsumer.pfixed>")) {
		t.Error("load pfixed not written")
	}
	var svc []string
	for _, w := range out.Warnings {
		if w.ID == 12 {
			svc = w.Fields
		}
	}
	if !slices.Equal(svc, []string{"capacitiveRating", "inductiveRating", "slope"}) {
		t.Errorf("SVC warnings = %v", svc)
	}
	if got := CIMClasses(ClassLoad); !slices.Equal(got, []string{"EnergyConsumer", "ConformLoad", "NonConformLoad"}) {
		t.Errorf("CIMClasses(Load) = %v", got)
	}
	if CIMClasses(ClassRectangle) != nil || CIMClasses(ClassJunctionPoint) != nil {
		t.Error("non-equipment classes have no CIM classes")
	}
}

func TestExportCIMObjectLink(t *testing.T) {
	src := "EnergySource"
	d := &Diagram{
		Width: 200, Height: 200,
		VoltageClasses: []VoltageClass{{ID: 1, Name: "10 kV"}},
		Nodes:          []Node{{ID: 1, X: 0, Y: 0}, {ID: 2, X: 0, Y: 40}, {ID: 3, X: 0, Y: 100}, {ID: 4, X: 100, Y: 40}, {ID: 5, X: 100, Y: 100}, {ID: 6, X: 200, Y: 100}, {ID: 7, X: 100, Y: 140}},
		Elements: []Element{
			{ID: 10, Class: ClassBreaker, Shape: "41", Name: "Q1", Voltage: 1, X: 0, Y: 20, Ports: []Port{{"1", 1}, {"2", 2}}},
			{ID: 11, Class: ClassBreaker, Shape: "41", Name: "Q2", Voltage: 1, X: 200, Y: 80, Ports: []Port{{"1", 6}, {"2", 6}}},
		},
		Connectors: []Connector{
			{ID: 20, Kind: KindLinkToObject, Name: "Load A", Voltage: 1, From: 2, To: 3, Points: []Point{{0, 40}, {0, 100}},
				Electrical: &Electrical{P: new(1.2)}},
			{ID: 21, Kind: KindLinkToObject, Name: "Feed B", Voltage: 1, CIM: src, From: 4, To: 5, Points: []Point{{100, 40}, {100, 100}}},
			{ID: 22, Kind: KindLinkToObject, Name: "Busy", Voltage: 1, From: 7, To: 6, Points: []Point{{100, 140}, {200, 100}}},
		},
	}
	out := ExportCIM(d, CIMOptions{Name: "t", Created: time.Unix(0, 0)})
	eq := parseRDF(t, out.EQ)
	byName := map[string]cimObj{}
	terminals := map[string]int{}
	for _, o := range eq {
		if n := o.props["IdentifiedObject.name"]; len(n) > 0 {
			byName[n[0]] = o
		}
		if o.class == "Terminal" {
			terminals[o.props["Terminal.ConductingEquipment"][0]]++
		}
	}
	if o := byName["Load A"]; o.class != "EnergyConsumer" || !slices.Equal(o.props["EnergyConsumer.pfixed"], []string{"1.2"}) || terminals["#"+o.id] != 1 {
		t.Errorf("Load A = %+v, terminals %d", o, terminals["#"+o.id])
	}
	if o := byName["Feed B"]; o.class != "EnergySource" || !slices.Equal(o.props["EnergySource.nominalVoltage"], []string{"10"}) {
		t.Errorf("Feed B = %+v", o)
	}
	if !bytes.Contains(out.DL, []byte("<cim:IdentifiedObject.name>LinkToObject</cim:IdentifiedObject.name>")) {
		t.Error("no LinkToObject style in the layout")
	}
	open := 0
	for _, w := range out.Warnings {
		if w.Kind == "openEnd" {
			open++
			if w.ID != 22 {
				t.Errorf("openEnd on %d, want 22 (its arrow end is on Q2)", w.ID)
			}
		}
	}
	if open != 1 {
		t.Errorf("openEnd warnings = %d, want 1: %+v", open, out.Warnings)
	}

	pages, err := ImportCIM([][]byte{out.EQ, out.DL}, CIMImportOptions{Terminals: map[string][]Point{"41": {{X: 0, Y: -20}, {X: 0, Y: 20}}},
		Classes: map[string]Class{"41": ClassBreaker}})
	if err != nil {
		t.Fatal(err)
	}
	links := map[string]Connector{}
	for _, c := range pages[0].Diagram.Connectors {
		if c.Kind == KindLinkToObject {
			links[c.Name] = c
		}
	}
	if c := links["Load A"]; c.CIM != "" || c.Electrical == nil || *c.Electrical.P != 1.2 || !slices.Equal(c.Points, []Point{{0, 40}, {0, 100}}) {
		t.Errorf("Load A back = %+v", c)
	}
	if c := links["Feed B"]; c.CIM != src {
		t.Errorf("Feed B back = %+v", c)
	}
	if got := ConnectorCIMClasses(KindLinkToObject); !slices.Equal(got, []string{"EnergyConsumer", "EnergySource"}) || ConnectorCIMClasses(KindBusWork) != nil {
		t.Errorf("ConnectorCIMClasses = %v", got)
	}
}

// A device in a class with no voltage in its name, and a node with no voltage at all, take the voltage of what they
// are switched to; no voltage level of 0 kV is written.
func TestExportCIMVoltageFromNeighbours(t *testing.T) {
	d := &Diagram{
		VoltageClasses: []VoltageClass{{ID: 1, Name: "10 kV"}, {ID: 2, Name: "coral"}},
		Nodes:          []Node{{ID: 1}, {ID: 2}, {ID: 3}},
		Elements: []Element{
			{ID: 10, Class: ClassBusBarSection, Shape: "24", Name: "B", Voltage: 1, Points: []Point{{0, 0}, {10, 0}}, Ports: []Port{{"1", 1}}},
			{ID: 11, Class: ClassBreaker, Shape: "41", Name: "Q", Voltage: 2, Ports: []Port{{"1", 1}, {"2", 2}}},
			{ID: 12, Class: ClassDisconnector, Shape: "162", Name: "D", Ports: []Port{{"1", 2}, {"2", 3}}},
			{ID: 13, Class: ClassLoad, Shape: "330001", Name: "L", Ports: []Port{{"1", 3}}},
		},
	}
	out := ExportCIM(d, CIMOptions{Name: "t", Created: time.Unix(0, 0)})
	eq := parseRDF(t, out.EQ)
	var vls, subs int
	for _, o := range eq {
		switch o.class {
		case "VoltageLevel":
			vls++
		case "Substation":
			subs++
		case "BaseVoltage":
			if o.props["BaseVoltage.nominalVoltage"][0] == "0" {
				t.Error("a 0 kV base voltage was written")
			}
		case "ConnectivityNode":
			if len(o.props["ConnectivityNode.ConnectivityNodeContainer"]) == 0 {
				t.Error("a node without a container")
			}
		}
	}
	if vls != 1 || subs != 1 {
		t.Errorf("voltage levels %d, substations %d; want 1 and 1", vls, subs)
	}
	voltageWarned := false
	for _, w := range out.Warnings {
		voltageWarned = voltageWarned || w.Kind == "voltage" && w.ID == 2
	}
	if !voltageWarned {
		t.Errorf("the class without a voltage should be reported: %+v", out.Warnings)
	}
}

func TestCIMClassTable(t *testing.T) {
	shapes := map[string]Class{"41": ClassBreaker, "43": ClassBreaker, "162": ClassDisconnector, "3": ClassRectangle}
	for _, bad := range []map[string][]string{
		{"3": {"Breaker"}},              // a rectangle is not exported as equipment
		{"999": {"Breaker"}},            // no such shape
		{"41": {}},                      // empty
		{"41": {"SynchronousMachine"}},  // not a switch
		{"41": {"Breaker", "Breaker"}},  // twice
		{LinkToObjectCode: {"Breaker"}}, // a link is an injection
	} {
		if _, err := NewCIMClassTable(shapes, bad); err == nil {
			t.Errorf("NewCIMClassTable(%v) should fail", bad)
		}
	}
	table, err := NewCIMClassTable(shapes, map[string][]string{
		"43":             {"DisconnectingCircuitBreaker", "Breaker"},
		"162":            {"Jumper", "Disconnector"},
		LinkToObjectCode: {"EnergySource", "EnergyConsumer"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(table.Of("41"), CIMClasses(ClassBreaker)) || table.Of("43")[0] != "DisconnectingCircuitBreaker" {
		t.Errorf("41 keeps its class's list and 43 gets its own: %v, %v", table.Of("41"), table.Of("43"))
	}

	// A shape's configured default is what its elements export as, and the CIM class comes back as that shape.
	d := &Diagram{
		Nodes: []Node{{ID: 1}, {ID: 2}, {ID: 3, Y: 10}, {ID: 4}, {ID: 5}},
		Elements: []Element{
			{ID: 10, Class: ClassDisconnector, Shape: "162", Name: "D1", Ports: []Port{{"1", 1}, {"2", 2}}},
			{ID: 11, Class: ClassBreaker, Shape: "43", Name: "Q43", Ports: []Port{{"1", 4}, {"2", 5}}},
		},
		Connectors: []Connector{{ID: 20, Kind: KindLinkToObject, Name: "W", From: 2, To: 3, Points: []Point{{0, 0}, {0, 10}}}},
	}
	out := ExportCIM(d, CIMOptions{Name: "t", Created: time.Unix(0, 0), Classes: table})
	classes := map[string]bool{}
	for _, o := range parseRDF(t, out.EQ) {
		classes[o.class] = true
	}
	if !classes["Jumper"] || classes["Disconnector"] || !classes["EnergySource"] || !classes["DisconnectingCircuitBreaker"] {
		t.Errorf("exported classes = %v", classes)
	}
	terms := map[string][]Point{"162": {{X: 0, Y: -20}, {X: 0, Y: 20}}, "41": {{X: 0, Y: -20}, {X: 0, Y: 20}}, "43": {{X: 0, Y: -20}, {X: 0, Y: 20}}}
	pages, err := ImportCIM([][]byte{out.EQ, out.DL}, CIMImportOptions{Terminals: terms, Classes: shapes, CIMClasses: table})
	if err != nil {
		t.Fatal(err)
	}
	back := map[string]Element{}
	for _, e := range pages[0].Diagram.Elements {
		back[e.Name] = e
	}
	if e := back["D1"]; e.Shape != "162" || e.CIM != "" {
		t.Errorf("D1 back = %+v (Jumper is its shape's default, so no choice is stored)", e)
	}
	if e := back["Q43"]; e.Shape != "43" || e.CIM != "" {
		t.Errorf("Q43 back = %+v", e)
	}
}

// A line between two equipped substations keeps them apart; a bare line end joins the substation across the line.
func TestExportCIMSubstations(t *testing.T) {
	d := &Diagram{
		VoltageClasses: []VoltageClass{{ID: 1, Name: "10 kV"}},
		Nodes:          []Node{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}, {ID: 5}},
		Elements: []Element{
			{ID: 10, Class: ClassBreaker, Shape: "41", Name: "QA", Voltage: 1, Ports: []Port{{"1", 1}, {"2", 2}}},
			{ID: 11, Class: ClassBreaker, Shape: "41", Name: "QB", Voltage: 1, Ports: []Port{{"1", 3}, {"2", 4}}},
		},
		Connectors: []Connector{
			{ID: 20, Kind: KindOverheadLine, Name: "AB", Voltage: 1, From: 2, To: 3, Points: []Point{{0, 0}, {10, 0}}},
			{ID: 21, Kind: KindOverheadLine, Name: "B-end", Voltage: 1, From: 4, To: 5, Points: []Point{{20, 0}, {30, 0}}},
		},
	}
	out := ExportCIM(d, CIMOptions{Name: "t", Created: time.Unix(0, 0)})
	subs := 0
	for _, o := range parseRDF(t, out.EQ) {
		if o.class == "Substation" {
			subs++
		}
	}
	if subs != 2 {
		t.Errorf("Substations = %d, want 2: QA's and QB's (the bare end of B-end joining QB's)", subs)
	}
}

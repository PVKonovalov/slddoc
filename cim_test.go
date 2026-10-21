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
	if n := count(eq, "VoltageLevel"); n != 2 {
		t.Errorf("VoltageLevels = %d, want 2 (the unused class is left out)", n)
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
		if got, ok := parseVoltage(name); !ok || got != want {
			t.Errorf("parseVoltage(%q) = %v, %v; want %v", name, got, ok, want)
		}
	}
	if _, ok := parseVoltage("aux"); ok {
		t.Error(`parseVoltage("aux") should fail`)
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
	if !slices.Equal(svc, []string{"capacitiveRating", "inductiveRating"}) {
		t.Errorf("SVC warnings = %v", svc)
	}
	if got := CIMClasses(ClassLoad); !slices.Equal(got, []string{"EnergyConsumer", "ConformLoad", "NonConformLoad"}) {
		t.Errorf("CIMClasses(Load) = %v", got)
	}
	if CIMClasses(ClassRectangle) != nil || CIMClasses(ClassJunctionPoint) != nil {
		t.Error("non-equipment classes have no CIM classes")
	}
}

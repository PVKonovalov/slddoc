package slddoc

import (
	"math"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

var testCIMTerminals = map[string][]Point{
	"14":     {{X: -10, Y: 0}, {X: 10, Y: 0}},
	"41":     {{X: 0, Y: -20}, {X: 0, Y: 20}},
	"330001": {{X: 0, Y: 0}},
}

var testCIMClasses = map[string]Class{"41": ClassBreaker, "24": ClassBusBarSection, "330001": ClassLoad, "47": ClassPowerTransformer}

// A foreign CGMES 2.4.15-style model: a y-up layout in small units, no wires drawn, equipment and layout in
// separate documents.
const foreignEQ = `<?xml version="1.0" encoding="utf-8"?>
<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#" xmlns:cim="http://iec.ch/TC57/2013/CIM-schema-cim16#">
  <cim:BaseVoltage rdf:ID="_bv"><cim:BaseVoltage.nominalVoltage>10.5</cim:BaseVoltage.nominalVoltage></cim:BaseVoltage>
  <cim:VoltageLevel rdf:ID="_vl"><cim:VoltageLevel.BaseVoltage rdf:resource="#_bv"/></cim:VoltageLevel>
  <cim:ConnectivityNode rdf:ID="_cn1"><cim:ConnectivityNode.ConnectivityNodeContainer rdf:resource="#_vl"/></cim:ConnectivityNode>
  <cim:ConnectivityNode rdf:ID="_cn2"><cim:ConnectivityNode.ConnectivityNodeContainer rdf:resource="#_vl"/></cim:ConnectivityNode>
  <cim:BusbarSection rdf:ID="_bus"><cim:IdentifiedObject.name>BUS</cim:IdentifiedObject.name><cim:Equipment.EquipmentContainer rdf:resource="#_vl"/></cim:BusbarSection>
  <cim:Terminal rdf:ID="_tb"><cim:Terminal.ConductingEquipment rdf:resource="#_bus"/><cim:Terminal.ConnectivityNode rdf:resource="#_cn1"/><cim:Terminal.sequenceNumber>1</cim:Terminal.sequenceNumber></cim:Terminal>
  <cim:Breaker rdf:ID="_q1"><cim:IdentifiedObject.name>Q1</cim:IdentifiedObject.name><cim:Switch.normalOpen>true</cim:Switch.normalOpen><cim:Equipment.EquipmentContainer rdf:resource="#_vl"/></cim:Breaker>
  <cim:Terminal rdf:ID="_tq1"><cim:Terminal.ConductingEquipment rdf:resource="#_q1"/><cim:Terminal.ConnectivityNode rdf:resource="#_cn1"/><cim:Terminal.sequenceNumber>1</cim:Terminal.sequenceNumber></cim:Terminal>
  <cim:Terminal rdf:ID="_tq2"><cim:Terminal.ConductingEquipment rdf:resource="#_q1"/><cim:Terminal.ConnectivityNode rdf:resource="#_cn2"/><cim:Terminal.sequenceNumber>2</cim:Terminal.sequenceNumber></cim:Terminal>
  <cim:EnergyConsumer rdf:ID="_ld"><cim:IdentifiedObject.name>LOAD</cim:IdentifiedObject.name><cim:EnergyConsumer.pfixed>2.5</cim:EnergyConsumer.pfixed><cim:Equipment.EquipmentContainer rdf:resource="#_vl"/></cim:EnergyConsumer>
  <cim:Terminal rdf:ID="_tl"><cim:Terminal.ConductingEquipment rdf:resource="#_ld"/><cim:Terminal.ConnectivityNode rdf:resource="#_cn2"/></cim:Terminal>
  <cim:AsynchronousMachine rdf:ID="_am"><cim:IdentifiedObject.name>M1</cim:IdentifiedObject.name></cim:AsynchronousMachine>
  <cim:DCLineSegment rdf:ID="_dc"/>
  <cim:Terminal rdf:ID="_tdc"><cim:Terminal.ConductingEquipment rdf:resource="#_dc"/></cim:Terminal>
</rdf:RDF>`

const foreignDL = `<?xml version="1.0" encoding="utf-8"?>
<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#" xmlns:cim="http://iec.ch/TC57/2013/CIM-schema-cim16#">
  <cim:Diagram rdf:ID="_d"><cim:IdentifiedObject.name>Feeder</cim:IdentifiedObject.name>
    <cim:Diagram.orientation rdf:resource="http://iec.ch/TC57/2013/CIM-schema-cim16#OrientationKind.positive"/></cim:Diagram>
  <cim:DiagramObject rdf:ID="_dob"><cim:DiagramObject.Diagram rdf:resource="#_d"/><cim:DiagramObject.IdentifiedObject rdf:resource="#_bus"/></cim:DiagramObject>
  <cim:DiagramObjectPoint rdf:ID="_p1"><cim:DiagramObjectPoint.DiagramObject rdf:resource="#_dob"/><cim:DiagramObjectPoint.sequenceNumber>1</cim:DiagramObjectPoint.sequenceNumber><cim:DiagramObjectPoint.xPosition>0</cim:DiagramObjectPoint.xPosition><cim:DiagramObjectPoint.yPosition>1</cim:DiagramObjectPoint.yPosition></cim:DiagramObjectPoint>
  <cim:DiagramObjectPoint rdf:ID="_p2"><cim:DiagramObjectPoint.DiagramObject rdf:resource="#_dob"/><cim:DiagramObjectPoint.sequenceNumber>2</cim:DiagramObjectPoint.sequenceNumber><cim:DiagramObjectPoint.xPosition>1</cim:DiagramObjectPoint.xPosition><cim:DiagramObjectPoint.yPosition>1</cim:DiagramObjectPoint.yPosition></cim:DiagramObjectPoint>
  <cim:DiagramObject rdf:ID="_doq"><cim:DiagramObject.Diagram rdf:resource="#_d"/><cim:DiagramObject.IdentifiedObject rdf:resource="#_q1"/></cim:DiagramObject>
  <cim:DiagramObjectPoint rdf:ID="_p3"><cim:DiagramObjectPoint.DiagramObject rdf:resource="#_doq"/><cim:DiagramObjectPoint.xPosition>0.5</cim:DiagramObjectPoint.xPosition><cim:DiagramObjectPoint.yPosition>0.8</cim:DiagramObjectPoint.yPosition></cim:DiagramObjectPoint>
  <cim:DiagramObject rdf:ID="_dol"><cim:DiagramObject.Diagram rdf:resource="#_d"/><cim:DiagramObject.IdentifiedObject rdf:resource="#_ld"/></cim:DiagramObject>
  <cim:DiagramObjectPoint rdf:ID="_p4"><cim:DiagramObjectPoint.DiagramObject rdf:resource="#_dol"/><cim:DiagramObjectPoint.xPosition>0.5</cim:DiagramObjectPoint.xPosition><cim:DiagramObjectPoint.yPosition>0.55</cim:DiagramObjectPoint.yPosition></cim:DiagramObjectPoint>
  <cim:DiagramObject rdf:ID="_dom"><cim:DiagramObject.Diagram rdf:resource="#_d"/><cim:DiagramObject.IdentifiedObject rdf:resource="#_am"/></cim:DiagramObject>
  <cim:DiagramObjectPoint rdf:ID="_p5"><cim:DiagramObjectPoint.DiagramObject rdf:resource="#_dom"/><cim:DiagramObjectPoint.xPosition>0.9</cim:DiagramObjectPoint.xPosition><cim:DiagramObjectPoint.yPosition>0.55</cim:DiagramObjectPoint.yPosition></cim:DiagramObjectPoint>
  <cim:DiagramObject rdf:ID="_dodc"><cim:DiagramObject.Diagram rdf:resource="#_d"/><cim:DiagramObject.IdentifiedObject rdf:resource="#_dc"/></cim:DiagramObject>
  <cim:DiagramObjectPoint rdf:ID="_p6"><cim:DiagramObjectPoint.DiagramObject rdf:resource="#_dodc"/><cim:DiagramObjectPoint.xPosition>0.1</cim:DiagramObjectPoint.xPosition><cim:DiagramObjectPoint.yPosition>0.2</cim:DiagramObjectPoint.yPosition></cim:DiagramObjectPoint>
</rdf:RDF>`

func TestImportCIMForeignLayout(t *testing.T) {
	pages, err := ImportCIM([][]byte{[]byte(foreignEQ), []byte(foreignDL)}, CIMImportOptions{Terminals: testCIMTerminals, Classes: testCIMClasses, Grid: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 1 || pages[0].Title != "Feeder" {
		t.Fatalf("pages = %+v", pages)
	}
	d := pages[0].Diagram
	byName := map[string]Element{}
	for _, e := range d.Elements {
		byName[e.Name] = e
	}
	bus, q1, load := byName["BUS"], byName["Q1"], byName["LOAD"]
	if bus.Class != ClassBusBarSection || q1.Class != ClassBreaker || load.Class != ClassLoad {
		t.Fatalf("elements = %+v", d.Elements)
	}
	if q1.State == nil || *q1.State != 0 {
		t.Error("normalOpen=true should give state 0")
	}
	if load.Electrical == nil || load.Electrical.P == nil || *load.Electrical.P != 2.5 {
		t.Error("pfixed not read")
	}
	// y-up: the busbar (y=1) is above the breaker (y=0.8), which is above the load.
	if !(bus.Points[0].Y < q1.Y && q1.Y < load.Y) {
		t.Errorf("y not flipped: bus %v, Q1 %v, load %v", bus.Points, q1.Y, load.Y)
	}
	// Scaled up from 0..1 units and snapped to the grid.
	if q1.Y-bus.Points[0].Y < 20 || int(q1.X)%10 != 0 || int(q1.Y)%10 != 0 {
		t.Errorf("not scaled/snapped: bus %v, Q1 at %v,%v", bus.Points, q1.X, q1.Y)
	}
	if len(d.VoltageClasses) != 1 || d.VoltageClasses[0].Name != "10.5 kV" || q1.Voltage != d.VoltageClasses[0].ID {
		t.Errorf("voltage classes = %+v, Q1 voltage %d", d.VoltageClasses, q1.Voltage)
	}
	// Two auto-routed wires: Q1 to the busbar and the load to Q1, one of them ending on the busbar's line.
	var wires []Connector
	for _, c := range d.Connectors {
		if c.Kind == KindBusWork {
			wires = append(wires, c)
		}
	}
	if len(wires) != 2 {
		t.Fatalf("wires = %+v", wires)
	}
	onBus := false
	for _, w := range wires {
		for _, p := range []Point{w.Points[0], w.Points[len(w.Points)-1]} {
			if p.Y == bus.Points[0].Y && p.X >= bus.Points[0].X && p.X <= bus.Points[1].X {
				onBus = true
			}
		}
	}
	if !onBus {
		t.Errorf("no wire reaches the busbar: %+v", wires)
	}
	if pages[0].Report.Skipped["DCLineSegment"] != 1 {
		t.Errorf("skipped = %v", pages[0].Report.Skipped)
	}
	if !strings.Contains(strings.Join(pages[0].Messages, ";"), "routed automatically") {
		t.Errorf("messages = %v", pages[0].Messages)
	}
	if byName["M1"].CIM != "AsynchronousMachine" || byName["M1"].Class != ClassSynchronousMotor {
		t.Errorf("asynchronous machine = %+v", byName["M1"])
	}
}

// cimDoc wraps CIM objects in an RDF document; refs are written "#id".
func cimDoc(objects string) []byte {
	return []byte(`<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#" xmlns:cim="http://iec.ch/TC57/CIM100#">` +
		objects + `</rdf:RDF>`)
}

func dlObject(id, of string, pts ...float64) string {
	var s strings.Builder
	s.WriteString(`<cim:DiagramObject rdf:ID="` + id + `"><cim:DiagramObject.Diagram rdf:resource="#d"/><cim:DiagramObject.IdentifiedObject rdf:resource="#` + of + `"/></cim:DiagramObject>`)
	for i := 0; i+1 < len(pts); i += 2 {
		s.WriteString(`<cim:DiagramObjectPoint rdf:ID="` + id + `p` + strconv.Itoa(i) + `"><cim:DiagramObjectPoint.DiagramObject rdf:resource="#` + id +
			`"/><cim:DiagramObjectPoint.sequenceNumber>` + strconv.Itoa(i/2+1) + `</cim:DiagramObjectPoint.sequenceNumber><cim:DiagramObjectPoint.xPosition>` +
			strconv.FormatFloat(pts[i], 'f', -1, 64) + `</cim:DiagramObjectPoint.xPosition><cim:DiagramObjectPoint.yPosition>` +
			strconv.FormatFloat(pts[i+1], 'f', -1, 64) + `</cim:DiagramObjectPoint.yPosition></cim:DiagramObjectPoint>`)
	}
	return s.String()
}

func terminal(id, eq, node, nodeKind string, seq int) string {
	return `<cim:Terminal rdf:ID="` + id + `"><cim:Terminal.ConductingEquipment rdf:resource="#` + eq + `"/><cim:Terminal.` + nodeKind +
		` rdf:resource="#` + node + `"/><cim:ACDCTerminal.sequenceNumber>` + strconv.Itoa(seq) + `</cim:ACDCTerminal.sequenceNumber></cim:Terminal>`
}

// The connections come from the model, never from the drawing: a foreign layout's wire of one node crossing
// another's gets a wire jump when their voltage classes differ (none when they match, as in manual drawing), two
// busbar sections of one node touching end to end are joined, and a bus-branch model's TopologicalNodes stand in for
// ConnectivityNodes.
func TestImportCIMConnectionsFromModel(t *testing.T) {
	for _, tc := range []struct {
		kind, bVL string
		jumps     int
	}{{"ConnectivityNode", "v2", 1}, {"TopologicalNode", "v2", 1}, {"ConnectivityNode", "v1", 0}} {
		kind := tc.kind
		eq := `<cim:BaseVoltage rdf:ID="bv1"><cim:BaseVoltage.nominalVoltage>10</cim:BaseVoltage.nominalVoltage></cim:BaseVoltage>
<cim:BaseVoltage rdf:ID="bv2"><cim:BaseVoltage.nominalVoltage>0.4</cim:BaseVoltage.nominalVoltage></cim:BaseVoltage>
<cim:VoltageLevel rdf:ID="v1"><cim:VoltageLevel.BaseVoltage rdf:resource="#bv1"/></cim:VoltageLevel>
<cim:VoltageLevel rdf:ID="v2"><cim:VoltageLevel.BaseVoltage rdf:resource="#bv2"/></cim:VoltageLevel>
<cim:` + kind + ` rdf:ID="a"><cim:` + kind + `.ConnectivityNodeContainer rdf:resource="#v1"/></cim:` + kind + `>
<cim:` + kind + ` rdf:ID="b"><cim:` + kind + `.ConnectivityNodeContainer rdf:resource="#` + tc.bVL + `"/></cim:` + kind + `>
<cim:BusbarSection rdf:ID="B1"><cim:IdentifiedObject.name>B1</cim:IdentifiedObject.name></cim:BusbarSection>` + terminal("tb1", "B1", "a", kind, 1) + `
<cim:BusbarSection rdf:ID="B2"><cim:IdentifiedObject.name>B2</cim:IdentifiedObject.name></cim:BusbarSection>` + terminal("tb2", "B2", "a", kind, 1) + `
<cim:Breaker rdf:ID="Q1"><cim:IdentifiedObject.name>Q1</cim:IdentifiedObject.name></cim:Breaker>` + terminal("tq1", "Q1", "a", kind, 1) + terminal("tq2", "Q1", "b", kind, 2) + `
<cim:EnergyConsumer rdf:ID="L"><cim:IdentifiedObject.name>L</cim:IdentifiedObject.name></cim:EnergyConsumer>` + terminal("tl", "L", "b", kind, 1)
		dl := `<cim:Diagram rdf:ID="d"><cim:IdentifiedObject.name>D</cim:IdentifiedObject.name></cim:Diagram>` +
			dlObject("o1", "B1", 0, 0, 3, 0) + dlObject("o2", "B2", 3, 0, 6, 0) + dlObject("o3", "Q1", 5, 2) + dlObject("o4", "L", 5, 4) +
			// a's wire from the busbar down across b's wire.
			dlObject("o5", "a", 1, 0, 1, 3) + dlObject("o6", "b", 0, 2.5, 3, 2.5)
		pages, err := ImportCIM([][]byte{cimDoc(eq), cimDoc(dl)}, CIMImportOptions{Terminals: testCIMTerminals, Classes: testCIMClasses, Grid: 10})
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		d := pages[0].Diagram
		jumps := 0
		for _, e := range d.Elements {
			if e.Class == ClassNonIntersection {
				jumps++
			}
		}
		if jumps != tc.jumps {
			t.Errorf("%s, b in %s: wire jumps = %d, want %d where a's wire crosses b's", kind, tc.bVL, jumps, tc.jumps)
		}
		n := terminalNodes(t, d)
		if n["B1/1"] == "" || n["B1/1"] != n["B2/1"] || n["B1/1"] != n["Q1/1"] {
			t.Errorf("%s: B1, B2 and Q1/1 should share a node: %v", kind, n)
		}
		if n["Q1/2"] == "" || n["Q1/2"] != n["L/1"] || n["Q1/2"] == n["Q1/1"] {
			t.Errorf("%s: Q1/2 and L should share a node of their own: %v", kind, n)
		}
	}
}

// TestCIMJumpsAndJunctionsRoundTrip: a 110 kV wire leaving its busbar crosses a 10 kV busbar. As in manual drawing
// it gets one wire jump, size step 1 over a busbar, and the busbar–buswork connection a junction point; exported to
// CIM and imported back, the jump keeps its size (no port off its terminal) and no second junction point appears.
func TestCIMJumpsAndJunctionsRoundTrip(t *testing.T) {
	eq := `<cim:BaseVoltage rdf:ID="bv1"><cim:BaseVoltage.nominalVoltage>110</cim:BaseVoltage.nominalVoltage></cim:BaseVoltage>
<cim:BaseVoltage rdf:ID="bv2"><cim:BaseVoltage.nominalVoltage>10</cim:BaseVoltage.nominalVoltage></cim:BaseVoltage>
<cim:VoltageLevel rdf:ID="v1"><cim:VoltageLevel.BaseVoltage rdf:resource="#bv1"/></cim:VoltageLevel>
<cim:VoltageLevel rdf:ID="v2"><cim:VoltageLevel.BaseVoltage rdf:resource="#bv2"/></cim:VoltageLevel>
<cim:ConnectivityNode rdf:ID="a"><cim:ConnectivityNode.ConnectivityNodeContainer rdf:resource="#v1"/></cim:ConnectivityNode>
<cim:ConnectivityNode rdf:ID="b"><cim:ConnectivityNode.ConnectivityNodeContainer rdf:resource="#v1"/></cim:ConnectivityNode>
<cim:ConnectivityNode rdf:ID="x"><cim:ConnectivityNode.ConnectivityNodeContainer rdf:resource="#v2"/></cim:ConnectivityNode>
<cim:BusbarSection rdf:ID="B1"><cim:IdentifiedObject.name>B1</cim:IdentifiedObject.name><cim:Equipment.EquipmentContainer rdf:resource="#v1"/></cim:BusbarSection>` +
		terminal("tb1", "B1", "a", "ConnectivityNode", 1) + `
<cim:BusbarSection rdf:ID="BX"><cim:IdentifiedObject.name>BX</cim:IdentifiedObject.name><cim:Equipment.EquipmentContainer rdf:resource="#v2"/></cim:BusbarSection>` +
		terminal("tbx", "BX", "x", "ConnectivityNode", 1) + `
<cim:Breaker rdf:ID="Q1"><cim:IdentifiedObject.name>Q1</cim:IdentifiedObject.name><cim:Equipment.EquipmentContainer rdf:resource="#v1"/></cim:Breaker>` +
		terminal("tq1", "Q1", "a", "ConnectivityNode", 1) + terminal("tq2", "Q1", "b", "ConnectivityNode", 2) + `
<cim:EnergyConsumer rdf:ID="L"><cim:IdentifiedObject.name>L</cim:IdentifiedObject.name><cim:Equipment.EquipmentContainer rdf:resource="#v1"/></cim:EnergyConsumer>` +
		terminal("tl", "L", "b", "ConnectivityNode", 1)
	dl := `<cim:Diagram rdf:ID="d"><cim:IdentifiedObject.name>D</cim:IdentifiedObject.name></cim:Diagram>` +
		dlObject("o1", "B1", 0, 0, 6, 0) + dlObject("o2", "BX", 0, 2, 6, 2) + dlObject("o3", "Q1", 1, 5) + dlObject("o4", "L", 1, 8) +
		dlObject("o5", "a", 1, 0, 1, 4)
	terms := map[string][]Point{"41": {{X: 0, Y: -20}, {X: 0, Y: 20}}, "330001": {{X: 0, Y: 0}}, "14": {{X: -10}, {X: 10}}, "7": {{}}}
	classes := map[string]Class{"41": ClassBreaker, "24": ClassBusBarSection, "330001": ClassLoad, "14": ClassNonIntersection,
		"7": ClassJunctionPoint}
	opt := CIMImportOptions{Terminals: terms, Classes: classes, Grid: 10}
	pages, err := ImportCIM([][]byte{cimDoc(eq), cimDoc(dl)}, opt)
	if err != nil {
		t.Fatal(err)
	}
	census := func(d *Diagram) (jumps []int, dots int) {
		for _, e := range d.Elements {
			switch e.Class {
			case ClassNonIntersection:
				jumps = append(jumps, e.Scale)
			case ClassJunctionPoint:
				dots++
			}
		}
		return jumps, dots
	}
	d := pages[0].Diagram
	jumps, dots := census(d)
	if !slices.Equal(jumps, []int{1}) || dots != 1 {
		t.Fatalf("imported: wire jump size steps %v, junction points %d; want [1] and 1", jumps, dots)
	}
	out := ExportCIM(d, CIMOptions{Name: "t", Created: time.Unix(0, 0)})
	back, err := ImportCIM([][]byte{out.EQ, out.DL}, opt)
	if err != nil {
		t.Fatal(err)
	}
	if jumps2, dots2 := census(back[0].Diagram); !slices.Equal(jumps2, jumps) || dots2 != dots {
		t.Errorf("round trip: wire jumps %v -> %v, junction points %d -> %d", jumps, jumps2, dots, dots2)
	}
	if off := offTerminals(back[0].Diagram, terms); len(off) > 0 {
		t.Errorf("round trip: ports off their terminals: %v", off)
	}
}

// TestCIMOneJumpPerCrossing: two buswork wires of different voltage classes crossing mid-way get one wire jump, in
// the first of them; the other passes through its gap, as in manual drawing.
func TestCIMOneJumpPerCrossing(t *testing.T) {
	var eq strings.Builder
	eq.WriteString(`<cim:BaseVoltage rdf:ID="bv1"><cim:BaseVoltage.nominalVoltage>110</cim:BaseVoltage.nominalVoltage></cim:BaseVoltage>
<cim:BaseVoltage rdf:ID="bv2"><cim:BaseVoltage.nominalVoltage>10</cim:BaseVoltage.nominalVoltage></cim:BaseVoltage>
<cim:VoltageLevel rdf:ID="v1"><cim:VoltageLevel.BaseVoltage rdf:resource="#bv1"/></cim:VoltageLevel>
<cim:VoltageLevel rdf:ID="v2"><cim:VoltageLevel.BaseVoltage rdf:resource="#bv2"/></cim:VoltageLevel>
<cim:ConnectivityNode rdf:ID="a"><cim:ConnectivityNode.ConnectivityNodeContainer rdf:resource="#v1"/></cim:ConnectivityNode>
<cim:ConnectivityNode rdf:ID="b"><cim:ConnectivityNode.ConnectivityNodeContainer rdf:resource="#v2"/></cim:ConnectivityNode>`)
	for _, l := range [][2]string{{"L1", "a"}, {"L2", "a"}, {"L3", "b"}, {"L4", "b"}} {
		eq.WriteString(`<cim:EnergyConsumer rdf:ID="` + l[0] + `"><cim:IdentifiedObject.name>` + l[0] + `</cim:IdentifiedObject.name></cim:EnergyConsumer>` +
			terminal("t"+l[0], l[0], l[1], "ConnectivityNode", 1))
	}
	dl := `<cim:Diagram rdf:ID="d"><cim:IdentifiedObject.name>D</cim:IdentifiedObject.name></cim:Diagram>` +
		dlObject("o1", "L1", 0, 5) + dlObject("o2", "L2", 10, 5) + dlObject("o3", "L3", 5, 0) + dlObject("o4", "L4", 5, 10) +
		dlObject("o5", "a", 0, 5, 10, 5) + dlObject("o6", "b", 5, 0, 5, 10)
	pages, err := ImportCIM([][]byte{cimDoc(eq.String()), cimDoc(dl)}, CIMImportOptions{Terminals: testCIMTerminals, Classes: testCIMClasses, Grid: 10})
	if err != nil {
		t.Fatal(err)
	}
	jumps := 0
	for _, e := range pages[0].Diagram.Elements {
		if e.Class == ClassNonIntersection {
			jumps++
		}
	}
	if jumps != 1 {
		t.Errorf("wire jumps = %d, want 1 at the crossing", jumps)
	}
}

// TestCIMNamesWithoutTexts: a foreign layout that writes no texts gets each named device and busbar labelled with
// its name, two names that would overlap one over the other; one that writes a text keeps just its own. The pages
// tell who drew them, and AutoLayout lays the same documents out automatically.
func TestCIMNamesWithoutTexts(t *testing.T) {
	eq := `<cim:BaseVoltage rdf:ID="bv"><cim:BaseVoltage.nominalVoltage>110</cim:BaseVoltage.nominalVoltage></cim:BaseVoltage>
<cim:VoltageLevel rdf:ID="v"><cim:VoltageLevel.BaseVoltage rdf:resource="#bv"/></cim:VoltageLevel>
<cim:ConnectivityNode rdf:ID="a"><cim:ConnectivityNode.ConnectivityNodeContainer rdf:resource="#v"/></cim:ConnectivityNode>
<cim:ConnectivityNode rdf:ID="b"><cim:ConnectivityNode.ConnectivityNodeContainer rdf:resource="#v"/></cim:ConnectivityNode>
<cim:BusbarSection rdf:ID="B1"><cim:IdentifiedObject.name>BUS1</cim:IdentifiedObject.name><cim:Equipment.EquipmentContainer rdf:resource="#v"/></cim:BusbarSection>` +
		terminal("tb", "B1", "a", "ConnectivityNode", 1) + `
<cim:Breaker rdf:ID="Q1"><cim:IdentifiedObject.name>BREAKER1</cim:IdentifiedObject.name><cim:Equipment.EquipmentContainer rdf:resource="#v"/></cim:Breaker>` +
		terminal("tq1", "Q1", "a", "ConnectivityNode", 1) + terminal("tq2", "Q1", "b", "ConnectivityNode", 2) + `
<cim:Breaker rdf:ID="Q2"><cim:IdentifiedObject.name>BREAKER2</cim:IdentifiedObject.name><cim:Equipment.EquipmentContainer rdf:resource="#v"/></cim:Breaker>` +
		terminal("tq3", "Q2", "b", "ConnectivityNode", 1) + terminal("tq4", "Q2", "a", "ConnectivityNode", 2)
	dl := `<cim:Diagram rdf:ID="d"><cim:IdentifiedObject.name>D</cim:IdentifiedObject.name></cim:Diagram>` +
		dlObject("o1", "B1", 0, 0, 10, 0) + dlObject("o2", "Q1", 2, 4) + dlObject("o3", "Q2", 3, 4)
	opt := CIMImportOptions{Terminals: testCIMTerminals, Classes: testCIMClasses, Grid: 10}
	pages, err := ImportCIM([][]byte{cimDoc(eq), cimDoc(dl)}, opt)
	if err != nil {
		t.Fatal(err)
	}
	if pages[0].Layout != CIMLayoutForeign {
		t.Errorf("layout = %q, want foreign", pages[0].Layout)
	}
	byText := map[string]Label{}
	for _, l := range pages[0].Diagram.Labels {
		byText[l.Text] = l
	}
	for _, name := range []string{"BUS1", "BREAKER1", "BREAKER2"} {
		if _, ok := byText[name]; !ok {
			t.Errorf("no label %q in %+v", name, pages[0].Diagram.Labels)
		}
	}
	if a, b := byText["BREAKER1"], byText["BREAKER2"]; a.Y == b.Y && math.Abs(a.X-b.X) < 6*float64(len(a.Text)) {
		t.Errorf("names overlap: %+v %+v", a, b)
	}

	text := `<cim:TextDiagramObject rdf:ID="x"><cim:DiagramObject.Diagram rdf:resource="#d"/><cim:DiagramObject.IdentifiedObject rdf:resource="#Q1"/><cim:TextDiagramObject.text>Q1</cim:TextDiagramObject.text></cim:TextDiagramObject>` +
		`<cim:DiagramObjectPoint rdf:ID="xp"><cim:DiagramObjectPoint.DiagramObject rdf:resource="#x"/><cim:DiagramObjectPoint.xPosition>2</cim:DiagramObjectPoint.xPosition><cim:DiagramObjectPoint.yPosition>3</cim:DiagramObjectPoint.yPosition></cim:DiagramObjectPoint>`
	pages, err = ImportCIM([][]byte{cimDoc(eq), cimDoc(dl + text)}, opt)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(pages[0].Diagram.Labels); n != 1 {
		t.Errorf("a layout with its own texts keeps them alone: %d labels", n)
	}

	opt.AutoLayout = true
	pages, err = ImportCIM([][]byte{cimDoc(eq), cimDoc(dl)}, opt)
	if err != nil {
		t.Fatal(err)
	}
	if pages[0].Layout != CIMLayoutGenerated {
		t.Errorf("auto layout = %q, want generated", pages[0].Layout)
	}
}

// TestCIMKeepsMRIDs: an import keeps the mRIDs of the equipment, its terminals and connectivity nodes, and an
// export writes them back; a copy that kept one gets a new one and a warning, and Nodes joined from two
// connectivity nodes make a new one.
func TestCIMKeepsMRIDs(t *testing.T) {
	eq := `<cim:BaseVoltage rdf:ID="bv"><cim:BaseVoltage.nominalVoltage>110</cim:BaseVoltage.nominalVoltage></cim:BaseVoltage>
<cim:VoltageLevel rdf:ID="v"><cim:VoltageLevel.BaseVoltage rdf:resource="#bv"/></cim:VoltageLevel>
<cim:ConnectivityNode rdf:ID="_a"><cim:IdentifiedObject.mRID>node-a</cim:IdentifiedObject.mRID><cim:ConnectivityNode.ConnectivityNodeContainer rdf:resource="#v"/></cim:ConnectivityNode>
<cim:ConnectivityNode rdf:ID="_b"><cim:ConnectivityNode.ConnectivityNodeContainer rdf:resource="#v"/></cim:ConnectivityNode>
<cim:BusbarSection rdf:ID="_B1"><cim:IdentifiedObject.mRID>bus-1</cim:IdentifiedObject.mRID><cim:IdentifiedObject.name>B1</cim:IdentifiedObject.name><cim:Equipment.EquipmentContainer rdf:resource="#v"/></cim:BusbarSection>` +
		terminal("_tb", "_B1", "_a", "ConnectivityNode", 1) + `
<cim:Breaker rdf:ID="_Q1"><cim:IdentifiedObject.mRID>breaker-1</cim:IdentifiedObject.mRID><cim:IdentifiedObject.name>Q1</cim:IdentifiedObject.name><cim:Equipment.EquipmentContainer rdf:resource="#v"/></cim:Breaker>` +
		terminal("_tq1", "_Q1", "_a", "ConnectivityNode", 1) + terminal("_tq2", "_Q1", "_b", "ConnectivityNode", 2) + `
<cim:EnergyConsumer rdf:ID="_L"><cim:IdentifiedObject.name>L</cim:IdentifiedObject.name><cim:Equipment.EquipmentContainer rdf:resource="#v"/></cim:EnergyConsumer>` +
		terminal("_tl", "_L", "_b", "ConnectivityNode", 1)
	dl := `<cim:Diagram rdf:ID="d"><cim:IdentifiedObject.name>D</cim:IdentifiedObject.name></cim:Diagram>` +
		dlObject("o1", "_B1", 0, 0, 10, 0) + dlObject("o2", "_Q1", 2, 4) + dlObject("o3", "_L", 2, 8)
	terms := map[string][]Point{"41": {{X: 0, Y: -20}, {X: 0, Y: 20}}, "330001": {{X: 0, Y: 0}}}
	classes := map[string]Class{"41": ClassBreaker, "24": ClassBusBarSection, "330001": ClassLoad}
	pages, err := ImportCIM([][]byte{cimDoc(eq), cimDoc(dl)}, CIMImportOptions{Terminals: terms, Classes: classes, Grid: 10})
	if err != nil {
		t.Fatal(err)
	}
	d := pages[0].Diagram
	byName := map[string]Element{}
	for _, e := range d.Elements {
		byName[e.Name] = e
	}
	q := byName["Q1"]
	if q.MRID != "breaker-1" || byName["B1"].MRID != "bus-1" || byName["L"].MRID != "L" {
		t.Fatalf("element mRIDs: %q %q %q", q.MRID, byName["B1"].MRID, byName["L"].MRID)
	}
	if len(q.Ports) != 2 || q.Ports[0].MRID != "tq1" || q.Ports[1].MRID != "tq2" {
		t.Errorf("Q1 ports: %+v", q.Ports)
	}
	nodeMRIDs := map[string]bool{}
	for _, n := range d.Nodes {
		nodeMRIDs[n.MRID] = true
	}
	if !nodeMRIDs["node-a"] || !nodeMRIDs["b"] {
		t.Errorf("node mRIDs: %v", nodeMRIDs)
	}

	out := string(ExportCIM(d, CIMOptions{Name: "t", Created: time.Unix(0, 0)}).EQ)
	for _, id := range []string{"breaker-1", "bus-1", "L", "tq1", "tq2", "tb", "tl", "node-a", "b"} {
		if !strings.Contains(out, `rdf:ID="_`+id+`"`) || !strings.Contains(out, "<cim:IdentifiedObject.mRID>"+id+"<") {
			t.Errorf("export lacks mRID %q", id)
		}
	}

	// A copy of Q1 that kept its mRIDs.
	cp := q
	cp.ID = d.LastID + 1
	cp.Ports = []Port{{Name: "1", Node: q.Ports[0].Node, MRID: "tq1"}, {Name: "2", Node: q.Ports[1].Node, MRID: "tq2"}}
	copied := *d
	copied.Elements = append(slices.Clone(d.Elements), cp)
	exp := ExportCIM(&copied, CIMOptions{Name: "t", Created: time.Unix(0, 0)})
	if !slices.ContainsFunc(exp.Warnings, func(w CIMWarning) bool { return w.Kind == "mrid" && w.ID == cp.ID }) {
		t.Errorf("the copy should be warned about: %+v", exp.Warnings)
	}
	if n := strings.Count(string(exp.EQ), `rdf:ID="_breaker-1"`); n != 1 {
		t.Errorf("breaker-1 written %d times", n)
	}

	// The two connectivity nodes joined by a wire.
	joined := *d
	joined.Connectors = append(slices.Clone(d.Connectors), Connector{ID: d.LastID + 2, Kind: KindBusWork,
		From: q.Ports[0].Node, To: q.Ports[1].Node, Points: []Point{{}, {}}})
	if out := string(ExportCIM(&joined, CIMOptions{Name: "t", Created: time.Unix(0, 0)}).EQ); strings.Contains(out, `rdf:ID="_node-a"`) {
		t.Error("joined nodes keep no mRID")
	}
}

// TestCIMClassFromAnyOrder: another profile describing an object by a general class (an SSH file's
// Equipment.inService of a busbar), read before the EQ that defines it, leaves it the class EQ gives.
func TestCIMClassFromAnyOrder(t *testing.T) {
	ssh := cimDoc(`<cim:Equipment rdf:about="#B1"><cim:Equipment.inService>true</cim:Equipment.inService></cim:Equipment>`)
	eq := cimDoc(`<cim:BaseVoltage rdf:ID="bv"><cim:BaseVoltage.nominalVoltage>110</cim:BaseVoltage.nominalVoltage></cim:BaseVoltage>
<cim:VoltageLevel rdf:ID="v"><cim:VoltageLevel.BaseVoltage rdf:resource="#bv"/></cim:VoltageLevel>
<cim:ConnectivityNode rdf:ID="a"><cim:ConnectivityNode.ConnectivityNodeContainer rdf:resource="#v"/></cim:ConnectivityNode>
<cim:BusbarSection rdf:ID="B1"><cim:IdentifiedObject.name>B1</cim:IdentifiedObject.name><cim:Equipment.EquipmentContainer rdf:resource="#v"/></cim:BusbarSection>` +
		terminal("tb", "B1", "a", "ConnectivityNode", 1))
	for _, docs := range [][][]byte{{eq, ssh}, {ssh, eq}} {
		pages, err := ImportCIM(docs, CIMImportOptions{Terminals: testCIMTerminals, Classes: testCIMClasses, Grid: 10})
		if err != nil {
			t.Fatal(err)
		}
		if !slices.ContainsFunc(pages[0].Diagram.Elements, func(e Element) bool { return e.Class == ClassBusBarSection }) {
			t.Errorf("the busbar is lost when SSH comes first: %+v", pages[0].Diagram.Elements)
		}
	}
}

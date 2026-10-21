package slddoc

import (
	"strings"
	"testing"
)

var testCIMTerminals = map[string][]Point{
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

package slddoc

import (
	"fmt"
	"strings"
	"testing"
)

// nodeStyleEQ is one bay of a substation A: busbar BUS, disconnector QS (terminal 1 on the busbar), breaker QB
// (terminal 1 towards QS) and line L1 to substation B.
var nodeStyleEQ = `<?xml version="1.0" encoding="utf-8"?>
<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#" xmlns:cim="http://iec.ch/TC57/CIM100#">
  <cim:BaseVoltage rdf:ID="_bv"><cim:BaseVoltage.nominalVoltage>110</cim:BaseVoltage.nominalVoltage></cim:BaseVoltage>
  <cim:Substation rdf:ID="_sa"><cim:IdentifiedObject.name>A</cim:IdentifiedObject.name></cim:Substation>
  <cim:Substation rdf:ID="_sb"><cim:IdentifiedObject.name>B</cim:IdentifiedObject.name></cim:Substation>
  <cim:VoltageLevel rdf:ID="_va"><cim:VoltageLevel.BaseVoltage rdf:resource="#_bv"/><cim:VoltageLevel.Substation rdf:resource="#_sa"/></cim:VoltageLevel>
  <cim:VoltageLevel rdf:ID="_vb"><cim:VoltageLevel.BaseVoltage rdf:resource="#_bv"/><cim:VoltageLevel.Substation rdf:resource="#_sb"/></cim:VoltageLevel>
  <cim:ConnectivityNode rdf:ID="_cnb"><cim:ConnectivityNode.ConnectivityNodeContainer rdf:resource="#_va"/></cim:ConnectivityNode>
  <cim:ConnectivityNode rdf:ID="_cnm"><cim:ConnectivityNode.ConnectivityNodeContainer rdf:resource="#_va"/></cim:ConnectivityNode>
  <cim:ConnectivityNode rdf:ID="_cnl"><cim:ConnectivityNode.ConnectivityNodeContainer rdf:resource="#_va"/></cim:ConnectivityNode>
  <cim:ConnectivityNode rdf:ID="_cnr"><cim:ConnectivityNode.ConnectivityNodeContainer rdf:resource="#_vb"/></cim:ConnectivityNode>
  <cim:BusbarSection rdf:ID="_bus"><cim:IdentifiedObject.name>BUS</cim:IdentifiedObject.name><cim:Equipment.EquipmentContainer rdf:resource="#_va"/></cim:BusbarSection>
  <cim:Disconnector rdf:ID="_qs"><cim:IdentifiedObject.name>QS</cim:IdentifiedObject.name><cim:Equipment.EquipmentContainer rdf:resource="#_va"/></cim:Disconnector>
  <cim:Breaker rdf:ID="_qb"><cim:IdentifiedObject.name>QB</cim:IdentifiedObject.name><cim:Equipment.EquipmentContainer rdf:resource="#_va"/></cim:Breaker>
  <cim:ACLineSegment rdf:ID="_ln"><cim:IdentifiedObject.name>L1</cim:IdentifiedObject.name><cim:ACLineSegment.r>1.5</cim:ACLineSegment.r></cim:ACLineSegment>
` + nodeStyleTerminals + `</rdf:RDF>`

var nodeStyleTerminals = func() string {
	var b strings.Builder
	for _, t := range []struct{ id, eq, cn, seq string }{
		{"_tb", "_bus", "_cnb", "1"}, {"_ts1", "_qs", "_cnb", "1"}, {"_ts2", "_qs", "_cnm", "2"},
		{"_tq1", "_qb", "_cnm", "1"}, {"_tq2", "_qb", "_cnl", "2"}, {"_tl1", "_ln", "_cnl", "1"}, {"_tl2", "_ln", "_cnr", "2"},
	} {
		fmt.Fprintf(&b, `  <cim:Terminal rdf:ID="%s"><cim:Terminal.ConductingEquipment rdf:resource="#%s"/><cim:Terminal.ConnectivityNode rdf:resource="#%s"/><cim:ACDCTerminal.sequenceNumber>%s</cim:ACDCTerminal.sequenceNumber></cim:Terminal>
`, t.id, t.eq, t.cn, t.seq)
	}
	return b.String()
}()

// nodeStyleDL draws the busbar as its node, QS's busbar terminal as a polyline below QS, QB without terminals, no
// line, and substation B as a marker above the bay.
var nodeStyleDL = func() string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?>
<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#" xmlns:cim="http://iec.ch/TC57/CIM100#">
  <cim:Diagram rdf:ID="_d"><cim:IdentifiedObject.name>A</cim:IdentifiedObject.name></cim:Diagram>
`)
	for _, o := range []struct {
		id, ref string
		pts     [][2]float64
	}{
		{"_dbus", "_cnb", [][2]float64{{0, 100}, {100, 100}}},
		{"_dqs", "_qs", [][2]float64{{50, 80}}},
		{"_dts1", "_ts1", [][2]float64{{50, 88}, {50, 100}}},
		{"_dqb", "_qb", [][2]float64{{50, 50}}},
		{"_dsb", "_sb", [][2]float64{{50, 10}}},
	} {
		fmt.Fprintf(&b, `  <cim:DiagramObject rdf:ID="%s"><cim:DiagramObject.Diagram rdf:resource="#_d"/><cim:DiagramObject.IdentifiedObject rdf:resource="#%s"/></cim:DiagramObject>
`, o.id, o.ref)
		for i, p := range o.pts {
			fmt.Fprintf(&b, `  <cim:DiagramObjectPoint rdf:ID="%s_%d"><cim:DiagramObjectPoint.DiagramObject rdf:resource="#%s"/><cim:DiagramObjectPoint.sequenceNumber>%d</cim:DiagramObjectPoint.sequenceNumber><cim:DiagramObjectPoint.xPosition>%g</cim:DiagramObjectPoint.xPosition><cim:DiagramObjectPoint.yPosition>%g</cim:DiagramObjectPoint.yPosition></cim:DiagramObjectPoint>
`, o.id, i, o.id, i+1, p[0], p[1])
		}
	}
	b.WriteString("</rdf:RDF>")
	return b.String()
}()

func TestImportCIMNodeStyleLayout(t *testing.T) {
	terminals := map[string][]Point{"41": {{X: 0, Y: -10}, {X: 0, Y: 10}}, "162": {{X: 0, Y: -10}, {X: 0, Y: 10}}, "14": {{X: -10}, {X: 10}}}
	classes := map[string]Class{"41": ClassBreaker, "162": ClassDisconnector, "24": ClassBusBarSection, "14": ClassNonIntersection}
	pages, err := ImportCIM([][]byte{[]byte(nodeStyleEQ), []byte(nodeStyleDL)}, CIMImportOptions{Terminals: terminals, Classes: classes, Grid: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 1 {
		t.Fatalf("pages = %d", len(pages))
	}
	d := pages[0].Diagram
	byName := map[string]Element{}
	for _, e := range d.Elements {
		if e.Class == ClassNonIntersection {
			t.Errorf("wire jump at (%g, %g): nothing crosses here", e.X, e.Y)
		}
		byName[e.Name] = e
	}
	bus, qs, qb := byName["BUS"], byName["QS"], byName["QB"]
	if bus.Class != ClassBusBarSection || len(bus.Points) != 2 || bus.Points[0].Y != bus.Points[1].Y {
		t.Fatalf("the node's drawing should be busbar BUS: %+v", d.Elements)
	}
	// QS's terminal 1 is drawn below it, on the busbar's side; QB's terminal 1 faces QS, below it.
	if qs.Orient != 180 || qb.Orient != 180 {
		t.Errorf("orientations: QS %d, QB %d; want 180, 180", qs.Orient, qb.Orient)
	}
	nodeAt := map[int]Point{}
	for _, n := range d.Nodes {
		nodeAt[n.ID] = Point{X: n.X, Y: n.Y}
	}
	port := func(e Element, name string) int {
		for _, p := range e.Ports {
			if p.Name == name {
				return p.Node
			}
		}
		t.Fatalf("%s has no port %s", e.Name, name)
		return 0
	}
	// The drawn terminal polyline runs from QS's terminal 1 down to the busbar, which has a port where it lands.
	qs1 := port(qs, "1")
	var drawn *Connector
	for i, c := range d.Connectors {
		if c.Kind == KindBusWork && (c.From == qs1 || c.To == qs1) {
			drawn = &d.Connectors[i]
		}
	}
	if drawn == nil {
		t.Fatalf("no wire at QS terminal 1: %+v", d.Connectors)
	}
	other := drawn.To
	if other == qs1 {
		other = drawn.From
	}
	onBus := false
	for _, p := range bus.Ports {
		onBus = onBus || p.Node == other
	}
	if !onBus || nodeAt[other].X != nodeAt[qs1].X {
		t.Errorf("QS's wire should drop straight onto the busbar: %+v, busbar ports %+v", drawn.Points, bus.Ports)
	}
	// Line L1 leaves QB's terminal 2 straight up to the level of substation B's marker, with its name.
	var line *Connector
	for i, c := range d.Connectors {
		if c.Kind == KindOverheadLine {
			line = &d.Connectors[i]
		}
	}
	if line == nil || line.Name != "L1" || line.Electrical == nil || *line.Electrical.R != 1.5 {
		t.Fatalf("line L1 not drawn: %+v", d.Connectors)
	}
	start, end := line.Points[0], line.Points[len(line.Points)-1]
	if line.From != port(qb, "2") || end.X != start.X || end.Y >= start.Y {
		t.Errorf("line from %v to %v (from node %d, QB terminal 2 node %d)", start, end, line.From, port(qb, "2"))
	}
	labelled := false
	for _, l := range d.Labels {
		labelled = labelled || l.Text == "L1" && l.For == line.ID
	}
	if !labelled {
		t.Errorf("line L1 has no label: %+v", d.Labels)
	}
}

// TestImportCIMNodeStyleOverdrawnTerminal draws QB's line terminal as a polyline down over QS: it is left to the
// routing, so no wire runs over QS and nothing needs a wire jump.
func TestImportCIMNodeStyleOverdrawnTerminal(t *testing.T) {
	extra := `  <cim:DiagramObject rdf:ID="_dtq2"><cim:DiagramObject.Diagram rdf:resource="#_d"/><cim:DiagramObject.IdentifiedObject rdf:resource="#_tq2"/></cim:DiagramObject>
  <cim:DiagramObjectPoint rdf:ID="_dtq2_0"><cim:DiagramObjectPoint.DiagramObject rdf:resource="#_dtq2"/><cim:DiagramObjectPoint.sequenceNumber>1</cim:DiagramObjectPoint.sequenceNumber><cim:DiagramObjectPoint.xPosition>50</cim:DiagramObjectPoint.xPosition><cim:DiagramObjectPoint.yPosition>58</cim:DiagramObjectPoint.yPosition></cim:DiagramObjectPoint>
  <cim:DiagramObjectPoint rdf:ID="_dtq2_1"><cim:DiagramObjectPoint.DiagramObject rdf:resource="#_dtq2"/><cim:DiagramObjectPoint.sequenceNumber>2</cim:DiagramObjectPoint.sequenceNumber><cim:DiagramObjectPoint.xPosition>50</cim:DiagramObjectPoint.xPosition><cim:DiagramObjectPoint.yPosition>95</cim:DiagramObjectPoint.yPosition></cim:DiagramObjectPoint>
</rdf:RDF>`
	dl := strings.Replace(nodeStyleDL, "</rdf:RDF>", extra, 1)
	terminals := map[string][]Point{"41": {{X: 0, Y: -10}, {X: 0, Y: 10}}, "162": {{X: 0, Y: -10}, {X: 0, Y: 10}}, "14": {{X: -10}, {X: 10}}}
	classes := map[string]Class{"41": ClassBreaker, "162": ClassDisconnector, "24": ClassBusBarSection, "14": ClassNonIntersection}
	pages, err := ImportCIM([][]byte{[]byte(nodeStyleEQ), []byte(dl)}, CIMImportOptions{Terminals: terminals, Classes: classes, Grid: 10})
	if err != nil {
		t.Fatal(err)
	}
	d := pages[0].Diagram
	var qs Element
	for _, e := range d.Elements {
		if e.Class == ClassNonIntersection {
			t.Errorf("wire jump at (%g, %g)", e.X, e.Y)
		}
		if e.Name == "QS" {
			qs = e
		}
	}
	for _, c := range d.Connectors {
		for k := 1; k < len(c.Points); k++ {
			if segDist(Point{X: qs.X, Y: qs.Y}, c.Points[k-1], c.Points[k]) < 1 {
				t.Errorf("%s %d %v runs over QS at (%g, %g)", c.Kind, c.ID, c.Points, qs.X, qs.Y)
			}
		}
	}
}

// TestImportCIMNodeStyleShortChainedDrawing draws QS's busbar terminal as a polyline stopping short of the busbar and
// the rest as a short drawing of the busbar's node, end to end: unlike a junction bar it is kept, so that connection
// needs no routing.
func TestImportCIMNodeStyleShortChainedDrawing(t *testing.T) {
	dl := strings.Replace(nodeStyleDL, `<cim:DiagramObjectPoint.xPosition>50</cim:DiagramObjectPoint.xPosition><cim:DiagramObjectPoint.yPosition>100</cim:DiagramObjectPoint.yPosition></cim:DiagramObjectPoint>`,
		`<cim:DiagramObjectPoint.xPosition>50</cim:DiagramObjectPoint.xPosition><cim:DiagramObjectPoint.yPosition>97</cim:DiagramObjectPoint.yPosition></cim:DiagramObjectPoint>`, 1)
	if dl == nodeStyleDL {
		t.Fatal("QS's terminal polyline not found")
	}
	dl = strings.Replace(dl, "</rdf:RDF>", `  <cim:DiagramObject rdf:ID="_dshort"><cim:DiagramObject.Diagram rdf:resource="#_d"/><cim:DiagramObject.IdentifiedObject rdf:resource="#_cnb"/></cim:DiagramObject>
  <cim:DiagramObjectPoint rdf:ID="_dshort_0"><cim:DiagramObjectPoint.DiagramObject rdf:resource="#_dshort"/><cim:DiagramObjectPoint.sequenceNumber>1</cim:DiagramObjectPoint.sequenceNumber><cim:DiagramObjectPoint.xPosition>50</cim:DiagramObjectPoint.xPosition><cim:DiagramObjectPoint.yPosition>97</cim:DiagramObjectPoint.yPosition></cim:DiagramObjectPoint>
  <cim:DiagramObjectPoint rdf:ID="_dshort_1"><cim:DiagramObjectPoint.DiagramObject rdf:resource="#_dshort"/><cim:DiagramObjectPoint.sequenceNumber>2</cim:DiagramObjectPoint.sequenceNumber><cim:DiagramObjectPoint.xPosition>50</cim:DiagramObjectPoint.xPosition><cim:DiagramObjectPoint.yPosition>100</cim:DiagramObjectPoint.yPosition></cim:DiagramObjectPoint>
</rdf:RDF>`, 1)
	terminals := map[string][]Point{"41": {{X: 0, Y: -10}, {X: 0, Y: 10}}, "162": {{X: 0, Y: -10}, {X: 0, Y: 10}}, "14": {{X: -10}, {X: 10}}}
	classes := map[string]Class{"41": ClassBreaker, "162": ClassDisconnector, "24": ClassBusBarSection, "14": ClassNonIntersection}
	pages, err := ImportCIM([][]byte{[]byte(nodeStyleEQ), []byte(dl)}, CIMImportOptions{Terminals: terminals, Classes: classes, Grid: 10})
	if err != nil {
		t.Fatal(err)
	}
	// QB's connection to QS is drawn by no one either way; QS's to the busbar must not join it.
	routed := ""
	for _, m := range pages[0].Messages {
		if strings.Contains(m, "routed automatically") {
			routed = m
		}
	}
	if !strings.HasPrefix(routed, "1 connections") {
		t.Errorf("only QB's connection should be routed: %v", pages[0].Messages)
	}
}

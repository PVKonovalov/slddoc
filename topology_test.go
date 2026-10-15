package slddoc

import (
	"fmt"
	"slices"
	"testing"
)

func TestBuildTopology_ExactCoincidenceAndBusTap(t *testing.T) {
	d := &Diagram{
		Elements: []Element{
			{ID: 1, Class: ClassBusBarSection, Points: []Point{{800, 240}, {1300, 240}}},
			{ID: 2, Ports: []Port{{Name: "1"}, {Name: "2"}}},
		},
		Connectors: []Connector{
			{ID: 1, Points: []Point{{900, 240}, {900, 270}}}, // taps the bus mid-span
		},
	}
	bindings := []portBinding{
		{elemIdx: 1, portIdx: 0, p: Point{900, 270}}, // coincides with w1's other end
		{elemIdx: 1, portIdx: 1, p: Point{900, 400}}, // dangling
	}

	buildTopology(d, bindings)

	if d.Connectors[0].From == 0 || d.Connectors[0].To == 0 {
		t.Fatalf("connector endpoints not resolved: %+v", d.Connectors[0])
	}
	if d.Elements[1].Ports[0].Node != d.Connectors[0].To {
		t.Errorf("breaker port 1 (%d) should share a node with the wire's far end (%d)",
			d.Elements[1].Ports[0].Node, d.Connectors[0].To)
	}
	if d.Elements[1].Ports[1].Node == d.Elements[1].Ports[0].Node {
		t.Errorf("breaker port 2 should NOT share a node with port 1")
	}

	// The wire's near end (900,240) lands exactly on the bus segment
	// (800,240)-(1300,240), so it must resolve to the bus's own node.
	var busNodeID int
	for _, n := range d.Nodes {
		if n.X == 800 && n.Y == 240 {
			busNodeID = n.ID
		}
	}
	if busNodeID == 0 {
		t.Fatalf("no node found at the bus's own coordinate: %+v", d.Nodes)
	}
	if d.Connectors[0].From != busNodeID {
		t.Errorf("wire tapping the bus mid-span should resolve to the bus's node %d, got %d", busNodeID, d.Connectors[0].From)
	}
}

func TestBuildTopology_SnapsSmallGaps(t *testing.T) {
	d := &Diagram{
		Elements: []Element{
			{ID: 1, Ports: []Port{{Name: "1"}}},
		},
		Connectors: []Connector{
			{ID: 1, Points: []Point{{900, 550}, {900, 570}}},
		},
	}
	bindings := []portBinding{
		{elemIdx: 0, portIdx: 0, p: Point{900, 553}}, // 3 units from the wire's end, within snapTolerance
	}

	buildTopology(d, bindings)

	if d.Elements[0].Ports[0].Node != d.Connectors[0].From {
		t.Errorf("a port within snapTolerance of a wire end should merge with it: port node %d, wire from %d",
			d.Elements[0].Ports[0].Node, d.Connectors[0].From)
	}
}

func TestOnSegment(t *testing.T) {
	cases := []struct {
		p, a, b Point
		want    bool
	}{
		{Point{950, 240}, Point{800, 240}, Point{1300, 240}, true},
		{Point{950, 241}, Point{800, 240}, Point{1300, 240}, false},
		{Point{1400, 240}, Point{800, 240}, Point{1300, 240}, false},
		{Point{900, 300}, Point{900, 200}, Point{900, 400}, true},
	}
	for _, c := range cases {
		if got := onSegment(c.p, c.a, c.b); got != c.want {
			t.Errorf("onSegment(%v, %v, %v) = %v, want %v", c.p, c.a, c.b, got, c.want)
		}
	}
}

// TestTapPortsOntoWires_RealCorpus checks that a ground switch drawn as a
// T-tap on a wire, its terminal 2 units from the wire's line mid-span
// (PS_35kV_Molvotitsy id 2066 on wire 2332), is connected: the wire is split
// at the tap into two pieces that both end on the switch's port. It used to
// come out connected to nothing, a red cross in the editor.
func TestTapPortsOntoWires_RealCorpus(t *testing.T) {
	const svg = `<?xml version="1.0"?>
<svg width="3000" height="2000" xmlns="http://www.w3.org/2000/svg">
<polyline points="1500,650 1600,650" style="fill:none;stroke:#965000;stroke-width:1 "  data-type="21" id="2332" data-voltage="#965000" />
<g id="2066" data-voltage="#965000" data-type="54" data-name="ЗР-35 2сш 35кВ" transform="rotate(180,1530,660)" >
<path d="M 1530 672 v -9 m -4 0 h 8 m -8 -20 h 8 m -4 0 v -8 m -8 0 h 16 m -2 -3 h -12 m 2 -3 h 8" style="fill:none;stroke:#965000;stroke-width:1" />
<path d="M 1536 652 h -12" data-state="0" style="fill:none;stroke:#965000;stroke-width:1" />
</g>
</svg>`
	d, report, err := Extract([]byte(svg), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Failed) != 0 || len(d.Elements) != 1 {
		t.Fatalf("failed=%v elements=%d", report.Failed, len(d.Elements))
	}
	if len(d.Connectors) != 2 {
		t.Fatalf("wire not split at the tap: %+v", d.Connectors)
	}
	port := d.Elements[0].Ports[0].Node
	for _, c := range d.Connectors {
		if c.From != port && c.To != port {
			t.Errorf("piece %d (%v) doesn't end on the ground switch's port node %d", c.ID, c.Points, port)
		}
		if c.Kind != KindBusWork {
			t.Errorf("piece %d kind %s, want BusWork", c.ID, c.Kind)
		}
	}
	if d.Connectors[0].ID == d.Connectors[1].ID {
		t.Errorf("both pieces share id %d", d.Connectors[0].ID)
	}
}

// TestTapPortsOntoWires_LineEntrance checks the overhead-line rules: a
// grounding device near a line's end (its substation entrance) taps it, the
// piece from that end becoming BusWork while the line keeps its id and
// name; one in the middle of the line, and any other device, doesn't.
func TestTapPortsOntoWires_LineEntrance(t *testing.T) {
	gs := func(id, x int) string {
		// A ground switch hanging below the line, its terminal 2 above y=100.
		return fmt.Sprintf(`<g id="%d" data-voltage="#965000" data-type="54" data-name="ЗН-%d" transform="rotate(180,%d,110)" >
<path d="M %d 122 v -9 m -4 0 h 8 m -8 -20 h 8 m -4 0 v -8 m -8 0 h 16 m -2 -3 h -12 m 2 -3 h 8" style="fill:none;stroke:#965000;stroke-width:1" />
</g>`, id, id, x, x)
	}
	svg := `<?xml version="1.0"?>
<svg width="1000" height="400" xmlns="http://www.w3.org/2000/svg">
<polyline points="100,100 900,100" style="fill:none;stroke:#965000;;stroke-width:1" data-name="Л-1" data-voltage="#965000" data-type="22" id="500" />
` + gs(10, 130) + gs(11, 500) + `
</svg>`
	d, _, err := Extract([]byte(svg), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	var line, stub *Connector
	for i := range d.Connectors {
		switch {
		case d.Connectors[i].ID == 500:
			line = &d.Connectors[i]
		case d.Connectors[i].Kind == KindBusWork:
			stub = &d.Connectors[i]
		}
	}
	if line == nil || stub == nil || len(d.Connectors) != 2 {
		t.Fatalf("connectors = %+v", d.Connectors)
	}
	if line.Kind != KindOverheadLine || line.Name != "Л-1" || line.Points[0] != (Point{X: 130, Y: 100}) {
		t.Errorf("line = %+v, want the overhead line Л-1 now starting at the tap (130,100)", *line)
	}
	if stub.Points[0] != (Point{X: 100, Y: 100}) || stub.Points[len(stub.Points)-1] != (Point{X: 130, Y: 100}) {
		t.Errorf("stub = %+v, want BusWork from the line's end to the tap", *stub)
	}
	for _, el := range d.Elements {
		tapped := el.Ports[0].Node == line.From
		if el.ID == 10 && !tapped {
			t.Errorf("ground switch at the line entrance isn't on the tap node")
		}
		if el.ID == 11 && (el.Ports[0].Node == line.From || el.Ports[0].Node == line.To) {
			t.Errorf("ground switch mid-span was tapped onto the overhead line")
		}
	}
}

// TestTapWireEndsOntoWires_TJunction checks a buswork T-junction: wire 3953
// ends on the middle of wire 3961, which runs on unbroken. 3961 is split there and all three pieces share one node. Two
// wires merely crossing (500 and 501) stay separate.
func TestTapWireEndsOntoWires_TJunction(t *testing.T) {
	const svg = `<?xml version="1.0"?>
<svg width="3000" height="2000" xmlns="http://www.w3.org/2000/svg">
<polyline points="1560,575 1560,605" style="fill:none;stroke:#965000;stroke-width:1" data-type="21" id="3961" data-voltage="#965000" />
<polyline points="1590,595 1560,595" style="fill:none;stroke:#965000;stroke-width:1" data-type="21" id="3953" data-voltage="#965000" />
<polyline points="100,100 200,100" style="fill:none;stroke:#965000;stroke-width:1" data-type="21" id="500" data-voltage="#965000" />
<polyline points="150,50 150,150" style="fill:none;stroke:#965000;stroke-width:1" data-type="21" id="501" data-voltage="#965000" />
</svg>`
	d, _, err := Extract([]byte(svg), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Connectors) != 5 {
		t.Fatalf("want 3961 split in two (5 connectors), got %+v", d.Connectors)
	}
	byID := map[int]Connector{}
	for _, c := range d.Connectors {
		byID[c.ID] = c
	}
	tap := byID[3953].To
	var pieces []Connector
	for _, c := range d.Connectors {
		if c.ID != 3953 && (c.From == tap || c.To == tap) {
			pieces = append(pieces, c)
		}
	}
	if len(pieces) != 2 || pieces[0].ID == pieces[1].ID {
		t.Fatalf("pieces on the junction node %d: %+v", tap, pieces)
	}
	for _, c := range pieces {
		if c.Kind != KindBusWork || c.Voltage != byID[3953].Voltage {
			t.Errorf("piece %+v lost the wire's kind or voltage", c)
		}
	}
	if a, b := byID[500], byID[501]; a.From == b.From || a.From == b.To || a.To == b.From || a.To == b.To {
		t.Errorf("crossing wires were joined: %+v %+v", a, b)
	}
}

// TestTapWiresThroughJunctions checks a junction point (1959) whose dot sits
// where wire 1997 crosses busbar 2018: the wire is split at the
// dot and both pieces join the busbar. An overhead line through a dot is split
// too; wire 1600 crossing the busbar with no dot stays unconnected.
func TestTapWiresThroughJunctions(t *testing.T) {
	const svg = `<?xml version="1.0"?>
<svg width="3000" height="2000" xmlns="http://www.w3.org/2000/svg">
<polyline points="1560,365 1560,445" style="fill:none;stroke:#00A0F0;stroke-width:1 "  data-type="21" id="1997" data-voltage="#00A0F0" />
<polyline points="1440,435 1720,435" style="fill:none;stroke:#00A0F0;stroke-width:4" data-voltage="#00A0F0" data-name="1SEC 110kV" data-type="24" id="2018" />
<polyline points="1600,400 1600,470" style="fill:none;stroke:#00A0F0;stroke-width:1 "  data-type="21" id="1600" data-voltage="#00A0F0" />
<polyline points="100,100 900,100" style="fill:none;stroke:#965000;stroke-width:1" data-name="Л-1" data-voltage="#965000" data-type="22" id="500" />
<circle cx="1560" cy="435" r="3" style="fill:#12161d;stroke:#00A0F0;stroke-width:1" data-type="7" id="1959" data-voltage="#12161d" />
<circle cx="500" cy="100" r="3" style="fill:#12161d;stroke:#965000;stroke-width:1" data-type="7" id="1960" data-voltage="#12161d" />
</svg>`
	d, report, err := Extract([]byte(svg), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Failed) != 0 {
		t.Fatalf("failed: %v", report.Failed)
	}
	junction := map[int]int{}
	for _, e := range d.Elements {
		if e.Class == ClassJunctionPoint {
			junction[e.ID] = e.Ports[0].Node
		}
	}
	onNode := func(node int) (ids []int, kinds []ConnectorKind) {
		for _, c := range d.Connectors {
			if c.From == node || c.To == node {
				ids, kinds = append(ids, c.ID), append(kinds, c.Kind)
			}
		}
		return
	}
	if ids, _ := onNode(junction[1959]); len(ids) != 2 || !slices.Contains(ids, 1997) {
		t.Errorf("wires on the busbar junction: %v, want 1997's two pieces", ids)
	}
	if ids, kinds := onNode(junction[1960]); len(ids) != 2 || kinds[0] != KindOverheadLine || kinds[1] != KindOverheadLine {
		t.Errorf("wires on the line junction: %v %v, want the line's two pieces", ids, kinds)
	}
	for _, c := range d.Connectors {
		if c.ID == 1600 && (c.From == junction[1959] || c.To == junction[1959]) {
			t.Errorf("wire 1600 crossing the busbar without a dot was joined to it")
		}
	}
}

// TestTapWireEndsOntoWires_PortAndEnd checks a ground switch (3421) whose port
// and the end of wire 3429 meet on the middle of wire 964, which runs on
// unbroken. Neither is a lone point, which the
// tapping rules used to require; now the wire is split there and all of them
// share one node.
func TestTapWireEndsOntoWires_PortAndEnd(t *testing.T) {
	const svg = `<?xml version="1.0"?>
<svg width="3000" height="2000" xmlns="http://www.w3.org/2000/svg">
<polyline points="1530,600 1530,700" style="fill:none;stroke:#965000;stroke-width:1" data-type="21" id="964" data-voltage="#965000" />
<polyline points="1480,600 1480,650 1530,650" style="fill:none;stroke:#965000;stroke-width:1" data-type="21" id="3429" data-voltage="#965000" />
<g id="3421" data-voltage="#965000" data-type="54" data-name="GS-110" transform="rotate(180,1530,660)" >
<path d="M 1530 672 v -9 m -4 0 h 8 m -8 -20 h 8 m -4 0 v -8 m -8 0 h 16 m -2 -3 h -12 m 2 -3 h 8" style="fill:none;stroke:#965000;stroke-width:1" />
<path d="M 1536 652 h -12" data-state="0" style="fill:none;stroke:#965000;stroke-width:1" />
</g>
</svg>`
	d, report, err := Extract([]byte(svg), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Failed) != 0 || len(d.Elements) != 1 {
		t.Fatalf("failed=%v elements=%d", report.Failed, len(d.Elements))
	}
	port := d.Elements[0].Ports[0].Node
	var on []int
	for _, c := range d.Connectors {
		if c.From == port || c.To == port {
			on = append(on, c.ID)
		}
	}
	slices.Sort(on)
	if len(d.Connectors) != 3 || len(on) != 3 || !slices.Contains(on, 964) || !slices.Contains(on, 3429) {
		t.Errorf("wires on the ground switch's node %d: %v, want 3429 and both pieces of 964 (%+v)", port, on, d.Connectors)
	}
}

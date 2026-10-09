package slddoc

import (
	"fmt"
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

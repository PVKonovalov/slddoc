package slddoc

import "testing"

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

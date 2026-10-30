package repair

import (
	"testing"

	"github.com/PVKonovalov/slddoc/internal/model"
)

var repairTerminals = map[string][]model.Point{
	"41": {{X: 0, Y: -10}, {X: 0, Y: 10}},
	"54": {{X: 0, Y: -20}, {X: 0, Y: 20}},
	"7":  {{X: 0, Y: 0}},
}

func TestRepair_FitsPortsAndJoinsWireEnd(t *testing.T) {
	d := &model.Diagram{
		LastID: 10,
		Nodes:  []model.Node{{ID: 2, X: 50, Y: 0}, {ID: 3, X: 50, Y: 40}},
		Elements: []model.Element{
			{ID: 1, Class: model.ClassBreaker, Shape: "41", X: 50, Y: 50},
		},
		Connectors: []model.Connector{{ID: 4, Kind: model.KindBusWork, From: 2, To: 3, Points: []model.Point{{X: 50, Y: 0}, {X: 50, Y: 40}}}},
	}
	if !Repair(d, repairTerminals) {
		t.Fatal("Repair reported no change")
	}
	ports := d.Elements[0].Ports
	if len(ports) != 2 || ports[0].Name != "1" || ports[1].Name != "2" {
		t.Fatalf("ports = %+v, want two fixed ports", ports)
	}
	if d.Connectors[0].To != ports[0].Node {
		t.Errorf("wire end on the upper terminal should join port 1: wire to %d, port node %d", d.Connectors[0].To, ports[0].Node)
	}
	if len(d.Nodes) != 3 {
		t.Errorf("nodes = %+v, want the wire's start plus one node per terminal", d.Nodes)
	}
	if Repair(d, repairTerminals) {
		t.Error("a second Repair should change nothing")
	}
}

func TestRepair_SplitsWireThroughJunction(t *testing.T) {
	d := &model.Diagram{
		LastID: 10,
		Nodes:  []model.Node{{ID: 2, X: 0, Y: 50}, {ID: 3, X: 100, Y: 50}, {ID: 5, X: 50, Y: 50}},
		Elements: []model.Element{
			{ID: 1, Class: model.ClassJunctionPoint, Shape: "7", X: 50, Y: 50, Ports: []model.Port{{Name: "1", Node: 5}}},
		},
		Connectors: []model.Connector{{ID: 4, Kind: model.KindBusWork, From: 2, To: 3, Points: []model.Point{{X: 0, Y: 50}, {X: 100, Y: 50}}}},
	}
	Repair(d, repairTerminals)
	if len(d.Connectors) != 2 {
		t.Fatalf("connectors = %+v, want the wire split in two", d.Connectors)
	}
	a, b := d.Connectors[0], d.Connectors[1]
	if a.ID != 4 || a.To != 5 || b.From != 5 || b.To != 3 || b.ID != 11 || d.LastID != 11 {
		t.Errorf("split = %+v / %+v (lastId %d), want 4: 2→5 and 11: 5→3", a, b, d.LastID)
	}
}

func TestRepair_RemovesDegenerateWire(t *testing.T) {
	d := &model.Diagram{
		LastID: 10,
		Nodes:  []model.Node{{ID: 2, X: 50, Y: 40}, {ID: 3, X: 50, Y: 40}, {ID: 6, X: 50, Y: 60}},
		Elements: []model.Element{
			{ID: 1, Class: model.ClassBreaker, Shape: "41", X: 50, Y: 50, Ports: []model.Port{{Name: "1", Node: 2}, {Name: "2", Node: 6}}},
		},
		Connectors: []model.Connector{{ID: 4, Kind: model.KindBusWork, From: 3, To: 2, Points: []model.Point{{X: 50, Y: 40}, {X: 50, Y: 40}}}},
	}
	Repair(d, repairTerminals)
	if len(d.Connectors) != 0 || len(d.Nodes) != 2 || d.Elements[0].Ports[0].Node != 2 {
		t.Errorf("connectors = %+v, nodes = %+v, ports = %+v: the zero-length wire should go, keeping the port's node",
			d.Connectors, d.Nodes, d.Elements[0].Ports)
	}
}

func TestEnsureIDs(t *testing.T) {
	d := &model.Diagram{
		Nodes:          []model.Node{{ID: 1}},
		VoltageClasses: []model.VoltageClass{{ID: 1, Color: "#fff"}},
		Elements:       []model.Element{{ID: 1, Voltage: 1, Ports: []model.Port{{Name: "1", Node: 1}}}},
		Labels:         []model.Label{{ID: 0}},
	}
	if !EnsureIDs(d) {
		t.Fatal("EnsureIDs reported no change")
	}
	if d.Labels[0].ID != 2 {
		t.Errorf("label id = %d, want 2", d.Labels[0].ID)
	}
	if d.Nodes[0].ID != 3 || d.Elements[0].Ports[0].Node != 3 {
		t.Errorf("colliding node should be renumbered with its port: node %d, port %d", d.Nodes[0].ID, d.Elements[0].Ports[0].Node)
	}
	if d.VoltageClasses[0].ID != 4 || d.Elements[0].Voltage != 4 || d.LastID != 4 {
		t.Errorf("colliding class should be renumbered with its use: class %d, voltage %d, lastId %d",
			d.VoltageClasses[0].ID, d.Elements[0].Voltage, d.LastID)
	}
	if EnsureIDs(d) {
		t.Error("a second EnsureIDs should change nothing")
	}
}

func TestInferSizeSteps(t *testing.T) {
	d := &model.Diagram{
		Nodes: []model.Node{{ID: 2, X: 50, Y: 10}, {ID: 3, X: 50, Y: 90}},
		Elements: []model.Element{
			{ID: 1, Class: model.ClassGroundSwitch, Shape: "54", X: 50, Y: 50, Ports: []model.Port{{Name: "1", Node: 2}, {Name: "2", Node: 3}}},
		},
	}
	if !InferSizeSteps(d, repairTerminals) || d.Elements[0].Scale != 2 {
		t.Errorf("scale = %d, want 2 (terminals ±20 drawn at ±40)", d.Elements[0].Scale)
	}
}

func TestMostUsedVoltage(t *testing.T) {
	d := &model.Diagram{
		Elements:   []model.Element{{Voltage: 3}, {Voltage: 2}},
		Connectors: []model.Connector{{Voltage: 2}, {Voltage: 3}},
	}
	if v := MostUsedVoltage(d); v != 2 {
		t.Errorf("MostUsedVoltage = %d, want 2 (a tie goes to the lower id)", v)
	}
}

func TestRepair_SplitsSharedBusbarNode(t *testing.T) {
	// As Extract builds it: every wire end along the busbar on one Node at its first point.
	d := &model.Diagram{
		LastID: 10,
		Nodes:  []model.Node{{ID: 2, X: 0, Y: 0}, {ID: 3, X: 30, Y: -40}, {ID: 4, X: 70, Y: 40}, {ID: 5, X: 0, Y: -40}},
		Elements: []model.Element{
			{ID: 1, Class: model.ClassBusBarSection, Shape: "24", Points: []model.Point{{X: 0, Y: 0}, {X: 100, Y: 0}}},
		},
		Connectors: []model.Connector{
			{ID: 6, Kind: model.KindBusWork, From: 3, To: 2, Points: []model.Point{{X: 30, Y: -40}, {X: 30, Y: 0}}},
			{ID: 7, Kind: model.KindBusWork, From: 2, To: 4, Points: []model.Point{{X: 70, Y: 0}, {X: 70, Y: 40}}},
			{ID: 8, Kind: model.KindBusWork, From: 5, To: 2, Points: []model.Point{{X: 0, Y: -40}, {X: 0, Y: 0}}},
		},
	}
	if !Repair(d, repairTerminals) {
		t.Fatal("Repair reported no change")
	}
	at := map[int]model.Point{}
	for _, n := range d.Nodes {
		at[n.ID] = model.Point{X: n.X, Y: n.Y}
	}
	ends := []struct {
		node int
		want model.Point
	}{{d.Connectors[0].To, model.Point{X: 30}}, {d.Connectors[1].From, model.Point{X: 70}}, {d.Connectors[2].To, model.Point{}}}
	for i, e := range ends {
		if at[e.node] != e.want {
			t.Errorf("wire %d ends on node %d at %v, want a node at %v", i, e.node, at[e.node], e.want)
		}
	}
	if ports := d.Elements[0].Ports; len(ports) != 3 {
		t.Errorf("busbar ports = %+v, want one per connection point", ports)
	}
	if Repair(d, repairTerminals) {
		t.Error("a second Repair should change nothing")
	}
}

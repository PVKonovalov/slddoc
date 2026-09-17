package slddoc

import (
	"bytes"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	state := 1
	d := &Diagram{
		Width: 100, Height: 200, Source: "example.xml",
		LastID:         5,
		Editor:         &EditorSettings{GridSpacing: 20, Snap: true, Background: "#12161d"},
		Layers:         []Layer{{ID: BaseLayer, Name: "Base"}, {ID: 10, Name: "Groups"}},
		VoltageClasses: []VoltageClass{{ID: 1, Name: "10kV", Color: "#962896"}},
		Nodes:          []Node{{ID: 2, X: 900, Y: 240}},
		Elements: []Element{{
			ID: 3, Class: ClassBreaker, Shape: "41", Name: "CB-1",
			Voltage: 1, Layer: BaseLayer, X: 900, Y: 660, State: &state,
			Ports: []Port{{Name: "1", Node: 2}},
		}},
		Connectors: []Connector{{
			ID: 4, Kind: KindBusbarWire, Voltage: 1, Layer: BaseLayer,
			From: 2, To: 2, Points: []Point{{X: 900, Y: 240}, {X: 900, Y: 270}},
		}},
		Labels: []Label{{For: 3, Layer: BaseLayer, X: 1, Y: 2, Size: 13, Text: "CB-1"}},
	}

	var buf bytes.Buffer
	if err := d.Save(&buf); err != nil {
		t.Fatal(err)
	}

	got, err := Load(&buf)
	if err != nil {
		t.Fatalf("Load: %v\nXML was:\n%s", err, buf.String())
	}

	if got.Width != d.Width || got.Source != d.Source || got.LastID != d.LastID {
		t.Errorf("diagram header mismatch: %+v", got)
	}
	if got.Editor == nil || got.Editor.GridSpacing != 20 || !got.Editor.Snap || got.Editor.Background != "#12161d" {
		t.Errorf("editor settings mismatch: %+v", got.Editor)
	}
	if len(got.Layers) != 2 || got.Layers[1].ID != 10 {
		t.Errorf("layers mismatch: %+v", got.Layers)
	}
	if len(got.VoltageClasses) != 1 || got.VoltageClasses[0].Color != "#962896" {
		t.Errorf("voltage classes mismatch: %+v", got.VoltageClasses)
	}
	if len(got.Elements) != 1 || got.Elements[0].State == nil || *got.Elements[0].State != 1 {
		t.Errorf("elements mismatch: %+v", got.Elements)
	}
	if len(got.Elements[0].Ports) != 1 || got.Elements[0].Ports[0].Node != 2 {
		t.Errorf("ports mismatch: %+v", got.Elements[0].Ports)
	}
	if len(got.Connectors) != 1 || len(got.Connectors[0].Points) != 2 {
		t.Errorf("connectors mismatch: %+v", got.Connectors)
	}
	if len(got.Labels) != 1 || got.Labels[0].For != 3 {
		t.Errorf("labels mismatch: %+v", got.Labels)
	}
}

func TestSaveLoadRoundTrip_NoEditorSettings(t *testing.T) {
	d := &Diagram{Width: 10, Height: 10}

	var buf bytes.Buffer
	if err := d.Save(&buf); err != nil {
		t.Fatal(err)
	}

	got, err := Load(&buf)
	if err != nil {
		t.Fatalf("Load: %v\nXML was:\n%s", err, buf.String())
	}
	if got.Editor != nil {
		t.Errorf("Editor = %+v, want nil for a diagram saved without one", got.Editor)
	}
}

package slddoc

import (
	"bytes"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	state := 1
	d := &Diagram{
		Width: 100, Height: 200, Source: "example.svg",
		Layers:         []Layer{{ID: BaseLayer, Name: "Base"}, {ID: 10, Name: "Контейнеры"}},
		VoltageClasses: []VoltageClass{{ID: 1, Name: "10кВ", Color: "#962896"}},
		Nodes:          []Node{{ID: 1, X: 900, Y: 240}},
		Elements: []Element{{
			ID: 1, Class: ClassBreaker, Shape: "41", Name: "В-10 Л-22",
			Voltage: 1, Layer: BaseLayer, X: 900, Y: 660, State: &state,
			Ports: []Port{{Name: "1", Node: 1}},
		}},
		Connectors: []Connector{{
			ID: 1, Kind: KindBusbarWire, Voltage: 1, Layer: BaseLayer,
			From: 1, To: 1, Points: []Point{{900, 240}, {900, 270}},
		}},
		Labels: []Label{{For: 1, Layer: BaseLayer, X: 1, Y: 2, Size: 13, Text: "В-10 Л-22"}},
	}

	var buf bytes.Buffer
	if err := d.Save(&buf); err != nil {
		t.Fatal(err)
	}

	got, err := Load(&buf)
	if err != nil {
		t.Fatalf("Load: %v\nXML was:\n%s", err, buf.String())
	}

	if got.Width != d.Width || got.Source != d.Source {
		t.Errorf("diagram header mismatch: %+v", got)
	}
	if len(got.Layers) != 2 || got.Layers[1].ID != 10 {
		t.Errorf("layers mismatch: %+v", got.Layers)
	}
	if len(got.Elements) != 1 || got.Elements[0].ID != 1 || got.Elements[0].State == nil || *got.Elements[0].State != 1 {
		t.Errorf("elements mismatch: %+v", got.Elements)
	}
	if len(got.Elements[0].Ports) != 1 || got.Elements[0].Ports[0].Node != 1 {
		t.Errorf("ports mismatch: %+v", got.Elements[0].Ports)
	}
	if len(got.Connectors) != 1 || len(got.Connectors[0].Points) != 2 {
		t.Errorf("connectors mismatch: %+v", got.Connectors)
	}
	if len(got.Labels) != 1 || got.Labels[0].Text != "В-10 Л-22" {
		t.Errorf("labels mismatch: %+v", got.Labels)
	}
}

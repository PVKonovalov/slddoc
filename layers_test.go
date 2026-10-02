package slddoc

import (
	"bytes"
	"strings"
	"testing"
)

// layeredDiagram has a rectangle and a wire on the base layer, and a label
// and a rectangle on layer 20 ("Disconnectors"), which is drawn first when its Z is
// lower than the base layer's.
func layeredDiagram(baseZ, layerZ int) *Diagram {
	return &Diagram{
		Width: 200, Height: 200,
		Layers: []Layer{{ID: BaseLayer, Name: "Base", Z: baseZ}, {ID: 20, Name: "Disconnectors", Z: layerZ}},
		Elements: []Element{
			{ID: 1, Class: ClassRectangle, Shape: "3", Layer: BaseLayer, Points: []Point{{40, 40}, {60, 60}}},
			{ID: 2, Class: ClassRectangle, Shape: "3", Layer: 20, Points: []Point{{0, 0}, {100, 100}}},
		},
		Connectors: []Connector{{ID: 3, Kind: KindBusWork, Layer: BaseLayer, Points: []Point{{0, 0}, {10, 0}}}},
		Labels:     []Label{{ID: 4, Layer: 20, X: 5, Y: 5, Size: 10, Text: "note"}},
	}
}

func TestRender_LayerZOrderAndAttrs(t *testing.T) {
	lib := NewSymbolLibrary(nil)
	render := func(d *Diagram) string {
		var buf bytes.Buffer
		if err := Render(d, lib, &buf, Static, "", nil); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}
	pos := func(out, needle string) int {
		i := strings.Index(out, needle)
		if i < 0 {
			t.Fatalf("missing %q in\n%s", needle, out)
		}
		return i
	}

	// Equal Z: the usual order, elements of both layers first, label last.
	out := render(layeredDiagram(0, 0))
	if !(pos(out, `id="1"`) < pos(out, `id="2"`) && pos(out, `id="2"`) < pos(out, `id="3"`) && pos(out, `id="3"`) < pos(out, `id="4"`)) {
		t.Errorf("equal Z order wrong:\n%s", out)
	}
	if !strings.Contains(out, `<rect data-layer="20" id="2"`) || !strings.Contains(out, `<g data-layer="20" data-type="5"`) {
		t.Errorf("data-layer missing:\n%s", out)
	}
	if strings.Contains(out, `data-layer="0"`) {
		t.Errorf("base layer items must carry no data-layer:\n%s", out)
	}
	if !strings.Contains(out, `<metadata>`+"\n"+`{"layers":[{"id":"20","label":"Disconnectors"}]}`+"\n"+`</metadata>`) {
		t.Errorf("metadata missing:\n%s", out)
	}

	// Layer 20 above the base layer: its rectangle and label come after
	// the base layer's wire.
	out = render(layeredDiagram(0, 5))
	if !(pos(out, `id="3"`) < pos(out, `id="2"`) && pos(out, `id="2"`) < pos(out, `id="4"`)) {
		t.Errorf("layer 20 should draw last:\n%s", out)
	}
	// Base layer above layer 20: layer 20's items come first.
	out = render(layeredDiagram(7, 0))
	if !(pos(out, `id="4"`) < pos(out, `id="1"`)) {
		t.Errorf("layer 20 should draw first:\n%s", out)
	}
	if !strings.Contains(out, `"base":{"label":"Base","z":7}`) {
		t.Errorf("base z missing from metadata:\n%s", out)
	}

	// Round trip: layers, their Z and every item's layer come back.
	back, report, err := Extract([]byte(out), "", nil)
	if err != nil || len(report.Failed) != 0 {
		t.Fatalf("extract: %v %+v", err, report)
	}
	if len(back.Layers) != 2 || back.Layers[0].Z != 7 || back.Layers[1] != (Layer{ID: 20, Name: "Disconnectors"}) {
		t.Errorf("layers = %+v", back.Layers)
	}
	for _, e := range back.Elements {
		want := map[int]int{1: BaseLayer, 2: 20}[e.ID]
		if e.Layer != want {
			t.Errorf("element %d layer %d, want %d", e.ID, e.Layer, want)
		}
	}
	if len(back.Labels) != 1 || back.Labels[0].Layer != 20 {
		t.Errorf("labels = %+v", back.Labels)
	}
}

// TestRender_NoLayerMetadataByDefault: a diagram with only a default base
// layer writes no <metadata>.
func TestRender_NoLayerMetadataByDefault(t *testing.T) {
	d := &Diagram{Width: 10, Height: 10, Layers: []Layer{{ID: BaseLayer, Name: "Base"}}}
	var buf bytes.Buffer
	if err := Render(d, NewSymbolLibrary(nil), &buf, Static, "", nil); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "<metadata>") {
		t.Errorf("unexpected metadata:\n%s", buf.String())
	}
}

// TestExtract_AddsUnlistedLayers: real corpus files tag items with layer
// ids their <metadata> doesn't list; each gets a layer of its own.
func TestExtract_AddsUnlistedLayers(t *testing.T) {
	const svg = `<svg xmlns="http://www.w3.org/2000/svg">
<metadata>
{"layers":[{"id":"20","label":"Disconnectors"}]}
</metadata>
<rect data-layer="10" id="5" x="0" y="0" width="10" height="10" data-type="3" />
<rect data-layer="20" id="6" x="0" y="0" width="10" height="10" data-type="3" />
</svg>`
	d, _, err := Extract([]byte(svg), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []Layer{{ID: BaseLayer, Name: "Base"}, {ID: 20, Name: "Disconnectors"}, {ID: 10, Name: "Layer 10"}}
	if len(d.Layers) != len(want) {
		t.Fatalf("layers = %+v", d.Layers)
	}
	for i := range want {
		if d.Layers[i] != want[i] {
			t.Errorf("layer %d = %+v, want %+v", i, d.Layers[i], want[i])
		}
	}
}

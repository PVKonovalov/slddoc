package extract

import (
	"bytes"
	"testing"

	"github.com/PVKonovalov/slddoc/internal/model"
	"github.com/PVKonovalov/slddoc/internal/render"
)

// TestExtractArrowDoubleHeaded reads Arrows (2) as xsde2svg draws them: fDouble="1" ones (horizontal and rotated)
// import as DoubleHeaded, single-headed ones don't.
func TestExtractArrowDoubleHeaded(t *testing.T) {
	const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="400" height="400">
<path d="M 80 130  l 7 -3 m 0 6 l -7 -3 l 30 0 l -7 -3 m 0 6 l 7 -3" style="fill:none;stroke:dimgray;stroke-width:1 "  id="1" data-type="2"  />
<path d="M 80 200 l 30 0 l -7 -3 m 0 6 l 7 -3" style="fill:none;stroke:dimgray;stroke-width:1 "  id="2" data-type="2"  />
<path d="M 100 100  l 7 -3 m 0 6 l -7 -3 h 42.43 l -7 -3 m 0 6 l 7 -3" style="fill:none;stroke:dimgray;stroke-width:1 " transform="rotate(45,100,100)" id="3" data-type="2" />
<path d="M 300 100 h 42.43 l -7 -3 m 0 6 l 7 -3" style="fill:none;stroke:dimgray;stroke-width:1 " transform="rotate(45,300,100)" id="4" data-type="2" />
<path d="M 300 300 l 0 8 l -3 -7 m 6 0 l -3 7" style="fill:none;stroke:dimgray;stroke-width:1 " id="5" data-type="2" />
</svg>`
	d, report, err := Extract([]byte(svg), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Failed) != 0 || len(d.Elements) != 5 {
		t.Fatalf("report = %+v, elements = %+v", report, d.Elements)
	}
	for i, want := range []bool{true, false, true, false, false} {
		el := d.Elements[i]
		if el.Class != model.ClassArrow || el.DoubleHeaded != want {
			t.Errorf("element %d = %+v, want DoubleHeaded %v", i, el, want)
		}
	}
	if p := d.Elements[0].Points; len(p) != 2 || p[0] != (model.Point{X: 80, Y: 130}) || p[1] != (model.Point{X: 110, Y: 130}) {
		t.Errorf("double-headed arrow points = %+v", p)
	}
}

// TestRender_ArrowDoubleHeadedRoundTrip checks that a saved double-headed Arrow imports back double-headed.
func TestRender_ArrowDoubleHeadedRoundTrip(t *testing.T) {
	lib := render.NewSymbolLibrary(map[string]string{})
	d := &model.Diagram{
		Width: 400, Height: 400,
		Layers: []model.Layer{{ID: model.BaseLayer, Name: "Base"}},
		Elements: []model.Element{
			{ID: 1, Class: model.ClassArrow, Shape: "2", X: 115, Y: 100, Points: []model.Point{{X: 100, Y: 100}, {X: 130, Y: 100}}, DoubleHeaded: true},
			{ID: 2, Class: model.ClassArrow, Shape: "2", X: 115, Y: 200, Points: []model.Point{{X: 100, Y: 200}, {X: 130, Y: 200}}},
		},
	}
	var buf bytes.Buffer
	if err := render.Render(d, lib, &buf, render.Static, "", nil); err != nil {
		t.Fatal(err)
	}
	back, report, err := Extract(buf.Bytes(), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Failed) != 0 || len(back.Elements) != 2 {
		t.Fatalf("report = %+v, elements = %+v", report, back.Elements)
	}
	if !back.Elements[0].DoubleHeaded || back.Elements[1].DoubleHeaded {
		t.Errorf("DoubleHeaded = %v, %v, want true, false", back.Elements[0].DoubleHeaded, back.Elements[1].DoubleHeaded)
	}
}

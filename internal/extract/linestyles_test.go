package extract

import (
	"bytes"
	"strings"
	"testing"

	"github.com/PVKonovalov/slddoc/internal/model"
	"github.com/PVKonovalov/slddoc/internal/render"
)

// TestExtract_LineStyles reads a Line's composite styles from xsde2svg's data-styles, and from its halo colors in
// markup written before data-styles existed.
func TestExtract_LineStyles(t *testing.T) {
	const svg = `<svg width="600" height="200" style='background-color: #f5ebeb;' xmlns="http://www.w3.org/2000/svg">
<polyline points="520,50 50,50" style="fill:none;stroke:#ec5d56;stroke-width:7 "  id="35" />
<polyline points="520,50 50,50" style="fill:none;stroke:#0009f5;stroke-width:7 "  id="35" />
<polyline points="520,50 50,50" style="fill:none;stroke:#f5ebeb;stroke-width:3 "  id="35" />
<polyline points="520,50 50,50" style="fill:none;stroke:black;stroke-width:1 "  data-type="1" id="35" data-voltage="black" />
<polyline points="520,100 50,100" style="fill:none;stroke:black;stroke-width:1 "  data-type="1" id="12" data-voltage="black" data-styles="Авария,КонтрольСнят" />
<polyline points="520,150 50,150" style="fill:none;stroke:black;stroke-width:1 "  data-type="1" id="3" data-voltage="black" />
</svg>`
	d, report, err := Extract([]byte(svg), "test", nil)
	if err != nil || len(report.Failed) != 0 || len(d.Elements) != 3 {
		t.Fatalf("err=%v report=%+v elements=%+v", err, report, d.Elements)
	}
	want := map[int]string{35: "not_linked,Перегрузка", 12: "Авария,КонтрольСнят", 3: ""}
	for _, el := range d.Elements {
		if el.Styles != want[el.ID] {
			t.Errorf("line %d: Styles = %q, want %q", el.ID, el.Styles, want[el.ID])
		}
	}
}

// TestRender_LineStyles: Static draws the library's halos (in their HaloColors override when set) before the line, as
// xsde2svg does, with the id only on the line, and reads back; Interactive is one <g> holding the id. A style without
// a halo in the library is only kept.
func TestRender_LineStyles(t *testing.T) {
	halos := model.LineHalos{"Перегрузка": {Color: "#0009f5", Gap: true}, "BadQuality": {Color: "#f7d047"}, "В_ремонте": {Color: "#808080"}}
	line := model.Element{ID: 5, Class: model.ClassLine, Shape: "1", Layer: 3, Stroke: "black", Styles: "Перегрузка,BadQuality,InSmena",
		HaloColors: "Перегрузка:#ff00ff,linked:#123456",
		Points:     []model.Point{{X: 10, Y: 20}, {X: 110, Y: 20}}}
	plain := model.Element{ID: 6, Class: model.ClassLine, Shape: "1", Points: []model.Point{{X: 10, Y: 40}, {X: 110, Y: 40}}}
	d := &model.Diagram{Width: 200, Height: 100, Editor: &model.EditorSettings{Background: "#f5ebeb"}, Elements: []model.Element{line, plain}}
	lib := render.NewSymbolLibrary(nil)
	lib.SetLineHalos(halos)

	var buf bytes.Buffer
	if err := render.Render(d, lib, &buf, render.Static, "", nil); err != nil {
		t.Fatal(err)
	}
	static := buf.String()
	halo := strings.Index(static, `<polyline data-layer="3" points="10,20 110,20" style="fill:none;stroke:#ff00ff;stroke-width:7" data-halo="Перегрузка" />`)
	gap := strings.Index(static, `<polyline data-layer="3" points="10,20 110,20" style="fill:none;stroke:#f5ebeb;stroke-width:3" />`)
	bad := strings.Index(static, `<polyline data-layer="3" points="10,20 110,20" style="fill:none;stroke:#f7d047;stroke-width:7" data-halo="BadQuality" />`)
	main := strings.Index(static, `data-type="1" id="5" data-styles="Перегрузка,BadQuality,InSmena" />`)
	if halo < 0 || gap < halo || bad < gap || main < bad {
		t.Errorf("Static: halo %d, gap %d, second halo %d, line %d:\n%s", halo, gap, bad, main, static)
	}
	if strings.Count(static, `id="5"`) != 1 || strings.Count(static, "data-halo") != 2 {
		t.Errorf("Static: id repeated or wrong halos:\n%s", static)
	}
	back, report, err := Extract([]byte(static), "test", nil)
	if err != nil || len(report.Failed) != 0 || len(back.Elements) != 2 {
		t.Fatalf("round trip: err=%v report=%+v elements=%+v", err, report, back.Elements)
	}
	got := back.Elements[0]
	model.DropDefaultHaloColors(&got, halos)
	if got.ID != 5 || got.Styles != line.Styles || got.HaloColors != "Перегрузка:#ff00ff" || got.Layer != 3 {
		t.Errorf("round trip = %+v", got)
	}

	frags, err := render.RenderFragments(d, lib, []int{5, 6}, render.Interactive, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	f := frags[5]
	if !strings.HasPrefix(f, `<g data-layer="3" id="5" data-styles="Перегрузка,BadQuality,InSmena" data-editor-kind="element" data-type="1">`) ||
		strings.Count(f, `id="5"`) != 1 || !strings.HasSuffix(strings.TrimSpace(f), "</g>") {
		t.Errorf("Interactive fragment:\n%s", f)
	}
	if !strings.HasPrefix(frags[6], "<polyline ") {
		t.Errorf("Interactive plain line:\n%s", frags[6])
	}

	buf.Reset()
	if err := render.Render(d, render.NewSymbolLibrary(nil), &buf, render.Static, "", nil); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "data-halo") {
		t.Errorf("no halos configured, yet drawn:\n%s", buf.String())
	}
}

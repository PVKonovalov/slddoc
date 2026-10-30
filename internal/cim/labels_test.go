package cim

import (
	"strings"
	"testing"

	"github.com/PVKonovalov/slddoc/internal/extract"
	"github.com/PVKonovalov/slddoc/internal/model"
	"github.com/PVKonovalov/slddoc/internal/render"
)

// TestExtract_RotatedLabel reads captions xsde2svg turns about their Modus origin, 13 above the text (rotate(a,x,y)):
// each keeps the angle, now about its own anchor, which moves where the turn takes it; it renders back the same way
// and survives a CIM round trip.
func TestExtract_RotatedLabel(t *testing.T) {
	const svg = `<?xml version="1.0"?>
<svg width="600" height="400" xmlns="http://www.w3.org/2000/svg">
<g data-type="5" data-name="A" id="1"><text x="100" y="113" style="fill:black;text-anchor:start;font-size:13px;font-family:Arial;" transform="rotate(-90,100,100)">A</text></g>
<g data-type="5" data-name="B" id="2"><text x="200" y="113" style="fill:black;text-anchor:start;font-size:13px;font-family:Arial;" transform="rotate(90,200,100)">B</text></g>
<g data-type="5" data-name="C" id="3"><text x="300" y="113" style="fill:black;text-anchor:start;font-size:13px;font-family:Arial;" transform="rotate(180,300,100)">C</text></g>
<g data-type="5" data-name="D" id="4"><text x="400" y="113" style="fill:black;text-anchor:start;font-size:13px;font-family:Arial;" transform="rotate(270,400,100)">D</text></g>
<g data-type="5" data-name="E" id="5"><text x="500" y="113" style="fill:black;text-anchor:start;font-size:13px;font-family:Arial;">E</text></g>
</svg>`
	d, _, err := extract.Extract([]byte(svg), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]model.Label{
		"A": {X: 113, Y: 100, Rotate: -90}, "B": {X: 187, Y: 100, Rotate: 90}, "C": {X: 300, Y: 87, Rotate: 180},
		"D": {X: 413, Y: 100, Rotate: -90}, "E": {X: 500, Y: 113},
	}
	byText := map[string]model.Label{}
	for _, l := range d.Labels {
		byText[l.Text] = l
		if w := want[l.Text]; l.X != w.X || l.Y != w.Y || l.Rotate != w.Rotate {
			t.Errorf("%s = (%g, %g) turned %g, want (%g, %g) turned %g", l.Text, l.X, l.Y, l.Rotate, w.X, w.Y, w.Rotate)
		}
	}
	if len(byText) != len(want) {
		t.Fatalf("labels = %+v", d.Labels)
	}

	var b strings.Builder
	render.WriteLabel(&b, byText["A"], render.Static)
	if !strings.Contains(b.String(), `transform="rotate(-90,113,100)"`) {
		t.Errorf("rendered = %s", b.String())
	}
	b.Reset()
	render.WriteLabel(&b, byText["E"], render.Static)
	if strings.Contains(b.String(), "transform") {
		t.Errorf("a level label has no transform: %s", b.String())
	}

	d.Nodes = append(d.Nodes, model.Node{ID: 100, X: 50, Y: 300}, model.Node{ID: 101, X: 50, Y: 320})
	d.Elements = append(d.Elements, model.Element{ID: 102, Class: model.ClassBreaker, Shape: "41", Name: "Q", X: 50, Y: 310,
		Ports: []model.Port{{Name: "1", Node: 100}, {Name: "2", Node: 101}}})
	out := ExportCIM(d, testExport(CIMOptions{Name: "t"}))
	pages, err := ImportCIM([][]byte{out.EQ, out.DL}, testImport(CIMImportOptions{Grid: 10}))
	if err != nil {
		t.Fatal(err)
	}
	back := 0
	for _, l := range pages[0].Diagram.Labels {
		w, ok := want[l.Text]
		if ok {
			back++
		}
		if ok && l.Rotate != w.Rotate {
			t.Errorf("%s back from CIM turned %g, want %g", l.Text, l.Rotate, w.Rotate)
		}
	}
	if back != len(want) {
		t.Errorf("%d of %d labels back from CIM", back, len(want))
	}
}

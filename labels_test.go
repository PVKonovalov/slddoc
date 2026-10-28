package slddoc

import (
	"strings"
	"testing"
)

// TestExtract_RTFTextBecomesLabel checks that RTF text (391), from real
// markup (an equipment list),
// is read as a plain Text label: every line, its position, size, colour and
// font, a fresh id, and nothing reported as skipped.
func TestExtract_RTFTextBecomesLabel(t *testing.T) {
	const svg = `<?xml version="1.0"?>
<svg width="600" height="400" xmlns="http://www.w3.org/2000/svg">
<!-- текст_RTF:391 -->
<text x="23" y="25" style="fill:black;font-size:13px;font-family:Arial" data-type="391" data-voltage="black" >РЛНД-2-110/660; прив. ПРН-220М<tspan style="fill:black;font-size:13px;font-family:Arial" x="23" dy="16" >РЛНД-1б-110/600; прив.ПРН-220М</tspan><tspan style="fill:black;font-size:13px;font-family:Arial" x="23" dy="16" >ОД-110/600; привод ШПО</tspan><tspan style="fill:black;font-size:13px;font-family:Arial" x="23" dy="16" >ТДТН-40000-110/35/10</tspan><tspan style="fill:black;font-size:13px;font-family:Arial" x="23" dy="16" > </tspan></text>
</svg>`
	d, report, err := Extract([]byte(svg), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Skipped) != 0 || len(report.Failed) != 0 {
		t.Fatalf("skipped=%v failed=%v", report.Skipped, report.Failed)
	}
	if len(d.Labels) != 1 {
		t.Fatalf("labels = %+v, want one", d.Labels)
	}
	l := d.Labels[0]
	want := "РЛНД-2-110/660; прив. ПРН-220М\nРЛНД-1б-110/600; прив.ПРН-220М\nОД-110/600; привод ШПО\nТДТН-40000-110/35/10"
	if l.Text != want {
		t.Errorf("text = %q, want %q", l.Text, want)
	}
	if l.X != 23 || l.Y != 25 || l.Size != 13 || l.Color != "black" || l.Font != "Arial" || l.ID == 0 || l.For != 0 {
		t.Errorf("label = %+v", l)
	}
}

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
	d, _, err := Extract([]byte(svg), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]Label{
		"A": {X: 113, Y: 100, Rotate: -90}, "B": {X: 187, Y: 100, Rotate: 90}, "C": {X: 300, Y: 87, Rotate: 180},
		"D": {X: 413, Y: 100, Rotate: -90}, "E": {X: 500, Y: 113},
	}
	byText := map[string]Label{}
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
	writeLabel(&b, byText["A"], Static)
	if !strings.Contains(b.String(), `transform="rotate(-90,113,100)"`) {
		t.Errorf("rendered = %s", b.String())
	}
	b.Reset()
	writeLabel(&b, byText["E"], Static)
	if strings.Contains(b.String(), "transform") {
		t.Errorf("a level label has no transform: %s", b.String())
	}

	d.Nodes = append(d.Nodes, Node{ID: 100, X: 50, Y: 300}, Node{ID: 101, X: 50, Y: 320})
	d.Elements = append(d.Elements, Element{ID: 102, Class: ClassBreaker, Shape: "41", Name: "Q", X: 50, Y: 310,
		Ports: []Port{{Name: "1", Node: 100}, {Name: "2", Node: 101}}})
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

// TestExtract_MirroredArresterAndHangingName reads xsde2svg's surge arresters (29) drawn with and without xMirror,
// turned and level, and a name hung below its point (dominant-baseline:text-before-edge): the mirror and the
// vertical anchor come back, and the anchor renders as xsde2svg writes it.
func TestExtract_MirroredArresterAndHangingName(t *testing.T) {
	const svg = `<?xml version="1.0"?>
<svg width="600" height="400" xmlns="http://www.w3.org/2000/svg">
<path d="M 450 430 v 10 h 5 v 20 h -5 v 10 v -10 h -5 v -20 h 5 z M 460 468 v -6 l -20 -24" style="fill:none;stroke:deepskyblue;stroke-width:1" id="1" data-voltage="deepskyblue" data-type="29" transform="rotate(-90,450,450)" />
<path d="M 300 130 v 10 h 5 v 20 h -5 v 10 v -10 h -5 v -20 h 5 z M 290 168 v -6 l 20 -24" style="fill:none;stroke:deepskyblue;stroke-width:1" id="2" data-voltage="deepskyblue" data-type="29" transform="rotate(-90,300,150)" />
<path d="M 100 130 v 10 h 5 v 20 h -5 v 10 v -10 h -5 v -20 h 5 z M 110 168 v -6 l -20 -24" style="fill:none;stroke:deepskyblue;stroke-width:1" id="3" data-voltage="deepskyblue" data-type="29" />
<g data-type="5" data-name="N" id="4"><text x="278" y="562" style="fill:black;text-anchor:end;dominant-baseline:text-before-edge;font-size:9px;font-family:Arial">N</text></g>
</svg>`
	d, _, err := Extract([]byte(svg), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[int]bool{1: true, 2: false, 3: true}
	for _, e := range d.Elements {
		if m, ok := want[e.ID]; ok && e.Mirror != m {
			t.Errorf("arrester %d mirror = %v, want %v", e.ID, e.Mirror, m)
		}
		delete(want, e.ID)
	}
	if len(want) != 0 {
		t.Errorf("arresters not read: %v", want)
	}
	if len(d.Labels) != 1 || d.Labels[0].VAlign != "top" {
		t.Fatalf("labels = %+v", d.Labels)
	}
	var b strings.Builder
	writeLabel(&b, d.Labels[0], Static)
	if !strings.Contains(b.String(), "dominant-baseline:text-before-edge;") {
		t.Errorf("rendered = %s", b.String())
	}
}

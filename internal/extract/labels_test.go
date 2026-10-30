package extract

import (
	"strings"
	"testing"

	"github.com/PVKonovalov/slddoc/internal/render"
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
	render.WriteLabel(&b, d.Labels[0], render.Static)
	if !strings.Contains(b.String(), "dominant-baseline:text-before-edge;") {
		t.Errorf("rendered = %s", b.String())
	}
}

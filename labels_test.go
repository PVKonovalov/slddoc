package slddoc

import "testing"

// TestExtract_RTFTextBecomesLabel checks that RTF text (391), from real
// markup (sld-svg examples/test/Test_391_text_RTF.svg, an equipment list),
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

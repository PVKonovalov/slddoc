package extract

import (
	"testing"

	"github.com/PVKonovalov/slddoc/internal/model"
)

// TestExtractDevice reads real xsde2svg Devices (element_130.go): a bare path, unrotated and rotated, each followed by
// its untyped "Wh" text, which must not become a free label; an unrelated caption after them still does.
func TestExtractDevice(t *testing.T) {
	const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="400" height="500">
<path d="M 135 385 h 30 v 30 h -30 z M 135 390 h 30" style="fill:none;stroke:olive;stroke-width:1" id="148795290" data-type="130" data-voltage="olive" />
<text x="150" y="402" style="fill:olive;text-anchor:middle;dominant-baseline:middle;font-size:18px;font-family:Arial font-weight: bold" >Wh</text>
<path d="M 235 385 h 30 v 30 h -30 z M 235 390 h 30" style="fill:none;stroke:olive;stroke-width:1" transform="rotate(90,250,400)" id="148795294" data-type="130" data-voltage="olive" />
<text x="250" y="402" style="fill:olive;text-anchor:middle;dominant-baseline:middle;font-size:18px;font-family:Arial font-weight: bold" >Wh</text>
<text x="300" y="450" style="fill:olive;font-size:12px" >Meter A</text>
</svg>`
	d, report, err := Extract([]byte(svg), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Failed) != 0 || len(report.Skipped) != 0 {
		t.Fatalf("report = %+v", report)
	}
	if len(d.Elements) != 2 {
		t.Fatalf("elements = %+v", d.Elements)
	}
	want := []struct {
		id     int
		x, y   float64
		orient int
	}{{148795290, 150, 400, 0}, {148795294, 250, 400, 90}}
	for i, w := range want {
		el := d.Elements[i]
		if el.Class != model.ClassDevice || el.Shape != "130" || el.ID != w.id || el.X != w.x || el.Y != w.y || el.Orient != w.orient || len(el.Ports) != 0 {
			t.Errorf("element %d = %+v, want id %d at (%v,%v) orient %d", i, el, w.id, w.x, w.y, w.orient)
		}
	}
	for _, l := range d.Labels {
		if l.Text == "Wh" {
			t.Errorf("stray Wh label %+v", l)
		}
	}
	if len(d.Labels) != 1 || d.Labels[0].Text != "Meter A" {
		t.Errorf("labels = %+v", d.Labels)
	}
}

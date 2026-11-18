package extract

import (
	"testing"

	"github.com/PVKonovalov/slddoc/internal/model"
)

// TestExtractArcContact reads arc-extinguishing contacts (code 320006) in element_3206.go's output format, as no real
// export has one: closed, open and rotated. Each is a Disconnector with terminals at the ends of its fixed legs (±21).
func TestExtractArcContact(t *testing.T) {
	const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="400" height="400">
<g id="1" data-type="320006" >
<path d="M 100 106 v -13" data-state="1" style="fill:none;stroke:white;stroke-width:1" data-voltage="white" />
<path d="M 96 91 h 8 m 0 18 h -8" style="fill:none;stroke:white;stroke-width:1" data-voltage="white" />
<path d="M 100 91 v -12 M 100 109 v 12" style="fill:none;stroke:white;stroke-width:1" data-voltage="white" />
</g>
<g id="2" data-type="320006" >
<path d="M 194 100 h 13" data-state="0" style="fill:none;stroke:white;stroke-width:1" data-voltage="white" />
<path d="M 196 91 h 8 m 0 18 h -8" style="fill:none;stroke:white;stroke-width:1" data-voltage="white" />
<path d="M 200 91 v -12 M 200 109 v 12" style="fill:none;stroke:white;stroke-width:1" data-voltage="white" />
</g>
<g id="3" data-type="320006" >
<g transform="rotate(90,300,100)" >
<path d="M 300 106 v -13" data-state="1" style="fill:none;stroke:white;stroke-width:1" data-voltage="white" />
<path d="M 296 91 h 8 m 0 18 h -8" style="fill:none;stroke:white;stroke-width:1" data-voltage="white" />
<path d="M 300 91 v -12 M 300 109 v 12" style="fill:none;stroke:white;stroke-width:1" data-voltage="white" />
</g>
</g>
</svg>`
	d, report, err := Extract([]byte(svg), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Failed) != 0 || len(report.Skipped) != 0 || len(d.Elements) != 3 {
		t.Fatalf("report = %+v, elements = %+v", report, d.Elements)
	}
	want := []struct {
		x, y   float64
		orient int
		state  int
	}{{100, 100, 0, 1}, {200, 100, 0, 0}, {300, 100, 90, 1}}
	for i, w := range want {
		el := d.Elements[i]
		if el.Class != model.ClassDisconnector || el.Shape != "320006" || el.X != w.x || el.Y != w.y || el.Orient != w.orient || el.State == nil || *el.State != w.state || len(el.Ports) != 2 {
			t.Errorf("element %d = %+v (state %v), want (%v,%v) orient %d state %d", i, el, el.State, w.x, w.y, w.orient, w.state)
		}
	}
}

package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/PVKonovalov/slddoc/internal/model"
)

// TestLoad_CircleFromCorners checks that a Circle saved the older way, by two
// bounding-box corners, loads as center and radius (half the larger side),
// and that one with no size gets the default radius.
func TestLoad_CircleFromCorners(t *testing.T) {
	const xml = `<?xml version="1.0" encoding="UTF-8"?>
<diagram width="100" height="100">
  <elements>
    <element id="1" class="Circle" shape="4" layer="0" x="0" y="0"><geometry><point x="913" y="1322"/><point x="927" y="1332"/></geometry></element>
    <element id="2" class="Circle" shape="4" layer="0" x="1000" y="470"></element>
    <element id="3" class="Circle" shape="4" layer="0" x="50" y="60" radius="0.5"></element>
  </elements>
</diagram>`
	d, err := model.Load(strings.NewReader(xml))
	if err != nil {
		t.Fatal(err)
	}
	want := []struct{ x, y, r float64 }{{920, 1327, 7}, {1000, 470, model.DefaultCircleRadius}, {50, 60, 0.5}}
	for i, w := range want {
		e := d.Elements[i]
		if e.X != w.x || e.Y != w.y || e.Radius != w.r || e.Points != nil {
			t.Errorf("circle %d = (%g,%g) r %g points %v, want (%g,%g) r %g", e.ID, e.X, e.Y, e.Radius, e.Points, w.x, w.y, w.r)
		}
	}
	var buf bytes.Buffer
	if err := Render(d, NewSymbolLibrary(nil), &buf, Static, "", nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `cx="50" cy="60" rx="0.5" ry="0.5"`) {
		t.Errorf("circle 3 not rendered with radius 0.5:\n%s", buf.String())
	}
}

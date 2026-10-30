package extract

import (
	"testing"

	"github.com/PVKonovalov/slddoc/internal/model"
)

// TestExtract_CircleCenterRadius checks that an imported shape-4 ellipse
// becomes a circle: center (cx, cy), radius the larger of rx/ry, no Points.
func TestExtract_CircleCenterRadius(t *testing.T) {
	const svg = `<?xml version="1.0"?>
<svg width="200" height="200" xmlns="http://www.w3.org/2000/svg">
<ellipse cx="920" cy="1327" rx="7" ry="5" style="fill:dimgray;stroke:#12161d;stroke-width:1" data-type="4" id="10" />
</svg>`
	d, _, err := Extract([]byte(svg), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Elements) != 1 {
		t.Fatalf("elements: %+v", d.Elements)
	}
	if e := d.Elements[0]; e.Class != model.ClassCircle || e.X != 920 || e.Y != 1327 || e.Radius != 7 || e.Points != nil {
		t.Errorf("circle = %+v, want center (920,1327) radius 7", e)
	}
}

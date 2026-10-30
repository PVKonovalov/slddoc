package extract

import (
	"math"
	"testing"

	"github.com/PVKonovalov/slddoc/internal/model"
)

// TestParseBlockingFilter has no real corpus instance to use (none exist),
// so its markup is element_389.go's own output format: a bare <path> with
// the id, data-voltage and data-type on it, unrotated, rotated about the
// center, and at a 2x export scale. The last case is this editor's own
// <g>-wrapped form.
func TestParseBlockingFilter(t *testing.T) {
	cases := []struct {
		name       string
		svg        string
		wantX      float64
		wantY      float64
		wantOrient int
		wantPorts  []model.Point
	}{
		{"unrotated", `<path d="M 190 100 l 10 11 v -22 l 10 11" style="fill:none;stroke:purple;stroke-width:1"  id="501" data-voltage="purple" data-type="389" />`,
			200, 100, 0, []model.Point{{X: 190, Y: 100}, {X: 210, Y: 100}}},
		{"rotated 90", `<path d="M 190 100 l 10 11 v -22 l 10 11" style="fill:none;stroke:purple;stroke-width:1" transform="rotate(90,200,100)" id="502" data-voltage="purple" data-type="389" />`,
			200, 100, 90, []model.Point{{X: 200, Y: 90}, {X: 200, Y: 110}}},
		{"2x scale", `<path d="M 380 300 l 20 22 v -44 l 20 22" style="fill:none;stroke:purple;stroke-width:1"  id="503" data-voltage="purple" data-type="389" />`,
			400, 300, 0, []model.Point{{X: 380, Y: 300}, {X: 420, Y: 300}}},
		{"editor <g>", `<g id="504" data-name="LT-1" data-voltage="purple" data-type="389" transform="rotate(-90,50,60)">
<path d="M 40 60 l 10 11 v -22 l 10 11" style="fill:none;stroke:purple;stroke-width:1" />
</g>`, 50, 60, -90, []model.Point{{X: 50, Y: 70}, {X: 50, Y: 50}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			el, ports, voltage, err := parseBlockingFilter(parseFirst(t, c.svg))
			if err != nil {
				t.Fatal(err)
			}
			if el.Class != model.ClassBlockingFilter || el.Shape != "389" || voltage != "purple" {
				t.Errorf("class/shape/voltage = %s/%s/%s", el.Class, el.Shape, voltage)
			}
			if el.X != c.wantX || el.Y != c.wantY || el.Orient != c.wantOrient {
				t.Errorf("anchor/orient = (%v,%v,%d), want (%v,%v,%d)", el.X, el.Y, el.Orient, c.wantX, c.wantY, c.wantOrient)
			}
			for i := range c.wantPorts {
				if len(ports) != 2 || math.Abs(ports[i].X-c.wantPorts[i].X) > 1e-9 || math.Abs(ports[i].Y-c.wantPorts[i].Y) > 1e-9 {
					t.Fatalf("ports = %v, want %v", ports, c.wantPorts)
				}
			}
		})
	}
}

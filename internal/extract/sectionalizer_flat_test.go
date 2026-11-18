package extract

import (
	"testing"

	"github.com/PVKonovalov/slddoc/internal/model"
)

// TestExtractSectionalizerFlat reads Sectionalizers (164) as an older xsde2svg draws them: only the current state, as
// flat paths with data-state, no visibility-swapped groups. Closed has a 10-wide top tick, Open an 8-wide one.
func TestExtractSectionalizerFlat(t *testing.T) {
	const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="400" height="400">
<g id="1" data-voltage="deepskyblue" data-type="164"  >
<path d="M 95 110 h 10 M 95 90 h 10 M 100 91 v 18 M 100 100 h 11 " data-state="1" style="fill:none;stroke:deepskyblue;stroke-width:1" />
<path d="M 111 100 l -8 3 v -6 z" style="fill:deepskyblue;stroke:deepskyblue;stroke-width:1" />
</g>
<g id="2" data-voltage="deepskyblue" data-type="164"  >
<path d="M 200 109 a 2 2 0 1 1 0 4 a 2 2 0 0 1 0 -4" style="fill:#12161d;stroke:deepskyblue;stroke-width:1" />
<path d="M 196 90 h 8 M 204 100 h 11 M 206 95 l -6 14" data-state="0" style="fill:none;stroke:deepskyblue;stroke-width:1" />
<path d="M 215 100 l -8 3 v -6 z " style="fill:deepskyblue;stroke:deepskyblue;stroke-width:1" />
</g>
</svg>`
	d, report, err := Extract([]byte(svg), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Failed) != 0 || len(report.Skipped) != 0 || len(d.Elements) != 2 {
		t.Fatalf("report = %+v, elements = %+v", report, d.Elements)
	}
	want := []struct {
		x, y  float64
		state int
	}{{100, 100, 1}, {200, 100, 0}}
	for i, w := range want {
		el := d.Elements[i]
		if el.Class != model.ClassSectionalizer || el.Shape != "164" || el.X != w.x || el.Y != w.y || el.Orient != 0 || el.State == nil || *el.State != w.state || len(el.Ports) != 2 || el.Voltage == 0 {
			t.Errorf("element %d = %+v (state %v), want (%v,%v) state %d", i, el, el.State, w.x, w.y, w.state)
		}
		for j, dy := range []float64{-10, 10} {
			var n *model.Node
			for k := range d.Nodes {
				if d.Nodes[k].ID == el.Ports[j].Node {
					n = &d.Nodes[k]
				}
			}
			if n == nil || n.X != w.x || n.Y != w.y+dy {
				t.Errorf("element %d port %d node = %+v, want (%v,%v)", i, j, n, w.x, w.y+dy)
			}
		}
	}
}

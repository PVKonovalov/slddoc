package extract

import (
	"math"
	"testing"

	"github.com/PVKonovalov/slddoc/internal/model"
)

// sectionalizerMirrorSVG holds Sectionalizers (164): 1-6 as xsde2svg draws them (1, 2, 5 with xMirror, 3, 4, 6
// without; 1, 3, 5 rotated 90 with legs out to ±20; 5 and 6 open), 7 and 8 as this editor saves them (7 mirrored with
// leads out to ±20, 8 neither).
const sectionalizerMirrorSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="400" height="400">
<g id="1" data-type="164" transform="rotate(90,250,110)" data-voltage="#FF5555" >
<g data-state="1" visibility="visible" >
<path d="M 250 90 v 10 M 250 130 v -10" style="fill:none;stroke:#FF5555;stroke-width:1" />
<g >
<path d="M 250 101 v 18" style="fill:none;stroke:#FF5555;stroke-width:1" />
</g>
<path d="M 245 120 h 10 M 245 100 h 10  M 250 110 h 11 " style="fill:none;stroke:#FF5555;stroke-width:1" />
<path d="M 261 110 l -8 3 v -6 z" style="fill:#FF5555;stroke:#FF5555;stroke-width:1" />
</g>
<g data-state="0" visibility="hidden" >
<path d="M 250 90 v 10 M 250 130 v -10" style="fill:none;stroke:#FF5555;stroke-width:1" />
<g >
<path d="M 256 105 l -6 14" style="fill:none;stroke:#FF5555;stroke-width:1" />
</g>
<path d="M 245 100 h 10 M 254 110 h 11" style="fill:none;stroke:#FF5555;stroke-width:1" />
<path d="M 265 110 l -8 3 v -6 z " style="fill:#FF5555;stroke:#FF5555;stroke-width:1" />
<path d="M 250 119 a 2 2 0 1 1 0 4 a 2 2 0 0 1 0 -4" style="fill:#f5ebeb;stroke:#FF5555;stroke-width:1" />
</g>
</g>
<g id="2" data-type="164"  data-voltage="#FF5555" >
<g data-state="1" visibility="visible" >
<g >
<path d="M 100 121 v 18" style="fill:none;stroke:#FF5555;stroke-width:1" />
</g>
<path d="M 95 140 h 10 M 95 120 h 10  M 100 130 h 11 " style="fill:none;stroke:#FF5555;stroke-width:1" />
<path d="M 111 130 l -8 3 v -6 z" style="fill:#FF5555;stroke:#FF5555;stroke-width:1" />
</g>
<g data-state="0" visibility="hidden" >
<g >
<path d="M 106 125 l -6 14" style="fill:none;stroke:#FF5555;stroke-width:1" />
</g>
<path d="M 95 120 h 10 M 104 130 h 11" style="fill:none;stroke:#FF5555;stroke-width:1" />
<path d="M 115 130 l -8 3 v -6 z " style="fill:#FF5555;stroke:#FF5555;stroke-width:1" />
<path d="M 100 139 a 2 2 0 1 1 0 4 a 2 2 0 0 1 0 -4" style="fill:#f5ebeb;stroke:#FF5555;stroke-width:1" />
</g>
</g>
<g id="3" data-type="164" transform="rotate(90,250,110)" data-voltage="#FF5555" >
<g data-state="1" visibility="visible" >
<path d="M 250 90 v 10 M 250 130 v -10" style="fill:none;stroke:#FF5555;stroke-width:1" />
<g >
<path d="M 250 101 v 18" style="fill:none;stroke:#FF5555;stroke-width:1" />
</g>
<path d="M 245 120 h 10 M 245 100 h 10  M 250 110 h -11 " style="fill:none;stroke:#FF5555;stroke-width:1" />
<path d="M 239 110 l 8 3 v -6 z" style="fill:#FF5555;stroke:#FF5555;stroke-width:1" />
</g>
<g data-state="0" visibility="hidden" >
<path d="M 250 90 v 10 M 250 130 v -10" style="fill:none;stroke:#FF5555;stroke-width:1" />
<g >
<path d="M 244 105 l 6 14" style="fill:none;stroke:#FF5555;stroke-width:1" />
</g>
<path d="M 245 100 h 10 M 246 110 h -11" style="fill:none;stroke:#FF5555;stroke-width:1" />
<path d="M 235 110 l 8 3 v -6 z " style="fill:#FF5555;stroke:#FF5555;stroke-width:1" />
<path d="M 250 119 a 2 2 0 1 1 0 4 a 2 2 0 0 1 0 -4" style="fill:#f5ebeb;stroke:#FF5555;stroke-width:1" />
</g>
</g>
<g id="4" data-type="164"  data-voltage="#FF5555" >
<g data-state="1" visibility="visible" >
<g >
<path d="M 100 121 v 18" style="fill:none;stroke:#FF5555;stroke-width:1" />
</g>
<path d="M 95 140 h 10 M 95 120 h 10  M 100 130 h -11 " style="fill:none;stroke:#FF5555;stroke-width:1" />
<path d="M 89 130 l 8 3 v -6 z" style="fill:#FF5555;stroke:#FF5555;stroke-width:1" />
</g>
<g data-state="0" visibility="hidden" >
<g >
<path d="M 94 125 l 6 14" style="fill:none;stroke:#FF5555;stroke-width:1" />
</g>
<path d="M 95 120 h 10 M 96 130 h -11" style="fill:none;stroke:#FF5555;stroke-width:1" />
<path d="M 85 130 l 8 3 v -6 z " style="fill:#FF5555;stroke:#FF5555;stroke-width:1" />
<path d="M 100 139 a 2 2 0 1 1 0 4 a 2 2 0 0 1 0 -4" style="fill:#f5ebeb;stroke:#FF5555;stroke-width:1" />
</g>
</g>
<g id="5" data-type="164" transform="rotate(90,250,110)" data-voltage="#FF5555" >
<g data-state="1" visibility="hidden" >
<path d="M 250 90 v 10 M 250 130 v -10" style="fill:none;stroke:#FF5555;stroke-width:1" />
<g >
<path d="M 250 101 v 18" style="fill:none;stroke:#FF5555;stroke-width:1" />
</g>
<path d="M 245 120 h 10 M 245 100 h 10  M 250 110 h 11 " style="fill:none;stroke:#FF5555;stroke-width:1" />
<path d="M 261 110 l -8 3 v -6 z" style="fill:#FF5555;stroke:#FF5555;stroke-width:1" />
</g>
<g data-state="0" visibility="visible" >
<path d="M 250 90 v 10 M 250 130 v -10" style="fill:none;stroke:#FF5555;stroke-width:1" />
<g >
<path d="M 256 105 l -6 14" style="fill:none;stroke:#FF5555;stroke-width:1" />
</g>
<path d="M 245 100 h 10 M 254 110 h 11" style="fill:none;stroke:#FF5555;stroke-width:1" />
<path d="M 265 110 l -8 3 v -6 z " style="fill:#FF5555;stroke:#FF5555;stroke-width:1" />
<path d="M 250 119 a 2 2 0 1 1 0 4 a 2 2 0 0 1 0 -4" style="fill:#f5ebeb;stroke:#FF5555;stroke-width:1" />
</g>
</g>
<g id="6" data-type="164"  data-voltage="#FF5555" >
<g data-state="1" visibility="hidden" >
<g >
<path d="M 100 121 v 18" style="fill:none;stroke:#FF5555;stroke-width:1" />
</g>
<path d="M 95 140 h 10 M 95 120 h 10  M 100 130 h -11 " style="fill:none;stroke:#FF5555;stroke-width:1" />
<path d="M 89 130 l 8 3 v -6 z" style="fill:#FF5555;stroke:#FF5555;stroke-width:1" />
</g>
<g data-state="0" visibility="visible" >
<g >
<path d="M 94 125 l 6 14" style="fill:none;stroke:#FF5555;stroke-width:1" />
</g>
<path d="M 95 120 h 10 M 96 130 h -11" style="fill:none;stroke:#FF5555;stroke-width:1" />
<path d="M 85 130 l 8 3 v -6 z " style="fill:#FF5555;stroke:#FF5555;stroke-width:1" />
<path d="M 100 139 a 2 2 0 1 1 0 4 a 2 2 0 0 1 0 -4" style="fill:#f5ebeb;stroke:#FF5555;stroke-width:1" />
</g>
</g>
<g id="7" data-name="" data-voltage="gray" data-type="164" transform="rotate(90,100,300) translate(200,0) scale(-1,1)">
<path d="M 95 290 h 10" style="fill:none;stroke:gray;stroke-width:1" />
<path d="M 95 310 h 10" style="fill:none;stroke:gray;stroke-width:1" />
<path d="M 100 291 v 18 M 100 300 h -11" data-state="1" style="fill:none;stroke:gray;stroke-width:1" />
<path d="M 89 300 l 8 3 v -6 z" style="fill:gray;stroke:gray;stroke-width:1" />
<path d="M 100 290 V 280 M 100 310 V 320" style="fill:none;stroke:gray;stroke-width:1" />
</g>
<g id="8" data-name="" data-voltage="gray" data-type="164" transform="rotate(90,200,300)">
<path d="M 195 290 h 10" style="fill:none;stroke:gray;stroke-width:1" />
<path d="M 195 310 h 10" style="fill:none;stroke:gray;stroke-width:1" />
<path d="M 200 291 v 18 M 200 300 h -11" data-state="1" style="fill:none;stroke:gray;stroke-width:1" />
<path d="M 189 300 l 8 3 v -6 z" style="fill:gray;stroke:gray;stroke-width:1" />
</g>
</svg>`

// TestExtractSectionalizerMirrorAndLegs reads a Sectionalizer's mirroring and its terminals at the ends of its legs.
func TestExtractSectionalizerMirrorAndLegs(t *testing.T) {
	d, report, err := Extract([]byte(sectionalizerMirrorSVG), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Failed) != 0 || len(d.Elements) != 8 {
		t.Fatalf("report = %+v, elements = %d", report, len(d.Elements))
	}
	want := []struct {
		x, y   float64
		orient int
		mirror bool
		state  int
		span   float64
	}{
		{250, 110, 90, true, 1, 40},
		{100, 130, 0, true, 1, 0},
		{250, 110, 90, false, 1, 40},
		{100, 130, 0, false, 1, 0},
		{250, 110, 90, true, 0, 40},
		{100, 130, 0, false, 0, 0},
		{100, 300, 90, true, 1, 40},
		{200, 300, 90, false, 1, 0},
	}
	for i, w := range want {
		el := d.Elements[i]
		if el.Class != model.ClassSectionalizer || el.X != w.x || el.Y != w.y || el.Orient != w.orient || el.Mirror != w.mirror || el.State == nil || *el.State != w.state {
			t.Errorf("element %d = (%v,%v) orient %d mirror %v state %v, want %+v", i+1, el.X, el.Y, el.Orient, el.Mirror, el.State, w)
		}
		half := 10.0
		if w.span > 0 {
			half = w.span / 2
		}
		for j, sign := range []float64{-1, 1} {
			var n *model.Node
			for k := range d.Nodes {
				if d.Nodes[k].ID == el.Ports[j].Node {
					n = &d.Nodes[k]
				}
			}
			r := float64(w.orient) * math.Pi / 180
			wx, wy := w.x-sign*half*math.Sin(r), w.y+sign*half*math.Cos(r)
			if n == nil || math.Abs(n.X-wx) > 1e-9 || math.Abs(n.Y-wy) > 1e-9 {
				t.Errorf("element %d port %d node = %+v, want (%v,%v)", i+1, j+1, n, wx, wy)
			}
		}
		if el.Span != w.span {
			t.Errorf("element %d span = %v, want %v", i+1, el.Span, w.span)
		}
	}
}

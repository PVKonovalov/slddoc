package slddoc

import (
	"math"
	"testing"
)

// TestParseWithdrawableSectionalizer uses real xsde2svg markup for shape 50
// (ctrlroom sld1 corpus): Closed and Open, both xMirror layouts, rotated
// (on an inner <g>, as the source writes it) and at a 2x export scale.
func TestParseWithdrawableSectionalizer(t *testing.T) {
	cases := []struct {
		name       string
		svg        string
		wantX      float64
		wantY      float64
		wantOrient int
		wantState  int
		wantMirror bool
		wantName   string
		wantPorts  []Point
	}{
		{
			name: "closed, xMirror 1 (Switches.svg 10)",
			svg: `<g id="10" data-type="50" >
<path d="M 550 286 v -12 m" data-state="1" style="fill:none;stroke:#FF5555;stroke-width:1" data-voltage="#FF5555" />
<path d="M 550 278 h 2 v 2 h -2 z " style="fill:#FF5555;stroke:#FF5555;stroke-width:1" />
<path d="M 550 258 l -4 4 m 4 -4 l 4 4 m -4 -4 v 12 m -4 0 h 8 m -4 2  m -4 20 h 8 m -4 0 v 12 l -4 -4 m 4 4 l 4 -4 M 550 310 l -4 -4 m 4 4 l 4 -4 M 550 250 l -4 4 m 4 -4 l 4 4" style="fill:none;stroke:#FF5555;stroke-width:1" />
<path d="M 550 310 l -4 -4 m 4 4 l 4 -4 M 550 250 l -4 4 m 4 -4 l 4 4" style="fill:none;stroke:#FF5555;stroke-width:1" />
</g>`,
			wantX: 550, wantY: 280, wantState: 1,
			wantPorts: []Point{{550, 250}, {550, 310}},
		},
		{
			name: "open, xMirror 0 (corpus id 120957050)",
			svg: `<g id="120957050" data-type="50" >
<path d="M 1824 2120 h 12" data-state="0" style="fill:none;stroke:#555555;stroke-width:1" data-voltage="#555555" data-name="ВС2-0,4" />
<path d="M 1828 2120 h 2 v 2 h -2 z " style="fill:#555555;stroke:#555555;stroke-width:1" />
<path d="M 1830 2098 l -4 4 m 4 -4 l 4 4 m -4 -4 v 12 m -4 0 h 8 m -4 2  m -4 20 h 8 m -4 0 v 12 l -4 -4 m 4 4 l 4 -4 M 1830 2150 l -4 -4 m 4 4 l 4 -4 M 1830 2090 l -4 4 m 4 -4 l 4 4" style="fill:none;stroke:#555555;stroke-width:1" />
<path d="M 1830 2150 l -4 -4 m 4 4 l 4 -4 M 1830 2090 l -4 4 m 4 -4 l 4 4" style="fill:none;stroke:#555555;stroke-width:1" />
</g>`,
			wantX: 1830, wantY: 2120, wantState: 0, wantMirror: true, wantName: "ВС2-0,4",
			wantPorts: []Point{{1830, 2090}, {1830, 2150}},
		},
		{
			name: "closed, xMirror 0, rotated 90 (VLADTEC2 67829)",
			svg: `<g id="67829" data-type="50" >
<g transform="rotate(90,130,3570)" >
<path d="M 130 3576 v -12 m" data-state="1" style="fill:none;stroke:#7F7F7F;stroke-width:1" data-voltage="#7F7F7F" />
<path d="M 130 3568 h -2 v -2 h 2 z " style="fill:#7F7F7F;stroke:#7F7F7F;stroke-width:1" />
<path d="M 130 3548 l -4 4 m 4 -4 l 4 4 m -4 -4 v 12 m -4 0 h 8 m -4 2  m -4 20 h 8 m -4 0 v 12 l -4 -4 m 4 4 l 4 -4 M 130 3600 l -4 -4 m 4 4 l 4 -4 M 130 3540 l -4 4 m 4 -4 l 4 4" style="fill:none;stroke:#7F7F7F;stroke-width:1" />
<path d="M 130 3600 l -4 -4 m 4 4 l 4 -4 M 130 3540 l -4 4 m 4 -4 l 4 4" style="fill:none;stroke:#7F7F7F;stroke-width:1" />
</g>
</g>`,
			wantX: 130, wantY: 3570, wantOrient: 90, wantState: 1, wantMirror: true,
			wantPorts: []Point{{160, 3570}, {100, 3570}},
		},
		{
			name: "closed, xMirror 1, rotated -90, 2x scale (corpus id 8242)",
			svg: `<g id="8242" data-type="50" >
<g transform="rotate(-90,1700,2140)" >
<path d="M 1700 2152 v -24 m" data-state="1" style="fill:none;stroke:#555555;stroke-width:1" data-voltage="#555555" data-name="АВ-0,4 ТСН-2" />
<path d="M 1700 2136 h 4 v 4 h -4 z " style="fill:#555555;stroke:#555555;stroke-width:1" />
<path d="M 1700 2096 l -8 8 m 8 -8 l 8 8 m -8 -8 v 24 m -8 0 h 16 m -8 4  m -8 40 h 16 m -8 0 v 24 l -8 -8 m 8 8 l 8 -8 M 1700 2200 l -8 -8 m 8 8 l 8 -8 M 1700 2080 l -8 8 m 8 -8 l 8 8" style="fill:none;stroke:#555555;stroke-width:1" />
<path d="M 1700 2200 l -8 -8 m 8 8 l 8 -8 M 1700 2080 l -8 8 m 8 -8 l 8 8" style="fill:none;stroke:#555555;stroke-width:1" />
</g>
</g>`,
			wantX: 1700, wantY: 2140, wantOrient: -90, wantState: 1, wantName: "АВ-0,4 ТСН-2",
			wantPorts: []Point{{1640, 2140}, {1760, 2140}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			el, ports, voltage, err := parseWithdrawableSectionalizer(parseFirst(t, c.svg))
			if err != nil {
				t.Fatal(err)
			}
			if el.Class != ClassSectionalizer || el.Shape != "50" || voltage == "" || el.Name != c.wantName {
				t.Errorf("class/shape/voltage/name = %s/%s/%q/%q", el.Class, el.Shape, voltage, el.Name)
			}
			if el.X != c.wantX || el.Y != c.wantY || el.Orient != c.wantOrient {
				t.Errorf("anchor/orient = (%v,%v,%d), want (%v,%v,%d)", el.X, el.Y, el.Orient, c.wantX, c.wantY, c.wantOrient)
			}
			if el.State == nil || *el.State != c.wantState || el.Mirror != c.wantMirror {
				t.Errorf("state/mirror = %v/%v, want %d/%v", el.State, el.Mirror, c.wantState, c.wantMirror)
			}
			if len(ports) != 2 || len(el.Ports) != 2 {
				t.Fatalf("ports = %v / %v", ports, el.Ports)
			}
			for i := range ports {
				if math.Abs(ports[i].X-c.wantPorts[i].X) > 1e-9 || math.Abs(ports[i].Y-c.wantPorts[i].Y) > 1e-9 {
					t.Errorf("ports = %v, want %v", ports, c.wantPorts)
					break
				}
			}
		})
	}
}

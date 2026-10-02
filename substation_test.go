package slddoc

import (
	"bytes"
	"fmt"
	"testing"
)

// TestParseSubstation uses real xsde2svg markup for shape 360 (ctrlroom
// corpus): small single-sector pictograms on a distribution-network map,
// plain and rotated, and a full-size one with a different outline and fill.
func TestParseSubstation(t *testing.T) {
	cases := []struct {
		name       string
		svg        string
		wantX      float64
		wantY      float64
		wantOrient int
		wantRadius float64
		wantFill   string
	}{
		{"radius 7 (corpus id 148703614)", `<g id="148703614"  data-type="360" data-voltage="#00B4C8" >
<path d="M 1790 1437 a 7 7 0 1 1 0 -14 a 7 7 0 0 1 0 14" style="fill:#00B4C8;stroke:#00B4C8;stroke-width:1" />
</g>`, 1790, 1430, 0, 7, "#00B4C8"},
		{"rotated -90 (corpus id 148703639)", `<g id="148703639" transform="rotate(-90,1490,1230)" data-type="360" data-voltage="#00B4C8" >
<path d="M 1490 1237 a 7 7 0 1 1 0 -14 a 7 7 0 0 1 0 14" style="fill:#00B4C8;stroke:#00B4C8;stroke-width:1" />
</g>`, 1490, 1230, -90, 7, "#00B4C8"},
		{"radius 20, own fill (corpus id 120958096)", `<g id="120958096" transform="rotate(-90,3800,2690)" data-type="360" data-voltage="coral" >
<path d="M 3800 2710 a 20 20 0 1 1 0 -40 a 20 20 0 0 1 0 40" style="fill:deepskyblue;stroke:coral;stroke-width:1" />
</g>`, 3800, 2690, -90, 0, "deepskyblue"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			el, ports, voltage, fills, err := parseSubstation(parseFirst(t, c.svg))
			if err != nil {
				t.Fatal(err)
			}
			if el.Class != ClassSubstation || el.Shape != "360" || len(el.Sectors) != 1 || len(fills) != 1 || fills[0] != c.wantFill || voltage == "" {
				t.Errorf("class/shape/sectors/fills/voltage = %s/%s/%v/%v/%q", el.Class, el.Shape, el.Sectors, fills, voltage)
			}
			if el.X != c.wantX || el.Y != c.wantY || el.Orient != c.wantOrient || el.Radius != c.wantRadius {
				t.Errorf("anchor/orient/radius = (%v,%v,%d,%v), want (%v,%v,%d,%v)", el.X, el.Y, el.Orient, el.Radius, c.wantX, c.wantY, c.wantOrient, c.wantRadius)
			}
			if len(ports) != 1 || ports[0] != (Point{c.wantX, c.wantY}) {
				t.Errorf("ports = %v", ports)
			}
		})
	}
}

// TestSubstation_RoundTrip renders 1 to 4 sectors, each a different voltage,
// rotated and at a non-default radius, and reads them back through Extract.
func TestSubstation_RoundTrip(t *testing.T) {
	classes := []VoltageClass{
		{ID: 1, Name: "110 kV", Color: "#0000ff"},
		{ID: 2, Name: "35 kV", Color: "#ff0000"},
		{ID: 3, Name: "10 kV", Color: "#00ff00"},
		{ID: 4, Name: "0.4 kV", Color: "#ffff00"},
		{ID: 5, Name: "outline", Color: "#ffffff"},
	}
	for n := 1; n <= 4; n++ {
		t.Run(fmt.Sprint(n, " sectors"), func(t *testing.T) {
			sectors := make([]SubstationSector, n)
			for i := range sectors {
				sectors[i].Voltage = i + 1
			}
			d := &Diagram{
				Width: 400, Height: 400, VoltageClasses: classes,
				Elements: []Element{{ID: 7, Class: ClassSubstation, Shape: "360", Voltage: 5, X: 100, Y: 200, Orient: 90, Radius: 14, Sectors: sectors}},
			}
			var buf bytes.Buffer
			if err := Render(d, NewSymbolLibrary(nil), &buf, Static, "", nil); err != nil {
				t.Fatal(err)
			}
			back, report, err := Extract(buf.Bytes(), "", nil)
			if err != nil || len(report.Failed) != 0 || len(back.Elements) != 1 {
				t.Fatalf("extract: %v %+v\n%s", err, report, buf.String())
			}
			g := back.Elements[0]
			if g.X != 100 || g.Y != 200 || g.Orient != 90 || g.Radius != 14 || len(g.Sectors) != n {
				t.Fatalf("extracted %+v\n%s", g, buf.String())
			}
			color := map[int]string{}
			for _, vc := range back.VoltageClasses {
				color[vc.ID] = vc.Color
			}
			if color[g.Voltage] != "#ffffff" {
				t.Errorf("outline = %q", color[g.Voltage])
			}
			for i, s := range g.Sectors {
				if color[s.Voltage] != classes[i].Color {
					t.Errorf("sector %d = %q, want %q", i, color[s.Voltage], classes[i].Color)
				}
			}
		})
	}
}

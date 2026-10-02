package slddoc

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// TestParsePowerPlant uses real xsde2svg markup for shape 38 (ctrlroom
// corpus): a rotated thermal plant
// at 2x scale, an unrotated hydro plant at 2.8x, and an instance of a kind
// the source doesn't draw (empty paths, no rotate()), which can't be placed.
func TestParsePowerPlant(t *testing.T) {
	cases := []struct {
		name       string
		svg        string
		wantX      float64
		wantY      float64
		wantOrient int
		wantRadius float64
		wantNType  int
		wantMirror bool
	}{
		{"thermal, rotated -180 (148703653)", `<g id="148703653" data-type="38" transform="rotate(-180,1270,1120)" data-voltage="#AA9600" >
<path d="M 1230 1120 v 40 h 80 v -40 z " style="stroke:#AA9600;stroke-width:1; fill:url(#diagonal)" />
<path d="M 1230 1120 v -40 h 80 v 40 z " style="stroke:#AA9600;stroke-width:1; fill:none" />
</g>`, 1270, 1120, normalizeOrient(-180), 40, 0, false},
		{"hydro (148703655)", `<g id="148703655" data-type="38"  data-voltage="#AA9600" >
<path d="M 1316 1184 h -112 v 112 z " style="stroke:#AA9600;stroke-width:1; fill:url(#horizontal)" />
<path d="M 1204 1296 h 112 v -112 " style="stroke:#AA9600;stroke-width:1; fill:none" />
</g>`, 1260, 1240, 0, 56, 1, false},
		{"hydro, xMirror (source format)", `<g id="9" data-type="38" data-voltage="red" >
<path d="M 80 80 h 40 v 40 z " style="stroke:red;stroke-width:1; fill:url(#horizontal)" />
<path d="M 120 120 h -40 v -40 " style="stroke:red;stroke-width:1; fill:none" />
</g>`, 100, 100, 0, 0, 1, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			el, ports, voltage, err := parsePowerPlant(parseFirst(t, c.svg))
			if err != nil {
				t.Fatal(err)
			}
			if el.Class != ClassPowerPlant || el.Shape != "38" || voltage == "" {
				t.Errorf("class/shape/voltage = %s/%s/%q", el.Class, el.Shape, voltage)
			}
			if el.X != c.wantX || el.Y != c.wantY || el.Orient != c.wantOrient || el.Radius != c.wantRadius || el.NType != c.wantNType || el.Mirror != c.wantMirror {
				t.Errorf("got (%v,%v) orient %d radius %v ntype %d mirror %v", el.X, el.Y, el.Orient, el.Radius, el.NType, el.Mirror)
			}
			if len(ports) != 1 || ports[0] != (Point{c.wantX, c.wantY}) {
				t.Errorf("ports = %v", ports)
			}
		})
	}

	empty := `<g id="32609" data-type="38"  data-voltage="#0099FF" >
<path d=""  />
<path d="" style="stroke:#0099FF;stroke-width:1; fill:none" />
</g>`
	if _, _, _, err := parsePowerPlant(parseFirst(t, empty)); err == nil {
		t.Error("an empty instance with no rotate() should fail: nowhere to place it")
	}
	rotatedEmpty := `<g id="32610" data-type="38" transform="rotate(90,300,400)" data-voltage="#0099FF" >
<path d=""  />
<path d="" style="stroke:#0099FF;stroke-width:1; fill:none" />
</g>`
	el, _, _, err := parsePowerPlant(parseFirst(t, rotatedEmpty))
	if err != nil || el.X != 300 || el.Y != 400 || el.NType != 0 || el.Radius != 0 {
		t.Errorf("empty with rotate() = %+v, %v; want thermal at (300,400)", el, err)
	}
}

// TestPowerPlant_RoundTrip renders both kinds (hydro mirrored too), rotated
// and not, and reads them back; the hatching is drawn as lines, never as a
// shared <pattern>.
func TestPowerPlant_RoundTrip(t *testing.T) {
	for _, c := range []struct {
		ntype  int
		mirror bool
		orient int
		radius float64
	}{{0, false, 0, 0}, {0, false, 90, 30}, {1, false, 0, 0}, {1, true, 0, 25}, {1, false, -90, 0}} {
		t.Run(fmt.Sprint(c), func(t *testing.T) {
			d := &Diagram{
				Width: 400, Height: 400, VoltageClasses: []VoltageClass{{ID: 1, Name: "110 kV", Color: "#0000ff"}},
				Elements: []Element{{ID: 3, Class: ClassPowerPlant, Shape: "38", Voltage: 1, X: 100, Y: 200, NType: c.ntype, Mirror: c.mirror, Orient: c.orient, Radius: c.radius}},
			}
			var buf bytes.Buffer
			if err := Render(d, NewSymbolLibrary(nil), &buf, Static, "", nil); err != nil {
				t.Fatal(err)
			}
			out := buf.String()
			if strings.Contains(out, "<pattern") || strings.Count(out, "<path") != 3 {
				t.Errorf("want three paths and no <pattern>:\n%s", out)
			}
			back, report, err := Extract(buf.Bytes(), "", nil)
			if err != nil || len(report.Failed) != 0 || len(back.Elements) != 1 {
				t.Fatalf("extract: %v %+v\n%s", err, report, out)
			}
			g := back.Elements[0]
			if g.X != 100 || g.Y != 200 || g.NType != c.ntype || g.Mirror != c.mirror || g.Orient != c.orient || g.Radius != c.radius {
				t.Errorf("extracted %+v\n%s", g, out)
			}
		})
	}
}

package extract

import (
	"bytes"
	"testing"

	"github.com/PVKonovalov/slddoc/internal/model"
	"github.com/PVKonovalov/slddoc/internal/render"
)

// transformerSVG holds power transformers (47) as xsde2svg draws them: 1 and 2 at size step -1 with windings 2 and 3
// led down, 2 mirrored, both regulated on winding 1; 3 mirrored at step 0, regulated on winding 3. The regulation
// arrow is in a <g data-tap-changer>.
const transformerSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="2000" height="1000">
<g id="1" data-name="T-1" data-type="47"  >
<circle cx="100" cy="163" r="15" style="fill:none;stroke:#FF5555;stroke-width:2" data-voltage="#FF5555" />
<path d="M 100 148 v -9" style="fill:none;stroke:#FF5555;stroke-width:2" data-voltage="#FF5555" />
<path d="M 100 163 l -7 -7 M 100 163 l 7 -7 M 100 163 l 0 7 M 103 163 l 3 0" style="fill:none;stroke:#12161d;stroke-width:1" />
<circle cx="112" cy="180" r="15" style="fill:none;stroke:purple;stroke-width:2" data-voltage="purple" />
<path d="M 114 195 v 7" style="fill:none;stroke:purple;stroke-width:2" data-voltage="purple" />
<path d="M 112 180 l -7 -7 M 112 180 l 7 -7 M 112 180 l 0 7" style="fill:none;stroke:#12161d;stroke-width:1" />
<circle cx="88" cy="180" r="15" style="fill:none;stroke:#339966;stroke-width:2" data-voltage="#339966" />
<path d="M 86 195 v 7" style="fill:none;stroke:#339966;stroke-width:2" data-voltage="#339966" />
<path d="M 88 175 l 5 11 l -11 0 z" style="fill:none;stroke:#12161d;stroke-width:1" />
<g data-tap-changer="0"  >
<path d="M 83 197 l 34 -34" style="fill:none;stroke:#12161d;stroke-width:1" />
<path d="M 120 165  l -5 -5 l 7 -2 z " style="fill:#12161d;stroke:#12161d;stroke-width:1" />
</g>
</g>
<g id="2" data-name="T-2" data-type="47"  >
<circle cx="100" cy="293" r="15" style="fill:none;stroke:#FF5555;stroke-width:2" data-voltage="#FF5555" />
<path d="M 100 278 v -9" style="fill:none;stroke:#FF5555;stroke-width:2" data-voltage="#FF5555" />
<path d="M 100 293 l -7 -7 M 100 293 l 7 -7 M 100 293 l 0 7 M 103 293 l 3 0" style="fill:none;stroke:#12161d;stroke-width:1" />
<circle cx="88" cy="310" r="15" style="fill:none;stroke:purple;stroke-width:2" data-voltage="purple" />
<path d="M 86 325 v 7" style="fill:none;stroke:purple;stroke-width:2" data-voltage="purple" />
<path d="M 88 310 l -7 -7 M 88 310 l 7 -7 M 88 310 l 0 7" style="fill:none;stroke:#12161d;stroke-width:1" />
<circle cx="112" cy="310" r="15" style="fill:none;stroke:#339966;stroke-width:2" data-voltage="#339966" />
<path d="M 114 325 v 7" style="fill:none;stroke:#339966;stroke-width:2" data-voltage="#339966" />
<path d="M 112 305 l 5 11 l -11 0 z" style="fill:none;stroke:#12161d;stroke-width:1" />
<g data-tap-changer="0"  >
<path d="M 83 327 l 34 -34" style="fill:none;stroke:#12161d;stroke-width:1" />
<path d="M 120 295  l -5 -5 l 7 -2 z " style="fill:#12161d;stroke:#12161d;stroke-width:1" />
</g>
</g>
<g id="3" data-name="T-3" data-type="47"  >
<circle cx="1917" cy="786" r="22" style="fill:none;stroke:#00A0F0;stroke-width:2" data-voltage="#00A0F0" />
<path d="M 1917 764 v -13" style="fill:none;stroke:#00A0F0;stroke-width:2" data-voltage="#00A0F0" />
<path d="M 1917 781 l 5 11 l -11 0 z" style="fill:none;stroke:#326400;stroke-width:1" />
<circle cx="1897" cy="811" r="22" style="fill:none;stroke:#965000;stroke-width:2" data-voltage="#965000" />
<path d="M 1875 811 h -8" style="fill:none;stroke:#965000;stroke-width:2" data-voltage="#965000" />
<path d="M 1897 811 l -7 -7 M 1897 811 l 7 -7 M 1897 811 l 0 7 M 1897 811 l 7 0" style="fill:none;stroke:#326400;stroke-width:1" />
<circle cx="1937" cy="811" r="22" style="fill:none;stroke:#962896;stroke-width:2" data-voltage="#962896" />
<path d="M 1959 811 h 8" style="fill:none;stroke:#962896;stroke-width:2" data-voltage="#962896" />
<path d="M 1937 811 l -7 -7 M 1937 811 l 7 -7 M 1937 811 l 0 7 M 1937 811 l 7 0" style="fill:none;stroke:#326400;stroke-width:1" />
<g data-tap-changer="2"  >
<path d="M 1892 836 l 50 -50" style="fill:none;stroke:#12161d;stroke-width:1" />
<path d="M 1945 788  l -5 -5 l 7 -2 z " style="fill:#12161d;stroke:#12161d;stroke-width:1" />
</g>
</g>
</svg>`

// transformerOldSVG holds 1 and 2 as an older xsde2svg draws them: the regulation arrow as two loose paths after the
// group.
const transformerOldSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="2000" height="1000">
<g id="1" data-name="T-1" data-type="47"  >
<circle cx="100" cy="163" r="15" style="fill:none;stroke:#FF5555;stroke-width:2" data-voltage="#FF5555" />
<path d="M 100 148 v -9" style="fill:none;stroke:#FF5555;stroke-width:2" data-voltage="#FF5555" />
<path d="M 100 163 l -7 -7 M 100 163 l 7 -7 M 100 163 l 0 7 M 103 163 l 3 0" style="fill:none;stroke:#12161d;stroke-width:1" />
<circle cx="112" cy="180" r="15" style="fill:none;stroke:purple;stroke-width:2" data-voltage="purple" />
<path d="M 114 195 v 7" style="fill:none;stroke:purple;stroke-width:2" data-voltage="purple" />
<path d="M 112 180 l -7 -7 M 112 180 l 7 -7 M 112 180 l 0 7" style="fill:none;stroke:#12161d;stroke-width:1" />
<circle cx="88" cy="180" r="15" style="fill:none;stroke:#339966;stroke-width:2" data-voltage="#339966" />
<path d="M 86 195 v 7" style="fill:none;stroke:#339966;stroke-width:2" data-voltage="#339966" />
<path d="M 88 175 l 5 11 l -11 0 z" style="fill:none;stroke:#12161d;stroke-width:1" />
</g>
<path d="M 83 197 l 34 -34" style="fill:none;stroke:#12161d;stroke-width:1"  />
<path d="M 120 165  l -5 -5 l 7 -2 z " style="fill:#12161d;stroke:#12161d;stroke-width:1"  />
<g id="2" data-name="T-2" data-type="47"  >
<circle cx="100" cy="293" r="15" style="fill:none;stroke:#FF5555;stroke-width:2" data-voltage="#FF5555" />
<path d="M 100 278 v -9" style="fill:none;stroke:#FF5555;stroke-width:2" data-voltage="#FF5555" />
<path d="M 100 293 l -7 -7 M 100 293 l 7 -7 M 100 293 l 0 7 M 103 293 l 3 0" style="fill:none;stroke:#12161d;stroke-width:1" />
<circle cx="88" cy="310" r="15" style="fill:none;stroke:purple;stroke-width:2" data-voltage="purple" />
<path d="M 86 325 v 7" style="fill:none;stroke:purple;stroke-width:2" data-voltage="purple" />
<path d="M 88 310 l -7 -7 M 88 310 l 7 -7 M 88 310 l 0 7" style="fill:none;stroke:#12161d;stroke-width:1" />
<circle cx="112" cy="310" r="15" style="fill:none;stroke:#339966;stroke-width:2" data-voltage="#339966" />
<path d="M 114 325 v 7" style="fill:none;stroke:#339966;stroke-width:2" data-voltage="#339966" />
<path d="M 112 305 l 5 11 l -11 0 z" style="fill:none;stroke:#12161d;stroke-width:1" />
</g>
<path d="M 83 327 l 34 -34" style="fill:none;stroke:#12161d;stroke-width:1"  />
<path d="M 120 295  l -5 -5 l 7 -2 z " style="fill:#12161d;stroke:#12161d;stroke-width:1"  />
</svg>`

type transformerWant struct {
	x, y      float64
	mirror    bool
	scale     int
	terminals [3]model.TerminalDirection
	tap       int
}

var transformerWants = []transformerWant{
	{100, 180, false, -1, [3]model.TerminalDirection{"", model.TerminalBottom, model.TerminalBottom}, 0},
	{100, 310, true, -1, [3]model.TerminalDirection{"", model.TerminalBottom, model.TerminalBottom}, 0},
	{1920, 810, true, 0, [3]model.TerminalDirection{}, 2},
}

func checkTransformers(t *testing.T, name string, d *model.Diagram, report Report, wants []transformerWant) {
	t.Helper()
	if len(report.Failed) != 0 || len(d.Elements) != len(wants) {
		t.Fatalf("%s: report = %+v, elements = %d", name, report, len(d.Elements))
	}
	for i, w := range wants {
		el := d.Elements[i]
		var terminals [3]model.TerminalDirection
		tap := -1
		for k, wd := range el.Windings {
			if k < 3 {
				terminals[k] = wd.Terminal
			}
			if wd.TapChanger {
				tap = k
			}
		}
		if el.X != w.x || el.Y != w.y || el.Mirror != w.mirror || el.Scale != w.scale || terminals != w.terminals || tap != w.tap {
			t.Errorf("%s transformer %d = (%v,%v) mirror %v scale %d terminals %v tap %d, want %+v", name, i+1, el.X, el.Y, el.Mirror, el.Scale, terminals, tap, w)
		}
	}
}

// TestExtractTransformerSizeMirrorLeadsTap reads a transformer's size step, mirroring, lead sides and regulated
// winding from xsde2svg's current and older markup.
func TestExtractTransformerSizeMirrorLeadsTap(t *testing.T) {
	d, report, err := Extract([]byte(transformerSVG), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	checkTransformers(t, "current", d, report, transformerWants)

	d, report, err = Extract([]byte(transformerOldSVG), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	checkTransformers(t, "older", d, report, transformerWants[:2])
}

// TestTransformerRoundTrip renders transformers with a size step, mirroring, a lead side and a regulated winding, and
// reads them back from the static SVG.
func TestTransformerRoundTrip(t *testing.T) {
	d, _, err := Extract([]byte(transformerSVG), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := render.Render(d, render.NewSymbolLibrary(nil), &buf, render.Static, "", nil); err != nil {
		t.Fatal(err)
	}
	back, report, err := Extract(buf.Bytes(), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	checkTransformers(t, "round trip", back, report, transformerWants)
}

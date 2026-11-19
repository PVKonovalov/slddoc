package extract

import (
	"testing"
)

// sourceAttrsSVG is xsde2svg 1.4.28 markup: 1-6 capacitor banks (172) at size steps 1, -1, 3, -2, 2 and 5, 7 one
// with data-mirror (xsde2svg ignores it for 172), 8 and 9 shape 34 mirrored and not, 10 a mirrored power transformer
// at step -1.
const sourceAttrsSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="2500" height="2000">
<path d="M 150 849 l 0 15 l -19 33 m 19 -33 l 19 33 h -38 z M 150 887 h 7 m -7 0 h -7 M 150 892 h 7 m -7 0 h -7" style="fill:none;stroke:teal;stroke-width:1" transform="rotate(90,150,880)" id="1" data-voltage="teal" data-type="172" data-scale="1" />
<path d="M 250 865 l 0 7 l -9 16 m 9 -16 l 9 16 h -18 z M 250 883 h 3 m -3 0 h -3 M 250 886 h 3 m -3 0 h -3" style="fill:none;stroke:teal;stroke-width:1"  id="2" data-voltage="teal" data-type="172" data-scale="-1" />
<path d="M 350 818 l 0 31 l -39 67 m 39 -67 l 39 67 h -78 z M 350 894 h 14 m -14 0 h -14 M 350 905 h 14 m -14 0 h -14" style="fill:none;stroke:teal;stroke-width:1" transform="rotate(-180,350,880)" id="3" data-voltage="teal" data-type="172" data-scale="3" />
<path d="M 450 870 l 0 5 l -6 11 m 6 -11 l 6 11 h -12 z M 450 882 h 2 m -2 0 h -2 M 450 884 h 2 m -2 0 h -2" style="fill:none;stroke:teal;stroke-width:1" transform="rotate(270,450,880)" id="4" data-voltage="teal" data-type="172" data-scale="-2" data-mirror="1" />
<path d="M 550 836 l 0 22 l -28 48 m 28 -48 l 28 48 h -56 z M 550 890 h 10 m -10 0 h -10 M 550 898 h 10 m -10 0 h -10" style="fill:none;stroke:teal;stroke-width:1"  id="5" data-voltage="teal" data-type="172" data-scale="2" data-mirror="1" />
<path d="M 990 756 l 0 62 l -79 135 m 79 -135 l 79 135 h -158 z M 990 908 h 28 m -28 0 h -28 M 990 930 h 28 m -28 0 h -28" style="fill:none;stroke:teal;stroke-width:10" transform="rotate(90,990,880)" id="6" data-voltage="teal" data-type="172" data-scale="5" />
<path d="M 700 378 l 0 11 l -14 24 m 14 -24 l 14 24 h -28 z M 700 405 h 5 m -5 0 h -5 M 700 409 h 5 m -5 0 h -5" style="fill:none;stroke:teal;stroke-width:1"  id="7" data-voltage="teal" data-type="172" data-mirror="1" />
<path d="M 2038 1508 h -8 a 6 4 0 0 1 0 -8 a 6 4 0 0 1 0 -8 h 8 M 2030 1489 v 22" style="fill:none;stroke:purple;stroke-width:1" id="8" data-voltage="purple"  data-type="34" data-mirror="1"  />
<path d="M 792 352 h 8 a 6 4 0 0 1 0 8 a 6 4 0 0 1 0 8 h -8 M 800 349 v 22" style="fill:none;stroke:deepskyblue;stroke-width:1" id="9" data-voltage="deepskyblue" data-type="34" transform="rotate(-90,800,360)" />
<g id="10" data-type="47" data-scale="-1" data-mirror="1"  >
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
</svg>`

// TestExtractSourceScaleAndMirror reads Scale from data-scale and Mirror from data-mirror.
func TestExtractSourceScaleAndMirror(t *testing.T) {
	d, report, err := Extract([]byte(sourceAttrsSVG), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		scale  int
		mirror bool
	}{{1, false}, {-1, false}, {3, false}, {-2, false}, {2, false}, {5, false}, {0, false}, {0, true}, {0, false}, {-1, true}}
	if len(report.Failed) != 0 || len(d.Elements) != len(want) {
		t.Fatalf("report = %+v, elements = %d", report, len(d.Elements))
	}
	for i, w := range want {
		if el := d.Elements[i]; el.Scale != w.scale || el.Mirror != w.mirror {
			t.Errorf("element %d (%s) scale %d mirror %v, want %+v", i+1, el.Shape, el.Scale, el.Mirror, w)
		}
	}
	if el := d.Elements[9]; el.X != 100 || el.Y != 310 {
		t.Errorf("transformer at (%v,%v), want (100,310)", el.X, el.Y)
	}
}

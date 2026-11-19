package extract

import (
	"strings"
	"testing"
)

// chassisSVG is Chassis (51) as xsde2svg 1.4.28 draws them: 1 Normal, 2 Service (rotated), 3 Normal mirrored,
// 4 Service mirrored, 5 Service mirrored at size step 3.
const chassisSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="1200" height="1000">
<g id="1" data-voltage="teal" data-type="51"  >
<path d="M 150 378 l -4 4 m 4 -4 l 4 4  m -4 0 v 36 m 0 4 l -4 -4 m 4 4 l 4 -4" data-trolley="1" style="stroke:teal;stroke-width:1" />
<path d="M 150 430 l -4 -4 m 4 4 l 4 -4 M 150 370 l -4 4 m 4 -4 l 4 4" style="stroke:teal;stroke-width:1" />
</g>
<g id="2" data-voltage="teal" data-type="51" transform="rotate(90,250,400)" >
<path d="M 260 378 l -4 4 m 4 -4 l 4 4  m -4 0 v 36 m 0 4 l -4 -4 m 4 4 l 4 -4" data-trolley="0" style="stroke:teal;stroke-width:1" />
<path d="M 250 430 l -4 -4 m 4 4 l 4 -4 M 250 370 l -4 4 m 4 -4 l 4 4" style="stroke:teal;stroke-width:1" />
</g>
<g id="3" data-voltage="teal" data-type="51" data-mirror="1"  >
<path d="M 700 378 l -4 4 m 4 -4 l 4 4  m -4 0 v 36 m 0 4 l -4 -4 m 4 4 l 4 -4" data-trolley="1" style="stroke:teal;stroke-width:1" />
<path d="M 700 430 l -4 -4 m 4 4 l 4 -4 M 700 370 l -4 4 m 4 -4 l 4 4" style="stroke:teal;stroke-width:1" />
</g>
<g id="4" data-voltage="teal" data-type="51" data-mirror="1" transform="rotate(90,800,400)" >
<path d="M 790 378 l -4 4 m 4 -4 l 4 4  m -4 0 v 36 m 0 4 l -4 -4 m 4 4 l 4 -4" data-trolley="0" style="stroke:teal;stroke-width:1" />
<path d="M 800 430 l -4 -4 m 4 4 l 4 -4 M 800 370 l -4 4 m 4 -4 l 4 4" style="stroke:teal;stroke-width:1" />
</g>
<g id="5" data-voltage="teal" data-type="51" data-scale="3" data-mirror="1" transform="rotate(-90,550,900)" >
<path d="M 522 838 l -11 11 m 11 -11 l 11 11  m -11 0 v 100 m 0 11 l -11 -11 m 11 11 l 11 -11" data-trolley="0" style="stroke:teal;stroke-width:1" />
<path d="M 550 984 l -11 -11 m 11 11 l 11 -11 M 550 816 l -11 11 m 11 -11 l 11 11" style="stroke:teal;stroke-width:1" />
</g>
</svg>`

// TestExtractChassisPosition reads a chassis' racking position from data-trolley and its mirroring from data-mirror,
// or, in an older xsde2svg's markup without data-mirror, from the side a racked-out body is shifted to.
func TestExtractChassisPosition(t *testing.T) {
	type want struct {
		position int
		mirror   bool
	}
	current := []want{{1, false}, {0, false}, {1, true}, {0, true}, {0, true}}
	// Without data-mirror, a chassis at Normal shows no mirroring (nor does it draw any).
	older := []want{{1, false}, {0, false}, {1, false}, {0, true}, {0, true}}
	for _, tc := range []struct {
		name  string
		svg   string
		wants []want
	}{
		{"current", chassisSVG, current},
		{"older", strings.NewReplacer(` data-mirror="1"`, "", ` data-scale=`, ` data-x=`).Replace(chassisSVG), older},
	} {
		d, report, err := Extract([]byte(tc.svg), "", nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(report.Failed) != 0 || len(d.Elements) != len(tc.wants) {
			t.Fatalf("%s: report = %+v, elements = %d", tc.name, report, len(d.Elements))
		}
		for i, w := range tc.wants {
			el := d.Elements[i]
			if el.Position == nil || *el.Position != w.position || el.Mirror != w.mirror {
				t.Errorf("%s chassis %d: position %v mirror %v, want %+v", tc.name, i+1, el.Position, el.Mirror, w)
			}
		}
		if el := d.Elements[4]; tc.name == "current" && el.Scale != 3 {
			t.Errorf("%s chassis 5: scale %d, want 3", tc.name, el.Scale)
		}
	}
}

package extract

import (
	"testing"

	"github.com/PVKonovalov/slddoc/internal/model"
)

// fuseFlatSVG is withdrawable fuses (154) as an older xsde2svg draws them: one <path>, no data-trolley. In order:
// default, Service, Test, Service rotated -45, Test rotated 180, Service at scale 3, Test at scale -2 rotated 90.
const fuseFlatSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="1100" height="1100">
<path d="M 260 38 v 12 h 5 v 20 h -5 v -20 h -5 v 20 h 5 v 12 M 260 29 l -4 4 m 4 -4 l 4 4 M 260 91 l -4 -4 m 4 4 l 4 -4 M 260 38 l -4 4 m 4 -4 l 4 4 M 260 82 l -4 -4 m 4 4 l 4 -4" style="fill:none;stroke:teal;stroke-width:1"  id="1" data-voltage="teal" data-type="154" />
<path d="M 310 38 v 12 h 5 v 20 h -5 v -20 h -5 v 20 h 5 v 12 M 300 29 l -4 4 m 4 -4 l 4 4 M 300 91 l -4 -4 m 4 4 l 4 -4 M 310 38 l -4 4 m 4 -4 l 4 4 M 310 82 l -4 -4 m 4 4 l 4 -4" style="fill:none;stroke:teal;stroke-width:1"  id="2" data-voltage="teal" data-type="154" />
<path d="M 150 378 v 12 h 5 v 20 h -5 v -20 h -5 v 20 h 5 v 12 M 150 369 l -4 4 m 4 -4 l 4 4 M 150 431 l -4 -4 m 4 4 l 4 -4 M 150 378 l -4 -4 m 4 4 l 4 -4 M 150 422 l -4 4 m 4 -4 l 4 4" style="fill:none;stroke:teal;stroke-width:1"  id="3" data-voltage="teal" data-type="154" />
<path d="M 690 138 v 12 h 5 v 20 h -5 v -20 h -5 v 20 h 5 v 12 M 680 129 l -4 4 m 4 -4 l 4 4 M 680 191 l -4 -4 m 4 4 l 4 -4 M 690 138 l -4 4 m 4 -4 l 4 4 M 690 182 l -4 -4 m 4 4 l 4 -4" style="fill:none;stroke:teal;stroke-width:1" transform="rotate(-45,680,160)" id="4" data-voltage="teal" data-type="154" />
<path d="M 150 578 v 12 h 5 v 20 h -5 v -20 h -5 v 20 h 5 v 12 M 150 569 l -4 4 m 4 -4 l 4 4 M 150 631 l -4 -4 m 4 4 l 4 -4 M 150 578 l -4 -4 m 4 4 l 4 -4 M 150 622 l -4 4 m 4 -4 l 4 4" style="fill:none;stroke:teal;stroke-width:1" transform="rotate(180,150,600)" id="5" data-voltage="teal" data-type="154" />
<path d="M 378 838 v 33 h 14 v 56 h -14 v -56 h -14 v 56 h 14 v 33 M 350 813 l -11 11 m 11 -11 l 11 11 M 350 987 l -11 -11 m 11 11 l 11 -11 M 378 838 l -11 11 m 11 -11 l 11 11 M 378 962 l -11 -11 m 11 11 l 11 -11" style="fill:none;stroke:teal;stroke-width:1"  id="6" data-voltage="teal" data-type="154" />
<path d="M 450 890 v 5 h 2 v 8 h -2 v -8 h -2 v 8 h 2 v 5 M 450 885 l -1 1 m 1 -1 l 1 1 M 450 915 l -1 -1 m 1 1 l 1 -1 M 450 890 l -1 -1 m 1 1 l 1 -1 M 450 910 l -1 1 m 1 -1 l 1 1" style="fill:none;stroke:teal;stroke-width:1" transform="rotate(90,450,900)" id="7" data-voltage="teal" data-type="154" />
</svg>`

// fuseGroupSVG is the same fuses as xsde2svg draws them now: the moving part in a <g data-trolley>.
const fuseGroupSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="1100" height="1100">
<g id="1" data-voltage="teal"  data-type="154"  >
<g  data-trolley="1" >
<path d="M 260 38 v 12 h 5 v 20 h -5 v -20 h -5 v 20 h 5 v 12 M 260 38 l -4 4 m 4 -4 l 4 4 M 260 82 l -4 -4 m 4 4 l 4 -4" style="fill:none;stroke:teal;stroke-width:1" />
</g>
<path d="M 260 29 l -4 4 m 4 -4 l 4 4 M 260 91 l -4 -4 m 4 4 l 4 -4" style="fill:none;stroke:teal;stroke-width:1" />
</g>
<g id="2" data-voltage="teal"  data-type="154"  >
<g transform="translate(10)" data-trolley="0" >
<path d="M 300 38 v 12 h 5 v 20 h -5 v -20 h -5 v 20 h 5 v 12 M 300 38 l -4 4 m 4 -4 l 4 4 M 300 82 l -4 -4 m 4 4 l 4 -4" style="fill:none;stroke:teal;stroke-width:1" />
</g>
<path d="M 300 29 l -4 4 m 4 -4 l 4 4 M 300 91 l -4 -4 m 4 4 l 4 -4" style="fill:none;stroke:teal;stroke-width:1" />
</g>
<g id="3" data-voltage="teal"  data-type="154"  >
<g  data-trolley="2" >
<path d="M 150 378 v 12 h 5 v 20 h -5 v -20 h -5 v 20 h 5 v 12 M 150 378 l -4 -4 m 4 4 l 4 -4 M 150 422 l -4 4 m 4 -4 l 4 4" style="fill:none;stroke:teal;stroke-width:1" />
</g>
<path d="M 150 369 l -4 4 m 4 -4 l 4 4 M 150 431 l -4 -4 m 4 4 l 4 -4" style="fill:none;stroke:teal;stroke-width:1" />
</g>
<g id="4" data-voltage="teal"  data-type="154" transform="rotate(-45,680,160)" >
<g transform="translate(10)" data-trolley="0" >
<path d="M 680 138 v 12 h 5 v 20 h -5 v -20 h -5 v 20 h 5 v 12 M 680 138 l -4 4 m 4 -4 l 4 4 M 680 182 l -4 -4 m 4 4 l 4 -4" style="fill:none;stroke:teal;stroke-width:1" />
</g>
<path d="M 680 129 l -4 4 m 4 -4 l 4 4 M 680 191 l -4 -4 m 4 4 l 4 -4" style="fill:none;stroke:teal;stroke-width:1" />
</g>
<g id="5" data-voltage="teal"  data-type="154" transform="rotate(180,150,600)" >
<g  data-trolley="2" >
<path d="M 150 578 v 12 h 5 v 20 h -5 v -20 h -5 v 20 h 5 v 12 M 150 578 l -4 -4 m 4 4 l 4 -4 M 150 622 l -4 4 m 4 -4 l 4 4" style="fill:none;stroke:teal;stroke-width:1" />
</g>
<path d="M 150 569 l -4 4 m 4 -4 l 4 4 M 150 631 l -4 -4 m 4 4 l 4 -4" style="fill:none;stroke:teal;stroke-width:1" />
</g>
<g id="6" data-voltage="teal"  data-type="154"  >
<g transform="translate(28)" data-trolley="0" >
<path d="M 350 838 v 33 h 14 v 56 h -14 v -56 h -14 v 56 h 14 v 33 M 350 838 l -11 11 m 11 -11 l 11 11 M 350 962 l -11 -11 m 11 11 l 11 -11" style="fill:none;stroke:teal;stroke-width:1" />
</g>
<path d="M 350 813 l -11 11 m 11 -11 l 11 11 M 350 987 l -11 -11 m 11 11 l 11 -11" style="fill:none;stroke:teal;stroke-width:1" />
</g>
<g id="7" data-voltage="teal"  data-type="154" transform="rotate(90,450,900)" >
<g  data-trolley="2" >
<path d="M 450 890 v 5 h 2 v 8 h -2 v -8 h -2 v 8 h 2 v 5 M 450 890 l -1 -1 m 1 1 l 1 -1 M 450 910 l -1 1 m 1 -1 l 1 1" style="fill:none;stroke:teal;stroke-width:1" />
</g>
<path d="M 450 885 l -1 1 m 1 -1 l 1 1 M 450 915 l -1 -1 m 1 1 l 1 -1" style="fill:none;stroke:teal;stroke-width:1" />
</g>
</svg>`

type fuseWant struct {
	x, y          float64
	orient, scale int
	position      int // -1: none
}

var fuseWants = []fuseWant{
	{260, 60, 0, 0, -1},
	{300, 60, 0, 0, 0},
	{150, 400, 0, 0, 2},
	{680, 160, 315, 0, 0},
	{150, 600, 180, 0, 2},
	{350, 900, 0, 3, 0},
	{450, 900, 90, -2, 2},
}

// TestExtractWithdrawableFuse reads a withdrawable fuse's anchor, orientation, scale and trolley position from both
// xsde2svg markups.
func TestExtractWithdrawableFuse(t *testing.T) {
	for _, tc := range []struct {
		name, svg   string
		defaultedTo int
	}{{"flat", fuseFlatSVG, -1}, {"group", fuseGroupSVG, 1}} {
		d, report, err := Extract([]byte(tc.svg), "", nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(report.Failed) != 0 || len(d.Elements) != len(fuseWants) {
			t.Fatalf("%s: report = %+v, elements = %d", tc.name, report, len(d.Elements))
		}
		for i, w := range fuseWants {
			el := d.Elements[i]
			if w.position == -1 {
				w.position = tc.defaultedTo
			}
			pos := -1
			if el.Position != nil {
				pos = *el.Position
			}
			if el.Class != model.ClassFuse || el.X != w.x || el.Y != w.y || el.Orient != w.orient || el.Scale != w.scale || pos != w.position {
				t.Errorf("%s fuse %d = (%v,%v) orient %d scale %d position %d, want %+v", tc.name, i+1, el.X, el.Y, el.Orient, el.Scale, pos, w)
			}
		}
	}
}

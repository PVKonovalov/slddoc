package slddoc

import (
	"bytes"
	"strings"
	"testing"
)

func TestAbsolutize(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{
			"rotated symbol",
			`<g id="1" data-type="41" transform="translate(10,20) rotate(90)">
<path d="M -7 -7 h 14 v 14 h -14 z" />
<circle cx="1" cy="2" r="3" />
</g>
`,
			`<g id="1" data-type="41" transform="rotate(90,10,20)">
<path d="M 3 13 h 14 v 14 h -14 z" />
<circle cx="11" cy="22" r="3" />
</g>
`,
		},
		{
			"unrotated symbol loses its transform",
			`<g id="1" transform="translate(10,20) rotate(0)"><rect x="-1" y="-2" width="2" height="4" /></g>`,
			`<g id="1"><rect x="9" y="18" width="2" height="4" /></g>`,
		},
		{
			"mirror flips about the anchor",
			`<g id="1" transform="translate(10,20) rotate(90) scale(-1,1)"><line x1="0" y1="0" x2="5" y2="-5" /></g>`,
			`<g id="1" transform="rotate(90,10,20) translate(20,0) scale(-1,1)"><line x1="10" y1="20" x2="15" y2="15" /></g>`,
		},
		{
			"nested transforms keep their effect",
			`<g id="1" transform="translate(10,20) rotate(90)">
<g transform="translate(0 21) rotate(-90)"><text x="0" y="5">A</text></g>
<g transform="translate(0,0)"><path d="M 0 0 v 9" /></g>
<g transform="rotate(45,1,2)"><polyline points="0,0 1,1" /></g>
</g>`,
			`<g id="1" transform="rotate(90,10,20)">
<g transform="translate(0,21) rotate(-90,10,20)"><text x="10" y="25">A</text></g>
<g><path d="M 10 20 v 9" /></g>
<g transform="rotate(45,11,22)"><polyline points="10,20 11,21" /></g>
</g>`,
		},
		{
			"self-closing placed path (an arrowhead)",
			`<path d="M 7 0 l -7 12 l -7 -12 z" style="fill:none" transform="translate(1980,252) rotate(180)" />`,
			`<path d="M 1987 252 l -7 12 l -7 -12 z" style="fill:none" transform="rotate(180,1980,252)" />`,
		},
		{
			"path commands: leading m, implicit pairs, arcs, H/V, relative curves",
			`<g transform="translate(10,20)"><path d="m 1 2 3 4 A 5 5 0 1 1 6 7 V 8 H 9 c 1 1 1 1 1 1 Q 0 0 1 1 Z" /></g>`,
			`<g><path d="m 11 22 3 4 A 5 5 0 1 1 16 27 V 28 H 19 c 1 1 1 1 1 1 Q 10 20 11 21 Z" /></g>`,
		},
		{
			"unplaced markup is untouched",
			`<polyline points="0,0 5,0" data-type="21" id="3" />
<text x="1" y="2">caption</text>`,
			`<polyline points="0,0 5,0" data-type="21" id="3" />
<text x="1" y="2">caption</text>`,
		},
	}
	for _, c := range cases {
		got, err := absolutize(c.in)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got, c.want)
		}
	}
}

func TestAbsolutize_UnsupportedFormKeepsFragment(t *testing.T) {
	for _, in := range []string{
		`<g transform="translate(10,20) rotate(90)"><g transform="skewX(10)"><path d="M 0 0" /></g></g>`,
		`<g transform="translate(10,20)"><path d='M 0 0' /></g>`,
	} {
		got, err := absolutize(in)
		if err == nil {
			t.Errorf("%s: expected an error", in)
		}
		if got != in {
			t.Errorf("%s: fragment changed to %s", in, got)
		}
	}
}

// TestRender_StaticIsAbsoluteInteractiveIsLocal checks the split: the saved
// document uses xsde2svg's absolute coordinates, the live canvas keeps the
// translate placement it drags by.
func TestRender_StaticIsAbsoluteInteractiveIsLocal(t *testing.T) {
	lib := NewSymbolLibrary(map[string]string{
		"41": `<path d="M -7 -7 h 14 v 14 h -14 z" style="stroke:{color}" />`,
	})
	state := 1
	d := &Diagram{
		Width: 200, Height: 200,
		VoltageClasses: []VoltageClass{{ID: 1, Name: "10kV", Color: "#962896"}},
		Elements: []Element{
			{ID: 1, Class: ClassBreaker, Shape: "41", Voltage: 1, X: 100, Y: 50, Orient: 90, State: &state},
			{ID: 2, Class: ClassPowerTransformer, Shape: "47", X: 100, Y: 150,
				Windings: []TransformerWinding{{Voltage: 1}, {Voltage: 1}}},
		},
		Connectors: []Connector{{ID: 3, Kind: KindBusWork, Voltage: 1, Name: "W1", Points: []Point{{X: 0, Y: 0}, {X: 10, Y: 0}}}},
	}

	var static, live bytes.Buffer
	if err := Render(d, lib, &static, Static, "", nil); err != nil {
		t.Fatal(err)
	}
	if err := Render(d, lib, &live, Interactive, "", nil); err != nil {
		t.Fatal(err)
	}
	s, l := static.String(), live.String()
	for _, want := range []string{
		`data-type="41" transform="rotate(90,100,50)">`,
		`<path d="M 93 43 h 14 v 14 h -14 z"`,
		`data-type="47">`, // unrotated: no transform at all
		`<circle cx="118" cy="150" r="22"`,
		`<polyline points="0,0 10,0" style="fill:none;stroke:#962896;stroke-width:1" data-name="W1" data-type="21" data-voltage="#962896" id="3" />`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("Static missing %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "translate(") {
		t.Errorf("Static still has a translate:\n%s", s)
	}
	for _, want := range []string{`transform="translate(100,50) rotate(90)"`, `transform="translate(100,150) rotate(0)"`, `<path d="M -7 -7 h 14 v 14 h -14 z"`} {
		if !strings.Contains(l, want) {
			t.Errorf("Interactive missing %q:\n%s", want, l)
		}
	}
}

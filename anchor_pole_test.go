package slddoc

import (
	"bytes"
	"fmt"
	"testing"
)

// TestParseAnchorPole uses real xsde2svg markup for shape 19 (ctrlroom
// corpus): bare paths at three export sizes, rotated or not, with the apex
// on either side (xMirror), one carrying a data-name.
func TestParseAnchorPole(t *testing.T) {
	cases := []struct {
		name       string
		svg        string
		wantX      float64
		wantY      float64
		wantOrient int
		wantMirror bool
		wantName   string
	}{
		{"rotated -90, size 14 (corpus id 148704423)", `<path d="M 1074 1325 l -28 14 v -28 z" style="fill:none;stroke:gray;stroke-width:1" transform="rotate(-90,1060,1325)" id="148704423" data-type="19"  data-voltage="gray" />`,
			1060, 1325, -90, false, ""},
		{"unrotated, size 10 (corpus id 1)", `<path d="M 901 483 l -20 10 v -20 z" style="fill:none;stroke:gray;stroke-width:1"  id="1" data-type="19"  data-voltage="gray" />`,
			891, 483, 0, false, ""},
		{"apex left, rotated 90, size 3 (corpus id 36543)", `<path d="M 1717 380 l 6 3 v -6 z" style="fill:none;stroke:deepskyblue;stroke-width:1" transform="rotate(90,1720,380)" id="36543" data-type="19"  data-voltage="deepskyblue" />`,
			1720, 380, 90, true, ""},
		{"named (corpus id 164)", `<path d="M 1173 820 l -6 3 v -6 z" style="fill:none;stroke:#12161d;stroke-width:1"  id="164" data-type="19" data-name="оп. 2" data-voltage="#12161d" />`,
			1170, 820, 0, false, "оп. 2"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			el, ports, voltage, err := parseAnchorPole(parseFirst(t, c.svg))
			if err != nil {
				t.Fatal(err)
			}
			if el.Class != ClassAnchorPole || el.Shape != "19" || voltage == "" || el.Name != c.wantName {
				t.Errorf("class/shape/voltage/name = %s/%s/%q/%q", el.Class, el.Shape, voltage, el.Name)
			}
			if el.X != c.wantX || el.Y != c.wantY || el.Orient != c.wantOrient || el.Mirror != c.wantMirror {
				t.Errorf("got (%v,%v) orient %d mirror %v, want (%v,%v) %d %v", el.X, el.Y, el.Orient, el.Mirror, c.wantX, c.wantY, c.wantOrient, c.wantMirror)
			}
			if len(ports) != 1 || ports[0] != (Point{c.wantX, c.wantY}) {
				t.Errorf("ports = %v", ports)
			}
		})
	}
}

// TestAnchorPole_RoundTrip renders the editor's own template, mirrored and
// rotated, and reads it back.
func TestAnchorPole_RoundTrip(t *testing.T) {
	lib := NewSymbolLibrary(map[string]string{"19": `<path d="M 10 0 l -20 10 v -20 z" style="fill:none;stroke:{color};stroke-width:1" />`})
	for _, c := range []struct {
		orient int
		mirror bool
	}{{0, false}, {-90, false}, {0, true}, {90, true}} {
		t.Run(fmt.Sprint(c), func(t *testing.T) {
			d := &Diagram{Width: 400, Height: 400, Elements: []Element{{ID: 2, Class: ClassAnchorPole, Shape: "19", X: 100, Y: 200, Orient: c.orient, Mirror: c.mirror}}}
			var buf bytes.Buffer
			if err := Render(d, lib, &buf, Static, "", nil); err != nil {
				t.Fatal(err)
			}
			back, report, err := Extract(buf.Bytes(), "", nil)
			if err != nil || len(report.Failed) != 0 || len(back.Elements) != 1 {
				t.Fatalf("extract: %v %+v\n%s", err, report, buf.String())
			}
			g := back.Elements[0]
			if g.X != 100 || g.Y != 200 || g.Orient != c.orient || g.Mirror != c.mirror {
				t.Errorf("extracted %+v\n%s", g, buf.String())
			}
		})
	}
}

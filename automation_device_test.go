package slddoc

import (
	"bytes"
	"strings"
	"testing"
)

// TestParseAutomationDevice has no real corpus instance to use (none
// exist), so its markup is element_103.go's own output format: a <g> with a
// rect and a centered text, no data-state. It reads back as Off.
func TestParseAutomationDevice(t *testing.T) {
	const svg = `<g id="77" data-type="103" data-voltage="#00ff00" >
<rect x="100" y="50" width="60" height="20" style="fill:#00ff00;stroke:black;stroke-width:1" />
<text x="130" y="62" style="fill:#000000;text-anchor:middle;dominant-baseline:middle;font-size:20px;font-family:Arial ;font-weight: bold" >ATS</text>
</g>`
	el, err := parseAutomationDevice(parseFirst(t, svg))
	if err != nil {
		t.Fatal(err)
	}
	if el.Class != ClassAutomationDevice || el.Shape != "103" || el.State != nil {
		t.Errorf("class/shape/state = %s/%s/%v", el.Class, el.Shape, el.State)
	}
	if len(el.Points) != 2 || el.Points[0] != (Point{100, 50}) || el.Points[1] != (Point{160, 70}) || el.X != 130 || el.Y != 60 {
		t.Errorf("points/anchor = %v (%v,%v)", el.Points, el.X, el.Y)
	}
	if el.FillOff != "#00ff00" || el.PropertyText != "ATS" || el.TextColor != "#000000" || el.TextSize != 20 || !el.Bold || el.Stroke != "black" || el.Fill != "" {
		t.Errorf("off look = %+v", el)
	}
}

// TestAutomationDevice_RoundTrip renders a tile in each state and reads it
// back: the shown state's look and the state itself survive (the other
// state's look isn't in the SVG at all, as in the source).
func TestAutomationDevice_RoundTrip(t *testing.T) {
	for _, state := range []int{0, 1} {
		s := state
		el := Element{
			ID: 9, Class: ClassAutomationDevice, Shape: "103", Name: "ATS-1", State: &s,
			Points:  []Point{{10, 10}, {70, 30}},
			FillOff: "#808080", PropertyText: "off", TextColor: "#ffffff",
			FillOn: "#00aa00", PropertyTextOn: "on", TextColorOn: "#000000",
		}
		d := &Diagram{Width: 100, Height: 100, Elements: []Element{el}}
		var buf bytes.Buffer
		if err := Render(d, NewSymbolLibrary(nil), &buf, Static, "", nil); err != nil {
			t.Fatal(err)
		}
		out := buf.String()
		wantText := map[int]string{0: ">off</text>", 1: ">on</text>"}[s]
		if !strings.Contains(out, wantText) {
			t.Errorf("state %d: want %s in\n%s", s, wantText, out)
		}
		back, report, err := Extract(buf.Bytes(), "", nil)
		if err != nil || len(report.Failed) != 0 || len(back.Elements) != 1 {
			t.Fatalf("state %d extract: %v %+v", s, err, report)
		}
		g := back.Elements[0]
		if g.State == nil || *g.State != s || g.Name != "ATS-1" {
			t.Fatalf("state %d: extracted %+v", s, g)
		}
		if s == 0 && (g.FillOff != "#808080" || g.PropertyText != "off" || g.TextColor != "#ffffff") {
			t.Errorf("off look = %+v", g)
		}
		if s == 1 && (g.FillOn != "#00aa00" || g.PropertyTextOn != "on" || g.TextColorOn != "#000000") {
			t.Errorf("on look = %+v", g)
		}
	}
}

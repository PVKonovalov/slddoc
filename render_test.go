package slddoc

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"
	"testing"
)

const testSymbols = `<symbols>
  <symbol shape="41">
    <template><![CDATA[
<path d="M -7 -7 h 14 v 14 h -14 z" style="fill:{fill};stroke:{color};stroke-width:1" />
]]></template>
  </symbol>
</symbols>`

func TestLoadSymbolLibrary(t *testing.T) {
	lib, err := LoadSymbolLibrary(strings.NewReader(testSymbols))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := lib.templates["41"]; !ok {
		t.Fatalf("templates = %+v, want shape 41", lib.templates)
	}
}

func TestRender_ProducesWellFormedSVG(t *testing.T) {
	lib, err := LoadSymbolLibrary(strings.NewReader(testSymbols))
	if err != nil {
		t.Fatal(err)
	}

	state := 1
	d := &Diagram{
		Width: 100, Height: 100,
		VoltageClasses: []VoltageClass{{ID: 1, Name: "10кВ", Color: "#962896"}},
		Elements: []Element{
			{ID: 1, Class: ClassBusBarSection, Voltage: 1, Points: []Point{{0, 0}, {100, 0}}},
			{ID: 2, Class: ClassBreaker, Shape: "41", Name: "В-1", Voltage: 1, X: 50, Y: 50, State: &state},
		},
		Connectors: []Connector{
			{ID: 1, Voltage: 1, Points: []Point{{50, 0}, {50, 43}}},
		},
		Labels: []Label{{X: 10, Y: 10, Size: 13, Text: "line one\nline two"}},
	}

	var buf bytes.Buffer
	if err := Render(d, lib, &buf); err != nil {
		t.Fatal(err)
	}

	var probe struct {
		XMLName xml.Name `xml:"svg"`
	}
	if err := xml.Unmarshal(buf.Bytes(), &probe); err != nil {
		t.Fatalf("rendered output is not well-formed XML: %v\n%s", err, buf.String())
	}

	out := buf.String()
	if !strings.Contains(out, `stroke:#962896`) {
		t.Errorf("voltage color not applied: %s", out)
	}
	if !strings.Contains(out, `fill:lawngreen`) {
		t.Errorf("state 1 should render lawngreen fill: %s", out)
	}
	if !strings.Contains(out, `translate(50,50) rotate(0)`) {
		t.Errorf("element not placed at its anchor: %s", out)
	}
	if !strings.Contains(out, "<tspan") {
		t.Errorf("multi-line label should emit a tspan: %s", out)
	}
}

func TestApplyStateLine(t *testing.T) {
	tmpl := `<path d="{state:PARALLEL|PERPENDICULAR|DIAGONAL}" />`
	one, zero, two, none := 1, 0, 2, (*int)(nil)

	cases := []struct {
		name  string
		state *int
		want  string
	}{
		{"closed", &one, "PARALLEL"},
		{"open", &zero, "PERPENDICULAR"},
		{"undefined", &two, "DIAGONAL"},
		{"unrecorded defaults to parallel", none, "PARALLEL"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := applyStateLine(tmpl, c.state)
			if !strings.Contains(got, c.want) {
				t.Errorf("applyStateLine(%v) = %q, want it to contain %q", c.state, got, c.want)
			}
		})
	}
}

func TestRender_AnnotatesTypeGroups(t *testing.T) {
	lib, err := LoadSymbolLibrary(strings.NewReader(testSymbols))
	if err != nil {
		t.Fatal(err)
	}
	d := &Diagram{
		Width: 100, Height: 100,
		Elements: []Element{
			{ID: 1, Class: ClassBreaker, Shape: "41", X: 1, Y: 1},
			{ID: 2, Class: ClassBreaker, Shape: "41", X: 2, Y: 2},
			{ID: 3, Class: ClassLamp, Shape: "106", X: 3, Y: 3, Radius: 8},
		},
		Connectors: []Connector{
			{ID: 1, Kind: KindOverheadLine, Points: []Point{{0, 0}, {1, 1}}},
			{ID: 2, Kind: KindOverheadLine, Points: []Point{{1, 1}, {2, 2}}},
			{ID: 3, Kind: KindCableLine, Points: []Point{{2, 2}, {3, 3}}},
		},
	}
	lib.templates["106"] = `<circle r="{radius}" style="fill:{color}" />`

	var buf bytes.Buffer
	if err := Render(d, lib, &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	if n := strings.Count(out, "<!-- Breaker:41 -->"); n != 1 {
		t.Errorf("want exactly one Breaker:41 header for the two consecutive breakers, got %d:\n%s", n, out)
	}
	if !strings.Contains(out, "<!-- Lamp:106 -->") {
		t.Errorf("missing Lamp:106 header: %s", out)
	}
	if n := strings.Count(out, "<!-- Overhead line -->"); n != 1 {
		t.Errorf("want exactly one Overhead line header for the two consecutive connectors, got %d:\n%s", n, out)
	}
	if !strings.Contains(out, "<!-- Cable line -->") {
		t.Errorf("missing Cable line header: %s", out)
	}
	if !strings.Contains(out, `r="8"`) {
		t.Errorf("lamp radius placeholder not substituted: %s", out)
	}
}

func TestRender_ElevatedClassesDrawnAfterConnectors(t *testing.T) {
	lib, err := LoadSymbolLibrary(strings.NewReader(testSymbols))
	if err != nil {
		t.Fatal(err)
	}
	lib.templates["7"] = `<circle r="3" style="stroke:{color}" />`
	lib.templates["106"] = `<circle r="{radius}" style="fill:{color}" />`
	lib.templates["320003"] = `<circle r="{radius}" style="fill:{color}" />`
	d := &Diagram{
		Width: 100, Height: 100,
		// Every elevated element is listed first in document order (as real
		// corpus SVGs place standalone point/status symbols wherever they
		// fall in the source), but each must still render after the
		// connectors loop, so it paints on top of any wire it sits on
		// instead of a later-painted wire cutting through it.
		Elements: []Element{
			{ID: 1, Class: ClassJunctionPoint, Shape: "7", X: 0, Y: 0},
			{ID: 2, Class: ClassLamp, Shape: "106", X: 1, Y: 1, Radius: 8},
			{ID: 3, Class: ClassFaultPassageIndicator, Shape: "320003", X: 2, Y: 2, Radius: 15},
			{ID: 4, Class: ClassBreaker, Shape: "41", X: 3, Y: 3},
		},
		Connectors: []Connector{
			{ID: 1, Kind: KindOverheadLine, Points: []Point{{0, 0}, {2, 2}}},
		},
	}

	var buf bytes.Buffer
	if err := Render(d, lib, &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	wireIdx := strings.Index(out, `points="0,0 2,2"`)
	breakerIdx := strings.Index(out, `id="4"`)
	if wireIdx == -1 || breakerIdx == -1 {
		t.Fatalf("missing expected elements in output: %s", out)
	}
	if breakerIdx > wireIdx {
		t.Errorf("ordinary breaker rendered after the wire (index %d > %d), want before, in document order: %s", breakerIdx, wireIdx, out)
	}
	for _, id := range []int{1, 2, 3} {
		idx := strings.Index(out, fmt.Sprintf(`id="%d"`, id))
		if idx == -1 {
			t.Fatalf("missing element %d in output: %s", id, out)
		}
		if idx < wireIdx {
			t.Errorf("elevated element %d rendered before its wire (index %d < %d), want after so it paints on top: %s", id, idx, wireIdx, out)
		}
	}
}

func TestRender_ReportsMissingShape(t *testing.T) {
	lib, err := LoadSymbolLibrary(strings.NewReader(`<symbols></symbols>`))
	if err != nil {
		t.Fatal(err)
	}
	d := &Diagram{
		Elements: []Element{{ID: 1, Class: ClassBreaker, Shape: "41", X: 1, Y: 1}},
	}
	var buf bytes.Buffer
	err = Render(d, lib, &buf)
	if err == nil {
		t.Fatal("expected an error for a missing shape")
	}
	if !strings.Contains(err.Error(), "41") {
		t.Errorf("error should name the missing shape: %v", err)
	}
	// The rest of the document must still be written.
	if !strings.Contains(buf.String(), "</svg>") {
		t.Errorf("output should still be a complete document: %s", buf.String())
	}
}

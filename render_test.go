package slddoc

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"math"
	"regexp"
	"strconv"
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

func TestNewSymbolLibrary(t *testing.T) {
	lib := NewSymbolLibrary(map[string]string{"41": "<path/>"})
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
		VoltageClasses: []VoltageClass{{ID: 1, Name: "10kV", Color: "#962896"}},
		Elements: []Element{
			{ID: 1, Class: ClassBusBarSection, Voltage: 1, Points: []Point{{X: 0, Y: 0}, {X: 100, Y: 0}}},
			{ID: 2, Class: ClassBreaker, Shape: "41", Name: "CB-1", Voltage: 1, X: 50, Y: 50, State: &state},
		},
		Connectors: []Connector{
			{ID: 3, Points: []Point{{X: 50, Y: 0}, {X: 50, Y: 43}}},
		},
		Labels: []Label{{X: 10, Y: 10, Size: 13, Text: "line one\nline two"}},
	}

	stateColors := []StateColor{
		{State: 0, Label: "Open", Color: "red"},
		{State: 1, Label: "Close", Color: "lawngreen"},
		{State: 2, Label: "Intermediate", Color: "yellow"},
	}

	var buf bytes.Buffer
	if err := Render(d, lib, &buf, Static, nil, stateColors...); err != nil {
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

func TestRender_DigitalDeviceMatchesXsde2svgFormat(t *testing.T) {
	lib := NewSymbolLibrary(map[string]string{})
	d := &Diagram{
		Width: 100, Height: 100,
		DigitalDevices: []DigitalDevice{
			{
				ID:     1,
				X:      1876,
				Y:      759,
				Size:   16,
				Anchor: "end",
				Bold:   true,
				Color:  "darkturquoise",
				VAlign: "middle",
				Name:   "R T-1 10",
				Value:  "0.00",
				Unit:   "MW",
			},
		},
	}

	var buf bytes.Buffer
	if err := Render(d, lib, &buf, Interactive, nil); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	for _, want := range []string{
		`data-type="134"`,
		`x="1876"`,
		`y="759"`,
		`id="1"`,
		`data-name="R T-1 10"`,
		`data-unit="MW"`,
		`data-editor-kind="digitaldevice"`,
		`fill:darkturquoise`,
		`text-anchor:end`,
		`dominant-baseline:middle`,
		`font-weight: bold`,
		`>0.00 <tspan`,
		`</tspan></text>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered digital device missing %q: %s", want, out)
		}
	}
	if strings.Contains(out, "<g") {
		t.Errorf("a digital device should be a bare <text>, not wrapped in a <g>: %s", out)
	}
}

func TestRender_DigitalDeviceOmitsUnitWhenEmpty(t *testing.T) {
	lib := NewSymbolLibrary(map[string]string{})
	d := &Diagram{
		Width: 100, Height: 100,
		DigitalDevices: []DigitalDevice{{ID: 1, X: 5, Y: 5, Size: 10, Value: "0.00"}},
	}

	var buf bytes.Buffer
	if err := Render(d, lib, &buf, Static, nil); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(out, "data-unit") || strings.Contains(out, "<tspan") {
		t.Errorf("an empty unit should omit both data-unit and the unit tspan entirely: %s", out)
	}
}

func TestRender_UsesEditorBackground(t *testing.T) {
	lib := NewSymbolLibrary(map[string]string{})
	d := &Diagram{Width: 10, Height: 10, Editor: &EditorSettings{Background: "#ffffff"}}

	var buf bytes.Buffer
	if err := Render(d, lib, &buf, Static, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "background-color: #ffffff") {
		t.Errorf("editor background not applied: %s", buf.String())
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
			{ID: 4, Kind: KindOverheadLine, Points: []Point{{X: 0, Y: 0}, {X: 1, Y: 1}}},
			{ID: 5, Kind: KindOverheadLine, Points: []Point{{X: 1, Y: 1}, {X: 2, Y: 2}}},
			{ID: 6, Kind: KindCableLine, Points: []Point{{X: 2, Y: 2}, {X: 3, Y: 3}}},
		},
	}
	lib.templates["106"] = `<circle r="{radius}" style="fill:{color}" />`

	var buf bytes.Buffer
	if err := Render(d, lib, &buf, Static, nil); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	if n := strings.Count(out, "<!-- Breaker:41 -->"); n != 1 {
		t.Errorf("want exactly one Breaker:41 header for the two consecutive breakers, got %d:\n%s", n, out)
	}
	if !strings.Contains(out, "<!-- Lamp:106 -->") {
		t.Errorf("missing Lamp:106 header: %s", out)
	}
	if n := strings.Count(out, "<!-- Overhead line:22 -->"); n != 1 {
		t.Errorf("want exactly one Overhead line:22 header for the two consecutive connectors, got %d:\n%s", n, out)
	}
	if !strings.Contains(out, "<!-- Cable line:23 -->") {
		t.Errorf("missing Cable line:23 header: %s", out)
	}
	if !strings.Contains(out, `r="8"`) {
		t.Errorf("lamp radius placeholder not substituted: %s", out)
	}
}

func TestRender_CableLineIsDashedByDefaultOverheadLineIsNot(t *testing.T) {
	lib, err := LoadSymbolLibrary(strings.NewReader(testSymbols))
	if err != nil {
		t.Fatal(err)
	}
	d := &Diagram{
		Width: 100, Height: 100,
		Connectors: []Connector{
			{ID: 1, Kind: KindOverheadLine, Points: []Point{{X: 0, Y: 0}, {X: 1, Y: 1}}},
			{ID: 2, Kind: KindCableLine, Points: []Point{{X: 2, Y: 2}, {X: 3, Y: 3}}},
		},
	}

	var buf bytes.Buffer
	if err := Render(d, lib, &buf, Static, nil); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	if !strings.Contains(out, "stroke-dasharray: 6,5;") {
		t.Errorf("cable line should default to a 6,5 dasharray: %s", out)
	}
	overhead := out[:strings.Index(out, `id="2"`)]
	if strings.Contains(overhead, "dasharray") {
		t.Errorf("overhead line should render solid by default: %s", overhead)
	}
}

func TestResolveCableLineDash(t *testing.T) {
	cases := []struct {
		style ConnectorLineStyle
		want  string
	}{
		{"", "stroke-dasharray: 6,5;"},
		{LineStyleDashed, "stroke-dasharray: 6,5;"},
		{LineStyleDashDot, "stroke-dasharray: 70 20 25 20;"},
		{LineStyleDotted, "stroke-dasharray: 3,2;"},
		{LineStyleSolid, ""},
		{"garbage", "stroke-dasharray: 6,5;"},
	}
	for _, c := range cases {
		t.Run(string(c.style), func(t *testing.T) {
			if got := resolveCableLineDash(c.style); got != c.want {
				t.Errorf("resolveCableLineDash(%q) = %q, want %q", c.style, got, c.want)
			}
		})
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
		// Every elevated element is listed first in document order, but each
		// must still render after the connectors loop, so it paints on top
		// of any wire it sits on instead of a later-painted wire cutting
		// through it.
		Elements: []Element{
			{ID: 1, Class: ClassJunctionPoint, Shape: "7", X: 0, Y: 0},
			{ID: 2, Class: ClassLamp, Shape: "106", X: 1, Y: 1, Radius: 8},
			{ID: 3, Class: ClassFaultPassageIndicator, Shape: "320003", X: 2, Y: 2, Radius: 15},
			{ID: 4, Class: ClassBreaker, Shape: "41", X: 3, Y: 3},
		},
		Connectors: []Connector{
			{ID: 5, Kind: KindOverheadLine, Points: []Point{{X: 0, Y: 0}, {X: 2, Y: 2}}},
		},
	}

	var buf bytes.Buffer
	if err := Render(d, lib, &buf, Static, nil); err != nil {
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
	err = Render(d, lib, &buf, Static, nil)
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

func TestRender_BusbarsAndConnectorsAreSelectable(t *testing.T) {
	lib := NewSymbolLibrary(map[string]string{})
	d := &Diagram{
		Width: 100, Height: 100,
		Elements: []Element{
			{ID: 1, Class: ClassBusBarSection, Points: []Point{{X: 0, Y: 0}, {X: 100, Y: 0}}},
		},
		Connectors: []Connector{
			{ID: 2, Points: []Point{{X: 0, Y: 10}, {X: 100, Y: 10}}},
		},
		Labels: []Label{{ID: 3, X: 5, Y: 5, Size: 10, Text: "Note"}},
	}

	var buf bytes.Buffer
	if err := Render(d, lib, &buf, Interactive, nil); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	if !strings.Contains(out, `id="1" data-editor-kind="element"`) {
		t.Errorf("busbar should carry its bare id and an element data-editor-kind marker: %s", out)
	}
	if !strings.Contains(out, `id="2" data-editor-kind="connector"`) {
		t.Errorf("connector should carry its bare id and a connector data-editor-kind marker: %s", out)
	}
	if !strings.Contains(out, `id="3" x="5" y="5" style="` /* label's own <text> attribute order */) ||
		!strings.Contains(out, `data-editor-kind="label"`) {
		t.Errorf("label should carry its bare id and a label data-editor-kind marker: %s", out)
	}
}

func TestRender_BusbarMatchesXsde2svgConventions(t *testing.T) {
	lib := NewSymbolLibrary(map[string]string{})
	d := &Diagram{
		Width: 2200, Height: 600,
		VoltageClasses: []VoltageClass{{ID: 1, Name: "110kV", Color: "#00A0F0"}},
		Elements: []Element{{
			ID: 957, Class: ClassBusBarSection, Shape: "24", Name: "1SEC 110kV", Voltage: 1,
			Points: []Point{{X: 1670, Y: 520}, {X: 2150, Y: 520}},
		}},
	}

	var buf bytes.Buffer
	if err := Render(d, lib, &buf, Static, nil); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	if !strings.Contains(out, `stroke:#00A0F0;stroke-width:4`) {
		t.Errorf("busbar should draw at 4px in its voltage class's color, like a real xsde2svg busbar: %s", out)
	}
	if !strings.Contains(out, `data-name="1SEC 110kV"`) {
		t.Errorf("busbar should carry its name as data-name, like a real xsde2svg busbar: %s", out)
	}
	if !strings.Contains(out, `data-voltage="#00A0F0"`) {
		t.Errorf("busbar should carry its resolved color as data-voltage, like a real xsde2svg busbar: %s", out)
	}
	if !strings.Contains(out, `data-type="24"`) {
		t.Errorf("busbar should carry its shape as data-type, like a real xsde2svg busbar: %s", out)
	}
	if !strings.Contains(out, `id="957"`) {
		t.Errorf("busbar should carry its own bare integer id, like a real xsde2svg busbar: %s", out)
	}
	if strings.Contains(out, "data-editor-kind") {
		t.Errorf("Static mode should not carry this editor's own data-editor-kind marker: %s", out)
	}
}

func TestRender_BusWorkConnectorCarriesDataType21(t *testing.T) {
	lib := NewSymbolLibrary(map[string]string{})
	d := &Diagram{
		Width: 2000, Height: 1200,
		VoltageClasses: []VoltageClass{{ID: 1, Name: "10kV", Color: "#962896"}},
		Connectors: []Connector{{
			ID: 3988, Kind: KindBusWork, Voltage: 1,
			Points: []Point{{X: 1310, Y: 560}, {X: 1310, Y: 670}},
		}},
	}

	var buf bytes.Buffer
	if err := Render(d, lib, &buf, Static, nil); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	if !strings.Contains(out, `data-type="21"`) {
		t.Errorf("a BusWork connector (what diagramOps.connectElements creates) should carry data-type=\"21\", like a real xsde2svg connection: %s", out)
	}
	if !strings.Contains(out, `id="3988"`) {
		t.Errorf("connector should carry its own bare integer id: %s", out)
	}
}

// TestRender_ObjectLinkMatchesXsde2svgFormat uses the exact real xsde2svg
// markup (a real instance's own polyline plus its own separate arrowhead
// <path>) that this shape's own geometry/rotation-angle formula were
// reverse-engineered from: <polyline points="1980,300 1980,252"
// style="fill:none;stroke:#00A0F0;;stroke-width:2" data-type="28"
// id="2120" data-voltage="#00A0F0" /> plus <path d="M 1987 252 l -7 12
// l -7 -12 z" style="fill:none;stroke:#00A0F0;stroke-width:2"
// transform="rotate(180,1980,252)" />. The arrowhead's own real on-screen
// position (the point that actually matters — its own d/transform strings
// are otherwise free to differ from the real instance's, since this
// package places it via the translate-then-rotate convention every symbol
// template already uses rather than that real instance's own single
// rotate(angle,cx,cy) around an absolute-coordinate path, and those two
// aren't interchangeable for a local-origin path — see writeObjectLink's
// own doc comment) is independently recomputed here from first principles
// (a real 2D rotation) and checked against the real instance's own known
// screen position, base corner (1973,252) and apex (1980,240), rather than
// trusting the rendered transform string to be correct by construction.
func TestRender_ObjectLinkMatchesXsde2svgFormat(t *testing.T) {
	lib := NewSymbolLibrary(map[string]string{})
	d := &Diagram{
		Width: 2000, Height: 1200,
		VoltageClasses: []VoltageClass{{ID: 1, Name: "110kV", Color: "#00A0F0"}},
		Connectors: []Connector{{
			ID: 2120, Kind: KindLinkToObject, Voltage: 1,
			Points: []Point{{X: 1980, Y: 300}, {X: 1980, Y: 252}},
		}},
	}

	var buf bytes.Buffer
	if err := Render(d, lib, &buf, Static, nil); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	for _, want := range []string{
		`data-type="28"`,
		`id="2120"`,
		`points="1980,300 1980,252"`,
		`stroke:#00A0F0;stroke-width:2`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered object link missing %q: %s", want, out)
		}
	}

	var pathD string
	var tx, ty, angle float64
	m := regexp.MustCompile(`<path d="([^"]+)" style="[^"]*" transform="translate\(([\d.-]+),([\d.-]+)\) rotate\(([\d.-]+)\)" />`).
		FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("no arrowhead <path translate(...) rotate(...)> found: %s", out)
	}
	pathD, tx, ty, angle = m[1], mustParseFloat(t, m[2]), mustParseFloat(t, m[3]), mustParseFloat(t, m[4])
	if pathD != "M 7 0 l -7 12 l -7 -12 z" {
		t.Fatalf("arrowhead path d = %q, want the reverse-engineered local geometry", pathD)
	}

	rad := angle * math.Pi / 180
	cos, sin := math.Cos(rad), math.Sin(rad)
	onScreen := func(lx, ly float64) (float64, float64) {
		return tx + lx*cos - ly*sin, ty + lx*sin + ly*cos
	}
	baseX, baseY := onScreen(7, 0)
	apexX, apexY := onScreen(0, 12)
	const eps = 0.01
	if math.Abs(baseX-1973) > eps || math.Abs(baseY-252) > eps {
		t.Errorf("arrowhead base corner on-screen = (%.4f,%.4f), want (1973,252)", baseX, baseY)
	}
	if math.Abs(apexX-1980) > eps || math.Abs(apexY-240) > eps {
		t.Errorf("arrowhead apex on-screen = (%.4f,%.4f), want (1980,240)", apexX, apexY)
	}
}

func mustParseFloat(t *testing.T, s string) float64 {
	t.Helper()
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		t.Fatalf("parsing %q as float: %v", s, err)
	}
	return v
}

func TestRender_StaticModeOmitsInteractiveMarkup(t *testing.T) {
	lib := NewSymbolLibrary(map[string]string{"41": `<path d="M 0 0" style="stroke:{color}"/>`})
	d := &Diagram{
		Width: 100, Height: 100,
		Elements: []Element{{ID: 1, Class: ClassBreaker, Shape: "41", X: 10, Y: 10}},
	}

	var buf bytes.Buffer
	if err := Render(d, lib, &buf, Static, nil); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(out, "data-editor-kind") {
		t.Errorf("Static mode should not carry this editor's own data-editor-kind marker: %s", out)
	}
	if strings.Contains(out, `fill="transparent"`) {
		t.Errorf("Static mode should not draw the interactive-only invisible hit-target circle: %s", out)
	}
	if !strings.Contains(out, `id="1"`) {
		t.Errorf("Static mode should still carry the element's own bare id, like a real xsde2svg document: %s", out)
	}
}

func TestRender_SwitchingDeviceCarriesDataFillAndDataState(t *testing.T) {
	lib := NewSymbolLibrary(map[string]string{
		"41": `<path d="M -7 -7 h 14 v 14 h -14 z"{fillAttr} style="fill:{fill};stroke:{color};stroke-width:1" />
<path d="{state:M 0 -5 v 10|M -5 0 h 10|M -3.5 -3.5 l 7 7}"{stateAttr} style="stroke:{color};stroke-width:1" />`,
	})
	state := 1
	d := &Diagram{
		Width: 100, Height: 100,
		Elements: []Element{{ID: 1, Class: ClassBreaker, Shape: "41", X: 10, Y: 10, State: &state}},
	}
	stateColors := []StateColor{
		{State: 0, Label: "Open", Color: "red"},
		{State: 1, Label: "Close", Color: "lawngreen"},
		{State: 2, Label: "Intermediate", Color: "yellow"},
	}

	var buf bytes.Buffer
	if err := Render(d, lib, &buf, Static, nil, stateColors...); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	if !strings.Contains(out, `data-fill="0:red,1:lawngreen,2:yellow"`) {
		t.Errorf("switching device's fill path should carry a real xsde2svg data-fill legend built from the configured state colors: %s", out)
	}
	if !strings.Contains(out, `data-state="1"`) {
		t.Errorf("switching device's state-indicator path should carry the element's own raw State as data-state: %s", out)
	}
	if !strings.Contains(out, `fill:lawngreen`) {
		t.Errorf("state 1 should resolve to the configured lawngreen fill: %s", out)
	}
}

func TestRender_MissingStateOmitsDataState(t *testing.T) {
	lib := NewSymbolLibrary(map[string]string{
		"41": `<path d="{state:M 0 -5 v 10|M -5 0 h 10|M -3.5 -3.5 l 7 7}"{stateAttr} style="stroke:{color};stroke-width:1" />`,
	})
	d := &Diagram{
		Width: 100, Height: 100,
		Elements: []Element{{ID: 1, Class: ClassBreaker, Shape: "41", X: 10, Y: 10}},
	}

	var buf bytes.Buffer
	if err := Render(d, lib, &buf, Static, nil, StateColor{State: 0, Label: "Open", Color: "red"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(out, "data-state") {
		t.Errorf("an element with no recorded State should carry no data-state attribute at all: %s", out)
	}
}

// TestRender_PowerTransformer2Winding checks a 2-winding transformer's own
// circle positions/legs/glyphs against hand-computed values, independently
// derived from the real element_47.go default-path constants (baseRadius
// 22, xShift 18, glyph shift 22/3=7, delta shift 22/2=11/half 5) rather
// than any single real corpus instance — the real corpus's own type-47
// instances turned out to mostly use non-default Size presets (see
// writePowerTransformer's own doc comment), so a literal byte-for-byte
// comparison against one specific real file would just be pinning that
// file's own Size choice, not this package's own default geometry.
func TestRender_PowerTransformer2Winding(t *testing.T) {
	lib := NewSymbolLibrary(map[string]string{})
	d := &Diagram{
		Width: 400, Height: 400,
		VoltageClasses: []VoltageClass{{ID: 1, Name: "110kV", Color: "#962896"}, {ID: 2, Name: "35kV", Color: "#965000"}},
		Elements: []Element{{
			ID: 7, Class: ClassPowerTransformer, Shape: "47", Name: "T-1", X: 100, Y: 100,
			Windings: []TransformerWinding{
				{Voltage: 1, Scheme: SchemeDelta},
				{Voltage: 2, Scheme: SchemeWyeN, Grounding: GroundingSolid},
			},
		}},
	}

	var buf bytes.Buffer
	if err := Render(d, lib, &buf, Static, nil); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	for _, want := range []string{
		`<!-- Power transformer:47 -->`,
		`<g id="7" data-name="T-1" data-voltage="gray" data-type="47" transform="translate(100,100) rotate(0)">`,
		// Winding 0 (delta, #962896): circle at local (+18,0), leg to
		// (+50,0) (circle edge 18+22=40, plus a 10-unit lead — chosen so
		// the tip lands on the 10-unit grid; see transformerLegLength's
		// own doc comment), and the delta triangle M(cx,cy-5) l(5,11)
		// l(-11,0) z.
		`<circle cx="18" cy="0" r="22" style="fill:none;stroke:#962896;stroke-width:2" data-voltage="#962896" />`,
		`<path d="M 40 0 L 50 0" style="fill:none;stroke:#962896;stroke-width:2" data-voltage="#962896" />`,
		`<path d="M 18 -5 l 5 11 l -11 0 z" style="fill:none;stroke:#962896;stroke-width:1" />`,
		// Winding 1 (wye-with-neutral + solid grounding, #965000): circle
		// at local (-18,0), leg to (-50,0), the 4-spoke wyeN glyph, and a
		// grounding mark just past the neutral spoke's own tip.
		`<circle cx="-18" cy="0" r="22" style="fill:none;stroke:#965000;stroke-width:2" data-voltage="#965000" />`,
		`<path d="M -40 0 L -50 0" style="fill:none;stroke:#965000;stroke-width:2" data-voltage="#965000" />`,
		`<path d="M -18 0 l -7 -7 M -18 0 l 7 -7 M -18 0 l 0 7 M -18 0 l 7 0" style="fill:none;stroke:#965000;stroke-width:1" />`,
		`<path d="M -8 0 v 4 M -13 4 h 10 M -11 7 h 6 M -9 10 h 2" style="fill:none;stroke:#965000;stroke-width:1" />`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("2-winding power transformer missing %q: %s", want, out)
		}
	}
	if strings.Contains(out, "a 18 18") {
		t.Errorf("a non-autotransformer should draw no tap decoration: %s", out)
	}
	if strings.Contains(out, "stroke-width:1\" />\n<path d=\"M -25") {
		t.Errorf("a transformer with no TapChanger winding should draw no regulation arrow: %s", out)
	}
}

// TestRender_PowerTransformer2WindingAutotransformerTap checks the tap's
// own real terminal (transformerTapOffset, local (0,-50) — grid-aligned
// and independent of Windings[0]'s own circle position) and the arc
// connecting it to Windings[0]'s own circle, for the case where that
// circle sits off-center (dX=18, the 2-winding default) rather than
// directly below the tap the way a 3/4-winding transformer's own
// top-positioned winding 0 does — the case that surfaced a real
// connectivity bug (see RELEASE.md): a real user reported the tap arc
// needing to "end with a connector," since the original version of this
// decoration was purely cosmetic, with no real terminal a wire could
// bind to at all.
func TestRender_PowerTransformer2WindingAutotransformerTap(t *testing.T) {
	lib := NewSymbolLibrary(map[string]string{})
	d := &Diagram{
		Width: 400, Height: 400,
		VoltageClasses: []VoltageClass{{ID: 1, Name: "110kV", Color: "teal"}, {ID: 2, Name: "35kV", Color: "purple"}},
		Elements: []Element{{
			ID: 13, Class: ClassPowerTransformer, Shape: "47", X: 100, Y: 100, Autotransformer: true,
			Windings: []TransformerWinding{{Voltage: 1, Scheme: SchemeWye}, {Voltage: 2, Scheme: SchemeWye}},
		}},
	}

	var buf bytes.Buffer
	if err := Render(d, lib, &buf, Static, nil); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	for _, want := range []string{
		// Winding 0's own circle at local (+18,0) (the 2-winding default).
		`<circle cx="18" cy="0" r="22" style="fill:none;stroke:teal;stroke-width:2" data-voltage="teal" />`,
		// The tap's own real terminal: stub from (0,-50) down to (0,-30),
		// still directly above the *anchor* (x=0), not winding 0's own
		// circle (x=18) — this is what keeps it grid-aligned regardless
		// of which winding position the tap ends up decorating. Drawn at
		// stroke-width 2, matching every regular winding lead — this is a
		// real electrical lead, not a thinner decorative line.
		`<path d="M 0 -50 L 0 -30" style="fill:none;stroke:teal;stroke-width:2" />`,
		// The connecting arc sweeps from (0,-30), radius 40, to a point on
		// winding 0's own circle rim 50° up from its own top, offset
		// toward whichever side the circle sits on (right, here, since
		// winding 0 sits at +18) — a broad, smooth sweep past the
		// circle's own top rather than a tight loop landing dead center.
		`<path d="M 0 -30 A 40 40 0 0 1 35 -14" style="fill:none;stroke:teal;stroke-width:2" />`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("2-winding autotransformer tap missing %q: %s", want, out)
		}
	}
}

// TestRender_PowerTransformer3WindingAutotransformer checks an
// autotransformer's own tap decoration (drawn on Windings[0]'s own real
// circle rather than real xsde2svg's own separate phantom-winding
// convention — see writeAutotransformerTap's own doc comment) and the
// regulation arrow, drawn once for whichever winding has TapChanger set.
func TestRender_PowerTransformer3WindingAutotransformer(t *testing.T) {
	lib := NewSymbolLibrary(map[string]string{})
	d := &Diagram{
		Width: 400, Height: 400,
		VoltageClasses: []VoltageClass{{ID: 1, Name: "110kV", Color: "teal"}, {ID: 2, Name: "35kV", Color: "purple"}, {ID: 3, Name: "10kV", Color: "olive"}},
		Elements: []Element{{
			ID: 9, Class: ClassPowerTransformer, Shape: "47", X: 100, Y: 100, Autotransformer: true,
			Windings: []TransformerWinding{
				{Voltage: 1, Scheme: SchemeWye, TapChanger: true},
				{Voltage: 2, Scheme: SchemeWye},
				{Voltage: 3, Scheme: SchemeWyeN},
			},
		}},
	}

	var buf bytes.Buffer
	if err := Render(d, lib, &buf, Static, nil); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	for _, want := range []string{
		// Winding 0 (top, default terminal for a 3-winding transformer):
		// circle at local (0,-25), the tap stub+arc, and the regulation
		// arrow (centered on the element's own anchor, teal — winding 0's
		// own color, since it's the one with TapChanger set).
		`<circle cx="0" cy="-25" r="22" style="fill:none;stroke:teal;stroke-width:2" data-voltage="teal" />`,
		// Winding 0's own real lead (top, legLen 13 — chosen so
		// topShift(25)+radius(22)+13=60 lands on the 10-unit grid), drawn
		// in addition to (not instead of) its own tap decoration below.
		`<path d="M 0 -47 L 0 -60" style="fill:none;stroke:teal;stroke-width:2" data-voltage="teal" />`,
		// The autotransformer's own extra "line" terminal
		// (transformerTapOffset): a real, connectable point at local
		// (0,-50) — always directly above the anchor regardless of
		// winding count, so it lands on the grid the same way a
		// top-positioned winding's own lead does — with a short stub
		// down to (0,-30) and a broad radius-40 arc sweeping to a point
		// on winding 0's own circle rim 50° up from its own top (17,-39)
		// — offset from dead-center-top even though winding 0 itself has
		// no horizontal offset of its own (dX=0 for a 3-winding's own top
		// winding), since the landing angle is independent of that.
		`<path d="M 0 -50 L 0 -30" style="fill:none;stroke:teal;stroke-width:2" />`,
		`<path d="M 0 -30 A 40 40 0 0 1 17 -39" style="fill:none;stroke:teal;stroke-width:2" />`,
		`<path d="M -25 25 L 25 -25" style="fill:none;stroke:teal;stroke-width:1" />`,
		`<path d="M 28 -23 l -5 -5 l 7 -2 z" style="fill:teal;stroke:teal;stroke-width:1" />`,
		// Windings 1/2 (right/left, matching the non-auto 3-winding
		// default) carry no tap decoration of their own, and their own
		// legLen (10 — xShift(18)+radius(22)+10=50, also grid-aligned)
		// differs from winding 0's own.
		`<circle cx="18" cy="0" r="22" style="fill:none;stroke:purple;stroke-width:2" data-voltage="purple" />`,
		`<path d="M 40 0 L 50 0" style="fill:none;stroke:purple;stroke-width:2" data-voltage="purple" />`,
		`<circle cx="-18" cy="0" r="22" style="fill:none;stroke:olive;stroke-width:2" data-voltage="olive" />`,
		`<path d="M -40 0 L -50 0" style="fill:none;stroke:olive;stroke-width:2" data-voltage="olive" />`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("autotransformer missing %q: %s", want, out)
		}
	}
}

// TestRender_PowerTransformer4Winding checks the 4-winding default circle
// layout (top/bottom/left/right), independently verified against a real
// (fractional-scale) 4-winding corpus instance whose proportions matched
// these same constants once the scale factor was divided back out.
func TestRender_PowerTransformer4Winding(t *testing.T) {
	lib := NewSymbolLibrary(map[string]string{})
	d := &Diagram{
		Width: 400, Height: 400,
		VoltageClasses: []VoltageClass{{ID: 1, Color: "teal"}, {ID: 2, Color: "purple"}, {ID: 3, Color: "olive"}, {ID: 4, Color: "purple"}},
		Elements: []Element{{
			ID: 11, Class: ClassPowerTransformer, Shape: "47", X: 100, Y: 100,
			Windings: []TransformerWinding{{Voltage: 1}, {Voltage: 2}, {Voltage: 3}, {Voltage: 4}},
		}},
	}

	var buf bytes.Buffer
	if err := Render(d, lib, &buf, Static, nil); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	for _, want := range []string{
		`<circle cx="0" cy="-20" r="22"`, // top
		`<circle cx="0" cy="18" r="22"`,  // bottom
		`<circle cx="-29" cy="0" r="22"`, // left
		`<circle cx="29" cy="0" r="22"`,  // right
		// Each winding's own lead tip lands on the 10-unit grid despite
		// each position using its own distinct legLen: top uses 8
		// (vertShift(20)+radius(22)+8=50), bottom uses 10
		// (xShift(18)+radius(22)+10=50 — winding 1 reuses xShift), and
		// left/right use 9 (sideShift(29)+radius(22)+9=60).
		`L 0 -50`, // top:    -20-22-8  = -50
		`L 0 50`,  // bottom:  18+22+10 =  50
		`L -60 0`, // left:   -29-22-9  = -60
		`L 60 0`,  // right:   29+22+9  =  60
	} {
		if !strings.Contains(out, want) {
			t.Errorf("4-winding power transformer missing %q: %s", want, out)
		}
	}
}

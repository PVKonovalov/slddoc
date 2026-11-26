package render

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"github.com/PVKonovalov/slddoc/internal/model"
)

//go:fix inline
func fptr(v float64) *float64 { return new(v) }

//go:fix inline
func bptr(v bool) *bool { return new(v) }

func renderBar(t *testing.T, b model.BarInstrument, mode RenderMode) string {
	t.Helper()
	var buf bytes.Buffer
	lib := NewSymbolLibrary(map[string]string{})
	if err := Render(&model.Diagram{BarInstruments: []model.BarInstrument{b}}, lib, &buf, mode, "", nil); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// tag returns the start tag of the node with the given class.
func tag(t *testing.T, svg, class string) string {
	t.Helper()
	m := regexp.MustCompile(`<[a-z]+ class="` + class + `"[^>]*>`).FindString(svg)
	if m == "" {
		t.Fatalf("no %s in\n%s", class, svg)
	}
	return m
}

func attr(s, name string) string {
	m := regexp.MustCompile(` ` + name + `="([^"]*)"`).FindStringSubmatch(s)
	if m == nil {
		return "<none>"
	}
	return m[1]
}

func TestBarInstrumentContract(t *testing.T) {
	b := model.BarInstrument{
		ID: 5, X: 10, Y: 20, Name: "U", Unit: "kV", Min: 0, Max: 150, Length: 150, Thickness: 6,
		LowAlarm: new(float64(15)), LowWarning: new(float64(30)), HighWarning: new(float64(120)), HighAlarm: new(float64(135)), Deadband: 1,
		Precision: 1, Value: 123.4,
	}
	svg := renderBar(t, b, Static)
	root := regexp.MustCompile(`<g id="5"[^>]*>`).FindString(svg)
	want := `<g id="5" data-type="320008" data-name="U" data-orient="h" data-len="150" data-thickness="6" data-min="0" data-max="150" data-origin="0" data-la="15" data-lw="30" data-hw="120" data-ha="135" data-deadband="1" data-precision="1" data-unit="kV" data-value="123.4" data-colors="normal:lawngreen;warning:#c9a800;alarm:red;bad:#808080" transform="translate(10,20)">`
	if root != want {
		t.Errorf("root:\n got %s\nwant %s", root, want)
	}
	if strings.Contains(svg, "data-editor-kind") || strings.Contains(svg, "bar-scale") || strings.Contains(svg, "bar-origin") {
		t.Errorf("static output has editor attributes, a scale or an origin line:\n%s", svg)
	}
	if a := tag(t, svg, "bar-axis"); strings.Contains(a, "transform") {
		t.Errorf("horizontal axis is transformed: %s", a)
	}
	track := tag(t, svg, "bar-track")
	if attr(track, "x") != "0" || attr(track, "y") != "-6" || attr(track, "width") != "150" || attr(track, "height") != "12" || attr(track, "fill") != "#3a3f47" {
		t.Errorf("track: %s", track)
	}
	fill := tag(t, svg, "bar-fill")
	if attr(fill, "x") != "0" || attr(fill, "y") != "-3" || attr(fill, "width") != "123.4" || attr(fill, "height") != "6" || attr(fill, "fill") != "#c9a800" {
		t.Errorf("fill: %s", fill)
	}
	for class, x := range map[string]string{"bar-limit-la": "15", "bar-limit-lw": "30", "bar-limit-hw": "120", "bar-limit-ha": "135"} {
		l := tag(t, svg, class)
		if attr(l, "x1") != x || attr(l, "x2") != x || strings.Contains(l, "hidden") {
			t.Errorf("%s: %s", class, l)
		}
	}
	for class, xw := range map[string][2]string{"bar-zone-la": {"0", "15"}, "bar-zone-lw": {"15", "15"}, "bar-zone-hw": {"120", "15"}, "bar-zone-ha": {"135", "15"}} {
		z := tag(t, svg, class)
		if attr(z, "x") != xw[0] || attr(z, "width") != xw[1] || strings.Contains(z, "hidden") {
			t.Errorf("%s: %s", class, z)
		}
	}
	for _, class := range []string{"bar-over-max", "bar-over-min"} {
		if attr(tag(t, svg, class), "visibility") != "hidden" {
			t.Errorf("%s visible", class)
		}
	}
	if !strings.Contains(svg, `fill="#c9a800" style="font-size:12px;font-family:Arial;text-anchor:start;dominant-baseline:middle">123.4 kV</text>`) {
		t.Errorf("value text:\n%s", svg)
	}
}

func TestBarInstrumentZones(t *testing.T) {
	base := model.BarInstrument{ID: 1, Min: 0, Max: 100, LowAlarm: new(float64(10)), LowWarning: new(float64(20)), HighWarning: new(float64(80)), HighAlarm: new(float64(90))}
	for _, c := range []struct {
		value          float64
		color          string
		overMin, overM string
	}{
		{5, "red", "hidden", "hidden"},
		{15, "#c9a800", "hidden", "hidden"},
		{50, "lawngreen", "hidden", "hidden"},
		{85, "#c9a800", "hidden", "hidden"},
		{95, "red", "hidden", "hidden"},
		{-5, "red", "visible", "hidden"},
		{105, "red", "hidden", "visible"},
	} {
		b := base
		b.Value = c.value
		svg := renderBar(t, b, Static)
		if got := attr(tag(t, svg, "bar-fill"), "fill"); got != c.color {
			t.Errorf("value %v: colour %s, want %s", c.value, got, c.color)
		}
		if attr(tag(t, svg, "bar-over-min"), "visibility") != c.overMin || attr(tag(t, svg, "bar-over-max"), "visibility") != c.overM {
			t.Errorf("value %v: overflow marks wrong", c.value)
		}
	}
	b := base
	b.Value = 105
	if w := attr(tag(t, renderBar(t, b, Static), "bar-fill"), "width"); w != "80" {
		t.Errorf("above max: width %s, want the default length 80", w)
	}
}

func TestBarInstrumentUnsetSetpoints(t *testing.T) {
	b := model.BarInstrument{ID: 1, Min: 0, Max: 100, Length: 100, HighWarning: new(float64(80)), Value: 90}
	svg := renderBar(t, b, Static)
	if strings.Contains(svg, "data-la=") || strings.Contains(svg, "data-ha=") || strings.Contains(svg, "data-deadband") {
		t.Errorf("unset setpoints written:\n%s", svg)
	}
	for _, class := range []string{"bar-limit-la", "bar-limit-lw", "bar-limit-ha", "bar-zone-la", "bar-zone-lw", "bar-zone-ha"} {
		if attr(tag(t, svg, class), "visibility") != "hidden" {
			t.Errorf("%s not hidden", class)
		}
	}
	hw := tag(t, svg, "bar-zone-hw")
	if attr(hw, "x") != "80" || attr(hw, "width") != "20" || strings.Contains(hw, "hidden") {
		t.Errorf("warning zone does not reach max with no alarm setpoint: %s", hw)
	}
	if attr(tag(t, svg, "bar-fill"), "fill") != "#c9a800" {
		t.Errorf("zone")
	}

	b.ValueSize = 18
	if !strings.Contains(renderBar(t, b, Static), `style="font-size:18px;font-family:Arial;`) {
		t.Errorf("value size 18 not used")
	}
	b.ShowZones = new(false)
	b.ShowValue = new(false)
	svg = renderBar(t, b, Static)
	if strings.Contains(svg, "bar-zone") || strings.Contains(svg, "bar-value") || !strings.Contains(svg, "bar-limit-la") {
		t.Errorf("zones/value off:\n%s", svg)
	}
}

func TestBarInstrumentOriginVertical(t *testing.T) {
	b := model.BarInstrument{
		ID: 1, Orient: model.BarVertical, Min: -50, Max: 50, Length: 100, Origin: new(float64(0)), Value: -20,
		ShowScale: new(true), Colors: &model.BarColors{Normal: "blue"},
	}
	svg := renderBar(t, b, Interactive)
	if a := tag(t, svg, "bar-axis"); attr(a, "transform") != "rotate(-90)" {
		t.Errorf("axis: %s", a)
	}
	fill := tag(t, svg, "bar-fill")
	if attr(fill, "x") != "30" || attr(fill, "width") != "20" || attr(fill, "fill") != "blue" {
		t.Errorf("fill from the origin: %s", fill)
	}
	if o := tag(t, svg, "bar-origin"); attr(o, "x1") != "50" {
		t.Errorf("origin line: %s", o)
	}
	for _, want := range []string{`data-origin="0"`, `data-colors="normal:blue;`, `data-editor-kind="barinstrument"`, `>-50</text>`, `>50</text>`, `>0</text>`, `rotate(90,`} {
		if !strings.Contains(svg, want) {
			t.Errorf("missing %s in\n%s", want, svg)
		}
	}
	lib := NewSymbolLibrary(map[string]string{})
	lib.SetBarDefaults(BarDefaults{Length: 200, ValueSize: 9, ShowValue: true, Colors: model.BarColors{Track: "black"}})
	frags, err := RenderFragments(&model.Diagram{BarInstruments: []model.BarInstrument{{ID: 3, Layer: 2, Min: 0, Max: 1}}}, lib, []int{3}, Static, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	f := frags[3]
	for _, want := range []string{`data-len="200"`, `data-layer="2"`, `fill="black"`, `class="bar-value"`, `font-size:9px`} {
		if !strings.Contains(f, want) {
			t.Errorf("fragment with library defaults: missing %s in\n%s", want, f)
		}
	}
	if strings.Contains(f, "bar-zones") {
		t.Errorf("library default ShowZones false ignored")
	}
}

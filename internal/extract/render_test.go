package extract

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"github.com/PVKonovalov/slddoc/internal/model"
	"github.com/PVKonovalov/slddoc/internal/render"
)

// TestRender_ObjectLinkIsOneGroup checks that an object link's line and
// arrowhead form a single item: Interactive puts data-editor-kind on the
// group (so the canvas selects the link from its arrowhead too), the
// polyline has no id of its own, data-layer lands on the group (so the
// arrowhead hides with its layer), and a saved Static file extracts back
// to the same connector.
func TestRender_ObjectLinkIsOneGroup(t *testing.T) {
	lib := render.NewSymbolLibrary(map[string]string{})
	d := &model.Diagram{
		Width: 400, Height: 400,
		Layers:         []model.Layer{{ID: model.BaseLayer, Name: "Base"}, {ID: 3, Name: "Links"}},
		VoltageClasses: []model.VoltageClass{{ID: 1, Name: "110kV", Color: "#00A0F0"}},
		Connectors: []model.Connector{{
			ID: 77, Kind: model.KindLinkToObject, Voltage: 1, Name: "Link-77", Layer: 3,
			Points: []model.Point{{X: 100, Y: 300}, {X: 100, Y: 200}, {X: 200, Y: 200}},
		}},
	}

	var buf bytes.Buffer
	if err := render.Render(d, lib, &buf, render.Interactive, "", nil); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, `<g data-layer="3" id="77" data-type="28" data-name="Link-77" data-voltage="#00A0F0" data-editor-kind="connector">`) {
		t.Errorf("Interactive object link should be one <g> with id, data-editor-kind and data-layer: %s", out)
	}
	if strings.Count(out, `id="77"`) != 1 {
		t.Errorf("only the group should carry the id: %s", out)
	}
	if !regexp.MustCompile(`<g [^>]*id="77"[^>]*>\n<polyline [^>]*/>\n<path [^>]*/>\n</g>`).MatchString(out) {
		t.Errorf("polyline and arrowhead should both sit inside the group: %s", out)
	}

	buf.Reset()
	if err := render.Render(d, lib, &buf, render.Static, "", nil); err != nil {
		t.Fatal(err)
	}
	got, _, err := Extract(buf.Bytes(), "link.svg", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Connectors) != 1 {
		t.Fatalf("extracted %d connectors, want 1: %+v", len(got.Connectors), got.Connectors)
	}
	c := got.Connectors[0]
	if c.ID != 77 || c.Kind != model.KindLinkToObject || c.Name != "Link-77" || c.Layer != 3 || len(c.Points) != 3 || c.Points[2] != (model.Point{X: 200, Y: 200}) {
		t.Errorf("extracted connector = %+v", c)
	}
}

func TestRender_RectangleRoundedCornersRoundTrip(t *testing.T) {
	d := &model.Diagram{
		Width: 100, Height: 100,
		Elements: []model.Element{
			{ID: 1, Class: model.ClassRectangle, Shape: "3", RadiusX: 50, Points: []model.Point{{X: 10, Y: 10}, {X: 50, Y: 30}}},
			{ID: 2, Class: model.ClassRectangle, Shape: "3", Points: []model.Point{{X: 60, Y: 10}, {X: 90, Y: 30}}},
		},
	}
	var buf bytes.Buffer
	if err := render.Render(d, render.NewSymbolLibrary(nil), &buf, render.Static, "", nil); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, `<rect id="1" x="10" y="10" width="40" height="20" rx="10" `) {
		t.Errorf("radius should be capped at half the shorter side: %s", out)
	}
	if strings.Contains(out, `<rect id="2" x="60" y="10" width="30" height="20" rx=`) {
		t.Errorf("square rectangle must not carry rx: %s", out)
	}

	got, _, err := Extract(buf.Bytes(), "test.svg", nil)
	if err != nil {
		t.Fatal(err)
	}
	radii := map[int]float64{}
	for _, e := range got.Elements {
		if e.Class == model.ClassRectangle {
			radii[e.ID] = e.RadiusX
		}
	}
	if radii[1] != 10 || radii[2] != 0 {
		t.Errorf("extracted radii = %v, want {1:10 2:0}", radii)
	}
}

func TestRender_GroupRoundTrip(t *testing.T) {
	d := &model.Diagram{
		Width: 100, Height: 100,
		Layers: []model.Layer{{ID: model.BaseLayer, Name: "Base"}, {ID: 9, Name: "Top", Z: 1}},
		Elements: []model.Element{
			{ID: 1, Class: model.ClassRectangle, Shape: "3", Layer: model.BaseLayer, Group: 5, Points: []model.Point{{X: 10, Y: 10}, {X: 50, Y: 30}}},
			{ID: 2, Class: model.ClassRectangle, Shape: "3", Layer: 9, Group: 5, Points: []model.Point{{X: 60, Y: 10}, {X: 90, Y: 30}}},
			{ID: 3, Class: model.ClassRectangle, Shape: "3", Layer: model.BaseLayer, Points: []model.Point{{X: 60, Y: 60}, {X: 90, Y: 90}}},
		},
		Labels: []model.Label{{ID: 4, Layer: model.BaseLayer, Group: 5, X: 10, Y: 50, Size: 12, Text: "caption"}},
	}
	var buf bytes.Buffer
	if err := render.Render(d, render.NewSymbolLibrary(nil), &buf, render.Static, "", nil); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{`<rect data-group="5" id="1"`, `<rect data-layer="9" data-group="5" id="2"`, `<g data-group="5" data-type="5"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %s", want, out)
		}
	}
	if strings.Contains(out, `<rect data-group`+`="5" id="3"`) || strings.Count(out, "data-group") != 3 {
		t.Errorf("only group members should carry data-group: %s", out)
	}

	got, _, err := Extract(buf.Bytes(), "test.svg", nil)
	if err != nil {
		t.Fatal(err)
	}
	groups := map[int]int{}
	for _, e := range got.Elements {
		groups[e.ID] = e.Group
	}
	for _, l := range got.Labels {
		groups[-1] = l.Group
	}
	if groups[1] == 0 || groups[1] != groups[2] || groups[1] != groups[-1] || groups[3] != 0 {
		t.Errorf("extracted groups = %v, want 1, 2 and the label sharing one non-zero id, 3 ungrouped", groups)
	}
	if groups[1] <= 4 || groups[1] > got.LastID {
		t.Errorf("group id %d should be fresh (above every item id) and within LastID %d", groups[1], got.LastID)
	}
}

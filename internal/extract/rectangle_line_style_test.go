package extract

import (
	"bytes"
	"strings"
	"testing"

	"github.com/PVKonovalov/slddoc/internal/model"
	"github.com/PVKonovalov/slddoc/internal/render"
)

// TestRectangleLineStyleRoundTrip draws a Rectangle's border in each line style and reads the style back.
func TestRectangleLineStyleRoundTrip(t *testing.T) {
	for _, c := range []struct {
		style model.ConnectorLineStyle
		dash  string
	}{
		{"", ""},
		{model.LineStyleDotted, "stroke-dasharray: 3,2;"},
		{model.LineStyleDashed, "stroke-dasharray: 6,5;"},
	} {
		d := &model.Diagram{Width: 200, Height: 200, Elements: []model.Element{{
			ID: 1, Class: model.ClassRectangle, Shape: "3", X: 50, Y: 40,
			Points: []model.Point{{X: 20, Y: 20}, {X: 80, Y: 60}}, LineStyle: c.style,
		}}}
		var buf bytes.Buffer
		if err := render.Render(d, render.NewSymbolLibrary(nil), &buf, render.Static, "", nil); err != nil {
			t.Fatal(err)
		}
		if c.dash != "" && !strings.Contains(buf.String(), c.dash) {
			t.Errorf("%q: no %s in %s", c.style, c.dash, buf.String())
		}
		if c.dash == "" && strings.Contains(buf.String(), "stroke-dasharray") {
			t.Errorf("solid border dashed: %s", buf.String())
		}
		got, report, err := Extract(buf.Bytes(), "", nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(report.Failed) != 0 || len(got.Elements) != 1 {
			t.Fatalf("report = %+v, elements = %d", report, len(got.Elements))
		}
		if ls := got.Elements[0].LineStyle; ls != c.style {
			t.Errorf("LineStyle = %q, want %q", ls, c.style)
		}
	}
}

package extract

import (
	"bytes"
	"strings"
	"testing"

	"github.com/PVKonovalov/slddoc/internal/model"
	"github.com/PVKonovalov/slddoc/internal/render"
)

// TestParseDisplayPanel reads a real xsde2svg Display panel (element_102.go): no data-state, so it reads back as Off.
func TestParseDisplayPanel(t *testing.T) {
	const svg = `<g id="2939" data-type="102" data-voltage="lawngreen" >
<rect x="170" y="80" width="70" height="20" style="fill:lawngreen;stroke:gray;stroke-width:1" />
<text x="205" y="92" style="fill:green;text-anchor:middle;dominant-baseline:middle;font-size:12px;font-family:Lucida Sans Unicode " >0</text>
</g>`
	el, err := parseStatusTile(parseFirst(t, svg), "102")
	if err != nil {
		t.Fatal(err)
	}
	if el.Class != model.ClassDisplayPanel || el.Shape != "102" || el.State != nil {
		t.Errorf("class/shape/state = %s/%s/%v", el.Class, el.Shape, el.State)
	}
	if len(el.Points) != 2 || el.Points[0] != (model.Point{X: 170, Y: 80}) || el.Points[1] != (model.Point{X: 240, Y: 100}) {
		t.Errorf("points = %v", el.Points)
	}
	if el.FillOff != "lawngreen" || el.PropertyText != "0" || el.TextColor != "green" || el.TextSize != 0 || el.Stroke != "gray" {
		t.Errorf("off look = %+v", el)
	}
}

// TestDisplayPanel_DefaultsAndRoundTrip checks the gray defaults of an unstyled panel and that a panel drawn On reads
// back On.
func TestDisplayPanel_DefaultsAndRoundTrip(t *testing.T) {
	plain := model.Element{ID: 5, Class: model.ClassDisplayPanel, Shape: "102", PropertyText: "0,0",
		Points: []model.Point{{X: 10, Y: 80}, {X: 80, Y: 100}}}
	on := 1
	lit := model.Element{ID: 6, Class: model.ClassDisplayPanel, Shape: "102", State: &on,
		Points: []model.Point{{X: 90, Y: 80}, {X: 160, Y: 100}}, FillOn: "lawngreen", PropertyTextOn: "1", TextColorOn: "green"}
	d := &model.Diagram{Width: 200, Height: 200, Elements: []model.Element{plain, lit}}
	var buf bytes.Buffer
	if err := render.Render(d, render.NewSymbolLibrary(nil), &buf, render.Static, "", nil); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		`data-type="102"`,
		`<rect x="10" y="80" width="70" height="20" style="fill:gray;stroke:gray;stroke-width:1" />`,
		`<text x="45" y="92"`,
		`>0,0</text>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("want %s in\n%s", want, out)
		}
	}
	back, report, err := Extract(buf.Bytes(), "", nil)
	if err != nil || len(report.Failed) != 0 || len(report.Skipped) != 0 || len(back.Elements) != 2 {
		t.Fatalf("extract: %v %+v", err, report)
	}
	got := back.Elements[1]
	if got.Class != model.ClassDisplayPanel || got.State == nil || *got.State != 1 || got.FillOn != "lawngreen" || got.PropertyTextOn != "1" || got.TextColorOn != "green" {
		t.Errorf("lit panel = %+v", got)
	}
}

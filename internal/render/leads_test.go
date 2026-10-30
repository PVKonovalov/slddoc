package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/PVKonovalov/slddoc/internal/model"
)

func TestRenderLeadExtension(t *testing.T) {
	lib := NewSymbolLibrary(map[string]string{"41": `<path d="M 0 -10 v 3 M 0 10 v -3" style="stroke:{color};stroke-width:1" />`})
	d := &model.Diagram{Width: 500, Height: 500, Elements: []model.Element{
		{ID: 1, Class: model.ClassBreaker, Shape: "41", X: 350, Y: 285, Scale: 2, Span: 60},
	}}
	var buf bytes.Buffer
	if err := Render(d, lib, &buf, Interactive, "", nil); err != nil {
		t.Fatal(err)
	}
	// 60/2 = 30 diagram units = 15 local units under scale(2).
	if !strings.Contains(buf.String(), `d="M 0 -10 V -15 M 0 10 V 15"`) {
		t.Errorf("missing lead extension in\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), `stroke-width:0.5`) {
		t.Errorf("lead extension must keep the stroke width under the size step:\n%s", buf.String())
	}
}

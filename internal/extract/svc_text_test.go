package extract

import (
	"bytes"
	"testing"

	"github.com/PVKonovalov/slddoc/internal/model"
	"github.com/PVKonovalov/slddoc/internal/render"
)

const svcTemplate = `<path d="M 0 0 v {terminal:6|-6}" style="fill:none;stroke:{color};stroke-width:2" />
<path d="M -14 {terminal:6|-22} h 28 v 16 h -28 z" style="fill:none;stroke:{color};stroke-width:2" />
<text x="0" y="{terminal:14|-14}" style="fill:{color};font-size:9px;font-family:Arial;text-anchor:middle;dominant-baseline:middle" >{text:SVC}</text>`

// TestExtractStaticVarCompensatorText reads a Static var compensator's label back from the editor's own rendering:
// a custom one into PropertyText, the default SVC as unset.
func TestExtractStaticVarCompensatorText(t *testing.T) {
	lib := render.NewSymbolLibrary(map[string]string{"330003": svcTemplate})
	for _, text := range []string{"", "SVC-2 <1>"} {
		d := &model.Diagram{Width: 200, Height: 200, Elements: []model.Element{{
			ID: 1, Class: model.ClassStaticVarCompensator, Shape: "330003", X: 50, Y: 50,
			Ports: []model.Port{{Name: "1"}}, PropertyText: text,
		}}}
		var buf bytes.Buffer
		if err := render.Render(d, lib, &buf, render.Static, "", nil); err != nil {
			t.Fatal(err)
		}
		got, report, err := Extract(buf.Bytes(), "", nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(report.Failed) != 0 || len(got.Elements) != 1 {
			t.Fatalf("report = %+v, elements = %d", report, len(got.Elements))
		}
		if pt := got.Elements[0].PropertyText; pt != text {
			t.Errorf("PropertyText = %q, want %q", pt, text)
		}
	}
}

// TestExtractStaticVarCompensatorTerminal reads a Static var compensator's terminal side back from the editor's own
// rendering, turned or not.
func TestExtractStaticVarCompensatorTerminal(t *testing.T) {
	lib := render.NewSymbolLibrary(map[string]string{"330003": svcTemplate})
	for _, dir := range []model.TerminalDirection{"", model.TerminalBottom} {
		for _, orient := range []int{0, 90, 180} {
			d := &model.Diagram{Width: 200, Height: 200, Elements: []model.Element{{
				ID: 1, Class: model.ClassStaticVarCompensator, Shape: "330003", X: 100, Y: 100, Orient: orient,
				Ports: []model.Port{{Name: "1"}}, Terminal: dir,
			}}}
			var buf bytes.Buffer
			if err := render.Render(d, lib, &buf, render.Static, "", nil); err != nil {
				t.Fatal(err)
			}
			got, report, err := Extract(buf.Bytes(), "", nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(report.Failed) != 0 || len(got.Elements) != 1 {
				t.Fatalf("report = %+v, elements = %d", report, len(got.Elements))
			}
			el := got.Elements[0]
			if el.Terminal != dir || el.Orient != orient || el.X != 100 || el.Y != 100 {
				t.Errorf("terminal %q orient %d: got terminal %q orient %d at (%v,%v)", dir, orient, el.Terminal, el.Orient, el.X, el.Y)
			}
		}
	}
}

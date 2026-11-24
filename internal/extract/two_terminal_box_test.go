package extract

import (
	"bytes"
	"testing"

	"github.com/PVKonovalov/slddoc/internal/model"
	"github.com/PVKonovalov/slddoc/internal/render"
)

const twoTerminalBoxTemplate = `<path d="{terminalAxis:M 0 -20 v 12 M 0 8 v 12|M -20 0 h 6 M 14 0 h 6}" style="fill:none;stroke:{color};stroke-width:2" />
<path d="M -14 -8 h 28 v 16 h -28 z" style="fill:none;stroke:{color};stroke-width:2" />
<text x="0" y="0" style="fill:{color};font-size:9px;font-family:Arial;text-anchor:middle;dominant-baseline:middle" >{text:}</text>`

// TestExtractTwoTerminalBox reads a Two-terminal box back from the editor's own rendering: its anchor, terminal
// position (above and below, or left and right) and label.
func TestExtractTwoTerminalBox(t *testing.T) {
	lib := render.NewSymbolLibrary(map[string]string{"330004": twoTerminalBoxTemplate})
	for _, text := range []string{"", "AB <1>"} {
		for _, dir := range []model.TerminalDirection{"", model.TerminalLeft} {
			d := &model.Diagram{Width: 200, Height: 200, Elements: []model.Element{{
				ID: 1, Class: model.ClassTwoTerminalBox, Shape: "330004", X: 100, Y: 100, Terminal: dir,
				Ports: []model.Port{{Name: "1"}, {Name: "2"}}, PropertyText: text,
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
			if el.Class != model.ClassTwoTerminalBox || el.Orient != 0 || el.Terminal != dir || el.X != 100 || el.Y != 100 || el.PropertyText != text {
				t.Errorf("text %q terminal %q: got %s orient %d terminal %q at (%v,%v) text %q", text, dir, el.Class, el.Orient, el.Terminal, el.X, el.Y, el.PropertyText)
			}
		}
	}
}

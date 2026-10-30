package extract

import (
	"bytes"
	"strings"
	"testing"

	"github.com/PVKonovalov/slddoc/internal/model"
	"github.com/PVKonovalov/slddoc/internal/render"
)

// TestExtract_LabelUnderlineItalic reads xsde2svg's underlined and italic captions (Modus fontStyle
// [BOLD,UNDERLINE] and [ITALIC]), draws them back the way xsde2svg does and keeps them through Save/Load.
func TestExtract_LabelUnderlineItalic(t *testing.T) {
	const svg = `<?xml version="1.0"?>
<svg width="600" height="400" xmlns="http://www.w3.org/2000/svg">
<text x="136" y="126" style="fill:black;text-anchor:start;font-size:18px;font-family:Arial;font-weight: bold;text-decoration: underline;white-space: pre;"  id="21" data-event="dc" data-voltage="black" >Section A</text>
<text x="8" y="300" style="fill:black;text-anchor:start;font-size:13px;font-family:Arial;font-style:italic;white-space: pre;" id="22" data-event="dc" data-voltage="black" >Note</text>
</svg>`
	d, _, err := Extract([]byte(svg), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Labels) != 2 {
		t.Fatalf("labels = %+v, want two", d.Labels)
	}
	a, b := d.Labels[0], d.Labels[1]
	if !a.Bold || !a.Underline || a.Italic || b.Bold || b.Underline || !b.Italic {
		t.Fatalf("styles: %+v / %+v", a, b)
	}

	var out bytes.Buffer
	if err := render.Render(d, render.NewSymbolLibrary(nil), &out, render.Static, "", nil); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"font-weight: bold;text-decoration: underline;white-space: pre;", "font-family:Arial;font-style:italic;white-space: pre;"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("rendered SVG lacks %q:\n%s", want, out.String())
		}
	}

	var xml bytes.Buffer
	if err := d.Save(&xml); err != nil {
		t.Fatal(err)
	}
	back, err := model.Load(&xml)
	if err != nil {
		t.Fatal(err)
	}
	if !back.Labels[0].Underline || !back.Labels[1].Italic {
		t.Errorf("after Save/Load: %+v", back.Labels)
	}
}

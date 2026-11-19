package extract

import (
	"bytes"
	"strings"
	"testing"

	"github.com/PVKonovalov/slddoc/internal/render"
)

// buttonSVG is Buttons (113) as xsde2svg 1.4.29 draws them: 1 without a font size (12px), 2-5 at font sizes 10, 12,
// 14 and 15 (17, 20, 23 and 25px), 6 at 14 and bold.
const buttonSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="1000" height="700">
<g id="1" data-type="113" data-voltage="#30d5c8" >
<rect x="250" y="50" width="60" height="30" style="fill:#30d5c8;stroke:white;stroke-width:1" />
<text x="280" y="67" style="fill:navy;text-anchor:middle;dominant-baseline:middle;font-size:12px;font-family:Arial " >Тест</text>
</g>
<g id="2" data-type="113" data-voltage="pink" >
<rect x="630" y="410" width="150" height="20" style="fill:pink;stroke:white;stroke-width:1" />
<text x="705" y="422" style="fill:darkred;text-anchor:middle;dominant-baseline:middle;font-size:17px;font-family:Arial " >Шрифт 10</text>
</g>
<g id="3" data-type="113" data-voltage="lightcyan" >
<rect x="630" y="460" width="150" height="30" style="fill:lightcyan;stroke:white;stroke-width:1" />
<text x="705" y="477" style="fill:cyan;text-anchor:middle;dominant-baseline:middle;font-size:20px;font-family:Arial " >Шрифт 12</text>
</g>
<g id="4" data-type="113" data-voltage="lightgreen" >
<rect x="630" y="520" width="150" height="40" style="fill:lightgreen;stroke:white;stroke-width:1" />
<text x="705" y="542" style="fill:#009B77;text-anchor:middle;dominant-baseline:middle;font-size:23px;font-family:Arial " >Шрифт 14</text>
</g>
<g id="5" data-type="113" data-voltage="lightskyblue" >
<rect x="630" y="590" width="150" height="50" style="fill:lightskyblue;stroke:white;stroke-width:1" />
<text x="705" y="617" style="fill:indigo;text-anchor:middle;dominant-baseline:middle;font-size:25px;font-family:Arial " >Шрифт 15</text>
</g>
<g id="6" data-type="113" data-voltage="#EAEAEA" >
<rect x="620" y="50" width="240" height="40" style="fill:#EAEAEA;stroke:white;stroke-width:1" />
<text x="740" y="72" style="fill:#12161d;text-anchor:middle;dominant-baseline:middle;font-size:23px;font-family:Arial ;font-weight: bold" >События по фидеру</text>
</g>
</svg>`

// TestExtractButtonTextSize reads a button's text size, and reads it back from the editor's own rendering.
func TestExtractButtonTextSize(t *testing.T) {
	want := []float64{12, 17, 20, 0, 25, 0}
	d, report, err := Extract([]byte(buttonSVG), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Failed) != 0 || len(d.Elements) != len(want) {
		t.Fatalf("report = %+v, elements = %d", report, len(d.Elements))
	}
	for i, w := range want {
		if el := d.Elements[i]; el.TextSize != w {
			t.Errorf("button %d text size %v, want %v", i+1, el.TextSize, w)
		}
	}
	if !d.Elements[5].Bold {
		t.Error("button 6 is not bold")
	}

	var buf bytes.Buffer
	if err := render.Render(d, render.NewSymbolLibrary(nil), &buf, render.Static, "", nil); err != nil {
		t.Fatal(err)
	}
	for _, size := range []string{"font-size:12px", "font-size:17px", "font-size:20px", "font-size:23px", "font-size:25px"} {
		if !strings.Contains(buf.String(), size) {
			t.Errorf("rendering lacks %s", size)
		}
	}
	back, _, err := Extract(buf.Bytes(), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	for i, w := range want {
		if el := back.Elements[i]; el.TextSize != w {
			t.Errorf("round trip button %d text size %v, want %v", i+1, el.TextSize, w)
		}
	}
}

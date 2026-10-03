package slddoc

import (
	"bytes"
	"strings"
	"testing"
)

// A real corpus breaker (poves.svg): body 28 wide (size step 2), leads
// drawn out to terminals 60 apart, beyond the library's 40 at that step.
const longLeadBreaker = `<svg xmlns="http://www.w3.org/2000/svg" width="500" height="500">
<g id="148706743" data-voltage="#00A0F0" data-type="41" data-name="В-110 л.Лч-3" >
<g  >
<path d="M 336 271 h 28 v 28 h -28 z" data-fill="0:red,1:lawngreen,2:yellow" style="fill:lawngreen;stroke:#00A0F0;stroke-width:2" />
<path d="M 350 273 v 24" data-state="1" style="stroke:#00A0F0;stroke-width:2" />
<path d="M 350 255 v 16 M 350 315 v -16" style="stroke:#00A0F0;stroke-width:2" />
</g>
</g>
</svg>`

func TestExtractLeadShapeScaleAndSpan(t *testing.T) {
	d, _, err := Extract([]byte(longLeadBreaker), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Elements) != 1 {
		t.Fatalf("elements = %d, want 1", len(d.Elements))
	}
	el := d.Elements[0]
	if el.Scale != 2 || el.Span != 60 {
		t.Errorf("scale/span = %d/%v, want 2/60", el.Scale, el.Span)
	}
	for _, tc := range []struct{ t, want Point }{
		{Point{0, -10}, Point{0, -30}},
		{Point{0, 10}, Point{0, 30}},
	} {
		if got := LeadTerminal(el, tc.t); got != tc.want {
			t.Errorf("LeadTerminal(%v) = %v, want %v", tc.t, got, tc.want)
		}
	}
}

func TestExtractLeadShapeDefaultSpanLeftUnset(t *testing.T) {
	// The same breaker at step 0 with the library's own 20 spacing.
	svg := `<svg xmlns="http://www.w3.org/2000/svg" width="500" height="500">
<g id="7" data-voltage="#00A0F0" data-type="41" data-name="B" >
<path d="M 343 278 h 14 v 14 h -14 z" style="fill:red;stroke:#00A0F0;stroke-width:1" />
<path d="M 350 280 v 10" data-state="1" style="stroke:#00A0F0;stroke-width:1" />
<path d="M 350 275 v 3 M 350 295 v -3" style="stroke:#00A0F0;stroke-width:1" />
</g>
</svg>`
	d, _, err := Extract([]byte(svg), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if el := d.Elements[0]; el.Scale != 0 || el.Span != 0 {
		t.Errorf("scale/span = %d/%v, want 0/0", el.Scale, el.Span)
	}
}

func TestRenderLeadExtension(t *testing.T) {
	lib := NewSymbolLibrary(map[string]string{"41": `<path d="M 0 -10 v 3 M 0 10 v -3" style="stroke:{color};stroke-width:1" />`})
	d := &Diagram{Width: 500, Height: 500, Elements: []Element{
		{ID: 1, Class: ClassBreaker, Shape: "41", X: 350, Y: 285, Scale: 2, Span: 60},
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

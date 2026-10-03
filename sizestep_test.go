package slddoc

import (
	"bytes"
	"strings"
	"testing"
)

func TestScaledTerminal(t *testing.T) {
	// Wire jump (shape 14) terminals at ±10, as the source draws its legs
	// (Scale(s, 10)) at each step seen in the corpus.
	for step, want := range map[int]float64{0: 10, 1: 14, 2: 20, 3: 28, -1: 7, -2: 5} {
		got := ScaledTerminal(Point{X: -10, Y: 10}, step)
		if got.X != -want || got.Y != want {
			t.Errorf("step %d: got %v, want ±%v", step, got, want)
		}
	}
}

func TestRenderSizeStep(t *testing.T) {
	lib := NewSymbolLibrary(map[string]string{
		"14": `<path d="M 10 0 L 3 0 A 3 3 0 0 0 -3 0 L -10 0" style="fill:none;stroke:{color};stroke-width:1" />`,
	})
	d := &Diagram{Width: 100, Height: 100, Elements: []Element{
		{ID: 1, Class: ClassNonIntersection, Shape: "14", X: 270, Y: 450, Orient: 90, Scale: 2},
	}}
	for _, tc := range []struct {
		mode RenderMode
		want []string
	}{
		{Interactive, []string{`transform="translate(270,450) rotate(90) scale(2)"`, `stroke-width:0.5`}},
		{Static, []string{`transform="rotate(90,270,450) translate(-270,-450) scale(2)"`, `M 280 450`, `stroke-width:0.5`}},
	} {
		var buf bytes.Buffer
		if err := Render(d, lib, &buf, tc.mode, "", nil); err != nil {
			t.Fatal(err)
		}
		for _, w := range tc.want {
			if !strings.Contains(buf.String(), w) {
				t.Errorf("mode %v: missing %q in\n%s", tc.mode, w, buf.String())
			}
		}
	}
}

func TestSizeStepIgnoredByCustomDrawnClasses(t *testing.T) {
	if UsesSizeStep(ClassPowerTransformer) || UsesSizeStep(ClassBusBarSection) {
		t.Error("custom-drawn classes must not use a size step")
	}
	if !UsesSizeStep(ClassNonIntersection) || !UsesSizeStep(ClassThyristor) {
		t.Error("template classes must use a size step")
	}
	// Every custom-drawn class bypasses the template path, so its own
	// output never carries the size step's scale().
	for c := range customDrawnClasses {
		d := &Diagram{Width: 100, Height: 100, Elements: []Element{
			{ID: 1, Class: c, Shape: "x", X: 10, Y: 10, Scale: 2,
				Points: []Point{{X: 0, Y: 0}, {X: 20, Y: 20}}},
		}}
		var buf bytes.Buffer
		if err := Render(d, NewSymbolLibrary(map[string]string{"x": `<path d="M 0 0 h 1" />`}), &buf, Interactive, "", nil); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(buf.String(), "scale(2)") {
			t.Errorf("%s: custom-drawn class rendered with the size step", c)
		}
	}
}

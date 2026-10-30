package model

import (
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

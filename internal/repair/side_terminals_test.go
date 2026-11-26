package repair

import (
	"slices"
	"testing"

	"github.com/PVKonovalov/slddoc/internal/model"
)

// TestSymbolTerminalsSideTerminals puts a Two-terminal box's terminals above and below it, or left and right of it
// when its Terminal is left or right.
func TestSymbolTerminalsSideTerminals(t *testing.T) {
	terminals := map[string][]model.Point{"330004": {{X: 0, Y: -20}, {X: 0, Y: 20}}}
	cases := []struct {
		dir  model.TerminalDirection
		want []model.Point
	}{
		{"", []model.Point{{X: 100, Y: 80}, {X: 100, Y: 120}}},
		{model.TerminalLeft, []model.Point{{X: 80, Y: 100}, {X: 120, Y: 100}}},
		{model.TerminalRight, []model.Point{{X: 80, Y: 100}, {X: 120, Y: 100}}},
	}
	for _, c := range cases {
		el := model.Element{Class: model.ClassTwoTerminalBox, Shape: "330004", X: 100, Y: 100, Terminal: c.dir}
		if got := SymbolTerminals(&el, terminals); !slices.Equal(got, c.want) {
			t.Errorf("terminal %q: got %v, want %v", c.dir, got, c.want)
		}
	}
}

// TestSymbolTerminalsUprightTerminal puts a Device's terminal on the side its Terminal names, whatever its Orient and
// Mirror, scaled by its size step.
func TestSymbolTerminalsUprightTerminal(t *testing.T) {
	terminals := map[string][]model.Point{"130": {{X: 0, Y: -20}}}
	cases := []struct {
		dir    model.TerminalDirection
		orient int
		mirror bool
		scale  int
		want   model.Point
	}{
		{"", 0, false, 0, model.Point{X: 100, Y: 80}},
		{model.TerminalTop, 90, true, 0, model.Point{X: 100, Y: 80}},
		{model.TerminalRight, 180, false, 0, model.Point{X: 120, Y: 100}},
		{model.TerminalBottom, -90, false, 0, model.Point{X: 100, Y: 120}},
		{model.TerminalLeft, 0, false, 0, model.Point{X: 80, Y: 100}},
		{model.TerminalBottom, 0, false, 2, model.Point{X: 100, Y: 140}},
	}
	for _, c := range cases {
		el := model.Element{Class: model.ClassDevice, Shape: "130", X: 100, Y: 100, Terminal: c.dir, Orient: c.orient, Mirror: c.mirror, Scale: c.scale}
		got := SymbolTerminals(&el, terminals)
		if len(got) != 1 || RoundPt(got[0]) != c.want {
			t.Errorf("terminal %q orient %d: got %v, want %v", c.dir, c.orient, got, c.want)
		}
	}
}

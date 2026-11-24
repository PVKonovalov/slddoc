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

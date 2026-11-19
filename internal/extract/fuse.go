package extract

import (
	"math"

	"github.com/PVKonovalov/slddoc/internal/model"
	"github.com/PVKonovalov/slddoc/internal/render"
	"github.com/PVKonovalov/slddoc/internal/svgtree"
)

// fuseTipHalf is a withdrawable fuse's (154) distance from its anchor to an outer chevron tip at step 0 in xsde2svg.
const fuseTipHalf = 31

// Withdrawable fuse positions, as xsde2svg's data-trolley.
const (
	trolleyService = 0
	trolleyTest    = 2
)

// readWithdrawableFuse sets a withdrawable fuse's (154) Position and Scale. n is its markup; tips are its two outer
// chevron tips and anchor its center, in the drawing's own (unrotated) frame. Position comes from data-trolley, or
// for xsde2svg markup without one (a single <path>) from the drawing: a body shifted off the axis is Service, inner
// chevrons pointing away from the body are Test. Scale comes from a scale() in the transform, else from the tips.
func readWithdrawableFuse(el *model.Element, n *svgtree.RawNode, tips [2]model.Point, anchor model.Point) {
	el.Position = parsePosition(n)
	if el.Position == nil {
		el.Position = fusePositionFromDrawing(n, anchor)
	}

	if ops, err := render.ParseTransformList(n.Attr("transform")); err == nil {
		for _, op := range ops {
			if op.Name == "scale" && len(op.Args) == 1 && op.Args[0] > 0 {
				el.Scale = clampStep(int(math.Round(2 * math.Log2(op.Args[0]))))
				return
			}
		}
	}
	half := math.Hypot(tips[1].X-tips[0].X, tips[1].Y-tips[0].Y) / 2
	if half > 0 {
		el.Scale = clampStep(int(math.Round(2 * math.Log2(half/fuseTipHalf))))
	}
}

func clampStep(step int) int {
	return min(max(step, render.MinSizeStep), render.MaxSizeStep)
}

// fusePositionFromDrawing reads the position of a fuse drawn as one <path>: body, outer chevrons, inner chevrons. The
// body starts at its top; the inner chevrons start at the same point. Returns nil for the default position.
func fusePositionFromDrawing(n *svgtree.RawNode, anchor model.Point) *int {
	paths := elementPaths(n)
	if len(paths) != 1 {
		return nil
	}
	subs, err := model.ParseSubpaths(paths[0].Attr("d"))
	if err != nil || len(subs) < 2 || len(subs[0]) == 0 {
		return nil
	}
	start := subs[0][0]
	if math.Abs(start.X-anchor.X) > 0.5 {
		p := trolleyService
		return &p
	}
	for _, sp := range subs[1:] {
		if len(sp) >= 2 && math.Hypot(sp[0].X-start.X, sp[0].Y-start.Y) <= 0.5 {
			if sp[1].Y < sp[0].Y {
				p := trolleyTest
				return &p
			}
			return nil
		}
	}
	return nil
}

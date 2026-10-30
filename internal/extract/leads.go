package extract

import (
	"math"
	"strconv"

	"github.com/PVKonovalov/slddoc/internal/model"
	"github.com/PVKonovalov/slddoc/internal/render"
	"github.com/PVKonovalov/slddoc/internal/svgtree"
)

// bodyHalfLength measures how far a two-terminal device's drawn body
// reaches along its own lead axis (local y, before rotation) from anchor:
// the largest |y - anchor.Y| over every drawn vertex lying off that axis
// (|x - anchor.X| >= 1). The leads themselves lie on the axis, so this is
// the body alone, whatever the per-instance lead length. Coordinates are
// the source's own unrotated ones (its rotate() only applies on display).
func bodyHalfLength(n *svgtree.RawNode, anchor model.Point) (float64, bool) {
	best, found := 0.0, false
	consider := func(p model.Point) {
		if math.Abs(p.X-anchor.X) < 1 {
			return
		}
		if h := math.Abs(p.Y - anchor.Y); h > best || !found {
			best, found = h, true
		}
	}
	for _, p := range elementPaths(n) {
		subs, err := model.ParseSubpaths(p.Attr("d"))
		if err != nil {
			continue
		}
		for _, sp := range subs {
			for _, q := range sp {
				consider(q)
			}
		}
	}
	for _, r := range n.Descendants("rect") {
		x, _ := strconv.ParseFloat(r.Attr("x"), 64)
		y, _ := strconv.ParseFloat(r.Attr("y"), 64)
		w, _ := strconv.ParseFloat(r.Attr("width"), 64)
		h, _ := strconv.ParseFloat(r.Attr("height"), 64)
		consider(model.Point{X: x, Y: y})
		consider(model.Point{X: x + w, Y: y + h})
	}
	return best, found
}

// inferLeads sets a lead shape's Scale from its drawn body and its Span from
// its two port points (left 0 when that is just the library's own spacing at
// that step). n is the element's source markup.
func inferLeads(el *model.Element, n *svgtree.RawNode, ports []model.Point) {
	ls, ok := render.LeadShapes[el.Shape]
	if !ok || len(ports) != 2 || n == nil {
		return
	}
	if h, ok := bodyHalfLength(n, model.Point{X: el.X, Y: el.Y}); ok && h > 0 {
		step := int(math.Round(2 * math.Log2(h/ls.BodyHalf)))
		el.Scale = min(max(step, render.MinSizeStep), render.MaxSizeStep)
	}
	span := math.Hypot(ports[0].X-ports[1].X, ports[0].Y-ports[1].Y)
	if span > 2*model.ScaledLength(ls.Terminal, el.Scale)+0.5 {
		el.Span = span
	}
}

package slddoc

import (
	"math"
	"strconv"
)

// bodyHalfLength measures how far a two-terminal device's drawn body
// reaches along its own lead axis (local y, before rotation) from anchor:
// the largest |y - anchor.Y| over every drawn vertex lying off that axis
// (|x - anchor.X| >= 1). The leads themselves lie on the axis, so this is
// the body alone, whatever the per-instance lead length. Coordinates are
// the source's own unrotated ones (its rotate() only applies on display).
func bodyHalfLength(n *rawNode, anchor Point) (float64, bool) {
	best, found := 0.0, false
	consider := func(p Point) {
		if math.Abs(p.X-anchor.X) < 1 {
			return
		}
		if h := math.Abs(p.Y - anchor.Y); h > best || !found {
			best, found = h, true
		}
	}
	for _, p := range elementPaths(n) {
		subs, err := parseSubpaths(p.attr("d"))
		if err != nil {
			continue
		}
		for _, sp := range subs {
			for _, q := range sp {
				consider(q)
			}
		}
	}
	for _, r := range n.descendants("rect") {
		x, _ := strconv.ParseFloat(r.attr("x"), 64)
		y, _ := strconv.ParseFloat(r.attr("y"), 64)
		w, _ := strconv.ParseFloat(r.attr("width"), 64)
		h, _ := strconv.ParseFloat(r.attr("height"), 64)
		consider(Point{x, y})
		consider(Point{x + w, y + h})
	}
	return best, found
}

// leadShape describes a two-terminal device whose source draws its leads
// to a per-instance length: terminals Scale(s, max(2, Distance)·10) apart
// (e.g. element_41.go's sdeDistanceInSvgInScale), the body Scale(s, ...)
// in between. Element.Span holds that terminal distance.
type leadShape struct {
	// bodyHalf is the body's own half-length along the lead axis at size
	// step 0, as bodyHalfLength measures it (calibrated on the corpus for
	// 41/42/162/164/203; the library template's for the rest).
	bodyHalf float64
	// terminal is |y| of the library template's two terminals (0,±terminal),
	// where its own drawn leads end.
	terminal float64
}

var leadShapes = map[string]leadShape{
	"41":  {bodyHalf: 7, terminal: 10},
	"42":  {bodyHalf: 8, terminal: 10},
	"71":  {bodyHalf: 9, terminal: 10},
	"162": {bodyHalf: 9, terminal: 10},
	"163": {bodyHalf: 10, terminal: 10},
	"164": {bodyHalf: 10, terminal: 10},
	"166": {bodyHalf: 9, terminal: 10},
	"203": {bodyHalf: 10, terminal: 10},
	"399": {bodyHalf: 9, terminal: 10},
}

// HasLeads reports whether shape draws its leads to Element.Span.
func HasLeads(shape string) bool {
	_, ok := leadShapes[shape]
	return ok
}

// sizeStepRange is the size steps Extract considers (the editor's own
// selector offers the same).
const minSizeStep, maxSizeStep = -2, 4

// inferLeads sets a lead shape's Scale from its drawn body and its Span from
// its two port points (left 0 when that is just the library's own spacing at
// that step). n is the element's source markup.
func inferLeads(el *Element, n *rawNode, ports []Point) {
	ls, ok := leadShapes[el.Shape]
	if !ok || len(ports) != 2 || n == nil {
		return
	}
	if h, ok := bodyHalfLength(n, Point{X: el.X, Y: el.Y}); ok && h > 0 {
		step := int(math.Round(2 * math.Log2(h/ls.bodyHalf)))
		el.Scale = min(max(step, minSizeStep), maxSizeStep)
	}
	span := math.Hypot(ports[0].X-ports[1].X, ports[0].Y-ports[1].Y)
	if span > 2*scaledLength(ls.terminal, el.Scale)+0.5 {
		el.Span = span
	}
}

// leadExtension is the extra path a lead shape with a Span draws from its
// template's own terminals (0,±terminal) out to (0,±Span/2), in the
// template's local units (divided by the size step's factor, since it is
// drawn under its scale()). "" when the Span doesn't reach past them.
func leadExtension(e Element, color string) string {
	ls, ok := leadShapes[e.Shape]
	if !ok || e.Span <= 0 {
		return ""
	}
	half := e.Span / 2 / SizeFactor(e.Scale)
	if half <= ls.terminal+1e-9 {
		return ""
	}
	t, h := fmtNum(ls.terminal), fmtNum(round6(half))
	return "\n<path d=\"M 0 -" + t + " V -" + h + " M 0 " + t + " V " + h + "\" style=\"fill:none;stroke:" + color + ";stroke-width:1\" />"
}

// LeadTerminal is where a lead shape's library terminal t (local, step-0)
// sits once its Span and size step apply: ScaledTerminal, pushed out along
// the lead axis to ±Span/2 when the Span reaches past it.
func LeadTerminal(e Element, t Point) Point {
	p := ScaledTerminal(t, e.Scale)
	if HasLeads(e.Shape) && e.Span/2 > math.Abs(p.Y) && p.Y != 0 {
		p.Y = math.Copysign(e.Span/2, p.Y)
	}
	return p
}

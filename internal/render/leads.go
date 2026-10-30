package render

import (
	"math"

	"github.com/PVKonovalov/slddoc/internal/model"
)

// leadShape describes a two-terminal device whose source draws its leads
// to a per-instance length: terminals Scale(s, max(2, Distance)·10) apart
// (e.g. element_41.go's sdeDistanceInSvgInScale), the body Scale(s, ...)
// in between. Element.Span holds that terminal distance.
type leadShape struct {
	// BodyHalf is the body's own half-length along the lead axis at size
	// step 0, as bodyHalfLength measures it (calibrated on the corpus for
	// 41/42/162/164/203; the library template's for the rest).
	BodyHalf float64
	// Terminal is |y| of the library template's two terminals (0,±terminal),
	// where its own drawn leads end.
	Terminal float64
}

var LeadShapes = map[string]leadShape{
	"41":  {BodyHalf: 7, Terminal: 10},
	"42":  {BodyHalf: 8, Terminal: 10},
	"71":  {BodyHalf: 9, Terminal: 10},
	"162": {BodyHalf: 9, Terminal: 10},
	"163": {BodyHalf: 10, Terminal: 10},
	"164": {BodyHalf: 10, Terminal: 10},
	"166": {BodyHalf: 9, Terminal: 10},
	"203": {BodyHalf: 10, Terminal: 10},
	"399": {BodyHalf: 9, Terminal: 10},
}

// HasLeads reports whether shape draws its leads to Element.Span.
func HasLeads(shape string) bool {
	_, ok := LeadShapes[shape]
	return ok
}

// sizeStepRange is the size steps Extract considers (the editor's own
// selector offers the same).
const MinSizeStep, MaxSizeStep = -2, 4

// leadExtension is the extra path a lead shape with a Span draws from its
// template's own terminals (0,±terminal) out to (0,±Span/2), in the
// template's local units (divided by the size step's factor, since it is
// drawn under its scale()). "" when the Span doesn't reach past them.
func leadExtension(e model.Element, color string) string {
	ls, ok := LeadShapes[e.Shape]
	if !ok || e.Span <= 0 {
		return ""
	}
	half := e.Span / 2 / model.SizeFactor(e.Scale)
	if half <= ls.Terminal+1e-9 {
		return ""
	}
	t, h := model.FmtNum(ls.Terminal), model.FmtNum(model.Round6(half))
	return "\n<path d=\"M 0 -" + t + " V -" + h + " M 0 " + t + " V " + h + "\" style=\"fill:none;stroke:" + color + ";stroke-width:1\" />"
}

// LeadTerminal is where a lead shape's library terminal t (local, step-0)
// sits once its Span and size step apply: ScaledTerminal, pushed out along
// the lead axis to ±Span/2 when the Span reaches past it.
func LeadTerminal(e model.Element, t model.Point) model.Point {
	p := model.ScaledTerminal(t, e.Scale)
	if HasLeads(e.Shape) && e.Span/2 > math.Abs(p.Y) && p.Y != 0 {
		p.Y = math.Copysign(e.Span/2, p.Y)
	}
	return p
}

package slddoc

import (
	"math"
	"regexp"
	"strconv"
)

// Element.Scale, the real xsde2svg per-element size step
// (internal/modus/helpers.go's Scale(scale, value) = int(value·√2^scale)).

// sizeStepEpsilon nudges v·√2^s away from zero before truncating, so a
// product that is a whole number in exact arithmetic (10·√2² = 20) isn't
// truncated one short by floating-point noise (19.999…).
const sizeStepEpsilon = 1e-9

// SizeFactor is a size step's own scale factor, √2^step.
func SizeFactor(step int) float64 {
	return math.Pow(math.Sqrt2, float64(step))
}

// scaledLength is the source's Scale(step, v): v·√2^step truncated toward
// zero, as Go's int() conversion does.
func scaledLength(v float64, step int) float64 {
	if step == 0 {
		return v
	}
	p := v * SizeFactor(step)
	if p < 0 {
		return math.Trunc(p - sizeStepEpsilon)
	}
	return math.Trunc(p + sizeStepEpsilon)
}

// ScaledTerminal is where a template symbol's terminal t (local, step-0
// coordinates) sits at the given size step: each coordinate scaled the
// source's own truncating way, so an imported wire end lands on it exactly.
func ScaledTerminal(t Point, step int) Point {
	return Point{X: scaledLength(t.X, step), Y: scaledLength(t.Y, step)}
}

// customDrawnClasses are the classes Render draws with dedicated code
// rather than their symbol library template (renderElementLocal's own
// early returns). Their geometry has its own size fields, so Scale doesn't
// apply to them.
var customDrawnClasses = map[Class]bool{
	ClassBusBarSection:      true,
	ClassPowerTransformer:   true,
	ClassPackageSubstation:  true,
	ClassPowerPlant:         true,
	ClassSubstation:         true,
	ClassEnclosedSubstation: true,
	ClassRectangle:          true,
	ClassArrow:              true,
	ClassCircle:             true,
	ClassButton:             true,
	ClassContainer:          true,
	ClassSmallWindow:        true,
	ClassPicture:            true,
	ClassAutomationDevice:   true,
	ClassWindowIcon:         true,
	ClassRoad:               true,
	ClassConnectorArrow:     true,
	ClassPostPole:           true,
	ClassLine:               true,
	ClassArc:                true,
	ClassPolygon:            true,
	ClassPowerflowIndicator: true,
	ClassTable:              true,
	ClassTable2:             true,
}

// UsesSizeStep reports whether Element.Scale applies to class c: true for
// every class drawn from its symbol library template.
func UsesSizeStep(c Class) bool {
	return !customDrawnClasses[c]
}

var strokeWidthRe = regexp.MustCompile(`(stroke-width\s*[:=]\s*"?)([0-9]*\.?[0-9]+)`)

// sizeStepTemplate prepares a template body drawn under an extra
// scale(factor): every stroke-width is divided by factor, so lines keep the
// width the source draws at any size step.
func sizeStepTemplate(body string, factor float64) string {
	return strokeWidthRe.ReplaceAllStringFunc(body, func(m string) string {
		sub := strokeWidthRe.FindStringSubmatch(m)
		v, err := strconv.ParseFloat(sub[2], 64)
		if err != nil {
			return m
		}
		return sub[1] + fmtNum(round6(v/factor))
	})
}

// sizeStepScale is the extra transform component for e's size step ("" at
// step 0).
func sizeStepScale(e Element) string {
	if e.Scale == 0 {
		return ""
	}
	return " scale(" + fmtNum(round6(SizeFactor(e.Scale))) + ")"
}

// round6 rounds to 6 decimals, for writing a size factor (√2² is
// 2.0000000000000004 in floating point).
func round6(v float64) float64 {
	return math.Round(v*1e6) / 1e6
}

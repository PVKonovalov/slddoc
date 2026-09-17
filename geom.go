package slddoc

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
)

var pathTokenRe = regexp.MustCompile(`[MmLlHhVvZzAa]|-?\d+(?:\.\d+)?`)

// parseSubpaths interprets a minimal subset of SVG path data — M/m, L/l,
// H/h, V/v, Z/z — sufficient for the symbol shapes xsde2svg draws for the
// element classes this package understands. It returns each subpath (one
// per M/m command) as its full sequence of points, in order. It is not a
// general SVG path parser: an arc, curve, or shorthand-repeated coordinate
// group is reported as an error rather than silently mishandled.
func parseSubpaths(d string) ([][]Point, error) {
	toks := pathTokenRe.FindAllString(d, -1)

	var subpaths [][]Point
	var cur []Point
	var x, y, startX, startY float64
	i := 0

	next := func() (float64, error) {
		if i >= len(toks) || isCommandLetter(toks[i]) {
			return 0, fmt.Errorf("slddoc: path %q: expected a number at token %d", d, i)
		}
		v, err := strconv.ParseFloat(toks[i], 64)
		if err != nil {
			return 0, fmt.Errorf("slddoc: path %q: %w", d, err)
		}
		i++
		return v, nil
	}

	for i < len(toks) {
		cmd := toks[i]
		i++
		switch cmd {
		case "M", "m":
			dx, err := next()
			if err != nil {
				return nil, err
			}
			dy, err := next()
			if err != nil {
				return nil, err
			}
			if cmd == "m" {
				x, y = x+dx, y+dy
			} else {
				x, y = dx, dy
			}
			startX, startY = x, y
			if cur != nil {
				subpaths = append(subpaths, cur)
			}
			cur = []Point{{X: x, Y: y}}
		case "L", "l":
			dx, err := next()
			if err != nil {
				return nil, err
			}
			dy, err := next()
			if err != nil {
				return nil, err
			}
			if cmd == "l" {
				x, y = x+dx, y+dy
			} else {
				x, y = dx, dy
			}
			cur = append(cur, Point{X: x, Y: y})
		case "H", "h":
			dx, err := next()
			if err != nil {
				return nil, err
			}
			if cmd == "h" {
				x += dx
			} else {
				x = dx
			}
			cur = append(cur, Point{X: x, Y: y})
		case "V", "v":
			dy, err := next()
			if err != nil {
				return nil, err
			}
			if cmd == "v" {
				y += dy
			} else {
				y = dy
			}
			cur = append(cur, Point{X: x, Y: y})
		case "Z", "z":
			x, y = startX, startY
			if len(cur) > 0 {
				cur = append(cur, Point{X: x, Y: y})
			}
		case "A", "a":
			// Only the endpoint is tracked, not the true elliptical curve:
			// extraction only ever needs an arc's start/end points (to find
			// a device's electrical extremities), never its rendered
			// shape, since rendering replays the original template text
			// verbatim rather than reconstructing it from stored points.
			for i := 0; i < 5; i++ { // rx, ry, x-axis-rotation, large-arc-flag, sweep-flag
				if _, err := next(); err != nil {
					return nil, err
				}
			}
			dx, err := next()
			if err != nil {
				return nil, err
			}
			dy, err := next()
			if err != nil {
				return nil, err
			}
			if cmd == "a" {
				x, y = x+dx, y+dy
			} else {
				x, y = dx, dy
			}
			cur = append(cur, Point{X: x, Y: y})
		default:
			return nil, fmt.Errorf("slddoc: path %q: unsupported command %q", d, cmd)
		}
	}
	if cur != nil {
		subpaths = append(subpaths, cur)
	}
	return subpaths, nil
}

func isCommandLetter(tok string) bool {
	switch tok {
	case "M", "m", "L", "l", "H", "h", "V", "v", "Z", "z", "A", "a":
		return true
	}
	return false
}

// rotate returns p rotated by angleDeg degrees around center, following the
// same clockwise-in-a-Y-down-plane convention as SVG's rotate(angle,cx,cy)
// transform. The result is rounded to the nearest integer: every coordinate
// in this format is an integer (xsde2svg draws on a ×10 grid), so rounding
// away floating-point rotation noise (e.g. cos(90°) landing on 6e-17
// instead of 0) is always correct, never a loss of precision.
func rotate(p, center Point, angleDeg float64) Point {
	if angleDeg == 0 {
		return p
	}
	rad := angleDeg * math.Pi / 180
	dx, dy := p.X-center.X, p.Y-center.Y
	sin, cos := math.Sin(rad), math.Cos(rad)
	return Point{
		X: math.Round(center.X + dx*cos - dy*sin),
		Y: math.Round(center.Y + dx*sin + dy*cos),
	}
}

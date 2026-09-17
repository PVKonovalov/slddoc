package slddoc

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
)

// pathTokenRe also matches every curve command letter (C/S/Q/T and their
// lowercase forms), even though parseSubpaths itself doesn't implement any
// of them — so that one shows up as a real, recognizable token hitting the
// "unsupported command" case below, rather than silently vanishing from
// the token stream the way an unmatched letter would, which would leave
// its own numeric arguments looking like more of whatever command
// preceded it (an easy, silent misparse once moreArgs' shorthand-repeat
// convention was added, since a stray number no longer reliably ends a
// command the way it used to).
var pathTokenRe = regexp.MustCompile(`[MmLlHhVvZzAaCcSsQqTt]|-?\d+(?:\.\d+)?`)

// parseSubpaths interprets a minimal subset of SVG path data — M/m, L/l,
// H/h, V/v, Z/z, A/a — sufficient for the symbol shapes xsde2svg draws for
// the element classes this package understands, including its own
// shorthand-repeated-coordinate-group convention (e.g. "h -15 0" is two
// horizontal linetos, "M x y x2 y2" is a moveto followed by an implicit
// lineto) — every command below repeats for as long as another parameter
// group follows without a fresh command letter. It returns each subpath
// (one per M/m command) as its full sequence of points, in order. It is
// not a general SVG path parser: a curve command (C/S/Q/T) is reported as
// an error rather than silently mishandled, and an arc's own true
// elliptical shape isn't reconstructed — only its endpoint is tracked (see
// the "A", "a" case below).
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

	// moreArgs reports whether another parameter group follows the one
	// just consumed, with no fresh command letter in between — SVG's own
	// shorthand-repeat convention (see the doc comment above).
	moreArgs := func() bool {
		return i < len(toks) && !isCommandLetter(toks[i])
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
			// A moveto's own repeated coordinate pairs are implicit
			// linetos, not additional movetos (per the SVG spec) — so
			// this loop never starts a further subpath.
			for moreArgs() {
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
				cur = append(cur, Point{X: x, Y: y})
			}
		case "L", "l":
			for {
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
				if !moreArgs() {
					break
				}
			}
		case "H", "h":
			for {
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
				if !moreArgs() {
					break
				}
			}
		case "V", "v":
			for {
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
				if !moreArgs() {
					break
				}
			}
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
			for {
				for range 5 { // rx, ry, x-axis-rotation, large-arc-flag, sweep-flag
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
				if !moreArgs() {
					break
				}
			}
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
	case "M", "m", "L", "l", "H", "h", "V", "v", "Z", "z", "A", "a",
		"C", "c", "S", "s", "Q", "q", "T", "t":
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

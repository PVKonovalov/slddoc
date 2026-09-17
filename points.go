package slddoc

import (
	"fmt"
	"strconv"
	"strings"
)

// parsePointList parses an SVG polyline/polygon points attribute
// ("x1,y1 x2,y2 ...").
func parsePointList(points string) ([]Point, error) {
	fields := strings.Fields(points)
	out := make([]Point, 0, len(fields))
	for _, f := range fields {
		xy := strings.SplitN(f, ",", 2)
		if len(xy) != 2 {
			return nil, fmt.Errorf("slddoc: malformed point %q in points=%q", f, points)
		}
		x, err := strconv.ParseFloat(xy[0], 64)
		if err != nil {
			return nil, fmt.Errorf("slddoc: point %q: %w", f, err)
		}
		y, err := strconv.ParseFloat(xy[1], 64)
		if err != nil {
			return nil, fmt.Errorf("slddoc: point %q: %w", f, err)
		}
		out = append(out, Point{X: x, Y: y})
	}
	if len(out) < 2 {
		return nil, fmt.Errorf("slddoc: points=%q has fewer than 2 points", points)
	}
	return out, nil
}

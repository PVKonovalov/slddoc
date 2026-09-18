package slddoc

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// This file parses the xsde2svg shape codes v1 understands into
// Element/Connector values. Each parser is derived from the corresponding
// element_<code>.go rendering function in the xsde2svg source (not by
// guessing from rendered output), so the port geometry below is exact, not
// heuristic:
//
//   - Every two-terminal device this package supports (Breaker: 41/43;
//     LoadBreakSwitch: 42; Disconnector: 162/71/49; ChokeCoil: 33;
//     CurrentTransformer: 34; SurgeArrester: 35/29; Fuse: 203/154 (154 is
//     the withdrawable variant — its own Position/data-trolley isn't
//     extracted, same as the withdrawable Breaker/Disconnector: it's a
//     purely render-side, this-editor-only concern); Capacitor: 388;
//     Reactor: 37; Starter: 76; NonIntersection: 14, a purely decorative
//     "hop" mark drawn where two crossing wires visually pass without
//     connecting, modeled with two ports anyway — one on each side — since
//     each one is still a real electrical node a wire's own end can land
//     on, same as this pattern's every other shape) is handled by one
//     function, parseTwoPortDevice: across every one of
//     these shapes, the two electrical ports always turn out to be the pair
//     of points reached furthest apart along the device's dominant axis,
//     among *every* point of *every* path in the element — even though the
//     specific subpath convention that produces them differs shape to
//     shape (sometimes a leg's starting M point is the port with its drawn
//     stub pointing inward, sometimes it's the point the stub's delta lands
//     on, sometimes the ports are literally a device's own outermost
//     drawing). Any decorative geometry (a state indicator, a body outline,
//     an arrow) is by construction always drawn *within* the span the
//     leads/leg reach, so it never disturbs the extremes. See
//     elements_test.go for the specific shapes this was verified against.
//   - Some of these shapes bake their orientation directly into the path's
//     own numbers when unrotated (breaker, disconnector, ...); others always
//     draw a fixed local shape and place it with a
//     transform="rotate(angle,cx,cy)" (choke coil, current transformer,
//     surge arrester, capacitor, ...). A rotate() can land on the element's
//     own top-level tag or on an inner wrapping <g> around just its
//     <path>s — xsde2svg emits either depending on shape/version, e.g. a
//     breaker/disconnector placed at a non-default angle wraps its
//     (still vertically-drawn) paths in a plain unrotated <g> normally, but
//     an inner <g transform="rotate(...)"> when turned — so the search
//     considers the element and its descendants, not just the element
//     itself. parseTwoPortDevice handles both cases: when a rotate() is
//     found, the element's own anchor is that transform's center and the
//     extracted extremes are rotated through it to get global port
//     coordinates (rotate() operates on raw, non-shifted coordinates, so
//     this needs no separate "make local" step); otherwise the anchor is
//     the ports' midpoint and the extremes are already global.
//   - GroundSwitch (54) and Ground (31) are single-electrical-port devices
//     at the element's own rotation anchor (Ground falls back to its path's
//     own first point when undrawn without a transform, since real
//     instances appear both ways).
//   - ReactorShunt (397), SurgeArrester (168, the grounded variant — as
//     opposed to 35/29, both two-port), CapacitorBank (172), and Generator
//     (173) are all single-electrical-port devices too (the grounded/
//     earthed/other side has no port of its own, same as
//     Ground/GroundSwitch): the port is always the combined path's own
//     first point, in the path's own *local* coordinates — but, unlike
//     Ground, a rotate() transform's own center is *not* necessarily that
//     same point for these four (e.g. Generator's own transform pivots on
//     its circle's center, 25 units from its terminal), so a found
//     transform has to be applied to that local point to get the real,
//     global anchor, the same way parseTwoPortDevice's own ports are
//     rotated, rather than just reusing the transform's center directly.
//     parseOnePortDevice is their common parser, parameterized by
//     Class/Shape since it's otherwise identical for all four shapes.
//   - PowerTransformer (47): only the 2-winding case is supported. Each
//     winding's lead is a direct <path> child with exactly one subpath of
//     exactly two points ("M x y h/v ±len"); the port is that subpath's
//     final point, rotated through the element's transform.
//   - BusBarSection (24) and generic wires (21/22/23/28) are plain
//     polylines; their points are kept as drawn.
//   - JunctionPoint (7) is a circle marking an explicit graph node.

var rotateRe = regexp.MustCompile(`rotate\(\s*(-?[\d.]+)\s*,\s*(-?[\d.]+)\s*,\s*(-?[\d.]+)\s*\)`)

// parseRotate extracts the (angle, cx, cy) of a transform="rotate(a,cx,cy)"
// attribute. ok is false if transform doesn't contain a rotate(...).
func parseRotate(transform string) (angle int, center Point, ok bool) {
	m := rotateRe.FindStringSubmatch(transform)
	if m == nil {
		return 0, Point{}, false
	}
	a, _ := strconv.ParseFloat(m[1], 64)
	cx, _ := strconv.ParseFloat(m[2], 64)
	cy, _ := strconv.ParseFloat(m[3], 64)
	return int(a), Point{X: cx, Y: cy}, true
}

// elementPaths returns every <path> that is either n itself (for a bare
// <path data-type="..."> element, which has no children to descend into) or
// a descendant of n.
func elementPaths(n *rawNode) []*rawNode {
	paths := n.descendants("path")
	if n.Tag == "path" {
		paths = append([]*rawNode{n}, paths...)
	}
	return paths
}

// allPathPoints collects every point of every subpath of every path in
// paths, in document order.
func allPathPoints(paths []*rawNode) ([]Point, error) {
	var pts []Point
	for _, p := range paths {
		subpaths, err := parseSubpaths(p.attr("d"))
		if err != nil {
			return nil, err
		}
		for _, sp := range subpaths {
			pts = append(pts, sp...)
		}
	}
	return pts, nil
}

// extremesAlongDominantAxis returns the two points of pts furthest apart
// along whichever of X or Y varies more across the set.
func extremesAlongDominantAxis(pts []Point) (Point, Point, error) {
	if len(pts) < 2 {
		return Point{}, Point{}, fmt.Errorf("slddoc: need at least 2 points to find extremes, got %d", len(pts))
	}
	minX, maxX, minY, maxY := pts[0], pts[0], pts[0], pts[0]
	for _, p := range pts[1:] {
		if p.X < minX.X {
			minX = p
		}
		if p.X > maxX.X {
			maxX = p
		}
		if p.Y < minY.Y {
			minY = p
		}
		if p.Y > maxY.Y {
			maxY = p
		}
	}
	if (maxX.X - minX.X) >= (maxY.Y - minY.Y) {
		return minX, maxX, nil
	}
	return minY, maxY, nil
}

func midpoint(a, b Point) Point {
	return Point{X: (a.X + b.X) / 2, Y: (a.Y + b.Y) / 2}
}

// orientFromPorts returns 90 when the two ports differ mainly along X
// (a horizontally-drawn symbol) or 0 when they differ mainly along Y
// (vertically-drawn, the native orientation xsde2svg's element formulas
// assume before any rotation).
func orientFromPorts(a, b Point) int {
	if math.Abs(a.X-b.X) > math.Abs(a.Y-b.Y) {
		return 90
	}
	return 0
}

// parseState looks for a data-state attribute on n itself (the case for a
// bare-element shape like Lamp's <circle>) or, failing that, on any of n's
// <path> children (the case for a <g>-wrapped shape whose state lives on its
// state-indicator path, e.g. a breaker/disconnector).
func parseState(n *rawNode) *int {
	if v, ok := n.Attrs["data-state"]; ok {
		if s, err := strconv.Atoi(v); err == nil {
			return &s
		}
	}
	for _, p := range elementPaths(n) {
		if v, ok := p.Attrs["data-state"]; ok {
			s, err := strconv.Atoi(v)
			if err == nil {
				return &s
			}
		}
	}
	return nil
}

// parseDataFill reads a data-fill="0:off,1:on" attribute (as written by
// element_106.go for a Lamp, and by the switch-like devices' state
// indicator, though the latter isn't read back here — see stateFill in
// render.go) and returns the colors for index 0 and 1.
func parseDataFill(dataFill string) (off, on string) {
	for entry := range strings.SplitSeq(dataFill, ",") {
		k, v, found := strings.Cut(entry, ":")
		if !found {
			continue
		}
		switch strings.TrimSpace(k) {
		case "0":
			off = strings.TrimSpace(v)
		case "1":
			on = strings.TrimSpace(v)
		}
	}
	return off, on
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// parseElementID converts a real SVG element's own id="..." attribute (an
// xsde2svg-assigned integer, always numeric in real corpus instances) into
// this package's own int Element/Connector/Node/Layer/VoltageClass id type.
func parseElementID(n *rawNode) (int, error) {
	s := n.attr("id")
	id, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("slddoc: invalid id %q: %w", s, err)
	}
	return id, nil
}

// parseTwoPortDevice handles every two-terminal shape documented at the top
// of this file: 41, 43, 42, 162, 71, 49, 33, 34, 35, 203, 388. voltage is
// the element's own raw data-voltage color, returned alongside rather than
// stored on Element (whose own Voltage field holds a resolved VoltageClass
// id, not yet known at parse time) — see Extract's own two-pass resolution.
func parseTwoPortDevice(n *rawNode, class Class, shape string) (Element, []Point, string, error) {
	id, err := parseElementID(n)
	if err != nil {
		return Element{}, nil, "", err
	}
	paths := elementPaths(n)
	if len(paths) == 0 {
		return Element{}, nil, "", fmt.Errorf("slddoc: element %s: no <path> geometry", n.attr("id"))
	}
	pts, err := allPathPoints(paths)
	if err != nil {
		return Element{}, nil, "", err
	}
	p1, p2, err := extremesAlongDominantAxis(pts)
	if err != nil {
		return Element{}, nil, "", fmt.Errorf("slddoc: element %s: %w", n.attr("id"), err)
	}

	var anchor Point
	var orient int
	ports := []Point{p1, p2}
	if angle, center, ok := parseRotate(n.firstAttrDescendant("transform")); ok {
		anchor, orient = center, angle
		ports[0] = rotate(p1, center, float64(angle))
		ports[1] = rotate(p2, center, float64(angle))
	} else {
		anchor = midpoint(p1, p2)
		orient = orientFromPorts(p1, p2)
	}

	voltage := firstNonEmpty(n.attr("data-voltage"), n.firstAttrDescendant("data-voltage"))
	return Element{
		ID:     id,
		Class:  class,
		Shape:  shape,
		Name:   n.attr("data-name"),
		Layer:  resolveLayer(n.attr("data-layer")),
		X:      anchor.X,
		Y:      anchor.Y,
		Orient: orient,
		State:  parseState(n),
		Ports: []Port{
			{Name: "1"},
			{Name: "2"},
		},
	}, ports, voltage, nil
}

// parseGround handles shape 31 (ground/earth terminal): a single electrical port.
// Real instances appear both with and without a rotate() transform; when
// absent, the port is the drawn path's own first point (the element's
// formula always starts drawing exactly at its origin/anchor).
func parseGround(n *rawNode) (Element, []Point, string, error) {
	id, err := parseElementID(n)
	if err != nil {
		return Element{}, nil, "", err
	}
	paths := elementPaths(n)
	if len(paths) == 0 {
		return Element{}, nil, "", fmt.Errorf("slddoc: ground %s: no <path> geometry", n.attr("id"))
	}

	var anchor Point
	var orient int
	if angle, center, ok := parseRotate(n.attr("transform")); ok {
		anchor, orient = center, angle
	} else {
		subpaths, err := parseSubpaths(paths[0].attr("d"))
		if err != nil {
			return Element{}, nil, "", err
		}
		if len(subpaths) == 0 || len(subpaths[0]) == 0 {
			return Element{}, nil, "", fmt.Errorf("slddoc: ground %s: empty path", n.attr("id"))
		}
		anchor = subpaths[0][0]
	}

	return Element{
		ID:     id,
		Class:  ClassGround,
		Shape:  "31",
		Name:   n.attr("data-name"),
		Layer:  resolveLayer(n.attr("data-layer")),
		X:      anchor.X,
		Y:      anchor.Y,
		Orient: orient,
		Ports:  []Port{{Name: "1"}},
	}, []Point{anchor}, n.attr("data-voltage"), nil
}

// parseGroundSwitch handles shape 54 (ground switch): a single electrical
// port at the element's own rotation anchor.
func parseGroundSwitch(n *rawNode) (Element, []Point, string, error) {
	id, err := parseElementID(n)
	if err != nil {
		return Element{}, nil, "", err
	}
	angle, center, ok := parseRotate(n.attr("transform"))
	if !ok {
		return Element{}, nil, "", fmt.Errorf("slddoc: ground switch %s: no rotate() transform (unrotated ground switches are not yet supported)", n.attr("id"))
	}
	return Element{
		ID:     id,
		Class:  ClassGroundSwitch,
		Shape:  "54",
		Name:   n.attr("data-name"),
		Layer:  resolveLayer(n.attr("data-layer")),
		X:      center.X,
		Y:      center.Y,
		Orient: angle,
		State:  parseState(n),
		Ports:  []Port{{Name: "1"}},
	}, []Point{center}, n.attr("data-voltage"), nil
}

// parseOnePortDevice handles a single-electrical-port device whose real
// connection point is its combined path's own first point, *in the path's
// own local/pre-rotation coordinates* — ReactorShunt (397), SurgeArrester
// (168, the grounded variant), CapacitorBank (172), Generator (173). Real
// instances appear both with and without a rotate() transform; when
// absent, the anchor is exactly that first point (the element's formula
// always starts drawing there). When present, the first point is *not*
// simply the rotate() transform's own center the way Ground (31)'s
// equivalent case is — for these four shapes the transform's center is a
// different, shape-specific reference point (e.g. Generator's own circle
// center, 25 units below its terminal), confirmed against a real corpus
// instance (vres.svg, id 148791225: `M 3630 575 ... transform="rotate(
// -270,3630,600)"` — center (3630,600) is 25 units from the path's own
// first point (3630,575), not equal to it) — so the first point has to be
// rotated *through* that transform to get the true, post-rotation anchor,
// the same way parseTwoPortDevice's own ports are, rather than just
// reusing the transform's center directly as parseGround does.
func parseOnePortDevice(n *rawNode, class Class, shape string) (Element, []Point, string, error) {
	id, err := parseElementID(n)
	if err != nil {
		return Element{}, nil, "", err
	}
	paths := elementPaths(n)
	if len(paths) == 0 {
		return Element{}, nil, "", fmt.Errorf("slddoc: element %s: no <path> geometry", n.attr("id"))
	}
	subpaths, err := parseSubpaths(paths[0].attr("d"))
	if err != nil {
		return Element{}, nil, "", err
	}
	if len(subpaths) == 0 || len(subpaths[0]) == 0 {
		return Element{}, nil, "", fmt.Errorf("slddoc: element %s: empty path", n.attr("id"))
	}
	rawAnchor := subpaths[0][0]

	anchor := rawAnchor
	var orient int
	if angle, center, ok := parseRotate(n.firstAttrDescendant("transform")); ok {
		anchor = rotate(rawAnchor, center, float64(angle))
		orient = angle
	}

	voltage := firstNonEmpty(n.attr("data-voltage"), n.firstAttrDescendant("data-voltage"))
	return Element{
		ID:     id,
		Class:  class,
		Shape:  shape,
		Name:   n.attr("data-name"),
		Layer:  resolveLayer(n.attr("data-layer")),
		X:      anchor.X,
		Y:      anchor.Y,
		Orient: orient,
		Ports:  []Port{{Name: "1"}},
	}, []Point{anchor}, voltage, nil
}

// parseVoltageTransformer handles shape 55 (voltage transformer) — the same
// anchor-finding logic as parseOnePortDevice, but a real instance never
// carries a data-voltage attribute anywhere on its own <g> or descendants
// (confirmed against the full sld-svg/examples/sld corpus: all of its own
// shape-55 instances lack one entirely) — its primary winding's own voltage
// color is only ever embedded directly in that same first <path>'s own
// style="stroke:...", the one whose first point is already this function's
// own anchor.
func parseVoltageTransformer(n *rawNode) (Element, []Point, string, error) {
	id, err := parseElementID(n)
	if err != nil {
		return Element{}, nil, "", err
	}
	paths := elementPaths(n)
	if len(paths) == 0 {
		return Element{}, nil, "", fmt.Errorf("slddoc: element %s: no <path> geometry", n.attr("id"))
	}
	subpaths, err := parseSubpaths(paths[0].attr("d"))
	if err != nil {
		return Element{}, nil, "", err
	}
	if len(subpaths) == 0 || len(subpaths[0]) == 0 {
		return Element{}, nil, "", fmt.Errorf("slddoc: element %s: empty path", n.attr("id"))
	}
	rawAnchor := subpaths[0][0]

	anchor := rawAnchor
	var orient int
	if angle, center, ok := parseRotate(n.firstAttrDescendant("transform")); ok {
		anchor = rotate(rawAnchor, center, float64(angle))
		orient = angle
	}

	voltage := styleProp(paths[0].attr("style"), "stroke")
	return Element{
		ID:     id,
		Class:  ClassVoltageTransformer,
		Shape:  "55",
		Name:   n.attr("data-name"),
		Layer:  resolveLayer(n.attr("data-layer")),
		X:      anchor.X,
		Y:      anchor.Y,
		Orient: orient,
		Ports:  []Port{{Name: "1"}},
	}, []Point{anchor}, voltage, nil
}

// parsePowerTransformer handles shape 47 (power transformer), 2-winding
// case only: each winding's lead is a direct <path> child with exactly one
// subpath of exactly two points, whose final point is the port.
func parsePowerTransformer(n *rawNode) (Element, []Point, error) {
	id, err := parseElementID(n)
	if err != nil {
		return Element{}, nil, err
	}
	angle, center, ok := parseRotate(n.attr("transform"))
	if !ok {
		return Element{}, nil, fmt.Errorf("slddoc: transformer %s: no rotate() transform", n.attr("id"))
	}

	var localPorts []Point
	for _, p := range n.childrenTagged("path") {
		subpaths, err := parseSubpaths(p.attr("d"))
		if err != nil {
			continue
		}
		if len(subpaths) == 1 && len(subpaths[0]) == 2 {
			localPorts = append(localPorts, subpaths[0][1])
		}
	}
	if len(localPorts) != 2 {
		return Element{}, nil, fmt.Errorf("slddoc: transformer %s: found %d lead(s), only the 2-winding case is supported in v1", n.attr("id"), len(localPorts))
	}

	globalPorts := make([]Point, 2)
	for i, lp := range localPorts {
		globalPorts[i] = rotate(lp, center, float64(angle))
	}

	return Element{
		ID:     id,
		Class:  ClassPowerTransformer,
		Shape:  "47",
		Name:   n.attr("data-name"),
		Layer:  resolveLayer(n.attr("data-layer")),
		X:      center.X,
		Y:      center.Y,
		Orient: angle,
		Ports: []Port{
			{Name: "1"},
			{Name: "2"},
		},
	}, globalPorts, nil
}

// parseBusBar handles shape 24 (busbar): a plain styled polyline that other
// geometry can tap anywhere along its length, not just at its endpoints.
func parseBusBar(n *rawNode) (Element, string, error) {
	id, err := parseElementID(n)
	if err != nil {
		return Element{}, "", err
	}
	pts, err := parsePointList(n.attr("points"))
	if err != nil {
		return Element{}, "", err
	}
	return Element{
		ID:     id,
		Class:  ClassBusBarSection,
		Shape:  "24",
		Name:   n.attr("data-name"),
		Layer:  resolveLayer(n.attr("data-layer")),
		X:      pts[0].X,
		Y:      pts[0].Y,
		Points: pts,
	}, n.attr("data-voltage"), nil
}

// parseJunctionPoint handles shape 7 (junction point): a small circle marking an
// explicit graph junction. Its own data-voltage attribute records the
// circle's fill (usually the page background), not its electrical color, so
// the voltage is read from the stroke style instead.
func parseJunctionPoint(n *rawNode) (Element, string, error) {
	id, err := parseElementID(n)
	if err != nil {
		return Element{}, "", err
	}
	cx, err := strconv.ParseFloat(n.attr("cx"), 64)
	if err != nil {
		return Element{}, "", fmt.Errorf("slddoc: point %s: %w", n.attr("id"), err)
	}
	cy, err := strconv.ParseFloat(n.attr("cy"), 64)
	if err != nil {
		return Element{}, "", fmt.Errorf("slddoc: point %s: %w", n.attr("id"), err)
	}
	return Element{
		ID:    id,
		Class: ClassJunctionPoint,
		Shape: "7",
		Layer: resolveLayer(n.attr("data-layer")),
		X:     cx,
		Y:     cy,
		Ports: []Port{{Name: "1"}},
	}, styleProp(n.attr("style"), "stroke"), nil
}

// parseLamp handles shape 106 (лампа/lamp): a standalone status-indicator
// circle, not a real electrical device — it carries no ports, since real
// corpus instances are never the endpoint of a drawn wire. Its data-voltage
// (per element_106.go) is always absent in real instances (its data-fill
// colors are read instead, since they carry the lamp's actual on/off
// display colors, e.g. "0:magenta,1:#12161d" — arbitrary per instance, not
// the fixed red/lawngreen/yellow convention the switch-like devices' own
// state indicator uses). Its radius (r) is likewise recorded rather than
// assumed constant: real instances draw meaningfully different sizes for
// different roles, e.g. r=11 standalone "Индикатор" panel lights vs. r=5
// lamps clustered in triplets next to a breaker (see Examples.svg).
func parseLamp(n *rawNode) (Element, error) {
	id, err := parseElementID(n)
	if err != nil {
		return Element{}, err
	}
	cx, err := strconv.ParseFloat(n.attr("cx"), 64)
	if err != nil {
		return Element{}, fmt.Errorf("slddoc: lamp %s: %w", n.attr("id"), err)
	}
	cy, err := strconv.ParseFloat(n.attr("cy"), 64)
	if err != nil {
		return Element{}, fmt.Errorf("slddoc: lamp %s: %w", n.attr("id"), err)
	}
	radius, err := strconv.ParseFloat(n.attr("r"), 64)
	if err != nil {
		return Element{}, fmt.Errorf("slddoc: lamp %s: %w", n.attr("id"), err)
	}
	off, on := parseDataFill(n.attr("data-fill"))
	return Element{
		ID:      id,
		Class:   ClassLamp,
		Shape:   "106",
		Name:    n.attr("data-name"),
		Layer:   resolveLayer(n.attr("data-layer")),
		X:       cx,
		Y:       cy,
		State:   parseState(n),
		FillOff: off,
		FillOn:  on,
		Radius:  radius,
	}, nil
}

// parseFaultPassageIndicator handles shape 320003 (ИКЗ/fault passage
// indicator): like Lamp, a standalone status-indicator circle, not a real
// electrical device — it carries no ports. Unlike Lamp it has no data-fill
// pair (every real corpus instance draws the same fixed dark fill/lime
// stroke regardless of state, per element320.go's "ИКЗ" case), so only its
// position and radius are extracted; Render supplies the fixed color. The
// circle is a child of the top-level <g>, not the bare element itself,
// since real instances also carry state-dependent decorative children
// (an "FPI" text label or arrow-icon graphics) alongside it.
func parseFaultPassageIndicator(n *rawNode) (Element, error) {
	id, err := parseElementID(n)
	if err != nil {
		return Element{}, err
	}
	circles := n.childrenTagged("circle")
	if len(circles) == 0 {
		return Element{}, fmt.Errorf("slddoc: fault passage indicator %s: no circle", n.attr("id"))
	}
	c := circles[0]
	cx, err := strconv.ParseFloat(c.attr("cx"), 64)
	if err != nil {
		return Element{}, fmt.Errorf("slddoc: fault passage indicator %s: %w", n.attr("id"), err)
	}
	cy, err := strconv.ParseFloat(c.attr("cy"), 64)
	if err != nil {
		return Element{}, fmt.Errorf("slddoc: fault passage indicator %s: %w", n.attr("id"), err)
	}
	radius, err := strconv.ParseFloat(c.attr("r"), 64)
	if err != nil {
		return Element{}, fmt.Errorf("slddoc: fault passage indicator %s: %w", n.attr("id"), err)
	}
	return Element{
		ID:     id,
		Class:  ClassFaultPassageIndicator,
		Shape:  "320003",
		Name:   n.attr("data-name"),
		Layer:  resolveLayer(n.attr("data-layer")),
		X:      cx,
		Y:      cy,
		Radius: radius,
	}, nil
}

var connectorKindByType = map[string]ConnectorKind{
	"21": KindBusbarWire,
	"22": KindOverheadLine,
	"23": KindCableLine,
	"28": KindLinkToObject,
}

// parseConnector handles the generic wire shapes (21, 22, 23, 28): a plain
// polyline whose two ends are the electrical connection points.
func parseConnector(n *rawNode) (Connector, string, error) {
	id, err := parseElementID(n)
	if err != nil {
		return Connector{}, "", err
	}
	pts, err := parsePointList(n.attr("points"))
	if err != nil {
		return Connector{}, "", err
	}
	kind := connectorKindByType[n.attr("data-type")]
	return Connector{
		ID:     id,
		Kind:   kind,
		Layer:  resolveLayer(n.attr("data-layer")),
		Dashed: styleProp(n.attr("style"), "stroke-dasharray") != "",
		Points: pts,
	}, n.attr("data-voltage"), nil
}

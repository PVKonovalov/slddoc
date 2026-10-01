package slddoc

import (
	"fmt"
	"math"
	"regexp"
	"sort"
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
//     Reactor: 37; Starter: 76; PowerCircuitBreaker: 399 (via
//     parsePowerCircuitBreaker, which also recovers Mirror; its own closed
//     blade path ends in a bare, argument-less "m", which parseSubpaths
//     tolerates); NonIntersection: 14, a purely decorative
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
//   - PowerTransformer (47): the 2/3/4-winding non-autotransformer case is
//     supported (an autotransformer's own tap decoration isn't recognized
//     as such — it's just read back as an ordinary extra path on whichever
//     winding it's drawn on). Each winding's lead is a direct <path> child
//     with exactly one subpath of exactly two points ("M x y h/v ±len");
//     the port is that subpath's final point, rotated through the
//     element's transform. A found lead's own connection-scheme glyph,
//     grounding mark, and regulation arrow aren't reverse-engineered back
//     into Scheme/Grounding/TapChanger — importing a real xsde2svg
//     transformer gets its winding count and lead positions back, not its
//     full nameplate configuration.
//   - Booster (6) is a two-port device whose own first <path> is the
//     circle plus both leads ("M a y h v a ... m ... h -v"): its two ports
//     are that path's two subpaths' own first points, rotated through the
//     path's own rotate() when present. The arrow and the winding mark
//     are never rotated by the real source, so only the first path is
//     read for geometry; a filled (closed, "z") second path is the
//     regulation arrow, read back as TapChanger.
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

// parsePowerCircuitBreaker handles shape 399 (Автомат силовой): its
// ports/anchor/state come from parseTwoPortDevice like any other
// two-terminal device, plus Mirror, which (unlike every other shape) is
// recovered from the drawing itself. element_399.go draws the small filled
// square beside the blade on one of two sides depending on its own xMirror
// flag; base.xml's own template uses the xMirror==1 geometry as its default
// (the more common one in real corpora), so an instance drawn with the
// xMirror==0 geometry is extracted as Mirror=true. Both paths are compared
// in their own unrotated coordinates (a rotate() only ever wraps them), so
// this needs no knowledge of the element's own orientation.
func parsePowerCircuitBreaker(n *rawNode) (Element, []Point, string, error) {
	el, ports, voltage, err := parseTwoPortDevice(n, ClassPowerCircuitBreaker, "399")
	if err != nil {
		return el, ports, voltage, err
	}
	paths := elementPaths(n)
	if len(paths) < 2 {
		return el, ports, voltage, nil
	}
	blade, err1 := parseSubpaths(paths[0].attr("d"))
	square, err2 := parseSubpaths(paths[1].attr("d"))
	if err1 != nil || err2 != nil || len(blade) == 0 || len(square) == 0 ||
		len(blade[0]) < 2 || len(square[0]) < 2 {
		return el, ports, voltage, nil
	}
	b0, b1 := blade[0][0], blade[0][1]
	q0, q1 := square[0][0], square[0][1]
	if b0.X == b1.X {
		// Closed (vertical blade): the xMirror==1 square starts drawing
		// rightward from the blade, the xMirror==0 one leftward.
		el.Mirror = q1.X < q0.X
	} else {
		// Open (horizontal blade): the xMirror==1 square sits right of the
		// blade's own center, the xMirror==0 one left of it.
		el.Mirror = q0.X < (b0.X+b1.X)/2
	}
	return el, ports, voltage, nil
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

// horizontalTwoPortShapes are the two-port shapes whose xsde2svg formula
// draws them horizontally before any rotation (Resistor, Thyristor), unlike the rest,
// so an unrotated instance's own Orient is 0, not orientFromPorts' 90.
var horizontalTwoPortShapes = map[string]bool{"156": true, "157": true}

// parseTwoPortDevice handles every two-terminal shape documented at the top
// of this file: 41, 43, 42, 162, 71, 49, 33, 34, 35, 203, 388, 156. voltage is
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
		anchor, orient = center, normalizeOrient(angle)
		ports[0] = rotate(p1, center, float64(angle))
		ports[1] = rotate(p2, center, float64(angle))
	} else {
		anchor = midpoint(p1, p2)
		orient = orientFromPorts(p1, p2)
		if horizontalTwoPortShapes[shape] {
			orient = 90 - orient
		}
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

// parseKnifeSwitch3 handles shape 175 (Рубильник 3-позиционный). The real
// source draws three r=2 circles in one path — the pivot, then the right
// and the left contact — whose relative moves are fixed numbers, not
// scaled, so each circle's center is its subpath's start plus (2, 0). The
// anchor is the rotate() center, or when unrotated, the pivot's x and the
// contacts' y + 10 (the source's origin). The source has no state and
// always draws the blade in the middle, so State is left unset (middle).
func parseKnifeSwitch3(n *rawNode) (Element, []Point, string, error) {
	id, err := parseElementID(n)
	if err != nil {
		return Element{}, nil, "", err
	}
	var circles *rawNode
	for _, p := range elementPaths(n) {
		if strings.ContainsAny(p.attr("d"), "aA") {
			circles = p
			break
		}
	}
	if circles == nil {
		return Element{}, nil, "", fmt.Errorf("slddoc: knife switch %s: no circles path", n.attr("id"))
	}
	subs, err := parseSubpaths(circles.attr("d"))
	if err != nil {
		return Element{}, nil, "", err
	}
	if len(subs) != 3 || len(subs[0]) == 0 || len(subs[1]) == 0 || len(subs[2]) == 0 {
		return Element{}, nil, "", fmt.Errorf("slddoc: knife switch %s: expected three circles", n.attr("id"))
	}
	center := func(sp []Point) Point { return Point{X: sp[0].X + 2, Y: sp[0].Y} }
	pivot, right, left := center(subs[0]), center(subs[1]), center(subs[2])

	anchor := Point{X: pivot.X, Y: right.Y + 10}
	angle, c, rotated := parseRotate(n.attr("transform"))
	if rotated {
		anchor = c
	}
	ports := []Point{
		rotate(pivot, anchor, float64(angle)),
		rotate(left, anchor, float64(angle)),
		rotate(right, anchor, float64(angle)),
	}
	return Element{
		ID:     id,
		Class:  ClassKnifeSwitch3,
		Shape:  "175",
		Name:   n.attr("data-name"),
		Layer:  resolveLayer(n.attr("data-layer")),
		X:      anchor.X,
		Y:      anchor.Y,
		Orient: normalizeOrient(angle),
		Ports: []Port{
			{Name: "1"},
			{Name: "2"},
			{Name: "3"},
		},
	}, ports, n.attr("data-voltage"), nil
}

// parseDisconnectorFuse handles shape 166 (Разъединитель-предохранитель).
// Its real markup nests an optional rotate() <g> inside the element's own
// <g>; the blade path carries data-state, data-voltage and data-name, the
// contact-bar path is drawn with h only (both bars when Closed, the top one
// plus the pivot circle when Open), and the legs path with v only. The
// anchor is the rotate() center, or when unrotated, the top bar's midpoint
// x and the y halfway between the top bar and the bottom bar (Closed) or
// the blade's pivot (Open, the blade path's first point) — both sit the
// same distance from the anchor at any export scale. The ports are the
// legs' outer ends when present, else the top bar and its mirror image
// below the anchor. Mirror is read from which way the Open blade swings.
func parseDisconnectorFuse(n *rawNode) (Element, []Point, string, error) {
	id, err := parseElementID(n)
	if err != nil {
		return Element{}, nil, "", err
	}
	var blade, bars, legs *rawNode
	for _, p := range elementPaths(n) {
		d := p.attr("d")
		switch {
		case p.attr("data-state") != "":
			blade = p
		case strings.ContainsAny(d, "hH"):
			bars = p
		case strings.ContainsAny(d, "vV"):
			legs = p
		}
	}
	if blade == nil || bars == nil {
		return Element{}, nil, "", fmt.Errorf("slddoc: disconnector-fuse %s: no blade or contact-bar path", n.attr("id"))
	}
	bladeSubs, err := parseSubpaths(blade.attr("d"))
	if err != nil {
		return Element{}, nil, "", err
	}
	barSubs, err := parseSubpaths(bars.attr("d"))
	if err != nil {
		return Element{}, nil, "", err
	}
	if len(bladeSubs) == 0 || len(bladeSubs[0]) < 2 || len(barSubs) == 0 || len(barSubs[0]) < 2 {
		return Element{}, nil, "", fmt.Errorf("slddoc: disconnector-fuse %s: unexpected geometry", n.attr("id"))
	}
	state := parseState(n)
	open := state != nil && *state == 0

	top := barSubs[0][0]
	bottomY := bladeSubs[0][0].Y
	if !open && len(barSubs) > 1 {
		bottomY = barSubs[1][0].Y
	}
	anchor := Point{X: (barSubs[0][0].X + barSubs[0][1].X) / 2, Y: (top.Y + bottomY) / 2}
	angle, center, rotated := parseRotate(n.firstAttrDescendant("transform"))
	if rotated {
		anchor = center
	}

	half := math.Abs(top.Y - anchor.Y)
	if legs != nil {
		if legSubs, err := parseSubpaths(legs.attr("d")); err == nil && len(legSubs) > 0 && len(legSubs[0]) > 1 {
			half = math.Abs(legSubs[0][len(legSubs[0])-1].Y - anchor.Y)
		}
	}
	mirror := open && bladeSubs[0][1].X > bladeSubs[0][0].X

	ports := []Point{
		rotate(Point{X: anchor.X, Y: anchor.Y - half}, anchor, float64(angle)),
		rotate(Point{X: anchor.X, Y: anchor.Y + half}, anchor, float64(angle)),
	}
	return Element{
		ID:     id,
		Class:  ClassDisconnectorFuse,
		Shape:  "166",
		Name:   firstNonEmpty(n.attr("data-name"), n.firstAttrDescendant("data-name")),
		Layer:  resolveLayer(n.attr("data-layer")),
		X:      anchor.X,
		Y:      anchor.Y,
		Orient: normalizeOrient(angle),
		Mirror: mirror,
		State:  state,
		Ports: []Port{
			{Name: "1"},
			{Name: "2"},
		},
	}, ports, firstNonEmpty(n.attr("data-voltage"), n.firstAttrDescendant("data-voltage")), nil
}

// parseShortCircuiterNoGround handles shape 163 (Короткозамыкатель без
// земли). Unlike Sectionalizer (164) its real markup is a single <g> whose
// rod/arm path carries a plain data-state, beside a filled arrowhead path
// (the only one closed with z), a pivot circle when Open, and, when the
// source's bus spacing (sde.Distance) exceeds the body, a separate path of
// two vertical legs. The anchor is the rotate() center, or when unrotated,
// the first contact bar's midpoint x and the arrowhead tip's y (the arm's
// own line). The ports are the legs' outer ends when present (the real
// connection points), else the contact bars' ends along the rod axis.
// Mirror is read from the arm's direction (xMirror draws it rightward).
func parseShortCircuiterNoGround(n *rawNode) (Element, []Point, string, error) {
	id, err := parseElementID(n)
	if err != nil {
		return Element{}, nil, "", err
	}
	var body, arrow, legs *rawNode
	for _, p := range elementPaths(n) {
		d := p.attr("d")
		switch {
		case p.attr("data-state") != "":
			body = p
		case strings.ContainsAny(d, "zZ"):
			arrow = p
		case !strings.ContainsAny(d, "aA"):
			legs = p
		}
	}
	if body == nil || arrow == nil {
		return Element{}, nil, "", fmt.Errorf("slddoc: short-circuiter %s: no rod or arrowhead path", n.attr("id"))
	}
	bodySubs, err := parseSubpaths(body.attr("d"))
	if err != nil {
		return Element{}, nil, "", err
	}
	arrowSubs, err := parseSubpaths(arrow.attr("d"))
	if err != nil {
		return Element{}, nil, "", err
	}
	if len(bodySubs) == 0 || len(bodySubs[0]) < 2 || len(arrowSubs) == 0 || len(arrowSubs[0]) == 0 {
		return Element{}, nil, "", fmt.Errorf("slddoc: short-circuiter %s: unexpected geometry", n.attr("id"))
	}

	angle, anchor, rotated := parseRotate(n.attr("transform"))
	if !rotated {
		anchor = Point{X: (bodySubs[0][0].X + bodySubs[0][1].X) / 2, Y: arrowSubs[0][0].Y}
	}

	half := math.Abs(bodySubs[0][0].Y - anchor.Y)
	if legs != nil {
		if legSubs, err := parseSubpaths(legs.attr("d")); err == nil && len(legSubs) > 0 && len(legSubs[0]) > 0 {
			half = math.Abs(legSubs[0][0].Y - anchor.Y)
		}
	}

	mirror := false
	for _, sp := range bodySubs {
		if len(sp) == 2 && math.Abs(sp[0].Y-anchor.Y) < 0.5 && math.Abs(sp[1].Y-anchor.Y) < 0.5 {
			mirror = sp[1].X > sp[0].X
			break
		}
	}

	ports := []Point{
		rotate(Point{X: anchor.X, Y: anchor.Y - half}, anchor, float64(angle)),
		rotate(Point{X: anchor.X, Y: anchor.Y + half}, anchor, float64(angle)),
	}
	return Element{
		ID:     id,
		Class:  ClassShortCircuiterNoGround,
		Shape:  "163",
		Name:   n.attr("data-name"),
		Layer:  resolveLayer(n.attr("data-layer")),
		X:      anchor.X,
		Y:      anchor.Y,
		Orient: normalizeOrient(angle),
		Mirror: mirror,
		State:  parseState(n),
		Ports: []Port{
			{Name: "1"},
			{Name: "2"},
		},
	}, ports, n.attr("data-voltage"), nil
}

// parseThyristor handles shape 157 (Тиристор): anode and cathode are found
// like any horizontal two-port device (parseTwoPortDevice), and the gate,
// port "3", is the drawn path's own last point, the gate stub's free end.
func parseThyristor(n *rawNode) (Element, []Point, string, error) {
	el, ports, voltage, err := parseTwoPortDevice(n, ClassThyristor, "157")
	if err != nil {
		return Element{}, nil, "", err
	}
	pts, err := allPathPoints(elementPaths(n))
	if err != nil {
		return Element{}, nil, "", err
	}
	gate := pts[len(pts)-1]
	if angle, center, ok := parseRotate(n.firstAttrDescendant("transform")); ok {
		gate = rotate(gate, center, float64(angle))
	}
	el.Ports = append(el.Ports, Port{Name: "3"})
	return el, append(ports, gate), voltage, nil
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
// port at the element's own rotation anchor. Real instances appear both
// with and without a rotate() transform (element_54.go only emits one when
// its own computed angle is non-zero) — when absent, the anchor is
// recovered from the drawn path's own geometry instead, the same
// "real instance never guaranteed a transform" gap PackageSubstation/
// EnclosedSubstation's own substationAnchorFromGeometry fixed for those two
// shapes (see that function's own doc comment). Unlike a simple "first
// point" fallback (Ground/31's own, where the formula's first M point *is*
// the anchor), element_54.go's own first path always starts drawing at a
// fixed *offset* from the anchor — "M x y+yx v -tail ..." where x is the
// anchor's own X (never offset) but y+yx is the anchor's own Y plus a
// scale-dependent vertical offset — so recovering Y needs that offset
// undone. yx and tail are both Scale(scaleChosed, N) of fixed source
// constants (12 and 9 respectively, in the common/plain draw branch every
// real corpus instance found actually uses — see this function's own doc
// comment on ShortDraw/CustomView not being modeled) that scale together,
// so their ratio (tail/yx = 9/12 = 0.75) holds regardless of any given
// diagram's own scale factor — confirmed against all 2856 real corpus
// instances found carrying a rotate() (whose true anchor is directly
// knowable from that transform's own center, letting yx be recovered
// independently and checked): every single one matches this exact ratio,
// so no real instance found uses ShortDraw/CustomView's own different
// ratios (0.583/1.286) instead. yx is therefore recovered as
// tail*(12/9) from the path's own first segment (whatever its own real
// scale), without needing to know or guess that diagram's scale factor.
func parseGroundSwitch(n *rawNode) (Element, []Point, string, error) {
	id, err := parseElementID(n)
	if err != nil {
		return Element{}, nil, "", err
	}

	var anchor Point
	var orient int
	if angle, center, ok := parseRotate(n.attr("transform")); ok {
		anchor, orient = center, angle
	} else {
		paths := elementPaths(n)
		if len(paths) == 0 {
			return Element{}, nil, "", fmt.Errorf("slddoc: ground switch %s: no <path> geometry", n.attr("id"))
		}
		subpaths, err := parseSubpaths(paths[0].attr("d"))
		if err != nil {
			return Element{}, nil, "", err
		}
		if len(subpaths) == 0 || len(subpaths[0]) < 2 {
			return Element{}, nil, "", fmt.Errorf("slddoc: ground switch %s: first path has fewer than 2 points", n.attr("id"))
		}
		p0, p1 := subpaths[0][0], subpaths[0][1]
		tail := p0.Y - p1.Y
		anchor = Point{X: p0.X, Y: p0.Y - tail*12/9}
	}

	return Element{
		ID:     id,
		Class:  ClassGroundSwitch,
		Shape:  "54",
		Name:   n.attr("data-name"),
		Layer:  resolveLayer(n.attr("data-layer")),
		X:      anchor.X,
		Y:      anchor.Y,
		Orient: orient,
		State:  parseState(n),
		Ports:  []Port{{Name: "1"}},
	}, []Point{anchor}, n.attr("data-voltage"), nil
}

// parseShortCircuiter handles shape 398 (short-circuiter): a single
// electrical port at the element's own rotation anchor, the same structure
// parseGroundSwitch uses above — unrotated instances are not yet supported
// for the same reason.
func parseShortCircuiter(n *rawNode) (Element, []Point, string, error) {
	id, err := parseElementID(n)
	if err != nil {
		return Element{}, nil, "", err
	}

	// Unlike most switching devices, this shape's own State isn't a plain
	// data-state attribute on a path — the real source wraps its two
	// alternate geometries in their own sibling
	// <g data-state="0|1" visibility="visible|hidden"> groups, the same
	// convention parseSectionalizer already handles (see that function's
	// own doc comment); only the visible group's own data-state is real.
	var state *int
	found := false
	for _, g := range n.childrenTagged("g") {
		if g.attr("visibility") != "visible" {
			continue
		}
		found = true
		if v, err := strconv.Atoi(g.attr("data-state")); err == nil {
			state = &v
		}
		break
	}
	if !found {
		return Element{}, nil, "", fmt.Errorf("slddoc: short-circuiter %s: no visible data-state group", n.attr("id"))
	}

	angle, center, ok := parseRotate(n.attr("transform"))
	if !ok {
		return Element{}, nil, "", fmt.Errorf("slddoc: short-circuiter %s: no rotate() transform (unrotated short-circuiters are not yet supported)", n.attr("id"))
	}
	return Element{
		ID:     id,
		Class:  ClassShortCircuiter,
		Shape:  "398",
		Name:   n.attr("data-name"),
		Layer:  resolveLayer(n.attr("data-layer")),
		X:      center.X,
		Y:      center.Y,
		Orient: angle,
		State:  state,
		Ports:  []Port{{Name: "1"}},
	}, []Point{center}, n.attr("data-voltage"), nil
}

// substationAnchorFromGeometry recovers PackageSubstation's (385) or
// EnclosedSubstation's (386) own real anchor straight from its drawn
// geometry rather than from any rotate() transform — real xsde2svg's own
// rotate(angle,x,y) never changes the geometry's own stored coordinates,
// only how it's displayed, so this works regardless of whether a rotate()
// is present at all (real xsde2svg only emits one when angle != 0 — a
// real corpus export confirmed unrotated instances of both shapes exist)
// or how deep it's nested (a newer real xsde2svg version nests it one
// level inside the outer <g> Extract actually matches on — see
// parsePackageSubstation's own doc comment). For the box/square variant (a
// <g> whose first child is the outer 36-unit <rect> — true for 385's own
// NType 0 and for the whole of 386, which has no other variant), the
// anchor is that rect's own center; for 385's own NType 1 triangle (a
// <path>, no <rect> at all), the anchor is the path's own literal starting
// point — the real source's own path formula begins exactly "M x y" at
// the unmodified anchor, before any relative moves.
func substationAnchorFromGeometry(n *rawNode) (Point, error) {
	if rects := n.descendants("rect"); len(rects) > 0 {
		r := rects[0]
		x, errX := strconv.ParseFloat(r.attr("x"), 64)
		y, errY := strconv.ParseFloat(r.attr("y"), 64)
		w, errW := strconv.ParseFloat(r.attr("width"), 64)
		h, errH := strconv.ParseFloat(r.attr("height"), 64)
		if errX != nil || errY != nil || errW != nil || errH != nil {
			return Point{}, fmt.Errorf("invalid outer rect geometry")
		}
		return Point{X: x + w/2, Y: y + h/2}, nil
	}
	paths := elementPaths(n)
	if len(paths) == 0 {
		return Point{}, fmt.Errorf("no <rect> or <path> geometry")
	}
	subpaths, err := parseSubpaths(paths[0].attr("d"))
	if err != nil {
		return Point{}, err
	}
	if len(subpaths) == 0 || len(subpaths[0]) == 0 {
		return Point{}, fmt.Errorf("empty path")
	}
	return subpaths[0][0], nil
}

// substationDataProperty reads one key out of PackageSubstation's/
// EnclosedSubstation's own data-property export attribute — a generic
// "key:value;key:value" format (the same lightweight one style.go's own
// styleProp already parses a style="..." attribute with, reused here
// verbatim since the grammar is identical), added to xsde2svg's own
// element_385.go/element_id386.go at the user's own request specifically
// to recover characteristics real corpus rendering leaves no other trace
// of: 385's own NType (key "ntype") and both shapes' own Tech.Closed
// dashed-outline state (key "closed", 0 dashed/1 solid — matches
// Element.State's own convention for these two shapes directly, no
// translation needed). Superseded 385's own earlier, single-purpose
// data-ntype attribute (still read as a fallback by parsePackageSubstation
// specifically — see its own doc comment). "" when the key, or the whole
// attribute, is absent.
func substationDataProperty(n *rawNode, key string) string {
	return styleProp(n.attr("data-property"), key)
}

// substationState parses PackageSubstation's/EnclosedSubstation's own
// data-property "closed" key (see substationDataProperty) into a State
// pointer — nil when absent (an older real export predating data-property
// entirely, the same known-gap state this had before), matching
// applyStateLine's own "nil defaults to the first/solid option" convention
// every other State-driven shape already relies on.
func substationState(n *rawNode) *int {
	closedStr := substationDataProperty(n, "closed")
	if closedStr == "" {
		return nil
	}
	closed, err := strconv.Atoi(closedStr)
	if err != nil {
		return nil
	}
	return &closed
}

// parsePackageSubstation handles shape 385 (Package transformer
// substation): its own anchor always comes from
// substationAnchorFromGeometry (rotate-invariant — see that function's own
// doc comment), and Orient from a rotate() transform found anywhere among
// n's descendants (n.firstAttrDescendant("transform"), the same pattern
// parseTwoPortDevice already uses for shapes whose rotate() can land on an
// inner wrapping <g> instead of the element's own top-level tag) rather
// than just n's own attribute — a real, currently-deployed xsde2svg version
// nests it one level inside the outer <g id data-type ...> Extract actually
// matches on, so checking only n's own attribute silently lost Orient for
// every real rotated instance found this way. NType comes from the real
// source's own data-property "ntype" key (see substationDataProperty),
// falling back to the older single-purpose data-ntype attribute when
// data-property itself is absent — a real, already-deployed xsde2svg
// v1.4.12 corpus export was independently found to carry that older
// attribute before data-property replaced it, so it's still read rather
// than silently dropped. PropertyText is the first descendant <text>'s
// own content — see Element.PropertyText's own doc comment; "" when the
// real instance carries none, the common case. Fill (the real source's
// own Abonent flag) is read back from the inner rect's/triangle path's
// own fill (see substationFill) — a real corpus instance directly
// reported as missing this (element id 148788827, its own Abonent-filled
// inner rectangle silently dropped) confirmed the gap. State (the real
// source's own Tech.Closed dashing) comes from data-property's own
// "closed" key (see substationState) — nil for an older export predating
// it, same as before.
func parsePackageSubstation(n *rawNode) (Element, []Point, string, error) {
	id, err := parseElementID(n)
	if err != nil {
		return Element{}, nil, "", err
	}
	anchor, err := substationAnchorFromGeometry(n)
	if err != nil {
		return Element{}, nil, "", fmt.Errorf("slddoc: package substation %s: %w", n.attr("id"), err)
	}
	angle, _, _ := parseRotate(n.firstAttrDescendant("transform"))
	ntypeStr := substationDataProperty(n, "ntype")
	if ntypeStr == "" {
		ntypeStr = n.attr("data-ntype")
	}
	ntype, _ := strconv.Atoi(ntypeStr)
	return Element{
		ID:           id,
		Class:        ClassPackageSubstation,
		Shape:        "385",
		Name:         n.attr("data-name"),
		Layer:        resolveLayer(n.attr("data-layer")),
		X:            anchor.X,
		Y:            anchor.Y,
		Orient:       angle,
		NType:        ntype,
		State:        substationState(n),
		Fill:         substationFill(n),
		PropertyText: substationPropertyText(n),
		Ports:        []Port{{Name: "1"}},
	}, []Point{anchor}, n.attr("data-voltage"), nil
}

// substationFill recovers PackageSubstation's/EnclosedSubstation's own
// Abonent-driven Fill from whichever of its own drawn shapes is the one
// that actually varies with it: the inner (second) <rect> for 385's own
// NType 0 box variant, or the single triangle <path> otherwise (385's own
// NType 1, and the whole of 386 — see writePackageSubstation/
// writeEnclosedSubstation) — "" (unset) when that shape's own fill is
// literally "none" (Abonent 0, the common case), matching how those two
// render functions already treat an unset Fill.
func substationFill(n *rawNode) string {
	var filled *rawNode
	if rects := n.descendants("rect"); len(rects) >= 2 {
		filled = rects[1]
	} else if paths := elementPaths(n); len(paths) > 0 {
		filled = paths[0]
	}
	if filled == nil {
		return ""
	}
	fill := styleProp(filled.attr("style"), "fill")
	if fill == "none" {
		return ""
	}
	return fill
}

// parseEnclosedSubstation handles shape 386 (Enclosed transformer
// substation, ZTP): same anchor/Orient/State/Fill/PropertyText recovery as
// parsePackageSubstation (see its own doc comment) — this shape just has
// no NType to recover, and its own data-property never carries an
// "ntype" key at all (no older single-purpose attribute to fall back to
// either, since 386 never had one).
func parseEnclosedSubstation(n *rawNode) (Element, []Point, string, error) {
	id, err := parseElementID(n)
	if err != nil {
		return Element{}, nil, "", err
	}
	anchor, err := substationAnchorFromGeometry(n)
	if err != nil {
		return Element{}, nil, "", fmt.Errorf("slddoc: enclosed substation %s: %w", n.attr("id"), err)
	}
	angle, _, _ := parseRotate(n.firstAttrDescendant("transform"))
	return Element{
		ID:           id,
		Class:        ClassEnclosedSubstation,
		Shape:        "386",
		Name:         n.attr("data-name"),
		Layer:        resolveLayer(n.attr("data-layer")),
		X:            anchor.X,
		Y:            anchor.Y,
		Orient:       angle,
		State:        substationState(n),
		Fill:         substationFill(n),
		PropertyText: substationPropertyText(n),
		Ports:        []Port{{Name: "1"}},
	}, []Point{anchor}, n.attr("data-voltage"), nil
}

// substationPropertyText reads PackageSubstation's/EnclosedSubstation's own
// optional overlay label back from the first <text> descendant, matching
// how writeSubstationPropertyText (render.go) draws it — see
// Element.PropertyText's own doc comment. "" when the real instance
// carries none, the common case.
func substationPropertyText(n *rawNode) string {
	texts := n.descendants("text")
	if len(texts) == 0 {
		return ""
	}
	return strings.TrimSpace(texts[0].Text)
}

// sectionalizerAnchorFromTick derives a Sectionalizer's own local anchor
// point from its "top tick" — a short, exactly-10-unit horizontal segment
// that, unlike anything else this shape draws, appears identically in both
// its Closed and Open geometry (see element_164.go's own path1/path2) and
// is always the topmost point in whichever one is active, at local
// (0,-10) relative to the anchor. Used only when the real source omits
// its own rotate() transform (Orient 0 — element_164.go only emits one
// when angle != 0), since a plain dominant-axis extremes search (what
// parseTwoPortDevice's own callers use) would instead find this shape's
// own open-contact circle, which reaches further from center (local y up
// to 13) than either real terminal (local y ±10).
func sectionalizerAnchorFromTick(stateGroup *rawNode) (Point, error) {
	var tick []Point
	for _, p := range elementPaths(stateGroup) {
		subpaths, err := parseSubpaths(p.attr("d"))
		if err != nil {
			return Point{}, err
		}
		for _, sp := range subpaths {
			if len(sp) != 2 || sp[0].Y != sp[1].Y || math.Abs(sp[1].X-sp[0].X) != 10 {
				continue
			}
			if tick == nil || sp[0].Y < tick[0].Y {
				tick = sp
			}
		}
	}
	if tick == nil {
		return Point{}, fmt.Errorf("no top tick found")
	}
	return Point{X: (tick[0].X + tick[1].X) / 2, Y: tick[0].Y + 10}, nil
}

// parseSectionalizer handles shape 164 (Отделитель/Sectionalizer): a
// two-terminal device whose real Closed(1)/Open(0) state, unlike every
// other switching device here, isn't a data-state attribute on a <path>
// (what parseState looks for) but a pair of visibility-swapped child
// groups, <g data-state="1" visibility="visible|hidden"> and <g
// data-state="0" visibility="hidden|visible"> (see element_164.go) — both
// always present, only one ever actually visible — so State is read
// directly from whichever one carries visibility="visible" instead. Real
// instances only carry their own rotate() transform when Orient != 0 (used
// directly, same as every other two-port shape); at Orient 0 the source
// omits it entirely (a real corpus's own Sectionalizer instances are
// overwhelmingly this case), so the anchor instead falls back to
// sectionalizerAnchorFromTick.
func parseSectionalizer(n *rawNode) (Element, []Point, string, error) {
	id, err := parseElementID(n)
	if err != nil {
		return Element{}, nil, "", err
	}

	var activeGroup *rawNode
	var state *int
	for _, g := range n.childrenTagged("g") {
		if g.attr("visibility") != "visible" {
			continue
		}
		activeGroup = g
		if v, err := strconv.Atoi(g.attr("data-state")); err == nil {
			state = &v
		}
		break
	}
	if activeGroup == nil {
		return Element{}, nil, "", fmt.Errorf("slddoc: sectionalizer %s: no visible data-state group", n.attr("id"))
	}

	var anchor Point
	var angle int
	if a, center, ok := parseRotate(n.attr("transform")); ok {
		angle, anchor = a, center
	} else {
		anchor, err = sectionalizerAnchorFromTick(activeGroup)
		if err != nil {
			return Element{}, nil, "", fmt.Errorf("slddoc: sectionalizer %s: %w", n.attr("id"), err)
		}
	}

	ports := []Point{
		rotate(Point{X: anchor.X, Y: anchor.Y - 10}, anchor, float64(angle)),
		rotate(Point{X: anchor.X, Y: anchor.Y + 10}, anchor, float64(angle)),
	}

	return Element{
		ID:     id,
		Class:  ClassSectionalizer,
		Shape:  "164",
		Name:   n.attr("data-name"),
		Layer:  resolveLayer(n.attr("data-layer")),
		X:      anchor.X,
		Y:      anchor.Y,
		Orient: angle,
		State:  state,
		Ports: []Port{
			{Name: "1"},
			{Name: "2"},
		},
	}, ports, n.attr("data-voltage"), nil
}

// parseOnePortDevice handles a single-electrical-port device whose real
// connection point is its combined path's own first point, *in the path's
// own local/pre-rotation coordinates* — ReactorShunt (397), SurgeArrester
// (168, the grounded variant), CapacitorBank (172), Generator (173),
// SynchronousCompensator (174). Real
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

// parseBooster handles shape 6 (Booster/voltage regulator, see the list
// at the top of this file). The anchor is the midpoint of the two drawn
// lead ends, which is also the circle's center and the real source's own
// rotate() center, so it works for unrotated instances too and at any
// export scale.
func parseBooster(n *rawNode) (Element, []Point, string, error) {
	id, err := parseElementID(n)
	if err != nil {
		return Element{}, nil, "", err
	}
	paths := elementPaths(n)
	if len(paths) == 0 {
		return Element{}, nil, "", fmt.Errorf("slddoc: booster %s: no <path> geometry", n.attr("id"))
	}
	subpaths, err := parseSubpaths(paths[0].attr("d"))
	if err != nil {
		return Element{}, nil, "", err
	}
	if len(subpaths) != 2 || len(subpaths[0]) == 0 || len(subpaths[1]) == 0 {
		return Element{}, nil, "", fmt.Errorf("slddoc: booster %s: expected a circle with two leads", n.attr("id"))
	}
	p1, p2 := subpaths[0][0], subpaths[1][0]
	anchor := midpoint(p1, p2)
	orient := 0
	if angle, center, ok := parseRotate(paths[0].attr("transform")); ok {
		p1 = rotate(p1, center, float64(angle))
		p2 = rotate(p2, center, float64(angle))
		anchor = center
		orient = normalizeOrient(angle)
	}
	tapChanger := false
	for _, p := range paths[1:] {
		if strings.ContainsAny(p.attr("d"), "zZ") {
			tapChanger = true
		}
	}
	voltage := firstNonEmpty(n.attr("data-voltage"), n.firstAttrDescendant("data-voltage"))
	return Element{
		ID:         id,
		Class:      ClassBooster,
		Shape:      "6",
		Name:       n.attr("data-name"),
		Layer:      resolveLayer(n.attr("data-layer")),
		X:          anchor.X,
		Y:          anchor.Y,
		Orient:     orient,
		TapChanger: tapChanger,
		Ports: []Port{
			{Name: "1"},
			{Name: "2"},
		},
	}, []Point{p1, p2}, voltage, nil
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

// parsePowerTransformer handles shape 47 (power transformer): each real
// winding is a <circle> child with its own lead, a <path> child with
// exactly one subpath of exactly two points ("M x y h/v ±len") — the port
// being that subpath's final point — and, optionally, its own
// connection-scheme glyph, a <path> child immediately after the lead if
// its own shape matches one of the three this package recognizes (see
// windingSchemeFromGlyph).
//
// A winding's own lead is *not* simply "the next <path> after its own
// <circle>" — real xsde2svg's own autotransformer geometry
// (element_47.go's own case-3 branch, the last real winding of a
// WindingNo==4 autotransformer) draws that one winding's own leg *before*
// its own circle, unlike every other winding of every other shape, which
// a strict document-order "circle, then its own trailing children"
// grouping would misparse — under-counting that winding's own lead
// entirely and misreading the next winding's own lead as a stray
// connection-scheme glyph instead. So instead, every lead-shaped
// candidate <path> in the whole element is collected first, then matched
// to whichever <circle> its own start point is closest to (a lead always
// starts right at its own circle's edge, one radius away at most) —
// correct regardless of document order, not just for this one known
// quirk. A decoration that isn't a winding's own lead or glyph at all
// (this package's own writePowerTransformer draws its regulation arrow
// only after every winding; real xsde2svg draws an autotransformer's own
// tap arc+stub before its first <circle> — see below) is never mistaken
// for one: a candidate too far from every circle to plausibly be its own
// lead is simply not assigned, and glyph detection only ever inspects the
// single <path> immediately following a winding's own now-correctly-
// identified lead (skipping it entirely if that next path was itself
// claimed as some other winding's own lead), which is also what keeps a
// regulation arrow's own closed-triangle arrowhead (the same "one
// subpath, four points, closed" shape a delta glyph has) from being
// misread as some winding's own delta scheme.
//
// Known gaps, not yet recovered from rendered geometry at all: which
// winding (if any) has TapChanger set (the regulation arrow's own
// presence/color don't reliably tie back to one specific winding), a
// winding's own NeutralGrounding (real xsde2svg's "neutral_ground"
// WindingType — a distinct, more complex glyph than plain wye-with-neutral
// — isn't pattern-matched here, so a grounded winding is read back as
// plain SchemeWyeN with Grounding left unset), and any of the rarer real
// WindingType values (zigzag, open_delta, ...) this package doesn't offer
// in Properties anyway. Recovering these reliably is exactly what adding
// dedicated data-* attributes to xsde2svg's own SVG output would fix,
// rather than pattern-matching geometry that was never meant to be parsed
// back.
func parsePowerTransformer(n *rawNode) (Element, []Point, []string, error) {
	id, err := parseElementID(n)
	if err != nil {
		return Element{}, nil, nil, err
	}
	// A real xsde2svg instance at angle 0 omits transform="rotate(...)"
	// entirely (same convention every other shape's own extraction
	// already handles — see parseTwoPortDevice/parseOnePortDevice's own
	// fallback for a missing rotate()) — unlike those, a transformer's own
	// untransformed anchor isn't simply its first path's own raw point or
	// two ports' own midpoint, so it's recovered below, once the winding
	// count is known, by reversing transformerWindingOffset against the
	// first circle's own absolute center.
	angle, center, hasRotate := parseRotate(n.attr("transform"))

	type circleInfo struct {
		childIdx int
		color    string
		center   Point
		radius   float64
	}
	var circles []circleInfo
	autotransformer := false
	sawCircle := false
	firstCircleChildIdx := -1
	for i, c := range n.Children {
		if c.Tag == "circle" {
			sawCircle = true
			if firstCircleChildIdx == -1 {
				firstCircleChildIdx = i
			}
			cx, _ := strconv.ParseFloat(c.attr("cx"), 64)
			cy, _ := strconv.ParseFloat(c.attr("cy"), 64)
			r, _ := strconv.ParseFloat(c.attr("r"), 64)
			if r == 0 {
				r = transformerRadius // this package's own default, for a malformed/missing r
			}
			circles = append(circles, circleInfo{childIdx: i, color: c.attr("data-voltage"), center: Point{X: cx, Y: cy}, radius: r})
			continue
		}
		if c.Tag == "path" && !sawCircle {
			// A real xsde2svg autotransformer draws its own tap arc+stub
			// for the (uncircled) first winding entry before any <circle>
			// at all — the one reliable, order-based signal this format
			// gives for "this is an autotransformer" without a dedicated
			// attribute.
			autotransformer = true
		}
	}
	if len(circles) < 2 || len(circles) > 4 {
		return Element{}, nil, nil, fmt.Errorf("slddoc: transformer %s: found %d winding(s), only 2/3/4-winding transformers are supported", n.attr("id"), len(circles))
	}

	type leadInfo struct {
		childIdx int
		end      Point
	}
	leadFor := make(map[int]leadInfo) // circle index -> its own lead
	claimedPath := map[int]bool{}     // child index -> claimed as some circle's own lead
	for i, c := range n.Children {
		if c.Tag != "path" || i < firstCircleChildIdx {
			// A candidate before the transformer's very first <circle> is
			// always an autotransformer's own tap arc+stub (see above),
			// never any winding's own lead — excluding it here matters
			// even though it's usually far from every circle, since a
			// tap's own arc endpoint can land close enough to a nearby
			// circle to win the distance race against that circle's own
			// real lead otherwise (an autotransformer's tap arc curves
			// back toward its own first real winding by construction).
			continue
		}
		subpaths, err := parseSubpaths(c.attr("d"))
		if err != nil || len(subpaths) != 1 || len(subpaths[0]) != 2 {
			continue
		}
		start, end := subpaths[0][0], subpaths[0][1]
		best, bestDist := -1, math.Inf(1)
		for ci, circ := range circles {
			if _, taken := leadFor[ci]; taken {
				continue
			}
			if d := distance(start, circ.center); d < bestDist {
				best, bestDist = ci, d
			}
		}
		// A real lead always starts within one radius of its own circle's
		// own real edge — generously double that circle's own real radius
		// (not this package's own default transformerRadius, since a real
		// instance's own Size preset can scale it well past the default;
		// one real corpus instance uses r="62") to allow room for its own
		// leg length too, while still safely excluding a regulation
		// arrow's own diagonal line (always centered on the element's own
		// anchor, tens of units further out).
		if best == -1 || bestDist > 2*circles[best].radius {
			continue
		}
		leadFor[best] = leadInfo{childIdx: i, end: end}
		claimedPath[i] = true
	}
	if len(leadFor) != len(circles) {
		return Element{}, nil, nil, fmt.Errorf("slddoc: transformer %s: found %d winding(s) but only %d own lead(s)", n.attr("id"), len(circles), len(leadFor))
	}

	localPorts := make([]Point, len(circles))
	colors := make([]string, len(circles))
	windings := make([]TransformerWinding, len(circles))
	for ci, circ := range circles {
		lead := leadFor[ci]
		localPorts[ci] = lead.end
		colors[ci] = circ.color
		for j := lead.childIdx + 1; j < len(n.Children); j++ {
			next := n.Children[j]
			if next.Tag == "circle" || claimedPath[j] {
				break
			}
			if next.Tag != "path" {
				continue
			}
			if subpaths, err := parseSubpaths(next.attr("d")); err == nil {
				windings[ci].Scheme = windingSchemeFromGlyph(subpaths)
			}
			break
		}
	}
	if !hasRotate {
		off0X, off0Y := transformerWindingOffset(len(circles), 0)
		center = Point{X: circles[0].center.X - off0X, Y: circles[0].center.Y - off0Y}
	}
	// A transformer's own anchor and each winding's own lead tip are
	// snapped to this editor's own 10-unit default grid (EditorSettings'
	// own GridSpacing default — see config.yaml) — real xsde2svg source
	// diagrams place an element's own anchor on a grid this fine already
	// almost universally, but a lead tip's own position is derived from
	// that anchor by fixed, non-grid-multiple offsets (transformerRadius
	// 22, the leg-length/shift constants, ...), so it essentially never
	// lands on the grid on its own even when the anchor does. The
	// resulting up-to-5-unit shift is well within snapTolerance's own
	// existing connectivity tolerance (topology.go — its own doc comment
	// already specifically cites "observed on a PowerTransformer's leads"
	// as the reason that tolerance exists at all), so this doesn't risk
	// a wire failing to bind to its own newly-snapped port.
	center = snapPointToGrid(center)
	globalPorts := make([]Point, len(localPorts))
	for i, lp := range localPorts {
		globalPorts[i] = snapPointToGrid(rotate(lp, center, float64(angle)))
	}

	ports := make([]Port, len(localPorts))
	for i := range ports {
		ports[i] = Port{Name: fmt.Sprintf("%d", i+1)}
	}

	return Element{
		ID:              id,
		Class:           ClassPowerTransformer,
		Shape:           "47",
		Name:            n.attr("data-name"),
		Layer:           resolveLayer(n.attr("data-layer")),
		X:               center.X,
		Y:               center.Y,
		Orient:          angle,
		Ports:           ports,
		Autotransformer: autotransformer,
		Windings:        windings,
	}, globalPorts, colors, nil
}

// extractGridSpacing is the fixed grid snapPointToGrid rounds a
// PowerTransformer's own extracted anchor/lead positions to — this
// package's own default (EditorSettings.GridSpacing's own default; see
// config.yaml), not something Extract's own signature threads a
// caller-chosen value through, since it exists only to compensate for
// this one shape's own non-grid-multiple internal offsets, not as a
// general "snap everything on import" feature.
const extractGridSpacing = 10.0

func snapPointToGrid(p Point) Point {
	return Point{
		X: math.Round(p.X/extractGridSpacing) * extractGridSpacing,
		Y: math.Round(p.Y/extractGridSpacing) * extractGridSpacing,
	}
}

// windingSchemeFromGlyph identifies a winding's own connection-scheme
// glyph from its already-parsed subpaths, matching writeWindingGlyph's own
// three shapes exactly (structurally — by subpath/point count, not exact
// coordinates, since a real instance's own shift constant can vary with
// its Size preset): plain wye is three 2-point subpaths (three spokes from
// one shared center, each its own M); wye-with-neutral (Yn) is the same
// but four; delta is a single 4-point subpath closed back on itself (the
// triangle's own "z"). Anything else — including this package's own
// autotransformer tap stub/arc, or a real xsde2svg regulation arrow's own
// closed-triangle arrowhead, which a naive point-count check alone could
// mistake for a delta glyph if it weren't for the caller only ever
// checking the one path slot immediately after a winding's own lead —
// returns "".
func windingSchemeFromGlyph(subpaths [][]Point) WindingScheme {
	allTwoPoints := func() bool {
		for _, sp := range subpaths {
			if len(sp) != 2 {
				return false
			}
		}
		return true
	}
	switch {
	case len(subpaths) == 3 && allTwoPoints():
		return SchemeWye
	case len(subpaths) == 4 && allTwoPoints():
		return SchemeWyeN
	case len(subpaths) == 1 && len(subpaths[0]) == 4 && subpaths[0][0] == subpaths[0][3]:
		return SchemeDelta
	}
	return ""
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

// parseRectangle handles shape 3 (Rectangle): a purely decorative
// annotation box, not real electrical equipment (see ClassRectangle's own
// doc comment) — no Ports are ever created for one. Real instances are a
// bare <rect x y width height style>, matching a busbar's own bare
// <polyline> (no wrapping <g>). Its own two Points are its top-left and
// bottom-right corners as drawn, (x,y) and (x+width,y+height) — Render
// doesn't require a Rectangle's two Points in any particular order, so
// this is just the simplest pair to derive directly from a real <rect>'s
// own attributes, not a meaningful convention of its own.
func parseRectangle(n *rawNode) (Element, error) {
	id, err := parseElementID(n)
	if err != nil {
		return Element{}, err
	}
	return parseRect(n, id, ClassRectangle, "3")
}

// parseSmallWindow handles shape 319 (Окошко/Small window, see
// ClassSmallWindow): Rectangle's same bare <rect>. element_319.go wrote no
// id before it was patched, so a missing one is left 0 for Extract to fill
// in; the border width is fixed, so StrokeWidth stays unset.
func parseSmallWindow(n *rawNode) (Element, error) {
	el, err := parseRect(n, parseOptionalID(n), ClassSmallWindow, "319")
	el.StrokeWidth = 0
	return el, err
}

// parseRect is parseRectangle/parseSmallWindow's shared <rect> reader.
func parseRect(n *rawNode, id int, class Class, shape string) (Element, error) {
	x, errX := strconv.ParseFloat(n.attr("x"), 64)
	y, errY := strconv.ParseFloat(n.attr("y"), 64)
	w, errW := strconv.ParseFloat(n.attr("width"), 64)
	h, errH := strconv.ParseFloat(n.attr("height"), 64)
	if errX != nil || errY != nil || errW != nil || errH != nil {
		return Element{}, fmt.Errorf("slddoc: %s %s: invalid x/y/width/height", class, n.attr("id"))
	}
	style := n.attr("style")
	// StrokeWidth left at 0 (unset) when absent or unparseable — Render's
	// own fallback already treats that as 1, the real source's own
	// minimum (mathext.Max(uint(1), ...) in element_3.go), so there's no
	// need to hardcode that default a second time here.
	strokeWidth, _ := strconv.ParseFloat(styleProp(style, "stroke-width"), 64)
	return Element{
		ID:          id,
		Class:       class,
		Shape:       shape,
		Name:        n.attr("data-name"),
		Layer:       resolveLayer(n.attr("data-layer")),
		X:           x + w/2,
		Y:           y + h/2,
		Fill:        styleProp(style, "fill"),
		Stroke:      styleProp(style, "stroke"),
		StrokeWidth: strokeWidth,
		Points:      []Point{{X: x, Y: y}, {X: x + w, Y: y + h}},
	}, nil
}

// parseCircle handles shape 4 (Circle): a purely decorative annotation
// ellipse (see ClassCircle's own doc comment) — no Ports are ever created
// for one. Real instances are a bare <ellipse cx cy rx ry style>, no
// wrapping <g> (matching writeCircle's own convention). Its own two Points
// are the ellipse's own bounding box corners, (cx-rx,cy-ry) and
// (cx+rx,cy+ry) — the same "two opposite corners" convention
// parseRectangle already uses, so Circle reuses every one of Rectangle's
// own Points-based frontend/backend machinery unchanged.
func parseCircle(n *rawNode) (Element, error) {
	id, err := parseElementID(n)
	if err != nil {
		return Element{}, err
	}
	cx, errCX := strconv.ParseFloat(n.attr("cx"), 64)
	cy, errCY := strconv.ParseFloat(n.attr("cy"), 64)
	rx, errRX := strconv.ParseFloat(n.attr("rx"), 64)
	ry, errRY := strconv.ParseFloat(n.attr("ry"), 64)
	if errCX != nil || errCY != nil || errRX != nil || errRY != nil {
		return Element{}, fmt.Errorf("slddoc: circle %s: invalid cx/cy/rx/ry", n.attr("id"))
	}
	style := n.attr("style")
	strokeWidth, _ := strconv.ParseFloat(styleProp(style, "stroke-width"), 64)
	return Element{
		ID:          id,
		Class:       ClassCircle,
		Shape:       "4",
		Name:        n.attr("data-name"),
		Layer:       resolveLayer(n.attr("data-layer")),
		X:           cx,
		Y:           cy,
		Fill:        styleProp(style, "fill"),
		Stroke:      styleProp(style, "stroke"),
		StrokeWidth: strokeWidth,
		Points:      []Point{{X: cx - rx, Y: cy - ry}, {X: cx + rx, Y: cy + ry}},
	}, nil
}

// parseArrow handles shape 2 (Arrow): a purely decorative annotation line
// with an open chevron arrowhead (see ClassArrow's own doc comment) — no
// Ports are ever created for one. Real instances are a bare <path d
// style>, no wrapping <g> (matching writeArrow's own convention), whose
// own "d" mixes the line itself with its arrowhead's zigzag strokes as one
// path (see writeArrow/arrowChevron's own doc comments for that shape).
// Rather than replicate the real xsde2svg source's own five separate
// draw-formula branches (four axis-aligned special cases plus one
// generic/rotated one) to recover the true two endpoints, this exploits a
// simpler invariant true of all of them: the path's very first point (the
// initial M) is always the arrow's own true start, and its true end is
// always the point *farthest* from that start — every arrowhead wing
// point sits only a few units (l3/l7, see arrowChevron) away from the
// tip, far closer than any real arrow's own length in practice. A
// transform="rotate(...)" on the node, when present, is applied to both
// derived points to get their real diagram-space positions; when absent
// (the axis-aligned case, which never gets one), the path's own
// coordinates are already absolute. DoubleHeaded is deliberately never
// set true here — telling a real instance's own doubled starting chevron
// apart from an ordinary single-headed one from raw geometry alone isn't
// reliable enough to be worth attempting, so an extracted double-headed
// arrow round-trips as a plausible-looking single-headed one instead of
// risking a false positive on an ordinary one.
func parseArrow(n *rawNode) (Element, error) {
	id, err := parseElementID(n)
	if err != nil {
		return Element{}, err
	}
	subpaths, err := parseSubpaths(n.attr("d"))
	if err != nil {
		return Element{}, err
	}
	var pts []Point
	for _, sp := range subpaths {
		pts = append(pts, sp...)
	}
	if len(pts) < 2 {
		return Element{}, fmt.Errorf("slddoc: arrow %s: fewer than 2 points in path", n.attr("id"))
	}
	p0, p1 := pts[0], pts[0]
	bestDist := 0.0
	for _, p := range pts[1:] {
		dist := math.Hypot(p.X-p0.X, p.Y-p0.Y)
		if dist > bestDist {
			bestDist = dist
			p1 = p
		}
	}
	if angle, center, ok := parseRotate(n.attr("transform")); ok {
		p0 = rotate(p0, center, float64(angle))
		p1 = rotate(p1, center, float64(angle))
	}

	style := n.attr("style")
	strokeWidth, _ := strconv.ParseFloat(styleProp(style, "stroke-width"), 64)
	return Element{
		ID:    id,
		Class: ClassArrow,
		Shape: "2",
		Name:  n.attr("data-name"),
		Layer: resolveLayer(n.attr("data-layer")),
		// X/Y is the midpoint of p0/p1, not p0 itself — matching the
		// frontend's own diagramOps.placeArrow convention (and Rectangle/
		// BusBarSection's own), even though writeArrow's real rendering
		// reads Points[0]/[1] directly and never looks at X/Y at all for
		// an Arrow; kept consistent anyway so a freshly extracted arrow
		// and a freshly drawn one carry the same kind of anchor.
		X:           (p0.X + p1.X) / 2,
		Y:           (p0.Y + p1.Y) / 2,
		Stroke:      styleProp(style, "stroke"),
		StrokeWidth: strokeWidth,
		Points:      []Point{p0, p1},
	}, nil
}

// parseButton handles shape 113 (Объемная кнопка/3D button): a purely
// decorative annotation widget (see ClassButton's own doc comment) — no
// Ports are ever created for one. A real instance is a <g data-type="113">
// wrapping a <rect x y width height style> (its own box) and, when it
// carries a label, a <text style>...</text> sibling (see writeButton's own
// doc comment for the exact markup) — the box's own two Points are its
// top-left/bottom-right corners, the same convention parseRectangle already
// uses. Bold is recovered from the text's own style the same
// strings.Contains(style, "font-weight: bold") way parseDigitalDevice's own
// is. A Button with no <text> child at all (real corpus shows this never
// happens, but a hand-edited file could) simply extracts with an empty
// PropertyText, same as an ordinary unlabeled Rectangle.
func parseButton(n *rawNode) (Element, error) {
	return parseTextBox(n, ClassButton, "113")
}

// parseWindowIcon handles shape 302 (Иконка окна/Window icon), drawn as
// Button's same <g><rect/><text/></g> (see writeTextBox). Its border width
// is fixed at 1 by the source, so StrokeWidth stays unset. A real
// instance's text style carries an empty "fill:", read as no TextColor
// (the black default).
func parseWindowIcon(n *rawNode) (Element, error) {
	el, err := parseTextBox(n, ClassWindowIcon, "302")
	el.StrokeWidth = 0
	return el, err
}

// containerDashStyles is the reverse of render.go's containerDashPatterns.
var containerDashStyles = map[string]ConnectorLineStyle{
	"3,2": LineStyleDotted,
	"6,5": LineStyleDashed,
}

// parseContainer handles shape 310 (Контейнер/Container, see
// ClassContainer). The patched xsde2svg (and Render) write one <g
// data-type="310"> holding the outline <path> and an optional caption
// <text>. An older export wrote only the caption inside the group and the
// outline as the bare, untyped <path> right after it, passed here as next;
// an older uncaptioned container carried no data-type at all and isn't
// recognizable. The caption is stored as drawn: TextDx/TextDy from the
// outline's top-left, its anchor/baseline/size/fill, and Orient from its
// rotate(angle,x,y).
func parseContainer(n, next *rawNode) (Element, error) {
	id, err := parseElementID(n)
	if err != nil {
		return Element{}, err
	}
	var outline *rawNode
	if paths := n.childrenTagged("path"); len(paths) > 0 {
		outline = paths[0]
	} else if next != nil && next.Tag == "path" && next.attr("data-type") == "" {
		outline = next
	}
	if outline == nil {
		return Element{}, fmt.Errorf("slddoc: container %d: no outline path", id)
	}
	subpaths, err := parseSubpaths(outline.attr("d"))
	if err != nil {
		return Element{}, err
	}
	if len(subpaths) == 0 {
		return Element{}, fmt.Errorf("slddoc: container %d: empty outline", id)
	}
	pts := subpaths[0]
	if len(pts) > 1 && pts[len(pts)-1] == pts[0] {
		pts = pts[:len(pts)-1] // the closing z
	}
	if len(pts) < 3 {
		return Element{}, fmt.Errorf("slddoc: container %d: %d points, want at least 3", id, len(pts))
	}
	style := outline.attr("style")
	strokeWidth, _ := strconv.ParseFloat(strings.TrimSpace(styleProp(style, "stroke-width")), 64)
	el := Element{
		ID:          id,
		Class:       ClassContainer,
		Shape:       "310",
		Layer:       resolveLayer(n.attr("data-layer")),
		X:           pts[0].X,
		Y:           pts[0].Y,
		Fill:        styleProp(style, "fill"),
		Stroke:      styleProp(style, "stroke"),
		StrokeWidth: strokeWidth,
		LineStyle:   containerDashStyles[strings.TrimSpace(styleProp(style, "stroke-dasharray"))],
		Points:      pts,
	}
	if t := firstTextChild(n); t != nil {
		lbl := textToLabel(t)
		el.PropertyText = lbl.Text
		el.TextColor = lbl.Color
		el.TextSize = lbl.Size
		el.TextAnchor = lbl.Anchor
		el.TextBaseline = strings.TrimSpace(styleProp(t.attr("style"), "dominant-baseline"))
		el.Name = n.attr("data-name")
		minX, minY := pts[0].X, pts[0].Y
		for _, p := range pts[1:] {
			minX, minY = math.Min(minX, p.X), math.Min(minY, p.Y)
		}
		el.TextDx, el.TextDy = lbl.X-minX, lbl.Y-minY
		if angle, _, ok := parseRotate(t.attr("transform")); ok {
			el.Orient = angle
		}
	}
	return el, nil
}

// parseTextBox is parseButton/parseWindowIcon's shared reader.
func parseTextBox(n *rawNode, class Class, shape string) (Element, error) {
	id, err := parseElementID(n)
	if err != nil {
		return Element{}, err
	}
	rects := n.childrenTagged("rect")
	if len(rects) == 0 {
		rects = n.descendants("rect")
	}
	if len(rects) == 0 {
		return Element{}, fmt.Errorf("slddoc: %s %s: no rect child", class, n.attr("id"))
	}
	rn := rects[0]
	x, errX := strconv.ParseFloat(rn.attr("x"), 64)
	y, errY := strconv.ParseFloat(rn.attr("y"), 64)
	w, errW := strconv.ParseFloat(rn.attr("width"), 64)
	h, errH := strconv.ParseFloat(rn.attr("height"), 64)
	if errX != nil || errY != nil || errW != nil || errH != nil {
		return Element{}, fmt.Errorf("slddoc: %s %s: invalid x/y/width/height", class, n.attr("id"))
	}
	rectStyle := rn.attr("style")
	strokeWidth, _ := strconv.ParseFloat(styleProp(rectStyle, "stroke-width"), 64)

	el := Element{
		ID:          id,
		Class:       class,
		Shape:       shape,
		Name:        n.attr("data-name"),
		Layer:       resolveLayer(n.attr("data-layer")),
		X:           x + w/2,
		Y:           y + h/2,
		Fill:        styleProp(rectStyle, "fill"),
		Stroke:      styleProp(rectStyle, "stroke"),
		StrokeWidth: strokeWidth,
		Points:      []Point{{X: x, Y: y}, {X: x + w, Y: y + h}},
	}

	if t := firstTextChild(n); t != nil {
		textStyle := t.attr("style")
		el.PropertyText = t.Text
		el.TextColor = styleProp(textStyle, "fill")
		el.Bold = strings.Contains(textStyle, "font-weight: bold") || strings.Contains(textStyle, "font-weight:bold")
	}
	return el, nil
}

// tableDashStyles is the reverse of render.go's own tableDashPatterns.
var tableDashStyles = map[string]ConnectorLineStyle{
	"6,5":         LineStyleDashed,
	"70 20 25 20": LineStyleDashDot,
}

// parseTable handles shape 312 (Таблица/Table): a purely decorative
// annotation box (see ClassTable's own doc comment) — no Ports are ever
// created for one. A real instance is a <g id data-type="312"> (this
// project's own addition to the real source, see ClassTable's own doc
// comment for why) wrapping a bare <rect x y width height style> and,
// when it carries a label, a <text style transform?>...</text> sibling
// (see writeTable's own doc comment for the exact markup) — the box's own
// two Points are its top-left/bottom-right corners, the same convention
// parseButton/parseRectangle already use. LineStyle is recovered from the
// rect's own style stroke-dasharray (see tableDashStyles); Orient from
// the text's own rotate() transform, when it carries one — only present
// when non-zero, the same conditional parsePole already handles for a
// different shape's own transform.
func parseTable(n *rawNode) (Element, error) {
	id, err := parseElementID(n)
	if err != nil {
		return Element{}, err
	}
	rects := n.childrenTagged("rect")
	if len(rects) == 0 {
		rects = n.descendants("rect")
	}
	if len(rects) == 0 {
		return Element{}, fmt.Errorf("slddoc: table %s: no rect child", n.attr("id"))
	}
	rn := rects[0]
	x, errX := strconv.ParseFloat(rn.attr("x"), 64)
	y, errY := strconv.ParseFloat(rn.attr("y"), 64)
	w, errW := strconv.ParseFloat(rn.attr("width"), 64)
	h, errH := strconv.ParseFloat(rn.attr("height"), 64)
	if errX != nil || errY != nil || errW != nil || errH != nil {
		return Element{}, fmt.Errorf("slddoc: table %s: invalid x/y/width/height", n.attr("id"))
	}
	rectStyle := rn.attr("style")
	strokeWidth, _ := strconv.ParseFloat(styleProp(rectStyle, "stroke-width"), 64)

	el := Element{
		ID:          id,
		Class:       ClassTable,
		Shape:       "312",
		Name:        n.attr("data-name"),
		Layer:       resolveLayer(n.attr("data-layer")),
		Fill:        styleProp(rectStyle, "fill"),
		Stroke:      styleProp(rectStyle, "stroke"),
		StrokeWidth: strokeWidth,
		LineStyle:   tableDashStyles[styleProp(rectStyle, "stroke-dasharray")],
		Points:      []Point{{X: x, Y: y}, {X: x + w, Y: y + h}},
	}

	if t := firstTextChild(n); t != nil {
		el.PropertyText = t.Text
		el.TextColor = styleProp(t.attr("style"), "fill")
		if angle, _, ok := parseRotate(t.attr("transform")); ok {
			el.Orient = angle
		}
	}
	return el, nil
}

// table2DashStyles is the reverse of render.go's own table2DashPatterns.
var table2DashStyles = map[string]ConnectorLineStyle{
	"3,2": LineStyleDashed,
}

// table2CellPathRe matches a Table2 (313) cell's own <path> d attribute —
// internal/modus/element_313.go's own "M x y h w v h h -w v -h " format
// (writeTable2 emits the identical shape) — capturing the cell's own
// top-left x,y and width,height; the trailing "h -w v -h" closing the
// rectangle back to its own start carries no extra information.
var table2CellPathRe = regexp.MustCompile(`^M\s+(-?[0-9.]+)\s+(-?[0-9.]+)\s+h\s+(-?[0-9.]+)\s+v\s+(-?[0-9.]+)`)

// tableBoundaries collects the distinct, sorted set of a Table2's own row
// or column edges from every cell's own start/end coordinate along one
// axis (start[i], start[i]+size[i] for each cell) — a small tolerance
// (0.5 unit) merges two edges floating-point/rounding noise would
// otherwise keep narrowly distinct, since a real Table2's own coordinates
// are always integers in practice.
func tableBoundaries(starts, sizes []float64) []float64 {
	edges := make([]float64, 0, len(starts)*2)
	for i, s := range starts {
		edges = append(edges, s, s+sizes[i])
	}
	sort.Float64s(edges)
	out := make([]float64, 0, len(edges))
	for _, e := range edges {
		if len(out) == 0 || e-out[len(out)-1] > 0.5 {
			out = append(out, e)
		}
	}
	return out
}

// boundaryIndex finds v's own index in a sorted boundaries slice (see
// tableBoundaries), within the same small tolerance — the row/column a
// cell starting at v belongs to, or -1 if none matches closely enough.
func boundaryIndex(boundaries []float64, v float64) int {
	for i, b := range boundaries {
		if math.Abs(b-v) <= 0.5 {
			return i
		}
	}
	return -1
}

// mostCommon returns the most frequently occurring string in vals (ties
// broken by whichever is seen first) — used to recover a Table2's own
// table-wide default Fill from its own cells' real rendered fill colors,
// since a real cell's own style always carries an already-resolved color
// whether or not that cell had a real per-cell override of its own (see
// ClassTable2's own doc comment on TableCell.Fill) — the majority value is
// the table's own real default in the overwhelmingly common case of a
// uniformly-colored table, with any genuinely differing cell still
// recovered exactly via its own TableCell.Fill.
func mostCommon(vals []string) string {
	counts := map[string]int{}
	best, bestCount := "", 0
	for _, v := range vals {
		counts[v]++
		if counts[v] > bestCount {
			best, bestCount = v, counts[v]
		}
	}
	return best
}

// parseTable2 handles shape 313 (Таблица 2/Table 2): a purely decorative
// multi-row/multi-column grid (see ClassTable2's own doc comment for the
// full model, and what it deliberately doesn't recover — cell merging,
// multi-paragraph cell text) — no Ports are ever created for one. A real
// instance is a <g id data-type="313"> (this project's own addition to
// the real source, see ClassTable2's own doc comment for why) wrapping one
// bare <path style> per real cell (table2CellPathRe's own d format) and,
// for a cell that carries one, an immediately-following <text> sibling
// (matching writeTable2's own document-order pairing exactly — real
// xsde2svg emits Path then, only when that cell's own text is non-empty,
// Textspan/Span/TextEnd right after it, the identical adjacency). Row/
// column boundaries (and so each cell's own Row/Col) are reconstructed
// purely from the cells' own real drawn geometry (tableBoundaries/
// boundaryIndex) — there's no other structure to read them from, since a
// real cell carries no row/col index of its own at all. Every real cell
// found gets its own TableCell entry regardless of whether it carries
// text or a Fill/TextColor override, so an entirely blank cell still
// round-trips as a real (empty) grid position rather than silently
// vanishing.
func parseTable2(n *rawNode) (Element, error) {
	id, err := parseElementID(n)
	if err != nil {
		return Element{}, err
	}

	type rawCell struct {
		x, y, w, h float64
		fill       string
		text       string
		textColor  string
	}
	var cells []rawCell
	for i, child := range n.Children {
		if child.Tag != "path" {
			continue
		}
		m := table2CellPathRe.FindStringSubmatch(child.attr("d"))
		if m == nil {
			continue
		}
		x, _ := strconv.ParseFloat(m[1], 64)
		y, _ := strconv.ParseFloat(m[2], 64)
		w, _ := strconv.ParseFloat(m[3], 64)
		h, _ := strconv.ParseFloat(m[4], 64)
		c := rawCell{x: x, y: y, w: w, h: h, fill: styleProp(child.attr("style"), "fill")}
		if i+1 < len(n.Children) && n.Children[i+1].Tag == "text" {
			t := n.Children[i+1]
			c.text = t.Text
			c.textColor = styleProp(t.attr("style"), "fill")
		}
		cells = append(cells, c)
	}
	if len(cells) == 0 {
		return Element{}, fmt.Errorf("slddoc: table2 %s: no cells found", n.attr("id"))
	}

	starts := make([]float64, len(cells))
	sizes := make([]float64, len(cells))
	for i, c := range cells {
		starts[i], sizes[i] = c.x, c.w
	}
	colBounds := tableBoundaries(starts, sizes)
	for i, c := range cells {
		starts[i], sizes[i] = c.y, c.h
	}
	rowBounds := tableBoundaries(starts, sizes)

	fills := make([]string, len(cells))
	for i, c := range cells {
		fills[i] = c.fill
	}
	defaultFill := mostCommon(fills)

	el := Element{
		ID:    id,
		Class: ClassTable2,
		Shape: "313",
		Name:  n.attr("data-name"),
		Layer: resolveLayer(n.attr("data-layer")),
		X:     colBounds[0],
		Y:     rowBounds[0],
		Fill:  defaultFill,
	}
	for i := 1; i < len(colBounds); i++ {
		el.ColumnWidths = append(el.ColumnWidths, colBounds[i]-colBounds[i-1])
	}
	for i := 1; i < len(rowBounds); i++ {
		el.RowHeights = append(el.RowHeights, rowBounds[i]-rowBounds[i-1])
	}

	// A real cell's own style always carries the same stroke/stroke-width/
	// dash regardless of position (the real source's own per-cell override
	// of these is a corpus rarity not modeled — see ClassTable2's own doc
	// comment), so the first cell found is representative of the whole
	// table.
	if firstPaths := n.childrenTagged("path"); len(firstPaths) > 0 {
		style := firstPaths[0].attr("style")
		strokeWidth, _ := strconv.ParseFloat(styleProp(style, "stroke-width"), 64)
		el.Stroke = styleProp(style, "stroke")
		el.StrokeWidth = strokeWidth
		el.LineStyle = table2DashStyles[styleProp(style, "stroke-dasharray")]
	}

	for _, c := range cells {
		row := boundaryIndex(rowBounds, c.y)
		col := boundaryIndex(colBounds, c.x)
		if row < 0 || col < 0 {
			continue
		}
		cell := TableCell{Row: row, Col: col, Text: c.text}
		if c.textColor != "" && c.textColor != "black" {
			cell.TextColor = c.textColor
		}
		if c.fill != "" && c.fill != defaultFill {
			cell.Fill = c.fill
		}
		el.Cells = append(el.Cells, cell)
	}

	return el, nil
}

// parseRoad handles shape 335 (Дорога/Road): a purely decorative
// geographic background line, not real electrical equipment (see
// ClassRoad's own doc comment) — no Ports are ever created for one. A real
// instance is a bare <polyline points style>, no wrapping <g> and no
// data-name (matching writeRoad's own convention — a real Road never
// carries one, unlike a real busbar's own bare polyline), so this reuses
// parseBusBar's own points/style reading exactly, just without a data-name.
func parseRoad(n *rawNode) (Element, error) {
	id, err := parseElementID(n)
	if err != nil {
		return Element{}, err
	}
	pts, err := parsePointList(n.attr("points"))
	if err != nil {
		return Element{}, err
	}
	style := n.attr("style")
	strokeWidth, _ := strconv.ParseFloat(styleProp(style, "stroke-width"), 64)
	return Element{
		ID:          id,
		Class:       ClassRoad,
		Shape:       "335",
		Layer:       resolveLayer(n.attr("data-layer")),
		X:           pts[0].X,
		Y:           pts[0].Y,
		Stroke:      styleProp(style, "stroke"),
		StrokeWidth: strokeWidth,
		Points:      pts,
	}, nil
}

// parsePole handles shape 292 (Опора стоечная/Post-type pole): a purely
// decorative structural marker, not real electrical equipment (see
// ClassPostPole's own doc comment) — no Ports are ever created for one. A
// real instance is a bare <rect> (Square) or <circle>, no wrapping <g> and
// no data-name (matching writePole's own convention, the same gap
// writeRoad's own doc comment notes for Road). n's own tag selects which:
// a <rect>'s own center/half-width become X/Y/Radius, a <circle>'s own
// cx/cy/r are read directly. Orient recovers only the angle from a
// rotate(angle,x,y) transform if present — never its own center, unlike
// GroundSwitch's (54) fallback-anchor case, since this shape's own tag
// already carries its true anchor as a literal attribute either way.
func parsePole(n *rawNode) (Element, error) {
	id, err := parseElementID(n)
	if err != nil {
		return Element{}, err
	}

	var x, y, radius float64
	var square bool
	switch n.Tag {
	case "rect":
		square = true
		rx, errX := strconv.ParseFloat(n.attr("x"), 64)
		ry, errY := strconv.ParseFloat(n.attr("y"), 64)
		w, errW := strconv.ParseFloat(n.attr("width"), 64)
		h, errH := strconv.ParseFloat(n.attr("height"), 64)
		if errX != nil || errY != nil || errW != nil || errH != nil {
			return Element{}, fmt.Errorf("slddoc: pole %s: invalid x/y/width/height", n.attr("id"))
		}
		x, y = rx+w/2, ry+h/2
		radius = w / 2
	case "circle":
		var errX, errY, errR error
		x, errX = strconv.ParseFloat(n.attr("cx"), 64)
		y, errY = strconv.ParseFloat(n.attr("cy"), 64)
		radius, errR = strconv.ParseFloat(n.attr("r"), 64)
		if errX != nil || errY != nil || errR != nil {
			return Element{}, fmt.Errorf("slddoc: pole %s: invalid cx/cy/r", n.attr("id"))
		}
	default:
		return Element{}, fmt.Errorf("slddoc: pole %s: expected <rect> or <circle>, got <%s>", n.attr("id"), n.Tag)
	}

	orient := 0
	if angle, _, ok := parseRotate(n.attr("transform")); ok {
		orient = angle
	}

	style := n.attr("style")
	return Element{
		ID:     id,
		Class:  ClassPostPole,
		Shape:  "292",
		Layer:  resolveLayer(n.attr("data-layer")),
		X:      x,
		Y:      y,
		Orient: orient,
		Radius: radius,
		Fill:   styleProp(style, "fill"),
		Stroke: styleProp(style, "stroke"),
		Square: square,
	}, nil
}

// lineDashStyles is the reverse of render.go's own lineDashPatterns — a
// real instance's own stroke-dasharray value back to the LineStyle that
// produces it. Anything else (absent, or a value neither real dash
// pattern this shape's own source ever produces) reads as
// LineStyleSolid/unset, the same "no known match, fall back to the
// default" approach parseVAlign already uses for a different field.
var lineDashStyles = map[string]ConnectorLineStyle{
	"6,5":     LineStyleDashed,
	"9 2 2 2": LineStyleDashDot,
}

// parseLine handles shape 1 (Линия/Line): a purely decorative generic
// line, not real electrical equipment (see ClassLine's own doc comment) —
// no Ports are ever created for one. A real instance is a bare <polyline
// points style>, no wrapping <g> and no data-name (same gap writeRoad's
// own doc comment notes for Road), so this reuses parseBusBar's own
// points/style reading exactly, just without a data-name, plus its own
// LineStyle recovered from the style's own stroke-dasharray (see
// lineDashStyles).
func parseLine(n *rawNode) (Element, error) {
	id, err := parseElementID(n)
	if err != nil {
		return Element{}, err
	}
	pts, err := parsePointList(n.attr("points"))
	if err != nil {
		return Element{}, err
	}
	style := n.attr("style")
	strokeWidth, _ := strconv.ParseFloat(styleProp(style, "stroke-width"), 64)
	return Element{
		ID:          id,
		Class:       ClassLine,
		Shape:       "1",
		Layer:       resolveLayer(n.attr("data-layer")),
		X:           pts[0].X,
		Y:           pts[0].Y,
		Stroke:      styleProp(style, "stroke"),
		StrokeWidth: strokeWidth,
		LineStyle:   lineDashStyles[styleProp(style, "stroke-dasharray")],
		Points:      pts,
	}, nil
}

// arcPathRe matches a real Arc's (shape 9) own single-command path,
// "M x,y A rx,ry rotation large-arc sweep x,y" (commas or spaces).
var arcPathRe = regexp.MustCompile(`^\s*M\s*(-?[\d.]+)[\s,]+(-?[\d.]+)\s*A\s*(-?[\d.]+)[\s,]+(-?[\d.]+)[\s,]+(-?[\d.]+)[\s,]+([01])[\s,]+([01])[\s,]+(-?[\d.]+)[\s,]+(-?[\d.]+)\s*$`)

// parseArc handles shape 9 (Дуга/Arc): a purely decorative elliptical arc
// (see ClassArc's own doc comment) — no Ports are ever created for one.
// A real instance is a bare <path> carrying a single arc command, read
// back exactly (start/end into Points, radii and both flags into their own
// fields; the real source's own fixed 1° rotation isn't stored — writeArc
// always writes it back). The real source never gives an arc an id, so id
// is what Extract passes in: the path's own id when present, a freshly
// synthesized one otherwise.
func parseArc(n *rawNode, id int) (Element, error) {
	m := arcPathRe.FindStringSubmatch(n.attr("d"))
	if m == nil {
		return Element{}, fmt.Errorf("slddoc: arc %q: not a single arc command", n.attr("d"))
	}
	num := func(i int) float64 {
		v, _ := strconv.ParseFloat(m[i], 64)
		return v
	}
	style := n.attr("style")
	strokeWidth, _ := strconv.ParseFloat(styleProp(style, "stroke-width"), 64)
	start := Point{X: num(1), Y: num(2)}
	end := Point{X: num(8), Y: num(9)}
	return Element{
		ID:          id,
		Class:       ClassArc,
		Shape:       "9",
		Layer:       resolveLayer(n.attr("data-layer")),
		X:           (start.X + end.X) / 2,
		Y:           (start.Y + end.Y) / 2,
		Stroke:      styleProp(style, "stroke"),
		StrokeWidth: strokeWidth,
		Points:      []Point{start, end},
		RadiusX:     num(3),
		RadiusY:     num(4),
		LargeArc:    m[6] == "1",
		Sweep:       m[7] == "1",
	}, nil
}

// polygonDashStyles is the reverse of render.go's own polygonDashPatterns.
var polygonDashStyles = map[string]ConnectorLineStyle{
	"10,20":       LineStyleDotted,
	"70 20 25 20": LineStyleDashDot,
}

// parsePolygon handles shape 16 (Многоугольник/Polygon): a purely
// decorative closed shape (see ClassPolygon's own doc comment) — no Ports
// are ever created for one. A real instance is a bare <polygon points
// style>, no wrapping <g> and no data-name, read the same way parseLine
// reads a Line, plus its own Fill; the anchor is its first vertex.
func parsePolygon(n *rawNode) (Element, error) {
	id, err := parseElementID(n)
	if err != nil {
		return Element{}, err
	}
	pts, err := parsePointList(n.attr("points"))
	if err != nil {
		return Element{}, err
	}
	if len(pts) < 3 {
		return Element{}, fmt.Errorf("slddoc: polygon %d: %d points, want at least 3", id, len(pts))
	}
	style := n.attr("style")
	strokeWidth, _ := strconv.ParseFloat(styleProp(style, "stroke-width"), 64)
	return Element{
		ID:          id,
		Class:       ClassPolygon,
		Shape:       "16",
		Layer:       resolveLayer(n.attr("data-layer")),
		X:           pts[0].X,
		Y:           pts[0].Y,
		Fill:        styleProp(style, "fill"),
		Stroke:      styleProp(style, "stroke"),
		StrokeWidth: strokeWidth,
		LineStyle:   polygonDashStyles[styleProp(style, "stroke-dasharray")],
		Points:      pts,
	}, nil
}

// firstCircleChild returns n itself when it's already a <circle> (the
// older, bare-element real xsde2svg export style several shapes still use —
// Lamp, Junction point, ...), or its first <circle> child/descendant when
// it isn't (a fixed xsde2svg version instead wraps that same circle, plus
// its own optional ParamText/SubscriptName <text> label, in a shared <g id
// data-type="...">, the same restructuring already made for
// PackageSubstation/EnclosedSubstation, Junction point, and Lamp) — nil if
// there's no <circle> anywhere in n's subtree either way.
func firstCircleChild(n *rawNode) *rawNode {
	if n.Tag == "circle" {
		return n
	}
	circles := n.childrenTagged("circle")
	if len(circles) == 0 {
		circles = n.descendants("circle")
	}
	if len(circles) == 0 {
		return nil
	}
	return circles[0]
}

// parseJunctionPoint handles shape 7 (junction point): a small circle
// marking an explicit graph junction. Its own data-voltage attribute
// records the circle's fill (usually the page background for a
// "bussed_link" instance — see Element.Fill's own doc comment), not its
// electrical color, so the voltage is read from the stroke style instead.
// Radius/Fill are read straight from the circle's own r/fill — real corpus
// shows both varying meaningfully per instance (see their own doc comments
// in model.go). Both the older bare <circle data-type="7"> real exports
// still use and a fixed version's own wrapped <g id data-type="7"> form
// (see firstCircleChild) are supported. See parseAttachedLabel (labels.go)
// for how a wrapped instance's own <text> sibling becomes a standalone
// Label, called separately by Extract's own dispatch (not here) since it
// needs this function's already-parsed Element.ID.
func parseJunctionPoint(n *rawNode) (Element, string, error) {
	id, err := parseElementID(n)
	if err != nil {
		return Element{}, "", err
	}
	circle := firstCircleChild(n)
	if circle == nil {
		return Element{}, "", fmt.Errorf("slddoc: junction point %s: no <circle> geometry", n.attr("id"))
	}
	cx, err := strconv.ParseFloat(circle.attr("cx"), 64)
	if err != nil {
		return Element{}, "", fmt.Errorf("slddoc: point %s: %w", n.attr("id"), err)
	}
	cy, err := strconv.ParseFloat(circle.attr("cy"), 64)
	if err != nil {
		return Element{}, "", fmt.Errorf("slddoc: point %s: %w", n.attr("id"), err)
	}
	radius, _ := strconv.ParseFloat(circle.attr("r"), 64)
	style := circle.attr("style")
	return Element{
		ID:     id,
		Class:  ClassJunctionPoint,
		Shape:  "7",
		Name:   n.attr("data-name"),
		Layer:  resolveLayer(n.attr("data-layer")),
		X:      cx,
		Y:      cy,
		Radius: radius,
		Fill:   styleProp(style, "fill"),
		Ports:  []Port{{Name: "1"}},
	}, styleProp(style, "stroke"), nil
}

// forkPathRe matches a real Fork's (shape 26) own path, "M x y l h -h m
// -h h l -h -h" (element_26.go), capturing its anchor and arm length.
var forkPathRe = regexp.MustCompile(`^\s*M\s*(-?[\d.]+)[\s,]+(-?[\d.]+)\s*l\s*(-?[\d.]+)[\s,]+-?[\d.]+\s*m`)

// normalizeOrient folds any rotate() angle (the real source writes e.g.
// -270, 270 and -180) into the 0/90/180/-90 set Properties offers.
func normalizeOrient(angle int) int {
	a := ((angle % 360) + 360) % 360
	if a == 270 {
		return -90
	}
	return a
}

// parseFork handles shape 26 (Развилка/Fork): a bare <path> "V" whose
// vertex is its anchor, optionally carrying its own rotate(angle,x,y)
// transform (see ClassFork's own doc comment). Its three ports are the
// vertex and both arm tips, rotated through that transform. The real
// source never gives it an id, so id is what Extract passes in (the path's
// own when present, a synthesized one otherwise). Radius is only set for
// an arm length other than the default, so a default-size fork looks the
// same as a freshly placed one.
func parseFork(n *rawNode, id int) (Element, []Point, string, error) {
	m := forkPathRe.FindStringSubmatch(n.attr("d"))
	if m == nil {
		return Element{}, nil, "", fmt.Errorf("slddoc: fork %q: unrecognized path", n.attr("d"))
	}
	x, _ := strconv.ParseFloat(m[1], 64)
	y, _ := strconv.ParseFloat(m[2], 64)
	h, _ := strconv.ParseFloat(m[3], 64)
	anchor := Point{X: x, Y: y}
	angle := 0
	if a, _, ok := parseRotate(n.attr("transform")); ok {
		angle = a
	}
	ports := []Point{anchor, {X: x + h, Y: y - h}, {X: x - h, Y: y - h}}
	for i := range ports {
		ports[i] = rotate(ports[i], anchor, float64(angle))
	}
	el := Element{
		ID:     id,
		Class:  ClassFork,
		Shape:  "26",
		Layer:  resolveLayer(n.attr("data-layer")),
		X:      x,
		Y:      y,
		Orient: normalizeOrient(angle),
		Ports:  []Port{{Name: "1"}, {Name: "2"}, {Name: "3"}},
	}
	if h != forkArmLength {
		el.Radius = h
	}
	return el, ports, n.attr("data-voltage"), nil
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
// lamps clustered in triplets next to a breaker (see Examples.svg). Every
// real xsde2svg export found so far draws this as a bare <circle
// data-type="106">, but a fixed version instead wraps it (plus its own
// optional ParamText/SubscriptName <text> label) in a shared <g id
// data-type="106">, the same restructuring already made for Junction point
// (7) — both forms are supported here (see firstCircleChild); data-state/
// data-fill/data-name/data-voltage all still resolve correctly either way,
// since parseState/parseDataFill and this function's own n.attr calls for
// those already check n itself, which the dispatch loop matches on
// data-type regardless of which form put it there. See parseAttachedLabel
// (labels.go) for how a wrapped instance's own <text> sibling becomes a
// standalone Label, called separately by Extract's own dispatch (not
// here) since it needs this function's already-parsed Element.ID.
func parseLamp(n *rawNode) (Element, error) {
	id, err := parseElementID(n)
	if err != nil {
		return Element{}, err
	}
	circle := firstCircleChild(n)
	if circle == nil {
		return Element{}, fmt.Errorf("slddoc: lamp %s: no <circle> geometry", n.attr("id"))
	}
	cx, err := strconv.ParseFloat(circle.attr("cx"), 64)
	if err != nil {
		return Element{}, fmt.Errorf("slddoc: lamp %s: %w", n.attr("id"), err)
	}
	cy, err := strconv.ParseFloat(circle.attr("cy"), 64)
	if err != nil {
		return Element{}, fmt.Errorf("slddoc: lamp %s: %w", n.attr("id"), err)
	}
	radius, err := strconv.ParseFloat(circle.attr("r"), 64)
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

// parseConnectorArrow handles shape 83 (Коннектор-стрелка/Connector arrow,
// see ClassConnectorArrow): a <g data-type="83"> holding the line and the
// arrowhead. element_83.go writes the line either as "M x y l dx dy" (an
// axis-aligned arrow; direction and length from the vector) or as
// "M x y h len" under rotate(angle,x,y) (a diagonal one); an extra group
// rotation can come on top of either. The tail (x,y) is the anchor and the
// one port; the length adds the 11-unit arrowhead the line stops short of.
func parseConnectorArrow(n *rawNode) (Element, []Point, error) {
	id, err := parseElementID(n)
	if err != nil {
		return Element{}, nil, err
	}
	paths := n.childrenTagged("path")
	if len(paths) < 2 {
		return Element{}, nil, fmt.Errorf("slddoc: connector arrow %d: want a line and an arrowhead path", id)
	}
	toks := pathTokenRe.FindAllString(paths[0].attr("d"), -1)
	num := func(i int) (float64, bool) {
		if i >= len(toks) {
			return 0, false
		}
		v, err := strconv.ParseFloat(toks[i], 64)
		return v, err == nil
	}
	if len(toks) < 4 || toks[0] != "M" {
		return Element{}, nil, fmt.Errorf("slddoc: connector arrow %d: unexpected line path", id)
	}
	x, okX := num(1)
	y, okY := num(2)
	var dx, dy float64
	var okD bool
	switch toks[3] {
	case "l":
		var okDy bool
		dx, okD = num(4)
		dy, okDy = num(5)
		okD = okD && okDy
	case "h":
		dx, okD = num(4)
	}
	if !okX || !okY || !okD {
		return Element{}, nil, fmt.Errorf("slddoc: connector arrow %d: unexpected line path", id)
	}
	angle := 0.0
	if dx != 0 || dy != 0 {
		angle = math.Atan2(dy, dx) * 180 / math.Pi
	}
	if a, _, ok := parseRotate(n.attr("transform")); ok {
		angle += float64(a)
	}
	head := paths[1].attr("style")
	el := Element{
		ID:         id,
		Class:      ClassConnectorArrow,
		Shape:      "83",
		Name:       n.attr("data-name"),
		Layer:      resolveLayer(n.attr("data-layer")),
		X:          x,
		Y:          y,
		Orient:     normalizeOrient(int(math.Round(angle))),
		Stroke:     styleProp(paths[0].attr("style"), "stroke"),
		HeadStroke: styleProp(head, "stroke"),
		Fill:       styleProp(head, "fill"),
		Ports:      []Port{{Name: "1"}},
	}
	if l := math.Round((math.Hypot(dx, dy)+connectorArrowHeadLength)*100) / 100; l != connectorArrowLength {
		el.Length = l
	}
	return el, []Point{{X: x, Y: y}}, nil
}

// parseConnectorPoint handles shape 10 (Коннектор/Connector, see
// ClassConnectorPoint): a real bare <rect data-type="10"> 10x10 on its
// anchor, or Render's <g data-type="10"> holding that <rect>. The anchor
// and its one port are the square's center, its stroke the element's own
// color (not registered as a voltage class), and Orient the rotate()
// angle when there is one.
func parseConnectorPoint(n *rawNode) (Element, []Point, error) {
	id, err := parseElementID(n)
	if err != nil {
		return Element{}, nil, err
	}
	rect := n
	if n.Tag != "rect" {
		rects := n.descendants("rect")
		if len(rects) == 0 {
			return Element{}, nil, fmt.Errorf("slddoc: connector %d: no <rect> geometry", id)
		}
		rect = rects[0]
	}
	x, errX := strconv.ParseFloat(rect.attr("x"), 64)
	y, errY := strconv.ParseFloat(rect.attr("y"), 64)
	w, errW := strconv.ParseFloat(rect.attr("width"), 64)
	h, errH := strconv.ParseFloat(rect.attr("height"), 64)
	if errX != nil || errY != nil || errW != nil || errH != nil {
		return Element{}, nil, fmt.Errorf("slddoc: connector %d: invalid x/y/width/height", id)
	}
	center := Point{X: x + w/2, Y: y + h/2}
	el := Element{
		ID:     id,
		Class:  ClassConnectorPoint,
		Shape:  "10",
		Name:   n.attr("data-name"),
		Layer:  resolveLayer(n.attr("data-layer")),
		X:      center.X,
		Y:      center.Y,
		Stroke: styleProp(rect.attr("style"), "stroke"),
		Ports:  []Port{{Name: "1"}},
	}
	if angle, _, ok := parseRotate(n.firstAttrDescendant("transform")); ok {
		el.Orient = normalizeOrient(angle)
	}
	return el, []Point{center}, nil
}

// parseLampOnPole handles shape 320002 (Лампа на опоре/Lamp on pole, see
// ClassLampOnPole): a <g data-type="320002"> holding a circle and two
// diagonal paths. The anchor is the circle's center, its stroke the
// element's own color (data-voltage carries that same color, not a voltage,
// so it isn't registered as a voltage class), and Orient comes from the
// group's rotate(angle,x,y), present only when the angle isn't 0.
func parseLampOnPole(n *rawNode) (Element, error) {
	id, err := parseElementID(n)
	if err != nil {
		return Element{}, err
	}
	circle := firstCircleChild(n)
	if circle == nil {
		return Element{}, fmt.Errorf("slddoc: lamp on pole %s: no <circle> geometry", n.attr("id"))
	}
	cx, errX := strconv.ParseFloat(circle.attr("cx"), 64)
	cy, errY := strconv.ParseFloat(circle.attr("cy"), 64)
	if errX != nil || errY != nil {
		return Element{}, fmt.Errorf("slddoc: lamp on pole %s: invalid cx/cy", n.attr("id"))
	}
	el := Element{
		ID:     id,
		Class:  ClassLampOnPole,
		Shape:  "320002",
		Name:   n.attr("data-name"),
		Layer:  resolveLayer(n.attr("data-layer")),
		X:      cx,
		Y:      cy,
		Stroke: styleProp(circle.attr("style"), "stroke"),
	}
	if angle, _, ok := parseRotate(n.attr("transform")); ok {
		el.Orient = normalizeOrient(angle)
	}
	return el, nil
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

// powerflowGlyphState is the reverse of render.go's own powerflowGlyph — a
// real instance's own text content back to State: "←" reads as 1, anything
// else (the common "→", or an unrecognized value) as nil/unset, matching
// writePowerflowIndicator's own nil-draws-"→" default.
func powerflowGlyphState(text string) *int {
	if strings.TrimSpace(text) == "←" {
		one := 1
		return &one
	}
	return nil
}

// parsePowerflowIndicator handles shape 320001 (Направление перетока/
// Powerflow direction): a purely decorative arrow glyph, not real
// electrical equipment (see ClassPowerflowIndicator's own doc comment) — no
// Ports are ever created for one. A real instance is a bare <text x y style
// transform data-type data-angle data-voltage>→|←</text>, no wrapping <g>
// (same gap writeLine/writeRoad's own doc comments note). Unlike
// parsePole's own conditional rotate parsing, the real source always emits
// both the rotate() transform and its own data-angle attribute (even for
// angle 0), so Orient is read directly from data-angle rather than via
// parseRotate.
func parsePowerflowIndicator(n *rawNode) (Element, error) {
	id, err := parseElementID(n)
	if err != nil {
		return Element{}, err
	}
	x, errX := strconv.ParseFloat(n.attr("x"), 64)
	y, errY := strconv.ParseFloat(n.attr("y"), 64)
	if errX != nil || errY != nil {
		return Element{}, fmt.Errorf("slddoc: powerflow indicator %s: invalid x/y", n.attr("id"))
	}
	orient, _ := strconv.Atoi(n.attr("data-angle"))
	style := n.attr("style")
	return Element{
		ID:        id,
		Class:     ClassPowerflowIndicator,
		Shape:     "320001",
		Layer:     resolveLayer(n.attr("data-layer")),
		X:         x,
		Y:         y - 3,
		Orient:    orient,
		State:     powerflowGlyphState(n.Text),
		TextColor: styleProp(style, "fill"),
	}, nil
}

// connectorKindByType is the reverse of render.go's own connectorTypeCode
// (must be kept in exact sync with it) — "21" is KindBusWork's own code,
// not KindBusbarWire's: real xsde2svg data-type="21" is a plain Buswork
// connector ("Ошиновка"), and KindBusbarWire has no code of its own at
// all (connectorTypeCode has no entry for it — it was removed as a
// palette choice, see wireKindIcon.ts's own doc comment, so a real
// instance is never produced by this editor and Extract never needs to
// recover one either).
var connectorKindByType = map[string]ConnectorKind{
	"21": KindBusWork,
	"22": KindOverheadLine,
	"23": KindCableLine,
	"28": KindLinkToObject,
}

// parseConnector handles the generic wire shapes (21, 22, 23, 28): a plain
// polyline whose two ends are the electrical connection points. An overhead
// or cable line (22/23) is instead a named <g id data-type data-name
// data-voltage> wrapping that polyline (see writeNamedLine), so a <g> takes
// its geometry from its first <polyline> child and its Name from data-name.
func parseConnector(n *rawNode) (Connector, string, error) {
	id, err := parseElementID(n)
	if err != nil {
		return Connector{}, "", err
	}
	geom := n
	if n.Tag == "g" {
		polys := n.childrenTagged("polyline")
		if len(polys) == 0 {
			return Connector{}, "", fmt.Errorf("slddoc: connector %s: no <polyline> geometry", n.attr("id"))
		}
		geom = polys[0]
	}
	pts, err := parsePointList(geom.attr("points"))
	if err != nil {
		return Connector{}, "", err
	}
	kind := connectorKindByType[n.attr("data-type")]
	return Connector{
		ID:     id,
		Kind:   kind,
		Name:   n.attr("data-name"),
		Layer:  resolveLayer(n.attr("data-layer")),
		Dashed: styleProp(geom.attr("style"), "stroke-dasharray") != "",
		Points: pts,
	}, n.attr("data-voltage"), nil
}

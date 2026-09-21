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
// every real rotated instance found this way. NType is read straight off
// the real source's own data-ntype export attribute (added to xsde2svg's
// own element_385.go specifically so this could be recovered at all, since
// neither of the shape's two real appearance variants otherwise leaves any
// other trace of which one was drawn). PropertyText is the first
// descendant <text>'s own content — see Element.PropertyText's own doc
// comment; "" when the real instance carries none, the common case. Fill
// (the real source's own Abonent flag) and State (its own Tech.Closed
// dashing) are not recovered — a known gap, same spirit as Arrow's own
// DoubleHeaded not being recoverable from geometry alone.
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
	ntype, _ := strconv.Atoi(n.attr("data-ntype"))
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
		PropertyText: substationPropertyText(n),
		Ports:        []Port{{Name: "1"}},
	}, []Point{anchor}, n.attr("data-voltage"), nil
}

// parseEnclosedSubstation handles shape 386 (Enclosed transformer
// substation, ZTP): same anchor/Orient/PropertyText recovery as
// parsePackageSubstation (see its own doc comment) — this shape just has
// no NType to recover.
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
	x, errX := strconv.ParseFloat(n.attr("x"), 64)
	y, errY := strconv.ParseFloat(n.attr("y"), 64)
	w, errW := strconv.ParseFloat(n.attr("width"), 64)
	h, errH := strconv.ParseFloat(n.attr("height"), 64)
	if errX != nil || errY != nil || errW != nil || errH != nil {
		return Element{}, fmt.Errorf("slddoc: rectangle %s: invalid x/y/width/height", n.attr("id"))
	}
	style := n.attr("style")
	// StrokeWidth left at 0 (unset) when absent or unparseable — Render's
	// own fallback already treats that as 1, the real source's own
	// minimum (mathext.Max(uint(1), ...) in element_3.go), so there's no
	// need to hardcode that default a second time here.
	strokeWidth, _ := strconv.ParseFloat(styleProp(style, "stroke-width"), 64)
	return Element{
		ID:          id,
		Class:       ClassRectangle,
		Shape:       "3",
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

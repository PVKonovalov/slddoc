package slddoc

import (
	"fmt"
	"io"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// defaultBackground is used when a Diagram carries no Editor.Background.
const defaultBackground = "#12161d"

// RenderMode controls whether Render adds this editor's own
// interactivity-only markup on top of an otherwise xsde2svg-faithful
// rendering.
type RenderMode int

const (
	// Static renders a clean, xsde2svg-faithful document: no
	// data-editor-kind attribute, no invisible hit-target geometry. Use
	// this for anything written to disk or served as a downloadable
	// artifact (the saved .svg, GET /diagrams/:name/svg) — an id, when the
	// element/connector has one, is still present either way, since a real
	// xsde2svg document carries one too.
	Static RenderMode = iota
	// Interactive adds a data-editor-kind="element"|"connector" attribute
	// so the frontend's canvas can hit-test a click against the real
	// rendered markup (the frontend computes its own bounding box per
	// element for a click-tolerance fallback, rather than this package
	// adding any invisible hit-target geometry of its own). Never written
	// to disk — use this only for the live in-app preview (POST /render).
	Interactive
)

var attrEscaper = strings.NewReplacer(`&`, "&amp;", `<`, "&lt;", `>`, "&gt;", `"`, "&quot;")

func esc(s string) string { return attrEscaper.Replace(s) }

func fmtNum(v float64) string {
	if v == math.Trunc(v) {
		return strconv.FormatInt(int64(v), 10)
	}
	return strconv.FormatFloat(v, 'g', -1, 64)
}

// StateColor is one entry of the install-wide Open/Close/Intermediate
// legend a switching device's state-driven {fill} and data-fill attribute
// are drawn from (see config.Config.StateColors) — supplied by the caller
// rather than fixed in this package, since it's a deployment's own choice
// of colors/labels, not part of the diagram or the symbol library.
type StateColor struct {
	State int
	Label string
	Color string
}

// stateColorSet is StateColors reshaped for fast per-element lookup plus
// the pre-built data-fill legend string, computed once per Render call
// rather than once per element.
type stateColorSet struct {
	colors   map[int]string
	fillAttr string
}

func newStateColorSet(colors []StateColor) stateColorSet {
	set := stateColorSet{colors: make(map[int]string, len(colors))}
	parts := make([]string, len(colors))
	for i, c := range colors {
		set.colors[c.State] = c.Color
		parts[i] = fmt.Sprintf("%d:%s", c.State, c.Color)
	}
	if len(parts) > 0 {
		set.fillAttr = fmt.Sprintf(` data-fill="%s"`, strings.Join(parts, ","))
	}
	return set
}

// fill resolves a switching device's current-state fill: the configured
// color for its State, or "none" when State was never recorded or doesn't
// match any configured entry.
func (s stateColorSet) fill(state *int) string {
	if state == nil {
		return "none"
	}
	if c, ok := s.colors[*state]; ok {
		return c
	}
	return "none"
}

// fpiColor resolves a FaultPassageIndicator's own ring/text color from its
// State, using a separate legend from switching devices' own fill (see
// config.Config.FPIStateColors) — its ring is always colored, unlike a
// switching device's {fill}, so an unrecorded State defaults to 0 (Open)
// rather than "none".
func (s stateColorSet) fpiColor(state *int) string {
	st := 0
	if state != nil {
		st = *state
	}
	if c, ok := s.colors[st]; ok {
		return c
	}
	return "none"
}

// stateAttr is a real xsde2svg breaker/switch's data-state attribute on its
// state-indicator path: the element's own raw State value, or no attribute
// at all when State was never recorded (fill's "none" case has no
// analogous data-state — there is no value to publish).
func stateAttr(state *int) string {
	if state == nil {
		return ""
	}
	return fmt.Sprintf(` data-state="%d"`, *state)
}

// positionAttr is a withdrawable device's own data-trolley attribute on the
// inner <g> wrapping its movable body (see base.xml shapes 43/49) — the
// element's own raw Position value, or no attribute at all when Position
// was never recorded, matching stateAttr's own convention.
func positionAttr(position *int) string {
	if position == nil {
		return ""
	}
	return fmt.Sprintf(` data-trolley="%d"`, *position)
}

// positionOffset is the x-shift (in local template units) applied to a
// withdrawable device's own movable body: 0 for the default/Normal
// position (1, or unrecorded), matching this editor's own reference
// (sld-viewer's applyTrolleyState) rather than the real xsde2svg exporter's
// own per-shape behavior, which only shifts for Service and leaves Test at
// the same x-origin — sld-viewer's simplified, direction-agnostic offset
// (both Service and Test eject the same way) is what this editor's own
// Position status mechanism was modeled on.
func positionOffset(position *int) string {
	if position == nil || *position == 1 {
		return "0"
	}
	return "10"
}

// stateLineRe matches a template's {state:parallel|perpendicular|diagonal}
// placeholder: the switch-like devices' internal state indicator, drawn
// parallel to the device's own (locally-vertical) axis when closed (state
// 1), perpendicular when open (state 0), and at 45° for any other recorded
// state.
var stateLineRe = regexp.MustCompile(`\{state:([^|}]*)\|([^|}]*)\|([^}]*)\}`)

func applyStateLine(tmpl string, state *int) string {
	return stateLineRe.ReplaceAllStringFunc(tmpl, func(m string) string {
		g := stateLineRe.FindStringSubmatch(m)
		switch {
		case state == nil, *state == 1:
			return g[1]
		case *state == 0:
			return g[2]
		default:
			return g[3]
		}
	})
}

// lampColor picks a Lamp element's current display color: FillOn when
// State records 1 (lit), FillOff otherwise (including an unrecorded
// state).
func lampColor(e Element) string {
	color := e.FillOff
	if e.State != nil && *e.State == 1 {
		color = e.FillOn
	}
	if color == "" {
		color = "none"
	}
	return color
}

// shapeName gives the equipment name Render annotates a run of same-Shape
// elements with. Kept per-shape rather than per-Class since a fixed
// breaker and a withdrawable one, e.g., are both ClassBreaker but draw and
// are labeled differently.
var shapeName = map[string]string{
	"2":      "Arrow",
	"3":      "Rectangle",
	"4":      "Circle",
	"7":      "Junction point",
	"14":     "Non-intersection",
	"56":     "Cable connector",
	"24":     "Busbar",
	"31":     "Ground terminal",
	"33":     "Choke coil",
	"34":     "Current transformer",
	"55":     "Voltage transformer",
	"37":     "Reactor",
	"397":    "Reactor (shunt)",
	"35":     "Surge arrester",
	"29":     "Surge arrester",
	"168":    "Surge arrester",
	"41":     "Breaker",
	"42":     "Load-break switch",
	"43":     "Breaker (withdrawable)",
	"47":     "Power transformer",
	"49":     "Disconnector (withdrawable)",
	"54":     "Ground switch",
	"71":     "Disconnector",
	"76":     "Starter",
	"106":    "Lamp",
	"154":    "Fuse (withdrawable)",
	"162":    "Disconnector",
	"164":    "Sectionalizer",
	"172":    "Capacitor bank",
	"52":     "Half-chassis",
	"51":     "Chassis",
	"173":    "Generator",
	"203":    "Fuse",
	"388":    "Capacitor",
	"320003": "Fault passage indicator",
}

// connectorKindName gives the name Render annotates a run of same-Kind
// connectors with, the same way shapeName does for elements.
var connectorKindName = map[string]string{
	string(KindBusbarWire):   "Busbar wire",
	string(KindOverheadLine): "Overhead line",
	string(KindCableLine):    "Cable line",
	string(KindBusWork):      "Buswork",
	string(KindLinkToObject): "Object link",
}

// connectorTypeCode gives a Connector's data-type, mirroring an Element's
// Shape-as-data-type: the xsde2svg type code for a generic object-to-object
// connection (this schema's ClassObjectLink/KindBusWork — what
// diagramOps.connectElements creates) is 21, KindOverheadLine's is 22
// (confirmed against sld-viewer/assets/sld/IEEE9bus.svg's own real
// xsde2svg-catalog-code documentation), KindCableLine's is 23, and
// KindLinkToObject's is 28. A Kind absent from this map renders with no
// data-type, same as before this existed.
var connectorTypeCode = map[ConnectorKind]string{
	KindBusWork:      "21",
	KindOverheadLine: "22",
	KindCableLine:    "23",
	KindLinkToObject: "28",
}

// namedLineStrokeWidth is a KindOverheadLine/KindCableLine connector's
// fixed stroke width — 1.5, heavier than an ordinary wire's 1px but
// lighter than a busbar's 4px. Confirmed against a real xsde2svg-exported
// overhead line exactly (same corpus file as connectorTypeCode's own
// comment); applied to cable line too for consistency between the two
// "named" connector kinds, absent its own corpus confirmation.
const namedLineStrokeWidth = 1.5

// cableLineDashPatterns maps a KindCableLine connector's own LineStyle to
// its real stroke-dasharray value, matching xsde2svg's own line-style
// switch (xsde2svg/internal/modus/element_23.go) exactly — LineStyleSolid
// resolves to "", same as every other kind's default. KindOverheadLine
// gets no such per-instance style — it stays solid unless the general
// Connector.Dashed flag is set, same as an ordinary wire.
var cableLineDashPatterns = map[ConnectorLineStyle]string{
	LineStyleSolid:   "",
	LineStyleDashed:  "stroke-dasharray: 6,5;",
	LineStyleDashDot: "stroke-dasharray: 70 20 25 20;",
	LineStyleDotted:  "stroke-dasharray: 3,2;",
}

// resolveCableLineDash resolves a KindCableLine connector's own dash
// pattern: an empty/unset LineStyle defaults to LineStyleDashed (this
// editor's original hardcoded cable-line dash, before this field
// existed, so an already-saved diagram keeps rendering the same way),
// and any value this editor doesn't recognize falls back to that same
// default rather than silently rendering solid.
func resolveCableLineDash(style ConnectorLineStyle) string {
	if style == "" {
		style = LineStyleDashed
	}
	if dash, ok := cableLineDashPatterns[style]; ok {
		return dash
	}
	return cableLineDashPatterns[LineStyleDashed]
}

// typeComment writes a "<!-- Name:shape -->" line the first time shape is
// seen or whenever it changes from the previous call, so consecutive
// same-shape elements/connectors get one header rather than a redundant
// repeat for every instance. last is updated in place.
func typeComment(w io.Writer, names map[string]string, key, code string, last *string) {
	if key == "" || key == *last {
		return
	}
	*last = key
	name, ok := names[key]
	if !ok {
		name = key
	}
	label := name
	if code != "" {
		label += ":" + code
	}
	fmt.Fprintf(w, "<!-- %s -->\n", esc(label))
}

// elementZOrder ranks the handful of Element classes that must draw above
// connectors rather than in ordinary document order, instead of the default
// 0 (drawn in document order, before connectors): JunctionPoint and
// FaultPassageIndicator sit directly on top of a wire — unlike ordinary
// equipment, which only ever touches a connector at a port, so painting the
// wire afterward would cut through them — and Lamp is a decorative status
// indicator meant to read as foreground UI. Render draws every such class in
// ascending order of this value, each tier after the connectors loop.
var elementZOrder = map[Class]int{
	ClassJunctionPoint:         1,
	ClassLamp:                  1,
	ClassFaultPassageIndicator: 1,
}

// renderElement writes one Element's symbol (or, for a BusBarSection, its
// drawn polyline), recording its Shape in missing/seenMissing when lib has
// no template for it. lastShape tracks the running type-comment header, the
// same way across whichever pass of Render calls it.
func renderElement(w io.Writer, lib *SymbolLibrary, voltageColor map[int]string, stateColors, fpiColors stateColorSet, e Element, missing *[]string, seenMissing map[string]bool, lastShape *string, mode RenderMode) {
	typeComment(w, shapeName, e.Shape, e.Shape, lastShape)

	var color string
	if e.Class == ClassLamp {
		// A Lamp's colors are its own FillOff/FillOn pair, not a
		// VoltageClass — it isn't part of the electrical network.
		color = lampColor(e)
	} else if e.Class == ClassFaultPassageIndicator {
		// {color} is only its own fixed background fill (the ring reads
		// as hollow against the canvas) — it isn't part of the electrical
		// network, so this never varies. Its own State instead drives
		// {fpiColor}, the ring/text color, via fpiColors below.
		color = defaultBackground
	} else {
		color = voltageColor[e.Voltage]
		if color == "" {
			// A PowerTransformer's two windings can carry different
			// voltages that this schema doesn't record per-port; fall
			// back to a visible neutral color rather than emitting an
			// empty stroke.
			color = "gray"
		}
	}
	if e.Class == ClassBusBarSection {
		// A busbar's own data-name/data-voltage/data-type mirror what a
		// real xsde2svg-exported busbar polyline carries (data-voltage is
		// the resolved color, not a VoltageClass id — this schema has no
		// separate concept of one for a bare polyline); drawn at 4px, a
		// deliberately heavier stroke than an ordinary wire's 1px, since a
		// busbar reads as the diagram's backbone, not just another wire.
		dataAttrs := fmt.Sprintf(" data-name=\"%s\" data-voltage=\"%s\" data-type=\"%s\"", esc(e.Name), esc(color), esc(e.Shape))
		writePolyline(w, e.ID, "element", e.Points, color, false, 4, dataAttrs, mode)
		return
	}
	if e.Class == ClassPowerTransformer {
		// A PowerTransformer's real geometry (circle count/position, each
		// winding's own connection glyph) is driven entirely by its own
		// Windings — not template substitution — so it bypasses the
		// template lookup below the same way ClassBusBarSection does.
		writePowerTransformer(w, e, voltageColor, color, mode)
		return
	}
	if e.Class == ClassRectangle {
		// A Rectangle's own size varies per instance (unlike every
		// template-drawn shape's fixed local geometry) and isn't part of
		// the electrical network at all (no Voltage to resolve color
		// from) — bypasses the template lookup below the same way
		// ClassBusBarSection/ClassPowerTransformer do, using its own
		// Fill/Stroke instead of the color computed above.
		writeRectangle(w, e, mode)
		return
	}
	if e.Class == ClassArrow {
		// Same reasoning as ClassRectangle just above — a decorative
		// annotation whose own geometry varies per instance and isn't
		// part of the electrical network, using its own Stroke instead of
		// the color computed above.
		writeArrow(w, e, mode)
		return
	}
	if e.Class == ClassCircle {
		// Same reasoning as ClassRectangle just above (it shares that
		// shape's own Fill/Stroke/Points convention exactly, just drawn
		// as an <ellipse>).
		writeCircle(w, e, mode)
		return
	}

	tmpl, ok := lib.templates[e.Shape]
	if !ok {
		if !seenMissing[e.Shape] {
			seenMissing[e.Shape] = true
			*missing = append(*missing, e.Shape)
		}
		return
	}
	body := applyStateLine(tmpl, e.State)
	// {counterRotate} is the negated Orient — wrapping a template fragment
	// in <g transform="rotate({counterRotate})"> cancels the outer <g
	// transform="...rotate(Orient)"> this function itself emits below, so
	// that fragment (e.g. FaultPassageIndicator's own "FPI" label) stays
	// upright regardless of the element's own rotation, while everything
	// else in the template (and the element's own terminals) still rotates
	// normally.
	body = strings.NewReplacer(
		"{color}", esc(color),
		"{fill}", stateColors.fill(e.State),
		"{radius}", fmtNum(e.Radius),
		"{fillAttr}", stateColors.fillAttr,
		"{stateAttr}", stateAttr(e.State),
		"{positionAttr}", positionAttr(e.Position),
		"{positionOffset}", positionOffset(e.Position),
		"{fpiColor}", fpiColors.fpiColor(e.State),
		"{counterRotate}", fmtNum(float64(-e.Orient)),
	).Replace(body)
	// data-editor-kind (this editor's own addition, not part of the
	// xsde2svg format) is what the frontend hit-tests against — it no
	// longer pairs with any invisible fixed-radius hit-target geometry
	// here (a single generously-sized circle around every symbol's own
	// anchor used to give small/thin templates a comfortable click area,
	// but that same fixed size either undershot a large or
	// anchor-offset-from-its-own-body shape like VoltageTransformer, or
	// overshot a small one enough to swallow a neighbor's click); the
	// frontend now computes each element's own real rendered bounding box
	// instead and uses that for both its selection highlight and a
	// click-tolerance fallback — see Canvas.tsx's own elementBoxes.
	// data-voltage/data-type mirror a real xsde2svg element's outer <g>
	// either way; id itself is just the element's own bare id.
	editorAttr := ""
	if mode == Interactive {
		editorAttr = " data-editor-kind=\"element\""
	}
	fmt.Fprintf(w, "<g id=\"%d\" data-name=\"%s\" data-voltage=\"%s\" data-type=\"%s\"%s transform=\"translate(%s,%s) rotate(%d)%s\">\n%s\n</g>\n",
		e.ID, esc(e.Name), esc(color), esc(e.Shape), editorAttr, fmtNum(e.X), fmtNum(e.Y), e.Orient, mirrorScale(e.Mirror), body)
}

// mirrorScale is a Mirror'd element's own extra transform component — a
// horizontal flip in the symbol's local frame, applied (via SVG transform
// composition order) before Orient's own rotation, matching the real
// xsde2svg source's own xMirror convention (see element_164.go's own
// mirroring branches, for one real example of what this flips between).
// Empty when unset, so an ordinary unmirrored element's own transform
// looks exactly as it always has.
func mirrorScale(mirror bool) string {
	if !mirror {
		return ""
	}
	return " scale(-1,1)"
}

// Render writes d as a fresh SVG document, using lib to place each
// Element's symbol. The output is a new, independently generated rendering
// of the diagram, not a byte-for-byte reproduction of any source file.
//
// Elements are drawn in three passes rather than strict document order: any
// Class absent from elementZOrder (the default, effectively 0) first, then
// connectors, then each elementZOrder tier in ascending order, and finally
// labels — see elementZOrder's doc comment for why.
//
// If d.Elements references a Shape absent from lib, Render still writes
// every other element and connector, then returns an error listing every
// missing shape once rendering is otherwise complete, so a single run
// surfaces the whole gap instead of stopping at the first one.
//
// mode controls whether this editor's own interactivity-only markup
// (data-editor-kind, wider hit targets) is added on top of the otherwise
// xsde2svg-faithful output — see RenderMode's doc comment.
//
// fpiStateColorLegend is the install-wide Open/Close/Intermediate legend a
// FaultPassageIndicator's own ring/text color is drawn from (see
// config.Config.FPIStateColors) — a separate legend from stateColorLegend
// since an FPI's own Open/Close meaning (and thus color) is inverted from a
// switching device's (see config.Config.FPIStateColors's own doc comment).
// Pass nil for a caller that hasn't wired one up (every FPI then renders
// with an unresolved "none" ring/text color, same as a switching device
// with no stateColorLegend).
//
// stateColorLegend is the install-wide Open/Close/Intermediate legend a
// switching device's state-driven fill is drawn from (see
// config.Config.StateColors) — omit it to render every such device with an
// unresolved ("none") fill and no data-fill attribute, e.g. from a caller
// that hasn't wired up a legend.
func Render(d *Diagram, lib *SymbolLibrary, w io.Writer, mode RenderMode, fpiStateColorLegend []StateColor, stateColorLegend ...StateColor) error {
	voltageColor := map[int]string{}
	for _, vc := range d.VoltageClasses {
		voltageColor[vc.ID] = vc.Color
	}
	stateColors := newStateColorSet(stateColorLegend)
	fpiColors := newStateColorSet(fpiStateColorLegend)

	background := defaultBackground
	if d.Editor != nil && d.Editor.Background != "" {
		background = d.Editor.Background
	}

	fmt.Fprintf(w, "<?xml version=\"1.0\"?>\n<svg width=\"%s\" height=\"%s\" style=\"stroke-width: 0px; background-color: %s;\" xmlns=\"http://www.w3.org/2000/svg\" xmlns:xlink=\"http://www.w3.org/1999/xlink\">\n",
		fmtNum(d.Width), fmtNum(d.Height), esc(background))

	var missing []string
	seenMissing := map[string]bool{}

	elevated := map[int][]Element{}

	var lastShape string
	for _, e := range d.Elements {
		if z := elementZOrder[e.Class]; z > 0 {
			elevated[z] = append(elevated[z], e)
			continue
		}
		renderElement(w, lib, voltageColor, stateColors, fpiColors, e, &missing, seenMissing, &lastShape, mode)
	}

	var lastConnKind string
	for _, c := range d.Connectors {
		code, hasCode := connectorTypeCode[c.Kind]
		typeComment(w, connectorKindName, string(c.Kind), code, &lastConnKind)
		if c.Kind == KindOverheadLine || c.Kind == KindCableLine {
			writeNamedLine(w, c, voltageColor[c.Voltage], code, mode)
			continue
		}
		if c.Kind == KindLinkToObject {
			writeObjectLink(w, c, voltageColor[c.Voltage], code, mode)
			continue
		}
		dataAttrs := ""
		if hasCode {
			dataAttrs = fmt.Sprintf(" data-type=\"%s\"", esc(code))
		}
		writePolyline(w, c.ID, "connector", c.Points, voltageColor[c.Voltage], c.Dashed, 1, dataAttrs, mode)
	}

	tiers := make([]int, 0, len(elevated))
	for z := range elevated {
		tiers = append(tiers, z)
	}
	sort.Ints(tiers)
	for _, z := range tiers {
		var lastTierShape string
		for _, e := range elevated[z] {
			renderElement(w, lib, voltageColor, stateColors, fpiColors, e, &missing, seenMissing, &lastTierShape, mode)
		}
	}

	for _, l := range d.Labels {
		writeLabel(w, l, mode)
	}

	for _, dd := range d.DigitalDevices {
		writeDigitalDevice(w, dd, mode)
	}

	fmt.Fprint(w, "</svg>\n")

	if len(missing) > 0 {
		return fmt.Errorf("slddoc: symbol library missing shape(s): %s", strings.Join(missing, ", "))
	}
	return nil
}

// writePolyline draws a busbar's or connector's geometry as a single flat
// <polyline>, matching a real xsde2svg-exported busbar/wire exactly — no
// synthetic wrapping <g> and no duplicate hit-target line. dataAttrs, when
// non-empty, is inserted verbatim (its own leading space included) before
// id — e.g. a busbar's data-name/data-voltage/data-type, mirroring what a
// real xsde2svg busbar polyline carries. id, when non-zero (0 is never a
// real assigned id — see Diagram.LastID's doc comment), is always written,
// in both RenderModes, since a real xsde2svg polyline carries one too;
// data-editor-kind (this editor's own addition, "element" for a busbar
// since it's an Element, "connector" for a wire — not part of that format)
// is added only in Interactive mode, for the frontend to hit-test a click
// against.
func writePolyline(w io.Writer, id int, kind string, pts []Point, color string, dashed bool, strokeWidth float64, dataAttrs string, mode RenderMode) {
	if color == "" {
		color = "black"
	}
	var sb strings.Builder
	for i, p := range pts {
		if i > 0 {
			sb.WriteByte(' ')
		}
		sb.WriteString(fmtNum(p.X))
		sb.WriteByte(',')
		sb.WriteString(fmtNum(p.Y))
	}
	dash := ""
	if dashed {
		dash = "stroke-dasharray: 14,9;"
	}
	points := esc(sb.String())
	width := fmtNum(strokeWidth)
	idAttrs := ""
	if id != 0 {
		if mode == Interactive {
			idAttrs = fmt.Sprintf(" id=\"%d\" data-editor-kind=\"%s\"", id, kind)
		} else {
			idAttrs = fmt.Sprintf(" id=\"%d\"", id)
		}
	}
	fmt.Fprintf(w, "<polyline points=\"%s\" style=\"fill:none;stroke:%s;%sstroke-width:%s\"%s%s />\n",
		points, esc(color), dash, width, dataAttrs, idAttrs)
}

// writeRectangle draws a Rectangle (shape 3) as a single flat <rect>, the
// same "no wrapping <g>" convention writePolyline uses for a busbar —
// matching the real xsde2svg source (internal/modus/element_3.go), which
// likewise emits a bare <rect> rather than a <g>-wrapped symbol. Its own
// two Points (any order — Render doesn't require a particular corner
// first) are normalized into a proper top-left x/y plus a positive
// width/height the same way that source's own rectX/rectY/w/h computation
// does. Fill/Stroke fall back to "none"/"white" when unset, matching a
// Lamp's own unset-color convention (see swatchColor, frontend
// PropertiesPanel.tsx) rather than resolving a VoltageClass color the way
// every real equipment shape's own {color} does — a decorative annotation
// box has no electrical voltage to resolve one from. StrokeWidth <= 0
// (unset) falls back to 1, the fixed value every other shape's own
// template hardcodes. data-voltage mirrors
// the resolved Stroke, the same "not a VoltageClass id, just the color
// actually drawn" convention a busbar's own data-voltage already uses. A
// Rectangle with fewer than 2 Points (never emitted by this editor itself,
// but a hand-edited or corrupt file could carry one) draws nothing rather
// than guessing a size.
func writeRectangle(w io.Writer, e Element, mode RenderMode) {
	if len(e.Points) < 2 {
		return
	}
	p0, p1 := e.Points[0], e.Points[1]
	x, y := math.Min(p0.X, p1.X), math.Min(p0.Y, p1.Y)
	width, height := math.Abs(p1.X-p0.X), math.Abs(p1.Y-p0.Y)

	fill := e.Fill
	if fill == "" {
		fill = "none"
	}
	stroke := e.Stroke
	if stroke == "" {
		stroke = "white"
	}
	strokeWidth := e.StrokeWidth
	if strokeWidth <= 0 {
		strokeWidth = 1
	}

	editorAttr := ""
	if mode == Interactive {
		editorAttr = " data-editor-kind=\"element\""
	}
	fmt.Fprintf(w, "<rect id=\"%d\" x=\"%s\" y=\"%s\" width=\"%s\" height=\"%s\" style=\"fill:%s;stroke:%s;stroke-width:%s\" data-name=\"%s\" data-voltage=\"%s\" data-type=\"3\"%s />\n",
		e.ID, fmtNum(x), fmtNum(y), fmtNum(width), fmtNum(height), esc(fill), esc(stroke), fmtNum(strokeWidth), esc(e.Name), esc(stroke), editorAttr)
}

// writeCircle draws a Circle (shape 4) as a single flat <ellipse>, the
// same "no wrapping <g>" convention writeRectangle uses — matching the
// real xsde2svg source (internal/modus/element_4.go: canvas.Ellipse), which
// likewise emits a bare <ellipse> rather than a <g>-wrapped symbol. Its own
// two Points (any order, same as writeRectangle's own) are normalized into
// a center (their own midpoint) plus a positive rx/ry, the same way that
// source's own x0/y0/w/h computation does. Fill/Stroke/StrokeWidth fall
// back exactly the same way writeRectangle's own do — this shape shares
// that one's entire color/width model, just rendered as an ellipse instead
// of a rect. A Circle with fewer than 2 Points draws nothing, same as
// writeRectangle.
func writeCircle(w io.Writer, e Element, mode RenderMode) {
	if len(e.Points) < 2 {
		return
	}
	p0, p1 := e.Points[0], e.Points[1]
	cx, cy := (p0.X+p1.X)/2, (p0.Y+p1.Y)/2
	rx, ry := math.Abs(p1.X-p0.X)/2, math.Abs(p1.Y-p0.Y)/2

	fill := e.Fill
	if fill == "" {
		fill = "none"
	}
	stroke := e.Stroke
	if stroke == "" {
		stroke = "white"
	}
	strokeWidth := e.StrokeWidth
	if strokeWidth <= 0 {
		strokeWidth = 1
	}

	editorAttr := ""
	if mode == Interactive {
		editorAttr = " data-editor-kind=\"element\""
	}
	fmt.Fprintf(w, "<ellipse id=\"%d\" cx=\"%s\" cy=\"%s\" rx=\"%s\" ry=\"%s\" style=\"fill:%s;stroke:%s;stroke-width:%s\" data-name=\"%s\" data-voltage=\"%s\" data-type=\"4\"%s />\n",
		e.ID, fmtNum(cx), fmtNum(cy), fmtNum(rx), fmtNum(ry), esc(fill), esc(stroke), fmtNum(strokeWidth), esc(e.Name), esc(stroke), editorAttr)
}

// arrowChevron is the open two-stroke chevron writeArrow draws at either
// end of its own line — the real xsde2svg source's own arrowhead shape
// (internal/modus/element_2.go), reproduced here as a single local-frame
// formula rather than that source's own four axis-aligned special cases
// plus one generic/rotated one: a horizontal chevron rotated 0°/180° by
// writeArrow's own wrapping transform is pixel-identical to what those
// special cases compute directly, so this package doesn't bother
// special-casing them. tipAtOrigin true draws the chevron with its own
// tip at local (0,0) pointing toward +x (used at the line's end, local
// x=length, where the line arrives *from* -x); false draws it pointing
// toward -x instead (used at the line's start, local x=0, pointing away
// from where the line leaves *toward* +x) — the real source's own
// end/doubEnd formulas, respectively. l3/l7 are fixed at the real
// source's own default values (3, 7) rather than scaled by StrokeWidth —
// real xsde2svg does technically scale them, but via reusing its own
// Scale(scaleChosed, n) helper with StrokeWidth-1 passed in the position
// that helper elsewhere expects a small fixed output-resolution preset
// index (0/1/2), not a real multiplier — not a relationship worth
// reproducing for this schema's own free-form StrokeWidth.
func arrowChevron(tipAtOrigin bool) string {
	const l3, l7 = 3.0, 7.0
	firstX := -l7
	if !tipAtOrigin {
		firstX = l7
	}
	return fmt.Sprintf(" l %s -%s m 0 %s l %s -%s", fmtNum(firstX), fmtNum(l3), fmtNum(l3*2), fmtNum(-firstX), fmtNum(l3))
}

// writeArrow draws an Arrow (shape 2) as a single flat <path>, the same
// "no wrapping <g>" convention writeRectangle/writePolyline use — a line
// from local (0,0) to (length,0), with arrowChevron's own open chevron at
// the end (and, when DoubleHeaded, a mirrored one at the start too),
// wrapped in transform="translate(x0,y0) rotate(angle)" so the whole thing
// points the right way — matching every ordinary template-drawn shape's
// own transform convention (Render's own translate(x,y) rotate(orient)),
// unlike writeRectangle/writePolyline which draw directly in absolute
// diagram coordinates since neither of those has a meaningful "local
// frame" distinct from the diagram's own. Stroke/StrokeWidth fall back the
// same way writeRectangle's own do; there's no Fill (an Arrow has no
// interior, always fill:none). An Arrow with fewer than 2 Points, or
// whose two Points coincide (zero length — nothing to point), draws
// nothing.
func writeArrow(w io.Writer, e Element, mode RenderMode) {
	if len(e.Points) < 2 {
		return
	}
	p0, p1 := e.Points[0], e.Points[1]
	dx, dy := p1.X-p0.X, p1.Y-p0.Y
	length := math.Hypot(dx, dy)
	if length == 0 {
		return
	}
	angle := math.Atan2(dy, dx) * 180 / math.Pi

	stroke := e.Stroke
	if stroke == "" {
		stroke = "white"
	}
	strokeWidth := e.StrokeWidth
	if strokeWidth <= 0 {
		strokeWidth = 1
	}

	d := "M 0 0"
	if e.DoubleHeaded {
		d += arrowChevron(false)
	}
	d += fmt.Sprintf(" h %s", fmtNum(length)) + arrowChevron(true)

	editorAttr := ""
	if mode == Interactive {
		editorAttr = " data-editor-kind=\"element\""
	}
	fmt.Fprintf(w, "<path id=\"%d\" d=\"%s\" style=\"fill:none;stroke:%s;stroke-width:%s\" data-name=\"%s\" data-voltage=\"%s\" data-type=\"2\" transform=\"translate(%s,%s) rotate(%s)\"%s />\n",
		e.ID, d, esc(stroke), fmtNum(strokeWidth), esc(e.Name), esc(stroke), fmtNum(p0.X), fmtNum(p0.Y), fmtNum(angle), editorAttr)
}

// writeNamedLine draws a KindOverheadLine/KindCableLine connector as a
// real xsde2svg-style <g id data-type data-name data-voltage> wrapping
// its own polyline — unlike every other connector kind (and a busbar),
// which writePolyline renders as a single flat polyline with no wrapping
// <g>, and no data-name, at all. Overhead line confirmed against both a
// real corpus file (sld-viewer/assets/sld/IEEE9bus.svg, id="302"
// data-name="Line2") and a user-supplied example matching it exactly;
// cable line given the same <g>-wrapping treatment for consistency
// between the two "named" kinds, since both are meant to carry a real
// identity (Connector.Name) a plain wire never does — though its own
// stroke defaults to dashed rather than solid (see
// resolveCableLineDash/Connector.LineStyle). code is
// connectorTypeCode[c.Kind], passed in rather than looked up again since
// the caller already has it from its own typeComment call.
func writeNamedLine(w io.Writer, c Connector, color string, code string, mode RenderMode) {
	if color == "" {
		color = "black"
	}
	var sb strings.Builder
	for i, p := range c.Points {
		if i > 0 {
			sb.WriteByte(' ')
		}
		sb.WriteString(fmtNum(p.X))
		sb.WriteByte(',')
		sb.WriteString(fmtNum(p.Y))
	}
	dash := ""
	if c.Kind == KindCableLine {
		dash = resolveCableLineDash(c.LineStyle)
	} else if c.Dashed {
		dash = "stroke-dasharray: 14,9;"
	}
	editorAttr := ""
	if mode == Interactive {
		editorAttr = " data-editor-kind=\"connector\""
	}
	fmt.Fprintf(w, "<g id=\"%d\" data-type=\"%s\" data-name=\"%s\" data-voltage=\"%s\"%s>\n<polyline points=\"%s\" style=\"fill:none;stroke:%s;%sstroke-width:%s\" />\n</g>\n",
		c.ID, esc(code), esc(c.Name), esc(color), editorAttr, esc(sb.String()), esc(color), dash, fmtNum(namedLineStrokeWidth))
}

// objectLinkStrokeWidth is a KindLinkToObject connector's own fixed stroke
// width — heavier than an ordinary wire's 1px, confirmed against a real
// xsde2svg-exported instance exactly.
const objectLinkStrokeWidth = 2

// writeObjectLink draws a KindLinkToObject connector (shape 28,
// "Связь с объектом"/"Object link" in the xsde2svg catalog) — the same
// flat, un-wrapped <polyline> convention as KindBusWork (writePolyline),
// just at objectLinkStrokeWidth instead of 1px, plus a triangular
// arrowhead marking direction: a separate sibling <path>, carrying no
// data-type/data-name/id of its own, positioned at the connector's own
// final point and rotated to continue pointing in the direction its last
// segment was already travelling. The arrowhead's own local geometry (base
// corners at local (±7,0), apex at local (0,12), i.e. drawn "pointing
// south" before rotation) is reverse-engineered from a real instance, but
// — unlike that real instance's own single rotate(angle,cx,cy) around an
// absolute-coordinate path — placed with the same translate-then-rotate
// convention renderElement already uses for every symbol template
// (transform="translate(x,y) rotate(angle)", local-origin path data): a
// bare rotate() around an arbitrary point is *not* equivalent to that when
// the path's own coordinates are local rather than pre-rotation-absolute
// (confirmed the hard way — an earlier version of this function paired
// local coordinates with a bare rotate(angle,cx,cy) and silently rendered
// the arrowhead thousands of units away from its own connector). The
// rotate() angle formula (atan2(-dx,dy), which reproduces a real
// instance's own effective 180° for a straight-up final segment) was also
// reverse-engineered from that same real instance.
func writeObjectLink(w io.Writer, c Connector, color, code string, mode RenderMode) {
	dataAttrs := ""
	if code != "" {
		dataAttrs = fmt.Sprintf(" data-type=\"%s\"", esc(code))
	}
	writePolyline(w, c.ID, "connector", c.Points, color, c.Dashed, objectLinkStrokeWidth, dataAttrs, mode)

	if len(c.Points) < 2 {
		return
	}
	from := c.Points[len(c.Points)-2]
	to := c.Points[len(c.Points)-1]
	dy := to.Y - from.Y
	if from.X == to.X && from.Y == to.Y {
		return
	}
	// from.X-to.X, not -(to.X-from.X): IEEE754 gives a-a exactly +0 for any
	// finite a, whereas negating a +0 difference yields -0, which flips
	// atan2's own result by a full turn (180 here becomes -180) — cosmetic
	// only (they're the same rotation), but real corpus instances always
	// read "180", not "-180", for a straight-up final segment.
	angle := math.Atan2(from.X-to.X, dy) * 180 / math.Pi
	arrowColor := color
	if arrowColor == "" {
		arrowColor = "black"
	}
	fmt.Fprintf(w, "<path d=\"M 7 0 l -7 12 l -7 -12 z\" style=\"fill:none;stroke:%s;stroke-width:%s\" transform=\"translate(%s,%s) rotate(%s)\" />\n",
		esc(arrowColor), fmtNum(objectLinkStrokeWidth), fmtNum(to.X), fmtNum(to.Y), fmtNum(angle))
}

// Power transformer (shape 47) geometry constants, transcribed from real
// xsde2svg's own element_47.go default (sde.Size==0) path — this editor
// doesn't offer that source's own Size 11-24 magic lookup table (a set of
// named presets overriding these same numbers for cosmetic leg-length
// tuning, not exposed anywhere in this editor's own Properties), so every
// PowerTransformer always uses the one true default set below.
const (
	transformerRadius     = 22                    // baseRadius
	transformerXShift     = 18                    // int(baseRadius/1.2) — 2-winding and 3-winding side-circle offset
	transformerTopShift   = 25                    // int(baseRadius*1.16) — 3-winding top-circle offset (real source: -25.52 truncated toward zero)
	transformerSideShift  = 29                    // 4-winding left/right-circle offset
	transformerVertShift  = 20                    // 4-winding top-circle offset (bottom reuses transformerXShift, matching real source exactly)
	transformerGlyphShift = transformerRadius / 3 // wye/wyeN spoke length (int division, matching element_47.go exactly)
	transformerArrowSize  = 20
	transformerArrowShift = 5
)

// transformerLegLength returns winding index i's own leg length, for a
// transformer with the given real winding count — chosen so that, for
// that winding's own *default* TerminalDirection (defaultTerminal), the
// lead tip's own distance from the anchor (transformerWindingOffset ±
// transformerRadius ± this) is always an exact multiple of
// diagramOps.ts's own default 10-unit grid, the same way every other
// shape's own fixed-offset terminal already does — without this, a
// freshly placed transformer's own anchor lands on-grid (the editor's own
// generic click-to-place snap) but its own lead tip never did, forcing a
// short non-orthogonal jog into an otherwise-orthogonal routed wire (see
// this package's own RELEASE.md for the real screenshot that surfaced
// this). Real xsde2svg itself varies its own leg length by
// direction/WindingNo too (h=11, h+lenShift=13, hLegs=10, hLegsTop=13) —
// unlike that convention, which exists for its own cosmetic reasons, this
// one is driven purely by which of the fixed offset constants above
// (transformerXShift/TopShift/SideShift/VertShift) that winding's own
// circle uses, each paired with whichever length makes
// offset+transformerRadius+length divisible by 10. A winding whose own
// Terminal is overridden away from its own default direction can still
// land off-grid on the axis perpendicular to its own chosen direction
// (that axis inherits the winding's own raw, non-grid-multiple offset
// instead) — a real but narrower residual gap than the one this fixes.
func transformerLegLength(count, i int) float64 {
	switch count {
	case 2:
		return 10 // transformerXShift(18)+radius(22)+10 = 50
	case 3:
		if i == 0 {
			return 13 // transformerTopShift(25)+radius(22)+13 = 60
		}
		return 10 // transformerXShift(18)+radius(22)+10 = 50
	case 4:
		switch i {
		case 0:
			return 8 // transformerVertShift(20)+radius(22)+8 = 50
		case 1:
			return 10 // transformerXShift(18)+radius(22)+10 = 50 (winding 1 reuses transformerXShift — see transformerWindingOffset)
		default:
			return 9 // transformerSideShift(29)+radius(22)+9 = 60
		}
	}
	return 10
}

// transformerWindingOffset returns real winding index i's own local
// (pre-rotation) circle-center offset from the transformer's own anchor,
// for a non-autotransformer with the given real winding count —
// transcribed from element_47.go's own default (non-auto) case, confirmed
// against real xsde2svg-exported corpus markup (both the default-scale
// 3-winding case and a fractional-scale 4-winding one, whose proportions
// matched this function's own constants exactly once the scale factor was
// divided back out).
func transformerWindingOffset(count, i int) (float64, float64) {
	switch count {
	case 2:
		if i == 0 {
			return transformerXShift, 0
		}
		return -transformerXShift, 0
	case 3:
		switch i {
		case 0:
			return 0, -transformerTopShift
		case 1:
			return transformerXShift, 0
		default:
			return -transformerXShift, 0
		}
	case 4:
		switch i {
		case 0:
			return 0, -transformerVertShift
		case 1:
			return 0, transformerXShift
		case 2:
			return -transformerSideShift, 0
		default:
			return transformerSideShift, 0
		}
	}
	return 0, 0
}

// defaultTerminal returns real winding index i's own conventional lead
// direction for a transformer with the given real winding count, used
// when that winding's own Terminal field is empty — matches
// element_47.go's own default (Chassis-unset) leg direction for each
// position exactly (a 2-winding transformer's default side-by-side
// horizontal layout, a 3-winding one's top/right/left, ...).
func defaultTerminal(count, i int) TerminalDirection {
	switch count {
	case 2:
		if i == 0 {
			return TerminalRight
		}
		return TerminalLeft
	case 3:
		switch i {
		case 0:
			return TerminalTop
		case 1:
			return TerminalRight
		default:
			return TerminalLeft
		}
	case 4:
		switch i {
		case 0:
			return TerminalTop
		case 1:
			return TerminalBottom
		case 2:
			return TerminalLeft
		default:
			return TerminalRight
		}
	}
	return TerminalRight
}

// transformerLegEndpoint returns a winding's own lead tip — its real
// electrical terminal, matching parsePowerTransformer's own "final point
// of a two-point lead path" extraction convention — given its own circle
// center, lead direction, and own leg length (transformerLegLength).
func transformerLegEndpoint(cx, cy float64, dir TerminalDirection, legLen float64) (float64, float64) {
	switch dir {
	case TerminalTop:
		return cx, cy - transformerRadius - legLen
	case TerminalBottom:
		return cx, cy + transformerRadius + legLen
	case TerminalLeft:
		return cx - transformerRadius - legLen, cy
	default: // TerminalRight
		return cx + transformerRadius + legLen, cy
	}
}

// writeTransformerLeg draws a winding's own straight lead from its
// circle's own edge to its lead tip (transformerLegEndpoint).
func writeTransformerLeg(w io.Writer, cx, cy float64, dir TerminalDirection, legLen float64, color string) {
	ex, ey := transformerLegEndpoint(cx, cy, dir, legLen)
	var sx, sy float64
	switch dir {
	case TerminalTop:
		sx, sy = cx, cy-transformerRadius
	case TerminalBottom:
		sx, sy = cx, cy+transformerRadius
	case TerminalLeft:
		sx, sy = cx-transformerRadius, cy
	default:
		sx, sy = cx+transformerRadius, cy
	}
	fmt.Fprintf(w, "<path d=\"M %s %s L %s %s\" style=\"fill:none;stroke:%s;stroke-width:2\" data-voltage=\"%s\" />\n",
		fmtNum(sx), fmtNum(sy), fmtNum(ex), fmtNum(ey), esc(color), esc(color))
}

// writeWindingGlyph draws a winding's own connection-scheme mark at its
// own circle center — the small inner symbol distinguishing
// wye/wye-with-neutral/delta, transcribed from element_47.go's own
// per-WindingType path formulas (confirmed against real corpus markup for
// all three: a plain "wye" 3-spoke mark, a "ЗВЕЗДА_С_НУЛЕМ" 4-line
// wye-with-neutral mark, and a closed-triangle "delta" mark using its own
// separate shift constant, baseRadius/2 rather than /3).
func writeWindingGlyph(w io.Writer, cx, cy float64, scheme WindingScheme, color string) {
	if scheme == "" {
		return
	}
	style := fmt.Sprintf("fill:none;stroke:%s;stroke-width:1", esc(color))
	shift := float64(transformerGlyphShift)
	switch scheme {
	case SchemeWye:
		fmt.Fprintf(w, "<path d=\"M %s %s l %s %s M %s %s l %s %s M %s %s l %s %s\" style=\"%s\" />\n",
			fmtNum(cx), fmtNum(cy), fmtNum(-shift), fmtNum(-shift),
			fmtNum(cx), fmtNum(cy), fmtNum(shift), fmtNum(-shift),
			fmtNum(cx), fmtNum(cy), fmtNum(0), fmtNum(shift),
			style)
	case SchemeWyeN:
		fmt.Fprintf(w, "<path d=\"M %s %s l %s %s M %s %s l %s %s M %s %s l %s %s M %s %s l %s %s\" style=\"%s\" />\n",
			fmtNum(cx), fmtNum(cy), fmtNum(-shift), fmtNum(-shift),
			fmtNum(cx), fmtNum(cy), fmtNum(shift), fmtNum(-shift),
			fmtNum(cx), fmtNum(cy), fmtNum(0), fmtNum(shift),
			fmtNum(cx), fmtNum(cy), fmtNum(shift), fmtNum(0),
			style)
	case SchemeDelta:
		deltaShift := float64(transformerRadius / 2)
		half := float64(int(transformerRadius/2) / 2)
		fmt.Fprintf(w, "<path d=\"M %s %s l %s %s l %s %s z\" style=\"%s\" />\n",
			fmtNum(cx), fmtNum(cy-half),
			fmtNum(half), fmtNum(deltaShift),
			fmtNum(-deltaShift), fmtNum(0),
			style)
	}
}

// writeGroundingMark draws a small, distinguishing mark for a wye-with-
// neutral winding's own NeutralGrounding, just past the wyeN glyph's own
// 4th (neutral) spoke tip — real xsde2svg only ever draws a distinct glyph
// for GroundingSolid (its own "neutral_ground" WindingType, extra
// ground-hatch marks); GroundingIsolated/GroundingResistor get their own
// small invented marks here (a ring, and a resistor zigzag), matching
// neither real xsde2svg output, per this editor's own explicit design
// choice to keep all three grounding states visually distinct instead.
func writeGroundingMark(w io.Writer, cx, cy float64, grounding NeutralGrounding, color string) {
	if grounding == "" {
		return
	}
	style := fmt.Sprintf("fill:none;stroke:%s;stroke-width:1", esc(color))
	nx := cx + transformerGlyphShift + 3 // just past the wyeN glyph's own horizontal spoke tip
	switch grounding {
	case GroundingSolid:
		// A standard earth-ground pictogram: a short stem plus three
		// horizontal bars of decreasing width.
		fmt.Fprintf(w, "<path d=\"M %s %s v 4 M %s %s h 10 M %s %s h 6 M %s %s h 2\" style=\"%s\" />\n",
			fmtNum(nx), fmtNum(cy),
			fmtNum(nx-5), fmtNum(cy+4),
			fmtNum(nx-3), fmtNum(cy+7),
			fmtNum(nx-1), fmtNum(cy+10),
			style)
	case GroundingIsolated:
		fmt.Fprintf(w, "<circle cx=\"%s\" cy=\"%s\" r=\"3\" style=\"%s\" />\n", fmtNum(nx+3), fmtNum(cy), style)
	case GroundingResistor:
		fmt.Fprintf(w, "<path d=\"M %s %s l 2 -3 l 2 3 l 2 -3 l 2 3 l 2 -3\" style=\"%s\" />\n", fmtNum(nx), fmtNum(cy), style)
	}
}

// transformerTapOffset is the local (pre-rotation, anchor-relative)
// position of an autotransformer's own extra "line" terminal — the tap
// arc's own far tip, a real electrical connection in its own right,
// distinct from every winding's own regular lead. Real xsde2svg source
// models it as its own TransformerWinding entry with no circle of its own
// (a 2-real-winding autotransformer's own source data carries 3 such
// entries — the first, never drawn as a circle, only supplies the tap's
// own color and this terminal's own position); this package draws it as
// a decoration connected to Windings[0]'s own real circle instead (see
// writeAutotransformerTap's own doc comment for why), but the terminal
// itself is real: TransformerLocalTerminals (diagramOps.ts) exposes this
// same point so a wire can actually connect there, matching a real
// instance's own external HV lead. Always directly above the anchor
// (local x=0), regardless of winding count or Windings[0]'s own dX, so it
// lands on the 10-unit grid by construction the same way every other
// fixed-offset terminal in this package does — unlike Windings[0]'s own
// circle position, which the tap's real xsde2svg counterpart is *not*
// anchored to (confirmed by reading element_47.go's own isAutoTrans i==0
// branch: its own dX/dY default to 0, i.e. the transformer's own overall
// anchor, not whatever dX a later real circle ends up at).
const transformerTapOffsetY = -50

// writeAutotransformerTap draws an autotransformer's own extra terminal
// (transformerTapOffset) as a short stub, plus a curved arc sweeping down
// to Windings[0]'s own real circle — connecting them visually, the same
// way a real instance's own tap arc visually leads into its first real
// winding. Drawn at the same stroke-width every regular winding lead
// uses (writeTransformerLeg), not a thinner one — this is a real
// electrical lead, not a decoration. Returns the terminal's own local
// position for the caller to treat as a real port the same way every
// winding's own lead already is.
func writeAutotransformerTap(w io.Writer, cx, cy float64, color string) (float64, float64) {
	tapX, tapY := 0.0, float64(transformerTapOffsetY)
	stubEndY := tapY + 20 // a short stub, leaving room for the arc below it
	style := fmt.Sprintf("fill:none;stroke:%s;stroke-width:2", esc(color))
	fmt.Fprintf(w, "<path d=\"M %s %s L %s %s\" style=\"%s\" />\n", fmtNum(tapX), fmtNum(tapY), fmtNum(tapX), fmtNum(stubEndY), style)

	// The arc's own landing point sits on the circle's own rim, offset
	// toward whichever side the circle itself sits on (not straight up
	// from its center) — a broad, generously-radiused sweep past the
	// circle's own top rather than a tight loop directly into it, closer
	// to how a real instance's own tap arc actually reads.
	sign := 1.0
	if cx < tapX {
		sign = -1.0
	}
	const landingAngle = 50 * math.Pi / 180
	targetX := math.Round(cx + sign*transformerRadius*math.Sin(landingAngle))
	targetY := math.Round(cy - transformerRadius*math.Cos(landingAngle))
	const arcRadius = 40
	sweep := 1
	if sign < 0 {
		sweep = 0
	}
	fmt.Fprintf(w, "<path d=\"M %s %s A %d %d 0 0 %d %s %s\" style=\"%s\" />\n",
		fmtNum(tapX), fmtNum(stubEndY), arcRadius, arcRadius, sweep, fmtNum(targetX), fmtNum(targetY), style)

	return tapX, tapY
}

// writePowerTransformer draws a PowerTransformer element (shape 47): one
// circle per winding (e.Windings, in order), each with its own lead,
// connection-scheme glyph, and (wye-with-neutral only) grounding mark; one
// diagonal regulation arrow, centered on the element's own anchor, for
// whichever winding has TapChanger set (matching real xsde2svg's own
// "last one wins" behavior when more than one winding requests it — see
// TransformerWinding.TapChanger's own doc comment); and, when
// VectorGroupLabel is set, that label as plain text below the symbol.
// Unlike real xsde2svg's own bare rotate(angle,cx,cy) transform, the whole
// symbol is drawn in local (pre-rotation) coordinates and wrapped in
// translate(x,y) rotate(orient) — the same convention renderElement's own
// template branch already uses for every other shape — which is also why
// the regulation arrow rotates along with the rest of the symbol here,
// unlike real xsde2svg output (there it's drawn with a literal no-op
// translate(0,0), so it never actually rotates with its own transformer;
// this editor's own version rotating together is more useful to a diagram
// author and was a deliberate deviation, not an oversight).
func writePowerTransformer(w io.Writer, e Element, voltageColor map[int]string, fallbackColor string, mode RenderMode) {
	count := len(e.Windings)
	if count < 2 {
		count = 2
	}

	editorAttr := ""
	if mode == Interactive {
		editorAttr = " data-editor-kind=\"element\""
	}
	fmt.Fprintf(w, "<g id=\"%d\" data-name=\"%s\" data-voltage=\"%s\" data-type=\"47\"%s transform=\"translate(%s,%s) rotate(%d)%s\">\n",
		e.ID, esc(e.Name), esc(fallbackColor), editorAttr, fmtNum(e.X), fmtNum(e.Y), e.Orient, mirrorScale(e.Mirror))

	for i := 0; i < count; i++ {
		var winding TransformerWinding
		if i < len(e.Windings) {
			winding = e.Windings[i]
		}
		color := voltageColor[winding.Voltage]
		if color == "" {
			color = fallbackColor
		}
		cx, cy := transformerWindingOffset(count, i)
		dir := winding.Terminal
		if dir == "" {
			dir = defaultTerminal(count, i)
		}

		fmt.Fprintf(w, "<circle cx=\"%s\" cy=\"%s\" r=\"%d\" style=\"fill:none;stroke:%s;stroke-width:2\" data-voltage=\"%s\" />\n",
			fmtNum(cx), fmtNum(cy), transformerRadius, esc(color), esc(color))
		writeTransformerLeg(w, cx, cy, dir, transformerLegLength(count, i), color)
		// The connection-scheme glyph (and its own grounding mark) is
		// wrapped in a counter-rotating <g> — rotate(-Orient) around the
		// winding's own circle center — so it stays upright on screen
		// regardless of the transformer's own Orient, the same way
		// {counterRotate} already keeps a FaultPassageIndicator's own "FPI"
		// label upright inside its own base.xml template: rotating a point
		// around (cx,cy) by -Orient first, then the outer <g>'s own
		// translate(x,y) rotate(Orient) rotates it right back by +Orient,
		// netting zero rotation for anything drawn at an offset from
		// (cx,cy) — while (cx,cy) itself, being the pivot, is untouched by
		// its own rotation and still moves with the winding exactly as
		// before.
		if e.Orient != 0 {
			fmt.Fprintf(w, "<g transform=\"rotate(%d,%s,%s)\">\n", -e.Orient, fmtNum(cx), fmtNum(cy))
		}
		writeWindingGlyph(w, cx, cy, winding.Scheme, color)
		if winding.Scheme == SchemeWyeN {
			writeGroundingMark(w, cx, cy, winding.Grounding, color)
		}
		if e.Orient != 0 {
			fmt.Fprint(w, "</g>\n")
		}
		if e.Autotransformer && i == 0 {
			_, _ = writeAutotransformerTap(w, cx, cy, color)
		}
	}

	regulatedColor := ""
	for i := 0; i < count && i < len(e.Windings); i++ {
		if e.Windings[i].TapChanger {
			regulatedColor = voltageColor[e.Windings[i].Voltage]
			if regulatedColor == "" {
				regulatedColor = fallbackColor
			}
		}
	}
	if regulatedColor != "" {
		style := fmt.Sprintf("fill:none;stroke:%s;stroke-width:1", esc(regulatedColor))
		endStyle := fmt.Sprintf("fill:%s;stroke:%s;stroke-width:1", esc(regulatedColor), esc(regulatedColor))
		x0, y0 := -transformerArrowSize-transformerArrowShift, transformerArrowSize+transformerArrowShift
		x1, y1 := transformerArrowSize+transformerArrowShift, -(transformerArrowSize + transformerArrowShift)
		fmt.Fprintf(w, "<path d=\"M %s %s L %s %s\" style=\"%s\" />\n", fmtNum(float64(x0)), fmtNum(float64(y0)), fmtNum(float64(x1)), fmtNum(float64(y1)), style)
		fmt.Fprintf(w, "<path d=\"M %s %s l -5 -5 l 7 -2 z\" style=\"%s\" />\n", fmtNum(float64(x1)+3), fmtNum(float64(y1)+2), endStyle)
	}

	if e.VectorGroupLabel != "" {
		// A fixed cosmetic offset below the symbol's own lowest possible
		// extent (transformerRadius, plus the largest transformerLegLength
		// value any winding count/position ever returns, plus a small
		// margin) — this text's own position isn't part of the electrical
		// geometry, so it doesn't need to be grid-aligned the way a lead
		// tip does.
		const vectorGroupLabelOffset = transformerRadius + 13 + 14
		fmt.Fprintf(w, "<text x=\"0\" y=\"%d\" style=\"fill:%s;font-size:10px;font-family:Arial\" text-anchor=\"middle\">%s</text>\n",
			vectorGroupLabelOffset, esc(fallbackColor), esc(e.VectorGroupLabel))
	}

	fmt.Fprint(w, "</g>\n")
}

func writeLabel(w io.Writer, l Label, mode RenderMode) {
	anchor := l.Anchor
	if anchor == "" {
		anchor = "start"
	}
	weight := ""
	if l.Bold {
		weight = "font-weight: bold;"
	}
	color := l.Color
	if color == "" {
		color = "white"
	}
	font := l.Font
	if font == "" {
		font = "Arial"
	}
	// dominant-baseline vertical-aligns the text block's own first line
	// against Y — "top"/"middle" get a real value, the original
	// baseline-at-Y behavior (VAlign empty, i.e. "bottom") gets none at
	// all, so an already-saved label with no valign attribute keeps
	// rendering exactly as before.
	baseline := ""
	switch l.VAlign {
	case "top":
		baseline = "dominant-baseline:hanging;"
	case "middle":
		baseline = "dominant-baseline:middle;"
	}
	style := fmt.Sprintf("fill:%s;text-anchor:%s;%sfont-size:%spx;font-family:%s;%swhite-space: pre;",
		color, anchor, baseline, fmtNum(l.Size), font, weight)
	editorAttr := ""
	if mode == Interactive {
		editorAttr = " data-editor-kind=\"label\""
	}

	lines := strings.Split(l.Text, "\n")
	fmt.Fprintf(w, "<text id=\"%d\" x=\"%s\" y=\"%s\" style=\"%s\"%s>%s", l.ID, fmtNum(l.X), fmtNum(l.Y), esc(style), editorAttr, esc(lines[0]))
	for _, ln := range lines[1:] {
		fmt.Fprintf(w, "<tspan x=\"%s\" dy=\"%s\" style=\"%s\">%s</tspan>",
			fmtNum(l.X), fmtNum(l.Size*1.4), esc(style), esc(ln))
	}
	fmt.Fprint(w, "</text>\n")
}

// digitalDeviceTypeCode is shape 134's own xsde2svg catalog code, written as
// data-type the same way an Element/Connector's own shape/kind code is.
const digitalDeviceTypeCode = "134"

// writeDigitalDevice draws a shape-134 SCADA readout as a bare <text> node —
// no wrapping <g>/transform, matching a real xsde2svg-exported one exactly
// (see DigitalDevice's own doc comment). Unlike writeLabel's tspans (each a
// new stacked line via dy), Unit's own tspan shares Value's line: same style,
// no dy, immediately after Value with a single separating space.
func writeDigitalDevice(w io.Writer, dd DigitalDevice, mode RenderMode) {
	anchor := dd.Anchor
	if anchor == "" {
		anchor = "start"
	}
	weight := ""
	if dd.Bold {
		weight = "font-weight: bold;"
	}
	color := dd.Color
	if color == "" {
		color = "white"
	}
	font := dd.Font
	if font == "" {
		font = "Arial"
	}
	baseline := ""
	switch dd.VAlign {
	case "top":
		baseline = "dominant-baseline:hanging;"
	case "middle":
		baseline = "dominant-baseline:middle;"
	}
	style := fmt.Sprintf("fill:%s;text-anchor:%s;%sfont-size:%spx;font-family:%s;%s",
		color, anchor, baseline, fmtNum(dd.Size), font, weight)
	editorAttr := ""
	if mode == Interactive {
		editorAttr = " data-editor-kind=\"digitaldevice\""
	}
	unitAttr := ""
	if dd.Unit != "" {
		unitAttr = fmt.Sprintf(" data-unit=\"%s\"", esc(dd.Unit))
	}
	fmt.Fprintf(w, "<text data-type=\"%s\" x=\"%s\" y=\"%s\" id=\"%d\" data-name=\"%s\"%s style=\"%s\"%s>%s",
		digitalDeviceTypeCode, fmtNum(dd.X), fmtNum(dd.Y), dd.ID, esc(dd.Name), unitAttr, esc(style), editorAttr, esc(dd.Value))
	if dd.Unit != "" {
		fmt.Fprintf(w, " <tspan style=\"%s\">%s</tspan>", esc(style), esc(dd.Unit))
	}
	fmt.Fprint(w, "</text>\n")
}

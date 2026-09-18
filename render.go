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
	"7":      "Junction point",
	"14":     "Non-intersection",
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
	"172":    "Capacitor bank",
	"52":     "Half-chassis",
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
}

// connectorTypeCode gives a Connector's data-type, mirroring an Element's
// Shape-as-data-type: the xsde2svg type code for a generic object-to-object
// connection (this schema's ClassObjectLink/KindBusWork — what
// diagramOps.connectElements creates) is 21, KindOverheadLine's is 22
// (confirmed against sld-viewer/assets/sld/IEEE9bus.svg's own real
// xsde2svg-catalog-code documentation), and KindCableLine's is 23. A Kind
// absent from this map renders with no data-type, same as before this
// existed.
var connectorTypeCode = map[ConnectorKind]string{
	KindBusWork:      "21",
	KindOverheadLine: "22",
	KindCableLine:    "23",
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
	fmt.Fprintf(w, "<g id=\"%d\" data-name=\"%s\" data-voltage=\"%s\" data-type=\"%s\"%s transform=\"translate(%s,%s) rotate(%d)\">\n%s\n</g>\n",
		e.ID, esc(e.Name), esc(color), esc(e.Shape), editorAttr, fmtNum(e.X), fmtNum(e.Y), e.Orient, body)
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

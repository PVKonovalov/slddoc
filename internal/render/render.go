package render

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"regexp"
	"sort"
	"strings"

	"github.com/PVKonovalov/slddoc/internal/model"
)

// defaultBackground is used when a Diagram carries no Editor.Background.
const defaultBackground = "#12161d"

// diagramBackground is the page background d renders on, which also backs
// the {background} template placeholder (e.g. Knife switch (44)'s circles,
// filled so they hide the blade's ends, as in the real source).
func diagramBackground(d *model.Diagram) string {
	if d.Editor != nil && d.Editor.Background != "" {
		return d.Editor.Background
	}
	return defaultBackground
}

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

// SeedStateColors gives d the legend's colors when d records none.
func SeedStateColors(d *model.Diagram, legend []StateColor) {
	if len(d.StateColors) > 0 {
		return
	}
	for _, c := range legend {
		d.StateColors = append(d.StateColors, model.DiagramStateColor{State: c.State, Color: c.Color})
	}
}

// diagramStateLegend returns legend with d's own colors in place of the
// legend's, followed by d's states the legend lacks.
func diagramStateLegend(d *model.Diagram, legend []StateColor) []StateColor {
	if len(d.StateColors) == 0 {
		return legend
	}
	own := make(map[int]string, len(d.StateColors))
	for _, c := range d.StateColors {
		own[c.State] = c.Color
	}
	out := make([]StateColor, 0, len(legend)+len(d.StateColors))
	for _, c := range legend {
		if color, ok := own[c.State]; ok {
			c.Color = color
			delete(own, c.State)
		}
		out = append(out, c)
	}
	for _, c := range d.StateColors {
		if color, ok := own[c.State]; ok {
			out = append(out, StateColor{State: c.State, Color: color})
			delete(own, c.State)
		}
	}
	return out
}

// stateColorSet is StateColors reshaped for fast per-element lookup plus
// the pre-built data-fill legend string, computed once per Render call
// rather than once per element.
type stateColorSet struct {
	colors          map[int]string
	fillAttr        string
	strokeStyleAttr string
}

// newStateColorSet builds the lookup and the legends rdss-dms recolors by state:
// data-fill="0:c0,1:c1" (fill) and data-stroke-style="0:c0/,1:c1/" (stroke color, dash kept).
func newStateColorSet(colors []StateColor) stateColorSet {
	set := stateColorSet{colors: make(map[int]string, len(colors))}
	fills := make([]string, len(colors))
	strokes := make([]string, len(colors))
	for i, c := range colors {
		set.colors[c.State] = c.Color
		fills[i] = fmt.Sprintf("%d:%s", c.State, c.Color)
		strokes[i] = fmt.Sprintf("%d:%s/", c.State, c.Color)
	}
	if len(colors) > 0 {
		set.fillAttr = fmt.Sprintf(` data-fill="%s"`, esc(strings.Join(fills, ",")))
		set.strokeStyleAttr = fmt.Sprintf(` data-stroke-style="%s"`, esc(strings.Join(strokes, ",")))
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

// tapChangerRe matches a template's {tapChanger:fragment} placeholder: the
// fragment is kept only when the element's own TapChanger is set, and
// dropped entirely otherwise (so a Static render carries no hidden
// geometry). The fragment itself may use every other placeholder.
var tapChangerRe = regexp.MustCompile(`\{tapChanger:([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}`)

func applyTapChanger(tmpl string, on bool) string {
	return tapChangerRe.ReplaceAllStringFunc(tmpl, func(m string) string {
		if !on {
			return ""
		}
		return tapChangerRe.FindStringSubmatch(m)[1]
	})
}

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
func lampColor(e model.Element) string {
	color := e.FillOff
	if e.State != nil && *e.State == 1 {
		color = e.FillOn
	}
	if color == "" {
		color = "none"
	}
	return color
}

// ShapeName gives the equipment name Render annotates a run of same-Shape
// elements with. Kept per-shape rather than per-Class since a fixed
// breaker and a withdrawable one, e.g., are both ClassBreaker but draw and
// are labeled differently.
var ShapeName = map[string]string{
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
	"6":      "Booster",
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
	"50":     "Sectionalizer (withdrawable)",
	"54":     "Ground switch",
	"398":    "Short-circuiter",
	"399":    "Power circuit breaker",
	"71":     "Disconnector",
	"76":     "Starter",
	"106":    "Lamp",
	"113":    "Button",
	"302":    "Window icon",
	"103":    "Automation device",
	"319":    "Small window",
	"360":    "Substation",
	"389":    "Blocking filter",
	"38":     "Power plant",
	"19":     "Metal anchor/angle pole",
	"11":     "Backdrop/image file",
	"310":    "Container",
	"146":    "Power pole",
	"154":    "Fuse (withdrawable)",
	"162":    "Disconnector",
	"164":    "Sectionalizer",
	"163":    "Short-circuiter without ground",
	"166":    "Disconnector-fuse",
	"172":    "Capacitor bank",
	"52":     "Half-chassis",
	"51":     "Chassis",
	"173":    "Generator",
	"174":    "Synchronous compensator",
	"39":     "Synchronous motor",
	"175":    "3-position knife switch",
	"44":     "Knife switch",
	"203":    "Fuse",
	"388":    "Capacitor",
	"156":    "Resistor",
	"157":    "Thyristor",
	"320003": "Fault passage indicator",
	"330001": "Load",
	"330002": "Network infeed",
	"330003": "Static var compensator",
	"385":    "Package substation",
	"386":    "Enclosed substation",
	"335":    "Road",
	"292":    "Post-type pole",
	"1":      "Line",
	"16":     "Polygon",
	"26":     "Fork",
	"9":      "Arc",
	"320001": "Powerflow direction",
	"320002": "Lamp on pole",
	"320007": "LED",
	"10":     "Connector",
	"83":     "Connector arrow",
	"312":    "Table",
	"313":    "Table 2",
}

// connectorKindName gives the name Render annotates a run of same-Kind
// connectors with, the same way ShapeName does for elements.
var connectorKindName = map[string]string{
	string(model.KindBusbarWire):   "Busbar wire",
	string(model.KindOverheadLine): "Overhead line",
	string(model.KindCableLine):    "Cable line",
	string(model.KindBusWork):      "Buswork",
	string(model.KindLinkToObject): "Object link",
}

// connectorTypeCode gives a Connector's data-type, mirroring an Element's
// Shape-as-data-type: the xsde2svg type code for a generic object-to-object
// connection (this schema's ClassObjectLink/KindBusWork — what
// diagramOps.connectElements creates) is 21, KindOverheadLine's is 22
// (confirmed against a real corpus file's own
// xsde2svg-catalog-code documentation), KindCableLine's is 23, and
// KindLinkToObject's is 28. A Kind absent from this map renders with no
// data-type, same as before this existed.
var connectorTypeCode = map[model.ConnectorKind]string{
	model.KindBusWork:      "21",
	model.KindOverheadLine: "22",
	model.KindCableLine:    "23",
	model.KindLinkToObject: "28",
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
var cableLineDashPatterns = map[model.ConnectorLineStyle]string{
	model.LineStyleSolid:   "",
	model.LineStyleDashed:  "stroke-dasharray: 6,5;",
	model.LineStyleDashDot: "stroke-dasharray: 70 20 25 20;",
	model.LineStyleDotted:  "stroke-dasharray: 3,2;",
}

// resolveCableLineDash resolves a KindCableLine connector's own dash
// pattern: an empty/unset LineStyle defaults to LineStyleDashed (this
// editor's original hardcoded cable-line dash, before this field
// existed, so an already-saved diagram keeps rendering the same way),
// and any value this editor doesn't recognize falls back to that same
// default rather than silently rendering solid.
func resolveCableLineDash(style model.ConnectorLineStyle) string {
	if style == "" {
		style = model.LineStyleDashed
	}
	if dash, ok := cableLineDashPatterns[style]; ok {
		return dash
	}
	return cableLineDashPatterns[model.LineStyleDashed]
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
// 0 (drawn in document order, before connectors): JunctionPoint,
// FaultPassageIndicator, and PowerflowIndicator sit directly on top of a
// wire — unlike ordinary equipment, which only ever touches a connector at a
// port, so painting the wire afterward would cut through them — and Lamp is
// a decorative status indicator meant to read as foreground UI. Render draws
// every such class in ascending order of this value, each tier after the
// connectors loop.
var elementZOrder = map[model.Class]int{
	model.ClassJunctionPoint:         1,
	model.ClassLamp:                  1,
	model.ClassFaultPassageIndicator: 1,
	model.ClassPowerflowIndicator:    1,
}

// renderElement writes one Element's symbol (or, for a BusBarSection, its
// drawn polyline), recording its Shape in missing/seenMissing when lib has
// no template for it. Doesn't write its own type-comment header (unlike an
// earlier version of this function) — that only makes sense in the context
// of a full, ordered document, not a standalone fragment, so a caller that
// wants one (Render, across both of its own element passes) writes it
// itself just before calling this, the same split renderConnector's own
// doc comment describes for a connector's. In Static mode the fragment is
// rewritten into absolute coordinates (see absolutize).
func renderElement(w io.Writer, lib *SymbolLibrary, voltageColor map[int]string, stateColors, fpiColors stateColorSet, defaultFPIText, background string, e model.Element, missing *[]string, seenMissing map[string]bool, mode RenderMode) {
	if mode != Static {
		renderElementLocal(w, lib, voltageColor, stateColors, fpiColors, defaultFPIText, background, e, missing, seenMissing, mode)
		return
	}
	var buf bytes.Buffer
	renderElementLocal(&buf, lib, voltageColor, stateColors, fpiColors, defaultFPIText, background, e, missing, seenMissing, mode)
	writeAbsolute(w, buf.String())
}

// writeAbsolute writes a Static fragment in absolute coordinates (see
// absolutize), or unchanged — still drawn correctly, just in the local
// form — if it uses a transform absolutize can't carry over.
func writeAbsolute(w io.Writer, frag string) {
	if abs, err := absolutize(frag); err == nil {
		frag = abs
	}
	io.WriteString(w, frag)
}

// renderElementLocal draws e in its own local frame, placed by
// transform="translate(x,y) rotate(orient)" (see renderElement).
func renderElementLocal(w io.Writer, lib *SymbolLibrary, voltageColor map[int]string, stateColors, fpiColors stateColorSet, defaultFPIText, background string, e model.Element, missing *[]string, seenMissing map[string]bool, mode RenderMode) {

	var color string
	if e.Class == model.ClassLamp {
		// A Lamp's colors are its own FillOff/FillOn pair, not a
		// VoltageClass — it isn't part of the electrical network.
		color = lampColor(e)
	} else if e.Class == model.ClassLampOnPole {
		// Same: its own Stroke, gray by default.
		color = e.Stroke
		if color == "" {
			color = "gray"
		}
	} else if e.Class == model.ClassConnectorPoint {
		// Same: its own Stroke, magenta by default.
		color = e.Stroke
		if color == "" {
			color = "magenta"
		}
	} else if e.Class == model.ClassFaultPassageIndicator {
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
	// junctionRadius/junctionFill back {junctionRadius}/{junctionFill} in
	// JunctionPoint's own base.xml template only — separate placeholders
	// from the generic {radius} (Lamp/FaultPassageIndicator's own, which
	// deliberately stays raw/unfallback-ed, 0 rendering invisible, since
	// those two rely on the frontend always seeding a real default on
	// placement instead) because JunctionPoint needs a *non-zero* fallback
	// to keep an already-placed/-saved instance with neither field set
	// looking exactly as it always has (radius 3, unfilled) — even though
	// real xsde2svg draws a filled dot at a genuinely varying radius far
	// more often (see Element.Radius/Fill's own doc comments); a real
	// extracted instance always sets both explicitly instead of relying on
	// either fallback.
	junctionRadius := e.Radius
	if junctionRadius == 0 {
		junctionRadius = 3
	}
	junctionFill := e.Fill
	if junctionFill == "" {
		junctionFill = "none"
	}
	// fpiText backs {fpiText} in FaultPassageIndicator's own base.xml
	// template only — its own long-standing fixed "FPI" label, now editable
	// via Element.PropertyText (see that field's own doc comment for why
	// this shape's default isn't "no label" the way 385/386's own empty
	// PropertyText is: there's no real source counterpart to match empty
	// against, since element_320.go's own custom-element case for this
	// shape draws no text at all). Falls back to Render's own
	// defaultFPIText (admin-configured — see its own doc comment) when
	// both are empty.
	fpiText := e.PropertyText
	if fpiText == "" {
		fpiText = defaultFPIText
	}
	if fpiText == "" {
		fpiText = "FPI"
	}
	if e.Class == model.ClassBusBarSection {
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
	if e.Class == model.ClassPowerTransformer {
		// A PowerTransformer's real geometry (circle count/position, each
		// winding's own connection glyph) is driven entirely by its own
		// Windings — not template substitution — so it bypasses the
		// template lookup below the same way ClassBusBarSection does.
		writePowerTransformer(w, e, voltageColor, color, mode)
		return
	}
	if e.Class == model.ClassPackageSubstation {
		// Real electrical equipment (unlike ClassRectangle/ClassArrow/
		// ClassCircle below) with a genuine voltage-driven color, but its
		// own two real appearance variants (NType) are structurally
		// different XML shapes (a single <path> vs a <g> of two <rect>s
		// plus a lead <line>) rather than a geometry tweak a static
		// template's own {state:...} substitution could express — same
		// "bypass the template lookup" reasoning as PowerTransformer's own
		// Windings-driven geometry.
		writePackageSubstation(w, e, color, mode)
		return
	}
	if e.Class == model.ClassPowerPlant {
		// Two variants (NType) with their own hatching.
		writePowerPlant(w, e, color, mode)
		return
	}
	if e.Class == model.ClassSubstation {
		// One sector path per voltage, a count Sectors sets per instance.
		writeSubstation(w, e, voltageColor, color, mode)
		return
	}
	if e.Class == model.ClassEnclosedSubstation {
		// Same reasoning as ClassPackageSubstation just above — real
		// equipment with a genuine voltage-driven color, but its own
		// fixed square-plus-triangle geometry is a <g> of two children,
		// not a single static template.
		writeEnclosedSubstation(w, e, color, mode)
		return
	}
	if e.Class == model.ClassRectangle {
		// A Rectangle's own size varies per instance (unlike every
		// template-drawn shape's fixed local geometry) and isn't part of
		// the electrical network at all (no Voltage to resolve color
		// from) — bypasses the template lookup below the same way
		// ClassBusBarSection/ClassPowerTransformer do, using its own
		// Fill/Stroke instead of the color computed above.
		writeRectangle(w, e, mode)
		return
	}
	if e.Class == model.ClassArrow {
		// Same reasoning as ClassRectangle just above — a decorative
		// annotation whose own geometry varies per instance and isn't
		// part of the electrical network, using its own Stroke instead of
		// the color computed above.
		writeArrow(w, e, mode)
		return
	}
	if e.Class == model.ClassCircle {
		// Same reasoning as ClassRectangle just above (it shares that
		// shape's own Fill/Stroke/Points convention exactly, just drawn
		// as an <ellipse>).
		writeCircle(w, e, mode)
		return
	}
	if e.Class == model.ClassButton {
		// Same reasoning as ClassRectangle just above — a decorative
		// annotation whose own geometry varies per instance and isn't
		// part of the electrical network — but unlike Rectangle/Arrow/
		// Circle it also draws its own centered PropertyText label, so it
		// needs a wrapping <g> rather than a bare tag.
		writeButton(w, e, mode)
		return
	}
	if e.Class == model.ClassContainer {
		// A decorative outline with a caption (see writeContainer).
		writeContainer(w, e, mode)
		return
	}
	if e.Class == model.ClassSmallWindow {
		// Rectangle's same bare <rect> (see writeSmallWindow).
		writeSmallWindow(w, e, mode)
		return
	}
	if e.Class == model.ClassPicture {
		// Rectangle's same two-corner frame, holding an image (see
		// writePicture).
		writePicture(w, e, mode)
		return
	}
	if e.Class == model.ClassAutomationDevice {
		// Button's box and label, picked by State (see writeAutomationDevice).
		writeAutomationDevice(w, e, mode)
		return
	}
	if e.Class == model.ClassWindowIcon {
		// Button's smaller sibling (see writeWindowIcon).
		writeWindowIcon(w, e, mode)
		return
	}
	if e.Class == model.ClassRoad {
		// Same reasoning as ClassRectangle just above — a decorative
		// annotation whose own geometry varies per instance and isn't
		// part of the electrical network — but its own Points are an
		// arbitrary multi-vertex polyline (BusBarSection's own convention),
		// not a fixed two-point shape, so it's drawn with writePolyline
		// directly rather than its own bespoke writeX function.
		writeRoad(w, e, mode)
		return
	}
	if e.Class == model.ClassConnectorArrow {
		// Its length varies per instance, which a static template can't
		// express (see writeConnectorArrow).
		writeConnectorArrow(w, e, mode)
		return
	}
	if e.Class == model.ClassPostPole {
		// Same reasoning as ClassRectangle just above — a decorative,
		// single-anchor marker whose own drawn tag (<rect> or <circle>)
		// switches on Square, something a single static template
		// substitution can't express, so it bypasses the template lookup
		// below the same way Rectangle/Circle/Button/Road do.
		writePole(w, e, mode)
		return
	}
	if e.Class == model.ClassLine {
		// Same reasoning as ClassRoad just above — a decorative annotation
		// whose own geometry is an arbitrary multi-vertex polyline
		// (BusBarSection's own convention), not part of the electrical
		// network, drawn with writePolyline directly.
		writeLine(w, e, mode)
		return
	}
	if e.Class == model.ClassArc {
		// Same reasoning as ClassLine just above — a decorative arc drawn
		// straight from its own stored SVG arc parameters.
		WriteArc(w, e, mode)
		return
	}
	if e.Class == model.ClassPolygon {
		// Same reasoning as ClassLine just above — a decorative closed
		// shape drawn straight from its own Points.
		writePolygon(w, e, mode)
		return
	}
	if e.Class == model.ClassPowerflowIndicator {
		// Same reasoning as ClassPostPole just above — a decorative,
		// single-anchor marker (here, a rotated arrow glyph rather than a
		// <rect>/<circle>) using its own TextColor/State instead of the
		// color computed above.
		writePowerflowIndicator(w, e, mode)
		return
	}
	if e.Class == model.ClassTable {
		// Same reasoning as ClassButton just above — a decorative
		// annotation whose own geometry varies per instance and isn't
		// part of the electrical network, using its own Fill/Stroke/
		// LineStyle instead of the color computed above, and always
		// drawing its own centered PropertyText label the identical
		// wrapping-<g> way Button's own does.
		writeTable(w, e, mode)
		return
	}
	if e.Class == model.ClassTable2 {
		// A Table2's own geometry (RowHeights/ColumnWidths/Cells) is
		// driven entirely by its own fields, not template substitution —
		// bypasses the template lookup below the same way
		// PowerTransformer's own Windings-driven geometry does.
		WriteTable2(w, e, mode)
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
	body := applyStateLine(applyTapChanger(tmpl, e.TapChanger), e.State)
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
		"{radius}", model.FmtNum(templateRadius(e)),
		"{fillAttr}", stateColors.fillAttr,
		"{stateAttr}", stateAttr(e.State),
		"{positionAttr}", positionAttr(e.Position),
		"{positionOffset}", positionOffset(e.Position),
		"{fpiColor}", fpiColors.fpiColor(e.State),
		"{fpiFillAttr}", fpiColors.fillAttr,
		"{fpiStrokeAttr}", fpiColors.strokeStyleAttr,
		"{counterRotate}", model.FmtNum(float64(-e.Orient)),
		"{junctionRadius}", model.FmtNum(junctionRadius),
		"{junctionFill}", esc(junctionFill),
		"{fpiText}", esc(fpiText),
		"{background}", esc(background),
	).Replace(body)
	body += leadExtension(e, esc(color))
	if e.Scale != 0 {
		body = model.SizeStepTemplate(body, model.SizeFactor(e.Scale))
	}
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
	fmt.Fprintf(w, "<g id=\"%d\" data-name=\"%s\" data-voltage=\"%s\" data-type=\"%s\"%s transform=\"translate(%s,%s) rotate(%d)%s%s\">\n%s\n</g>\n",
		e.ID, esc(e.Name), esc(color), esc(e.Shape), editorAttr, model.FmtNum(e.X), model.FmtNum(e.Y), e.Orient, mirrorScale(e.Mirror), model.SizeStepScale(e), body)
}

// ForkArmLength is a Fork's (shape 26) own default arm length, the real
// source's own Scale(scaleChosed, 10) at scale 0.
const ForkArmLength = 10

// templateRadius is the value a symbol template's own {radius} placeholder
// draws with: an element's own Radius, except that a Fork with none
// falls back to ForkArmLength (every other shape using {radius} draws a
// literal 0 when unset, as it always has).
func templateRadius(e model.Element) float64 {
	if e.Class == model.ClassFork && e.Radius <= 0 {
		return ForkArmLength
	}
	return e.Radius
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
// defaultFPIText is the label a FaultPassageIndicator (320003) draws
// centered on itself when its own Element.PropertyText is unset — this
// shape's own real source draws no text at all (see PropertyText's own
// doc comment in model.go), so there's nothing to derive a default from;
// it's admin-configurable per install (config.Config.Indicators.
// DefaultFPIText) rather than hardcoded, the same reasoning
// fpiStateColorLegend/stateColorLegend already get their own config
// section for. "" falls back to the literal "FPI" this project has always
// shown, for a caller that hasn't wired one up.
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
func Render(d *model.Diagram, lib *SymbolLibrary, w io.Writer, mode RenderMode, defaultFPIText string, fpiStateColorLegend []StateColor, stateColorLegend ...StateColor) error {
	voltageColor := map[int]string{}
	for _, vc := range d.VoltageClasses {
		voltageColor[vc.ID] = vc.Color
	}
	stateColors := newStateColorSet(diagramStateLegend(d, stateColorLegend))
	fpiColors := newStateColorSet(fpiStateColorLegend)

	background := diagramBackground(d)

	fmt.Fprintf(w, "<?xml version=\"1.0\"?>\n<svg width=\"%s\" height=\"%s\" style=\"stroke-width: 0px; background-color: %s;\" xmlns=\"http://www.w3.org/2000/svg\" xmlns:xlink=\"http://www.w3.org/1999/xlink\">\n",
		model.FmtNum(d.Width), model.FmtNum(d.Height), esc(background))

	io.WriteString(w, writeLayerMetadata(d))

	var missing []string
	seenMissing := map[string]bool{}

	// Items are drawn layer group by layer group, lowest Layer.Z first (see
	// Layer.Z); an unknown layer counts as Z 0. Within a group the order is
	// fixed: devices, wires, elementZOrder's indicator tiers, text labels,
	// digital devices.
	layerZ := map[int]int{}
	for _, l := range d.Layers {
		layerZ[l.ID] = l.Z
	}
	zSet := map[int]bool{}
	for _, e := range d.Elements {
		zSet[layerZ[e.Layer]] = true
	}
	for _, c := range d.Connectors {
		zSet[layerZ[c.Layer]] = true
	}
	for _, l := range d.Labels {
		zSet[layerZ[l.Layer]] = true
	}
	for _, dd := range d.DigitalDevices {
		zSet[layerZ[dd.Layer]] = true
	}
	groups := make([]int, 0, len(zSet))
	for z := range zSet {
		groups = append(groups, z)
	}
	sort.Ints(groups)

	for _, group := range groups {
		elevated := map[int][]model.Element{}

		var lastShape string
		for _, e := range d.Elements {
			if layerZ[e.Layer] != group {
				continue
			}
			if z := elementZOrder[e.Class]; z > 0 {
				elevated[z] = append(elevated[z], e)
				continue
			}
			typeComment(w, ShapeName, e.Shape, e.Shape, &lastShape)
			renderElementLayered(w, lib, voltageColor, stateColors, fpiColors, defaultFPIText, background, e, &missing, seenMissing, mode)
		}

		var lastConnKind string
		for _, c := range d.Connectors {
			if layerZ[c.Layer] != group {
				continue
			}
			code := connectorTypeCode[c.Kind]
			typeComment(w, connectorKindName, string(c.Kind), code, &lastConnKind)
			renderConnectorLayered(w, c, voltageColor, mode)
		}

		tiers := make([]int, 0, len(elevated))
		for z := range elevated {
			tiers = append(tiers, z)
		}
		sort.Ints(tiers)
		for _, z := range tiers {
			var lastTierShape string
			for _, e := range elevated[z] {
				typeComment(w, ShapeName, e.Shape, e.Shape, &lastTierShape)
				renderElementLayered(w, lib, voltageColor, stateColors, fpiColors, defaultFPIText, background, e, &missing, seenMissing, mode)
			}
		}

		wroteLabelComment := false
		for _, l := range d.Labels {
			if layerZ[l.Layer] != group {
				continue
			}
			if !wroteLabelComment {
				fmt.Fprintf(w, "<!-- Text:%s -->\n", labelTypeCode)
				wroteLabelComment = true
			}
			writeLabelLayered(w, l, mode)
		}

		wroteDDComment := false
		for _, dd := range d.DigitalDevices {
			if layerZ[dd.Layer] != group {
				continue
			}
			if !wroteDDComment {
				fmt.Fprintf(w, "<!-- Digital device2:%s -->\n", digitalDeviceTypeCode)
				wroteDDComment = true
			}
			writeDigitalDeviceLayered(w, dd, mode)
		}
	}

	fmt.Fprint(w, "</svg>\n")

	if len(missing) > 0 {
		return fmt.Errorf("slddoc: symbol library missing shape(s): %s", strings.Join(missing, ", "))
	}
	return nil
}

// RenderFragments is Render's incremental-editing counterpart: instead of a
// whole document, it renders only the requested ids' own markup — for a
// caller that already has a live rendering of (an earlier version of) d and
// only needs fresh markup for whatever actually changed since, rather than
// regenerating and re-transferring the entire diagram on every edit (the
// "big diagram, small edit" cost — see TODO.md's own "Reducing frontend/
// backend traffic" section for the fuller rationale). It still runs the
// same whole-diagram voltage/topology resolution pass Render does (an id's
// own rendered color, e.g., can depend on a VoltageClass that isn't itself
// one of the requested ids), but only writes markup for ids actually asked
// for — every other Element/Connector/Label/DigitalDevice in d is resolved
// against but never rendered, so this is still O(diagram size) to run, just
// not O(diagram size) to transfer back.
//
// ids may name an Element, a Connector, a Label, or a DigitalDevice — this
// schema's ids are one shared space across all four (see Diagram.LastID's
// own doc comment, and diagramOps.IdSequence on the frontend), so a plain
// int works as the lookup key regardless of which kind an id turns out to
// be, without the caller having to say which in advance. An id present in
// ids but no longer found in d at all (the caller's own record of
// something it just deleted locally) is silently skipped, not an error —
// the caller already knows it's gone and isn't asking this to confirm it;
// the returned map simply won't have an entry for that id.
//
// Two things Render does that this deliberately doesn't: it never writes
// per-shape/per-Kind type-comment headers (typeComment) — those only make
// sense in the context of a full, ordered document grouping same-shape
// runs together, not a handful of scattered, unrelated fragments — and it
// ignores elementZOrder/z-order tiering entirely, since a fragment is
// meant to replace one already-positioned DOM node in place (keeping
// whatever position a prior full Render already gave it), not to be
// inserted fresh into document order.
func RenderFragments(d *model.Diagram, lib *SymbolLibrary, ids []int, mode RenderMode, defaultFPIText string, fpiStateColorLegend []StateColor, stateColorLegend ...StateColor) (map[int]string, error) {
	voltageColor := map[int]string{}
	for _, vc := range d.VoltageClasses {
		voltageColor[vc.ID] = vc.Color
	}
	stateColors := newStateColorSet(diagramStateLegend(d, stateColorLegend))
	fpiColors := newStateColorSet(fpiStateColorLegend)

	want := make(map[int]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}

	fragments := make(map[int]string, len(ids))
	var missing []string
	seenMissing := map[string]bool{}

	for _, e := range d.Elements {
		if !want[e.ID] {
			continue
		}
		var buf bytes.Buffer
		renderElementLayered(&buf, lib, voltageColor, stateColors, fpiColors, defaultFPIText, diagramBackground(d), e, &missing, seenMissing, mode)
		fragments[e.ID] = buf.String()
	}

	for _, c := range d.Connectors {
		if !want[c.ID] {
			continue
		}
		var buf bytes.Buffer
		renderConnectorLayered(&buf, c, voltageColor, mode)
		fragments[c.ID] = buf.String()
	}

	for _, l := range d.Labels {
		if !want[l.ID] {
			continue
		}
		var buf bytes.Buffer
		writeLabelLayered(&buf, l, mode)
		fragments[l.ID] = buf.String()
	}

	for _, dd := range d.DigitalDevices {
		if !want[dd.ID] {
			continue
		}
		var buf bytes.Buffer
		writeDigitalDeviceLayered(&buf, dd, mode)
		fragments[dd.ID] = buf.String()
	}

	if len(missing) > 0 {
		return fragments, fmt.Errorf("slddoc: symbol library missing shape(s): %s", strings.Join(missing, ", "))
	}
	return fragments, nil
}

// The *Layered writers are Render/RenderFragments' entry points for one
// item: the item's own markup with data-layer and data-group added to its
// root node (withItemAttrs), in both modes, so a viewer can toggle layers or
// pick a group on the exported SVG and the canvas can hide layers while
// editing.

func renderElementLayered(w io.Writer, lib *SymbolLibrary, voltageColor map[int]string, stateColors, fpiColors stateColorSet, defaultFPIText, background string, e model.Element, missing *[]string, seenMissing map[string]bool, mode RenderMode) {
	if e.Layer == model.BaseLayer && e.Group == 0 {
		renderElement(w, lib, voltageColor, stateColors, fpiColors, defaultFPIText, background, e, missing, seenMissing, mode)
		return
	}
	var buf bytes.Buffer
	renderElement(&buf, lib, voltageColor, stateColors, fpiColors, defaultFPIText, background, e, missing, seenMissing, mode)
	io.WriteString(w, withItemAttrs(buf.String(), e.Layer, e.Group))
}

func renderConnectorLayered(w io.Writer, c model.Connector, voltageColor map[int]string, mode RenderMode) {
	if c.Layer == model.BaseLayer && c.Group == 0 {
		renderConnector(w, c, voltageColor, mode)
		return
	}
	var buf bytes.Buffer
	renderConnector(&buf, c, voltageColor, mode)
	io.WriteString(w, withItemAttrs(buf.String(), c.Layer, c.Group))
}

func writeLabelLayered(w io.Writer, l model.Label, mode RenderMode) {
	if l.Layer == model.BaseLayer && l.Group == 0 {
		WriteLabel(w, l, mode)
		return
	}
	var buf bytes.Buffer
	WriteLabel(&buf, l, mode)
	io.WriteString(w, withItemAttrs(buf.String(), l.Layer, l.Group))
}

func writeDigitalDeviceLayered(w io.Writer, dd model.DigitalDevice, mode RenderMode) {
	if dd.Layer == model.BaseLayer && dd.Group == 0 {
		writeDigitalDevice(w, dd, mode)
		return
	}
	var buf bytes.Buffer
	writeDigitalDevice(&buf, dd, mode)
	io.WriteString(w, withItemAttrs(buf.String(), dd.Layer, dd.Group))
}

// renderConnector writes one Connector's own rendered markup — the
// Kind-based dispatch (a named `<g>` for OverheadLine/CableLine, an object
// link's own arrow-plus-stub, or a plain `<polyline>` for everything else)
// Render's own connectors loop used to inline directly; factored out so
// RenderFragments can render a single connector's own fragment without
// duplicating that branching. Does not write the connectors loop's own
// typeComment — that only makes sense in the context of a full, ordered
// document, not a standalone fragment, so callers that want one (Render)
// still write it themselves just before calling this.
//
// A connector with no (or an unknown) voltage class is drawn gray, the same
// visible neutral an element falls back to — not black, which disappears
// against the usual dark diagram background.
func renderConnector(w io.Writer, c model.Connector, voltageColor map[int]string, mode RenderMode) {
	if mode != Static {
		renderConnectorLocal(w, c, voltageColor, mode)
		return
	}
	var buf bytes.Buffer
	renderConnectorLocal(&buf, c, voltageColor, mode)
	writeAbsolute(w, buf.String())
}

// renderConnectorLocal is renderConnector before the Static absolute pass
// (only an object link's arrowhead is placed in a local frame).
func renderConnectorLocal(w io.Writer, c model.Connector, voltageColor map[int]string, mode RenderMode) {
	color := voltageColor[c.Voltage]
	if color == "" {
		color = "gray"
	}
	code := connectorTypeCode[c.Kind]
	if c.Kind == model.KindOverheadLine || c.Kind == model.KindCableLine {
		writeNamedLine(w, c, color, code, mode)
		return
	}
	if c.Kind == model.KindLinkToObject {
		writeObjectLink(w, c, color, code, mode)
		return
	}
	writePolyline(w, c.ID, "connector", c.Points, color, c.Dashed, 1, wireDataAttrs(c, color, code), mode)
}

// wireDataAttrs is a bare wire polyline's (buswork 21, object link 28)
// data attributes, as real xsde2svg writes them: data-name when named,
// data-type, and the resolved data-voltage color.
func wireDataAttrs(c model.Connector, color, code string) string {
	var sb strings.Builder
	if c.Name != "" {
		fmt.Fprintf(&sb, " data-name=\"%s\"", esc(c.Name))
	}
	if code != "" {
		fmt.Fprintf(&sb, " data-type=\"%s\"", esc(code))
	}
	fmt.Fprintf(&sb, " data-voltage=\"%s\"", esc(color))
	return sb.String()
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
func writePolyline(w io.Writer, id int, kind string, pts []model.Point, color string, dashed bool, strokeWidth float64, dataAttrs string, mode RenderMode) {
	if color == "" {
		color = "black"
	}
	var sb strings.Builder
	for i, p := range pts {
		if i > 0 {
			sb.WriteByte(' ')
		}
		sb.WriteString(model.FmtNum(p.X))
		sb.WriteByte(',')
		sb.WriteString(model.FmtNum(p.Y))
	}
	dash := ""
	if dashed {
		dash = "stroke-dasharray: 14,9;"
	}
	points := esc(sb.String())
	width := model.FmtNum(strokeWidth)
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
func writeRectangle(w io.Writer, e model.Element, mode RenderMode) {
	writeRect(w, e, mode, "3", "white", e.StrokeWidth, e.RadiusX)
}

// writeSmallWindow draws a Small window (shape 319) as Rectangle's same bare
// <rect>, with element_319.go's fixed 1px border and gray as the default
// border color.
func writeSmallWindow(w io.Writer, e model.Element, mode RenderMode) {
	writeRect(w, e, mode, "319", "gray", 1, 0)
}

// writePicture draws a Picture (shape 11). Static is real xsde2svg's own bare
// <image x y width height xlink:href> (element_11_12.go), plus this
// editor's id/data-name/data-type, and nothing at all when there is no
// image, as in the source. Interactive wraps the same <image> in a <g> the
// canvas can select and drag, but leaves its href out: the canvas strips
// every Href from the diagram it posts (so a large backdrop isn't resent on
// every edit) and fills the attribute in itself from its own state. A
// Picture without an image (Href empty) gets a dashed placeholder frame
// there instead, so it can still be seen and picked. Fewer than 2 Points
// draws nothing, as for writeRect.
func writePicture(w io.Writer, e model.Element, mode RenderMode) {
	if len(e.Points) < 2 {
		return
	}
	p0, p1 := e.Points[0], e.Points[1]
	x, y := math.Min(p0.X, p1.X), math.Min(p0.Y, p1.Y)
	width, height := math.Abs(p1.X-p0.X), math.Abs(p1.Y-p0.Y)
	dims := fmt.Sprintf(`x="%s" y="%s" width="%s" height="%s"`, model.FmtNum(x), model.FmtNum(y), model.FmtNum(width), model.FmtNum(height))

	if mode != Interactive {
		if e.Href == "" {
			return
		}
		fmt.Fprintf(w, "<image id=\"%d\" %s xlink:href=\"%s\" data-name=\"%s\" data-type=\"11\" />\n",
			e.ID, dims, esc(e.Href), esc(e.Name))
		return
	}
	fmt.Fprintf(w, "<g id=\"%d\" data-name=\"%s\" data-type=\"11\" data-editor-kind=\"element\">\n", e.ID, esc(e.Name))
	if e.Href == "" {
		fmt.Fprintf(w, "<rect %s style=\"fill:none;stroke:gray;stroke-width:1;stroke-dasharray:4 2\" />\n", dims)
	} else {
		fmt.Fprintf(w, "<image %s />\n", dims)
	}
	io.WriteString(w, "</g>\n")
}

// writeRect is writeRectangle/writeSmallWindow's shared writer: code is the
// data-type, defaultStroke the unset-Stroke fallback, strokeWidth the
// border width (0 = 1), radius the corner radius (0 = square, capped at half
// the shorter side).
func writeRect(w io.Writer, e model.Element, mode RenderMode, code, defaultStroke string, strokeWidth, radius float64) {
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
		stroke = defaultStroke
	}
	if strokeWidth <= 0 {
		strokeWidth = 1
	}

	corner := ""
	if radius = math.Min(radius, math.Min(width, height)/2); radius > 0 {
		corner = fmt.Sprintf(" rx=\"%s\"", model.FmtNum(radius))
	}
	editorAttr := ""
	if mode == Interactive {
		editorAttr = " data-editor-kind=\"element\""
	}
	fmt.Fprintf(w, "<rect id=\"%d\" x=\"%s\" y=\"%s\" width=\"%s\" height=\"%s\"%s style=\"fill:%s;stroke:%s;stroke-width:%s\" data-name=\"%s\" data-voltage=\"%s\" data-type=\"%s\"%s />\n",
		e.ID, model.FmtNum(x), model.FmtNum(y), model.FmtNum(width), model.FmtNum(height), corner, esc(fill), esc(stroke), model.FmtNum(strokeWidth), esc(e.Name), esc(stroke), code, editorAttr)
}

// writeCircle draws a Circle (shape 4) as a bare <ellipse> with rx = ry =
// Radius about its center X/Y, as xsde2svg's element_4.go emits it (no
// wrapping <g>). A Circle still holding old bounding-box Points, or no
// radius, is converted first (circleFromCorners). Fill/Stroke/StrokeWidth
// fall back as writeRectangle's do.
func writeCircle(w io.Writer, e model.Element, mode RenderMode) {
	e = model.CircleFromCorners(e)
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
	r := model.FmtNum(e.Radius)
	fmt.Fprintf(w, "<ellipse id=\"%d\" cx=\"%s\" cy=\"%s\" rx=\"%s\" ry=\"%s\" style=\"fill:%s;stroke:%s;stroke-width:%s\" data-name=\"%s\" data-voltage=\"%s\" data-type=\"4\"%s />\n",
		e.ID, model.FmtNum(e.X), model.FmtNum(e.Y), r, r, esc(fill), esc(stroke), model.FmtNum(strokeWidth), esc(e.Name), esc(stroke), editorAttr)
}

// buttonFontSize is a Button's (113) own PropertyText size — fixed, not
// per-instance, matching every real corpus instance found (23px regardless
// of the button's own drawn width/height).
const buttonFontSize = 23

// writeButton draws a Button (shape 113) as a <rect>+<text> pair inside a
// wrapping <g id data-type="113">, matching real xsde2svg-exported markup
// (internal/modus/element_113.go's own canvas.Group/Rect/Textspan calls) —
// unlike writeRectangle/writeCircle/writeArrow, this shape always draws a
// centered label too, so it needs the wrapping <g> real bare-tag shapes
// don't. Its own two Points (any order, same convention as
// writeRectangle's) are normalized into a top-left x/y plus a positive
// width/height. Fill/Stroke/StrokeWidth fall back exactly the same way
// writeRectangle's own do ("none"/"white"/1) — real corpus always draws a
// solid background, but this editor's own placeButton defaults it
// transparent anyway, the same "user picks a real fill" convention every
// other decorative annotation shape already uses. TextColor falls back to
// white, the more common real case; Bold draws PropertyText with font-weight:bold,
// matching the real source's own ParamText.FontStyle-driven "BOLD" branch.
// A Button with fewer than 2 Points draws nothing, same as writeRectangle.
func writeButton(w io.Writer, e model.Element, mode RenderMode) {
	writeTextBox(w, e, mode, textBoxStyle{
		code: "113", fontSize: buttonFontSize, stroke: "white", textColor: "white", strokeWidth: e.StrokeWidth,
	})
}

// windowIconFontSize is a Window icon's (302) own label size: the source's
// Scale(scaleChosed, 12), and 12px in every real corpus instance.
const windowIconFontSize = 12

// writeWindowIcon draws a Window icon (shape 302) the way
// internal/modus/element_302.go does: Button's <g><rect/><text/></g>, but
// with a 12px label 2 units below the box's center, a fixed 1px border, and
// black as the default border and text color (the source leaves its text
// fill empty, which a browser draws black; a real color is written instead).
func writeWindowIcon(w io.Writer, e model.Element, mode RenderMode) {
	writeTextBox(w, e, mode, textBoxStyle{
		code: "302", fontSize: windowIconFontSize, textDY: 2, stroke: "black", textColor: "black", strokeWidth: 1,
	})
}

// Connector arrow (83) geometry: the default total length and the
// arrowhead's length/half-width, element_83.go's own fixed 11 and 5.
const (
	ConnectorArrowLength     = 30
	ConnectorArrowHeadLength = 11
	connectorArrowHeadHalf   = 5
)

// writeConnectorArrow draws a Connector arrow (shape 83) in its own local
// frame — the line from its tail (the anchor, its terminal) along +x, then
// the source's arrowhead triangle — placed by translate(x,y) rotate(Orient)
// like any symbol; Static rewrites that into absolute coordinates with
// rotate(Orient,x,y), the form element_83.go itself writes for a diagonal
// arrow. Stroke falls back to coral, HeadStroke to dimgray, Fill to white.
func writeConnectorArrow(w io.Writer, e model.Element, mode RenderMode) {
	length := e.Length
	if length <= 0 {
		length = ConnectorArrowLength
	}
	// Rounded to the hundredths element_83.go itself writes ("%.2f").
	shaft := math.Round((length-ConnectorArrowHeadLength)*100) / 100
	if shaft < 0 {
		shaft = 0
	}
	stroke := e.Stroke
	if stroke == "" {
		stroke = "coral"
	}
	head := e.HeadStroke
	if head == "" {
		head = "dimgray"
	}
	fill := e.Fill
	if fill == "" {
		fill = "white"
	}
	editorAttr := ""
	if mode == Interactive {
		editorAttr = " data-editor-kind=\"element\""
	}
	fmt.Fprintf(w, "<g id=\"%d\" data-name=\"%s\" data-voltage=\"%s\" data-type=\"83\"%s transform=\"translate(%s,%s) rotate(%d)\">\n",
		e.ID, esc(e.Name), esc(stroke), editorAttr, model.FmtNum(e.X), model.FmtNum(e.Y), e.Orient)
	fmt.Fprintf(w, "<path d=\"M 0 0 h %s\" style=\"fill:none;stroke:%s;stroke-width:1\" />\n", model.FmtNum(shaft), esc(stroke))
	fmt.Fprintf(w, "<path d=\"M %s %d l %d %d l %d %d z\" style=\"fill:%s;stroke:%s;stroke-width:1\" />\n",
		model.FmtNum(shaft), connectorArrowHeadHalf, ConnectorArrowHeadLength, -connectorArrowHeadHalf, -ConnectorArrowHeadLength, -connectorArrowHeadHalf, esc(fill), esc(head))
	fmt.Fprint(w, "</g>\n")
}

// containerDashPatterns maps a Container's (310) LineStyle to its
// stroke-dasharray, as internal/modus/element_310.go writes it
// ("пунктирная"/"штриховая"). Any other style draws solid.
var containerDashPatterns = map[model.ConnectorLineStyle]string{
	model.LineStyleDotted: "stroke-dasharray: 3,2;",
	model.LineStyleDashed: "stroke-dasharray: 6,5;",
}

// containerFontSize is a Container's caption size when TextSize is unset:
// the source's Scale(textScale, 14).
const containerFontSize = 14

// containerTextAnchor returns a Container caption's on-canvas anchor: the
// outline's top-left plus TextDx/TextDy.
func containerTextAnchor(e model.Element) (float64, float64) {
	minX, minY := e.Points[0].X, e.Points[0].Y
	for _, p := range e.Points[1:] {
		minX, minY = math.Min(minX, p.X), math.Min(minY, p.Y)
	}
	return minX + e.TextDx, minY + e.TextDy
}

// writeContainer draws a Container (shape 310) as one <g id data-type="310"
// data-name="caption" data-voltage="stroke"> holding its closed outline
// <path> and, when it has one, its caption <text> — the form the patched
// xsde2svg element_310.go writes (an older export instead put the outline
// in a bare <path> after the group; see parseContainer). Fill falls back
// to "none", Stroke to "gray", StrokeWidth to 1, TextColor to white,
// TextSize to 14 and TextAnchor/TextBaseline to middle. A Container with
// fewer than 3 Points draws nothing.
func writeContainer(w io.Writer, e model.Element, mode RenderMode) {
	if len(e.Points) < 3 {
		return
	}
	fill := e.Fill
	if fill == "" {
		fill = "none"
	}
	stroke := e.Stroke
	if stroke == "" {
		stroke = "gray"
	}
	strokeWidth := e.StrokeWidth
	if strokeWidth <= 0 {
		strokeWidth = 1
	}
	var d strings.Builder
	for i, p := range e.Points {
		if i == 0 {
			d.WriteString("M ")
		} else {
			d.WriteString(" L")
		}
		d.WriteString(model.FmtNum(p.X) + " " + model.FmtNum(p.Y))
	}
	d.WriteString(" z")

	editorAttr := ""
	if mode == Interactive {
		editorAttr = " data-editor-kind=\"element\""
	}
	nameAttr := ""
	if e.PropertyText != "" {
		nameAttr = fmt.Sprintf(" data-name=\"%s\"", esc(e.PropertyText))
	}
	fmt.Fprintf(w, "<g id=\"%d\" data-type=\"310\"%s data-voltage=\"%s\"%s>\n", e.ID, nameAttr, esc(stroke), editorAttr)
	fmt.Fprintf(w, "<path d=\"%s\" style=\"fill:%s;stroke:%s;%sstroke-width:%s\" />\n",
		d.String(), esc(fill), esc(stroke), containerDashPatterns[e.LineStyle], model.FmtNum(strokeWidth))
	if e.PropertyText != "" {
		textColor := e.TextColor
		if textColor == "" {
			textColor = "white"
		}
		size := e.TextSize
		if size <= 0 {
			size = containerFontSize
		}
		anchor := e.TextAnchor
		if anchor == "" {
			anchor = "middle"
		}
		baseline := e.TextBaseline
		if baseline == "" {
			baseline = "middle"
		}
		x, y := containerTextAnchor(e)
		rotate := ""
		if e.Orient != 0 {
			rotate = fmt.Sprintf(" transform=\"rotate(%d,%s,%s)\"", e.Orient, model.FmtNum(x), model.FmtNum(y))
		}
		style := fmt.Sprintf("fill:%s;text-anchor:%s;dominant-baseline:%s;font-size:%spx;font-family:Arial", textColor, anchor, baseline, model.FmtNum(size))
		fmt.Fprintf(w, "<text x=\"%s\" y=\"%s\" style=\"%s\"%s>%s</text>\n", model.FmtNum(x), model.FmtNum(y), esc(style), rotate, esc(e.PropertyText))
	}
	fmt.Fprint(w, "</g>\n")
}

// textBoxStyle is what differs between Button and Window icon: their
// data-type, label size and vertical offset, the default border and text
// colors, and the border width.
type textBoxStyle struct {
	code        string
	fontSize    int
	textDY      float64
	stroke      string
	textColor   string
	strokeWidth float64
}

// AutomationDeviceFontSize is an Automation device's (103) default label
// size, the source's Scale(scaleChosed, 12).
const AutomationDeviceFontSize = 12

// writeAutomationDevice draws an Automation device (shape 103) the way
// internal/modus/element_103.go does: Button's <g><rect/><text/></g> with a
// 1px border (Stroke, the source's typ103_default black when unset) and the
// label 2 units below the center, its fill, text and text color picked by
// State: On (1) FillOn/PropertyTextOn/TextColorOn, otherwise
// FillOff/PropertyText/TextColor. The source writes no state; this editor
// adds data-state (when set) so an exported tile reads back in its state.
func writeAutomationDevice(w io.Writer, e model.Element, mode RenderMode) {
	if len(e.Points) < 2 {
		return
	}
	p0, p1 := e.Points[0], e.Points[1]
	x, y := math.Min(p0.X, p1.X), math.Min(p0.Y, p1.Y)
	width, height := math.Abs(p1.X-p0.X), math.Abs(p1.Y-p0.Y)
	on := e.State != nil && *e.State == 1
	fill, text, textColor := e.FillOff, e.PropertyText, e.TextColor
	if on {
		fill, text, textColor = e.FillOn, e.PropertyTextOn, e.TextColorOn
	}
	if fill == "" {
		fill = "none"
	}
	if textColor == "" {
		textColor = "black"
	}
	stroke := e.Stroke
	if stroke == "" {
		stroke = "black"
	}
	size := e.TextSize
	if size <= 0 {
		size = AutomationDeviceFontSize
	}
	weight := ""
	if e.Bold {
		weight = ";font-weight: bold"
	}
	editorAttr := ""
	if mode == Interactive {
		editorAttr = " data-editor-kind=\"element\""
	}
	fmt.Fprintf(w, "<g id=\"%d\" data-type=\"103\" data-name=\"%s\" data-voltage=\"%s\"%s%s>\n", e.ID, esc(e.Name), esc(fill), stateAttr(e.State), editorAttr)
	fmt.Fprintf(w, "<rect x=\"%s\" y=\"%s\" width=\"%s\" height=\"%s\" style=\"fill:%s;stroke:%s;stroke-width:1\" />\n",
		model.FmtNum(x), model.FmtNum(y), model.FmtNum(width), model.FmtNum(height), esc(fill), esc(stroke))
	if text != "" {
		style := fmt.Sprintf("fill:%s;text-anchor:middle;dominant-baseline:middle;font-size:%spx;font-family:Arial%s", textColor, model.FmtNum(size), weight)
		fmt.Fprintf(w, "<text x=\"%s\" y=\"%s\" style=\"%s\">%s</text>\n", model.FmtNum(x+width/2), model.FmtNum(y+height/2+2), esc(style), esc(text))
	}
	fmt.Fprint(w, "</g>\n")
}

// writeTextBox is writeButton/writeWindowIcon's shared writer.
func writeTextBox(w io.Writer, e model.Element, mode RenderMode, st textBoxStyle) {
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
		stroke = st.stroke
	}
	strokeWidth := st.strokeWidth
	if strokeWidth <= 0 {
		strokeWidth = 1
	}
	textColor := e.TextColor
	if textColor == "" {
		textColor = st.textColor
	}
	weight := ""
	if e.Bold {
		weight = ";font-weight: bold"
	}

	editorAttr := ""
	if mode == Interactive {
		editorAttr = " data-editor-kind=\"element\""
	}
	fmt.Fprintf(w, "<g id=\"%d\" data-type=\"%s\" data-name=\"%s\" data-voltage=\"%s\"%s>\n", e.ID, st.code, esc(e.Name), esc(stroke), editorAttr)
	fmt.Fprintf(w, "<rect x=\"%s\" y=\"%s\" width=\"%s\" height=\"%s\" style=\"fill:%s;stroke:%s;stroke-width:%s\" />\n",
		model.FmtNum(x), model.FmtNum(y), model.FmtNum(width), model.FmtNum(height), esc(fill), esc(stroke), model.FmtNum(strokeWidth))
	if e.PropertyText != "" {
		style := fmt.Sprintf("fill:%s;text-anchor:middle;dominant-baseline:middle;font-size:%dpx;font-family:Arial%s", textColor, st.fontSize, weight)
		fmt.Fprintf(w, "<text x=\"%s\" y=\"%s\" style=\"%s\">%s</text>\n", model.FmtNum(x+width/2), model.FmtNum(y+height/2+st.textDY), esc(style), esc(e.PropertyText))
	}
	fmt.Fprint(w, "</g>\n")
}

// tableFontSize is a Table's (312) own PropertyText size — fixed, not
// per-instance, the same simplification buttonFontSize already makes (the
// real source's own default computes to ~13.6px; a genuinely varying
// per-instance ParamText.Font.Size is not modeled).
const tableFontSize = 14

// tableDashPatterns maps a Table's (312) own LineStyle to its real
// stroke-dasharray value, matching xsde2svg's own line-style switch
// (internal/modus/element_312.go's own "штриховая"/"штрихпунктирная" cases)
// exactly — LineStyleDashed's own value happens to match Line's (1) own,
// but LineStyleDashDot's doesn't (it matches a KindCableLine connector's
// own instead — a real coincidence in the source, not a pattern to read
// into), so this shape needs its own map rather than reusing either.
var tableDashPatterns = map[model.ConnectorLineStyle]string{
	model.LineStyleSolid:   "",
	model.LineStyleDashed:  "stroke-dasharray: 6,5;",
	model.LineStyleDashDot: "stroke-dasharray: 70 20 25 20;",
}

// writeTable draws a Table (shape 312) as a <rect>+<text> pair inside a
// wrapping <g id data-type="312">, the same structure writeButton already
// uses — matching real xsde2svg-exported markup only since this project's
// own request added that wrapping <g> to internal/modus/element_312.go
// (see ClassTable's own doc comment for why: a real instance had no
// stable id/grouping of its own at all before that). Its own two Points
// (any order, same convention as writeRectangle's) are normalized into a
// top-left x/y plus a positive width/height. Fill falls back to "none",
// Stroke to "white" — the real source's own fallback for both is "none"
// (this editor has no per-object-type default-color config to replicate
// that against, so an unconfigured table would otherwise render with no
// visible border at all; every other decorative shape here already picks
// a visible fallback over literal fidelity for the same reason).
// PropertyText, when set, draws centered and — unlike writeButton's own
// label, which never rotates — is rotated around the box's own center by
// Orient when non-zero, matching the real source's own ParamText.Orient
// (only the label rotates; the box itself never does). A Table with fewer
// than 2 Points draws nothing, same as writeRectangle.
func writeTable(w io.Writer, e model.Element, mode RenderMode) {
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
	dash := tableDashPatterns[e.LineStyle]
	textColor := e.TextColor
	if textColor == "" {
		textColor = "black"
	}

	editorAttr := ""
	if mode == Interactive {
		editorAttr = " data-editor-kind=\"element\""
	}
	fmt.Fprintf(w, "<g id=\"%d\" data-type=\"312\" data-name=\"%s\" data-voltage=\"%s\"%s>\n", e.ID, esc(e.Name), esc(stroke), editorAttr)
	fmt.Fprintf(w, "<rect x=\"%s\" y=\"%s\" width=\"%s\" height=\"%s\" style=\"fill:%s;stroke:%s;%sstroke-width:%s\" />\n",
		model.FmtNum(x), model.FmtNum(y), model.FmtNum(width), model.FmtNum(height), esc(fill), esc(stroke), dash, model.FmtNum(strokeWidth))
	if e.PropertyText != "" {
		cx, cy := x+width/2, y+height/2
		rotate := ""
		if e.Orient != 0 {
			rotate = fmt.Sprintf(" transform=\"rotate(%d,%s,%s)\"", e.Orient, model.FmtNum(cx), model.FmtNum(cy))
		}
		style := fmt.Sprintf("fill:%s;text-anchor:middle;dominant-baseline:middle;font-size:%dpx;font-family:Arial", textColor, tableFontSize)
		fmt.Fprintf(w, "<text x=\"%s\" y=\"%s\" style=\"%s\"%s>%s</text>\n", model.FmtNum(cx), model.FmtNum(cy), style, rotate, esc(e.PropertyText))
	}
	fmt.Fprint(w, "</g>\n")
}

// tableCellFontSize is a Table2's (313) own per-cell text size — fixed,
// not per-instance, the same simplification tableFontSize/buttonFontSize
// already make.
const tableCellFontSize = 11

// table2DashPatterns maps a Table2's (313) own LineStyle to its real
// stroke-dasharray value, matching xsde2svg's own line-style switch
// (internal/modus/element_313.go's own single "пунктирная" case) — unlike
// every other LineStyle consumer, the real source only ever recognizes one
// dash variant for this shape, so LineStyleDashDot has no real value to
// resolve to and falls back to the same "" (solid) unset/unrecognized
// default LineStyleDotted already does for every consumer.
var table2DashPatterns = map[model.ConnectorLineStyle]string{
	model.LineStyleSolid:  "",
	model.LineStyleDashed: "stroke-dasharray: 3,2;",
}

// WriteTable2 draws a Table2 (shape 313) as a grid of individually-drawn
// bare <path> cells (each a closed rectangle, matching the real source's
// own canvas.Path M-h-v-h-v convention) inside a wrapping <g id
// data-type="313"> — see ClassTable2's own doc comment for why that
// wrapping <g> exists at all (this project's own request, since a real
// cell carries no id/grouping of its own). Cumulative sums of
// RowHeights/ColumnWidths (from the table's own X,Y top-left anchor) place
// each cell; a (Row, Col) with no entry in Cells at all, or one naming a
// row/column index out of range, is simply not drawn (see TableCell's own
// doc comment). Each cell's own Fill falls back to the table-wide Fill
// (itself falling back to "white", the real source's own default when
// neither a cell nor the table sets one); Stroke/StrokeWidth/LineStyle are
// shared by every cell (the real source's own rare per-cell override of
// stroke width/style isn't modeled). cell.Text, when set, draws centered
// in its own cell in cell.TextColor (falling back to black, the real
// source's own default). A Table2 with no RowHeights or no ColumnWidths
// draws nothing, the same "not enough geometry" treatment writeRectangle/
// writeTable give too few Points.
func WriteTable2(w io.Writer, e model.Element, mode RenderMode) {
	if len(e.RowHeights) == 0 || len(e.ColumnWidths) == 0 {
		return
	}

	stroke := e.Stroke
	if stroke == "" {
		stroke = "white"
	}
	strokeWidth := e.StrokeWidth
	if strokeWidth <= 0 {
		strokeWidth = 1
	}
	dash := table2DashPatterns[e.LineStyle]
	defaultFill := e.Fill
	if defaultFill == "" {
		defaultFill = "white"
	}

	// rowY[i]/colX[j] is row i's/column j's own top/left edge, relative to
	// the table's own X,Y anchor — cumulative sums of RowHeights/
	// ColumnWidths, one entry longer than each so a cell's own bottom/
	// right edge is always rowY[i+1]/colX[j+1] without a separate bounds
	// check.
	rowY := make([]float64, len(e.RowHeights)+1)
	for i, h := range e.RowHeights {
		rowY[i+1] = rowY[i] + h
	}
	colX := make([]float64, len(e.ColumnWidths)+1)
	for j, cw := range e.ColumnWidths {
		colX[j+1] = colX[j] + cw
	}

	editorAttr := ""
	if mode == Interactive {
		editorAttr = " data-editor-kind=\"element\""
	}
	fmt.Fprintf(w, "<g id=\"%d\" data-type=\"313\" data-name=\"%s\" data-voltage=\"%s\"%s>\n", e.ID, esc(e.Name), esc(stroke), editorAttr)
	for _, cell := range e.Cells {
		if cell.Row < 0 || cell.Row >= len(e.RowHeights) || cell.Col < 0 || cell.Col >= len(e.ColumnWidths) {
			continue
		}
		x := e.X + colX[cell.Col]
		y := e.Y + rowY[cell.Row]
		cw := colX[cell.Col+1] - colX[cell.Col]
		ch := rowY[cell.Row+1] - rowY[cell.Row]

		fill := cell.Fill
		if fill == "" {
			fill = defaultFill
		}
		style := fmt.Sprintf("fill:%s;stroke:%s;%sstroke-width:%s", fill, stroke, dash, model.FmtNum(strokeWidth))
		path := fmt.Sprintf("M %s %s h %s v %s h %s v %s ", model.FmtNum(x), model.FmtNum(y), model.FmtNum(cw), model.FmtNum(ch), model.FmtNum(-cw), model.FmtNum(-ch))
		fmt.Fprintf(w, "<path d=\"%s\" style=\"%s\" />\n", path, style)

		if cell.Text != "" {
			textColor := cell.TextColor
			if textColor == "" {
				textColor = "black"
			}
			textStyle := fmt.Sprintf("fill:%s;text-anchor:middle;dominant-baseline:middle;font-size:%dpx;font-family:Arial", textColor, tableCellFontSize)
			fmt.Fprintf(w, "<text x=\"%s\" y=\"%s\" style=\"%s\">%s</text>\n", model.FmtNum(x+cw/2), model.FmtNum(y+ch/2), textStyle, esc(cell.Text))
		}
	}
	fmt.Fprint(w, "</g>\n")
}

// writeRoad draws a Road (shape 335) as a single flat <polyline>, matching
// a real xsde2svg-exported one exactly (internal/modus/element_335.go's own
// canvas.Polyline call): no wrapping <g>, no data-name (unlike a busbar's
// own writePolyline call, a real Road instance never carries one). Stroke
// falls back to white, the same unset-color convention every other
// decorative annotation shape uses; StrokeWidth falls back to 8 rather than
// the generic 1 every other shape's own unset default is — a real Road is
// never actually drawn that thin (real corpus shows 8-12), so a thumbnail
// still reads as a road. A Road with fewer than 2 Points draws nothing,
// same as writeRectangle.
func writeRoad(w io.Writer, e model.Element, mode RenderMode) {
	if len(e.Points) < 2 {
		return
	}
	stroke := e.Stroke
	if stroke == "" {
		stroke = "white"
	}
	strokeWidth := e.StrokeWidth
	if strokeWidth <= 0 {
		strokeWidth = 8
	}
	dataAttrs := fmt.Sprintf(" data-voltage=\"%s\" data-type=\"335\"", esc(stroke))
	writePolyline(w, e.ID, "element", e.Points, stroke, false, strokeWidth, dataAttrs, mode)
}

// lineDashPatterns maps a Line's (shape 1) own LineStyle to its real
// stroke-dasharray value, matching xsde2svg's own line-style switch
// (internal/modus/element_1.go's own "штриховая"/"штрихпунктирная" cases)
// exactly — different literal numbers than a KindCableLine connector's own
// (cableLineDashPatterns): the two real sources don't share dash values
// just because this schema shares the enum type between them.
// LineStyleDotted has no real source counterpart for this shape (see
// ClassLine's own doc comment), so it's mapped the same as unset/solid
// rather than guessing at a value nothing real ever produces.
var lineDashPatterns = map[model.ConnectorLineStyle]string{
	model.LineStyleSolid:   "",
	model.LineStyleDashed:  "stroke-dasharray: 6,5;",
	model.LineStyleDashDot: "stroke-dasharray: 9 2 2 2;",
}

// writeLine draws a Line (shape 1) as a single flat <polyline>, matching a
// real xsde2svg-exported one exactly (internal/modus/element_1.go's own
// canvas.Polyline call): no wrapping <g>, no data-name (same gap writeRoad's
// own doc comment notes for Road). Stroke/StrokeWidth fall back to
// "black"/1, the real source's own defaults (writePolyline's own built-in
// "" -> "black" fallback covers Stroke; StrokeWidth is resolved here since
// writePolyline takes it as a plain already-resolved number). Doesn't reuse
// writePolyline for the dash portion — unlike its own generic bool
// "dashed" flag (a single fixed pattern, used by BusBarSection/Connector.
// Dashed), Line needs one of its own two real dash values, the same
// "resolve first, pass the real string in" approach writeNamedLine already
// uses for a KindCableLine connector's own dash. A Line with fewer than 2
// Points draws nothing, same as writeRoad.
func writeLine(w io.Writer, e model.Element, mode RenderMode) {
	if len(e.Points) < 2 {
		return
	}
	stroke := e.Stroke
	if stroke == "" {
		stroke = "black"
	}
	strokeWidth := e.StrokeWidth
	if strokeWidth <= 0 {
		strokeWidth = 1
	}
	dash := lineDashPatterns[e.LineStyle]
	var sb strings.Builder
	for i, p := range e.Points {
		if i > 0 {
			sb.WriteByte(' ')
		}
		sb.WriteString(model.FmtNum(p.X))
		sb.WriteByte(',')
		sb.WriteString(model.FmtNum(p.Y))
	}
	editorAttr := ""
	if mode == Interactive {
		editorAttr = " data-editor-kind=\"element\""
	}
	idAttr := ""
	if e.ID != 0 {
		idAttr = fmt.Sprintf(" id=\"%d\"", e.ID)
	}
	fmt.Fprintf(w, "<polyline points=\"%s\" style=\"fill:none;stroke:%s;%sstroke-width:%s\" data-voltage=\"%s\" data-type=\"1\"%s%s />\n",
		esc(sb.String()), esc(stroke), dash, model.FmtNum(strokeWidth), esc(stroke), idAttr, editorAttr)
}

// polygonDashPatterns maps a Polygon's (shape 16) own LineStyle to its real
// stroke-dasharray value, matching xsde2svg's own line-style switch
// (internal/modus/element_16.go's own "пунктирная"/"штрихпунктирная"
// cases) exactly — different values than Line's (lineDashPatterns). The
// real source has no plain dashed style for this shape, so
// LineStyleDashed draws solid, the same as unset.
var polygonDashPatterns = map[model.ConnectorLineStyle]string{
	model.LineStyleSolid:   "",
	model.LineStyleDotted:  "stroke-dasharray: 10,20;",
	model.LineStyleDashDot: "stroke-dasharray: 70 20 25 20;",
}

// writePolygon draws a Polygon (shape 16) as a single flat <polygon>,
// matching a real xsde2svg-exported one (internal/modus/element_16.go's
// own canvas.Polygon call): no wrapping <g>, no data-name, data-voltage
// carrying its own stroke color the same way writeLine's does. Fill falls
// back to "none", Stroke to "black" and StrokeWidth to 1. A Polygon with
// fewer than 3 Points draws nothing.
func writePolygon(w io.Writer, e model.Element, mode RenderMode) {
	if len(e.Points) < 3 {
		return
	}
	fill := e.Fill
	if fill == "" {
		fill = "none"
	}
	stroke := e.Stroke
	if stroke == "" {
		stroke = "black"
	}
	strokeWidth := e.StrokeWidth
	if strokeWidth <= 0 {
		strokeWidth = 1
	}
	dash := polygonDashPatterns[e.LineStyle]
	var sb strings.Builder
	for i, p := range e.Points {
		if i > 0 {
			sb.WriteByte(' ')
		}
		sb.WriteString(model.FmtNum(p.X))
		sb.WriteByte(',')
		sb.WriteString(model.FmtNum(p.Y))
	}
	editorAttr := ""
	if mode == Interactive {
		editorAttr = " data-editor-kind=\"element\""
	}
	idAttr := ""
	if e.ID != 0 {
		idAttr = fmt.Sprintf(" id=\"%d\"", e.ID)
	}
	fmt.Fprintf(w, "<polygon points=\"%s\" style=\"fill:%s;stroke:%s;%sstroke-width:%s\"%s data-type=\"16\" data-voltage=\"%s\"%s />\n",
		esc(sb.String()), esc(fill), esc(stroke), dash, model.FmtNum(strokeWidth), idAttr, esc(stroke), editorAttr)
}

// arcRotation is the fixed x-axis rotation (degrees) the real source
// writes into every arc command (element_9.go passes a constant 1).
const arcRotation = 1

// WriteArc draws an Arc (shape 9) as a single flat <path d="M x,y A rx,ry
// 1 large sweep x,y">, matching a real xsde2svg-exported one
// (internal/modus/element_9.go's own canvas.Arc call): no wrapping <g>, no
// data-name. Stroke falls back to "black" and StrokeWidth to 0.25 (the real
// source's own default Width of 1, drawn at a quarter). Unlike the real
// source, which never gives an arc an id, this writes one whenever the
// element has one, so it stays individually selectable. An Arc with fewer
// than 2 Points draws nothing.
func WriteArc(w io.Writer, e model.Element, mode RenderMode) {
	if len(e.Points) < 2 {
		return
	}
	stroke := e.Stroke
	if stroke == "" {
		stroke = "black"
	}
	strokeWidth := e.StrokeWidth
	if strokeWidth <= 0 {
		strokeWidth = 0.25
	}
	flag := func(b bool) string {
		if b {
			return "1"
		}
		return "0"
	}
	s, t := e.Points[0], e.Points[1]
	d := fmt.Sprintf("M%s,%s A%s,%s %d %s %s %s,%s", model.FmtNum(s.X), model.FmtNum(s.Y), model.FmtNum(e.RadiusX), model.FmtNum(e.RadiusY),
		arcRotation, flag(e.LargeArc), flag(e.Sweep), model.FmtNum(t.X), model.FmtNum(t.Y))
	editorAttr := ""
	if mode == Interactive {
		editorAttr = " data-editor-kind=\"element\""
	}
	idAttr := ""
	if e.ID != 0 {
		idAttr = fmt.Sprintf(" id=\"%d\"", e.ID)
	}
	fmt.Fprintf(w, "<path d=\"%s\" style=\"fill:none;stroke:%s;stroke-width:%s\"%s data-type=\"9\" data-voltage=\"%s\"%s />\n",
		esc(d), esc(stroke), model.FmtNum(strokeWidth), idAttr, esc(stroke), editorAttr)
}

// polePostRadius is the drawn radius/half-width a PostPole (shape 292)
// falls back to when Radius is unset — matching the real source's own
// fixed Scale(scaleChosed, 10) default (internal/modus/element_292.go),
// shared by both its round and square variants.
const polePostRadius = 10

// writePole draws a PostPole (shape 292) as a single flat <rect> or
// <circle> depending on Square, matching a real xsde2svg-exported one
// exactly: no wrapping <g>, no data-name (element_292.go never computes
// one, the same gap writeRoad's own doc comment notes for Road). Fill
// falls back to "none", the same convention every other decorative
// annotation shape uses; Stroke falls back to "gray" rather than
// Rectangle/Arrow/Button's own "white" — the real corpus's own dominant
// color for this shape specifically. StrokeWidth is always 1, matching
// the real source's own hardcoded value (unlike Rectangle/Road, no real
// corpus variance to justify a field for it). Orient rotates the drawn
// shape around its own anchor (X,Y) when set, mirroring the real source's
// own rotate(angle,x,y) exactly — visually inert either way (see
// ClassPostPole's own doc comment) but still round-tripped for fidelity
// with a real instance that carries one.
func writePole(w io.Writer, e model.Element, mode RenderMode) {
	fill := e.Fill
	if fill == "" {
		fill = "none"
	}
	stroke := e.Stroke
	if stroke == "" {
		stroke = "gray"
	}
	radius := e.Radius
	if radius <= 0 {
		radius = polePostRadius
	}
	rotate := ""
	if e.Orient != 0 {
		rotate = fmt.Sprintf(" transform=\"rotate(%d,%s,%s)\"", e.Orient, model.FmtNum(e.X), model.FmtNum(e.Y))
	}
	editorAttr := ""
	if mode == Interactive {
		editorAttr = " data-editor-kind=\"element\""
	}
	style := fmt.Sprintf("fill:%s;stroke:%s;stroke-width:1", esc(fill), esc(stroke))
	if e.Square {
		side := radius * 2
		fmt.Fprintf(w, "<rect id=\"%d\" x=\"%s\" y=\"%s\" width=\"%s\" height=\"%s\" style=\"%s\" data-voltage=\"%s\" data-type=\"292\"%s%s />\n",
			e.ID, model.FmtNum(e.X-radius), model.FmtNum(e.Y-radius), model.FmtNum(side), model.FmtNum(side), style, esc(stroke), rotate, editorAttr)
		return
	}
	fmt.Fprintf(w, "<circle id=\"%d\" cx=\"%s\" cy=\"%s\" r=\"%s\" style=\"%s\" data-voltage=\"%s\" data-type=\"292\"%s%s />\n",
		e.ID, model.FmtNum(e.X), model.FmtNum(e.Y), model.FmtNum(radius), style, esc(stroke), rotate, editorAttr)
}

// powerflowGlyph picks a PowerflowIndicator's (shape 320001) own arrow
// character from State, matching the real source's own `FState != "0"`
// check exactly: nil/0 draws "→", anything else draws "←".
// powerflowTextLegend is a PowerflowIndicator's data-text: the glyph per live state, so
// rdss-dms can turn the arrow (powerflowGlyph's own mapping).
const powerflowTextLegend = "0:→,1:←"

func powerflowGlyph(state *int) string {
	if state != nil && *state != 0 {
		return "←"
	}
	return "→"
}

// writePowerflowIndicator draws a PowerflowIndicator (shape 320001) as a
// single flat <text>, matching a real xsde2svg-exported one exactly
// (internal/modus/element_320.go's own "Направление перетока" case): no
// wrapping <g>, no data-name (same gap writeRoad's own doc comment notes
// for Road). TextColor falls back to "black" (see that field's own doc
// comment for why this differs from Button's own "white" default).
// Font size/vertical shift are fixed (26/3), matching the real source's
// own values at its default diagram scale — real corpus shows this only
// varying *between* diagrams, not per instance (see ClassPowerflowIndicator's
// own doc comment). Unlike writePole's own conditional rotate, the real
// source always emits the rotate() transform (even for angle 0 — confirmed
// against real corpus, e.g. `transform="rotate(0,1530,1200)"`), so this
// does too, along with the real data-angle attribute Extract reads back.
func writePowerflowIndicator(w io.Writer, e model.Element, mode RenderMode) {
	color := e.TextColor
	if color == "" {
		color = "black"
	}
	editorAttr := ""
	if mode == Interactive {
		editorAttr = " data-editor-kind=\"element\""
	}
	idAttr := ""
	if e.ID != 0 {
		idAttr = fmt.Sprintf(" id=\"%d\"", e.ID)
	}
	fmt.Fprintf(w, "<text x=\"%s\" y=\"%s\" style=\"font-size:26;fill:%s;font-weight: bold\" transform=\"rotate(%d,%s,%s)\"%s data-type=\"320001\" data-angle=\"%d\" data-voltage=\"%s\" data-text=\"%s\"%s>%s</text>\n",
		model.FmtNum(e.X), model.FmtNum(e.Y+3), esc(color), e.Orient, model.FmtNum(e.X), model.FmtNum(e.Y), idAttr, e.Orient, esc(color), powerflowTextLegend, editorAttr, powerflowGlyph(e.State))
}

// writePackageSubstation draws a PackageSubstation (shape 385) — a
// facility-level pictogram, not switchgear with real terminals in the
// usual sense, but the real xsde2svg source does give it exactly one real
// electrical connection point (see base.xml's own Terminals entry for
// this shape), so unlike ClassRectangle/ClassArrow/ClassCircle it still
// resolves a genuine voltage-driven {color} (the color argument, computed
// by the caller the same way every ordinary equipment class's own does)
// and uses the standard translate(X,Y) rotate(Orient) mirrorScale(Mirror)
// transform every anchor-based symbol uses, rather than an absolute-Points
// one. Two independent, per-instance visual traits, confirmed against the
// real source (internal/modus/element_385.go, element385) and directly
// against a real xsde2svg v1.4.12 corpus export: NType selects between this
// shape's own two real appearance variants — 0/unset (the common case)
// draws a 36-unit outer square, an 18-unit-wide rectangle centered inside
// it, and a short lead stub above the top edge (the real terminal's own
// local position, (0,-20), 2 units above the box on the 10-grid where
// the source draws 4); 1 draws a plain flat-topped downward-pointing
// triangle instead, its own apex at local (0,18) — not at the anchor
// itself, matching the box variant's own center-of-bounding-box anchor
// semantics exactly, confirmed against that same real corpus file (no
// separate lead stub drawn for this variant, but the same terminal
// position still applies either way — both variants' own top edge sits
// at the identical local y=-18, so nothing needs to shift to keep the
// terminal meaningful across both). NType is only recoverable on Extract
// via a data-ntype export attribute that real v1.4.12 corpus file already
// carries (confirming this schema's own choice of attribute name matches
// a real, already-deployed convention) but this repo's own local
// xsde2svg checkout — an older version — didn't yet have; added there
// too (element_385.go) so an Extract round-trip through *this* repo's own
// tooling has something to read. The real source's own Abonent flag
// (fills the inner rectangle solid instead of outline-only) is folded
// into this schema's ordinary Fill field instead of a dedicated boolean —
// the same free-choice-color convention ClassRectangle/ClassCircle
// already use — rather than being locked to {color} the way the real
// source's own on/off flag is. The real source's own Tech.Closed (a
// dashed outline when "0") reuses this schema's ordinary State field:
// unset/1 (nil defaults to the first option, matching applyStateLine's
// own convention elsewhere) draws a solid outline, 0 dashes it — this
// shape has no third state, so it isn't otherwise a real switching
// device and carries no state-color legend of its own. Both NType variants
// are drawn inside the same single <g id data-type data-ntype ... transform>
// wrapper (real corpus once had NType 1 as a bare, unwrapped <path> in an
// older xsde2svg version, but every currently-deployed real instance found
// wraps it exactly like NType 0 now) — see writeSubstationPropertyText for
// PropertyText's own optional overlay label, drawn as a sibling inside that
// same <g>.
func writePackageSubstation(w io.Writer, e model.Element, color string, mode RenderMode) {
	fill := e.Fill
	if fill == "" {
		fill = "none"
	}
	dash := ""
	if e.State != nil && *e.State == 0 {
		dash = "stroke-dasharray:6,5;"
	}
	editorAttr := ""
	if mode == Interactive {
		editorAttr = " data-editor-kind=\"element\""
	}
	transform := fmt.Sprintf("translate(%s,%s) rotate(%d)%s", model.FmtNum(e.X), model.FmtNum(e.Y), e.Orient, mirrorScale(e.Mirror))
	ntypeAttr := fmt.Sprintf(" data-ntype=\"%d\"", e.NType)

	fmt.Fprintf(w, "<g id=\"%d\" data-name=\"%s\" data-voltage=\"%s\" data-type=\"385\"%s%s transform=\"%s\">\n",
		e.ID, esc(e.Name), esc(color), ntypeAttr, editorAttr, transform)
	if e.NType == 1 {
		fmt.Fprintf(w, "<path d=\"M -18 -18 L 18 -18 L 0 18 Z\" style=\"fill:%s;stroke:%s;%sstroke-width:1\" />\n", esc(fill), esc(color), dash)
	} else {
		fmt.Fprintf(w, "<rect x=\"-18\" y=\"-18\" width=\"36\" height=\"36\" style=\"fill:none;stroke:%s;%sstroke-width:1\" />\n", esc(color), dash)
		fmt.Fprintf(w, "<rect x=\"-9\" y=\"-18\" width=\"18\" height=\"36\" style=\"fill:%s;stroke:%s;%sstroke-width:1\" />\n", esc(fill), esc(color), dash)
		fmt.Fprintf(w, "<line x1=\"0\" y1=\"-18\" x2=\"0\" y2=\"-20\" style=\"stroke:%s;%sstroke-width:1\" />\n", esc(color), dash)
	}
	writeSubstationPropertyText(w, e)
	fmt.Fprint(w, "</g>\n")
}

// substationRadius is a Substation's circle radius: Element.Radius, or the
// source's 20 when unset.
func substationRadius(e model.Element) float64 {
	if e.Radius > 0 {
		return e.Radius
	}
	return 20
}

// writeSubstation draws a Substation (shape 360) as element_360.go does, in
// its local frame around the center: one filled path per sector (Sectors,
// 1–4; none counts as one), outlined in color. 1 is a whole circle, 2 the
// right and left halves, 3 upper-right, bottom and upper-left, 4 the
// quadrants clockwise from upper-right, the 3-sector split points scaled
// from the source's 19/27/7 at radius 20. A sector without a voltage is
// filled with color. The source's xMirror reverses the sector colors, which
// is exactly the geometric flip Mirror applies.
func writeSubstation(w io.Writer, e model.Element, voltageColor map[int]string, color string, mode RenderMode) {
	r := substationRadius(e)
	n := min(max(len(e.Sectors), 1), 4)
	fills := make([]string, n)
	for i := range fills {
		fills[i] = color
		if i < len(e.Sectors) {
			if c := voltageColor[e.Sectors[i].Voltage]; c != "" {
				fills[i] = c
			}
		}
	}
	f := model.FmtNum
	l31, l32, l33 := r*19/20, r*27/20, r*7/20
	var paths []string
	switch n {
	case 1:
		paths = []string{fmt.Sprintf("M 0 %s a %s %s 0 1 1 0 %s a %s %s 0 0 1 0 %s", f(r), f(r), f(r), f(-2*r), f(r), f(r), f(2*r))}
	case 2:
		paths = []string{
			fmt.Sprintf("M 0 0 v %s a %s %s 0 0 0 0 %s z", f(r), f(r), f(r), f(-2*r)),
			fmt.Sprintf("M 0 0 v %s a %s %s 0 0 0 0 %s z", f(-r), f(r), f(r), f(2*r)),
		}
	case 3:
		paths = []string{
			fmt.Sprintf("M 0 0 v %s a %s %s 0 0 1 %s %s z", f(-r), f(r), f(r), f(l31), f(l32)),
			fmt.Sprintf("M 0 0 l %s %s a %s %s 0 0 1 %s 0 z", f(l31), f(l33), f(r), f(r), f(-2*l31)),
			fmt.Sprintf("M 0 0 l %s %s a %s %s 0 0 1 %s %s z", f(-l31), f(l33), f(r), f(r), f(l31), f(-l32)),
		}
	case 4:
		paths = []string{
			fmt.Sprintf("M 0 0 v %s a %s %s 0 0 1 %s %s z", f(-r), f(r), f(r), f(r), f(r)),
			fmt.Sprintf("M 0 0 v %s a %s %s 0 0 0 %s %s z", f(r), f(r), f(r), f(r), f(-r)),
			fmt.Sprintf("M 0 0 h %s a %s %s 0 0 0 %s %s z", f(-r), f(r), f(r), f(r), f(r)),
			fmt.Sprintf("M 0 0 h %s a %s %s 0 0 1 %s %s z", f(-r), f(r), f(r), f(r), f(-r)),
		}
	}
	editorAttr := ""
	if mode == Interactive {
		editorAttr = " data-editor-kind=\"element\""
	}
	transform := fmt.Sprintf("translate(%s,%s) rotate(%d)%s", model.FmtNum(e.X), model.FmtNum(e.Y), e.Orient, mirrorScale(e.Mirror))
	fmt.Fprintf(w, "<g id=\"%d\" data-name=\"%s\" data-voltage=\"%s\" data-type=\"360\"%s transform=\"%s\">\n",
		e.ID, esc(e.Name), esc(color), editorAttr, transform)
	for i, d := range paths {
		fmt.Fprintf(w, "<path d=\"%s\" style=\"fill:%s;stroke:%s;stroke-width:1\" />\n", d, esc(fills[i]), esc(color))
	}
	fmt.Fprint(w, "</g>\n")
}

// powerPlantHatch is the source's hatching pitch: its <pattern> tiles are
// 10 units, whatever the export scale.
const powerPlantHatch = 10

// writePowerPlant draws a PowerPlant (shape 38) as element_38.go does, in
// its local frame around the center with half side L (Radius, default 20):
// first the hatched part's outline, then the other part's (exactly the
// source's two paths, so Extract reads both kinds back), then the hatching.
// Thermal (NType 0): the lower half hatched with "/" lines, the upper half
// outlined. Hydro (NType 1): the upper-left triangle hatched with
// horizontal lines, the lower-right one an open outline; the source's
// xMirror (the other diagonal) is exactly the flip Mirror applies. The
// source hatches through a shared <pattern id="diagonal|horizontal">
// written again for every instance, so in a browser every hatch takes the
// first instance's color; here each instance draws its own lines, in its
// own color, at the same 10-unit pitch.
func writePowerPlant(w io.Writer, e model.Element, color string, mode RenderMode) {
	l := substationRadius(e)
	f := model.FmtNum
	var hatched, other string
	var hatch strings.Builder
	if e.NType == 1 {
		hatched = fmt.Sprintf("M %s %s h %s v %s z", f(l), f(-l), f(-2*l), f(2*l))
		other = fmt.Sprintf("M %s %s h %s v %s", f(-l), f(l), f(2*l), f(-2*l))
		// Horizontal lines across the triangle x >= -l, y >= -l, x+y <= 0.
		for y := -l + powerPlantHatch; y < l; y += powerPlantHatch {
			fmt.Fprintf(&hatch, "M %s %s H %s ", f(-l), f(y), f(-y))
		}
	} else {
		hatched = fmt.Sprintf("M %s 0 v %s h %s v %s z", f(-l), f(l), f(2*l), f(-l))
		other = fmt.Sprintf("M %s 0 v %s h %s v %s z", f(-l), f(-l), f(2*l), f(l))
		// "/" lines x+y = c across the lower half x in [-l,l], y in [0,l].
		x0, x1, y0, y1 := -l, l, 0.0, l
		for c := math.Ceil((x0+y0)/powerPlantHatch) * powerPlantHatch; c < x1+y1; c += powerPlantHatch {
			lo := math.Max(y0, c-x1)
			hi := math.Min(y1, c-x0)
			if hi-lo > 1e-9 {
				fmt.Fprintf(&hatch, "M %s %s L %s %s ", f(c-lo), f(lo), f(c-hi), f(hi))
			}
		}
	}
	editorAttr := ""
	if mode == Interactive {
		editorAttr = " data-editor-kind=\"element\""
	}
	transform := fmt.Sprintf("translate(%s,%s) rotate(%d)%s", model.FmtNum(e.X), model.FmtNum(e.Y), e.Orient, mirrorScale(e.Mirror))
	fmt.Fprintf(w, "<g id=\"%d\" data-name=\"%s\" data-voltage=\"%s\" data-type=\"38\"%s transform=\"%s\">\n",
		e.ID, esc(e.Name), esc(color), editorAttr, transform)
	style := fmt.Sprintf("fill:none;stroke:%s;stroke-width:1", esc(color))
	fmt.Fprintf(w, "<path d=\"%s\" style=\"%s\" />\n", hatched, style)
	fmt.Fprintf(w, "<path d=\"%s\" style=\"%s\" />\n", other, style)
	if hatch.Len() > 0 {
		fmt.Fprintf(w, "<path d=\"%s\" style=\"%s\" />\n", strings.TrimSpace(hatch.String()), style)
	}
	fmt.Fprint(w, "</g>\n")
}

// writeSubstationPropertyText draws PackageSubstation's/EnclosedSubstation's
// own optional overlay label (Element.PropertyText) as a sibling of their
// drawn geometry, inside the same single combined-transform <g> both
// writePackageSubstation/writeEnclosedSubstation wrap everything in. The
// real source keeps this text out of its own separate rotate() group
// entirely so it stays upright and centered on the shape regardless of
// Orient/Mirror — confirmed against real corpus instances carrying both a
// non-zero rotation and a label at once (e.g. a 385 rotated -90° with its
// own text still drawn unrotated at the same anchor). This schema instead
// gives the <text> its own local counter-transform (undoing the parent
// <g>'s Orient rotation and Mirror flip) to the same visual effect, rather
// than splitting the parent into nested rotate/non-rotate groups the way
// the real source does — every other symbol element already relies on its
// own single combined transform for Canvas.tsx's own getBBox()-based
// click-tolerance box and drag-in-DOM logic, and restructuring that here
// would break both for just these two shapes.
func writeSubstationPropertyText(w io.Writer, e model.Element) {
	if e.PropertyText == "" {
		return
	}
	var parts []string
	if e.Mirror {
		parts = append(parts, "scale(-1,1)")
	}
	if e.Orient != 0 {
		parts = append(parts, fmt.Sprintf("rotate(%d)", -e.Orient))
	}
	textTransform := ""
	if len(parts) > 0 {
		textTransform = fmt.Sprintf(" transform=\"%s\"", strings.Join(parts, " "))
	}
	fmt.Fprintf(w, "<text x=\"0\" y=\"0\"%s style=\"fill:white;text-anchor:middle;alignment-baseline:middle;font-size:17px;font-family:Arial\">%s</text>\n",
		textTransform, esc(e.PropertyText))
}

// writeEnclosedSubstation draws an EnclosedSubstation (shape 386) — the
// same kind of facility-level pictogram as PackageSubstation, and drawn
// with the exact same free-choice Fill (the real source's own Abonent
// flag) and dashed-when-0 State (the real source's own Tech.Closed) this
// package's writePackageSubstation already uses, but with a single fixed
// appearance instead of two real variants: a 36-unit square outline
// always drawn together with a downward-pointing triangle inside it (its
// own apex at local (0,18), the exact same real formula
// writePackageSubstation's own NType==1 triangle uses — confirmed against
// the same real xsde2svg v1.4.12 corpus export that shape's own doc
// comment cites; this repo's own local xsde2svg checkout again predates
// this — its own element_id386.go has a stale apex-at-the-anchor
// formula). Unlike PackageSubstation, the real source draws no separate
// lead stub for this shape's own single real electrical terminal — its
// local position, (0,-20), was confirmed directly by the user rather
// than from any drawn geometry.
func writeEnclosedSubstation(w io.Writer, e model.Element, color string, mode RenderMode) {
	fill := e.Fill
	if fill == "" {
		fill = "none"
	}
	dash := ""
	if e.State != nil && *e.State == 0 {
		dash = "stroke-dasharray:6,5;"
	}
	editorAttr := ""
	if mode == Interactive {
		editorAttr = " data-editor-kind=\"element\""
	}
	transform := fmt.Sprintf("translate(%s,%s) rotate(%d)%s", model.FmtNum(e.X), model.FmtNum(e.Y), e.Orient, mirrorScale(e.Mirror))

	fmt.Fprintf(w, "<g id=\"%d\" data-name=\"%s\" data-voltage=\"%s\" data-type=\"386\"%s transform=\"%s\">\n",
		e.ID, esc(e.Name), esc(color), editorAttr, transform)
	fmt.Fprintf(w, "<rect x=\"-18\" y=\"-18\" width=\"36\" height=\"36\" style=\"fill:none;stroke:%s;%sstroke-width:1\" />\n", esc(color), dash)
	fmt.Fprintf(w, "<path d=\"M -18 -18 L 18 -18 L 0 18 Z\" style=\"fill:%s;stroke:%s;%sstroke-width:1\" />\n", esc(fill), esc(color), dash)
	writeSubstationPropertyText(w, e)
	fmt.Fprint(w, "</g>\n")
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
	return fmt.Sprintf(" l %s -%s m 0 %s l %s -%s", model.FmtNum(firstX), model.FmtNum(l3), model.FmtNum(l3*2), model.FmtNum(-firstX), model.FmtNum(l3))
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
func writeArrow(w io.Writer, e model.Element, mode RenderMode) {
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
	d += fmt.Sprintf(" h %s", model.FmtNum(length)) + arrowChevron(true)

	editorAttr := ""
	if mode == Interactive {
		editorAttr = " data-editor-kind=\"element\""
	}
	fmt.Fprintf(w, "<path id=\"%d\" d=\"%s\" style=\"fill:none;stroke:%s;stroke-width:%s\" data-name=\"%s\" data-voltage=\"%s\" data-type=\"2\" transform=\"translate(%s,%s) rotate(%s)\"%s />\n",
		e.ID, d, esc(stroke), model.FmtNum(strokeWidth), esc(e.Name), esc(stroke), model.FmtNum(p0.X), model.FmtNum(p0.Y), model.FmtNum(angle), editorAttr)
}

// writeNamedLine draws a KindOverheadLine/KindCableLine connector as a
// real xsde2svg-style <g id data-type data-name data-voltage> wrapping
// its own polyline — unlike every other connector kind (and a busbar),
// which writePolyline renders as a single flat polyline with no wrapping
// <g>, and no data-name, at all. Overhead line confirmed against both a
// real corpus file (id="302" data-name="Line2") and a user-supplied example matching it exactly;
// cable line given the same <g>-wrapping treatment for consistency
// between the two "named" kinds, since both are meant to carry a real
// identity (Connector.Name) a plain wire never does — though its own
// stroke defaults to dashed rather than solid (see
// resolveCableLineDash/Connector.LineStyle). code is
// connectorTypeCode[c.Kind], passed in rather than looked up again since
// the caller already has it from its own typeComment call.
func writeNamedLine(w io.Writer, c model.Connector, color string, code string, mode RenderMode) {
	if color == "" {
		color = "black"
	}
	var sb strings.Builder
	for i, p := range c.Points {
		if i > 0 {
			sb.WriteByte(' ')
		}
		sb.WriteString(model.FmtNum(p.X))
		sb.WriteByte(',')
		sb.WriteString(model.FmtNum(p.Y))
	}
	dash := ""
	if c.Kind == model.KindCableLine {
		dash = resolveCableLineDash(c.LineStyle)
	} else if c.Dashed {
		dash = "stroke-dasharray: 14,9;"
	}
	editorAttr := ""
	if mode == Interactive {
		editorAttr = " data-editor-kind=\"connector\""
	}
	fmt.Fprintf(w, "<g id=\"%d\" data-type=\"%s\" data-name=\"%s\" data-voltage=\"%s\"%s>\n<polyline points=\"%s\" style=\"fill:none;stroke:%s;%sstroke-width:%s\" />\n</g>\n",
		c.ID, esc(code), esc(c.Name), esc(color), editorAttr, esc(sb.String()), esc(color), dash, model.FmtNum(namedLineStrokeWidth))
}

// objectLinkStrokeWidth is a KindLinkToObject connector's own fixed stroke
// width — heavier than an ordinary wire's 1px, confirmed against a real
// xsde2svg-exported instance exactly.
const objectLinkStrokeWidth = 2

// writeObjectLink draws a KindLinkToObject connector (shape 28,
// "Связь с объектом"/"Object link" in the xsde2svg catalog): one
// <g id data-type="28" [data-name] data-voltage> wrapping the line (a bare
// <polyline> at objectLinkStrokeWidth, no attributes of its own beyond its
// style) and a triangular arrowhead <path> at the connector's final point,
// rotated to continue the direction its last segment travels. The group
// makes the pair one item, like an overhead/cable line's <g>: the canvas
// hit-tests and selects the arrowhead too, a fragment patch or delete
// replaces both, and data-layer (withItemAttrs, the root node) hides both.
// xsde2svg (internal/modus/element_28.go) writes the same group since
// 1.4.21; older exports draw the two as flat siblings, which Extract still
// reads (parseConnector takes a <g>'s first <polyline> or a bare one).
//
// The arrowhead's local geometry (base corners at local (±7,0), apex at
// local (0,12), "pointing south" before rotation) and its rotate() angle,
// atan2(-dx,dy) (a straight-up final segment gives 180, as real instances
// read), are reverse-engineered from a real instance. It is placed with
// translate(x,y) rotate(angle) and local path data like every symbol
// template; Static's absolute pass rewrites that into xsde2svg's
// rotate(angle,cx,cy) around absolute coordinates. (A bare rotate around
// an arbitrary point is not equivalent with local coordinates: an earlier
// version paired the two and drew the arrowhead thousands of units off.)
func writeObjectLink(w io.Writer, c model.Connector, color, code string, mode RenderMode) {
	if color == "" {
		color = "black"
	}
	nameAttr := ""
	if c.Name != "" {
		nameAttr = fmt.Sprintf(" data-name=\"%s\"", esc(c.Name))
	}
	editorAttr := ""
	if mode == Interactive {
		editorAttr = " data-editor-kind=\"connector\""
	}
	fmt.Fprintf(w, "<g id=\"%d\" data-type=\"%s\"%s data-voltage=\"%s\"%s>\n", c.ID, esc(code), nameAttr, esc(color), editorAttr)
	// id 0: the group carries the id, so the polyline writes none.
	writePolyline(w, 0, "connector", c.Points, color, c.Dashed, objectLinkStrokeWidth, "", mode)
	if mode == Static {
		// absolutize rewrites a top-level fragment, not one nested in this
		// group, so the arrowhead is converted on its own.
		var arrow bytes.Buffer
		writeObjectLinkArrow(&arrow, c.Points, color)
		writeAbsolute(w, arrow.String())
	} else {
		writeObjectLinkArrow(w, c.Points, color)
	}
	io.WriteString(w, "</g>\n")
}

// writeObjectLinkArrow is writeObjectLink's arrowhead at the last of pts;
// nothing for fewer than two points or a zero-length final segment.
func writeObjectLinkArrow(w io.Writer, pts []model.Point, color string) {
	if len(pts) < 2 {
		return
	}
	from := pts[len(pts)-2]
	to := pts[len(pts)-1]
	if from.X == to.X && from.Y == to.Y {
		return
	}
	// from.X-to.X, not -(to.X-from.X): IEEE754 gives a-a exactly +0 for any
	// finite a, whereas negating a +0 difference yields -0, which flips
	// atan2's own result by a full turn (180 here becomes -180) — cosmetic
	// only (they're the same rotation), but real corpus instances always
	// read "180", not "-180", for a straight-up final segment.
	angle := math.Atan2(from.X-to.X, to.Y-from.Y) * 180 / math.Pi
	fmt.Fprintf(w, "<path d=\"M 7 0 l -7 12 l -7 -12 z\" style=\"fill:none;stroke:%s;stroke-width:%s\" transform=\"translate(%s,%s) rotate(%s)\" />\n",
		esc(color), model.FmtNum(objectLinkStrokeWidth), model.FmtNum(to.X), model.FmtNum(to.Y), model.FmtNum(angle))
}

// Power transformer (shape 47) geometry constants, transcribed from real
// xsde2svg's own element_47.go default (sde.Size==0) path — this editor
// doesn't offer that source's own Size 11-24 magic lookup table (a set of
// named presets overriding these same numbers for cosmetic leg-length
// tuning, not exposed anywhere in this editor's own Properties), so every
// PowerTransformer always uses the one true default set below.
const (
	TransformerRadius     = 22                    // baseRadius
	transformerGridShift  = 20                    // 2-/3-winding side and 4-winding bottom-circle offset: xsde2svg's int(baseRadius/1.2)=18, widened onto the grid
	transformerTopShift   = 30                    // 3-winding top-circle offset: xsde2svg's int(baseRadius*1.16)=25, widened onto the grid
	transformerSideShift  = 30                    // 4-winding left/right-circle offset: xsde2svg's 29, widened onto the grid
	transformerVertShift  = 20                    // 4-winding top-circle offset
	transformerGlyphShift = TransformerRadius / 3 // wye/wyeN spoke length (int division, matching element_47.go exactly)
	transformerArrowSize  = 20
	transformerArrowShift = 5
)

// TransformerLeadLength is every winding's lead length past its circle
// edge. Every circle offset (transformerWindingOffset) is a multiple of 10
// — xsde2svg's own 18, 25 and 29 widened to 20, 30 and 30 — so with the
// radius of 22 every lead tip lands on the editor's default 10-unit grid,
// whichever direction the winding is led. Without this, a freshly placed
// transformer's own anchor lands on-grid but its lead tip doesn't, forcing
// a short non-orthogonal jog into an otherwise-orthogonal routed wire (see
// this package's RELEASE.md).
const TransformerLeadLength = 8

// Xsde2svgWindingZeroOffset is winding 0's circle-center offset as real
// xsde2svg draws it (element_47.go), before this editor widened the offsets
// onto the grid. Extract uses it to recover an unrotated transformer's
// anchor from a real exported file's first circle.
func Xsde2svgWindingZeroOffset(count int) (float64, float64) {
	switch count {
	case 2:
		return 18, 0
	case 3:
		return 0, -25
	case 4:
		return 0, -20
	}
	return 0, 0
}

// TransformerWindingOffset returns real winding index i's own local
// (pre-rotation) circle-center offset from the transformer's own anchor,
// for a non-autotransformer with the given real winding count —
// transcribed from element_47.go's own default (non-auto) case, confirmed
// against real xsde2svg-exported corpus markup (both the default-scale
// 3-winding case and a fractional-scale 4-winding one, whose proportions
// matched this function's own constants exactly once the scale factor was
// divided back out).
func TransformerWindingOffset(count, i int) (float64, float64) {
	switch count {
	case 2:
		if i == 0 {
			return transformerGridShift, 0
		}
		return -transformerGridShift, 0
	case 3:
		switch i {
		case 0:
			return 0, -transformerTopShift
		case 1:
			return transformerGridShift, 0
		default:
			return -transformerGridShift, 0
		}
	case 4:
		switch i {
		case 0:
			return 0, -transformerVertShift
		case 1:
			return 0, transformerGridShift
		case 2:
			return -transformerSideShift, 0
		default:
			return transformerSideShift, 0
		}
	}
	return 0, 0
}

// DefaultTerminal returns real winding index i's own conventional lead
// direction for a transformer with the given real winding count, used
// when that winding's own Terminal field is empty — matches
// element_47.go's own default (Chassis-unset) leg direction for each
// position exactly (a 2-winding transformer's default side-by-side
// horizontal layout, a 3-winding one's top/right/left, ...).
func DefaultTerminal(count, i int) model.TerminalDirection {
	switch count {
	case 2:
		if i == 0 {
			return model.TerminalRight
		}
		return model.TerminalLeft
	case 3:
		switch i {
		case 0:
			return model.TerminalTop
		case 1:
			return model.TerminalRight
		default:
			return model.TerminalLeft
		}
	case 4:
		switch i {
		case 0:
			return model.TerminalTop
		case 1:
			return model.TerminalBottom
		case 2:
			return model.TerminalLeft
		default:
			return model.TerminalRight
		}
	}
	return model.TerminalRight
}

// TransformerLegEndpoint returns a winding's own lead tip — its real
// electrical terminal, matching parsePowerTransformer's own "final point
// of a two-point lead path" extraction convention — given its own circle
// center, lead direction, and lead length (transformerLeadLength).
func TransformerLegEndpoint(cx, cy float64, dir model.TerminalDirection, legLen float64) (float64, float64) {
	switch dir {
	case model.TerminalTop:
		return cx, cy - TransformerRadius - legLen
	case model.TerminalBottom:
		return cx, cy + TransformerRadius + legLen
	case model.TerminalLeft:
		return cx - TransformerRadius - legLen, cy
	default: // TerminalRight
		return cx + TransformerRadius + legLen, cy
	}
}

// writeTransformerLeg draws a winding's own straight lead from its
// circle's own edge to its lead tip (transformerLegEndpoint).
func writeTransformerLeg(w io.Writer, cx, cy float64, dir model.TerminalDirection, legLen float64, color string) {
	ex, ey := TransformerLegEndpoint(cx, cy, dir, legLen)
	var sx, sy float64
	switch dir {
	case model.TerminalTop:
		sx, sy = cx, cy-TransformerRadius
	case model.TerminalBottom:
		sx, sy = cx, cy+TransformerRadius
	case model.TerminalLeft:
		sx, sy = cx-TransformerRadius, cy
	default:
		sx, sy = cx+TransformerRadius, cy
	}
	fmt.Fprintf(w, "<path d=\"M %s %s L %s %s\" style=\"fill:none;stroke:%s;stroke-width:2\" data-voltage=\"%s\" />\n",
		model.FmtNum(sx), model.FmtNum(sy), model.FmtNum(ex), model.FmtNum(ey), esc(color), esc(color))
}

// writeWindingGlyph draws a winding's own connection-scheme mark at its
// own circle center — the small inner symbol distinguishing
// wye/wye-with-neutral/delta, transcribed from element_47.go's own
// per-WindingType path formulas (confirmed against real corpus markup for
// all three: a plain "wye" 3-spoke mark, a "ЗВЕЗДА_С_НУЛЕМ" 4-line
// wye-with-neutral mark, and a closed-triangle "delta" mark using its own
// separate shift constant, baseRadius/2 rather than /3).
func writeWindingGlyph(w io.Writer, cx, cy float64, scheme model.WindingScheme, color string) {
	if scheme == "" {
		return
	}
	style := fmt.Sprintf("fill:none;stroke:%s;stroke-width:1", esc(color))
	shift := float64(transformerGlyphShift)
	switch scheme {
	case model.SchemeWye:
		fmt.Fprintf(w, "<path d=\"M %s %s l %s %s M %s %s l %s %s M %s %s l %s %s\" style=\"%s\" />\n",
			model.FmtNum(cx), model.FmtNum(cy), model.FmtNum(-shift), model.FmtNum(-shift),
			model.FmtNum(cx), model.FmtNum(cy), model.FmtNum(shift), model.FmtNum(-shift),
			model.FmtNum(cx), model.FmtNum(cy), model.FmtNum(0), model.FmtNum(shift),
			style)
	case model.SchemeWyeN:
		fmt.Fprintf(w, "<path d=\"M %s %s l %s %s M %s %s l %s %s M %s %s l %s %s M %s %s l %s %s\" style=\"%s\" />\n",
			model.FmtNum(cx), model.FmtNum(cy), model.FmtNum(-shift), model.FmtNum(-shift),
			model.FmtNum(cx), model.FmtNum(cy), model.FmtNum(shift), model.FmtNum(-shift),
			model.FmtNum(cx), model.FmtNum(cy), model.FmtNum(0), model.FmtNum(shift),
			model.FmtNum(cx), model.FmtNum(cy), model.FmtNum(shift), model.FmtNum(0),
			style)
	case model.SchemeDelta:
		deltaShift := float64(TransformerRadius / 2)
		half := float64(int(TransformerRadius/2) / 2)
		fmt.Fprintf(w, "<path d=\"M %s %s l %s %s l %s %s z\" style=\"%s\" />\n",
			model.FmtNum(cx), model.FmtNum(cy-half),
			model.FmtNum(half), model.FmtNum(deltaShift),
			model.FmtNum(-deltaShift), model.FmtNum(0),
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
func writeGroundingMark(w io.Writer, cx, cy float64, grounding model.NeutralGrounding, color string) {
	if grounding == "" {
		return
	}
	style := fmt.Sprintf("fill:none;stroke:%s;stroke-width:1", esc(color))
	nx := cx + transformerGlyphShift + 3 // just past the wyeN glyph's own horizontal spoke tip
	switch grounding {
	case model.GroundingSolid:
		// A standard earth-ground pictogram: a short stem plus three
		// horizontal bars of decreasing width.
		fmt.Fprintf(w, "<path d=\"M %s %s v 4 M %s %s h 10 M %s %s h 6 M %s %s h 2\" style=\"%s\" />\n",
			model.FmtNum(nx), model.FmtNum(cy),
			model.FmtNum(nx-5), model.FmtNum(cy+4),
			model.FmtNum(nx-3), model.FmtNum(cy+7),
			model.FmtNum(nx-1), model.FmtNum(cy+10),
			style)
	case model.GroundingIsolated:
		fmt.Fprintf(w, "<circle cx=\"%s\" cy=\"%s\" r=\"3\" style=\"%s\" />\n", model.FmtNum(nx+3), model.FmtNum(cy), style)
	case model.GroundingResistor:
		fmt.Fprintf(w, "<path d=\"M %s %s l 2 -3 l 2 3 l 2 -3 l 2 3 l 2 -3\" style=\"%s\" />\n", model.FmtNum(nx), model.FmtNum(cy), style)
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
// fixed-offset terminal in this package does; at -70 it sits above a 3- or
// 4-winding top winding's own lead tip (-60 / -50) — unlike Windings[0]'s own
// circle position, which the tap's real xsde2svg counterpart is *not*
// anchored to (confirmed by reading element_47.go's own isAutoTrans i==0
// branch: its own dX/dY default to 0, i.e. the transformer's own overall
// anchor, not whatever dX a later real circle ends up at).
const TransformerTapOffsetY = -70

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
	tapX, tapY := 0.0, float64(TransformerTapOffsetY)
	stubEndY := tapY + 20 // a short stub, leaving room for the arc below it
	style := fmt.Sprintf("fill:none;stroke:%s;stroke-width:2", esc(color))
	fmt.Fprintf(w, "<path d=\"M %s %s L %s %s\" style=\"%s\" />\n", model.FmtNum(tapX), model.FmtNum(tapY), model.FmtNum(tapX), model.FmtNum(stubEndY), style)

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
	targetX := math.Round(cx + sign*TransformerRadius*math.Sin(landingAngle))
	targetY := math.Round(cy - TransformerRadius*math.Cos(landingAngle))
	const arcRadius = 40
	sweep := 1
	if sign < 0 {
		sweep = 0
	}
	fmt.Fprintf(w, "<path d=\"M %s %s A %d %d 0 0 %d %s %s\" style=\"%s\" />\n",
		model.FmtNum(tapX), model.FmtNum(stubEndY), arcRadius, arcRadius, sweep, model.FmtNum(targetX), model.FmtNum(targetY), style)

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
func writePowerTransformer(w io.Writer, e model.Element, voltageColor map[int]string, fallbackColor string, mode RenderMode) {
	count := max(len(e.Windings), 2)

	// Static leaves data-voltage off the outer <g>, as real xsde2svg does:
	// each winding's own circle and lead carry their own.
	attrs := fmt.Sprintf(" data-voltage=\"%s\"", esc(fallbackColor))
	if mode == Interactive {
		attrs += " data-type=\"47\" data-editor-kind=\"element\""
	} else {
		attrs = " data-type=\"47\""
	}
	fmt.Fprintf(w, "<g id=\"%d\" data-name=\"%s\"%s transform=\"translate(%s,%s) rotate(%d)%s\">\n",
		e.ID, esc(e.Name), attrs, model.FmtNum(e.X), model.FmtNum(e.Y), e.Orient, mirrorScale(e.Mirror))

	for i := range count {
		var winding model.TransformerWinding
		if i < len(e.Windings) {
			winding = e.Windings[i]
		}
		color := voltageColor[winding.Voltage]
		if color == "" {
			color = fallbackColor
		}
		cx, cy := TransformerWindingOffset(count, i)
		dir := winding.Terminal
		if dir == "" {
			dir = DefaultTerminal(count, i)
		}

		fmt.Fprintf(w, "<circle cx=\"%s\" cy=\"%s\" r=\"%d\" style=\"fill:none;stroke:%s;stroke-width:2\" data-voltage=\"%s\" />\n",
			model.FmtNum(cx), model.FmtNum(cy), TransformerRadius, esc(color), esc(color))
		writeTransformerLeg(w, cx, cy, dir, TransformerLeadLength, color)
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
		// A winding with no glyph (no Scheme) writes no empty group.
		var glyph bytes.Buffer
		writeWindingGlyph(&glyph, cx, cy, winding.Scheme, color)
		if winding.Scheme == model.SchemeWyeN {
			writeGroundingMark(&glyph, cx, cy, winding.Grounding, color)
		}
		if glyph.Len() > 0 && e.Orient != 0 {
			fmt.Fprintf(w, "<g transform=\"rotate(%d,%s,%s)\">\n%s</g>\n", -e.Orient, model.FmtNum(cx), model.FmtNum(cy), glyph.String())
		} else {
			w.Write(glyph.Bytes())
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
		fmt.Fprintf(w, "<path d=\"M %s %s L %s %s\" style=\"%s\" />\n", model.FmtNum(float64(x0)), model.FmtNum(float64(y0)), model.FmtNum(float64(x1)), model.FmtNum(float64(y1)), style)
		fmt.Fprintf(w, "<path d=\"M %s %s l -5 -5 l 7 -2 z\" style=\"%s\" />\n", model.FmtNum(float64(x1)+3), model.FmtNum(float64(y1)+2), endStyle)
	}

	if e.VectorGroupLabel != "" {
		// A fixed cosmetic offset below the symbol's own lowest possible
		// extent (transformerRadius, plus a 13-unit allowance for the
		// lead, plus a small margin) — this text's own position isn't part of the electrical
		// geometry, so it doesn't need to be grid-aligned the way a lead
		// tip does.
		const vectorGroupLabelOffset = TransformerRadius + 13 + 14
		fmt.Fprintf(w, "<text x=\"0\" y=\"%d\" style=\"fill:%s;font-size:10px;font-family:Arial\" text-anchor=\"middle\">%s</text>\n",
			vectorGroupLabelOffset, esc(fallbackColor), esc(e.VectorGroupLabel))
	}

	fmt.Fprint(w, "</g>\n")
}

// labelTypeCode is a free caption's (Label's) xsde2svg catalog code.
const labelTypeCode = "5"

// WriteLabel draws a Label. Static writes real xsde2svg's own form, a
// <g data-type="5" data-name="…" id="…"> wrapping the <text>, where
// data-name is the caption's own text (lines joined by spaces) — Extract
// links it back to the element of that name. Interactive keeps the bare
// <text>, typed with data-type="5" too, since the canvas drags a label by
// setting x/y on the node carrying data-editor-kind="label". A turned label
// carries transform="rotate(a,x,y)" about its own anchor.
func WriteLabel(w io.Writer, l model.Label, mode RenderMode) {
	anchor := l.Anchor
	if anchor == "" {
		anchor = "start"
	}
	weight := ""
	if l.Bold {
		weight = "font-weight: bold;"
	}
	// In xsde2svg's order: bold, underline, italic.
	if l.Underline {
		weight += "text-decoration: underline;"
	}
	if l.Italic {
		weight += "font-style:italic;"
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
		baseline = "dominant-baseline:text-before-edge;"
	case "middle":
		baseline = "dominant-baseline:middle;"
	}
	style := fmt.Sprintf("fill:%s;text-anchor:%s;%sfont-size:%spx;font-family:%s;%swhite-space: pre;",
		color, anchor, baseline, model.FmtNum(l.Size), font, weight)
	lines := strings.Split(l.Text, "\n")
	textAttrs := fmt.Sprintf(" id=\"%d\" data-type=\"%s\"", l.ID, labelTypeCode)
	if mode == Interactive {
		textAttrs += " data-editor-kind=\"label\""
	} else {
		fmt.Fprintf(w, "<g data-type=\"%s\" data-name=\"%s\" id=\"%d\">\n", labelTypeCode, esc(strings.Join(lines, " ")), l.ID)
		textAttrs = ""
	}
	if l.Rotate != 0 {
		textAttrs += fmt.Sprintf(" transform=\"rotate(%s,%s,%s)\"", model.FmtNum(l.Rotate), model.FmtNum(l.X), model.FmtNum(l.Y))
	}
	fmt.Fprintf(w, "<text%s x=\"%s\" y=\"%s\" style=\"%s\">%s", textAttrs, model.FmtNum(l.X), model.FmtNum(l.Y), esc(style), esc(lines[0]))
	for _, ln := range lines[1:] {
		fmt.Fprintf(w, "<tspan x=\"%s\" dy=\"%s\" style=\"%s\">%s</tspan>",
			model.FmtNum(l.X), model.FmtNum(l.Size*1.4), esc(style), esc(ln))
	}
	fmt.Fprint(w, "</text>\n")
	if mode != Interactive {
		fmt.Fprint(w, "</g>\n")
	}
}

// digitalDeviceTypeCode is shape 134's own xsde2svg catalog code, written as
// data-type the same way an Element/Connector's own shape/kind code is.
const digitalDeviceTypeCode = "134"

// writeDigitalDevice draws a shape-134 SCADA readout as a bare <text> node —
// no wrapping <g>/transform, matching a real xsde2svg-exported one exactly
// (see DigitalDevice's own doc comment). Unlike WriteLabel's tspans (each a
// new stacked line via dy), Unit's own tspan shares Value's line: same style,
// no dy, immediately after Value with a single separating space.
func writeDigitalDevice(w io.Writer, dd model.DigitalDevice, mode RenderMode) {
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
		baseline = "dominant-baseline:text-before-edge;"
	case "middle":
		baseline = "dominant-baseline:middle;"
	}
	style := fmt.Sprintf("fill:%s;text-anchor:%s;%sfont-size:%spx;font-family:%s;%s",
		color, anchor, baseline, model.FmtNum(dd.Size), font, weight)
	editorAttr := ""
	if mode == Interactive {
		editorAttr = " data-editor-kind=\"digitaldevice\""
	}
	unitAttr := ""
	if dd.Unit != "" {
		unitAttr = fmt.Sprintf(" data-unit=\"%s\"", esc(dd.Unit))
	}
	fmt.Fprintf(w, "<text data-type=\"%s\" x=\"%s\" y=\"%s\" id=\"%d\" data-name=\"%s\"%s style=\"%s\"%s>%s",
		digitalDeviceTypeCode, model.FmtNum(dd.X), model.FmtNum(dd.Y), dd.ID, esc(dd.Name), unitAttr, esc(style), editorAttr, esc(dd.Value))
	if dd.Unit != "" {
		fmt.Fprintf(w, " <tspan style=\"%s\">%s</tspan>", esc(style), esc(dd.Unit))
	}
	fmt.Fprint(w, "</text>\n")
}

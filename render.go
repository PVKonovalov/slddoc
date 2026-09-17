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

// defaultBackground matches the background-color xsde2svg wrote on every
// example diagram in this corpus. v1 does not model a per-diagram
// background.
const defaultBackground = "#12161d"

var attrEscaper = strings.NewReplacer(`&`, "&amp;", `<`, "&lt;", `>`, "&gt;", `"`, "&quot;")

func esc(s string) string { return attrEscaper.Replace(s) }

func fmtNum(v float64) string {
	if v == math.Trunc(v) {
		return strconv.FormatInt(int64(v), 10)
	}
	return strconv.FormatFloat(v, 'g', -1, 64)
}

func stateFill(state *int) string {
	if state == nil {
		return "none"
	}
	switch *state {
	case 0:
		return "red"
	case 1:
		return "lawngreen"
	default:
		return "yellow"
	}
}

// stateLineRe matches a template's {state:parallel|perpendicular|diagonal}
// placeholder: the switch-like devices' internal state indicator, drawn
// parallel to the device's own (locally-vertical) axis when closed (state
// 1), perpendicular when open (state 0), and at 45° for any other recorded
// state (verified against xsde2svg source for parallel/perpendicular — see
// element_41.go, element_42.go, element_43.go, element_71.go; the diagonal
// case is a live-status convention this static export format doesn't
// itself encode, per user-supplied domain knowledge).
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
// state, since every real corpus instance is state 0/unlit).
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

// shapeName gives the English equipment name Render annotates a run of
// same-Shape elements with, mirroring the source SVG's own
// "<!-- <russian_name>:<code> -->" comment convention (one per contiguous
// block of same-data-type elements — see e.g. Examples.svg). Kept
// per-shape rather than per-Class since the source itself distinguishes,
// e.g., a fixed breaker (41: "выключатель") from a withdrawable one (43:
// "выключатель_выдвижной") despite both being ClassBreaker.
var shapeName = map[string]string{
	"7":      "Junction point",
	"24":     "Busbar",
	"31":     "Ground terminal",
	"33":     "Choke coil",
	"34":     "Current transformer",
	"35":     "Surge arrester",
	"41":     "Breaker",
	"42":     "Load-break switch",
	"43":     "Breaker (withdrawable)",
	"47":     "Power transformer",
	"49":     "Disconnector (withdrawable)",
	"54":     "Ground switch",
	"71":     "Disconnector",
	"106":    "Lamp",
	"162":    "Disconnector",
	"203":    "Fuse",
	"388":    "Capacitor",
	"320003": "Fault passage indicator",
}

// connectorKindName gives the English wire kind name Render annotates a run
// of same-Kind connectors with, the same way shapeName does for elements.
var connectorKindName = map[string]string{
	string(KindBusbarWire):   "Busbar wire",
	string(KindOverheadLine): "Overhead line",
	string(KindCableLine):    "Cable line",
	string(KindObjectLink):   "Object link",
}

// typeComment writes a "<!-- Name:shape -->" line the first time shape is
// seen or whenever it changes from the previous call, so consecutive
// same-shape elements/connectors get one header the way the source SVG
// does — never a redundant repeat for every instance. last is updated
// in place.
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
func renderElement(w io.Writer, lib *SymbolLibrary, voltageColor map[int]string, e Element, missing *[]string, seenMissing map[string]bool, lastShape *string) {
	typeComment(w, shapeName, e.Shape, e.Shape, lastShape)

	var color string
	if e.Class == ClassLamp {
		// A Lamp's colors are its own data-fill off/on pair, not a
		// VoltageClass — it isn't part of the electrical network.
		color = lampColor(e)
	} else if e.Class == ClassFaultPassageIndicator {
		// Every real instance draws the same fixed dark fill regardless
		// of state; it isn't part of the electrical network either.
		color = defaultBackground
	} else {
		color = voltageColor[e.Voltage]
		if color == "" {
			// A PowerTransformer's two windings can carry different
			// voltages that v1's schema doesn't record per-port (see
			// parsePowerTransformer); fall back to a visible neutral
			// color rather than emitting an empty stroke.
			color = "gray"
		}
	}
	if e.Class == ClassBusBarSection {
		writePolyline(w, e.Points, color, false)
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
	body = strings.NewReplacer(
		"{color}", esc(color),
		"{fill}", stateFill(e.State),
		"{radius}", fmtNum(e.Radius),
	).Replace(body)
	fmt.Fprintf(w, "<g id=\"%d\" data-name=\"%s\" transform=\"translate(%s,%s) rotate(%d)\">\n%s\n</g>\n",
		e.ID, esc(e.Name), fmtNum(e.X), fmtNum(e.Y), e.Orient, body)
}

// Render writes d as a fresh SVG document, using lib to place each
// Element's symbol. It does not attempt to reproduce the source SVG
// byte-for-byte (see the package doc comment); the output is a new,
// independently generated rendering of the same diagram.
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
func Render(d *Diagram, lib *SymbolLibrary, w io.Writer) error {
	voltageColor := map[int]string{}
	for _, vc := range d.VoltageClasses {
		voltageColor[vc.ID] = vc.Color
	}

	fmt.Fprintf(w, "<?xml version=\"1.0\"?>\n<svg width=\"%s\" height=\"%s\" style=\"stroke-width: 0px; background-color: %s;\" xmlns=\"http://www.w3.org/2000/svg\" xmlns:xlink=\"http://www.w3.org/1999/xlink\">\n",
		fmtNum(d.Width), fmtNum(d.Height), defaultBackground)

	var missing []string
	seenMissing := map[string]bool{}

	elevated := map[int][]Element{}

	var lastShape string
	for _, e := range d.Elements {
		if z := elementZOrder[e.Class]; z > 0 {
			elevated[z] = append(elevated[z], e)
			continue
		}
		renderElement(w, lib, voltageColor, e, &missing, seenMissing, &lastShape)
	}

	var lastConnKind string
	for _, c := range d.Connectors {
		typeComment(w, connectorKindName, string(c.Kind), "", &lastConnKind)
		writePolyline(w, c.Points, voltageColor[c.Voltage], c.Dashed)
	}

	tiers := make([]int, 0, len(elevated))
	for z := range elevated {
		tiers = append(tiers, z)
	}
	sort.Ints(tiers)
	for _, z := range tiers {
		var lastTierShape string
		for _, e := range elevated[z] {
			renderElement(w, lib, voltageColor, e, &missing, seenMissing, &lastTierShape)
		}
	}

	for _, l := range d.Labels {
		writeLabel(w, l)
	}

	fmt.Fprint(w, "</svg>\n")

	if len(missing) > 0 {
		return fmt.Errorf("slddoc: symbol library missing shape(s): %s", strings.Join(missing, ", "))
	}
	return nil
}

func writePolyline(w io.Writer, pts []Point, color string, dashed bool) {
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
	fmt.Fprintf(w, "<polyline points=\"%s\" style=\"fill:none;stroke:%s;%sstroke-width:1\" />\n",
		esc(sb.String()), esc(color), dash)
}

func writeLabel(w io.Writer, l Label) {
	anchor := l.Anchor
	if anchor == "" {
		anchor = "start"
	}
	weight := ""
	if l.Bold {
		weight = "font-weight: bold;"
	}
	style := fmt.Sprintf("fill:white;text-anchor:%s;font-size:%spx;font-family:Arial;%swhite-space: pre;",
		anchor, fmtNum(l.Size), weight)

	lines := strings.Split(l.Text, "\n")
	fmt.Fprintf(w, "<text x=\"%s\" y=\"%s\" style=\"%s\">%s", fmtNum(l.X), fmtNum(l.Y), esc(style), esc(lines[0]))
	for _, ln := range lines[1:] {
		fmt.Fprintf(w, "<tspan x=\"%s\" dy=\"%s\" style=\"%s\">%s</tspan>",
			fmtNum(l.X), fmtNum(l.Size*1.4), esc(style), esc(ln))
	}
	fmt.Fprint(w, "</text>\n")
}

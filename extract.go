package slddoc

import (
	"fmt"
	"strconv"
)

// Report summarizes one Extract call: what was captured, and, for
// visibility rather than silent data loss, what the source SVG contained
// that v1 doesn't understand yet.
type Report struct {
	Elements       int
	Connectors     int
	Labels         int
	DigitalDevices int
	Nodes          int
	// Skipped counts top-level nodes with a data-type code Extract does not
	// (yet) support, keyed by that code (e.g. "310" for a decorative container).
	Skipped map[string]int
	// Failed lists the ids of elements that matched a known data-type but
	// whose geometry didn't parse (e.g. a transformer with other than two
	// windings).
	Failed []string
}

// elementDataTypes are the xsde2svg ObjectType codes this package's v1
// extractor understands. Everything else is counted in Report.Skipped
// rather than causing an error, per this package's doc comment.
var elementDataTypes = map[string]bool{
	"7": true, "21": true, "22": true, "23": true, "24": true, "28": true,
	"41": true, "42": true, "43": true, "54": true, "71": true, "162": true,
	"49": true, "47": true, "31": true, "33": true, "34": true, "35": true,
	"203": true, "388": true, "106": true, "320003": true, "37": true,
	"397": true, "29": true, "76": true, "154": true, "168": true,
	"172": true, "173": true, "14": true, "55": true, "52": true, "51": true,
	"164": true, "3": true, "2": true, "4": true, "56": true, "398": true,
	"385": true, "386": true, "32": true, "113": true, "335": true, "292": true, "1": true,
	"320001": true,
	"312":    true, "313": true,
}

// twoPortShapes maps a two-terminal shape code (see parseTwoPortDevice) to
// its real-world equipment class.
var twoPortShapes = map[string]Class{
	"41": ClassBreaker, "43": ClassBreaker,
	"42": ClassLoadBreakSwitch,
	"71": ClassDisconnector, "162": ClassDisconnector, "49": ClassDisconnector,
	"33":  ClassChokeCoil,
	"34":  ClassCurrentTransformer,
	"35":  ClassSurgeArrester,
	"29":  ClassSurgeArrester,
	"203": ClassFuse,
	"154": ClassFuse,
	"388": ClassCapacitor,
	"37":  ClassReactor,
	"76":  ClassStarter,
	"14":  ClassNonIntersection,
	"51":  ClassChassis,
	"56":  ClassCableConnector,
	"32":  ClassCableJoint,
}

// unrecognizedShapeName gives a human-readable name for an xsde2svg
// ObjectType code this package's v1 extractor doesn't (yet) turn into a
// real Element — addMissingLabel's own fallback when shapeName (every
// code this package *does* render, keyed the same way) doesn't have one
// either, i.e. every code Report.Skipped can report. English names taken
// from the xsde2svg catalog's own object-type list, not derived from
// anything in this package.
var unrecognizedShapeName = map[string]string{
	"6":    "Booster/voltage regulator (single-winding power transformer)",
	"9":    "Arc",
	"10":   "Connector",
	"11":   "Backdrop/image file",
	"16":   "Polygon",
	"19":   "Metal anchor/angle pole",
	"26":   "Fork/branch point",
	"38":   "Thermal power plant",
	"39":   "Synchronous motor",
	"44":   "Knife switch",
	"50":   "Withdrawable sectionalizer",
	"60":   "Zone division",
	"71":   "RZD connection/disconnector",
	"83":   "Connector arrow",
	"102":  "Panel/board",
	"103":  "Automation device",
	"130":  "Device",
	"146":  "Power pole",
	"156":  "Resistor",
	"157":  "Thyristor",
	"163":  "Short-circuiter without ground",
	"166":  "Disconnector-fuse",
	"174":  "Synchronous compensator",
	"175":  "3-position knife switch",
	"302":  "Window icon",
	"310":  "Container",
	"319":  "Small window",
	"320":  "Custom element",
	"360":  "Substation",
	"389":  "Blocking filter",
	"391":  "RTF text",
	"399":  "Power circuit breaker",
	"3206": "RZD connection/disconnector (arc-extinguishing contacts)",
}

// missingElementAnchor makes a best-effort attempt at a diagram-space
// position for a top-level node Extract couldn't otherwise parse, so
// addMissingLabel's own diagnostic Label lands close to where the real
// element would have been instead of not appearing at all. Tries, in
// order: a rotate() transform's own center (the same true anchor most
// real two-port shapes use), then a descendant <path>'s own first drawn
// point, then a descendant <circle>'s own cx/cy, then a <rect>'s own
// center (n itself included for both — a bare untyped shape, unlike every
// real equipment symbol, generally isn't wrapped in its own outer <g> at
// all, the same reason parseRectangle/parseCircle read n's own attributes
// directly rather than a descendant's). false when none of these apply
// (a genuinely empty/unparseable node, or one recognized well enough for
// its own dedicated parser but shaped nothing like a rect/circle/path) —
// addMissingLabel skips adding a Label in that case, though
// Report.Skipped/Failed still record it either way.
func missingElementAnchor(n *rawNode) (Point, bool) {
	if _, center, ok := parseRotate(n.firstAttrDescendant("transform")); ok {
		return center, true
	}
	for _, p := range elementPaths(n) {
		subpaths, err := parseSubpaths(p.attr("d"))
		if err != nil {
			continue
		}
		for _, sp := range subpaths {
			if len(sp) > 0 {
				return sp[0], true
			}
		}
	}
	circles := n.descendants("circle")
	if n.Tag == "circle" {
		circles = append([]*rawNode{n}, circles...)
	}
	for _, c := range circles {
		cx, errX := strconv.ParseFloat(c.attr("cx"), 64)
		cy, errY := strconv.ParseFloat(c.attr("cy"), 64)
		if errX == nil && errY == nil {
			return Point{X: cx, Y: cy}, true
		}
	}
	rects := n.descendants("rect")
	if n.Tag == "rect" {
		rects = append([]*rawNode{n}, rects...)
	}
	for _, r := range rects {
		x, errX := strconv.ParseFloat(r.attr("x"), 64)
		y, errY := strconv.ParseFloat(r.attr("y"), 64)
		w, errW := strconv.ParseFloat(r.attr("width"), 64)
		h, errH := strconv.ParseFloat(r.attr("height"), 64)
		if errX == nil && errY == nil && errW == nil && errH == nil {
			return Point{X: x + w/2, Y: y + h/2}, true
		}
	}
	return Point{}, false
}

// addMissingLabel appends a red diagnostic Label at n's own best-effort
// position, naming the xsde2svg ObjectType code (dt) that couldn't be
// turned into a real Element/Connector — either because Extract doesn't
// recognize dt at all (a Report.Skipped call site) or because a
// recognized dt's own specific instance failed to parse (a Report.Failed
// one) — so nothing from the source SVG goes silently missing from the
// extracted diagram: a diagram author sees exactly where and what wasn't
// carried over, rather than a topology with an unexplained gap. Its own
// ID is left unset (0), the same as any other extracted Label lacking a
// real source id — see ensureLastId's own doc comment (frontend
// diagramOps.ts) for how that gets backfilled into a real unique one.
func addMissingLabel(d *Diagram, n *rawNode, dt string) {
	anchor, ok := missingElementAnchor(n)
	if !ok {
		return
	}
	name := dt
	if known, ok := shapeName[dt]; ok {
		name = fmt.Sprintf("%s (%s)", known, dt)
	} else if known, ok := unrecognizedShapeName[dt]; ok {
		name = fmt.Sprintf("%s (%s)", known, dt)
	}
	d.Labels = append(d.Labels, Label{
		Layer:  resolveLayer(n.attr("data-layer")),
		X:      anchor.X,
		Y:      anchor.Y,
		Size:   13,
		Text:   fmt.Sprintf("Missing: %s #%s", name, n.attr("id")),
		Color:  "red",
		Anchor: "middle",
	})
}

// maxNumericID walks n's entire subtree (n included) and returns the
// largest integer "id" attribute found anywhere, or 0 if none parse — the
// same id space every real Element/Connector/Label/DigitalDevice's own
// source id already lives in (see Diagram.LastID's own doc comment).
// Extract uses this to seed a counter for the rare element it must
// synthesize an id for itself (one with no id in the original at all —
// see parseDigitalDeviceBackgroundRect), so a freshly assigned id can
// never collide with any real one already present in the document.
func maxNumericID(n *rawNode) int {
	max := 0
	if id, err := strconv.Atoi(n.attr("id")); err == nil && id > max {
		max = id
	}
	for _, c := range n.Children {
		if m := maxNumericID(c); m > max {
			max = m
		}
	}
	return max
}

// Extract parses raw as an xsde2svg-generated SVG and builds a Diagram.
func Extract(raw []byte, source string, voltageHints map[string]string) (*Diagram, Report, error) {
	root, err := parseRawTree(raw)
	if err != nil {
		return nil, Report{}, fmt.Errorf("slddoc: parsing SVG: %w", err)
	}
	if root == nil || root.Tag != "svg" {
		return nil, Report{}, fmt.Errorf("slddoc: not an SVG document")
	}

	width, _ := strconv.ParseFloat(root.attr("width"), 64)
	height, _ := strconv.ParseFloat(root.attr("height"), 64)
	nextSynthID := maxNumericID(root) + 1

	d := &Diagram{Width: width, Height: height, Source: source}
	report := Report{Skipped: map[string]int{}}

	// colors accumulates every raw data-voltage color seen (elements and
	// connectors both) just to compute the distinct set buildVoltageClasses
	// turns into d.VoltageClasses. elementVoltage/connectorVoltage instead
	// keep each element's/connector's own raw color index-aligned with
	// d.Elements/d.Connectors, for the per-instance resolution pass below —
	// needed because Element.Voltage/Connector.Voltage is an int (a
	// resolved VoltageClass id), not something a raw color string can be
	// stashed in along the way the way it could when Voltage was a string.
	var colors []string
	var elementVoltage []string
	var connectorVoltage []string
	var bindings []portBinding
	var labelNodes []*rawNode
	// windingColors keys a PowerTransformer's own index in d.Elements to
	// its own per-winding raw colors (parsePowerTransformer's own extra
	// return value), index-aligned with that same element's own Windings —
	// resolved into each TransformerWinding.Voltage in the same pass that
	// resolves elementVoltage/connectorVoltage below, once colorToID
	// exists. A PowerTransformer has no single representative color of its
	// own (addElement is still called with "" for it, same as before), so
	// this needs its own side-channel rather than reusing elementVoltage.
	windingColors := map[int][]string{}

	addElement := func(el Element, ports []Point, voltage string) {
		colors = append(colors, voltage)
		elementVoltage = append(elementVoltage, voltage)
		idx := len(d.Elements)
		d.Elements = append(d.Elements, el)
		for i, p := range ports {
			bindings = append(bindings, portBinding{idx, i, p})
		}
	}

	for i, n := range root.Children {
		if n.Tag == "metadata" {
			layers, err := parseLayers(n.Text)
			if err != nil {
				return nil, Report{}, err
			}
			d.Layers = layers
			continue
		}

		dt := n.attr("data-type")
		if dt == "" {
			continue // <defs>, style helpers, and other untyped nodes
		}
		if dt == "5" {
			labelNodes = append(labelNodes, n)
			continue
		}
		if dt == "134" {
			if dd, ok := parseDigitalDevice(n); ok {
				d.DigitalDevices = append(d.DigitalDevices, dd)
				if i > 0 {
					if rect, ok := parseDigitalDeviceBackgroundRect(root.Children[i-1]); ok {
						// The real source never gives this rect its own id
						// (see parseDigitalDeviceBackgroundRect's own doc
						// comment) — synthesize one now rather than leaving
						// it at 0, which every other Element's id treats as
						// "unset" and which every synthesized rect would
						// otherwise share, colliding with each other.
						rect.ID = nextSynthID
						nextSynthID++
						addElement(rect, nil, "")
					}
				}
			} else {
				report.Failed = append(report.Failed, n.attr("id"))
				addMissingLabel(d, n, dt)
			}
			continue
		}
		if !elementDataTypes[dt] {
			report.Skipped[dt]++
			addMissingLabel(d, n, dt)
			continue
		}

		switch dt {
		case "24":
			el, voltage, err := parseBusBar(n)
			if err != nil {
				report.Failed = append(report.Failed, n.attr("id"))
				addMissingLabel(d, n, dt)
				continue
			}
			addElement(el, nil, voltage)

		case "3":
			el, err := parseRectangle(n)
			if err != nil {
				report.Failed = append(report.Failed, n.attr("id"))
				addMissingLabel(d, n, dt)
				continue
			}
			addElement(el, nil, "")

		case "4":
			el, err := parseCircle(n)
			if err != nil {
				report.Failed = append(report.Failed, n.attr("id"))
				addMissingLabel(d, n, dt)
				continue
			}
			addElement(el, nil, "")

		case "2":
			el, err := parseArrow(n)
			if err != nil {
				report.Failed = append(report.Failed, n.attr("id"))
				addMissingLabel(d, n, dt)
				continue
			}
			addElement(el, nil, "")

		case "113":
			el, err := parseButton(n)
			if err != nil {
				report.Failed = append(report.Failed, n.attr("id"))
				addMissingLabel(d, n, dt)
				continue
			}
			addElement(el, nil, "")

		case "335":
			el, err := parseRoad(n)
			if err != nil {
				report.Failed = append(report.Failed, n.attr("id"))
				addMissingLabel(d, n, dt)
				continue
			}
			addElement(el, nil, "")

		case "292":
			el, err := parsePole(n)
			if err != nil {
				report.Failed = append(report.Failed, n.attr("id"))
				addMissingLabel(d, n, dt)
				continue
			}
			addElement(el, nil, "")

		case "1":
			el, err := parseLine(n)
			if err != nil {
				report.Failed = append(report.Failed, n.attr("id"))
				addMissingLabel(d, n, dt)
				continue
			}
			addElement(el, nil, "")

		case "21", "22", "23", "28":
			c, voltage, err := parseConnector(n)
			if err != nil {
				report.Failed = append(report.Failed, n.attr("id"))
				addMissingLabel(d, n, dt)
				continue
			}
			colors = append(colors, voltage)
			connectorVoltage = append(connectorVoltage, voltage)
			d.Connectors = append(d.Connectors, c)

		case "7":
			el, voltage, err := parseJunctionPoint(n)
			if err != nil {
				report.Failed = append(report.Failed, n.attr("id"))
				addMissingLabel(d, n, dt)
				continue
			}
			addElement(el, []Point{{X: el.X, Y: el.Y}}, voltage)
			// A fixed xsde2svg version wraps a junction point's own
			// optional ParamText/SubscriptName <text> in the same <g> as
			// its circle (see parseJunctionPoint's own doc comment) — read
			// separately here since it needs el.ID, already resolved above.
			if lbl, ok := parseAttachedLabel(n, el.ID); ok {
				d.Labels = append(d.Labels, lbl)
			}

		case "106":
			el, err := parseLamp(n)
			if err != nil {
				report.Failed = append(report.Failed, n.attr("id"))
				addMissingLabel(d, n, dt)
				continue
			}
			addElement(el, nil, "")
			// A fixed xsde2svg version wraps a Lamp's own optional
			// ParamText/SubscriptName <text> in the same <g> as its
			// circle (see parseLamp's own doc comment) — read separately
			// here since it needs el.ID, already resolved above.
			if lbl, ok := parseAttachedLabel(n, el.ID); ok {
				d.Labels = append(d.Labels, lbl)
			}

		case "320003":
			el, err := parseFaultPassageIndicator(n)
			if err != nil {
				report.Failed = append(report.Failed, n.attr("id"))
				addMissingLabel(d, n, dt)
				continue
			}
			addElement(el, nil, "")

		case "320001":
			el, err := parsePowerflowIndicator(n)
			if err != nil {
				report.Failed = append(report.Failed, n.attr("id"))
				addMissingLabel(d, n, dt)
				continue
			}
			addElement(el, nil, "")

		case "312":
			el, err := parseTable(n)
			if err != nil {
				report.Failed = append(report.Failed, n.attr("id"))
				addMissingLabel(d, n, dt)
				continue
			}
			addElement(el, nil, "")

		case "313":
			el, err := parseTable2(n)
			if err != nil {
				report.Failed = append(report.Failed, n.attr("id"))
				addMissingLabel(d, n, dt)
				continue
			}
			addElement(el, nil, "")

		case "41", "42", "43", "71", "162", "49", "33", "34", "35", "203", "388", "37", "29", "76", "154", "14", "51", "56", "32":
			el, ports, voltage, err := parseTwoPortDevice(n, twoPortShapes[dt], dt)
			if err != nil {
				report.Failed = append(report.Failed, n.attr("id"))
				addMissingLabel(d, n, dt)
				continue
			}
			addElement(el, ports, voltage)

		case "397":
			el, ports, voltage, err := parseOnePortDevice(n, ClassReactorShunt, "397")
			if err != nil {
				report.Failed = append(report.Failed, n.attr("id"))
				addMissingLabel(d, n, dt)
				continue
			}
			addElement(el, ports, voltage)

		case "168":
			el, ports, voltage, err := parseOnePortDevice(n, ClassSurgeArrester, "168")
			if err != nil {
				report.Failed = append(report.Failed, n.attr("id"))
				addMissingLabel(d, n, dt)
				continue
			}
			addElement(el, ports, voltage)

		case "172":
			el, ports, voltage, err := parseOnePortDevice(n, ClassCapacitorBank, "172")
			if err != nil {
				report.Failed = append(report.Failed, n.attr("id"))
				addMissingLabel(d, n, dt)
				continue
			}
			addElement(el, ports, voltage)

		case "52":
			el, ports, voltage, err := parseOnePortDevice(n, ClassHalfChassis, "52")
			if err != nil {
				report.Failed = append(report.Failed, n.attr("id"))
				addMissingLabel(d, n, dt)
				continue
			}
			addElement(el, ports, voltage)

		case "55":
			el, ports, voltage, err := parseVoltageTransformer(n)
			if err != nil {
				report.Failed = append(report.Failed, n.attr("id"))
				addMissingLabel(d, n, dt)
				continue
			}
			addElement(el, ports, voltage)

		case "173":
			el, ports, voltage, err := parseOnePortDevice(n, ClassGenerator, "173")
			if err != nil {
				report.Failed = append(report.Failed, n.attr("id"))
				addMissingLabel(d, n, dt)
				continue
			}
			addElement(el, ports, voltage)

		case "31":
			el, ports, voltage, err := parseGround(n)
			if err != nil {
				report.Failed = append(report.Failed, n.attr("id"))
				addMissingLabel(d, n, dt)
				continue
			}
			addElement(el, ports, voltage)

		case "54":
			el, ports, voltage, err := parseGroundSwitch(n)
			if err != nil {
				report.Failed = append(report.Failed, n.attr("id"))
				addMissingLabel(d, n, dt)
				continue
			}
			addElement(el, ports, voltage)

		case "398":
			el, ports, voltage, err := parseShortCircuiter(n)
			if err != nil {
				report.Failed = append(report.Failed, n.attr("id"))
				addMissingLabel(d, n, dt)
				continue
			}
			addElement(el, ports, voltage)

		case "385":
			el, ports, voltage, err := parsePackageSubstation(n)
			if err != nil {
				report.Failed = append(report.Failed, n.attr("id"))
				addMissingLabel(d, n, dt)
				continue
			}
			addElement(el, ports, voltage)

		case "386":
			el, ports, voltage, err := parseEnclosedSubstation(n)
			if err != nil {
				report.Failed = append(report.Failed, n.attr("id"))
				addMissingLabel(d, n, dt)
				continue
			}
			addElement(el, ports, voltage)

		case "164":
			el, ports, voltage, err := parseSectionalizer(n)
			if err != nil {
				report.Failed = append(report.Failed, n.attr("id"))
				addMissingLabel(d, n, dt)
				continue
			}
			addElement(el, ports, voltage)

		case "47":
			el, ports, wColors, err := parsePowerTransformer(n)
			if err != nil {
				report.Failed = append(report.Failed, n.attr("id"))
				addMissingLabel(d, n, dt)
				continue
			}
			idx := len(d.Elements)
			addElement(el, ports, "")
			windingColors[idx] = wColors
			colors = append(colors, wColors...)
		}
	}

	if d.Layers == nil {
		d.Layers, _ = parseLayers("")
	}

	classes, colorToID := buildVoltageClasses(colors, voltageHints)
	d.VoltageClasses = classes
	for i := range d.Elements {
		d.Elements[i].Voltage = colorToID[normalizeColor(elementVoltage[i])]
	}
	for i := range d.Connectors {
		d.Connectors[i].Voltage = colorToID[normalizeColor(connectorVoltage[i])]
	}
	for idx, wColors := range windingColors {
		for i, c := range wColors {
			if i < len(d.Elements[idx].Windings) {
				d.Elements[idx].Windings[i].Voltage = colorToID[normalizeColor(c)]
			}
		}
	}

	if nextSynthID-1 > d.LastID {
		d.LastID = nextSynthID - 1
	}

	buildTopology(d, bindings)
	// append, not assign — d.Labels may already hold addMissingLabel's own
	// diagnostic entries from earlier in this same loop, which matchLabels
	// itself knows nothing about and would otherwise silently clobber.
	d.Labels = append(d.Labels, matchLabels(d, labelNodes)...)

	report.Elements = len(d.Elements)
	report.Connectors = len(d.Connectors)
	report.Labels = len(d.Labels)
	report.DigitalDevices = len(d.DigitalDevices)
	report.Nodes = len(d.Nodes)
	return d, report, nil
}

package slddoc

import (
	"fmt"
	"strconv"
)

// Report summarizes one Extract call: what was captured, and, for
// visibility rather than silent data loss, what the source SVG contained
// that v1 doesn't understand yet.
type Report struct {
	Elements   int
	Connectors int
	Labels     int
	Nodes      int
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
	"203": true, "388": true, "106": true, "320003": true,
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
	"203": ClassFuse,
	"388": ClassCapacitor,
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

	addElement := func(el Element, ports []Point, voltage string) {
		colors = append(colors, voltage)
		elementVoltage = append(elementVoltage, voltage)
		idx := len(d.Elements)
		d.Elements = append(d.Elements, el)
		for i, p := range ports {
			bindings = append(bindings, portBinding{idx, i, p})
		}
	}

	for _, n := range root.Children {
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
		if !elementDataTypes[dt] {
			report.Skipped[dt]++
			continue
		}

		switch dt {
		case "24":
			el, voltage, err := parseBusBar(n)
			if err != nil {
				report.Failed = append(report.Failed, n.attr("id"))
				continue
			}
			addElement(el, nil, voltage)

		case "21", "22", "23", "28":
			c, voltage, err := parseConnector(n)
			if err != nil {
				report.Failed = append(report.Failed, n.attr("id"))
				continue
			}
			colors = append(colors, voltage)
			connectorVoltage = append(connectorVoltage, voltage)
			d.Connectors = append(d.Connectors, c)

		case "7":
			el, voltage, err := parseJunctionPoint(n)
			if err != nil {
				report.Failed = append(report.Failed, n.attr("id"))
				continue
			}
			addElement(el, []Point{{X: el.X, Y: el.Y}}, voltage)

		case "106":
			el, err := parseLamp(n)
			if err != nil {
				report.Failed = append(report.Failed, n.attr("id"))
				continue
			}
			addElement(el, nil, "")

		case "320003":
			el, err := parseFaultPassageIndicator(n)
			if err != nil {
				report.Failed = append(report.Failed, n.attr("id"))
				continue
			}
			addElement(el, nil, "")

		case "41", "42", "43", "71", "162", "49", "33", "34", "35", "203", "388":
			el, ports, voltage, err := parseTwoPortDevice(n, twoPortShapes[dt], dt)
			if err != nil {
				report.Failed = append(report.Failed, n.attr("id"))
				continue
			}
			addElement(el, ports, voltage)

		case "31":
			el, ports, voltage, err := parseGround(n)
			if err != nil {
				report.Failed = append(report.Failed, n.attr("id"))
				continue
			}
			addElement(el, ports, voltage)

		case "54":
			el, ports, voltage, err := parseGroundSwitch(n)
			if err != nil {
				report.Failed = append(report.Failed, n.attr("id"))
				continue
			}
			addElement(el, ports, voltage)

		case "47":
			el, ports, err := parsePowerTransformer(n)
			if err != nil {
				report.Failed = append(report.Failed, n.attr("id"))
				continue
			}
			addElement(el, ports, "")
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

	buildTopology(d, bindings)
	d.Labels = matchLabels(d, labelNodes)

	report.Elements = len(d.Elements)
	report.Connectors = len(d.Connectors)
	report.Labels = len(d.Labels)
	report.Nodes = len(d.Nodes)
	return d, report, nil
}

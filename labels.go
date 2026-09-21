package slddoc

import (
	"strconv"
	"strings"
)

// parseOptionalID parses a real SVG node's own id="..." attribute the same
// way parseElementID does, but leniently: missing or non-numeric returns 0
// (this model's own "unset" sentinel — see Diagram.LastID's doc comment)
// rather than failing the whole parse the way parseElementID's hard error
// does — a Label/DigitalDevice's own id, unlike an Element/Connector/
// Node's, is never referenced by anything else, so there's nothing to
// break by leaving it unset. Real xsde2svg-exported instances do carry a
// real numeric id here (confirmed against sld-svg's own example corpus and
// sld-viewer's), the same as an Element's; 0 only happens for a
// synthetically-constructed source lacking one.
func parseOptionalID(n *rawNode) int {
	id, err := strconv.Atoi(n.attr("id"))
	if err != nil {
		return 0
	}
	return id
}

// parseVAlign reverses writeLabel/writeDigitalDevice's own
// dominant-baseline mapping back into Label.VAlign/DigitalDevice.VAlign's
// convention: "hanging" -> "top", "middle" -> "middle", anything else
// (including absent, the real xsde2svg default) -> "" (bottom/baseline).
func parseVAlign(style string) string {
	switch styleProp(style, "dominant-baseline") {
	case "hanging":
		return "top"
	case "middle":
		return "middle"
	default:
		return ""
	}
}

// firstTextChild returns n's own first <text> child, or its first <text>
// descendant if it has none directly (a real xsde2svg export's own nesting
// depth for this varies by data-type/version) — nil if there's no <text>
// anywhere in n's subtree.
func firstTextChild(n *rawNode) *rawNode {
	texts := n.childrenTagged("text")
	if len(texts) == 0 {
		texts = n.descendants("text")
	}
	if len(texts) == 0 {
		return nil
	}
	return texts[0]
}

// textToLabel reads one <text>/<tspan> node's own styling/content into a
// Label — everything except ID/Layer/For, which differ by caller (parseLabel
// resolves ID from its own container node and leaves For for matchLabels to
// fill in by name; parseAttachedLabel instead already knows the owning
// Element's id directly and leaves this Label's own ID at 0).
func textToLabel(t *rawNode) Label {
	x, _ := strconv.ParseFloat(t.attr("x"), 64)
	y, _ := strconv.ParseFloat(t.attr("y"), 64)
	style := t.attr("style")

	size := 0.0
	if fs := styleProp(style, "font-size"); fs != "" {
		size, _ = strconv.ParseFloat(strings.TrimSuffix(fs, "px"), 64)
	}

	var lines []string
	if s := strings.TrimSpace(t.Text); s != "" {
		lines = append(lines, s)
	}
	for _, tspan := range t.childrenTagged("tspan") {
		if s := strings.TrimSpace(tspan.Text); s != "" {
			lines = append(lines, s)
		}
	}

	return Label{
		X:      x,
		Y:      y,
		Size:   size,
		Anchor: styleProp(style, "text-anchor"),
		Bold:   strings.Contains(style, "font-weight: bold") || strings.Contains(style, "font-weight:bold"),
		Color:  styleProp(style, "fill"),
		VAlign: parseVAlign(style),
		Font:   styleProp(style, "font-family"),
		Text:   strings.Join(lines, "\n"),
	}
}

// parseLabel reads a data-type="5" group's <text>/<tspan> content into a
// Label. It does not yet know which Element it belongs to — matchLabels
// resolves that afterward by data-name, since a label's own id has no
// relation to the id of the equipment it names.
func parseLabel(n *rawNode) (Label, string, bool) {
	t := firstTextChild(n)
	if t == nil {
		return Label{}, "", false
	}
	lbl := textToLabel(t)
	lbl.ID = parseOptionalID(n)
	lbl.Layer = resolveLayer(n.attr("data-layer"))
	return lbl, n.attr("data-name"), true
}

// parseAttachedLabel reads a real xsde2svg element's own embedded <text>
// sibling — drawn via the ParamText/SubscriptName mechanism directly inside
// the same <g> Extract matched the owning element on, rather than as its
// own separately-typed data-type="5" node matchLabels resolves by
// data-name (see JunctionPoint's own parseJunctionPoint, the first user of
// this) — into a standalone Label already linked via For to that element's
// own id. forID is the owning Element's own already-parsed ID, not n's own
// id attribute: the synthesized Label deliberately leaves its own ID at 0
// (unset) rather than reusing n's, which would collide with the owning
// Element's own ID since both live on the very same <g> — relying on the
// same ensureLastId frontend backfill that already disambiguates any Label
// left at 0 (see diagramOps.ts's own doc comment) to give it a real one the
// first time this diagram is opened.
func parseAttachedLabel(n *rawNode, forID int) (Label, bool) {
	t := firstTextChild(n)
	if t == nil {
		return Label{}, false
	}
	lbl := textToLabel(t)
	lbl.Layer = resolveLayer(n.attr("data-layer"))
	lbl.For = forID
	return lbl, true
}

// parseDigitalDevice reads a data-type="134" node into a DigitalDevice.
// Unlike data-type="5" (parseLabel), a real xsde2svg-exported digital
// device carries data-type directly on the <text> node itself, not on a
// wrapping <g> — see writeDigitalDevice's own doc comment — so n is
// normally the text node already; the childrenTagged/descendants fallback
// is kept anyway in case some exporter does wrap it, the same defensive
// order parseLabel already uses.
func parseDigitalDevice(n *rawNode) (DigitalDevice, bool) {
	t := n
	if n.Tag != "text" {
		texts := n.childrenTagged("text")
		if len(texts) == 0 {
			texts = n.descendants("text")
		}
		if len(texts) == 0 {
			return DigitalDevice{}, false
		}
		t = texts[0]
	}

	x, _ := strconv.ParseFloat(t.attr("x"), 64)
	y, _ := strconv.ParseFloat(t.attr("y"), 64)
	style := t.attr("style")

	size := 0.0
	if fs := styleProp(style, "font-size"); fs != "" {
		size, _ = strconv.ParseFloat(strings.TrimSuffix(fs, "px"), 64)
	}

	return DigitalDevice{
		ID:     parseOptionalID(n),
		Layer:  resolveLayer(n.attr("data-layer")),
		X:      x,
		Y:      y,
		Size:   size,
		Anchor: styleProp(style, "text-anchor"),
		Bold:   strings.Contains(style, "font-weight: bold") || strings.Contains(style, "font-weight:bold"),
		Color:  styleProp(style, "fill"),
		VAlign: parseVAlign(style),
		Font:   styleProp(style, "font-family"),
		Name:   n.attr("data-name"),
		Value:  strings.TrimSpace(t.Text),
		Unit:   n.attr("data-unit"),
	}, true
}

// matchLabels resolves each label's owning Element by exact data-name
// match. A name shared by more than one element (or by none) is left
// unresolved: the label still renders standalone, just without a For link a
// viewer could use to highlight the equipment on hover.
func matchLabels(d *Diagram, nodes []*rawNode) []Label {
	byName := map[string][]int{}
	for _, e := range d.Elements {
		if e.Name != "" {
			byName[e.Name] = append(byName[e.Name], e.ID)
		}
	}

	labels := make([]Label, 0, len(nodes))
	for _, n := range nodes {
		lbl, name, ok := parseLabel(n)
		if !ok {
			continue
		}
		if ids := byName[name]; len(ids) == 1 {
			lbl.For = ids[0]
		}
		labels = append(labels, lbl)
	}
	return labels
}

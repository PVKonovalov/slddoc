package slddoc

import (
	"strconv"
	"strings"
)

// parseLabel reads a data-type="5" group's <text>/<tspan> content into a
// Label. It does not yet know which Element it belongs to — matchLabels
// resolves that afterward by data-name, since a label's own id has no
// relation to the id of the equipment it names.
func parseLabel(n *rawNode) (Label, string, bool) {
	texts := n.childrenTagged("text")
	if len(texts) == 0 {
		texts = n.descendants("text")
	}
	if len(texts) == 0 {
		return Label{}, "", false
	}
	t := texts[0]

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
		Layer:  resolveLayer(n.attr("data-layer")),
		X:      x,
		Y:      y,
		Size:   size,
		Anchor: styleProp(style, "text-anchor"),
		Bold:   strings.Contains(style, "font-weight: bold") || strings.Contains(style, "font-weight:bold"),
		Text:   strings.Join(lines, "\n"),
	}, n.attr("data-name"), true
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

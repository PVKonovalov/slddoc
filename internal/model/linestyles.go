package model

import "strings"

// LineHalo is how a Line composite style is drawn: a line StrokeWidth+6 wide in Color under the Line, as xsde2svg
// (element_1.go) draws its own; Gap adds a background-colored stripe StrokeWidth+2 wide inside it.
type LineHalo struct {
	Color string
	Gap   bool
}

// LineHalos maps Modus composite style names to their halos. slddoc has no built-in table: the application supplies
// it (SymbolLibrary.SetLineHalos).
type LineHalos map[string]LineHalo

// Xsde2svgLineHaloColors are the halo colors xsde2svg writes per style, by which Extract recognises halos in markup
// without data-styles.
var Xsde2svgLineHaloColors = map[string]string{
	"BadQuality": "#f7d047",
	"linked":     "#4aa42e",
	"not_linked": "#ec5d56",
	"NotNormal":  "#f6c560",
	"Выделение":  "#51b3b5",
	"Перегрузка": "#0009f5",
}

// LineStyleNames splits a Line's Styles (or a source styles="[a,b]") into names, dropping blanks.
func LineStyleNames(styles string) []string {
	styles = strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(styles), "["), "]")
	var names []string
	for name := range strings.SplitSeq(styles, ",") {
		if name = strings.TrimSpace(name); name != "" {
			names = append(names, name)
		}
	}
	return names
}

// HaloColor is e's HaloColors override for style name, or "".
func HaloColor(e Element, name string) string {
	for pair := range strings.SplitSeq(e.HaloColors, ",") {
		if k, v, found := strings.Cut(pair, ":"); found && strings.TrimSpace(k) == name {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// DropDefaultHaloColors removes from e's HaloColors the colors equal to their halos' color in halos.
func DropDefaultHaloColors(e *Element, halos LineHalos) {
	var kept []string
	for pair := range strings.SplitSeq(e.HaloColors, ",") {
		k, v, found := strings.Cut(pair, ":")
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if !found || k == "" || v == "" {
			continue
		}
		if h, ok := halos[k]; ok && strings.EqualFold(h.Color, v) {
			continue
		}
		kept = append(kept, k+":"+v)
	}
	e.HaloColors = strings.Join(kept, ",")
}

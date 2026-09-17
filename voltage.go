package slddoc

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strings"
)

// LoadVoltageHints reads a ';'-separated "hexColor;className" table (no
// header; blank lines and lines starting with "//" are ignored). A hex
// color itself starts with '#', so '#' cannot double as this format's
// comment marker the way dictionary.csv-style files often use it. See
// voltage-hints.csv at the repo root.
func LoadVoltageHints(r io.Reader) (map[string]string, error) {
	hints := map[string]string{}
	sc := bufio.NewScanner(r)
	line := 0
	for sc.Scan() {
		line++
		row := strings.TrimSpace(sc.Text())
		if row == "" || strings.HasPrefix(row, "//") {
			continue
		}
		parts := strings.SplitN(row, ";", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("slddoc: voltage hints line %d: expected 'color;name', got %q", line, row)
		}
		hints[normalizeColor(parts[0])] = strings.TrimSpace(parts[1])
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return hints, nil
}

func normalizeColor(c string) string {
	return strings.ToLower(strings.TrimSpace(c))
}

// buildVoltageClasses assigns a stable class ID to each distinct color seen
// in the diagram, naming it from hints when known and falling back to the
// color itself (a placeholder for a human to fill in, same pattern as a
// blank row in a svgtext translation CSV). Output is sorted by color for
// determinism, independent of scan order.
func buildVoltageClasses(colors []string, hints map[string]string) ([]VoltageClass, map[string]int) {
	seen := map[string]bool{}
	var distinct []string
	for _, c := range colors {
		nc := normalizeColor(c)
		if nc == "" || seen[nc] {
			continue
		}
		seen[nc] = true
		distinct = append(distinct, nc)
	}
	sort.Strings(distinct)

	classes := make([]VoltageClass, 0, len(distinct))
	colorToID := map[string]int{}
	for i, c := range distinct {
		id := i + 1
		name := hints[c]
		if name == "" {
			name = c
		}
		classes = append(classes, VoltageClass{ID: id, Name: name, Color: c})
		colorToID[c] = id
	}
	return classes, colorToID
}

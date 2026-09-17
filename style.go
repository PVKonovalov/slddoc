package slddoc

import "strings"

// styleProp reads one "name:value" declaration out of a raw SVG style
// attribute (semicolon-separated, as xsde2svg always emits it), returning
// "" if the property isn't present.
func styleProp(style, name string) string {
	for _, decl := range strings.Split(style, ";") {
		parts := strings.SplitN(decl, ":", 2)
		if len(parts) != 2 {
			continue
		}
		if strings.TrimSpace(parts[0]) == name {
			return strings.TrimSpace(parts[1])
		}
	}
	return ""
}

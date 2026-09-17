package slddoc

import (
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// SymbolLibrary holds one reusable, local-coordinate SVG template per
// Element.Shape code, used to render a Diagram's elements back into SVG
// markup. Each template is written around local origin (0,0) in the
// shape's native (unrotated) orientation; the renderer places it with
// transform="translate(x,y) rotate(orient)".
type SymbolLibrary struct {
	templates map[string]string
}

// NewSymbolLibrary builds a SymbolLibrary directly from a shape->template
// map, for a caller (internal/elements) that has already loaded and merged
// one or more element-library files.
func NewSymbolLibrary(templates map[string]string) *SymbolLibrary {
	return &SymbolLibrary{templates: templates}
}

type symbolsFile struct {
	XMLName xml.Name `xml:"symbols"`
	Symbol  []struct {
		Shape    string `xml:"shape,attr"`
		Template string `xml:"template"`
	} `xml:"symbol"`
}

// LoadSymbolLibrary reads a symbols.xml file.
func LoadSymbolLibrary(r io.Reader) (*SymbolLibrary, error) {
	var raw symbolsFile
	if err := xml.NewDecoder(r).Decode(&raw); err != nil {
		return nil, fmt.Errorf("slddoc: parsing symbol library: %w", err)
	}
	lib := &SymbolLibrary{templates: map[string]string{}}
	for _, s := range raw.Symbol {
		lib.templates[s.Shape] = strings.TrimSpace(s.Template)
	}
	return lib, nil
}

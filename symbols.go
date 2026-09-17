package slddoc

import (
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// SymbolLibrary holds one reusable, local-coordinate SVG template per
// xsde2svg shape code (Element.Shape), used to render a Diagram's elements
// back into SVG markup. Each template is written around local origin
// (0,0) in the shape's native (unrotated) orientation; the renderer places
// it with transform="translate(x,y) rotate(orient)". See symbols.xml at the
// repo root.
type SymbolLibrary struct {
	templates map[string]string
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

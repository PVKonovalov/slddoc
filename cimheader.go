package slddoc

import (
	"bytes"
	"encoding/xml"
	"io"
	"strings"
)

// CIMHeader is the md:FullModel header of one CIM document (IEC 61970-552): who made it and when, as the document
// states it. AuthoritySet is this editor's for its own export (CIMAuthoritySet).
type CIMHeader struct {
	Profiles     []string `json:"profiles,omitempty"`
	Description  string   `json:"description,omitempty"`
	AuthoritySet string   `json:"authoritySet,omitempty"`
	Created      string   `json:"created,omitempty"`
}

// CIMAuthoritySet is the modelingAuthoritySet ExportCIM writes.
const CIMAuthoritySet = cimAuthoritySet

// CIMHeaders returns the header of each document of docs that has one, in order.
func CIMHeaders(docs [][]byte) []CIMHeader {
	var out []CIMHeader
	for _, doc := range docs {
		if h, ok := cimHeader(doc); ok {
			out = append(out, h)
		}
	}
	return out
}

// CIMLayoutHeader is the header of the Diagram Layout document among headers, else the first; false when there is
// none.
func CIMLayoutHeader(headers []CIMHeader) (CIMHeader, bool) {
	for _, h := range headers {
		for _, p := range h.Profiles {
			if strings.Contains(p, "DiagramLayout") {
				return h, true
			}
		}
	}
	if len(headers) == 0 {
		return CIMHeader{}, false
	}
	return headers[0], true
}

// cimHeader reads doc's md:FullModel; false when doc has none before its first other object.
func cimHeader(doc []byte) (CIMHeader, bool) {
	var h CIMHeader
	dec := xml.NewDecoder(bytes.NewReader(doc))
	depth, in := 0, false
	var prop string
	var text strings.Builder
	for {
		tok, err := dec.Token()
		if err == io.EOF || err != nil {
			return h, in
		}
		switch tk := tok.(type) {
		case xml.StartElement:
			depth++
			switch {
			case depth == 2 && tk.Name.Local == "FullModel":
				in = true
			case depth == 2:
				return h, in
			case depth == 3 && in:
				prop = tk.Name.Local
				text.Reset()
			}
		case xml.CharData:
			if depth == 3 && in {
				text.Write(tk)
			}
		case xml.EndElement:
			if depth == 3 && in {
				v := strings.TrimSpace(text.String())
				switch prop {
				case "Model.profile":
					h.Profiles = append(h.Profiles, v)
				case "Model.description":
					h.Description = v
				case "Model.modelingAuthoritySet":
					h.AuthoritySet = v
				case "Model.created":
					h.Created = v
				}
			}
			if depth == 2 && in {
				return h, true
			}
			depth--
		}
	}
}

package render

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/PVKonovalov/slddoc/internal/model"
)

// Layers in SVG follow real xsde2svg's own "detail levels": every item off
// the base layer carries data-layer="<id>", and a <metadata> block right
// after <svg> lists the layers as {"layers":[{"id":"20","label":"Disconnectors"}]},
// which a viewer (ctrlroom's Layers switches) uses to show or hide each
// one. The base layer is never listed and its items carry no data-layer.
// This package adds two keys a viewer ignores: "z" on a layer (its
// drawing order, see Layer.Z) and "base" for the base layer's own name and
// Z when they differ from the defaults, so both survive an SVG round trip.

type metadataLayer struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Z     int    `json:"z,omitempty"`
}

type metadataBase struct {
	Label string `json:"label,omitempty"`
	Z     int    `json:"z,omitempty"`
}

type LayerMetadata struct {
	Layers []metadataLayer `json:"layers"`
	Base   *metadataBase   `json:"base,omitempty"`
}

const BaseLayerName = "Base"

// writeLayerMetadata returns the <metadata> block for d's layers, or "" when
// there is nothing beyond a default base layer to describe.
func writeLayerMetadata(d *model.Diagram) string {
	meta := LayerMetadata{Layers: []metadataLayer{}}
	for _, l := range d.Layers {
		if l.ID == model.BaseLayer {
			if (l.Name != "" && l.Name != BaseLayerName) || l.Z != 0 {
				meta.Base = &metadataBase{Label: l.Name, Z: l.Z}
			}
			continue
		}
		meta.Layers = append(meta.Layers, metadataLayer{ID: strconv.Itoa(l.ID), Label: l.Name, Z: l.Z})
	}
	if len(meta.Layers) == 0 && meta.Base == nil {
		return ""
	}
	raw, err := json.Marshal(meta)
	if err != nil {
		return ""
	}
	// Metadata is element content: escape what XML needs escaping.
	return "<metadata>\n" + textEscaper.Replace(string(raw)) + "\n</metadata>\n"
}

var textEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

var firstStartTag = regexp.MustCompile(`<[A-Za-z][\w:.-]*`)

// withItemAttrs adds data-layer="<layer>" (unless layer is BaseLayer,
// matching xsde2svg) and data-group="<group>" (unless 0) to frag's first
// start tag, the item's own root node.
func withItemAttrs(frag string, layer, group int) string {
	var attrs string
	if layer != model.BaseLayer {
		attrs += fmt.Sprintf(` data-layer="%d"`, layer)
	}
	if group != 0 {
		attrs += fmt.Sprintf(` data-group="%d"`, group)
	}
	if attrs == "" {
		return frag
	}
	loc := firstStartTag.FindStringIndex(frag)
	if loc == nil {
		return frag
	}
	return frag[:loc[1]] + attrs + frag[loc[1]:]
}

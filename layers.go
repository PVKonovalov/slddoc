package slddoc

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
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

type layerMetadata struct {
	Layers []metadataLayer `json:"layers"`
	Base   *metadataBase   `json:"base,omitempty"`
}

const baseLayerName = "Base"

// parseLayers reads an SVG's <metadata> JSON payload and prepends the
// always-present BaseLayer, so every element has somewhere to point its
// layer reference even when the source SVG never wrote a data-layer
// attribute for it.
func parseLayers(metadataText string) ([]Layer, error) {
	layers := []Layer{{ID: BaseLayer, Name: baseLayerName}}

	text := strings.TrimSpace(metadataText)
	if text == "" {
		return layers, nil
	}

	var parsed layerMetadata
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		return nil, fmt.Errorf("slddoc: parsing metadata layers: %w", err)
	}
	if parsed.Base != nil {
		if parsed.Base.Label != "" {
			layers[0].Name = parsed.Base.Label
		}
		layers[0].Z = parsed.Base.Z
	}
	for _, l := range parsed.Layers {
		id, err := strconv.Atoi(l.ID)
		if err != nil {
			return nil, fmt.Errorf("slddoc: parsing metadata layer id %q: %w", l.ID, err)
		}
		if id == BaseLayer {
			continue
		}
		layers = append(layers, Layer{ID: id, Name: l.Label, Z: l.Z})
	}
	return layers, nil
}

func resolveLayer(dataLayer string) int {
	if dataLayer == "" {
		return BaseLayer
	}
	id, err := strconv.Atoi(dataLayer)
	if err != nil {
		return BaseLayer
	}
	return id
}

// addMissingLayers gives every layer id an item references but the
// <metadata> didn't list (real corpus files often tag items with a
// detail level that has no entry) a Layer of its own, named after its id,
// so it can be shown, hidden and reassigned like any other.
func addMissingLayers(d *Diagram) {
	known := map[int]bool{}
	for _, l := range d.Layers {
		known[l.ID] = true
	}
	var missing []int
	note := func(id int) {
		if !known[id] {
			known[id] = true
			missing = append(missing, id)
		}
	}
	for _, e := range d.Elements {
		note(e.Layer)
	}
	for _, c := range d.Connectors {
		note(c.Layer)
	}
	for _, l := range d.Labels {
		note(l.Layer)
	}
	for _, dd := range d.DigitalDevices {
		note(dd.Layer)
	}
	sort.Ints(missing)
	for _, id := range missing {
		d.Layers = append(d.Layers, Layer{ID: id, Name: fmt.Sprintf("Layer %d", id)})
	}
}

// writeLayerMetadata returns the <metadata> block for d's layers, or "" when
// there is nothing beyond a default base layer to describe.
func writeLayerMetadata(d *Diagram) string {
	meta := layerMetadata{Layers: []metadataLayer{}}
	for _, l := range d.Layers {
		if l.ID == BaseLayer {
			if (l.Name != "" && l.Name != baseLayerName) || l.Z != 0 {
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

// withLayerAttr adds data-layer="<layer>" to frag's first start tag (the
// item's own root node), unless layer is BaseLayer, matching xsde2svg.
func withLayerAttr(frag string, layer int) string {
	if layer == BaseLayer {
		return frag
	}
	loc := firstStartTag.FindStringIndex(frag)
	if loc == nil {
		return frag
	}
	return frag[:loc[1]] + fmt.Sprintf(` data-layer="%d"`, layer) + frag[loc[1]:]
}

package extract

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/PVKonovalov/slddoc/internal/model"
	"github.com/PVKonovalov/slddoc/internal/render"
	"github.com/PVKonovalov/slddoc/internal/svgtree"
)

// parseLayers reads an SVG's <metadata> JSON payload and prepends the
// always-present BaseLayer, so every element has somewhere to point its
// layer reference even when the source SVG never wrote a data-layer
// attribute for it.
func parseLayers(metadataText string) ([]model.Layer, error) {
	layers := []model.Layer{{ID: model.BaseLayer, Name: render.BaseLayerName}}

	text := strings.TrimSpace(metadataText)
	if text == "" {
		return layers, nil
	}

	var parsed render.LayerMetadata
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
		if id == model.BaseLayer {
			continue
		}
		layers = append(layers, model.Layer{ID: id, Name: l.Label, Z: l.Z})
	}
	return layers, nil
}

func resolveLayer(dataLayer string) int {
	if dataLayer == "" {
		return model.BaseLayer
	}
	id, err := strconv.Atoi(dataLayer)
	if err != nil {
		return model.BaseLayer
	}
	return id
}

// addMissingLayers gives every layer id an item references but the
// <metadata> didn't list (real corpus files often tag items with a
// detail level that has no entry) a Layer of its own, named after its id,
// so it can be shown, hidden and reassigned like any other.
func addMissingLayers(d *model.Diagram) {
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
		d.Layers = append(d.Layers, model.Layer{ID: id, Name: fmt.Sprintf("Layer %d", id)})
	}
}

// parseGroup reads a node's data-group; 0 when absent or invalid.
func parseGroup(n *svgtree.RawNode) int {
	if n == nil {
		return 0
	}
	g, err := strconv.Atoi(n.Attr("data-group"))
	if err != nil || g < 0 {
		return 0
	}
	return g
}

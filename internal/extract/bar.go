package extract

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/PVKonovalov/slddoc/internal/model"
	"github.com/PVKonovalov/slddoc/internal/svgtree"
)

var fontSizeRe = regexp.MustCompile(`font-size:\s*([0-9.]+)px`)

var translateRe = regexp.MustCompile(`translate\(\s*([-+0-9.eE]+)[\s,]+([-+0-9.eE]+)\s*\)`)

// parseBarInstrument reads a bar instrument written by Render (data-type="320008") back: the values from its data-*
// attributes, the show options from whether its value text, scale and zones are present, and its colours, as
// overrides, from data-colors and the track's fill. ok is false when it has no valid min and max.
func parseBarInstrument(n *svgtree.RawNode) (model.BarInstrument, bool) {
	num := func(attr string) (float64, bool) {
		v, err := strconv.ParseFloat(strings.TrimSpace(n.Attr(attr)), 64)
		return v, err == nil
	}
	opt := func(attr string) *float64 {
		if v, ok := num(attr); ok {
			return &v
		}
		return nil
	}
	lo, okMin := num("data-min")
	hi, okMax := num("data-max")
	if !okMin || !okMax {
		return model.BarInstrument{}, false
	}
	b := model.BarInstrument{
		ID:    parseOptionalID(n),
		Layer: resolveLayer(n.Attr("data-layer")),
		Group: parseGroup(n),
		Name:  n.Attr("data-name"),
		Unit:  n.Attr("data-unit"),
		Min:   lo, Max: hi,
		LowAlarm: opt("data-la"), LowWarning: opt("data-lw"), HighWarning: opt("data-hw"), HighAlarm: opt("data-ha"),
	}
	if m := translateRe.FindStringSubmatch(n.Attr("transform")); m != nil {
		b.X, _ = strconv.ParseFloat(m[1], 64)
		b.Y, _ = strconv.ParseFloat(m[2], 64)
	}
	if n.Attr("data-orient") == model.BarVertical {
		b.Orient = model.BarVertical
	}
	b.Length, _ = num("data-len")
	b.Thickness, _ = num("data-thickness")
	if o := opt("data-origin"); o != nil && *o != lo {
		b.Origin = o
	}
	b.Deadband, _ = num("data-deadband")
	b.Value, _ = num("data-value")
	b.Precision, _ = strconv.Atoi(n.Attr("data-precision"))

	has := func(class string) *bool {
		v := false
		var walk func(*svgtree.RawNode)
		walk = func(c *svgtree.RawNode) {
			if c.Attr("class") == class {
				v = true
			}
			for _, k := range c.Children {
				walk(k)
			}
		}
		walk(n)
		return &v
	}
	b.ShowValue, b.ShowScale, b.ShowZones = has("bar-value"), has("bar-scale"), has("bar-zones")
	for _, t := range n.Descendants("text") {
		if t.Attr("class") == "bar-value" {
			if m := fontSizeRe.FindStringSubmatch(t.Attr("style")); m != nil {
				b.ValueSize, _ = strconv.ParseFloat(m[1], 64)
			}
		}
	}

	var c model.BarColors
	for part := range strings.SplitSeq(n.Attr("data-colors"), ";") {
		k, v, ok := strings.Cut(part, ":")
		if !ok {
			continue
		}
		switch strings.TrimSpace(k) {
		case "normal":
			c.Normal = strings.TrimSpace(v)
		case "warning":
			c.Warning = strings.TrimSpace(v)
		case "alarm":
			c.Alarm = strings.TrimSpace(v)
		case "bad":
			c.Bad = strings.TrimSpace(v)
		}
	}
	for _, r := range n.Descendants("rect") {
		if r.Attr("class") == "bar-track" {
			c.Track = r.Attr("fill")
		}
	}
	if c != (model.BarColors{}) {
		b.Colors = &c
	}
	return b, true
}

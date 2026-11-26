package render

import (
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/PVKonovalov/slddoc/internal/model"
)

// BarDefaults is how a bar instrument looks where it sets nothing of its own.
type BarDefaults struct {
	Length, Thickness, ValueSize    float64
	ShowValue, ShowScale, ShowZones bool
	Colors                          model.BarColors
}

// DefaultBarDefaults is used until SymbolLibrary.SetBarDefaults is called.
var DefaultBarDefaults = BarDefaults{
	Length: 80, Thickness: 4, ValueSize: 12, ShowValue: true, ShowZones: true,
	Colors: model.BarColors{Track: "#3a3f47", Normal: "lawngreen", Warning: "#c9a800", Alarm: "red", Bad: "#808080"},
}

// SetBarDefaults sets the look of bar instruments that set nothing of their own; an empty field of d keeps
// DefaultBarDefaults'.
func (l *SymbolLibrary) SetBarDefaults(d BarDefaults) {
	l.bar = &d
}

func (l *SymbolLibrary) barDefaults() BarDefaults {
	out := DefaultBarDefaults
	if l == nil || l.bar == nil {
		return out
	}
	d := *l.bar
	if d.Length > 0 {
		out.Length = d.Length
	}
	if d.Thickness > 0 {
		out.Thickness = d.Thickness
	}
	if d.ValueSize > 0 {
		out.ValueSize = d.ValueSize
	}
	out.ShowValue, out.ShowScale, out.ShowZones = d.ShowValue, d.ShowScale, d.ShowZones
	out.Colors = mergeBarColors(out.Colors, d.Colors)
	return out
}

func mergeBarColors(base, over model.BarColors) model.BarColors {
	pick := func(b, o string) string {
		if o != "" {
			return o
		}
		return b
	}
	return model.BarColors{
		Track: pick(base.Track, over.Track), Normal: pick(base.Normal, over.Normal), Warning: pick(base.Warning, over.Warning),
		Alarm: pick(base.Alarm, over.Alarm), Bad: pick(base.Bad, over.Bad),
	}
}

// BarZone is the zone of v for b's own setpoints, without hysteresis: "alarm", "warning" or "normal".
func BarZone(b model.BarInstrument, v float64) string {
	below := func(s *float64) bool { return s != nil && v < *s }
	above := func(s *float64) bool { return s != nil && v > *s }
	switch {
	case below(b.LowAlarm) || above(b.HighAlarm):
		return "alarm"
	case below(b.LowWarning) || above(b.HighWarning):
		return "warning"
	}
	return "normal"
}

// FormatBarValue is the value text: v with precision decimals, then a space and the unit when there is one.
func FormatBarValue(v float64, precision int, unit string) string {
	s := strconv.FormatFloat(v, 'f', max(precision, 0), 64)
	if unit != "" {
		s += " " + unit
	}
	return s
}

const (
	barTrackMargin = 3  // the track is this much wider than the bar on each side
	barArrowGap    = 2  // between a track end and its overflow arrowhead
	barTextGap     = 4  // between the overflow arrowhead and the value text
	barScaleSize   = 10 // scale text font size
	barScaleColor  = "#c0c0c0"
	barOriginColor = "#e0e0e0"
	barZoneOpacity = "0.35"
)

// writeBarInstrument writes b as the bar instrument SVG contract: one <g> with the data-* attributes the viewer reads,
// the axis group (rotated for a vertical one) with the track, zones, setpoint lines, origin line, bar, overflow
// arrowheads and scale, and the upright value text.
func writeBarInstrument(w io.Writer, lib *SymbolLibrary, b model.BarInstrument, mode RenderMode) {
	def := lib.barDefaults()
	length, thick := b.Length, b.Thickness
	if length <= 0 {
		length = def.Length
	}
	if thick <= 0 {
		thick = def.Thickness
	}
	show := func(v *bool, d bool) bool {
		if v != nil {
			return *v
		}
		return d
	}
	showValue, showScale, showZones := show(b.ShowValue, def.ShowValue), show(b.ShowScale, def.ShowScale), show(b.ShowZones, def.ShowZones)
	colors := def.Colors
	if b.Colors != nil {
		colors = mergeBarColors(colors, *b.Colors)
	}
	span := b.Max - b.Min
	if span <= 0 {
		span = 1
	}
	pos := func(v float64) float64 { return (math.Min(math.Max(v, b.Min), b.Min+span) - b.Min) / span * length }
	origin := b.Min
	if b.Origin != nil {
		origin = *b.Origin
	}
	zoneColor := map[string]string{"normal": colors.Normal, "warning": colors.Warning, "alarm": colors.Alarm}
	fill := zoneColor[BarZone(b, b.Value)]
	n := model.FmtNum
	orient := model.BarHorizontal
	if b.Vertical() {
		orient = model.BarVertical
	}

	var a strings.Builder
	fmt.Fprintf(&a, `id="%d" data-type="%s" data-name="%s" data-orient="%s" data-len="%s" data-thickness="%s" data-min="%s" data-max="%s" data-origin="%s"`,
		b.ID, model.BarInstrumentShape, esc(b.Name), orient, n(length), n(thick), n(b.Min), n(b.Max), n(origin))
	setpoints := []struct {
		attr  string
		v     *float64
		color string
	}{{"la", b.LowAlarm, colors.Alarm}, {"lw", b.LowWarning, colors.Warning}, {"hw", b.HighWarning, colors.Warning}, {"ha", b.HighAlarm, colors.Alarm}}
	for _, s := range setpoints {
		if s.v != nil {
			fmt.Fprintf(&a, ` data-%s="%s"`, s.attr, n(*s.v))
		}
	}
	if b.Deadband != 0 {
		fmt.Fprintf(&a, ` data-deadband="%s"`, n(b.Deadband))
	}
	fmt.Fprintf(&a, ` data-precision="%d"`, b.Precision)
	if b.Unit != "" {
		fmt.Fprintf(&a, ` data-unit="%s"`, esc(b.Unit))
	}
	fmt.Fprintf(&a, ` data-value="%s" data-colors="%s" transform="translate(%s,%s)"`, n(b.Value),
		esc(fmt.Sprintf("normal:%s;warning:%s;alarm:%s;bad:%s", colors.Normal, colors.Warning, colors.Alarm, colors.Bad)), n(b.X), n(b.Y))
	if mode == Interactive {
		a.WriteString(` data-editor-kind="barinstrument"`)
	}
	fmt.Fprintf(w, "<g %s>\n", a.String())

	half := thick/2 + barTrackMargin
	if b.Vertical() {
		fmt.Fprint(w, "<g class=\"bar-axis\" transform=\"rotate(-90)\">\n")
	} else {
		fmt.Fprint(w, "<g class=\"bar-axis\">\n")
	}
	if mode == Interactive {
		m := half + barArrowGap
		fmt.Fprintf(w, "<rect x=\"%s\" y=\"%s\" width=\"%s\" height=\"%s\" style=\"fill:transparent;stroke:none\"/>\n", n(-m), n(-m), n(length+2*m), n(2*m))
	}
	fmt.Fprintf(w, "<rect class=\"bar-track\" x=\"0\" y=\"%s\" width=\"%s\" height=\"%s\" fill=\"%s\"/>\n", n(-half), n(length), n(2*half), esc(colors.Track))

	hidden := func(v *float64) string {
		if v == nil {
			return ` visibility="hidden"`
		}
		return ""
	}
	at := func(v *float64, unset float64) float64 {
		if v == nil {
			return unset
		}
		return pos(*v)
	}
	if showZones {
		zones := []struct {
			class    string
			from, to float64
			color    string
			v        *float64
		}{
			{"la", 0, at(b.LowAlarm, 0), colors.Alarm, b.LowAlarm},
			{"lw", at(b.LowAlarm, 0), at(b.LowWarning, 0), colors.Warning, b.LowWarning},
			{"hw", at(b.HighWarning, length), at(b.HighAlarm, length), colors.Warning, b.HighWarning},
			{"ha", at(b.HighAlarm, length), length, colors.Alarm, b.HighAlarm},
		}
		fmt.Fprint(w, "<g class=\"bar-zones\">\n")
		for _, z := range zones {
			fmt.Fprintf(w, "<rect class=\"bar-zone-%s\" x=\"%s\" y=\"%s\" width=\"%s\" height=\"%s\" fill=\"%s\" fill-opacity=\"%s\"%s/>\n",
				z.class, n(z.from), n(-half), n(math.Max(z.to-z.from, 0)), n(2*half), esc(z.color), barZoneOpacity, hidden(z.v))
		}
		fmt.Fprint(w, "</g>\n")
	}

	fmt.Fprint(w, "<g class=\"bar-limits\">\n")
	for _, s := range setpoints {
		x := n(at(s.v, 0))
		fmt.Fprintf(w, "<line class=\"bar-limit-%s\" x1=\"%s\" y1=\"%s\" x2=\"%s\" y2=\"%s\" stroke=\"%s\" stroke-width=\"1.5\"%s/>\n",
			s.attr, x, n(-half), x, n(half), esc(s.color), hidden(s.v))
	}
	fmt.Fprint(w, "</g>\n")

	o, p := pos(origin), pos(b.Value)
	if origin != b.Min {
		fmt.Fprintf(w, "<line class=\"bar-origin\" x1=\"%s\" y1=\"%s\" x2=\"%s\" y2=\"%s\" stroke=\"%s\" stroke-width=\"1\"/>\n",
			n(o), n(-half-barArrowGap), n(o), n(half+barArrowGap), barOriginColor)
	}
	fmt.Fprintf(w, "<rect class=\"bar-fill\" x=\"%s\" y=\"%s\" width=\"%s\" height=\"%s\" fill=\"%s\"/>\n",
		n(math.Min(o, p)), n(-thick/2), n(math.Abs(p-o)), n(thick), esc(fill))

	visible := func(on bool) string {
		if on {
			return "visible"
		}
		return "hidden"
	}
	end := length + barArrowGap
	fmt.Fprintf(w, "<path class=\"bar-over-max\" visibility=\"%s\" d=\"M %s %s L %s 0 L %s %s Z\" fill=\"%s\"/>\n",
		visible(b.Value > b.Max), n(end), n(-half), n(end+half), n(end), n(half), esc(colors.Alarm))
	fmt.Fprintf(w, "<path class=\"bar-over-min\" visibility=\"%s\" d=\"M %s %s L %s 0 L %s %s Z\" fill=\"%s\"/>\n",
		visible(b.Value < b.Min), n(-barArrowGap), n(-half), n(-barArrowGap-half), n(-barArrowGap), n(half), esc(colors.Alarm))

	if showScale {
		fmt.Fprintf(w, "<g class=\"bar-scale\" style=\"font-size:%dpx;font-family:Arial;fill:%s\">\n", barScaleSize, barScaleColor)
		ticks := []float64{b.Min, b.Max}
		if origin != b.Min && origin != b.Max {
			ticks = append(ticks, origin)
		}
		y := half + barArrowGap
		for _, t := range ticks {
			x := pos(t)
			if b.Vertical() {
				fmt.Fprintf(w, "<text x=\"%s\" y=\"%s\" transform=\"rotate(90,%s,%s)\" style=\"text-anchor:start;dominant-baseline:middle\">%s</text>\n",
					n(x), n(y), n(x), n(y), n(t))
			} else {
				fmt.Fprintf(w, "<text x=\"%s\" y=\"%s\" style=\"text-anchor:middle;dominant-baseline:text-before-edge\">%s</text>\n", n(x), n(y), n(t))
			}
		}
		fmt.Fprint(w, "</g>\n")
	}
	fmt.Fprint(w, "</g>\n")

	if showValue {
		text := esc(FormatBarValue(b.Value, b.Precision, b.Unit))
		size := b.ValueSize
		if size <= 0 {
			size = def.ValueSize
		}
		style := fmt.Sprintf("font-size:%spx;font-family:Arial", n(size))
		if b.Vertical() {
			fmt.Fprintf(w, "<text class=\"bar-value\" x=\"0\" y=\"%s\" fill=\"%s\" style=\"%s;text-anchor:middle;dominant-baseline:text-before-edge\">%s</text>\n",
				n(barArrowGap+half+barTextGap), esc(fill), style, text)
		} else {
			fmt.Fprintf(w, "<text class=\"bar-value\" x=\"%s\" y=\"0\" fill=\"%s\" style=\"%s;text-anchor:start;dominant-baseline:middle\">%s</text>\n",
				n(end+half+barTextGap), esc(fill), style, text)
		}
	}
	fmt.Fprint(w, "</g>\n")
}

func writeBarInstrumentLayered(w io.Writer, lib *SymbolLibrary, b model.BarInstrument, mode RenderMode) {
	if b.Layer == model.BaseLayer && b.Group == 0 {
		writeBarInstrument(w, lib, b, mode)
		return
	}
	var buf strings.Builder
	writeBarInstrument(&buf, lib, b, mode)
	io.WriteString(w, withItemAttrs(buf.String(), b.Layer, b.Group))
}

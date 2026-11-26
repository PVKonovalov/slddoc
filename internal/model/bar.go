package model

import (
	"errors"
	"fmt"
	"strings"
)

// BarInstrumentShape is the bar instrument's type code, written as data-type.
const BarInstrumentShape = "320008"

// Bar instrument orientations.
const (
	BarHorizontal = "h" // grows along +x
	BarVertical   = "v" // grows upward
)

// BarInstrument shows a measured value as a bar along a track from Min to Max, coloured by the zone the value is in
// (see the setpoints). It is this project's own item, not a Modus shape.
type BarInstrument struct {
	ID    int `xml:"id,attr" json:"id"`
	Layer int `xml:"layer,attr" json:"layer"`
	Group int `xml:"group,attr,omitempty" json:"group,omitempty"`
	// X, Y is the track's min end, on its axis.
	X float64 `xml:"x,attr" json:"x"`
	Y float64 `xml:"y,attr" json:"y"`
	// Orient is BarHorizontal (also when empty) or BarVertical.
	Orient string `xml:"orient,attr,omitempty" json:"orient,omitempty"`
	// Length and Thickness are the track's length and the bar's thickness; 0 takes the renderer's default.
	Length    float64 `xml:"length,attr,omitempty" json:"length,omitempty"`
	Thickness float64 `xml:"thickness,attr,omitempty" json:"thickness,omitempty"`
	Name      string  `xml:"name,attr,omitempty" json:"name,omitempty"`
	Unit      string  `xml:"unit,attr,omitempty" json:"unit,omitempty"`
	Min       float64 `xml:"min,attr" json:"min"`
	Max       float64 `xml:"max,attr" json:"max"`
	// Origin is where the bar starts; nil: Min.
	Origin *float64 `xml:"origin,attr,omitempty" json:"origin,omitempty"`
	// The setpoints set in the editor; nil: not set. A bound setpoint (BindingSetpointLA...) overrides its value at
	// run time.
	LowAlarm    *float64 `xml:"lowAlarm,attr,omitempty" json:"lowAlarm,omitempty"`
	LowWarning  *float64 `xml:"lowWarning,attr,omitempty" json:"lowWarning,omitempty"`
	HighWarning *float64 `xml:"highWarning,attr,omitempty" json:"highWarning,omitempty"`
	HighAlarm   *float64 `xml:"highAlarm,attr,omitempty" json:"highAlarm,omitempty"`
	// Deadband is the hysteresis of zone changes, applied by the viewer.
	Deadband float64 `xml:"deadband,attr,omitempty" json:"deadband,omitempty"`
	// Value is the current (default) value shown until live data arrives; it may lie outside Min..Max.
	Value float64 `xml:"value,attr" json:"value"`
	// ValueSize is the value text's font size; 0 takes the renderer's default.
	ValueSize float64 `xml:"valueSize,attr,omitempty" json:"valueSize,omitempty"`
	// ShowValue, ShowScale and ShowZones: nil takes the renderer's default.
	ShowValue *bool `xml:"showValue,attr,omitempty" json:"showValue,omitempty"`
	ShowScale *bool `xml:"showScale,attr,omitempty" json:"showScale,omitempty"`
	ShowZones *bool `xml:"showZones,attr,omitempty" json:"showZones,omitempty"`
	// Colors overrides the renderer's default colours; an empty one keeps the default.
	Colors *BarColors `xml:"colors,omitempty" json:"colors,omitempty"`
	// Precision, Features, ComponentType, ComponentEdited and Bindings are as in Element.
	Precision       int       `xml:"precision,attr,omitempty" json:"precision,omitempty"`
	Features        string    `xml:"features,attr,omitempty" json:"features,omitempty"`
	ComponentType   string    `xml:"componentType,attr,omitempty" json:"componentType,omitempty"`
	ComponentEdited bool      `xml:"componentEdited,attr,omitempty" json:"componentEdited,omitempty"`
	Bindings        []Binding `xml:"bindings>binding,omitempty" json:"bindings,omitempty"`
}

// BarColors are a bar instrument's colours: the track, and the bar and value text in each zone.
type BarColors struct {
	Track   string `xml:"track,attr,omitempty" json:"track,omitempty"`
	Normal  string `xml:"normal,attr,omitempty" json:"normal,omitempty"`
	Warning string `xml:"warning,attr,omitempty" json:"warning,omitempty"`
	Alarm   string `xml:"alarm,attr,omitempty" json:"alarm,omitempty"`
	Bad     string `xml:"bad,attr,omitempty" json:"bad,omitempty"`
}

// Vertical tells whether b grows upward.
func (b BarInstrument) Vertical() bool { return b.Orient == BarVertical }

// ValidateBarInstrument checks b's values: Min < Max; the set setpoints ordered LowAlarm ≤ LowWarning ≤ HighWarning
// ≤ HighAlarm; the setpoints and Origin within Min..Max; Deadband ≥ 0; Length, Thickness and ValueSize not negative;
// a known Orient. It returns all problems joined into one error, or nil.
func ValidateBarInstrument(b BarInstrument) error {
	var errs []error
	if b.Min >= b.Max {
		errs = append(errs, fmt.Errorf("min %s is not less than max %s", FmtNum(b.Min), FmtNum(b.Max)))
	}
	named := []struct {
		name string
		v    *float64
	}{{"origin", b.Origin}, {"lowAlarm", b.LowAlarm}, {"lowWarning", b.LowWarning}, {"highWarning", b.HighWarning}, {"highAlarm", b.HighAlarm}}
	for _, n := range named {
		if n.v != nil && b.Min < b.Max && (*n.v < b.Min || *n.v > b.Max) {
			errs = append(errs, fmt.Errorf("%s %s is outside %s..%s", n.name, FmtNum(*n.v), FmtNum(b.Min), FmtNum(b.Max)))
		}
	}
	prev := -1
	for i, n := range named[1:] {
		if n.v == nil {
			continue
		}
		if prev >= 0 && *named[prev+1].v > *n.v {
			errs = append(errs, fmt.Errorf("%s %s is greater than %s %s", named[prev+1].name, FmtNum(*named[prev+1].v), n.name, FmtNum(*n.v)))
		}
		prev = i
	}
	if b.Deadband < 0 {
		errs = append(errs, fmt.Errorf("deadband %s is negative", FmtNum(b.Deadband)))
	}
	if b.Length < 0 || b.Thickness < 0 {
		errs = append(errs, errors.New("length and thickness must be positive"))
	}
	if b.ValueSize < 0 {
		errs = append(errs, fmt.Errorf("value size %s is negative", FmtNum(b.ValueSize)))
	}
	if b.Orient != "" && b.Orient != BarHorizontal && b.Orient != BarVertical {
		errs = append(errs, fmt.Errorf("unknown orient %q", b.Orient))
	}
	return errors.Join(errs...)
}

// ValidateBarInstruments checks every bar instrument of d (ValidateBarInstrument) and returns one warning per
// invalid instrument. Nothing is changed: a stored invalid instrument is kept as it is.
func ValidateBarInstruments(d *Diagram) []string {
	var out []string
	for _, b := range d.BarInstruments {
		if err := ValidateBarInstrument(b); err != nil {
			out = append(out, fmt.Sprintf("bar instrument %d: %s", b.ID, strings.ReplaceAll(err.Error(), "\n", "; ")))
		}
	}
	return out
}

package model

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

//go:fix inline
func fp(v float64) *float64 { return new(v) }

//go:fix inline
func bp(v bool) *bool { return new(v) }

func TestBarInstrumentRoundTrip(t *testing.T) {
	full := BarInstrument{
		ID: 7, Layer: BaseLayer, Group: 9, X: 10, Y: 20.5, Orient: BarVertical, Length: 120, Thickness: 6,
		Name: "U", Unit: "kV", Min: -50, Max: 150, Origin: new(float64(0)),
		LowAlarm: new(float64(-40)), LowWarning: new(float64(-30)), HighWarning: new(float64(121)), HighAlarm: new(float64(126)), Deadband: 1.5, Value: 123.4, ValueSize: 14.5,
		ShowValue: new(true), ShowScale: new(false), ShowZones: new(true),
		Colors:    &BarColors{Track: "#111", Normal: "green", Warning: "yellow", Alarm: "red", Bad: "gray"},
		Precision: 1, Features: "1", ComponentType: "5", ComponentEdited: true,
		Bindings: []Binding{
			{Source: "rsdu", Kind: BindingMeas1, Ref: "1:2"},
			{Source: "rsdu", Kind: BindingSetpointHA, Ref: "1:3", Option: "GLT_ANALOG_OPT_INSTANT"},
		},
	}
	required := BarInstrument{ID: 8, Layer: BaseLayer, Min: 0, Max: 100}
	for name, b := range map[string]BarInstrument{"full": full, "required": required} {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := (&Diagram{Layers: []Layer{{ID: BaseLayer}}, BarInstruments: []BarInstrument{b}}).Save(&buf); err != nil {
				t.Fatal(err)
			}
			got, err := Load(&buf)
			if err != nil {
				t.Fatal(err)
			}
			if len(got.BarInstruments) != 1 || !reflect.DeepEqual(got.BarInstruments[0], b) {
				t.Errorf("round trip:\n got %+v\nwant %+v\n%s", got.BarInstruments, b, buf.String())
			}
		})
	}

	var buf bytes.Buffer
	if err := (&Diagram{BarInstruments: []BarInstrument{required}}).Save(&buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `<barInstrument id="8" layer="0" x="0" y="0" min="0" max="100" value="0"/>`) {
		t.Errorf("a bare instrument is not one self-closing tag:\n%s", buf.String())
	}
	buf.Reset()
	if err := (&Diagram{}).Save(&buf); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "barInstruments") {
		t.Errorf("an empty list is written:\n%s", buf.String())
	}
}

func TestValidateBarInstrument(t *testing.T) {
	ok := BarInstrument{Min: 0, Max: 100, LowAlarm: new(float64(5)), LowWarning: new(float64(10)), HighAlarm: new(float64(95)), Origin: new(float64(0)), Value: 150}
	if err := ValidateBarInstrument(ok); err != nil {
		t.Errorf("valid instrument: %v", err)
	}
	for _, c := range []struct {
		name string
		edit func(*BarInstrument)
		want string
	}{
		{"min not less than max", func(b *BarInstrument) { b.Max = 0 }, "min 0 is not less than max 0"},
		{"setpoints out of order", func(b *BarInstrument) { b.HighWarning = new(float64(99)) }, "highWarning 99 is greater than highAlarm 95"},
		{"unset setpoint skipped", func(b *BarInstrument) { b.LowWarning = new(float64(96)) }, "lowWarning 96 is greater than highAlarm 95"},
		{"setpoint outside range", func(b *BarInstrument) { b.LowAlarm = new(float64(-1)) }, "lowAlarm -1 is outside 0..100"},
		{"origin outside range", func(b *BarInstrument) { b.Origin = new(float64(101)) }, "origin 101 is outside 0..100"},
		{"negative deadband", func(b *BarInstrument) { b.Deadband = -1 }, "deadband -1 is negative"},
		{"negative length", func(b *BarInstrument) { b.Length = -1 }, "length and thickness must be positive"},
		{"unknown orient", func(b *BarInstrument) { b.Orient = "x" }, `unknown orient "x"`},
		{"negative value size", func(b *BarInstrument) { b.ValueSize = -2 }, "value size -2 is negative"},
	} {
		b := ok
		c.edit(&b)
		err := ValidateBarInstrument(b)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want %q", c.name, err, c.want)
		}
	}

	d := &Diagram{BarInstruments: []BarInstrument{ok, {ID: 4, Min: 1, Max: 1, Deadband: -1}}}
	w := ValidateBarInstruments(d)
	if len(w) != 1 || w[0] != "bar instrument 4: min 1 is not less than max 1; deadband -1 is negative" {
		t.Errorf("warnings: %q", w)
	}
}

func TestSetpointBindingsOnlyOnBar(t *testing.T) {
	sp := Binding{Source: "rsdu", Kind: BindingSetpointLA, Ref: "1:2"}
	d := &Diagram{
		BarInstruments: []BarInstrument{{ID: 1, Min: 0, Max: 1, Bindings: []Binding{sp}}},
		DigitalDevices: []DigitalDevice{{ID: 2, Bindings: []Binding{sp}}},
	}
	err := ValidateBindings(d)
	if err == nil || err.Error() != `digital device 2: "setpointLA" binding is only for a bar instrument` {
		t.Errorf("got %v", err)
	}
}

package extract

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/PVKonovalov/slddoc/internal/model"
	"github.com/PVKonovalov/slddoc/internal/render"
)

func TestBarInstrumentRoundTrip(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	b := func(v bool) *bool { return &v }
	colors := &model.BarColors{Track: "#3a3f47", Normal: "lawngreen", Warning: "#c9a800", Alarm: "red", Bad: "#808080"}
	for name, in := range map[string]model.BarInstrument{
		"all": {
			ID: 7, Layer: 3, Group: 0, X: 10, Y: 20.5, Orient: model.BarVertical, Length: 120, Thickness: 6,
			Name: "U", Unit: "kV", Min: -50, Max: 150, Origin: f(0),
			LowAlarm: f(-40), LowWarning: f(-30), HighWarning: f(121), HighAlarm: f(126), Deadband: 1.5, Value: 123.4, ValueSize: 16,
			ShowValue: b(true), ShowScale: b(true), ShowZones: b(false), Precision: 2,
			Colors: &model.BarColors{Track: "#111", Normal: "green", Warning: "yellow", Alarm: "#f00", Bad: "gray"},
		},
		"defaults": {ID: 8, Min: 0, Max: 100, Length: 80, Thickness: 4, Value: 50, ValueSize: 12, ShowValue: b(true), ShowScale: b(false), ShowZones: b(true), Colors: colors},
	} {
		t.Run(name, func(t *testing.T) {
			d := &model.Diagram{Layers: []model.Layer{{ID: model.BaseLayer}, {ID: 3}}, BarInstruments: []model.BarInstrument{in}}
			var buf bytes.Buffer
			if err := render.Render(d, render.NewSymbolLibrary(nil), &buf, render.Static, "", nil); err != nil {
				t.Fatal(err)
			}
			got, report, err := Extract(buf.Bytes(), "test.svg", nil)
			if err != nil {
				t.Fatal(err)
			}
			if report.BarInstruments != 1 || len(got.BarInstruments) != 1 {
				t.Fatalf("report %+v, instruments %+v", report, got.BarInstruments)
			}
			if !reflect.DeepEqual(got.BarInstruments[0], in) {
				t.Errorf("round trip:\n got %+v\nwant %+v\n%s", got.BarInstruments[0], in, buf.String())
			}
		})
	}
}

package model

import (
	"bytes"
	"strings"
	"testing"
)

func TestElectricalRoundTrip(t *testing.T) {
	d := &Diagram{
		Width: 100, Height: 100,
		Elements: []Element{
			{ID: 1, Class: ClassBreaker, Shape: "41", Electrical: &Electrical{RatedCurrent: new(1250.0)}},
			{ID: 2, Class: ClassPowerTransformer, Shape: "47", Windings: []TransformerWinding{
				{Scheme: SchemeWyeN, RatedU: new(115.0), RatedS: new(25.0), R: new(2.54), X: new(55.9), PhaseAngleClock: new(0)},
				{Scheme: SchemeDelta, RatedU: new(11.0), PhaseAngleClock: new(11)},
			}},
			{ID: 3, Class: ClassCapacitor, Shape: "8", Electrical: &Electrical{MaximumSections: new(2), NormalSections: new(1)}},
		},
		Connectors: []Connector{{ID: 4, Kind: KindOverheadLine, From: 5, To: 6,
			Points:     []Point{{0, 0}, {10, 0}},
			Electrical: &Electrical{Length: new(12.5), R: new(3.1), X: new(5.2), Bch: new(0.0)},
		}},
	}
	var buf bytes.Buffer
	if err := d.Save(&buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `<electrical length="12.5" r="3.1" x="5.2" bch="0"/>`) {
		t.Errorf("line parameters not written as expected:\n%s", buf.String())
	}
	got, err := Load(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if e := got.Elements[0].Electrical; e == nil || e.RatedCurrent == nil || *e.RatedCurrent != 1250 {
		t.Errorf("breaker ratedCurrent lost: %+v", e)
	}
	w := got.Elements[1].Windings
	if len(w) != 2 || w[0].X == nil || *w[0].X != 55.9 || w[1].PhaseAngleClock == nil || *w[1].PhaseAngleClock != 11 || w[1].R != nil {
		t.Errorf("winding parameters mismatch: %+v", w)
	}
	if w[0].PhaseAngleClock == nil || *w[0].PhaseAngleClock != 0 {
		t.Errorf("a zero clock number must round-trip as set: %+v", w[0])
	}
	if e := got.Elements[2].Electrical; e == nil || e.MaximumSections == nil || *e.MaximumSections != 2 || e.NomU != nil {
		t.Errorf("shunt parameters mismatch: %+v", e)
	}
	if e := got.Connectors[0].Electrical; e == nil || e.Bch == nil || *e.Bch != 0 || e.Gch != nil {
		t.Errorf("line parameters mismatch: %+v", e)
	}
	if got.Elements[1].Electrical != nil {
		t.Errorf("an element without parameters must stay without them")
	}
}

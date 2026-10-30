package cim

import (
	"strings"
	"testing"

	"github.com/PVKonovalov/slddoc/internal/model"
)

func TestCIMWindingVoltages(t *testing.T) {
	eq := transformerUnderBaysEQ()
	trs, err := CIMSameVoltageTransformers([][]byte{[]byte(eq)})
	if err != nil {
		t.Fatal(err)
	}
	if len(trs) != 1 || trs[0].Name != "T" || len(trs[0].Windings) != 2 || trs[0].Windings[0].KV != 33 ||
		trs[0].Windings[1].MRID != "Te2" || trs[0].Windings[1].End != 2 {
		t.Fatalf("transformers = %+v", trs)
	}

	terms := map[string][]model.Point{"41": {{X: 0, Y: -10}, {X: 0, Y: 10}}, "162": {{X: 0, Y: -10}, {X: 0, Y: 10}},
		"330001": {{X: 0, Y: 0}}, "330002": {{X: 0, Y: 0}}}
	pages, err := ImportCIM([][]byte{[]byte(eq)}, testImport(CIMImportOptions{Terminals: terms, Grid: 10,
		WindingVoltages: map[string]float64{"Te2": 11}}))
	if err != nil {
		t.Fatal(err)
	}
	d := pages[0].Diagram
	class := map[int]string{}
	for _, vc := range d.VoltageClasses {
		class[vc.ID] = vc.Name
	}
	got := map[string]string{}
	for _, e := range d.Elements {
		got[e.Name] = class[e.Voltage]
		if e.Class == model.ClassPowerTransformer {
			if len(e.Windings) != 2 || class[e.Windings[0].Voltage] != "33 kV" || class[e.Windings[1].Voltage] != "11 kV" {
				t.Errorf("windings = %+v (%v)", e.Windings, class)
			}
		}
	}
	for name, want := range map[string]string{"B": "11 kV", "Q1": "11 kV", "Q2": "11 kV", "L1": "11 kV", "D": "33 kV", "G": "33 kV"} {
		if got[name] != want {
			t.Errorf("%s: voltage %q, want %q", name, got[name], want)
		}
	}
	if again, _ := CIMSameVoltageTransformers([][]byte{[]byte(eq)}); len(again) != 1 {
		t.Errorf("the documents themselves must stay as they are: %+v", again)
	}

	// A side joined back to the other winding keeps its voltage; only the winding takes the new one.
	ring := strings.Replace(eq, "</rdf:RDF>", `<cim:Breaker rdf:ID="_QR"><cim:IdentifiedObject.name>QR</cim:IdentifiedObject.name><cim:Equipment.EquipmentContainer rdf:resource="#_v"/></cim:Breaker>
<cim:Terminal rdf:ID="_t-QR-1"><cim:Terminal.ConductingEquipment rdf:resource="#_QR"/><cim:Terminal.ConnectivityNode rdf:resource="#_bus"/><cim:ACDCTerminal.sequenceNumber>1</cim:ACDCTerminal.sequenceNumber></cim:Terminal>
<cim:Terminal rdf:ID="_t-QR-2"><cim:Terminal.ConductingEquipment rdf:resource="#_QR"/><cim:Terminal.ConnectivityNode rdf:resource="#_in"/><cim:ACDCTerminal.sequenceNumber>2</cim:ACDCTerminal.sequenceNumber></cim:Terminal>
</rdf:RDF>`, 1)
	pages, err = ImportCIM([][]byte{[]byte(ring)}, testImport(CIMImportOptions{Terminals: terms, Grid: 10,
		WindingVoltages: map[string]float64{"Te2": 11}}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(pages[0].Warnings, ";"), "only the winding takes 11 kV") {
		t.Errorf("warnings = %v", pages[0].Warnings)
	}
}

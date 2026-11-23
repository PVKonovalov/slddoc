package model

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestFunctionsRoundTrip(t *testing.T) {
	d := &Diagram{
		Width: 100, Height: 100,
		Editor:         &EditorSettings{DesignationFormat: DesignationShort, DesignationLayer: 9},
		VoltageClasses: []VoltageClass{{ID: 1, Name: "110 kV", Color: "#ff0000"}},
		Elements:       []Element{{ID: 5, Class: ClassBreaker, Shape: "41", Name: "=E1=H03-QA1", Function: 12}},
		Functions: []Function{
			{ID: 11, Kind: FunctionVoltageLevel, Code: "E", Number: 1, Voltage: 1},
			{ID: 12, Parent: 11, Kind: FunctionOutgoingFeeder, Code: "H", Number: 3, Digits: 2, Guessed: true},
			{ID: 13, Parent: 11, Kind: FunctionTransformer, Code: "T", Number: 1, Pair: 14},
		},
	}
	var buf bytes.Buffer
	if err := d.Save(&buf); err != nil {
		t.Fatal(err)
	}
	xml := buf.String()
	for _, want := range []string{
		`<function id="12" parent="11" kind="outgoingFeeder" code="H" number="3" digits="2" guessed="true"/>`,
		`function="12"`, `designationFormat="short"`, `designationLayer="9"`,
	} {
		if !strings.Contains(xml, want) {
			t.Errorf("saved XML lacks %s", want)
		}
	}
	got, err := Load(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Functions, d.Functions) || got.Elements[0].Function != 12 || *got.Editor != *d.Editor {
		t.Errorf("round trip lost functions: %+v", got.Functions)
	}
}

func TestLoad_NoFunctions(t *testing.T) {
	d, err := Load(strings.NewReader(`<diagram width="10" height="10"><elements><element id="1" class="Breaker" shape="41" layer="0" x="0" y="0"/></elements></diagram>`))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Functions) != 0 || d.Elements[0].Function != 0 {
		t.Errorf("a file without functions should load with none: %+v", d.Functions)
	}
	var buf bytes.Buffer
	if err := d.Save(&buf); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "functions") {
		t.Error("an empty hierarchy should not be written")
	}
}

func TestFunctionDesignation(t *testing.T) {
	fs := []Function{
		{ID: 1, Kind: FunctionVoltageLevel, Code: "E", Number: 1},
		{ID: 2, Parent: 1, Kind: FunctionOutgoingFeeder, Code: "H", Number: 3, Digits: 2},
		{ID: 3, Parent: 4, Code: "A", Number: 1},
		{ID: 4, Parent: 3, Code: "B", Number: 2},
	}
	for id, want := range map[int]string{2: "=E1=H03", 1: "=E1", 9: "", 3: "=B2=A1"} {
		if got := FunctionDesignation(fs, id); got != want {
			t.Errorf("FunctionDesignation(%d) = %q, want %q", id, got, want)
		}
	}
	if got := ProductPart("=E1=T1-QA1"); got != "-QA1" {
		t.Errorf("ProductPart = %q", got)
	}
}

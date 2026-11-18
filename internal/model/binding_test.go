package model

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func boundDiagram() *Diagram {
	return &Diagram{
		Width: 100, Height: 100,
		Scheme: &SchemeRef{Source: "rsdu", Ref: "4512"},
		Object: &Binding{Source: "rsdu", Kind: BindingEquipment, Ref: "1001:7", Name: "Substation"},
		Layers: []Layer{{ID: BaseLayer, Name: "Base"}},
		Elements: []Element{{
			ID: 3, Class: ClassBreaker, Shape: "41", Layer: BaseLayer, X: 10, Y: 20,
			Precision: 2, Features: "F1,F2",
			Bindings: []Binding{
				{Source: "rsdu", Kind: BindingEquipment, Ref: "1023:5531", Option: "GLT_OBJECT_OPT_COMPLEX", Name: "Q1", Type: "OTYP_SWITCH", UID: "0b6f"},
				{Source: "rsdu", Kind: BindingMeas1, Ref: "2011:88231", Option: "GLT_BOOL_OPT_INSTANT", Scale: -1},
			},
		}},
		DigitalDevices: []DigitalDevice{{
			ID: 6, Layer: BaseLayer, X: 5, Y: 6, Size: 16, Value: "0.00", Precision: 3,
			Bindings: []Binding{{Source: "rsdu", Kind: BindingMeas1, Ref: "2011:9", Option: "GLT_ANALOG_OPT_INSTANT", Scale: 1000}},
		}},
	}
}

func TestBindingsXMLRoundTrip(t *testing.T) {
	d := boundDiagram()
	var buf bytes.Buffer
	if err := d.Save(&buf); err != nil {
		t.Fatal(err)
	}
	saved := buf.String()
	for _, want := range []string{
		`<scheme source="rsdu" ref="4512"/>`,
		`<binding source="rsdu" kind="meas1" ref="2011:88231" option="GLT_BOOL_OPT_INSTANT" scale="-1"/>`,
		`precision="2" features="F1,F2"`,
	} {
		if !strings.Contains(saved, want) {
			t.Errorf("saved .xsld lacks %s:\n%s", want, saved)
		}
	}
	got, err := Load(strings.NewReader(saved))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Scheme, d.Scheme) || !reflect.DeepEqual(got.Object, d.Object) {
		t.Errorf("scheme/object: got %+v %+v", got.Scheme, got.Object)
	}
	e := got.Elements[0]
	if !reflect.DeepEqual(e.Bindings, d.Elements[0].Bindings) || e.Precision != 2 || e.Features != "F1,F2" {
		t.Errorf("element: got %+v", e)
	}
	if !reflect.DeepEqual(got.DigitalDevices[0].Bindings, d.DigitalDevices[0].Bindings) || got.DigitalDevices[0].Precision != 3 {
		t.Errorf("digital device: got %+v", got.DigitalDevices[0])
	}
}

func TestBindingsJSONRoundTrip(t *testing.T) {
	d := boundDiagram()
	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"bindings":[{"source":"rsdu","kind":"equipment","ref":"1023:5531"`) {
		t.Errorf("unexpected JSON: %s", raw)
	}
	var got Diagram
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Elements[0].Bindings, d.Elements[0].Bindings) || !reflect.DeepEqual(got.Object, d.Object) {
		t.Errorf("JSON round trip lost bindings: %s", raw)
	}
}

func TestBindingsAbsentWriteNothing(t *testing.T) {
	d := &Diagram{
		Width: 100, Height: 100,
		Layers:         []Layer{{ID: BaseLayer, Name: "Base"}},
		Elements:       []Element{{ID: 3, Class: ClassBreaker, Shape: "41", Layer: BaseLayer}},
		DigitalDevices: []DigitalDevice{{ID: 6, Layer: BaseLayer, Size: 16, Value: "0"}},
	}
	var first bytes.Buffer
	if err := d.Save(&first); err != nil {
		t.Fatal(err)
	}
	for _, unwanted := range []string{"<bindings", "<binding", "<scheme", "<object", "precision=", "features="} {
		if strings.Contains(first.String(), unwanted) {
			t.Errorf("diagram without bindings writes %s:\n%s", unwanted, first.String())
		}
	}
	if !strings.Contains(first.String(), `<digitalDevice id="6" layer="0" x="0" y="0" size="16" value="0"/>`) {
		t.Errorf("digital device without bindings is not self-closing:\n%s", first.String())
	}
	loaded, err := Load(bytes.NewReader(first.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	var second bytes.Buffer
	if err := loaded.Save(&second); err != nil {
		t.Fatal(err)
	}
	if first.String() != second.String() {
		t.Errorf("load/save changed a diagram without bindings:\n%s\n---\n%s", first.String(), second.String())
	}
}

func TestValidateBindings(t *testing.T) {
	if err := ValidateBindings(boundDiagram()); err != nil {
		t.Fatalf("valid diagram: %v", err)
	}
	cases := []struct {
		name string
		edit func(d *Diagram)
		want string
	}{
		{"unknown kind", func(d *Diagram) { d.Elements[0].Bindings[0].Kind = "topology" }, `element 3: unknown binding kind "topology"`},
		{"empty ref", func(d *Diagram) { d.DigitalDevices[0].Bindings[0].Ref = "" }, `digital device 6: "meas1" binding: empty source or ref`},
		{"empty source", func(d *Diagram) { d.Object.Source = "" }, `diagram object: "equipment" binding: empty source or ref`},
		{"duplicate kind", func(d *Diagram) { d.Elements[0].Bindings[1].Kind = BindingEquipment }, `element 3: more than one "equipment" binding`},
		{"empty scheme", func(d *Diagram) { d.Scheme.Ref = "" }, "diagram scheme: empty source or ref"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := boundDiagram()
			c.edit(d)
			err := ValidateBindings(d)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("got %v, want %q", err, c.want)
			}
		})
	}
}

func TestSchemeExportedRoundTrip(t *testing.T) {
	d := boundDiagram()
	at := time.Date(2026, 11, 17, 10, 30, 0, 0, time.UTC)
	d.Scheme.Exported = &at
	d.Scheme.Registered = true
	var buf bytes.Buffer
	if err := d.Save(&buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `<scheme source="rsdu" ref="4512" exported="2026-11-17T10:30:00Z" registered="true"></scheme>`) &&
		!strings.Contains(buf.String(), `<scheme source="rsdu" ref="4512" exported="2026-11-17T10:30:00Z" registered="true"/>`) {
		t.Errorf("scheme not written as expected:\n%s", buf.String())
	}
	got, err := Load(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if got.Scheme.Exported == nil || !got.Scheme.Exported.Equal(at) || !got.Scheme.Registered {
		t.Errorf("exported = %v", got.Scheme.Exported)
	}
	head, err := ReadScheme(bytes.NewReader(buf.Bytes()))
	if err != nil || head == nil || head.Ref != "4512" || head.Exported == nil || !head.Exported.Equal(at) || !head.Registered {
		t.Errorf("ReadScheme = %+v, %v", head, err)
	}
	j, _ := json.Marshal(d.Scheme)
	if string(j) != `{"source":"rsdu","ref":"4512","exported":"2026-11-17T10:30:00Z","registered":true}` {
		t.Errorf("json %s", j)
	}
}

func TestReadScheme(t *testing.T) {
	plain := boundDiagram()
	plain.Scheme = nil
	var none bytes.Buffer
	if err := plain.Save(&none); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, xml, ref string
		err            bool
	}{
		{"after editor", `<?xml version="1.0"?><diagram width="1" height="1"><editor grid="10"><x/></editor><scheme source="rsdu" ref="7"/><layers/></diagram>`, "7", false},
		{"no scheme", none.String(), "", false},
		{"stops at layers", `<diagram><layers><layer id="0"/></layers><scheme source="rsdu" ref="9"/></diagram>`, "", false},
		{"not a diagram", `<svg/>`, "", true},
		{"broken", `<diagram><editor>`, "", true},
	}
	for _, c := range cases {
		got, err := ReadScheme(strings.NewReader(c.xml))
		if (err != nil) != c.err {
			t.Errorf("%s: err %v", c.name, err)
			continue
		}
		ref := ""
		if got != nil {
			ref = got.Ref
		}
		if ref != c.ref {
			t.Errorf("%s: ref %q, want %q", c.name, ref, c.ref)
		}
	}
}

// TestLabelConnectorBindings: labels and connectors keep bindings, precision and features through Save and Load, a
// bound label's text without the indentation around its <bindings>, an unbound one's exactly.
func TestLabelConnectorBindings(t *testing.T) {
	jump := Binding{Source: "rsdu", Kind: BindingJump, Ref: "43:2000", Option: "GLT_OBJECT_OPT_COMPLEX", Name: "Panel"}
	line := Binding{Source: "rsdu", Kind: BindingEquipment, Ref: "1024:7", Name: "Line segment"}
	d := &Diagram{
		Width: 100, Height: 100,
		Layers: []Layer{{ID: BaseLayer, Name: "Base"}},
		Labels: []Label{
			{ID: 1, Layer: BaseLayer, X: 1, Y: 2, Size: 12, Text: "Panel\nsecond line", Precision: 1, Features: "F1", ComponentType: "2000076", Bindings: []Binding{jump}},
			{ID: 2, Layer: BaseLayer, X: 1, Y: 2, Size: 12, Text: "free\n  "},
		},
		Connectors: []Connector{{ID: 5, Kind: KindBusWork, Layer: BaseLayer, From: 8, To: 9, ComponentType: "1489",
			Points: []Point{{0, 0}, {10, 0}}, Bindings: []Binding{line}}},
		Nodes: []Node{{ID: 8, X: 0, Y: 0}, {ID: 9, X: 10, Y: 0}},
	}
	if err := ValidateBindings(d); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := d.Save(&buf); err != nil {
		t.Fatal(err)
	}
	got, err := Load(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatal(err)
	}
	l := got.Labels[0]
	if l.Text != "Panel\nsecond line" || l.Precision != 1 || l.Features != "F1" || l.ComponentType != "2000076" || !reflect.DeepEqual(l.Bindings, []Binding{jump}) {
		t.Errorf("bound label: %q %+v\n%s", l.Text, l, buf.String())
	}
	if got.Labels[1].Text != "free\n  " || got.Labels[1].Bindings != nil {
		t.Errorf("unbound label: %q", got.Labels[1].Text)
	}
	if !reflect.DeepEqual(got.Connectors[0].Bindings, []Binding{line}) || got.Connectors[0].ComponentType != "1489" {
		t.Errorf("connector: %+v", got.Connectors[0])
	}

	d.Connectors[0].Bindings = append(d.Connectors[0].Bindings, line)
	d.Labels[0].Bindings = append(d.Labels[0].Bindings, Binding{Kind: BindingJump})
	if err := ValidateBindings(d); err == nil || !strings.Contains(err.Error(), "connector 5") || !strings.Contains(err.Error(), "label 1") {
		t.Errorf("validate: %v", err)
	}
}

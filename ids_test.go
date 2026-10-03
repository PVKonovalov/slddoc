package slddoc

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAssignUniqueIDs(t *testing.T) {
	d := &Diagram{
		Layers:         []Layer{{ID: 0}, {ID: 9}},
		VoltageClasses: []VoltageClass{{ID: 1, Color: "#f00"}},
		Nodes:          []Node{{ID: 1}, {ID: 2}},
		Elements: []Element{
			{ID: 1, Voltage: 1, Ports: []Port{{Name: "1", Node: 1}}},
			{ID: 5, Ports: []Port{{Name: "1", Node: 2}}},
		},
		Connectors:     []Connector{{ID: 2, Voltage: 1, From: 1, To: 2}},
		DigitalDevices: []DigitalDevice{{ID: 5}},
		Labels:         []Label{{ID: 0, For: 5}},
		Editor:         &EditorSettings{DefaultVoltage: 1},
	}
	next := assignUniqueIDs(d, 9)
	seen := map[int]bool{0: true, 9: true}
	check := func(what string, id int) {
		if seen[id] {
			t.Errorf("%s id %d is not unique", what, id)
		}
		seen[id] = true
	}
	for _, n := range d.Nodes {
		check("node", n.ID)
	}
	for _, v := range d.VoltageClasses {
		check("voltage class", v.ID)
	}
	for _, e := range d.Elements {
		check("element", e.ID)
	}
	for _, c := range d.Connectors {
		check("connector", c.ID)
	}
	for _, dd := range d.DigitalDevices {
		check("digital device", dd.ID)
	}
	for _, l := range d.Labels {
		check("label", l.ID)
	}
	vc := d.VoltageClasses[0].ID
	if d.Elements[0].Voltage != vc || d.Connectors[0].Voltage != vc || d.Editor.DefaultVoltage != vc {
		t.Errorf("voltage references not remapped to %d: %+v %+v %+v", vc, d.Elements[0], d.Connectors[0], d.Editor)
	}
	if d.Elements[0].Ports[0].Node != d.Nodes[0].ID || d.Connectors[0].From != d.Nodes[0].ID || d.Connectors[0].To != d.Nodes[1].ID {
		t.Errorf("node references not remapped")
	}
	if d.Elements[1].ID != 5 || d.Labels[0].For != 5 {
		t.Errorf("element 5 must keep its id (a label points at it)")
	}
	if next <= 9 {
		t.Errorf("next = %d, want past every assigned id", next)
	}
}

// Every id an extracted corpus diagram uses is unique.
func TestExtractCorpusIDsUnique(t *testing.T) {
	files, _ := filepath.Glob("../sld-svg/examples/sld/*.svg")
	if len(files) == 0 {
		t.Skip("corpus not checked out next to this repo")
	}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		d, _, err := Extract(raw, "", nil)
		if err != nil {
			continue
		}
		seen := map[int]string{}
		add := func(kind string, id int) {
			if id == 0 {
				t.Errorf("%s: %s with id 0", filepath.Base(f), kind)
				return
			}
			if prev, ok := seen[id]; ok {
				t.Errorf("%s: id %d used by both %s and %s", filepath.Base(f), id, prev, kind)
			}
			seen[id] = kind
		}
		for _, x := range d.Nodes {
			add("node", x.ID)
		}
		for _, x := range d.VoltageClasses {
			add("voltage class", x.ID)
		}
		for _, x := range d.Elements {
			add("element", x.ID)
		}
		for _, x := range d.Connectors {
			add("connector", x.ID)
		}
		for _, x := range d.DigitalDevices {
			add("digital device", x.ID)
		}
		for _, x := range d.Labels {
			add("label", x.ID)
		}
		for id := range seen {
			if id > d.LastID {
				t.Errorf("%s: id %d above lastId %d", filepath.Base(f), id, d.LastID)
				break
			}
		}
	}
}

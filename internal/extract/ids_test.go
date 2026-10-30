package extract

import (
	"os"
	"path/filepath"
	"testing"
)

// Every id an extracted corpus diagram uses is unique.
func TestExtractCorpusIDsUnique(t *testing.T) {
	files, _ := filepath.Glob("../../../sld-svg/examples/sld/*.svg")
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

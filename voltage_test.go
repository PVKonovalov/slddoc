package slddoc

import (
	"strings"
	"testing"
)

func TestBuildVoltageClasses(t *testing.T) {
	hints := map[string]string{"#962896": "10кВ"}
	classes, colorToID := buildVoltageClasses([]string{"#962896", "#326400", "", "#962896"}, hints)

	if len(classes) != 2 {
		t.Fatalf("got %d classes, want 2 (blank colors dropped, duplicates collapsed): %+v", len(classes), classes)
	}
	// Sorted by color: "#326400" < "#962896".
	if classes[0].Color != "#326400" || classes[0].Name != "#326400" {
		t.Errorf("class 0 = %+v, want color #326400 with no hint so name falls back to the color", classes[0])
	}
	if classes[1].Color != "#962896" || classes[1].Name != "10кВ" {
		t.Errorf("class 1 = %+v, want the hinted name 10кВ", classes[1])
	}
	if colorToID["#962896"] != classes[1].ID {
		t.Errorf("colorToID mismatch: %+v", colorToID)
	}
}

func TestLoadVoltageHints(t *testing.T) {
	hints, err := LoadVoltageHints(strings.NewReader("// comment\n#962896;10кВ\n#326400;6кВ\n\n"))
	if err != nil {
		t.Fatal(err)
	}
	if hints["#962896"] != "10кВ" || hints["#326400"] != "6кВ" || len(hints) != 2 {
		t.Errorf("hints = %+v", hints)
	}
}

func TestLoadVoltageHints_Malformed(t *testing.T) {
	if _, err := LoadVoltageHints(strings.NewReader("not-a-valid-line")); err == nil {
		t.Fatal("expected an error for a line without a ';'")
	}
}

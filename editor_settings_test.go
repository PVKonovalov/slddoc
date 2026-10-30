package slddoc

import (
	"bytes"
	"strings"
	"testing"
)

func TestEditorSettings_JumpSameVoltageRoundTrip(t *testing.T) {
	for _, c := range []struct {
		value *bool
		attr  string
	}{
		{new(true), `jumpSameVoltage="true"`},
		{new(false), `jumpSameVoltage="false"`},
		{nil, ""},
	} {
		d := &Diagram{Width: 100, Height: 100, Editor: &EditorSettings{GridSpacing: 10, JumpSameVoltage: c.value}}
		var buf bytes.Buffer
		if err := d.Save(&buf); err != nil {
			t.Fatal(err)
		}
		saved := buf.String()
		if c.attr != "" && !strings.Contains(saved, c.attr) || c.attr == "" && strings.Contains(saved, "jumpSameVoltage") {
			t.Fatalf("saved %v as:\n%s", c.value, saved)
		}
		back, err := Load(&buf)
		if err != nil {
			t.Fatal(err)
		}
		got := back.Editor.JumpSameVoltage
		if (got == nil) != (c.value == nil) || got != nil && *got != *c.value {
			t.Errorf("%v loaded as %v", c.value, got)
		}
	}
}

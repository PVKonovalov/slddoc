package slddoc

import (
	"bytes"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	state := 1
	d := &Diagram{
		Width: 100, Height: 200, Source: "example.xml",
		LastID:         5,
		Editor:         &EditorSettings{GridSpacing: 20, Snap: true, Background: "#12161d"},
		Layers:         []Layer{{ID: BaseLayer, Name: "Base"}, {ID: 10, Name: "Groups"}},
		VoltageClasses: []VoltageClass{{ID: 1, Name: "10kV", Color: "#962896"}},
		Nodes:          []Node{{ID: 2, X: 900, Y: 240}},
		Elements: []Element{{
			ID: 3, Class: ClassBreaker, Shape: "41", Name: "CB-1",
			Voltage: 1, Layer: BaseLayer, X: 900, Y: 660, State: &state,
			Ports: []Port{{Name: "1", Node: 2}},
		}},
		Connectors: []Connector{{
			// KindOverheadLine, not KindBusbarWire — Load rewrites the
			// latter to KindBusWork (see its own doc comment), which
			// would make this round-trip test's own choice of Kind
			// silently misleading.
			ID: 4, Kind: KindOverheadLine, Voltage: 1, Layer: BaseLayer,
			From: 2, To: 2, Points: []Point{{X: 900, Y: 240}, {X: 900, Y: 270}},
		}},
		Labels:         []Label{{For: 3, Layer: BaseLayer, X: 1, Y: 2, Size: 13, Text: "CB-1"}},
		DigitalDevices: []DigitalDevice{{ID: 6, Layer: BaseLayer, X: 5, Y: 6, Size: 16, Name: "R T-1", Value: "0.00", Unit: "MW"}},
	}

	var buf bytes.Buffer
	if err := d.Save(&buf); err != nil {
		t.Fatal(err)
	}
	saved := buf.String()

	// The Breaker element above has no Points (only BusBarSection uses
	// them) — encoding/xml's own omitempty is silently ignored for a
	// chained "parent>child" tag like Element.Points' own "geometry>point",
	// so without emptyPathWrapperLine's own fix this would still carry a
	// meaningless empty <geometry/>.
	if bytes.Contains(buf.Bytes(), []byte("<geometry")) {
		t.Errorf("saved XML should not carry an empty <geometry> wrapper for an element with no points: %s", saved)
	}

	got, err := Load(&buf)
	if err != nil {
		t.Fatalf("Load: %v\nXML was:\n%s", err, saved)
	}

	if got.Width != d.Width || got.Source != d.Source || got.LastID != d.LastID {
		t.Errorf("diagram header mismatch: %+v", got)
	}
	if got.Editor == nil || got.Editor.GridSpacing != 20 || !got.Editor.Snap || got.Editor.Background != "#12161d" {
		t.Errorf("editor settings mismatch: %+v", got.Editor)
	}
	if len(got.Layers) != 2 || got.Layers[1].ID != 10 {
		t.Errorf("layers mismatch: %+v", got.Layers)
	}
	if len(got.VoltageClasses) != 1 || got.VoltageClasses[0].Color != "#962896" {
		t.Errorf("voltage classes mismatch: %+v", got.VoltageClasses)
	}
	if len(got.Elements) != 1 || got.Elements[0].State == nil || *got.Elements[0].State != 1 {
		t.Errorf("elements mismatch: %+v", got.Elements)
	}
	if len(got.Elements[0].Ports) != 1 || got.Elements[0].Ports[0].Node != 2 {
		t.Errorf("ports mismatch: %+v", got.Elements[0].Ports)
	}
	if len(got.Connectors) != 1 || len(got.Connectors[0].Points) != 2 || got.Connectors[0].Kind != KindOverheadLine {
		t.Errorf("connectors mismatch: %+v", got.Connectors)
	}
	if len(got.Labels) != 1 || got.Labels[0].For != 3 {
		t.Errorf("labels mismatch: %+v", got.Labels)
	}
	if len(got.DigitalDevices) != 1 || got.DigitalDevices[0].Name != "R T-1" || got.DigitalDevices[0].Unit != "MW" {
		t.Errorf("digital devices mismatch: %+v", got.DigitalDevices)
	}
}

// TestLoad_RewritesLegacyBusbarWire covers Extract's own former bug (fixed
// alongside this test): connectorKindByType misread a real data-type="21"
// (Ошиновка/Buswork) connector as KindBusbarWire instead of KindBusWork —
// "21" was always KindBusWork's own code, never KindBusbarWire's (which
// has none at all — see connectorKindByType's own doc comment). The
// frontend itself never creates a KindBusbarWire connector, so every one
// already saved to disk by that bug is rewritten back to KindBusWork here
// on Load, the same way kindObjectLinkLegacy already is.
func TestLoad_RewritesLegacyBusbarWire(t *testing.T) {
	d := &Diagram{
		Width: 100, Height: 100,
		Connectors: []Connector{{ID: 1, Kind: KindBusbarWire, Points: []Point{{X: 0, Y: 0}, {X: 1, Y: 1}}}},
	}
	var buf bytes.Buffer
	if err := d.Save(&buf); err != nil {
		t.Fatal(err)
	}
	got, err := Load(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Connectors) != 1 || got.Connectors[0].Kind != KindBusWork {
		t.Errorf("Load should rewrite a stored KindBusbarWire connector to KindBusWork, got %+v", got.Connectors)
	}
}

// TestSave_OmitsEmptyPathWrapperTags covers every "parent>child" xml tag in
// this model (see emptyPathWrapperLine's own doc comment) with nothing in
// it, and confirms none of their empty wrapper elements survive into the
// saved XML — not even collapsed to self-closing, entirely absent — while
// a genuinely populated one (a BusBarSection's own Points, a
// PowerTransformer's own Windings) still round-trips.
func TestSave_OmitsEmptyPathWrapperTags(t *testing.T) {
	d := &Diagram{
		Width: 10, Height: 10,
		Elements: []Element{
			{ID: 1, Class: ClassBusBarSection, Points: []Point{{X: 0, Y: 0}, {X: 10, Y: 0}}},
			{ID: 2, Class: ClassLamp, Shape: "106"},
			{ID: 3, Class: ClassPowerTransformer, Shape: "47", Windings: []TransformerWinding{{Scheme: SchemeWye}, {Scheme: SchemeWye}}},
			{ID: 4, Class: ClassBreaker, Shape: "41"},
			{ID: 5, Class: ClassTable2, Shape: "313", RowHeights: []float64{20}, ColumnWidths: []float64{60}, Cells: []TableCell{{Row: 0, Col: 0}}},
		},
	}

	var buf bytes.Buffer
	if err := d.Save(&buf); err != nil {
		t.Fatal(err)
	}
	saved := buf.String()

	for _, tag := range []string{"<layers", "<voltageClasses", "<nodes", "<connectors", "<labels", "<digitalDevices"} {
		if bytes.Contains(buf.Bytes(), []byte(tag)) {
			t.Errorf("saved XML should omit the empty %s wrapper entirely: %s", tag, saved)
		}
	}
	// The Lamp (no Points) must not carry an empty <geometry/>, but the
	// BusBarSection (real Points) must still carry a real one.
	if bytes.Count(buf.Bytes(), []byte("<geometry")) != 1 {
		t.Errorf("expected exactly one real <geometry> (the busbar's), got: %s", saved)
	}
	// The Breaker (no Windings) must not carry an empty <windings/>, but the
	// PowerTransformer (real Windings) must still carry a real one.
	if bytes.Count(buf.Bytes(), []byte("<windings")) != 1 {
		t.Errorf("expected exactly one real <windings> (the transformer's), got: %s", saved)
	}
	// The Breaker (no RowHeights/ColumnWidths/Cells) must not carry empty
	// <rows/>/<columns/>/<cells/>, but the Table2 (real ones) must still
	// carry real ones.
	for _, tag := range []string{"<rows", "<columns", "<cells"} {
		if bytes.Count(buf.Bytes(), []byte(tag)) != 1 {
			t.Errorf("expected exactly one real %s (the table2's), got: %s", tag, saved)
		}
	}

	got, err := Load(&buf)
	if err != nil {
		t.Fatalf("Load: %v\nXML was:\n%s", err, saved)
	}
	var busbar, lamp, table2 Element
	for _, e := range got.Elements {
		switch e.ID {
		case 1:
			busbar = e
		case 2:
			lamp = e
		case 5:
			table2 = e
		}
	}
	if len(table2.RowHeights) != 1 || len(table2.ColumnWidths) != 1 || len(table2.Cells) != 1 {
		t.Errorf("table2 should still round-trip its own real grid: %+v", table2)
	}
	if len(busbar.Points) != 2 {
		t.Errorf("busbar should still round-trip its own real points: %+v", busbar)
	}
	if len(lamp.Points) != 0 {
		t.Errorf("lamp should have no points: %+v", lamp)
	}
	if len(got.Labels) != 0 || len(got.Connectors) != 0 || len(got.DigitalDevices) != 0 || len(got.VoltageClasses) != 0 {
		t.Errorf("everything omitted for being empty should still load back empty, not error: %+v", got)
	}
}

func TestSaveLoadRoundTrip_NoEditorSettings(t *testing.T) {
	d := &Diagram{Width: 10, Height: 10}

	var buf bytes.Buffer
	if err := d.Save(&buf); err != nil {
		t.Fatal(err)
	}

	got, err := Load(&buf)
	if err != nil {
		t.Fatalf("Load: %v\nXML was:\n%s", err, buf.String())
	}
	if got.Editor != nil {
		t.Errorf("Editor = %+v, want nil for a diagram saved without one", got.Editor)
	}
}

// TestSaveLoadRoundTrip_Table2 covers Element's own RowHeights/
// ColumnWidths ([]float64, marshaled as repeated child elements rather
// than a struct-wrapped list the way Points/Windings are) and Cells
// ([]TableCell) round-tripping through Save/Load intact.
func TestSaveLoadRoundTrip_Table2(t *testing.T) {
	d := &Diagram{
		Width: 100, Height: 100,
		Elements: []Element{{
			ID: 1, Class: ClassTable2, Shape: "313",
			X: 10, Y: 20,
			RowHeights:   []float64{20, 30},
			ColumnWidths: []float64{60, 50, 40},
			Cells: []TableCell{
				{Row: 0, Col: 0, Text: "A"},
				{Row: 0, Col: 1, Text: "B", Fill: "red", TextColor: "white"},
			},
		}},
	}

	var buf bytes.Buffer
	if err := d.Save(&buf); err != nil {
		t.Fatal(err)
	}
	got, err := Load(&buf)
	if err != nil {
		t.Fatalf("Load: %v\nXML was:\n%s", err, buf.String())
	}
	if len(got.Elements) != 1 {
		t.Fatalf("elements = %+v, want 1", got.Elements)
	}
	e := got.Elements[0]
	if len(e.RowHeights) != 2 || e.RowHeights[0] != 20 || e.RowHeights[1] != 30 {
		t.Errorf("RowHeights = %v, want [20 30]", e.RowHeights)
	}
	if len(e.ColumnWidths) != 3 || e.ColumnWidths[0] != 60 || e.ColumnWidths[1] != 50 || e.ColumnWidths[2] != 40 {
		t.Errorf("ColumnWidths = %v, want [60 50 40]", e.ColumnWidths)
	}
	if len(e.Cells) != 2 {
		t.Fatalf("Cells = %+v, want 2 entries", e.Cells)
	}
	if e.Cells[0].Row != 0 || e.Cells[0].Col != 0 || e.Cells[0].Text != "A" {
		t.Errorf("Cells[0] = %+v, want {Row:0 Col:0 Text:A}", e.Cells[0])
	}
	if e.Cells[1].Fill != "red" || e.Cells[1].TextColor != "white" {
		t.Errorf("Cells[1] = %+v, want Fill:red TextColor:white", e.Cells[1])
	}
}

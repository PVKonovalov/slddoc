package slddoc

import "testing"

const testDiagramSVG = `<?xml version="1.0"?>
<svg width="2000" height="1000" style='stroke-width: 0px; background-color: #12161d;' xmlns="http://www.w3.org/2000/svg">
<metadata>
{"layers":[{"id":"10","label":"Контейнеры"}]}
</metadata>
<polyline points="810,240 1320,240" style="fill:none;stroke:#326400;stroke-width:2" data-voltage="#326400" data-name="СШ 6 кВ" data-type="24" id="1" />
<circle cx="900" cy="240" r="3" style="fill:#12161d;stroke:#326400;stroke-width:1" data-type="7" id="2" data-voltage="#12161d" />
<polyline points="900,240 900,280" style="fill:none;stroke:#326400;stroke-width:1" data-type="21" id="3" data-voltage="#326400" />
<g id="4" data-voltage="#326400" data-type="41" data-name="Breaker 1" >
<g>
<path d="M 893 283 h 14 v 14 h -14 z" style="fill:lawngreen;stroke:#326400;stroke-width:1" />
<path d="M 900 285 v 10" data-state="1" style="stroke:#326400;stroke-width:1" />
<path d="M 900 280 v 3 M 900 300 v -3" style="stroke:#326400;stroke-width:1" />
</g>
</g>
<g data-type="5" data-name="Breaker 1" data-event="dc" id="2674">
<text x="910" y="293" style="fill:white;text-anchor:start;font-size:13px;font-family:Arial;white-space: pre;" >Breaker 1</text>
</g>
<text x="1876" y="759" style="fill:darkturquoise;text-anchor:end;dominant-baseline:middle;font-size:16px;font-family:Arial ;font-weight: bold" data-type="134" id="148704875" data-name="R T-1 10" data-unit="МВт" >0.00 <tspan style="fill:darkturquoise;text-anchor:end;dominant-baseline:middle;font-size:16px;font-family:Arial ;font-weight: bold" >MW</tspan></text>
<polyline points="0,0 5,0" style="fill:none;stroke:black;stroke-width:1" data-type="1" id="99" />
</svg>
`

func TestExtract_EndToEnd(t *testing.T) {
	d, report, err := Extract([]byte(testDiagramSVG), "test.svg", map[string]string{"#326400": "6кВ"})
	if err != nil {
		t.Fatal(err)
	}

	if len(d.Layers) != 2 || d.Layers[1].ID != 10 {
		t.Errorf("layers = %+v", d.Layers)
	}
	if len(d.VoltageClasses) != 1 || d.VoltageClasses[0].Name != "6кВ" {
		t.Errorf("voltage classes = %+v", d.VoltageClasses)
	}
	if report.Elements != 4 {
		t.Errorf("report.Elements = %d, want 4 (bus, point, breaker, line)", report.Elements)
	}
	if report.Connectors != 1 {
		t.Errorf("report.Connectors = %d, want 1", report.Connectors)
	}
	if report.Labels != 1 {
		t.Errorf("report.Labels = %d, want 1", report.Labels)
	}
	if report.Nodes != 3 {
		t.Errorf("report.Nodes = %d, want 3 (bus+point+wire-start merged, wire-end+port1 merged, port2 alone)", report.Nodes)
	}

	var breaker Element
	for _, e := range d.Elements {
		if e.Class == ClassBreaker {
			breaker = e
		}
	}
	if breaker.ID == 0 {
		t.Fatalf("no breaker in %+v", d.Elements)
	}
	if breaker.Ports[0].Node == breaker.Ports[1].Node {
		t.Errorf("breaker's two ports must resolve to different nodes: %+v", breaker.Ports)
	}

	if len(d.Labels) != 1 || d.Labels[0].For != breaker.ID {
		t.Errorf("label should match the breaker by name: %+v", d.Labels)
	}
	if d.Labels[0].ID != 2674 {
		t.Errorf("label should keep its own real source id, got %+v", d.Labels[0])
	}
	if d.Labels[0].Color != "white" || d.Labels[0].Font != "Arial" || d.Labels[0].VAlign != "" {
		t.Errorf("label should keep its own real fill/font/baseline, got %+v", d.Labels[0])
	}

	if report.DigitalDevices != 1 || len(d.DigitalDevices) != 1 {
		t.Errorf("report.DigitalDevices = %d, len(d.DigitalDevices) = %d, want 1 each", report.DigitalDevices, len(d.DigitalDevices))
	} else {
		dd := d.DigitalDevices[0]
		if dd.ID != 148704875 || dd.Name != "R T-1 10" || dd.Value != "0.00" || dd.Unit != "МВт" || dd.Anchor != "end" || !dd.Bold {
			t.Errorf("digital device = %+v", dd)
		}
		// A real digital device's own per-instance color/baseline matter a
		// lot (status indication) — dropping these silently defaulted every
		// extracted one to white/bottom, which is what surfaced this bug.
		if dd.Color != "darkturquoise" || dd.Font != "Arial" || dd.VAlign != "middle" {
			t.Errorf("digital device should keep its own real fill/font/baseline, got %+v", dd)
		}
	}

	// The decorative border (data-type="1", a plain line) is a purely
	// decorative annotation (ClassLine), not skipped.
	if len(report.Skipped) != 0 {
		t.Errorf("report.Skipped = %+v, want none", report.Skipped)
	}
	var line Element
	for _, e := range d.Elements {
		if e.Class == ClassLine {
			line = e
		}
	}
	if line.ID == 0 {
		t.Fatalf("no line in %+v", d.Elements)
	}
	if len(line.Points) != 2 || line.Points[0] != (Point{X: 0, Y: 0}) || line.Points[1] != (Point{X: 5, Y: 0}) {
		t.Errorf("line.Points = %+v, want [(0,0) (5,0)]", line.Points)
	}
	if line.Stroke != "black" || line.StrokeWidth != 1 {
		t.Errorf("line stroke/width = %q/%v, want black/1", line.Stroke, line.StrokeWidth)
	}
}

// testDigitalDeviceBackgroundSVG matches real xsde2svg output (element134):
// a bare, untyped, id-less <rect> immediately preceding a data-type="134"
// <text> is that reading's own colored background — see
// parseDigitalDeviceBackgroundRect's own doc comment for why it's recovered
// as a standalone Rectangle rather than a field on DigitalDevice itself. The
// second reading has no preceding rect at all (an older export, or simply
// one instance among many not carrying one), the third is preceded by an
// ordinary real Rectangle (its own id) that must NOT be swept in as a
// background just because it happens to sit next to an unrelated reading,
// and the fourth has its own second background rect — checked to make sure
// two synthesized rects get distinct ids, not colliding with each other.
const testDigitalDeviceBackgroundSVG = `<?xml version="1.0"?>
<svg width="600" height="600" style='stroke-width: 0px; background-color: #12161d;' xmlns="http://www.w3.org/2000/svg">
<rect x="570" y="630" width="60" height="30" style="fill:#FFFFCC;stroke:#777777;stroke-width:1" />
<text x="620" y="647" style="fill:#333333;text-anchor:end;dominant-baseline:middle;font-size:19px;font-family:Arial " data-type="134" id="148793466" data-name="Ia" data-unit="" data-voltage="#777777" >0 </text>
<text x="620" y="677" style="fill:#333333;text-anchor:end;dominant-baseline:middle;font-size:19px;font-family:Arial " data-type="134" id="148793467" data-name="Ib" data-unit="" >0 </text>
<rect x="10" y="10" width="40" height="20" style="fill:red;stroke:black;stroke-width:1" data-type="3" id="500" />
<text x="60" y="27" style="fill:#333333;text-anchor:end;dominant-baseline:middle;font-size:19px;font-family:Arial " data-type="134" id="501" data-name="Ic" data-unit="" >0 </text>
<rect x="510" y="660" width="60" height="30" style="fill:#99FF99;stroke:#777777;stroke-width:1" />
<text x="560" y="677" style="fill:#333333;text-anchor:end;dominant-baseline:middle;font-size:19px;font-family:Arial " data-type="134" id="148793468" data-name="Id" data-unit="" >0 </text>
</svg>
`

func TestExtract_DigitalDeviceBackgroundRect(t *testing.T) {
	d, report, err := Extract([]byte(testDigitalDeviceBackgroundSVG), "test.svg", nil)
	if err != nil {
		t.Fatal(err)
	}

	if report.DigitalDevices != 4 || len(d.DigitalDevices) != 4 {
		t.Fatalf("report.DigitalDevices = %d, len(d.DigitalDevices) = %d, want 4 each", report.DigitalDevices, len(d.DigitalDevices))
	}

	var rects []Element
	for _, e := range d.Elements {
		if e.Class == ClassRectangle {
			rects = append(rects, e)
		}
	}
	// Exactly three Rectangles: the real shape-3 one (id 500, untouched) and
	// the two synthesized from Ia's/Id's own background rects. The second
	// reading (no preceding rect) and third (preceded by a real, unrelated
	// Rectangle) must not produce or reuse a phantom background.
	if len(rects) != 3 {
		t.Fatalf("elements with ClassRectangle = %+v, want 3", rects)
	}

	var real Element
	var synthesized []Element
	for _, r := range rects {
		if r.ID == 500 {
			real = r
		} else {
			synthesized = append(synthesized, r)
		}
	}
	if real.ID == 0 || real.Fill != "red" {
		t.Errorf("real Rectangle (id 500) should extract normally, got %+v", rects)
	}
	if len(synthesized) != 2 {
		t.Fatalf("synthesized background Rectangles = %+v, want 2", synthesized)
	}
	// Each must be a real, non-zero id that collides with nothing else
	// already in the document (id 0 doubles as "unset" elsewhere in this
	// model) and, critically, not with EACH OTHER either — the highest real
	// id in this fixture is 148793468 (the last reading, Id), so both
	// synthesized ids must land above that, and must differ from each other.
	maxRealID := 148793468
	if synthesized[0].ID <= maxRealID || synthesized[1].ID <= maxRealID {
		t.Errorf("synthesized Rectangles should get ids above every real id in the document (>%d), got %d and %d", maxRealID, synthesized[0].ID, synthesized[1].ID)
	}
	if synthesized[0].ID == synthesized[1].ID {
		t.Errorf("two synthesized background Rectangles must not collide on the same id, both got %d", synthesized[0].ID)
	}
	wantLastID := synthesized[0].ID
	if synthesized[1].ID > wantLastID {
		wantLastID = synthesized[1].ID
	}
	if d.LastID != wantLastID {
		t.Errorf("d.LastID = %d, want %d (the highest synthesized id)", d.LastID, wantLastID)
	}
	// synthesized[0]/[1] follow document order: Ia's rect, then Id's.
	if synthesized[0].Fill != "#FFFFCC" || synthesized[0].Stroke != "#777777" || synthesized[0].StrokeWidth != 1 {
		t.Errorf("Ia's synthesized background Rectangle fill/stroke/width = %q/%q/%v, want #FFFFCC/#777777/1", synthesized[0].Fill, synthesized[0].Stroke, synthesized[0].StrokeWidth)
	}
	if synthesized[1].Fill != "#99FF99" {
		t.Errorf("Id's synthesized background Rectangle fill = %q, want #99FF99", synthesized[1].Fill)
	}
	wantPoints := []Point{{X: 570, Y: 630}, {X: 630, Y: 660}}
	if len(synthesized[0].Points) != 2 || synthesized[0].Points[0] != wantPoints[0] || synthesized[0].Points[1] != wantPoints[1] {
		t.Errorf("Ia's synthesized background Rectangle points = %+v, want %+v", synthesized[0].Points, wantPoints)
	}
}

// testAutotransformerSVG is real xsde2svg output (xsde2svg/examples/test/svg/
// Test_47_AutoTransformer2-1.svg, its own id="1" instance) — a 2-real-winding
// autotransformer whose own tap arc+stub is drawn before its first <circle>.
const testAutotransformerSVG = `<?xml version="1.0"?>
<svg width="600" height="600" style='stroke-width: 0px; background-color: #12161d;' xmlns="http://www.w3.org/2000/svg">
<g id="1"  data-type="47"  >
<path d="M 350 57 a 40 40 0 0 1 22 35" style="fill:none;stroke:teal;stroke-width:1" data-voltage="teal" />
<path d="M 350 52 v 6" style="fill:none;stroke:teal;stroke-width:2" data-voltage="teal" />
<circle cx="350" cy="92" r="22" style="fill:none;stroke:purple;stroke-width:1" data-voltage="purple" />
<path d="M 328 90 h -11" style="fill:none;stroke:purple;stroke-width:2" data-voltage="purple" />
<circle cx="350" cy="128" r="22" style="fill:none;stroke:olive;stroke-width:2" data-voltage="olive" />
<path d="M 372 130 h 11" style="fill:none;stroke:olive;stroke-width:2" data-voltage="olive" />
</g>
</svg>
`

// TestExtract_PowerTransformerAutotransformerAndVoltages is a regression
// test for two bugs a real user hit together on a real corpus diagram:
// every extracted transformer collapsing to 2 windings regardless of its
// real winding count (parsePowerTransformer found the real lead count but
// never populated Element.Windings, and writePowerTransformer derives its
// own drawn circle count from len(Windings), not len(Ports)), and every
// winding silently losing its own real color/voltage class (extraction
// never resolved a winding's own data-voltage into a VoltageClass id at
// all). Also checks Autotransformer is recovered from the real tap
// arc+stub's own document position (before the first <circle>).
func TestExtract_PowerTransformerAutotransformerAndVoltages(t *testing.T) {
	d, _, err := Extract([]byte(testAutotransformerSVG), "test.svg", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Elements) != 1 {
		t.Fatalf("elements = %+v, want exactly 1 transformer", d.Elements)
	}
	tr := d.Elements[0]
	if !tr.Autotransformer {
		t.Errorf("Autotransformer = false, want true (real tap arc+stub precede the first <circle>)")
	}
	if len(tr.Windings) != 2 {
		t.Fatalf("len(Windings) = %d, want 2 (matching the 2 real <circle>s, not collapsed to a hardcoded default)", len(tr.Windings))
	}
	if len(tr.Ports) != 2 {
		t.Errorf("len(Ports) = %d, want 2", len(tr.Ports))
	}

	byColor := map[string]int{}
	for _, vc := range d.VoltageClasses {
		byColor[vc.Color] = vc.ID
	}
	purpleID, olive1 := byColor["purple"], byColor["olive"]
	if purpleID == 0 || olive1 == 0 {
		t.Fatalf("voltage classes = %+v, want both purple and olive present", d.VoltageClasses)
	}
	if tr.Windings[0].Voltage != purpleID {
		t.Errorf("Windings[0].Voltage = %d, want %d (purple)", tr.Windings[0].Voltage, purpleID)
	}
	if tr.Windings[1].Voltage != olive1 {
		t.Errorf("Windings[1].Voltage = %d, want %d (olive)", tr.Windings[1].Voltage, olive1)
	}
	if tr.Windings[0].Voltage == tr.Windings[1].Voltage {
		t.Errorf("the two windings' own colors differ (purple vs olive) but resolved to the same voltage class: %+v", tr.Windings)
	}
}

// testAutotransformer3WindingSVG is real xsde2svg output
// (sld-svg/examples/sld/Test_47_AutoTransformer3Text.svg, its own id="1"
// instance) — a 3-real-winding autotransformer (element_47.go's own
// WindingNo==4 case). Its third real winding's own lead
// ("M 340 112 v 11") is drawn *before* its own circle
// ("<circle cx="340" cy="90" ...>"), unlike every other winding of every
// other shape — a real quirk of element_47.go's own case-3 branch, found
// by the user comparing this file's own real xsde2svg rendering against
// this package's own extract-then-render round trip and seeing a visibly
// different third winding.
const testAutotransformer3WindingSVG = `<?xml version="1.0"?>
<svg width="1930" height="1420" style='stroke-width: 0px; background-color: #f5ebeb;' xmlns="http://www.w3.org/2000/svg">
<g id="1"  data-type="47"  >
<path d="M 360 25 a 40 40 0 0 1 22 35" style="fill:none;stroke:seagreen;stroke-width:1" data-voltage="seagreen" />
<path d="M 360 20 v 6" style="fill:none;stroke:seagreen;stroke-width:2" data-voltage="seagreen" />
<circle cx="360" cy="60" r="22" style="fill:none;stroke:#ffc055;stroke-width:1" data-voltage="#ffc055" />
<path d="M 338 60 h -11" style="fill:none;stroke:#ffc055;stroke-width:2" data-voltage="#ffc055" />
<circle cx="380" cy="90" r="22" style="fill:none;stroke:purple;stroke-width:2" data-voltage="purple" />
<path d="M 402 90 h 11" style="fill:none;stroke:purple;stroke-width:2" data-voltage="purple" />
<path d="M 340 112 v 11" style="fill:none;stroke:red;stroke-width:2" data-voltage="red" />
<circle cx="340" cy="90" r="22" style="fill:none;stroke:red;stroke-width:2" data-voltage="red" />
</g>
</svg>
`

// TestExtract_PowerTransformerLeadBeforeCircle is a regression test for a
// real xsde2svg autotransformer quirk (see testAutotransformer3WindingSVG's
// own doc comment): a strict "circle, then its own trailing lead" document-
// order grouping under-counts this transformer's own third winding
// entirely and misreads its actual lead as a (non-matching, silently
// discarded) connection-scheme glyph for the second winding instead —
// parsePowerTransformer's own lead-to-circle assignment is distance-based
// specifically to get this right regardless of document order.
func TestExtract_PowerTransformerLeadBeforeCircle(t *testing.T) {
	d, _, err := Extract([]byte(testAutotransformer3WindingSVG), "test.svg", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Elements) != 1 {
		t.Fatalf("elements = %+v, want exactly 1 transformer", d.Elements)
	}
	tr := d.Elements[0]
	if !tr.Autotransformer {
		t.Errorf("Autotransformer = false, want true")
	}
	if len(tr.Windings) != 3 || len(tr.Ports) != 3 {
		t.Fatalf("windings=%d ports=%d, want 3 real windings (#ffc055, purple, red), not 2", len(tr.Windings), len(tr.Ports))
	}

	byColor := map[string]int{}
	for _, vc := range d.VoltageClasses {
		byColor[vc.Color] = vc.ID
	}
	wantColors := []string{"#ffc055", "purple", "red"}
	for i, want := range wantColors {
		id := byColor[want]
		if id == 0 {
			t.Fatalf("voltage classes = %+v, want %q present", d.VoltageClasses, want)
		}
		if tr.Windings[i].Voltage != id {
			t.Errorf("Windings[%d].Voltage = %d, want %d (%s)", i, tr.Windings[i].Voltage, id, want)
		}
	}

	// M 340 112 v 11 ends at (340,123) — no rotate() on this <g>, so that's
	// also its real global position, grid-snapped to (340,120). Confirm it
	// landed on the *third* winding (red), not swallowed as a false glyph
	// on the second (purple), or lost to the tap arc's own similarly-shaped
	// endpoint.
	if len(d.Nodes) < 3 {
		t.Fatalf("nodes = %+v, want at least 3 (one per real lead)", d.Nodes)
	}
	var redPort Point
	for _, node := range d.Nodes {
		if node.ID == tr.Ports[2].Node {
			redPort = Point{X: node.X, Y: node.Y}
		}
	}
	if redPort != (Point{X: 340, Y: 120}) {
		t.Errorf("third winding's own port = %v, want {340 120} (its real lead tip, grid-snapped — not a discarded glyph check)", redPort)
	}
}

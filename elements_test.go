package slddoc

import (
	"bytes"
	"math"
	"testing"
)

func parseFirst(t *testing.T, fragment string) *rawNode {
	t.Helper()
	root, err := parseRawTree([]byte("<svg>" + fragment + "</svg>"))
	if err != nil {
		t.Fatalf("parseRawTree: %v", err)
	}
	if len(root.Children) == 0 {
		t.Fatalf("fragment produced no top-level node: %s", fragment)
	}
	return root.Children[0]
}

func TestParseBreakerLike_Fixed(t *testing.T) {
	n := parseFirst(t, `
<g id="2413" data-voltage="#962896" data-type="41" data-name="В-10 Л-22" >
<g  >
<path d="M 893 653 h 14 v 14 h -14 z" data-fill="0:red,1:lawngreen,2:yellow" style="fill:lawngreen;stroke:#962896;stroke-width:1" />
<path d="M 900 655 v 10" data-state="1" style="stroke:#962896;stroke-width:1" />
<path d="M 900 650 v 3 M 900 670 v -3" style="stroke:#962896;stroke-width:1" />
</g>
</g>`)

	el, ports, voltage, err := parseTwoPortDevice(n, ClassBreaker, "41")
	if err != nil {
		t.Fatal(err)
	}
	if el.Class != ClassBreaker || voltage != "#962896" || el.Name != "В-10 Л-22" {
		t.Errorf("unexpected element: %+v (voltage %q)", el, voltage)
	}
	if el.X != 900 || el.Y != 660 {
		t.Errorf("anchor = (%v,%v), want (900,660)", el.X, el.Y)
	}
	if el.Orient != 0 {
		t.Errorf("orient = %d, want 0 (vertical)", el.Orient)
	}
	if el.State == nil || *el.State != 1 {
		t.Errorf("state = %v, want 1", el.State)
	}
	wantPorts := []Point{{900, 650}, {900, 670}}
	if ports[0] != wantPorts[0] || ports[1] != wantPorts[1] {
		t.Errorf("ports = %v, want %v", ports, wantPorts)
	}
}

func TestParseBreakerLike_Withdrawable(t *testing.T) {
	// The real element carries a third inner path — the withdrawable
	// "trolley" isolating-contact detail (a short stub from the box plus a
	// chevron, on both sides) — narrower in span than the outer arrows and
	// so must not be mistaken for the true ports.
	n := parseFirst(t, `
<g id="148694374" data-voltage="#326400" data-name="В-6 Л-22 (3)" data-type="43"  >
<g  data-trolley="1" >
<path d="M 892 292 h 17 v 17 h -17 z" data-fill="0:red,1:lawngreen,2:yellow" style="fill:lawngreen;stroke:#326400;stroke-width:1" />
<path d="M 900 295 v 11" data-state="1" style="stroke:#326400;stroke-width:1" />
<path d="M 900 292 v -13 m 0 0 l -4 4 m 4 -4 l 4 4 M 900 308 v 13 m 0 0 l -4 -4 m 4 4 l 4 -4" style="stroke:#326400;stroke-width:1" />
</g>
<path d="M 900 270 l -4 4 m 8 0 l -4 -4 M 900 330 l -4 -4 m 8 0 l -4 4" style="stroke:#326400;stroke-width:1" />
</g>`)

	el, ports, _, err := parseTwoPortDevice(n, ClassBreaker, "43")
	if err != nil {
		t.Fatal(err)
	}
	if el.X != 900 || el.Y != 300 {
		t.Errorf("anchor = (%v,%v), want (900,300)", el.X, el.Y)
	}
	wantPorts := []Point{{900, 270}, {900, 330}}
	if ports[0] != wantPorts[0] || ports[1] != wantPorts[1] {
		t.Errorf("ports = %v, want %v (the outer arrows, not the narrower inner trolley detail or its lowercase 'm' chevron offsets)", ports, wantPorts)
	}
}

func TestParseDisconnector(t *testing.T) {
	n := parseFirst(t, `
<g id="2409" data-type="162" data-name="ТР-10 Л-22">
<path d="M 900 586 v -13" data-state="1" style="fill:none;stroke:#962896;stroke-width:1" data-voltage="#962896" />
<path d="M 896 571 h 8 m 0 18 h -8" style="fill:none;stroke:#962896;stroke-width:1" data-voltage="#962896" />
<path d="M 900 571 v -1 M 900 589 v 1" style="fill:none;stroke:#962896;stroke-width:1" data-voltage="#962896" />
</g>`)

	el, ports, voltage, err := parseTwoPortDevice(n, ClassDisconnector, "162")
	if err != nil {
		t.Fatal(err)
	}
	if el.X != 900 || el.Y != 580 {
		t.Errorf("anchor = (%v,%v), want (900,580)", el.X, el.Y)
	}
	wantPorts := []Point{{900, 570}, {900, 590}}
	if ports[0] != wantPorts[0] || ports[1] != wantPorts[1] {
		t.Errorf("ports = %v, want %v (must use the subpath's final point, not its M start)", ports, wantPorts)
	}
	if voltage != "#962896" {
		t.Errorf("voltage = %q, want #962896 (read from a descendant path, since the <g> itself carries none)", voltage)
	}
}

func TestParseGroundSwitch(t *testing.T) {
	n := parseFirst(t, `
<g id="2511" data-voltage="#962896" data-type="54" data-name="ЗР-10 В-10 Л-22" transform="rotate(90,910,620)" >
<path d="M 910 632 v -9 m -4 0 h 8 m -8 -20 h 8 m -4 0 v -8 m -8 0 h 16 m -2 -3 h -12 m 2 -3 h 8" style="fill:none;stroke:#962896;stroke-width:1" />
<path d="M 916 612 h -12" data-state="0" style="fill:none;stroke:#962896;stroke-width:1" />
</g>`)

	el, ports, _, err := parseGroundSwitch(n)
	if err != nil {
		t.Fatal(err)
	}
	if el.X != 910 || el.Y != 620 || el.Orient != 90 {
		t.Errorf("anchor/orient = (%v,%v,%d), want (910,620,90)", el.X, el.Y, el.Orient)
	}
	if len(ports) != 1 || ports[0] != (Point{910, 620}) {
		t.Errorf("ports = %v, want a single port at the rotation anchor", ports)
	}
	if el.State == nil || *el.State != 0 {
		t.Errorf("state = %v, want 0", el.State)
	}
}

func TestParsePowerTransformer(t *testing.T) {
	n := parseFirst(t, `
<g id="2463" data-name="Т-1 1,6МВА" data-type="47" transform="rotate(-270,900,500)" >
<circle cx="918" cy="500" r="22" style="fill:none;stroke:#962896;stroke-width:2" data-voltage="#962896" />
<path d="M 940 500 h 13" style="fill:none;stroke:#962896;stroke-width:2" data-voltage="#962896" />
<path d="M 918 500 l -7 -7 M 918 500 l 7 -7 M 918 500 l 0 7" style="fill:none;stroke:#326400;stroke-width:1" />
<circle cx="882" cy="500" r="22" style="fill:none;stroke:#326400;stroke-width:2" data-voltage="#326400" />
<path d="M 860 500 h -13" style="fill:none;stroke:#326400;stroke-width:2" data-voltage="#326400" />
<path d="M 882 500 l -7 -7 M 882 500 l 7 -7 M 882 500 l 0 7" style="fill:none;stroke:#326400;stroke-width:1" />
</g>`)

	el, ports, colors, err := parsePowerTransformer(n)
	if err != nil {
		t.Fatal(err)
	}
	if el.X != 900 || el.Y != 500 || el.Orient != -270 {
		t.Errorf("anchor/orient = (%v,%v,%d), want (900,500,-270)", el.X, el.Y, el.Orient)
	}
	// Local leads are at (953,500) and (847,500); rotate(-270) about
	// (900,500) turns +90° clockwise, so both ports land on x=900 at
	// y=553/447 before grid-snapping to the nearest 10 (550/450).
	if len(ports) != 2 || ports[0] != (Point{900, 550}) || ports[1] != (Point{900, 450}) {
		t.Errorf("ports = %v, want [{900 550} {900 450}]", ports)
	}
	if len(colors) != 2 || colors[0] != "#962896" || colors[1] != "#326400" {
		t.Errorf("colors = %v, want [#962896 #326400]", colors)
	}
	if len(el.Windings) != 2 || el.Windings[0].Scheme != SchemeWye || el.Windings[1].Scheme != SchemeWye {
		t.Errorf("windings = %+v, want two plain wye windings", el.Windings)
	}
	if el.Autotransformer {
		t.Errorf("el.Autotransformer = true, want false")
	}
}

// TestParsePowerTransformer_SkipsTrailingDecoration checks a 3-winding
// transformer whose markup carries a trailing decoration path after the
// last winding's own circle+lead+glyph — the same document position a
// regulation arrow or (writePowerTransformer's own invention) an
// autotransformer tap stub would occupy — with the same
// single-two-point-subpath shape a real lead has. parsePowerTransformer
// must still find exactly 3 leads, not 4, because it only looks for a
// winding's own lead in the <path> children between that winding's own
// <circle> and the next one (see its own doc comment).
func TestParsePowerTransformer_SkipsTrailingDecoration(t *testing.T) {
	n := parseFirst(t, `
<g id="5" data-name="AT-1" data-type="47" transform="rotate(0,200,200)" >
<circle cx="218" cy="200" r="22" style="fill:none;stroke:teal;stroke-width:2" data-voltage="teal" />
<path d="M 240 200 h 13" style="fill:none;stroke:teal;stroke-width:2" data-voltage="teal" />
<path d="M 218 200 l -7 -7 M 218 200 l 7 -7 M 218 200 l 0 7" style="fill:none;stroke:teal;stroke-width:1" />
<circle cx="200" cy="175" r="22" style="fill:none;stroke:purple;stroke-width:2" data-voltage="purple" />
<path d="M 200 153 v -13" style="fill:none;stroke:purple;stroke-width:2" data-voltage="purple" />
<circle cx="182" cy="200" r="22" style="fill:none;stroke:olive;stroke-width:2" data-voltage="olive" />
<path d="M 160 200 h -13" style="fill:none;stroke:olive;stroke-width:2" data-voltage="olive" />
<path d="M 175 175 L 225 125" style="fill:none;stroke:teal;stroke-width:1" />
</g>`)

	el, ports, colors, err := parsePowerTransformer(n)
	if err != nil {
		t.Fatal(err)
	}
	if len(ports) != 3 {
		t.Errorf("ports = %v (len %d), want exactly 3 leads, not the trailing decoration counted as a 4th", ports, len(ports))
	}
	if len(el.Ports) != 3 {
		t.Errorf("el.Ports = %v, want 3 entries", el.Ports)
	}
	if len(colors) != 3 || colors[0] != "teal" || colors[1] != "purple" || colors[2] != "olive" {
		t.Errorf("colors = %v, want [teal purple olive]", colors)
	}
	// Only winding 0 has a real connection glyph in this fixture; the
	// trailing decoration after winding 2's own lead must not be
	// misidentified as its own delta/wye scheme.
	if len(el.Windings) != 3 || el.Windings[0].Scheme != SchemeWye || el.Windings[1].Scheme != "" || el.Windings[2].Scheme != "" {
		t.Errorf("windings = %+v, want [wye, none, none]", el.Windings)
	}
}

func TestParseTwoPortDevice_LoadBreakSwitch(t *testing.T) {
	n := parseFirst(t, `
<g id="4616" data-voltage="#962896" data-name="ВН-10 Л-25 РП 10 кВ Юпитер" data-type="42"  >
<path d="M 600 790 v 2  m 0 16 v 2 m -8 -2 h 16 m -16 -16 h 16 " data-fill="0:red,1:lawngreen,2:yellow" style="fill:none;stroke:#962896;stroke-width:1" />
<path d="M 600 792  l 8 8 l -8 8 l -8 -8 l 8 -8 " style="fill:red;stroke:#962896;stroke-width:1" />
<path d="M 600 795 v 10" data-state="1" style="fill:none;stroke:#962896;stroke-width:1" />
</g>`)

	el, ports, _, err := parseTwoPortDevice(n, ClassLoadBreakSwitch, "42")
	if err != nil {
		t.Fatal(err)
	}
	if el.X != 600 || el.Y != 800 {
		t.Errorf("anchor = (%v,%v), want (600,800)", el.X, el.Y)
	}
	wantPorts := []Point{{600, 790}, {600, 810}}
	if ports[0] != wantPorts[0] || ports[1] != wantPorts[1] {
		t.Errorf("ports = %v, want %v (the top port is a subpath's start, the bottom one its end — extremes must handle both)", ports, wantPorts)
	}
}

func TestParseTwoPortDevice_WithdrawableDisconnector(t *testing.T) {
	n := parseFirst(t, `
<g id="67" data-voltage="#962896" data-name="СР-10" data-type="49"  >
<path d="M 2460 158 l -4 4 m 4 -4 l 4 4 m -4 -4 v 12 m -4 0 h 8 m -4 2 v 16 m -4 2 h 8 m -4 0 v 12 l -4 -4 m 4 4 l 4 -4" style="fill:none;stroke:#962896;stroke-width:1" />
<path d="M 2460 210 l -4 -4 m 4 4 l 4 -4 M 2460 150 l -4 4 m 4 -4 l 4 4" style="fill:none;stroke:#962896;stroke-width:1" />
</g>`)

	el, ports, _, err := parseTwoPortDevice(n, ClassDisconnector, "49")
	if err != nil {
		t.Fatal(err)
	}
	if el.X != 2460 || el.Y != 180 {
		t.Errorf("anchor = (%v,%v), want (2460,180)", el.X, el.Y)
	}
	wantPorts := []Point{{2460, 150}, {2460, 210}}
	if ports[0] != wantPorts[0] || ports[1] != wantPorts[1] {
		t.Errorf("ports = %v, want %v", ports, wantPorts)
	}
}

func TestParseTwoPortDevice_ChokeCoil_RotatedWithArcs(t *testing.T) {
	n := parseFirst(t, `<path d="M 1052 297 v 5 h 1 m 0 0 a 5 5 0 1 1 0 10 a 5 5 0 1 1 0 10 a 5 5 0 1 1 0 10 h -1 v 5" style="fill:none;stroke:#00A0F0;stroke-width:1" id="3368" data-voltage="#00A0F0" data-type="33" transform="rotate(180,1052,317)" />`)

	el, ports, _, err := parseTwoPortDevice(n, ClassChokeCoil, "33")
	if err != nil {
		t.Fatal(err)
	}
	if el.X != 1052 || el.Y != 317 || el.Orient != 180 {
		t.Errorf("anchor/orient = (%v,%v,%d), want (1052,317,180)", el.X, el.Y, el.Orient)
	}
	// Local extremes (1052,297) and (1052,337) rotated 180° about (1052,317)
	// land back on the same X, reflected in Y.
	wantPorts := []Point{{1052, 337}, {1052, 297}}
	if ports[0] != wantPorts[0] || ports[1] != wantPorts[1] {
		t.Errorf("ports = %v, want %v (arc endpoints must be tracked, then rotated through the transform)", ports, wantPorts)
	}
}

func TestParseTwoPortDevice_CurrentTransformer(t *testing.T) {
	n := parseFirst(t, `<path d="M 814 489 h 8 a 6 4 0 0 1 0 8 a 6 4 0 0 1 0 8 h -8 M 822 486 v 22" style="fill:none;stroke:#00A0F0;stroke-width:1" id="3316" data-voltage="#00A0F0" data-name="ТТ-110 л.Вд-1" data-type="34"  />`)

	el, ports, _, err := parseTwoPortDevice(n, ClassCurrentTransformer, "34")
	if err != nil {
		t.Fatal(err)
	}
	if el.X != 822 || el.Y != 497 {
		t.Errorf("anchor = (%v,%v), want (822,497)", el.X, el.Y)
	}
	wantPorts := []Point{{822, 486}, {822, 508}}
	if ports[0] != wantPorts[0] || ports[1] != wantPorts[1] {
		t.Errorf("ports = %v, want %v (the straight-through line's ends, wider than the coil loop)", ports, wantPorts)
	}
}

func TestParseTwoPortDevice_SurgeArrester(t *testing.T) {
	n := parseFirst(t, `
<g id="4529" data-voltage="#962896" data-type="35" transform="rotate(-90,1785,1132)" >
<path d="M 1785 1122 v 8  M 1785 1142 v -8" style="fill:none;stroke:#962896;stroke-width:1" />
<path d="M 1785 1130  l -3 -6 l 6 0 z M 1785 1134  l -3 6 l 6 0 z" style="fill:#962896;stroke:#962896;stroke-width:1" />
</g>`)

	el, ports, _, err := parseTwoPortDevice(n, ClassSurgeArrester, "35")
	if err != nil {
		t.Fatal(err)
	}
	if el.X != 1785 || el.Y != 1132 || el.Orient != -90 {
		t.Errorf("anchor/orient = (%v,%v,%d), want (1785,1132,-90)", el.X, el.Y, el.Orient)
	}
	if len(ports) != 2 {
		t.Fatalf("ports = %v, want 2", ports)
	}
}

func TestParseTwoPortDevice_Fuse(t *testing.T) {
	n := parseFirst(t, `<path d="M 1060 890 v 20 m 0 -20 h 5 v 20 h -10 v -20 h 5" style="fill:none;stroke:#962896;stroke-width:1"  id="2455" data-voltage="#962896" data-type="203" />`)

	el, ports, _, err := parseTwoPortDevice(n, ClassFuse, "203")
	if err != nil {
		t.Fatal(err)
	}
	if el.X != 1060 || el.Y != 900 {
		t.Errorf("anchor = (%v,%v), want (1060,900)", el.X, el.Y)
	}
	wantPorts := []Point{{1060, 890}, {1060, 910}}
	if ports[0] != wantPorts[0] || ports[1] != wantPorts[1] {
		t.Errorf("ports = %v, want %v", ports, wantPorts)
	}
}

func TestParseTwoPortDevice_Capacitor(t *testing.T) {
	n := parseFirst(t, `<path d="M 735 302 v 5 m -12 0 h 24 m -24 10 h 24  m -12 0 v 5" style="fill:none;stroke:#965000;stroke-width:1" transform="rotate(-90,735,312)" id="2535" data-voltage="#965000" data-type="388" />`)

	el, ports, _, err := parseTwoPortDevice(n, ClassCapacitor, "388")
	if err != nil {
		t.Fatal(err)
	}
	if el.X != 735 || el.Y != 312 || el.Orient != -90 {
		t.Errorf("anchor/orient = (%v,%v,%d), want (735,312,-90)", el.X, el.Y, el.Orient)
	}
	if len(ports) != 2 {
		t.Fatalf("ports = %v, want 2", ports)
	}
}

// TestParseVoltageTransformer uses the exact real xsde2svg markup
// (sld-viewer's own PS_110kV_Example.svg, id="4473") that surfaced this
// gap: a real shape-55 instance carries no data-voltage attribute
// anywhere, unlike every other one-port device — its own voltage color
// only lives in the primary winding's own style="stroke:...".
func TestParseVoltageTransformer(t *testing.T) {
	n := parseFirst(t, `<g id="4473" data-type="55" data-name="ТН-110 л.Лч-2 ф-А" transform="rotate(-90,2380,370)" >
<path d="M 2380 354 v -5" style="fill:none;stroke:#00A0F0;stroke-width:1" />
<circle cx="2380" cy="366" r="12" style="fill:none;stroke:#00A0F0;stroke-width:1" />
<circle cx="2380" cy="383" r="12" style="fill:none;stroke:#D2D2D2;stroke-width:1" />
</g>`)

	el, ports, voltage, err := parseVoltageTransformer(n)
	if err != nil {
		t.Fatal(err)
	}
	if voltage != "#00A0F0" {
		t.Errorf("voltage = %q, want the primary winding's own stroke color #00A0F0", voltage)
	}
	if el.Class != ClassVoltageTransformer || el.Shape != "55" || el.Name != "ТН-110 л.Лч-2 ф-А" {
		t.Errorf("element = %+v", el)
	}
	if len(ports) != 1 {
		t.Fatalf("ports = %v, want 1", ports)
	}
}

func TestParseGround_WithTransform(t *testing.T) {
	n := parseFirst(t, `<path d="M 1805 1132 v -10 m -8 10 h 16 m -2 3 h -12 m 2 3 h 8" style="fill:none;stroke:#962896;stroke-width:1" id="3726" data-voltage="#962896" data-type="31" transform="rotate(-90,1805,1132)" />`)

	el, ports, _, err := parseGround(n)
	if err != nil {
		t.Fatal(err)
	}
	if el.X != 1805 || el.Y != 1132 || el.Orient != -90 {
		t.Errorf("anchor/orient = (%v,%v,%d), want (1805,1132,-90)", el.X, el.Y, el.Orient)
	}
	if len(ports) != 1 || ports[0] != (Point{1805, 1132}) {
		t.Errorf("ports = %v, want a single port at the rotation anchor", ports)
	}
}

func TestParseGround_WithoutTransform(t *testing.T) {
	n := parseFirst(t, `<path d="M 1815 962 v -10 m -8 10 h 16 m -2 3 h -12 m 2 3 h 8" style="fill:none;stroke:#00A0F0;stroke-width:1" id="4009" data-voltage="#00A0F0" data-type="31"  />`)

	el, ports, _, err := parseGround(n)
	if err != nil {
		t.Fatal(err)
	}
	if el.X != 1815 || el.Y != 962 || el.Orient != 0 {
		t.Errorf("anchor/orient = (%v,%v,%d), want (1815,962,0) (no transform: the port is the path's own first point)", el.X, el.Y, el.Orient)
	}
	if len(ports) != 1 || ports[0] != (Point{1815, 962}) {
		t.Errorf("ports = %v", ports)
	}
}

func TestParseBusBar(t *testing.T) {
	n := parseFirst(t, `<polyline points="810,240 1320,240" style="fill:none;stroke:#326400;stroke-width:2" data-voltage="#326400" data-name="СШ 6 кВ" data-type="24" id="148694372" />`)

	el, _, err := parseBusBar(n)
	if err != nil {
		t.Fatal(err)
	}
	if el.Class != ClassBusBarSection || el.Name != "СШ 6 кВ" || len(el.Points) != 2 {
		t.Errorf("unexpected element: %+v", el)
	}
}

func TestParseJunctionPoint(t *testing.T) {
	n := parseFirst(t, `<circle cx="900" cy="240" r="3" style="fill:#12161d;stroke:#326400;stroke-width:1" data-type="7" id="148694381" data-voltage="#12161d" />`)

	el, voltage, err := parseJunctionPoint(n)
	if err != nil {
		t.Fatal(err)
	}
	if el.X != 900 || el.Y != 240 {
		t.Errorf("anchor = (%v,%v), want (900,240)", el.X, el.Y)
	}
	if voltage != "#326400" {
		t.Errorf("voltage = %q, want #326400 (stroke, not the fill recorded in data-voltage)", voltage)
	}
}

func TestParseLamp(t *testing.T) {
	n := parseFirst(t, `<circle cx="382" cy="203" r="11" style="fill:none;stroke:slategray;stroke-width:2" data-type="106" data-state="0" data-fill="0:none,1:red" data-name="АВ Валдай" id="148704876" />`)

	el, err := parseLamp(n)
	if err != nil {
		t.Fatal(err)
	}
	if el.Class != ClassLamp || el.X != 382 || el.Y != 203 {
		t.Errorf("unexpected element: %+v", el)
	}
	if el.FillOff != "none" || el.FillOn != "red" {
		t.Errorf("fillOff/fillOn = %q/%q, want none/red", el.FillOff, el.FillOn)
	}
	if el.Radius != 11 {
		t.Errorf("radius = %v, want 11 (from the circle's own r attribute)", el.Radius)
	}
	if el.State == nil || *el.State != 0 {
		t.Errorf("state = %v, want 0", el.State)
	}
	if len(el.Ports) != 0 {
		t.Errorf("ports = %v, want none (a lamp is a status indicator, not a wired device)", el.Ports)
	}
}

func TestParseFaultPassageIndicator(t *testing.T) {
	// Real instances vary in their decorative children (an "FPI" text label
	// here, arrow-icon graphics on others) but always carry exactly one
	// circle with the same fixed fill/stroke.
	n := parseFirst(t, `
<g id="148795595" data-type="320003" >
<circle cx="310" cy="550" r="15" style="fill:#12161d;stroke:lime;stroke-width:0.80" />
<text x="310" y="550" style="fill:lime;text-anchor:middle;dominant-baseline:middle;font-size:13px;font-family:Arial" >FPI</text>
</g>`)

	el, err := parseFaultPassageIndicator(n)
	if err != nil {
		t.Fatal(err)
	}
	if el.Class != ClassFaultPassageIndicator || el.Shape != "320003" {
		t.Errorf("unexpected element: %+v", el)
	}
	if el.X != 310 || el.Y != 550 {
		t.Errorf("anchor = (%v,%v), want (310,550)", el.X, el.Y)
	}
	if el.Radius != 15 {
		t.Errorf("radius = %v, want 15 (from the circle's own r attribute)", el.Radius)
	}
	if len(el.Ports) != 0 {
		t.Errorf("ports = %v, want none (a fault passage indicator is a status indicator, not a wired device)", el.Ports)
	}
}

func TestParsePowerflowIndicator(t *testing.T) {
	// Real corpus markup (PS_Novaya_L2_PS_Nelushka_L3.svg): the "←" variant,
	// with a non-zero rotation.
	n := parseFirst(t, `<text x="1380" y="452" style="font-size:18;fill:skyblue;font-weight: bold" transform="rotate(90,1380,450)" id="148791591" data-type="320001" data-angle="90" >←</text>`)

	el, err := parsePowerflowIndicator(n)
	if err != nil {
		t.Fatal(err)
	}
	if el.Class != ClassPowerflowIndicator || el.Shape != "320001" {
		t.Errorf("unexpected element: %+v", el)
	}
	if el.X != 1380 || el.Y != 449 {
		t.Errorf("anchor = (%v,%v), want (1380,449) (y recovered from the text's own y=452 minus its own +3 shift)", el.X, el.Y)
	}
	if el.Orient != 90 {
		t.Errorf("orient = %v, want 90 (from data-angle)", el.Orient)
	}
	if el.TextColor != "skyblue" {
		t.Errorf("textColor = %q, want skyblue", el.TextColor)
	}
	if el.State == nil || *el.State != 1 {
		t.Errorf("state = %v, want 1 (the \"←\" glyph)", el.State)
	}
	if len(el.Ports) != 0 {
		t.Errorf("ports = %v, want none (a powerflow indicator is a decorative annotation, not a wired device)", el.Ports)
	}

	// The "→" variant reads back as State nil/unset, not a plain 0 — real
	// instances never carry a data-state attribute of their own for this
	// shape, and there's no reason to disagree once extracted.
	n2 := parseFirst(t, `<text x="990" y="153" style="font-size:26;fill:skyblue;font-weight: bold" transform="rotate(-180,990,150)" id="148796122" data-type="320001" data-angle="-180" >→</text>`)
	el2, err := parsePowerflowIndicator(n2)
	if err != nil {
		t.Fatal(err)
	}
	if el2.State != nil {
		t.Errorf("state = %v, want nil for the \"→\" glyph", el2.State)
	}
	if el2.Orient != -180 {
		t.Errorf("orient = %v, want -180 (from data-angle)", el2.Orient)
	}
}

func TestParseTable(t *testing.T) {
	n := parseFirst(t, `<g id="1" data-type="312" data-name="Note" data-voltage="#952896" >
<rect x="10" y="10" width="50" height="30" style="fill:gray;stroke:#952896;stroke-dasharray: 6,5;stroke-width:2" />
<text x="35" y="25" style="fill:yellow;text-anchor:middle;dominant-baseline:middle;font-size:14px;font-family:Arial" transform="rotate(90,35,25)">Hello</text>
</g>`)

	el, err := parseTable(n)
	if err != nil {
		t.Fatal(err)
	}
	if el.Class != ClassTable || el.Shape != "312" || el.Name != "Note" {
		t.Errorf("unexpected element: %+v", el)
	}
	if len(el.Points) != 2 || el.Points[0] != (Point{X: 10, Y: 10}) || el.Points[1] != (Point{X: 60, Y: 40}) {
		t.Errorf("points = %+v, want [(10,10),(60,40)]", el.Points)
	}
	if el.Fill != "gray" || el.Stroke != "#952896" || el.StrokeWidth != 2 {
		t.Errorf("fill/stroke/strokeWidth = %q/%q/%v, want gray/#952896/2", el.Fill, el.Stroke, el.StrokeWidth)
	}
	if el.LineStyle != LineStyleDashed {
		t.Errorf("lineStyle = %q, want dashed (from stroke-dasharray: 6,5)", el.LineStyle)
	}
	if el.PropertyText != "Hello" || el.TextColor != "yellow" {
		t.Errorf("propertyText/textColor = %q/%q, want Hello/yellow", el.PropertyText, el.TextColor)
	}
	if el.Orient != 90 {
		t.Errorf("orient = %v, want 90 (from the label's own rotate() transform)", el.Orient)
	}
}

func TestParseTable_NoLabel(t *testing.T) {
	n := parseFirst(t, `<g id="1" data-type="312" data-voltage="white" >
<rect x="0" y="0" width="20" height="20" style="fill:none;stroke:white;stroke-width:1" />
</g>`)

	el, err := parseTable(n)
	if err != nil {
		t.Fatal(err)
	}
	if el.PropertyText != "" || el.Orient != 0 {
		t.Errorf("a table with no label should have no PropertyText/Orient: %+v", el)
	}
}

func TestParseTable2_RoundTripsThroughRender(t *testing.T) {
	// Build a Table2, render it (the same markup a real, now-patched
	// xsde2svg export would carry — see ClassTable2's own doc comment),
	// then parse that rendered output straight back — the strongest
	// available check that parseTable2's own geometry reconstruction
	// (tableBoundaries/boundaryIndex/mostCommon) actually inverts
	// writeTable2 correctly, not just that it doesn't error.
	original := Element{
		ID: 1, Class: ClassTable2, Shape: "313",
		X: 100, Y: 200,
		RowHeights:   []float64{20, 30},
		ColumnWidths: []float64{60, 50, 40},
		Stroke:       "#952896",
		StrokeWidth:  1,
		LineStyle:    LineStyleDashed,
		Fill:         "gray",
		Cells: []TableCell{
			{Row: 0, Col: 0, Text: "A"},
			{Row: 0, Col: 1, Text: "B", Fill: "red", TextColor: "white"},
			{Row: 0, Col: 2},
			{Row: 1, Col: 0, Text: "D"},
			{Row: 1, Col: 1},
			{Row: 1, Col: 2, Text: "F"},
		},
	}

	var buf bytes.Buffer
	writeTable2(&buf, original, Static)

	n := parseFirst(t, buf.String())
	got, err := parseTable2(n)
	if err != nil {
		t.Fatalf("parseTable2: %v\nrendered markup was:\n%s", err, buf.String())
	}

	if got.X != original.X || got.Y != original.Y {
		t.Errorf("anchor = (%v,%v), want (%v,%v)", got.X, got.Y, original.X, original.Y)
	}
	if len(got.RowHeights) != len(original.RowHeights) || len(got.ColumnWidths) != len(original.ColumnWidths) {
		t.Fatalf("grid shape = %d rows x %d cols, want %d x %d", len(got.RowHeights), len(got.ColumnWidths), len(original.RowHeights), len(original.ColumnWidths))
	}
	for i := range original.RowHeights {
		if math.Abs(got.RowHeights[i]-original.RowHeights[i]) > 0.01 {
			t.Errorf("RowHeights[%d] = %v, want %v", i, got.RowHeights[i], original.RowHeights[i])
		}
	}
	for j := range original.ColumnWidths {
		if math.Abs(got.ColumnWidths[j]-original.ColumnWidths[j]) > 0.01 {
			t.Errorf("ColumnWidths[%d] = %v, want %v", j, got.ColumnWidths[j], original.ColumnWidths[j])
		}
	}
	if got.Stroke != original.Stroke || got.StrokeWidth != original.StrokeWidth || got.LineStyle != original.LineStyle {
		t.Errorf("stroke/strokeWidth/lineStyle = %q/%v/%q, want %q/%v/%q", got.Stroke, got.StrokeWidth, got.LineStyle, original.Stroke, original.StrokeWidth, original.LineStyle)
	}
	if got.Fill != original.Fill {
		t.Errorf("recovered default fill = %q, want %q (the majority real cell fill)", got.Fill, original.Fill)
	}
	if len(got.Cells) != len(original.Cells) {
		t.Fatalf("cells = %+v, want %d entries", got.Cells, len(original.Cells))
	}
	byPos := map[[2]int]TableCell{}
	for _, c := range got.Cells {
		byPos[[2]int{c.Row, c.Col}] = c
	}
	for _, want := range original.Cells {
		got, ok := byPos[[2]int{want.Row, want.Col}]
		if !ok {
			t.Errorf("missing cell (%d,%d)", want.Row, want.Col)
			continue
		}
		if got.Text != want.Text || got.Fill != want.Fill || got.TextColor != want.TextColor {
			t.Errorf("cell (%d,%d) = %+v, want %+v", want.Row, want.Col, got, want)
		}
	}
}

func TestParseTable2_NoCellsIsAnError(t *testing.T) {
	n := parseFirst(t, `<g id="1" data-type="313" ></g>`)
	if _, err := parseTable2(n); err == nil {
		t.Error("expected an error for a table2 with no cell <path> children")
	}
}

func TestParseConnector(t *testing.T) {
	n := parseFirst(t, `<polyline points="900,360 900,420" style="fill:none;stroke:#326400;stroke-dasharray: 14,9;stroke-width:1 " data-type="23" id="148694387" data-voltage="#326400" />`)

	c, _, err := parseConnector(n)
	if err != nil {
		t.Fatal(err)
	}
	if c.Kind != KindCableLine || !c.Dashed || len(c.Points) != 2 {
		t.Errorf("unexpected connector: %+v", c)
	}
}

// TestParseConnector_Kind covers every data-type code parseConnector
// understands against its own real xsde2svg code — "21" (Ошиновка/
// Buswork) must resolve to KindBusWork, not KindBusbarWire, which has no
// code of its own at all (see connectorKindByType's own doc comment) —
// this is the exact real corpus data that surfaced the bug this test
// guards against (a real data-type="21" connector, id=148795659, from
// "Энергомониторинг_Л-5_Почеп - Л-3_Зеленая.svg").
func TestParseConnector_Kind(t *testing.T) {
	cases := []struct {
		dataType string
		want     ConnectorKind
	}{
		{"21", KindBusWork},
		{"22", KindOverheadLine},
		{"23", KindCableLine},
		{"28", KindLinkToObject},
	}
	for _, c := range cases {
		t.Run(c.dataType, func(t *testing.T) {
			n := parseFirst(t, `<polyline points="0,0 1,1" style="fill:none;stroke:#962896;stroke-width:2 " data-type="`+c.dataType+`" id="1" data-voltage="#962896" />`)
			conn, _, err := parseConnector(n)
			if err != nil {
				t.Fatal(err)
			}
			if conn.Kind != c.want {
				t.Errorf("data-type=%q: Kind = %q, want %q", c.dataType, conn.Kind, c.want)
			}
		})
	}
}

package slddoc

import (
	"bytes"
	"math"
	"strings"
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
	// The stub tip (910,632), rotated 90° about the anchor — where the
	// wire meets it, not the anchor itself.
	if len(ports) != 1 || ports[0] != (Point{898, 620}) {
		t.Errorf("ports = %v, want a single port at the stub tip (898,620)", ports)
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
	// The stub's free end (1805,1122), rotated -90° about the anchor.
	if len(ports) != 1 || ports[0] != (Point{1795, 1132}) {
		t.Errorf("ports = %v, want a single port at the stub's free end (1795,1132)", ports)
	}
}

func TestParseGround_WithoutTransform(t *testing.T) {
	n := parseFirst(t, `<path d="M 1815 962 v -10 m -8 10 h 16 m -2 3 h -12 m 2 3 h 8" style="fill:none;stroke:#00A0F0;stroke-width:1" id="4009" data-voltage="#00A0F0" data-type="31"  />`)

	el, ports, _, err := parseGround(n)
	if err != nil {
		t.Fatal(err)
	}
	if el.X != 1815 || el.Y != 962 || el.Orient != 0 {
		t.Errorf("anchor/orient = (%v,%v,%d), want (1815,962,0) (no transform: the anchor is the path's own first point)", el.X, el.Y, el.Orient)
	}
	if len(ports) != 1 || ports[0] != (Point{1815, 952}) {
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

func TestParsePowerCircuitBreaker(t *testing.T) {
	cases := []struct {
		name       string
		svg        string
		wantX      float64
		wantY      float64
		wantState  int
		wantMirror bool
		wantPorts  []Point
	}{
		{
			// Closed, drawn with element_399.go's xMirror==0 geometry.
			name: "closed",
			svg: `<g id="1250" data-name="АВТСН1-0,4" data-type="399" >
<path d="M 240 916 v -12 m" data-state="1" style="fill:none;stroke:#555555;stroke-width:1" data-voltage="#555555" />
<path d="M 240 908 h -2 v -2 h 2 z " style="fill:#555555;stroke:#555555;stroke-width:1" data-voltage="#555555" />
<path d="M 236 901 h 8 m 0 18 h -8" style="fill:none;stroke:#555555;stroke-width:1" data-voltage="#555555" />
<path d="M 240 901 v -11 M 240 919 v 11" style="fill:none;stroke:#555555;stroke-width:1" data-voltage="#555555" />
</g>`,
			wantX: 240, wantY: 910, wantState: 1, wantMirror: true,
			wantPorts: []Point{{240, 890}, {240, 930}},
		},
		{
			// Open, drawn with the xMirror==1 geometry (base.xml's default).
			name: "open",
			svg: `<g id="148812638" data-type="399" >
<path d="M 444 160 h 12" data-state="0" style="fill:none;stroke:#FF5555;stroke-width:1" data-voltage="#FF5555" />
<path d="M 452 160 h 2 v 2 h -2 z " style="fill:#FF5555;stroke:#FF5555;stroke-width:1" data-voltage="#FF5555" />
<path d="M 446 151 h 8 m 0 18 h -8" style="fill:none;stroke:#FF5555;stroke-width:1" data-voltage="#FF5555" />
<path d="M 450 151 v -1 M 450 169 v 1" style="fill:none;stroke:#FF5555;stroke-width:1" data-voltage="#FF5555" />
</g>`,
			wantX: 450, wantY: 160, wantState: 0, wantMirror: false,
			wantPorts: []Point{{450, 150}, {450, 170}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			el, ports, _, err := parsePowerCircuitBreaker(parseFirst(t, c.svg))
			if err != nil {
				t.Fatal(err)
			}
			if el.Class != ClassPowerCircuitBreaker || el.Shape != "399" {
				t.Errorf("class/shape = %s/%s", el.Class, el.Shape)
			}
			if el.X != c.wantX || el.Y != c.wantY {
				t.Errorf("anchor = (%v,%v), want (%v,%v)", el.X, el.Y, c.wantX, c.wantY)
			}
			if el.State == nil || *el.State != c.wantState {
				t.Errorf("state = %v, want %d", el.State, c.wantState)
			}
			if el.Mirror != c.wantMirror {
				t.Errorf("mirror = %v, want %v", el.Mirror, c.wantMirror)
			}
			if len(ports) != 2 || ports[0] != c.wantPorts[0] || ports[1] != c.wantPorts[1] {
				t.Errorf("ports = %v, want %v", ports, c.wantPorts)
			}
		})
	}
}

func TestParsePolygon(t *testing.T) {
	n := parseFirst(t, `<polygon points="108,55 115,62 117,59 117,30 108,30" style="fill:#663300;stroke:white;stroke-dasharray: 70 20 25 20;stroke-width:2 " id="148796057" data-type="16" data-voltage="white" />`)
	el, err := parsePolygon(n)
	if err != nil {
		t.Fatal(err)
	}
	if el.ID != 148796057 || el.Class != ClassPolygon || el.Shape != "16" {
		t.Errorf("unexpected element: %+v", el)
	}
	if el.Fill != "#663300" || el.Stroke != "white" || el.StrokeWidth != 2 || el.LineStyle != LineStyleDashDot {
		t.Errorf("style = fill %q stroke %q width %v lineStyle %q", el.Fill, el.Stroke, el.StrokeWidth, el.LineStyle)
	}
	if len(el.Points) != 5 || el.Points[4] != (Point{108, 30}) || el.X != 108 || el.Y != 55 {
		t.Errorf("points = %v, anchor (%v,%v)", el.Points, el.X, el.Y)
	}

	if _, err := parsePolygon(parseFirst(t, `<polygon points="1,1 2,2" id="1" data-type="16" />`)); err == nil {
		t.Error("want an error for a 2-point polygon")
	}
}

func TestParseArc(t *testing.T) {
	n := parseFirst(t, `<path d="M399,2705 A65,15 1 1 0 490,2684" style="fill:none;stroke:#12161d;stroke-width:0.25" data-type="9" data-voltage="#12161d" />`)
	el, err := parseArc(n, 42)
	if err != nil {
		t.Fatal(err)
	}
	if el.ID != 42 || el.Class != ClassArc || el.Shape != "9" {
		t.Errorf("unexpected element: %+v", el)
	}
	if len(el.Points) != 2 || el.Points[0] != (Point{399, 2705}) || el.Points[1] != (Point{490, 2684}) {
		t.Errorf("points = %v", el.Points)
	}
	if el.RadiusX != 65 || el.RadiusY != 15 || !el.LargeArc || el.Sweep {
		t.Errorf("arc params = rx %v ry %v large %v sweep %v", el.RadiusX, el.RadiusY, el.LargeArc, el.Sweep)
	}
	if el.Stroke != "#12161d" || el.StrokeWidth != 0.25 {
		t.Errorf("style = %q %v", el.Stroke, el.StrokeWidth)
	}

	var buf bytes.Buffer
	writeArc(&buf, el, Static)
	want := `<path d="M399,2705 A65,15 1 1 0 490,2684" style="fill:none;stroke:#12161d;stroke-width:0.25" id="42" data-type="9" data-voltage="#12161d" />`
	if got := strings.TrimSpace(buf.String()); got != want {
		t.Errorf("writeArc =\n%s\nwant\n%s", got, want)
	}
}

func TestExtract_ArcWithoutIDGetsSynthesizedID(t *testing.T) {
	svg := `<svg width="100" height="100"><polygon points="0,0 1,0 1,1" style="fill:none;stroke:white;stroke-width:1" id="7" data-type="16" data-voltage="white" />` +
		`<path d="M10,10 A5,5 1 1 0 20,10" style="fill:none;stroke:white;stroke-width:0.25" data-type="9" data-voltage="white" />` +
		`<path d="M30,10 A5,5 1 1 0 40,10" style="fill:none;stroke:white;stroke-width:0.25" data-type="9" data-voltage="white" /></svg>`
	d, rep, err := Extract([]byte(svg), "t", nil)
	if err != nil {
		t.Fatal(err)
	}
	var ids []int
	for _, e := range d.Elements {
		if e.Class == ClassArc {
			ids = append(ids, e.ID)
		}
	}
	if len(ids) != 2 || ids[0] != 8 || ids[1] != 9 || len(rep.Failed) != 0 {
		t.Errorf("arc ids = %v, failed = %v, want [8 9] and none failed", ids, rep.Failed)
	}
	if d.LastID < 9 {
		t.Errorf("lastId = %d, want >= 9", d.LastID)
	}
}

func TestParseFork(t *testing.T) {
	cases := []struct {
		name       string
		svg        string
		wantOrient int
		wantRadius float64
		wantPorts  []Point
	}{
		{
			name:      "unrotated",
			svg:       `<path d="M 240 80 l 10 -10 m -10 10 l -10 -10" style="fill:none;stroke:#7F7F7F;stroke-width:1"  data-type="26" data-voltage="#7F7F7F" />`,
			wantPorts: []Point{{240, 80}, {250, 70}, {230, 70}},
		},
		{
			name:       "rotated -270 (normalized to 90)",
			svg:        `<path d="M 400 750 l 10 -10 m -10 10 l -10 -10" style="fill:none;stroke:#7F7F7F;stroke-width:1" transform="rotate(-270,400,750)" data-type="26" data-voltage="#7F7F7F" />`,
			wantOrient: 90,
			wantPorts:  []Point{{400, 750}, {410, 760}, {410, 740}},
		},
		{
			name:       "scaled",
			svg:        `<path d="M 153 657 l 7 -7 m -7 7 l -7 -7" style="fill:none;stroke:purple;stroke-width:1" transform="rotate(-180,153,657)" data-type="26" data-voltage="purple" />`,
			wantOrient: 180,
			wantRadius: 7,
			wantPorts:  []Point{{153, 657}, {146, 664}, {160, 664}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			el, ports, voltage, err := parseFork(parseFirst(t, c.svg), 5)
			if err != nil {
				t.Fatal(err)
			}
			if el.ID != 5 || el.Class != ClassFork || el.Shape != "26" || len(el.Ports) != 3 || voltage == "" {
				t.Errorf("unexpected element: %+v (voltage %q)", el, voltage)
			}
			if el.Orient != c.wantOrient || el.Radius != c.wantRadius {
				t.Errorf("orient/radius = %d/%v, want %d/%v", el.Orient, el.Radius, c.wantOrient, c.wantRadius)
			}
			for i := range c.wantPorts {
				if ports[i] != c.wantPorts[i] {
					t.Errorf("ports = %v, want %v", ports, c.wantPorts)
					break
				}
			}
		})
	}
}

func TestExtract_ForkConnectsAtItsVertex(t *testing.T) {
	svg := `<svg width="400" height="1000">` +
		`<polyline points="153,776 153,931" style="fill:none;stroke:purple;stroke-width:1 " data-type="21" id="56876" data-voltage="purple" />` +
		`<path d="M 153 776 l 7 -7 m -7 7 l -7 -7" style="fill:none;stroke:purple;stroke-width:1" data-type="26" data-voltage="purple" /></svg>`
	d, rep, err := Extract([]byte(svg), "t", nil)
	if err != nil {
		t.Fatal(err)
	}
	var fork *Element
	for i := range d.Elements {
		if d.Elements[i].Class == ClassFork {
			fork = &d.Elements[i]
		}
	}
	if fork == nil || len(rep.Failed) != 0 {
		t.Fatalf("no fork extracted (failed %v)", rep.Failed)
	}
	if fork.ID != 56877 {
		t.Errorf("id = %d, want synthesized 56877", fork.ID)
	}
	if len(d.Connectors) != 1 || fork.Ports[0].Node == 0 || fork.Ports[0].Node != d.Connectors[0].From {
		t.Errorf("vertex port node %d, connector %+v: want the wire to start at the fork's vertex", fork.Ports[0].Node, d.Connectors)
	}
}

// TestParseBooster uses real xsde2svg markup (ctrlroom sld1 corpus, shape
// 6 at export scale r=14): an unrotated instance with its regulation
// arrow, and a rotated one without it. The arrow and the winding mark are
// never rotated by the real source, so only the first path's own rotate()
// counts.
func TestParseBooster(t *testing.T) {
	cases := []struct {
		name       string
		svg        string
		wantX      float64
		wantY      float64
		wantOrient int
		wantTap    bool
		wantPorts  []Point
	}{
		{
			name: "unrotated with arrow",
			svg: `<g id="25956" data-voltage="purple" data-type="6"  >
<path d="M 2152 1130 h 4 a 14 14 0 0 1 28 0 a 14 14 0 0 1 -28 0 m 32 0 h -4" style="fill:none;stroke:purple;stroke-width:2"  />
<path d="M 2156 1144 l 42 -42 m 0 0 l -5 3 l 2 2 z" style="fill:#12161d;stroke:#12161d;stroke-width:1" />
<path d="M 2170 1126 v 4 l 5 5" style="fill:none;stroke:#12161d;stroke-width:1" />
</g>`,
			wantX: 2170, wantY: 1130, wantOrient: 0, wantTap: true,
			wantPorts: []Point{{2152, 1130}, {2188, 1130}},
		},
		{
			name: "rotated without arrow",
			svg: `<g id="30381" data-voltage="purple" data-type="6"  >
<path d="M 2182 1540 h 4 a 14 14 0 0 1 28 0 a 14 14 0 0 1 -28 0 m 32 0 h -4" style="fill:none;stroke:purple;stroke-width:2" transform="rotate(90,2200,1540)" />
<path d="M 2200 1536 v 4 l 5 5" style="fill:none;stroke:#12161d;stroke-width:1" />
</g>`,
			wantX: 2200, wantY: 1540, wantOrient: 90, wantTap: false,
			wantPorts: []Point{{2200, 1522}, {2200, 1558}},
		},
		{
			name: "rotated -270",
			svg: `<g id="1" data-voltage="purple" data-type="6"  >
<path d="M 72 100 h 7 a 21 21 0 0 1 42 0 a 21 21 0 0 1 -42 0 m 49 0 h -7" style="fill:none;stroke:purple;stroke-width:2" transform="rotate(-270,100,100)" />
</g>`,
			wantX: 100, wantY: 100, wantOrient: 90, wantTap: false,
			wantPorts: []Point{{100, 72}, {100, 128}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			el, ports, voltage, err := parseBooster(parseFirst(t, c.svg))
			if err != nil {
				t.Fatal(err)
			}
			if el.Class != ClassBooster || el.Shape != "6" || voltage != "purple" {
				t.Errorf("class/shape/voltage = %s/%s/%s", el.Class, el.Shape, voltage)
			}
			if el.X != c.wantX || el.Y != c.wantY || el.Orient != c.wantOrient {
				t.Errorf("anchor/orient = (%v,%v,%d), want (%v,%v,%d)", el.X, el.Y, el.Orient, c.wantX, c.wantY, c.wantOrient)
			}
			if el.TapChanger != c.wantTap {
				t.Errorf("tapChanger = %v, want %v", el.TapChanger, c.wantTap)
			}
			if len(ports) != 2 || ports[0] != c.wantPorts[0] || ports[1] != c.wantPorts[1] {
				t.Errorf("ports = %v, want %v", ports, c.wantPorts)
			}
			if len(el.Ports) != 2 {
				t.Errorf("element ports = %v, want 2", el.Ports)
			}
		})
	}
}

// TestParseResistor uses real xsde2svg markup (ctrlroom corpus, shape 156,
// a bare <path>): an unrotated instance, whose anchor falls back to the
// midpoint of its lead ends (1px off the real one, the source itself being
// asymmetric), and a rotated one, whose anchor is the rotate() center.
func TestParseResistor(t *testing.T) {
	cases := []struct {
		name       string
		svg        string
		wantX      float64
		wantY      float64
		wantOrient int
		wantPorts  []Point
	}{
		{
			name:  "unrotated",
			svg:   `<path d="M 734 920 h 15 m 0 8 v -16 h 40 v 16 z m 40 -8 h 15" style="fill:none;stroke:#C9A0DC;stroke-width:1"  id="8448" data-voltage="#C9A0DC" data-type="156" />`,
			wantX: 769, wantY: 920, wantOrient: 0,
			wantPorts: []Point{{734, 920}, {804, 920}},
		},
		{
			name:  "rotated",
			svg:   `<path d="M 2084 930 h 15 m 0 8 v -16 h 40 v 16 z m 40 -8 h 15" style="fill:none;stroke:#C9A0DC;stroke-width:1" transform="rotate(90,2120,930)" id="4306" data-voltage="#C9A0DC" data-type="156" />`,
			wantX: 2120, wantY: 930, wantOrient: 90,
			wantPorts: []Point{{2120, 894}, {2120, 964}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			el, ports, voltage, err := parseTwoPortDevice(parseFirst(t, c.svg), twoPortShapes["156"], "156")
			if err != nil {
				t.Fatal(err)
			}
			if el.Class != ClassResistor || el.Shape != "156" || voltage != "#C9A0DC" {
				t.Errorf("class/shape/voltage = %s/%s/%s", el.Class, el.Shape, voltage)
			}
			if el.X != c.wantX || el.Y != c.wantY || el.Orient != c.wantOrient {
				t.Errorf("anchor/orient = (%v,%v,%d), want (%v,%v,%d)", el.X, el.Y, el.Orient, c.wantX, c.wantY, c.wantOrient)
			}
			if len(ports) != 2 || ports[0] != c.wantPorts[0] || ports[1] != c.wantPorts[1] {
				t.Errorf("ports = %v, want %v", ports, c.wantPorts)
			}
		})
	}
}

// TestParseThyristor uses real xsde2svg markup (ctrlroom corpus, shape 157,
// "Нива" hydro plants): an unrotated instance, whose anchor falls back to
// the midpoint of its lead ends (1px off the real one, the source itself
// being asymmetric), and one rotated by 180, whose anchor is the rotate()
// center. The gate stub's free end is the third port.
func TestParseThyristor(t *testing.T) {
	cases := []struct {
		name       string
		svg        string
		wantX      float64
		wantY      float64
		wantOrient int
		wantPorts  []Point
	}{
		{
			name: "unrotated",
			svg: `<g id="36186" data-type="157"  data-voltage="#6600CC" >
<path d="M 1460 1380 l -8 0 l -20 10 l 0 -20 l 20 10 m -20 0 l -10 0 m 30 10 l 0 -20 l 5 -5 l 0 -4" style="fill:#12161d;stroke:#6600CC;stroke-width:1" />
</g>`,
			wantX: 1441, wantY: 1380, wantOrient: 0,
			wantPorts: []Point{{1422, 1380}, {1460, 1380}, {1457, 1361}},
		},
		{
			name: "rotated 180",
			svg: `<g id="33372" data-type="157" transform="rotate(180,270,1660)" data-voltage="#6600CC" >
<path d="M 290 1660 l -8 0 l -20 10 l 0 -20 l 20 10 m -20 0 l -10 0 m 30 10 l 0 -20 l 5 -5 l 0 -4" style="fill:#12161d;stroke:#6600CC;stroke-width:1" />
</g>`,
			wantX: 270, wantY: 1660, wantOrient: 180,
			wantPorts: []Point{{288, 1660}, {250, 1660}, {253, 1679}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			el, ports, voltage, err := parseThyristor(parseFirst(t, c.svg))
			if err != nil {
				t.Fatal(err)
			}
			if el.Class != ClassThyristor || el.Shape != "157" || voltage != "#6600CC" {
				t.Errorf("class/shape/voltage = %s/%s/%s", el.Class, el.Shape, voltage)
			}
			if el.X != c.wantX || el.Y != c.wantY || el.Orient != c.wantOrient {
				t.Errorf("anchor/orient = (%v,%v,%d), want (%v,%v,%d)", el.X, el.Y, el.Orient, c.wantX, c.wantY, c.wantOrient)
			}
			if len(ports) != 3 || ports[0] != c.wantPorts[0] || ports[1] != c.wantPorts[1] || ports[2] != c.wantPorts[2] {
				t.Errorf("ports = %v, want %v", ports, c.wantPorts)
			}
			if len(el.Ports) != 3 || el.Ports[2].Name != "3" {
				t.Errorf("element ports = %v, want 1, 2, 3", el.Ports)
			}
		})
	}
}

// TestParseShortCircuiterNoGround uses real xsde2svg markup (ctrlroom fixed
// corpus, shape 163): an unrotated Closed instance (anchor from the
// geometry), a rotated, mirrored Open one, and one with bus-spacing legs,
// whose ports are the legs' outer ends (export scale 1.4, so ±28).
func TestParseShortCircuiterNoGround(t *testing.T) {
	cases := []struct {
		name       string
		svg        string
		wantX      float64
		wantY      float64
		wantOrient int
		wantMirror bool
		wantState  int
		wantPorts  []Point
	}{
		{
			name: "unrotated closed",
			svg: `<g id="803" data-voltage="deepskyblue" data-type="163" data-name="ОДТ-1-110"  >
<path d="M 505 590 h 10 M 505 570 h 10 M 510 571 v 18 M 510 580 h -11 " data-state="1" style="fill:none;stroke:deepskyblue;stroke-width:1" />
<path d="M 510 580 l -8 3 v -6 z" style="fill:deepskyblue;stroke:deepskyblue;stroke-width:1" />
</g>`,
			wantX: 510, wantY: 580, wantOrient: 0, wantMirror: false, wantState: 1,
			wantPorts: []Point{{510, 570}, {510, 590}},
		},
		{
			name: "rotated mirrored open",
			svg: `<g id="120954787" data-voltage="deepskyblue" data-type="163" data-name="КЗТ-2-110(А)" transform="rotate(-90,870,950)" >
<path d="M 870 959 a 2 2 0 1 1 0 4 a 2 2 0 0 1 0 -4" style="fill:#12161d;stroke:deepskyblue;stroke-width:1" />
<path d="M 866 940 h 8 M 874 950 h 11 M 876 945 l -6 14" data-state="0" style="fill:none;stroke:deepskyblue;stroke-width:1" />
<path d="M 874 950 l 8 3 v -6 z " style="fill:deepskyblue;stroke:deepskyblue;stroke-width:1" />
</g>`,
			wantX: 870, wantY: 950, wantOrient: -90, wantMirror: true, wantState: 0,
			wantPorts: []Point{{860, 950}, {880, 950}},
		},
		{
			name: "legs",
			svg: `<g id="34160" data-voltage="deepskyblue" data-type="163"  transform="rotate(-90,1453,500)" >
<path d="M 1453 472 v 14 M 1453 528 v -14" style="fill:none;stroke:deepskyblue;stroke-width:1" />
<path d="M 1453 512 a 2 2 0 1 1 0 4 a 2 2 0 0 1 0 -4" style="fill:#12161d;stroke:deepskyblue;stroke-width:1" />
<path d="M 1449 486 h 11 M 1449 500 h -15 M 1445 493 l 8 19" data-state="0" style="fill:none;stroke:deepskyblue;stroke-width:1" />
<path d="M 1448 500 l -11 4 v -8 z " style="fill:deepskyblue;stroke:deepskyblue;stroke-width:1" />
</g>`,
			wantX: 1453, wantY: 500, wantOrient: -90, wantMirror: false, wantState: 0,
			wantPorts: []Point{{1425, 500}, {1481, 500}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			el, ports, voltage, err := parseShortCircuiterNoGround(parseFirst(t, c.svg))
			if err != nil {
				t.Fatal(err)
			}
			if el.Class != ClassShortCircuiterNoGround || el.Shape != "163" || voltage != "deepskyblue" {
				t.Errorf("class/shape/voltage = %s/%s/%s", el.Class, el.Shape, voltage)
			}
			if el.X != c.wantX || el.Y != c.wantY || el.Orient != c.wantOrient || el.Mirror != c.wantMirror {
				t.Errorf("anchor/orient/mirror = (%v,%v,%d,%v), want (%v,%v,%d,%v)", el.X, el.Y, el.Orient, el.Mirror, c.wantX, c.wantY, c.wantOrient, c.wantMirror)
			}
			if el.State == nil || *el.State != c.wantState {
				t.Errorf("state = %v, want %d", el.State, c.wantState)
			}
			if len(ports) != 2 || math.Abs(ports[0].X-c.wantPorts[0].X) > 1e-9 || math.Abs(ports[0].Y-c.wantPorts[0].Y) > 1e-9 ||
				math.Abs(ports[1].X-c.wantPorts[1].X) > 1e-9 || math.Abs(ports[1].Y-c.wantPorts[1].Y) > 1e-9 {
				t.Errorf("ports = %v, want %v", ports, c.wantPorts)
			}
		})
	}
}

// TestParseDisconnectorFuse uses real xsde2svg markup (ctrlroom fixed
// corpus, shape 166): an unrotated Closed instance at export scale 2 (legs
// 3 long, so ports ±21), an unrotated Open one (anchor from the top bar
// and the blade's pivot), and an Open one rotated by an inner <g>.
func TestParseDisconnectorFuse(t *testing.T) {
	cases := []struct {
		name       string
		svg        string
		wantX      float64
		wantY      float64
		wantOrient int
		wantState  int
		wantPorts  []Point
	}{
		{
			name: "unrotated closed",
			svg: `<g id="120954436" data-type="166" >
<path d="M 240 1648 v 24 m 0 -24 h 4 v 24 h -8 v -24 h 4" data-state="1" style="fill:none;stroke:purple;stroke-width:1" data-voltage="purple" data-name="РПТСН1-10" />
<path d="M 232 1642 h 16 m 0 36 h -16" style="fill:none;stroke:purple;stroke-width:1" />
<path d="M 240 1642 v -3 M 240 1678 v 3" style="fill:none;stroke:purple;stroke-width:1" />
</g>`,
			wantX: 240, wantY: 1660, wantOrient: 0, wantState: 1,
			wantPorts: []Point{{240, 1639}, {240, 1681}},
		},
		{
			name: "unrotated open",
			svg: `<g id="120952948" data-type="166" >
<path d="M 590 1929 l -8 -16 m -1 3 l 4 -2 l 5 10 l -4 2 z" data-state="0" style="fill:none;stroke:purple;stroke-width:1" data-voltage="purple" data-name="ПМ1" />
<path d="M 590 1926 a 2 2 0 1 1 0 4 a 2 2 0 0 1 0 -4" style="fill:#12161d;stroke:purple;stroke-width:1" />
<path d="M 586 1911 h 8" style="fill:none;stroke:purple;stroke-width:1" />
<path d="M 590 1911 v -1 M 590 1929 v 1" style="fill:none;stroke:purple;stroke-width:1" />
</g>`,
			wantX: 590, wantY: 1920, wantOrient: 0, wantState: 0,
			wantPorts: []Point{{590, 1910}, {590, 1930}},
		},
		{
			name: "rotated open",
			svg: `<g id="1360" data-type="166" >
<g transform="rotate(90,740,1430)" >
<path d="M 740 1439 l -8 -16 m -1 3 l 4 -2 l 5 10 l -4 2 z" data-state="0" style="fill:none;stroke:purple;stroke-width:1" data-voltage="purple" data-name="ПМ-2" />
<path d="M 740 1436 a 2 2 0 1 1 0 4 a 2 2 0 0 1 0 -4" style="fill:#12161d;stroke:purple;stroke-width:1" />
<path d="M 736 1421 h 8" style="fill:none;stroke:purple;stroke-width:1" />
<path d="M 740 1421 v -1 M 740 1439 v 1" style="fill:none;stroke:purple;stroke-width:1" />
</g>
</g>`,
			wantX: 740, wantY: 1430, wantOrient: 90, wantState: 0,
			wantPorts: []Point{{750, 1430}, {730, 1430}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			el, ports, voltage, err := parseDisconnectorFuse(parseFirst(t, c.svg))
			if err != nil {
				t.Fatal(err)
			}
			if el.Class != ClassDisconnectorFuse || el.Shape != "166" || voltage != "purple" || el.Name == "" {
				t.Errorf("class/shape/voltage/name = %s/%s/%s/%q", el.Class, el.Shape, voltage, el.Name)
			}
			if el.X != c.wantX || el.Y != c.wantY || el.Orient != c.wantOrient || el.Mirror {
				t.Errorf("anchor/orient/mirror = (%v,%v,%d,%v), want (%v,%v,%d,false)", el.X, el.Y, el.Orient, el.Mirror, c.wantX, c.wantY, c.wantOrient)
			}
			if el.State == nil || *el.State != c.wantState {
				t.Errorf("state = %v, want %d", el.State, c.wantState)
			}
			if len(ports) != 2 || math.Abs(ports[0].X-c.wantPorts[0].X) > 1e-9 || math.Abs(ports[0].Y-c.wantPorts[0].Y) > 1e-9 ||
				math.Abs(ports[1].X-c.wantPorts[1].X) > 1e-9 || math.Abs(ports[1].Y-c.wantPorts[1].Y) > 1e-9 {
				t.Errorf("ports = %v, want %v", ports, c.wantPorts)
			}
		})
	}
}

// TestParseSynchronousCompensator uses real xsde2svg markup (ctrlroom
// fixed corpus, shape 174): the anchor and only port are the stem's tip,
// the circle path's first point, like Generator (173).
func TestParseSynchronousCompensator(t *testing.T) {
	svg := `<g id="33352" data-voltage="purple" data-type="174" >
<path d="M 1040 2699 v 5 m -16 16 a 16 16 0 0 1 32 0 a 16 16 0 0 1 -32 0" style="fill:none;stroke:purple;stroke-width:2"  />
<path d="M 1033 2723 h 14 m -14 -6 h 14" style="fill:none;stroke:purple;stroke-width:2" />
</g>`
	el, ports, voltage, err := parseOnePortDevice(parseFirst(t, svg), ClassSynchronousCompensator, "174")
	if err != nil {
		t.Fatal(err)
	}
	if el.Class != ClassSynchronousCompensator || el.Shape != "174" || voltage != "purple" {
		t.Errorf("class/shape/voltage = %s/%s/%s", el.Class, el.Shape, voltage)
	}
	if el.X != 1040 || el.Y != 2699 || el.Orient != 0 {
		t.Errorf("anchor/orient = (%v,%v,%d), want (1040,2699,0)", el.X, el.Y, el.Orient)
	}
	if len(ports) != 1 || ports[0] != (Point{1040, 2699}) {
		t.Errorf("ports = %v, want [(1040,2699)]", ports)
	}
}

// TestParseSynchronousMotor uses real xsde2svg markup (ctrlroom sld1
// corpus, shape 39): the anchor and only port
// are the stem's tip, the circle path's first point, rotated through the
// path's rotate() about the circle's center when there is one.
func TestParseSynchronousMotor(t *testing.T) {
	cases := []struct {
		name       string
		svg        string
		wantX      float64
		wantY      float64
		wantOrient int
	}{
		{"unrotated", `<g id="12913" data-voltage="#CC9900" data-type="39" >
<path d="M 1140 1733 v 5 m -12 12 a 12 12 0 0 1 24 0 a 12 12 0 0 1 -24 0" style="fill:none;stroke:#CC9900;stroke-width:2"  />
<text x="1140" y="1750" style="fill:#CC9900;font-size:12px;font-family:Arial;text-anchor:middle;dominant-baseline:middle"  >M</text>
</g>`, 1140, 1733, 0},
		{"rotated", `<g id="29570" data-voltage="#CC9900" data-type="39" >
<path d="M 890 757 v 7 m -16 16 a 16 16 0 0 1 32 0 a 16 16 0 0 1 -32 0" style="fill:none;stroke:#CC9900;stroke-width:2" transform="rotate(180,890,780)" />
<text x="890" y="780" style="fill:#CC9900;font-size:16px;font-family:Arial;text-anchor:middle;dominant-baseline:middle" transform="rotate(180,890,780)" >M</text>
</g>`, 890, 803, 180},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			el, ports, voltage, err := parseOnePortDevice(parseFirst(t, c.svg), ClassSynchronousMotor, "39")
			if err != nil {
				t.Fatal(err)
			}
			if el.Class != ClassSynchronousMotor || el.Shape != "39" || voltage != "#CC9900" {
				t.Errorf("class/shape/voltage = %s/%s/%s", el.Class, el.Shape, voltage)
			}
			if math.Abs(el.X-c.wantX) > 1e-9 || math.Abs(el.Y-c.wantY) > 1e-9 || el.Orient != c.wantOrient {
				t.Errorf("anchor/orient = (%v,%v,%d), want (%v,%v,%d)", el.X, el.Y, el.Orient, c.wantX, c.wantY, c.wantOrient)
			}
			if len(ports) != 1 || math.Abs(ports[0].X-c.wantX) > 1e-9 || math.Abs(ports[0].Y-c.wantY) > 1e-9 {
				t.Errorf("ports = %v, want [(%v,%v)]", ports, c.wantX, c.wantY)
			}
		})
	}
}

// TestParseKnifeSwitch3 has no real corpus instance to use (none exist), so
// its markup is element_175.go's own output format at two positions: the
// ports are the circles' centers (pivot, left, right) and State stays
// unset (the source's only, middle, position).
func TestParseKnifeSwitch3(t *testing.T) {
	cases := []struct {
		name       string
		svg        string
		wantX      float64
		wantY      float64
		wantOrient int
		wantPorts  []Point
	}{
		{
			name: "unrotated",
			svg: `<g id="7" data-type="175" data-voltage="purple" >
<path d="M 100 190 v 15" style="fill:purple;stroke:purple;stroke-width:4" />
<path d="M 98 205 a 2 2 0 0 1 4 0 a 2 2 0 0 1 -4 0 m 10 -15 a 2 2 0 0 1 4 0 a 2 2 0 0 1 -4 0 m -20 0 a 2 2 0 0 1 4 0 a 2 2 0 0 1 -4 0" style="fill:#12161d;stroke:purple;stroke-width:2" />
</g>`,
			wantX: 100, wantY: 200, wantOrient: 0,
			wantPorts: []Point{{100, 205}, {90, 190}, {110, 190}},
		},
		{
			name: "rotated 180",
			svg: `<g id="7" data-type="175" transform="rotate(180,100,200)" data-voltage="purple" >
<path d="M 100 190 v 15" style="fill:purple;stroke:purple;stroke-width:4" />
<path d="M 98 205 a 2 2 0 0 1 4 0 a 2 2 0 0 1 -4 0 m 10 -15 a 2 2 0 0 1 4 0 a 2 2 0 0 1 -4 0 m -20 0 a 2 2 0 0 1 4 0 a 2 2 0 0 1 -4 0" style="fill:#12161d;stroke:purple;stroke-width:2" />
</g>`,
			wantX: 100, wantY: 200, wantOrient: 180,
			wantPorts: []Point{{100, 195}, {110, 210}, {90, 210}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			el, ports, voltage, err := parseKnifeSwitch(parseFirst(t, c.svg), ClassKnifeSwitch3, "175")
			if err != nil {
				t.Fatal(err)
			}
			if el.Class != ClassKnifeSwitch3 || el.Shape != "175" || voltage != "purple" || el.State != nil {
				t.Errorf("class/shape/voltage/state = %s/%s/%s/%v", el.Class, el.Shape, voltage, el.State)
			}
			if el.X != c.wantX || el.Y != c.wantY || el.Orient != c.wantOrient {
				t.Errorf("anchor/orient = (%v,%v,%d), want (%v,%v,%d)", el.X, el.Y, el.Orient, c.wantX, c.wantY, c.wantOrient)
			}
			if len(ports) != 3 || len(el.Ports) != 3 {
				t.Fatalf("ports = %v / %v, want 3", ports, el.Ports)
			}
			for i := range ports {
				if math.Abs(ports[i].X-c.wantPorts[i].X) > 1e-9 || math.Abs(ports[i].Y-c.wantPorts[i].Y) > 1e-9 {
					t.Errorf("ports = %v, want %v", ports, c.wantPorts)
					break
				}
			}
		})
	}
}

// TestParseKnifeSwitch uses real xsde2svg markup for shape 44 (ctrlroom
// sld1 corpus: Switches.svg id 7, unrotated, and id 7163, rotated -90):
// the same three circles as shape 175, with the blade on the left contact.
func TestParseKnifeSwitch(t *testing.T) {
	cases := []struct {
		name       string
		svg        string
		wantX      float64
		wantY      float64
		wantOrient int
		wantPorts  []Point
	}{
		{
			name: "unrotated",
			svg: `<g id="7" data-type="44"  >
<path d="M 240 50 l 10 15" style="fill:#FF5555;stroke:#FF5555;stroke-width:4" />
<path d="M 248 65 a 2 2 0 0 1 4 0 a 2 2 0 0 1 -4 0 m 10 -15 a 2 2 0 0 1 4 0 a 2 2 0 0 1 -4 0 m -20 0 a 2 2 0 0 1 4 0 a 2 2 0 0 1 -4 0" style="fill:#f5ebeb;stroke:#FF5555;stroke-width:2" />
</g>`,
			wantX: 250, wantY: 60, wantOrient: 0,
			wantPorts: []Point{{250, 65}, {240, 50}, {260, 50}},
		},
		{
			name: "rotated -90",
			svg: `<g id="7163" data-type="44" transform="rotate(-90,1790,890)" >
<path d="M 1780 880 l 10 15" style="fill:#555555;stroke:#555555;stroke-width:4" />
<path d="M 1788 895 a 2 2 0 0 1 4 0 a 2 2 0 0 1 -4 0 m 10 -15 a 2 2 0 0 1 4 0 a 2 2 0 0 1 -4 0 m -20 0 a 2 2 0 0 1 4 0 a 2 2 0 0 1 -4 0" style="fill:#12161d;stroke:#555555;stroke-width:2" />
</g>`,
			wantX: 1790, wantY: 890, wantOrient: -90,
			wantPorts: []Point{{1795, 890}, {1780, 900}, {1780, 880}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			el, ports, _, err := parseKnifeSwitch(parseFirst(t, c.svg), ClassKnifeSwitch, "44")
			if err != nil {
				t.Fatal(err)
			}
			if el.Class != ClassKnifeSwitch || el.Shape != "44" || el.State != nil {
				t.Errorf("class/shape/state = %s/%s/%v", el.Class, el.Shape, el.State)
			}
			if el.X != c.wantX || el.Y != c.wantY || el.Orient != c.wantOrient {
				t.Errorf("anchor/orient = (%v,%v,%d), want (%v,%v,%d)", el.X, el.Y, el.Orient, c.wantX, c.wantY, c.wantOrient)
			}
			if len(ports) != 3 || len(el.Ports) != 3 {
				t.Fatalf("ports = %v / %v, want 3", ports, el.Ports)
			}
			for i := range ports {
				if math.Abs(ports[i].X-c.wantPorts[i].X) > 1e-9 || math.Abs(ports[i].Y-c.wantPorts[i].Y) > 1e-9 {
					t.Errorf("ports = %v, want %v", ports, c.wantPorts)
					break
				}
			}
		})
	}
}

// TestWindowIcon_RealCorpusRoundTrip extracts a real xsde2svg Window icon
// (shape 302, ПС 35 кВ 49С Валаам.svg, id 120957326) and renders it back:
// the same box, and the label at the same 12px position, with its empty
// "fill:" written as the black a browser draws it in.
func TestWindowIcon_RealCorpusRoundTrip(t *testing.T) {
	const real = `<?xml version="1.0"?>
<svg width="3910" height="1900" xmlns="http://www.w3.org/2000/svg">
<!-- иконка_окна:302 -->
<g id="120957326" data-type="302" >
<rect x="2210" y="950" width="40" height="30" style="fill:orange;stroke:black;stroke-width:1" />
<text x="2230" y="967" style="fill:;text-anchor:middle;dominant-baseline:middle;font-size:12px;font-family:Arial " >Окно</text>
</g>
</svg>`
	d, report, err := Extract([]byte(real), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Failed) != 0 || len(report.Skipped) != 0 || len(d.Elements) != 1 {
		t.Fatalf("failed=%v skipped=%v elements=%+v", report.Failed, report.Skipped, d.Elements)
	}
	e := d.Elements[0]
	want := Element{ID: 120957326, Class: ClassWindowIcon, Shape: "302", X: 2230, Y: 965, Fill: "orange", Stroke: "black",
		PropertyText: "Окно", Points: []Point{{X: 2210, Y: 950}, {X: 2250, Y: 980}}}
	if e.ID != want.ID || e.Class != want.Class || e.Shape != want.Shape || e.X != want.X || e.Y != want.Y ||
		e.Fill != want.Fill || e.Stroke != want.Stroke || e.StrokeWidth != 0 || e.TextColor != "" ||
		e.PropertyText != want.PropertyText || len(e.Points) != 2 || e.Points[0] != want.Points[0] || e.Points[1] != want.Points[1] {
		t.Fatalf("extracted %+v, want %+v", e, want)
	}

	var buf bytes.Buffer
	if err := Render(d, NewSymbolLibrary(nil), &buf, Static, "", nil); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, s := range []string{
		`<!-- Window icon:302 -->`,
		`<g id="120957326" data-type="302" data-name="" data-voltage="black">`,
		`<rect x="2210" y="950" width="40" height="30" style="fill:orange;stroke:black;stroke-width:1" />`,
		`<text x="2230" y="967" style="fill:black;text-anchor:middle;dominant-baseline:middle;font-size:12px;font-family:Arial">Окно</text>`,
	} {
		if !strings.Contains(out, s) {
			t.Errorf("render missing %q:\n%s", s, out)
		}
	}
}

// TestContainer_RealCorpus reads real xsde2svg Containers (shape 310) in
// both export forms: the older one (caption alone in the group, the
// outline as the bare <path> after it; ПС 110 Демянск.svg and a rotated
// caption from another real file) and the patched one (outline and
// caption in one group, from Test_310_conteyner.xsde), captioned or not.
// Each renders back in the patched form at the same geometry.
func TestContainer_RealCorpus(t *testing.T) {
	const svg = `<?xml version="1.0"?>
<svg width="10000" height="5000" xmlns="http://www.w3.org/2000/svg">
<!-- контейнер:310 -->
<g data-type="310" data-name="ПС 110 кВ Демянск" data-event="dc" data-layer="10" id="120996397" >
<text x="485" y="440" style="fill:none;text-anchor:middle;dominant-baseline:text-before-edge;font-size:28px;font-family:Arial"  >ПС 110 кВ Демянск</text>
</g>
<path d="M 670 440 L300 440 L300 130 L670 130 z" style="fill:none;stroke:dimgray;stroke-dasharray: 3,2;stroke-width:1 " />
<g data-type="310" data-name="ПС Сельхозкомплекс" data-event="dc" data-layer="10" id="120991010" >
<text x="9409" y="3554" style="fill:white;text-anchor:end;dominant-baseline:baseline;font-size:39px;font-family:Arial" transform="rotate(-90,9409,3554)" >ПС Сельхозкомплекс</text>
</g>
<path d="M 9320 3360 L9320 3540 L9470 3540 L9470 3360 z" style="fill:none;stroke:gray;stroke-dasharray: 3,2;stroke-width:1 " />
<g data-type="310"   data-layer="10" id="148694366" data-voltage="dimgray" >
<path d="M 620 80 L410 80 L410 40 L620 40 z" style="fill:none;stroke:dimgray;stroke-width:1 " />
</g>
<g data-type="310" data-name="Бор" data-event="dc" data-layer="10" id="148701673" data-voltage="gray" >
<path d="M 1130 150 L1130 100 L910 100 L910 150 z" style="fill:none;stroke:gray;stroke-dasharray: 3,2;stroke-width:1 " />
<text x="1020" y="150" style="fill:none;text-anchor:middle;dominant-baseline:text-before-edge;font-size:39px;font-family:Arial Narrow"  >Бор</text>
</g>
</svg>`
	d, report, err := Extract([]byte(svg), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Failed) != 0 || len(report.Skipped) != 0 || len(d.Elements) != 4 {
		t.Fatalf("failed=%v skipped=%v elements=%d", report.Failed, report.Skipped, len(d.Elements))
	}
	byID := map[int]Element{}
	for _, e := range d.Elements {
		if e.Class != ClassContainer || e.Shape != "310" {
			t.Errorf("element %d: class %s shape %s", e.ID, e.Class, e.Shape)
		}
		byID[e.ID] = e
	}
	dem := byID[120996397]
	if len(dem.Points) != 4 || dem.Points[0] != (Point{670, 440}) || dem.Stroke != "dimgray" || dem.LineStyle != LineStyleDotted ||
		dem.PropertyText != "ПС 110 кВ Демянск" || dem.TextColor != "none" || dem.TextSize != 28 ||
		dem.TextAnchor != "middle" || dem.TextBaseline != "text-before-edge" || dem.TextDx != 185 || dem.TextDy != 310 || dem.Layer != 10 {
		t.Errorf("Демянск = %+v", dem)
	}
	rot := byID[120991010]
	if rot.Orient != -90 || rot.TextDx != 89 || rot.TextDy != 194 || rot.TextAnchor != "end" || rot.TextBaseline != "baseline" {
		t.Errorf("rotated caption = %+v", rot)
	}
	plain := byID[148694366]
	if plain.PropertyText != "" || len(plain.Points) != 4 || plain.LineStyle != "" {
		t.Errorf("uncaptioned = %+v", plain)
	}

	var buf bytes.Buffer
	if err := Render(d, NewSymbolLibrary(nil), &buf, Static, "", nil); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, s := range []string{
		`<!-- Container:310 -->`,
		`<g data-layer="10" id="120996397" data-type="310" data-name="ПС 110 кВ Демянск" data-voltage="dimgray">
<path d="M 670 440 L300 440 L300 130 L670 130 z" style="fill:none;stroke:dimgray;stroke-dasharray: 3,2;stroke-width:1" />
<text x="485" y="440" style="fill:none;text-anchor:middle;dominant-baseline:text-before-edge;font-size:28px;font-family:Arial">ПС 110 кВ Демянск</text>
</g>`,
		`<text x="9409" y="3554" style="fill:white;text-anchor:end;dominant-baseline:baseline;font-size:39px;font-family:Arial" transform="rotate(-90,9409,3554)">ПС Сельхозкомплекс</text>`,
		`<g data-layer="10" id="148694366" data-type="310" data-voltage="dimgray">
<path d="M 620 80 L410 80 L410 40 L620 40 z" style="fill:none;stroke:dimgray;stroke-width:1" />
</g>`,
	} {
		if !strings.Contains(out, s) {
			t.Errorf("render missing %q:\n%s", s, out)
		}
	}
	again, report, err := Extract(buf.Bytes(), "", nil)
	if err != nil || len(report.Failed) != 0 || len(again.Elements) != 4 {
		t.Fatalf("re-extract: err=%v failed=%v n=%d", err, report.Failed, len(again.Elements))
	}
}

// TestPowerPole_RealCorpus extracts real xsde2svg Power poles (shape 146:
// ПС 110 кВ Крутая.svg rotated 180, Поопорная схема ТП Кувизино rotated
// 90, and an unrotated one) as two-terminal devices anchored at the
// circle's center with ports at the lead tips, and renders one back to the
// same path.
func TestPowerPole_RealCorpus(t *testing.T) {
	const svg = `<?xml version="1.0"?>
<svg width="4000" height="2000" xmlns="http://www.w3.org/2000/svg">
<!-- электроопора:146 -->
<path d="M 340 1030 v 2 a 8 8 0 1 0 0 16 v 2 v -2 a 8 8 0 1 0 0 -16 z" style="fill:none;stroke:purple;stroke-width:1" transform="rotate(180,340,1040)" id="120959254" data-type="146"  data-voltage="purple" />
<path d="M 758 1290 v 2 a 8 8 0 1 0 0 16 v 2 v -2 a 8 8 0 1 0 0 -16 z" style="fill:none;stroke:#962896;stroke-width:1" transform="rotate(90,758,1300)" id="148703450" data-type="146"  data-voltage="#962896" />
<path d="M 3100 556 v 2 a 8 8 0 1 0 0 16 v 2 v -2 a 8 8 0 1 0 0 -16 z" style="fill:none;stroke:#C9A0DC;stroke-width:1"  id="120960230" data-type="146"  data-voltage="#C9A0DC" />
</svg>`
	d, report, err := Extract([]byte(svg), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Failed) != 0 || len(report.Skipped) != 0 || len(d.Elements) != 3 {
		t.Fatalf("failed=%v skipped=%v elements=%d", report.Failed, report.Skipped, len(d.Elements))
	}
	nodeAt := map[int]Point{}
	for _, n := range d.Nodes {
		nodeAt[n.ID] = Point{n.X, n.Y}
	}
	want := map[int]struct {
		x, y   float64
		orient int
		tips   [2]Point
	}{
		120959254: {340, 1040, 180, [2]Point{{340, 1030}, {340, 1050}}},
		148703450: {758, 1300, 90, [2]Point{{748, 1300}, {768, 1300}}},
		120960230: {3100, 566, 0, [2]Point{{3100, 556}, {3100, 576}}},
	}
	for _, e := range d.Elements {
		w := want[e.ID]
		if e.Class != ClassPowerPole || e.Shape != "146" || e.X != w.x || e.Y != w.y || e.Orient != w.orient || e.Voltage == 0 || len(e.Ports) != 2 {
			t.Errorf("pole %d = %+v, want (%v,%v) orient %d", e.ID, e, w.x, w.y, w.orient)
			continue
		}
		got := map[Point]bool{nodeAt[e.Ports[0].Node]: true, nodeAt[e.Ports[1].Node]: true}
		if !got[w.tips[0]] || !got[w.tips[1]] {
			t.Errorf("pole %d ports at %v, want %v", e.ID, got, w.tips)
		}
	}

	lib := NewSymbolLibrary(map[string]string{
		"146": `<path d="M 0 -10 v 2 a 8 8 0 1 0 0 16 v 2 v -2 a 8 8 0 1 0 0 -16 z" style="fill:none;stroke:{color};stroke-width:1" />`,
	})
	var buf bytes.Buffer
	if err := Render(d, lib, &buf, Static, "", nil); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		`<!-- Power pole:146 -->`,
		`data-type="146" transform="rotate(180,340,1040)">
<path d="M 340 1030 v 2 a 8 8 0 1 0 0 16 v 2 v -2 a 8 8 0 1 0 0 -16 z" style="fill:none;stroke:purple;stroke-width:1" />`,
	} {
		if !strings.Contains(buf.String(), s) {
			t.Errorf("render missing %q:\n%s", s, buf.String())
		}
	}
	again, report, err := Extract(buf.Bytes(), "", nil)
	if err != nil || len(report.Failed) != 0 || len(again.Elements) != 3 {
		t.Fatalf("re-extract: err=%v failed=%v n=%d", err, report.Failed, len(again.Elements))
	}
}

// TestLampOnPole_RealCorpus reads real xsde2svg Lamps on pole (shape
// 320002, Поопорная схема ТП ул.Энергетиков д.20 Л-3 ПС Валдай.svg and a
// rotated one), plus the old source's duplicated data-voltage, and renders
// one back to the same circle and diagonals.
func TestLampOnPole_RealCorpus(t *testing.T) {
	const svg = `<?xml version="1.0"?>
<svg width="2000" height="2000" xmlns="http://www.w3.org/2000/svg">
<g id="148706395"  data-type="320002" >
<circle cx="671" cy="983" r="6" style="fill:none;stroke:gray;stroke-width:1" />
<path d="M 667 979 l 8 8 " style="fill:none;stroke:gray;stroke-width:1" />
<path d="M 675 979 l -8 8 " style="fill:none;stroke:gray;stroke-width:1" />
</g>
<g id="148706607" transform="rotate(-180,917,1025)" data-type="320002" >
<circle cx="917" cy="1025" r="6" style="fill:none;stroke:gray;stroke-width:1" />
<path d="M 913 1021 l 8 8 " style="fill:none;stroke:gray;stroke-width:1" />
<path d="M 921 1021 l -8 8 " style="fill:none;stroke:gray;stroke-width:1" />
</g>
<g id="7" data-type="320002" data-voltage="coral" data-voltage="coral">
<circle cx="100" cy="100" r="6" style="fill:none;stroke:coral;stroke-width:1" />
</g>
</svg>`
	d, report, err := Extract([]byte(svg), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Failed) != 0 || len(report.Skipped) != 0 || len(d.Elements) != 3 || len(d.VoltageClasses) != 0 {
		t.Fatalf("failed=%v skipped=%v elements=%d voltage classes=%v", report.Failed, report.Skipped, len(d.Elements), d.VoltageClasses)
	}
	want := map[int]Element{
		148706395: {X: 671, Y: 983, Stroke: "gray"},
		148706607: {X: 917, Y: 1025, Stroke: "gray", Orient: 180},
		7:         {X: 100, Y: 100, Stroke: "coral"},
	}
	for _, e := range d.Elements {
		w := want[e.ID]
		if e.Class != ClassLampOnPole || e.Shape != "320002" || e.X != w.X || e.Y != w.Y || e.Stroke != w.Stroke || e.Orient != w.Orient || len(e.Ports) != 0 {
			t.Errorf("lamp %d = %+v, want %+v", e.ID, e, w)
		}
	}

	lib := NewSymbolLibrary(map[string]string{"320002": `<circle cx="0" cy="0" r="6" style="fill:none;stroke:{color};stroke-width:1" />
<path d="M -4 -4 l 8 8" style="fill:none;stroke:{color};stroke-width:1" />
<path d="M 4 -4 l -8 8" style="fill:none;stroke:{color};stroke-width:1" />`})
	var buf bytes.Buffer
	if err := Render(d, lib, &buf, Static, "", nil); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		`<!-- Lamp on pole:320002 -->`,
		`<g id="148706395" data-name="" data-voltage="gray" data-type="320002">
<circle cx="671" cy="983" r="6" style="fill:none;stroke:gray;stroke-width:1" />
<path d="M 667 979 l 8 8" style="fill:none;stroke:gray;stroke-width:1" />
<path d="M 675 979 l -8 8" style="fill:none;stroke:gray;stroke-width:1" />`,
		`data-type="320002" transform="rotate(180,917,1025)">`,
	} {
		if !strings.Contains(buf.String(), s) {
			t.Errorf("render missing %q:\n%s", s, buf.String())
		}
	}
}

// TestConnectorArrow_RealCorpus reads real xsde2svg Connector arrows
// (shape 83) in both of element_83.go's forms — an axis-aligned one
// (Поопорная схема ТП ул.Энергетиков д.20 Л-3 ПС Валдай.svg, pointing left
// from the end of its overhead line Л-1, here in the current named-<g>
// line form) and a diagonal one under rotate(137) — and renders the first
// back at the same geometry.
func TestConnectorArrow_RealCorpus(t *testing.T) {
	const svg = `<?xml version="1.0"?>
<svg width="2000" height="2000" xmlns="http://www.w3.org/2000/svg">
<g id="148703890" data-type="22" data-name="Л-1" data-voltage="gray">
<polyline points="45,145 115,145" style="fill:none;stroke:gray;stroke-width:1.5" />
</g>
<g id="148703877" data-type="83"  data-voltage="coral" >
<path d="M 45 145 l -19 0" style="fill:none;stroke:coral;stroke-width:1" />
<path d=" M 26 140 l -11 5 l 11 5 z " style="fill:white;stroke:dimgray;stroke-width:1" />
</g>
<g id="148705554" data-type="83" transform="rotate(137,1356,1437)" data-voltage="coral" >
<path d="M 1356 1437 h 15.17" style="fill:none;stroke:coral;stroke-width:1" />
<path d=" M 1371.17 1442 l 11 -5 l -11 -5 z " style="fill:white;stroke:dimgray;stroke-width:1" />
</g>
</svg>`
	d, report, err := Extract([]byte(svg), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Failed) != 0 || len(report.Skipped) != 0 || len(d.Elements) != 2 || len(d.Connectors) != 1 {
		t.Fatalf("failed=%v skipped=%v elements=%d connectors=%d", report.Failed, report.Skipped, len(d.Elements), len(d.Connectors))
	}
	byID := map[int]Element{}
	for _, e := range d.Elements {
		byID[e.ID] = e
	}
	left := byID[148703877]
	if left.Class != ClassConnectorArrow || left.X != 45 || left.Y != 145 || left.Orient != 180 || left.Length != 0 ||
		left.Stroke != "coral" || left.HeadStroke != "dimgray" || left.Fill != "white" || len(left.Ports) != 1 {
		t.Errorf("left arrow = %+v", left)
	}
	if left.Ports[0].Node != d.Connectors[0].From && left.Ports[0].Node != d.Connectors[0].To {
		t.Errorf("arrow port node %d not joined to line Л-1 (%d-%d)", left.Ports[0].Node, d.Connectors[0].From, d.Connectors[0].To)
	}
	diag := byID[148705554]
	if diag.X != 1356 || diag.Y != 1437 || diag.Orient != 137 || diag.Length != 26.17 {
		t.Errorf("diagonal arrow = %+v", diag)
	}

	var buf bytes.Buffer
	if err := Render(d, NewSymbolLibrary(nil), &buf, Static, "", nil); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		`<!-- Connector arrow:83 -->`,
		`<g id="148703877" data-name="" data-voltage="coral" data-type="83" transform="rotate(180,45,145)">
<path d="M 45 145 h 19" style="fill:none;stroke:coral;stroke-width:1" />
<path d="M 64 150 l 11 -5 l -11 -5 z" style="fill:white;stroke:dimgray;stroke-width:1" />`,
		`transform="rotate(137,1356,1437)">
<path d="M 1356 1437 h 15.17"`,
	} {
		if !strings.Contains(buf.String(), s) {
			t.Errorf("render missing %q:\n%s", s, buf.String())
		}
	}
	again, report, err := Extract(buf.Bytes(), "", nil)
	if err != nil || len(report.Failed) != 0 || len(again.Elements) != 2 {
		t.Fatalf("re-extract: err=%v failed=%v n=%d", err, report.Failed, len(again.Elements))
	}
	for _, e := range again.Elements {
		w := byID[e.ID]
		if e.X != w.X || e.Y != w.Y || e.Orient != w.Orient || e.Length != w.Length {
			t.Errorf("round trip %d = %+v, want %+v", e.ID, e, w)
		}
	}
}

// TestSmallWindow_RealCorpus reads real xsde2svg Small windows (shape 319):
// two from older exports without an id (PS_110kV_Gazovaya.svg,
// PS_110kV_Demyansk.svg), which get fresh ids, and one from the patched
// element_319.go with its id; none of their colors becomes a voltage class.
// Each renders back as the same bare <rect>.
func TestSmallWindow_RealCorpus(t *testing.T) {
	const svg = `<?xml version="1.0"?>
<svg width="3000" height="1000" xmlns="http://www.w3.org/2000/svg">
<rect x="1242" y="263" width="30" height="30" style="fill:none;stroke:#00A0F0;stroke-width:1" data-type="319" data-voltage="#00A0F0" />
<rect x="2828" y="250" width="27" height="24" style="fill:none;stroke:#00A0F0;stroke-width:1" data-type="319" data-voltage="#00A0F0" />
<rect x="2164" y="276" width="30" height="30" style="fill:none;stroke:#00A0F0;stroke-width:1" id="3844" data-type="319" data-voltage="#00A0F0" />
</svg>`
	d, report, err := Extract([]byte(svg), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Failed) != 0 || len(report.Skipped) != 0 || len(d.Elements) != 3 || len(d.VoltageClasses) != 0 {
		t.Fatalf("failed=%v skipped=%v elements=%d voltage classes=%v", report.Failed, report.Skipped, len(d.Elements), d.VoltageClasses)
	}
	ids := map[int]bool{}
	for _, e := range d.Elements {
		if e.Class != ClassSmallWindow || e.Shape != "319" || e.ID == 0 || e.Stroke != "#00A0F0" || e.Fill != "none" || e.StrokeWidth != 0 || len(e.Points) != 2 {
			t.Errorf("small window = %+v", e)
		}
		ids[e.ID] = true
	}
	if len(ids) != 3 || !ids[3844] {
		t.Errorf("ids = %v, want 3 distinct including 3844", ids)
	}

	var buf bytes.Buffer
	if err := Render(d, NewSymbolLibrary(nil), &buf, Static, "", nil); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		`<!-- Small window:319 -->`,
		`<rect id="3844" x="2164" y="276" width="30" height="30" style="fill:none;stroke:#00A0F0;stroke-width:1" data-name="" data-voltage="#00A0F0" data-type="319" />`,
		`x="2828" y="250" width="27" height="24" style="fill:none;stroke:#00A0F0;stroke-width:1"`,
	} {
		if !strings.Contains(buf.String(), s) {
			t.Errorf("render missing %q:\n%s", s, buf.String())
		}
	}
}

// TestConnectorPoint_RealCorpus reads a real xsde2svg Connector (shape 10,
// ПС 35 кВ 45С Тохма.svg id 1359) with the overhead line and object link
// that end at its center: both wires and its one port share one node, and
// its magenta doesn't become a voltage class. It renders back as the same
// 10x10 square.
func TestConnectorPoint_RealCorpus(t *testing.T) {
	const svg = `<?xml version="1.0"?>
<svg width="2000" height="2000" xmlns="http://www.w3.org/2000/svg">
<polyline points="400,1280 400,1310" style="fill:none;stroke:purple;;stroke-width:3"  data-voltage="purple" data-type="22" id="1358" />
<polyline points="400,1310 400,1330" style="fill:none;stroke:purple;;stroke-width:1" data-type="28" id="1357" data-voltage="purple" />
<rect x="395" y="1305" width="10" height="10" style="fill:none;stroke:magenta;stroke-width:1"  id="1359" data-type="10" data-voltage="magenta" />
</svg>`
	d, report, err := Extract([]byte(svg), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Failed) != 0 || len(report.Skipped) != 0 || len(d.Elements) != 1 || len(d.Connectors) != 2 {
		t.Fatalf("failed=%v skipped=%v elements=%d connectors=%d", report.Failed, report.Skipped, len(d.Elements), len(d.Connectors))
	}
	for _, vc := range d.VoltageClasses {
		if strings.EqualFold(vc.Color, "magenta") {
			t.Errorf("connector color became a voltage class: %+v", d.VoltageClasses)
		}
	}
	e := d.Elements[0]
	if e.Class != ClassConnectorPoint || e.Shape != "10" || e.X != 400 || e.Y != 1310 || e.Stroke != "magenta" || len(e.Ports) != 1 {
		t.Fatalf("connector = %+v", e)
	}
	node := e.Ports[0].Node
	for _, c := range d.Connectors {
		if c.From != node && c.To != node {
			t.Errorf("connector %d (%d-%d) doesn't end on the connector's node %d", c.ID, c.From, c.To, node)
		}
	}

	lib := NewSymbolLibrary(map[string]string{"10": `<rect x="-5" y="-5" width="10" height="10" style="fill:none;stroke:{color};stroke-width:1" />`})
	var buf bytes.Buffer
	if err := Render(d, lib, &buf, Static, "", nil); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		`<!-- Connector:10 -->`,
		`<g id="1359" data-name="" data-voltage="magenta" data-type="10">
<rect x="395" y="1305" width="10" height="10" style="fill:none;stroke:magenta;stroke-width:1" />`,
	} {
		if !strings.Contains(buf.String(), s) {
			t.Errorf("render missing %q:\n%s", s, buf.String())
		}
	}
	again, report, err := Extract(buf.Bytes(), "", nil)
	if err != nil || len(report.Failed) != 0 || len(again.Elements) != 1 || again.Elements[0].X != 400 || again.Elements[0].Y != 1310 {
		t.Fatalf("re-extract: err=%v failed=%v elements=%+v", err, report.Failed, again.Elements)
	}
}

func TestParseShortCircuiterPortAtLeadEnd(t *testing.T) {
	n := parseFirst(t, `
<g id="1015" data-voltage="#00A0F0" data-type="398" data-name="КЗ-110 Т-2 ф.А" transform="rotate(90,850,545)" >
<g data-state="1" visibility="hidden" >
<path d="M 850 556 l 0 -8" style="fill:none;stroke:#00A0F0;stroke-width:1" />
</g>
<g data-state="0" visibility="visible" >
<path d="M 850 556 l 0 -8" style="fill:none;stroke:#00A0F0;stroke-width:1" />
<path d="M 849 539  v -6  l 8 3 z" style="fill:#00A0F0;stroke:#00A0F0;stroke-width:1" />
</g>
</g>`)
	el, ports, _, err := parseShortCircuiter(n)
	if err != nil {
		t.Fatal(err)
	}
	if el.X != 850 || el.Y != 545 || el.Orient != 90 {
		t.Errorf("anchor/orient = (%v,%v,%d), want (850,545,90)", el.X, el.Y, el.Orient)
	}
	// The lead's free end (850,556), rotated 90° about the anchor.
	if len(ports) != 1 || ports[0] != (Point{839, 545}) {
		t.Errorf("ports = %v, want [(839,545)]", ports)
	}
}

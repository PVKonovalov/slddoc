package slddoc

import "testing"

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

	el, ports, err := parsePowerTransformer(n)
	if err != nil {
		t.Fatal(err)
	}
	if el.X != 900 || el.Y != 500 || el.Orient != -270 {
		t.Errorf("anchor/orient = (%v,%v,%d), want (900,500,-270)", el.X, el.Y, el.Orient)
	}
	// Local leads are at (953,500) and (847,500); rotate(-270) about
	// (900,500) turns +90° clockwise, so both ports land on x=900.
	if len(ports) != 2 || ports[0] != (Point{900, 553}) || ports[1] != (Point{900, 447}) {
		t.Errorf("ports = %v, want [{900 553} {900 447}]", ports)
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

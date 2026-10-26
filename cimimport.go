package slddoc

import (
	"bytes"
	"cmp"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// CIM import: CIM RDF/XML documents (CGMES 3.0 or 2.4.15; any set of profiles, read by class and property name
// whatever their namespace) to one diagram per Diagram Layout (DL) Diagram. Equipment becomes elements placed at
// their DiagramObject points, AC line segments become lines along theirs, and every ConnectivityNode is wired: the
// wires the layout draws for it (DiagramObjects of the node itself, as ExportCIM writes them) are kept, and any part
// of the node they leave unconnected gets an orthogonal wire to the rest (to its busbar when it has one). Repair is
// then expected to turn wire ends and terminals into ports, as after any import.

// CIMImportOptions tells ImportCIM about the editor: Terminals maps a symbol shape to its local terminals and Classes
// a shape to its element class (from the element library); Grid is the grid step foreign layouts are snapped to.
type CIMImportOptions struct {
	Terminals  map[string][]Point
	Classes    map[string]Class
	Grid       float64
	CIMClasses CIMClassTable // which CIM classes a shape stands for (nil: the built-in lists)
	AutoLayout bool          // lay the equipment out automatically even when the documents hold a diagram layout
}

// CIMPage is one imported diagram: Title is the DL Diagram's name, Report counts what was made (Skipped by CIM
// class) and Messages notes what was guessed.
type CIMPage struct {
	Title    string
	Diagram  *Diagram
	Report   Report
	Messages []string
	Warnings []string // what in the file looks wrong (a nominal voltage no grid has)
	Layout   CIMLayoutKind
}

// CIMLayoutKind says who drew a page: this editor's export, another tool's diagram layout, or the automatic layout.
type CIMLayoutKind string

const (
	CIMLayoutOwn       CIMLayoutKind = "own"
	CIMLayoutForeign   CIMLayoutKind = "foreign"
	CIMLayoutGenerated CIMLayoutKind = "generated"
)

// ErrNoDiagramLayout is returned when the documents hold nothing to draw: no DL Diagram that places any equipment and
// no equipment to lay out.
var ErrNoDiagramLayout = errors.New("slddoc: no equipment to draw in the CIM documents")

type cimObject struct {
	id    string
	class string
	props map[string][]string // property name after its class prefix → values (references normalized)
}

func (o *cimObject) get(name string) string {
	if o == nil || len(o.props[name]) == 0 {
		return ""
	}
	return o.props[name][0]
}

func (o *cimObject) num(name string) (float64, bool) {
	v, err := strconv.ParseFloat(strings.TrimSpace(o.get(name)), 64)
	return v, err == nil
}

func (o *cimObject) numPtr(name string) *float64 {
	if v, ok := o.num(name); ok {
		return &v
	}
	return nil
}

// cimRef normalizes an rdf:ID, rdf:about or rdf:resource to a bare id: "#_x", "_x", "urn:uuid:x" and "x" all give
// "x".
func cimRef(s string) string {
	s = strings.TrimPrefix(s, "#")
	s = strings.TrimPrefix(s, "urn:uuid:")
	return strings.TrimPrefix(s, "_")
}

// cimGeneralClasses are abstract CIM classes a profile may describe an object by instead of its own.
var cimGeneralClasses = map[string]bool{"IdentifiedObject": true, "PowerSystemResource": true, "Equipment": true,
	"ConductingEquipment": true, "ACDCTerminal": true, "RegulatingCondEq": true, "RotatingMachine": true,
	"ShuntCompensator": true, "TapChanger": true, "OperationalLimit": true, "ConnectivityNodeContainer": true,
	"EquipmentContainer": true}

// parseCIM reads RDF/XML documents into one object store, merging objects that several profiles describe.
func parseCIM(docs [][]byte) (map[string]*cimObject, error) {
	objs := map[string]*cimObject{}
	var inverse [][3]string // object id, the property to state, the object stating it from the other end
	for _, doc := range docs {
		dec := xml.NewDecoder(bytes.NewReader(doc))
		depth := 0
		var cur *cimObject
		var prop string
		var text strings.Builder
		for {
			tok, err := dec.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("slddoc: reading CIM: %w", err)
			}
			switch tk := tok.(type) {
			case xml.StartElement:
				depth++
				switch depth {
				case 2:
					cur = nil
					if strings.Contains(tk.Name.Space, "ModelDescription") || strings.Contains(tk.Name.Space, "rdf-syntax") {
						continue
					}
					id, defines := "", false
					for _, a := range tk.Attr {
						if a.Name.Local == "ID" || a.Name.Local == "about" {
							id, defines = cimRef(a.Value), a.Name.Local == "ID"
						}
					}
					if id == "" {
						continue
					}
					o := objs[id]
					if o == nil {
						o = &cimObject{id: id, props: map[string][]string{}}
						objs[id] = o
					}
					// Another profile may describe an object by a general class (SSH's Equipment.inService of a busbar):
					// the defining rdf:ID's class wins, and a specific one over a general one, whatever the order.
					if o.class == "" || defines || cimGeneralClasses[o.class] && !cimGeneralClasses[tk.Name.Local] {
						o.class = tk.Name.Local
					}
					cur = o
				case 3:
					if cur == nil {
						continue
					}
					prop = tk.Name.Local[strings.LastIndex(tk.Name.Local, ".")+1:]
					text.Reset()
					for _, a := range tk.Attr {
						if a.Name.Local == "resource" {
							v := a.Value
							if i := strings.LastIndex(v, "#"); i >= 0 && strings.HasPrefix(v, "http") {
								v = v[i+1:] // an enum value, e.g. WindingConnection.D
							} else {
								v = cimRef(v)
								if fwd, ok := cimInverseRoles[tk.Name.Local]; ok {
									inverse = append(inverse, [3]string{v, fwd, cur.id})
								}
							}
							cur.props[prop] = append(cur.props[prop], v)
							prop = ""
						}
					}
				}
			case xml.CharData:
				if depth == 3 && cur != nil && prop != "" {
					text.Write(tk)
				}
			case xml.EndElement:
				if depth == 3 && cur != nil && prop != "" {
					cur.props[prop] = append(cur.props[prop], strings.TrimSpace(text.String()))
					prop = ""
				}
				depth--
			}
		}
	}
	// What a document states only from the other end (a container listing its equipment) reads as if stated from
	// this one, unless that is stated too.
	for _, inv := range inverse {
		if o := objs[inv[0]]; o != nil && len(o.props[inv[1]]) == 0 {
			o.props[inv[1]] = []string{inv[2]}
		}
	}
	return objs, nil
}

// cimInverseRoles maps an association written from the end the importer doesn't read (CIM's inverse role, as in
// a container's list of its equipment) to the property it reads at the other end.
var cimInverseRoles = map[string]string{
	"ConnectivityNodeContainer.ConnectivityNodes": "ConnectivityNodeContainer",
	"EquipmentContainer.Equipments":               "EquipmentContainer",
	"Substation.VoltageLevels":                    "Substation",
	"VoltageLevel.Bays":                           "VoltageLevel",
	"ConductingEquipment.Terminals":               "ConductingEquipment",
	"ConnectivityNode.Terminals":                  "ConnectivityNode",
	"TopologicalNode.Terminal":                    "TopologicalNode",
	"PowerTransformer.PowerTransformerEnd":        "PowerTransformer",
	"Terminal.TransformerEnd":                     "Terminal",
	"Diagram.DiagramElements":                     "Diagram",
	"DiagramObject.DiagramObjectPoints":           "DiagramObject",
	"IdentifiedObject.DiagramObjects":             "IdentifiedObject",
	"BaseVoltage.VoltageLevel":                    "BaseVoltage",
}

// cimEquipmentShapes maps a CIM equipment class to the element class and shape it is drawn with by default.
var cimEquipmentShapes = map[string]struct {
	class Class
	shape string
}{
	"Breaker":                     {ClassBreaker, "41"},
	"DisconnectingCircuitBreaker": {ClassBreaker, "41"},
	"Disconnector":                {ClassDisconnector, "162"},
	"LoadBreakSwitch":             {ClassLoadBreakSwitch, "42"},
	"GroundDisconnector":          {ClassGroundSwitch, "54"},
	"Fuse":                        {ClassFuse, "203"},
	"Switch":                      {ClassSectionalizer, "164"},
	"Ground":                      {ClassGround, "31"},
	"SynchronousMachine":          {ClassGenerator, "173"},
	"AsynchronousMachine":         {ClassSynchronousMotor, "39"},
	"SeriesCompensator":           {ClassReactor, "37"},
	"LinearShuntCompensator":      {ClassCapacitorBank, "172"},
	"NonlinearShuntCompensator":   {ClassCapacitorBank, "172"},
	"PetersenCoil":                {ClassReactorShunt, "397"},
	"BusbarSection":               {ClassBusBarSection, "24"},
	"PowerTransformer":            {ClassPowerTransformer, "47"},
	"CurrentTransformer":          {ClassCurrentTransformer, "34"},
	"PotentialTransformer":        {ClassVoltageTransformer, "55"},
	"SurgeArrester":               {ClassSurgeArrester, "168"},
	"FaultIndicator":              {ClassFaultPassageIndicator, "320003"},
	"WaveTrap":                    {ClassBlockingFilter, "389"},
	"Junction":                    {ClassJunctionPoint, "7"},
	"EnergyConsumer":              {ClassLoad, "330001"},
	"ConformLoad":                 {ClassLoad, "330001"},
	"NonConformLoad":              {ClassLoad, "330001"},
	"ExternalNetworkInjection":    {ClassNetworkInfeed, "330002"},
	"EquivalentInjection":         {ClassNetworkInfeed, "330002"},
	"EnergySource":                {ClassNetworkInfeed, "330002"},
	"StaticVarCompensator":        {ClassStaticVarCompensator, "330003"},
}

var cimAuxiliaryClasses = map[string]bool{
	"CurrentTransformer": true, "PotentialTransformer": true, "SurgeArrester": true, "FaultIndicator": true, "WaveTrap": true,
}

// shapeFor is the element class and shape CIM class cls is drawn with: a shape the table made default to it, else
// its own symbol, else the first shape (by code) whose classes include it.
func (im *cimImporter) shapeFor(cls string) (Class, string, bool) {
	codes := make([]string, 0, len(im.opt.CIMClasses))
	for code := range im.opt.CIMClasses {
		if _, ok := im.opt.Classes[code]; ok && code != LinkToObjectCode {
			codes = append(codes, code)
		}
	}
	slices.Sort(codes)
	for _, code := range codes {
		// Only a default the table changed; shapes keeping their class's built-in default draw it as before.
		l, c := im.opt.CIMClasses[code], im.opt.Classes[code]
		if builtin := CIMClasses(c); len(l) > 0 && l[0] == cls && (len(builtin) == 0 || builtin[0] != cls) {
			return c, code, true
		}
	}
	if def, ok := cimEquipmentShapes[cls]; ok {
		return def.class, def.shape, true
	}
	for _, code := range codes {
		if slices.Contains(im.opt.CIMClasses[code], cls) {
			return im.opt.Classes[code], code, true
		}
	}
	return "", "", false
}

// classesOf is the CIM classes an element of shape code (class c) can stand for.
func (im *cimImporter) classesOf(code string, c Class) []string {
	return im.opt.CIMClasses.elementClasses(Element{Shape: code, Class: c})
}

func (im *cimImporter) drawable(cls string) bool {
	_, _, ok := im.shapeFor(cls)
	return ok
}

// isNode reports whether o is a node equipment connects at: a ConnectivityNode, or a TopologicalNode in a
// bus-branch model.
func isNode(o *cimObject) bool {
	return o.class == "ConnectivityNode" || o.class == "TopologicalNode"
}

// cimImporter holds the parsed documents and their indexes.
type cimImporter struct {
	opt       CIMImportOptions
	ownLayout bool // the documents hold a diagram layout (set aside with AutoLayout)
	objs      map[string]*cimObject
	byClass   map[string][]*cimObject
	terminals map[string][]*cimObject // equipment id → its Terminals, by sequence number
	points    map[string][]*cimObject // DiagramObject id → its points, in order
	dos       map[string][]*cimObject // Diagram id → its DiagramObjects
	layerOf   map[string]string       // DiagramObject id → VisibilityLayer id
}

// ImportCIM converts CIM documents to diagrams, one per DL Diagram that places at least one item.
func ImportCIM(docs [][]byte, opt CIMImportOptions) ([]CIMPage, error) {
	if opt.Grid <= 0 {
		opt.Grid = 10
	}
	objs, err := parseCIM(docs)
	if err != nil {
		return nil, err
	}
	nameVoltages(objs)
	im := &cimImporter{opt: opt, objs: objs, byClass: map[string][]*cimObject{}, terminals: map[string][]*cimObject{},
		points: map[string][]*cimObject{}, dos: map[string][]*cimObject{}, layerOf: map[string]string{}}
	ids := make([]string, 0, len(objs))
	for id := range objs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		o := objs[id]
		im.byClass[o.class] = append(im.byClass[o.class], o)
		switch o.class {
		case "Terminal":
			// A bus-branch model has no ConnectivityNodes: its TopologicalNodes stand in for them.
			if o.get("ConnectivityNode") == "" && o.get("TopologicalNode") != "" {
				o.props["ConnectivityNode"] = o.props["TopologicalNode"]
			}
			eq := o.get("ConductingEquipment")
			im.terminals[eq] = append(im.terminals[eq], o)
		case "DiagramObjectPoint":
			do := o.get("DiagramObject")
			im.points[do] = append(im.points[do], o)
		case "DiagramObject", "TextDiagramObject":
			im.dos[o.get("Diagram")] = append(im.dos[o.get("Diagram")], o)
		case "VisibilityLayer":
			for _, do := range o.props["VisibleObjects"] {
				if _, ok := im.layerOf[do]; !ok {
					im.layerOf[do] = id
				}
			}
		}
	}
	seq := func(o *cimObject) float64 {
		if v, ok := o.num("sequenceNumber"); ok {
			return v
		}
		return math.MaxFloat64
	}
	for _, list := range im.terminals {
		sort.SliceStable(list, func(i, j int) bool { return seq(list[i]) < seq(list[j]) })
	}
	for _, list := range im.points {
		sort.SliceStable(list, func(i, j int) bool { return seq(list[i]) < seq(list[j]) })
	}
	diagrams := im.byClass["Diagram"]
	im.ownLayout = len(diagrams) > 0
	if len(diagrams) == 0 || opt.AutoLayout {
		diagrams = im.autoLayout()
	}
	var pages []CIMPage
	for _, dg := range diagrams {
		if p, ok := im.page(dg); ok {
			pages = append(pages, p)
		}
	}
	if len(pages) == 0 {
		return nil, ErrNoDiagramLayout
	}
	return pages, nil
}

// nameVoltages gives a VoltageLevel whose BaseVoltage is missing (often in a file not sent along) or holds no
// nominal voltage the one its name states, "ОРУ-110 кВ" or "6,3 kV" (a number with its unit; ParseVoltage).
func nameVoltages(objs map[string]*cimObject) {
	ids := slices.Sorted(maps.Keys(objs))
	for _, id := range ids {
		vl := objs[id]
		if vl.class != "VoltageLevel" {
			continue
		}
		bv := objs[vl.get("BaseVoltage")]
		if kv, ok := bv.num("nominalVoltage"); ok && kv > 0 {
			continue
		}
		kv, ok := unitVoltage(vl.get("name"))
		if !ok {
			continue
		}
		ref := vl.get("BaseVoltage")
		if ref == "" {
			ref = "_named-bv-" + strconv.FormatFloat(kv, 'f', -1, 64)
			vl.props["BaseVoltage"] = []string{ref}
		}
		if bv == nil {
			bv = &cimObject{id: ref, class: "BaseVoltage", props: map[string][]string{}}
			objs[ref] = bv
		}
		bv.props["nominalVoltage"] = []string{strconv.FormatFloat(kv, 'f', -1, 64)}
	}
}

var unitVoltageRe = regexp.MustCompile(`(?i)([0-9]+(?:[.,][0-9]+)?)\s*(kv|кв|mv|мв|v|в)`)

// unitVoltage is the voltage, in kV, a name states as a number with its unit ("110 кВ", "6,3kV", "400 V"); a number
// without one (a busbar's "1СШ") states none.
func unitVoltage(name string) (float64, bool) {
	for _, m := range unitVoltageRe.FindAllStringSubmatchIndex(name, -1) {
		if end := m[1]; end < len(name) {
			if r, _ := utf8.DecodeRuneInString(name[end:]); unicode.IsLetter(r) {
				continue
			}
		}
		return ParseVoltage(name[m[0]:m[1]])
	}
	return 0, false
}

// --- one page

// cimAttach is one thing that must reach a ConnectivityNode: a terminal or line end at a point. Attachments sharing
// a group are already connected (the ports of a junction, wire jump or current transformer); an optional one joins
// when a wire reaches it but is never wired on its own.
type cimAttach struct {
	at       Point
	optional bool
	group    int
	owner    int // the element whose port it is (0 for a line end)
}

type cimPageBuilder struct {
	im         *cimImporter
	d          *Diagram
	report     Report
	messages   []string
	warnings   []string
	nextID     int
	voltages   map[string]int         // BaseVoltage id → VoltageClass id
	layers     map[string]int         // VisibilityLayer id → Layer id
	itemOf     map[string]int         // CIM object id → element/connector id, for labels
	attach     map[string][]cimAttach // ConnectivityNode id → what must reach it
	busbars    map[string][][2]Point  // ConnectivityNode id → busbar segments on it
	wires      map[string][][]Point   // ConnectivityNode id → wires drawn for it
	cnLayer    map[string]int
	doFor      map[string]*cimObject // object id → its DiagramObject on this page
	portAt     map[int][]Point       // element id → where its ports sit, when not at its symbol's terminals
	glued      map[string]Point      // DiagramObjectGluePoint id → the point of a terminal glued there
	portCN     map[int][]string      // element id → the ConnectivityNode of each port ("" for none)
	connCN     map[int][2]string     // connector id → the ConnectivityNodes of its From and To ends
	pending    []cimPending          // junctions, jumps and auxiliary devices, joined once all else is placed
	knives     map[string]*cimKnife
	knifeKey   []string
	tf         func(Point) Point
	own        bool
	synthetic  bool             // laid out by autoLayout
	foreign    bool             // neither own nor synthetic: drawn by another tool
	termDrawn  map[string]Point // Terminal id → where a foreign layout draws it (its polyline's device end)
	termWires  []cimTermWire
	faced      map[int]bool   // element id → turned to face the terminals its layout draws
	nodeCN     map[int]string // Node id → the ConnectivityNode it stands for
	baseOrient map[int]int    // element id → its orientation in the layout, before turning to face its terminals
}

// cimTermWire is a Terminal a foreign layout draws as a polyline: wire idx of node cn, its device end first or last.
type cimTermWire struct {
	cn    string
	idx   int
	first bool
	term  string
}

// cimPending is an element whose ports all join ConnectivityNode cn as one piece.
type cimPending struct {
	id       int
	cn       string
	optional bool
}

// cimKnife collects the two Disconnectors ExportCIM writes for one changeover knife switch.
type cimKnife struct {
	id                      int
	pivot, left, right      string
	leftClosed, rightClosed bool
	at                      [3]Point // drawn pivot, left and right terminal points (zero when not drawn)
}

func (b *cimPageBuilder) take() int {
	b.nextID++
	return b.nextID
}

func (im *cimImporter) page(dg *cimObject) (CIMPage, bool) {
	dos := im.dos[dg.id]
	b := &cimPageBuilder{im: im, d: &Diagram{Layers: []Layer{{ID: BaseLayer, Name: baseLayerName}}},
		report: Report{Skipped: map[string]int{}}, voltages: map[string]int{}, layers: map[string]int{},
		itemOf: map[string]int{}, attach: map[string][]cimAttach{}, busbars: map[string][][2]Point{},
		wires: map[string][][]Point{}, cnLayer: map[string]int{}, knives: map[string]*cimKnife{},
		doFor: map[string]*cimObject{}, portAt: map[int][]Point{}, glued: map[string]Point{},
		portCN: map[int][]string{}, connCN: map[int][2]string{}, termDrawn: map[string]Point{}, faced: map[int]bool{},
		baseOrient: map[int]int{}}
	b.transform(dg, dos)
	b.foreign = !b.own && !b.synthetic
	for _, do := range dos {
		if ref := do.get("IdentifiedObject"); ref != "" && b.doFor[ref] == nil {
			b.doFor[ref] = do
		}
	}

	// A wire or line end glued to a terminal ends where the terminal is drawn.
	for _, do := range dos {
		if t := im.objs[do.get("IdentifiedObject")]; t != nil && t.class == "Terminal" {
			pts := b.doPoints(do)
			for k, p := range im.points[do.id] {
				if g := p.get("DiagramObjectGluePoint"); g != "" && k < len(pts) {
					if _, ok := b.glued[g]; !ok {
						b.glued[g] = pts[k]
					}
				}
			}
		}
	}

	// A foreign layout may draw a terminal as a polyline from its device to its node: a wire of that node, whose
	// device end goes onto the symbol's terminal once the device is placed (snapTerminalWires).
	if b.foreign {
		for _, do := range dos {
			if t := im.objs[do.get("IdentifiedObject")]; do.class == "DiagramObject" && t != nil && t.class == "Terminal" {
				b.terminalWire(t, do)
			}
		}
	}

	// Wires first: placing a device matches its terminals to their ends. A foreign layout may draw a busbar as its
	// node: a drawing of a node with a BusbarSection not drawn on its own becomes that busbar; one shorter than two
	// grid steps that no other drawing of its node ends at only marks a junction, which the wiring makes anyway.
	var junctionMark func(cn string, do *cimObject) bool
	if b.foreign {
		junctionMark = b.junctionMarks(dos)
	}
	for _, do := range dos {
		if obj := im.objs[do.get("IdentifiedObject")]; do.class == "DiagramObject" && obj != nil && isNode(obj) {
			if bb := b.undrawnBusbar(obj.id); b.foreign && bb != nil && len(b.doPoints(do)) >= 2 {
				b.element(bb, do)
				continue
			}
			if pts := b.gluedEnds(do); len(pts) >= 2 && !(b.foreign && pathLength(pts) < 2*b.im.opt.Grid && junctionMark(obj.id, do)) {
				b.wires[obj.id] = append(b.wires[obj.id], pts)
				b.cnLayer[obj.id] = b.layer(do)
			}
		}
	}
	for _, do := range dos {
		if do.class != "DiagramObject" {
			continue
		}
		obj := im.objs[do.get("IdentifiedObject")]
		if obj == nil {
			continue
		}
		if _, placed := b.itemOf[obj.id]; placed && !isNode(obj) {
			continue // a second DiagramObject for the same equipment on this page: drawn once
		}
		switch {
		case isNode(obj):
			if pts := b.doPoints(do); len(pts) == 1 {
				b.passThrough(obj, do, pts[0])
			}
		case obj.class == "ACLineSegment":
			b.line(obj, do)
		case b.isObjectLink(obj, do):
			b.objectLink(obj, do)
		case obj.class == "Terminal" || obj.class == "PowerTransformerEnd":
		default:
			if _, _, ok := im.shapeFor(obj.class); ok {
				b.element(obj, do)
			} else if _, isEq := im.terminals[obj.id]; isEq {
				b.report.Skipped[obj.class]++
			}
		}
	}
	if b.report.Elements == 0 && b.report.Connectors == 0 {
		return CIMPage{}, false
	}
	if b.foreign {
		b.dropOverdrawn()
		b.faceConnections()
		b.snapTerminalWires()
		b.undrawnLines(dos)
	}
	b.finishKnives()
	b.joinPending()
	b.wireNodes()
	b.identities()
	b.labels(dos)
	if b.foreign && !slices.ContainsFunc(dos, func(do *cimObject) bool { return do.class == "TextDiagramObject" }) {
		if n := b.nameLabels(); n > 0 {
			b.messages = append(b.messages, fmt.Sprintf("the layout writes no names: %d labels added from the equipment's names", n))
		}
	}
	b.d.LastID = b.nextID
	b.report.Nodes = len(b.d.Nodes)
	title := dg.get("name")
	if title == "" {
		title = "diagram"
	}
	kind := CIMLayoutForeign
	if b.own {
		kind = CIMLayoutOwn
	} else if b.synthetic {
		kind = CIMLayoutGenerated
	}
	return CIMPage{Title: title, Diagram: b.d, Report: b.report, Messages: b.messages, Warnings: b.warnings, Layout: kind}, true
}

// transform sets the page's coordinate mapping: y flipped for a y-up diagram; a layout this editor exported (its
// "shape-" styles) kept 1:1; any other scaled so the median distance between neighbouring devices is six grid steps,
// moved to a margin and snapped to the grid.
func (b *cimPageBuilder) transform(dg *cimObject, dos []*cimObject) {
	if dg.get("synthetic") == "true" {
		b.synthetic = true
		b.tf = func(p Point) Point { return p }
		for _, do := range dos {
			for _, p := range b.im.points[do.id] {
				x, _ := p.num("xPosition")
				y, _ := p.num("yPosition")
				b.d.Width, b.d.Height = math.Max(b.d.Width, x+10*b.im.opt.Grid), math.Max(b.d.Height, y+6*b.im.opt.Grid)
			}
		}
		if b.im.ownLayout {
			b.messages = append(b.messages, "the file's diagram layout set aside: laid out automatically from the connections")
		} else {
			b.messages = append(b.messages, "the file has no diagram layout: laid out automatically from the connections")
		}
		return
	}
	flip := strings.HasSuffix(dg.get("orientation"), "positive")
	for _, do := range dos {
		if s := b.im.objs[do.get("DiagramObjectStyle")]; s != nil && strings.HasPrefix(s.get("name"), "shape-") {
			b.own = true
			break
		}
	}
	raw := func(p *cimObject) Point {
		x, _ := p.num("xPosition")
		y, _ := p.num("yPosition")
		if flip {
			y = -y
		}
		return Point{X: x, Y: y}
	}
	var all, centres []Point
	for _, do := range dos {
		pts := b.im.points[do.id]
		for _, p := range pts {
			all = append(all, raw(p))
		}
		if len(pts) == 1 && do.class == "DiagramObject" {
			if o := b.im.objs[do.get("IdentifiedObject")]; o != nil && b.im.drawable(o.class) {
				centres = append(centres, raw(pts[0]))
			}
		}
	}
	grid := b.im.opt.Grid
	if b.own {
		b.tf = func(p Point) Point { return p }
		w, _ := dg.num("x2InitialView")
		h, _ := dg.num("y2InitialView")
		b.d.Width, b.d.Height = w, h
		for _, p := range all {
			b.d.Width, b.d.Height = math.Max(b.d.Width, p.X+2*grid), math.Max(b.d.Height, p.Y+2*grid)
		}
		return
	}
	scale := 1.0
	if nn := medianNearest(centres); nn > 0 {
		scale = 6 * grid / nn
	}
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, p := range all {
		minX, minY = math.Min(minX, p.X), math.Min(minY, p.Y)
		maxX, maxY = math.Max(maxX, p.X), math.Max(maxY, p.Y)
	}
	if len(all) == 0 {
		minX, minY, maxX, maxY = 0, 0, 0, 0
	}
	margin := 6 * grid
	snap := func(v float64) float64 { return math.Round(v/grid) * grid }
	b.tf = func(p Point) Point {
		return Point{X: snap((p.X-minX)*scale + margin), Y: snap((p.Y-minY)*scale + margin)}
	}
	b.d.Width = snap((maxX-minX)*scale + 2*margin)
	b.d.Height = snap((maxY-minY)*scale + 2*margin)
	if scale != 1 {
		b.messages = append(b.messages, fmt.Sprintf("layout scaled by %.3g and snapped to the grid", scale))
	}
}

// medianNearest is the median of each point's distance to its nearest other point (0 for fewer than two).
func medianNearest(pts []Point) float64 {
	if len(pts) < 2 {
		return 0
	}
	ds := make([]float64, 0, len(pts))
	for i, p := range pts {
		best := math.Inf(1)
		for j, q := range pts {
			if i != j {
				if d := math.Hypot(p.X-q.X, p.Y-q.Y); d > 0 && d < best {
					best = d
				}
			}
		}
		if !math.IsInf(best, 1) {
			ds = append(ds, best)
		}
	}
	if len(ds) == 0 {
		return 0
	}
	slices.Sort(ds)
	return ds[len(ds)/2]
}

func (b *cimPageBuilder) doPoints(do *cimObject) []Point {
	var out []Point
	for _, p := range b.im.points[do.id] {
		x, _ := p.num("xPosition")
		y, _ := p.num("yPosition")
		if dg := b.im.objs[do.get("Diagram")]; dg != nil && strings.HasSuffix(dg.get("orientation"), "positive") {
			y = -y
		}
		q := b.tf(Point{X: x, Y: y})
		if len(out) == 0 || out[len(out)-1] != q {
			out = append(out, q)
		}
	}
	return out
}

// gluedEnds is do's points with each end moved onto the terminal it is glued to, if any.
func (b *cimPageBuilder) gluedEnds(do *cimObject) []Point {
	raw := b.im.points[do.id]
	pts := b.doPoints(do)
	if len(raw) < 2 || len(pts) < 2 {
		return pts
	}
	if g, ok := b.glued[raw[0].get("DiagramObjectGluePoint")]; ok {
		pts[0] = g
	}
	if g, ok := b.glued[raw[len(raw)-1].get("DiagramObjectGluePoint")]; ok {
		pts[len(pts)-1] = g
	}
	return pts
}

func (b *cimPageBuilder) layer(do *cimObject) int {
	vl := b.im.layerOf[do.id]
	if vl == "" {
		return BaseLayer
	}
	if id, ok := b.layers[vl]; ok {
		return id
	}
	o := b.im.objs[vl]
	id := len(b.d.Layers)
	for slices.ContainsFunc(b.d.Layers, func(l Layer) bool { return l.ID == id }) {
		id++
	}
	z, _ := o.num("drawingOrder")
	name := o.get("name")
	if name == "" {
		name = fmt.Sprintf("Layer %d", id)
	}
	b.d.Layers = append(b.d.Layers, Layer{ID: id, Name: name, Z: int(z)})
	b.layers[vl] = id
	return id
}

// cimMaxNominalKV is the highest nominal voltage, in kV, taken without a warning; no AC grid runs above about 1200 kV.
const cimMaxNominalKV = 2000

// voltage returns the VoltageClass of BaseVoltage bv (0 for none), creating it, named "<kV> kV", on first use.
func (b *cimPageBuilder) voltage(bv string) int {
	o := b.im.objs[bv]
	if o == nil {
		return 0
	}
	if id, ok := b.voltages[bv]; ok {
		return id
	}
	kv, _ := o.num("nominalVoltage")
	name := strconv.FormatFloat(kv, 'f', -1, 64) + " kV"
	if kv > cimMaxNominalKV {
		b.warnings = append(b.warnings, fmt.Sprintf("base voltage %q has a nominal voltage of %s kV, more than any grid "+
			"has: CIM's nominalVoltage is in kV, and the file may have written volts", cmp.Or(o.get("name"), bv), strconv.FormatFloat(kv, 'f', -1, 64)))
	}
	for _, vc := range b.d.VoltageClasses {
		if vc.Name == name {
			b.voltages[bv] = vc.ID
			return vc.ID
		}
	}
	id := b.take()
	b.d.VoltageClasses = append(b.d.VoltageClasses, VoltageClass{ID: id, Name: name})
	b.voltages[bv] = id
	return id
}

// equipmentVoltage is obj's BaseVoltage: its own, else its container's (a VoltageLevel), else that of the
// container of its first terminal's ConnectivityNode.
func (b *cimPageBuilder) equipmentVoltage(obj *cimObject) int {
	if v := obj.get("BaseVoltage"); v != "" {
		return b.voltage(v)
	}
	if c := b.im.objs[obj.get("EquipmentContainer")]; c != nil && b.im.containerBV(c) != "" {
		return b.voltage(b.im.containerBV(c))
	}
	for _, t := range b.im.terminals[obj.id] {
		if cn := b.im.objs[t.get("ConnectivityNode")]; cn != nil {
			if c := b.im.objs[cn.get("ConnectivityNodeContainer")]; c != nil && b.im.containerBV(c) != "" {
				return b.voltage(b.im.containerBV(c))
			}
		}
	}
	return 0
}

func snapOrient(rotation float64) int {
	o := int(math.Round(rotation/90)) * 90 % 360
	if o < 0 {
		o += 360
	}
	if o == 270 {
		return -90
	}
	return o
}

// element places one equipment item and records what its terminals must reach.
func (b *cimPageBuilder) element(obj, do *cimObject) {
	class, shape, _ := b.im.shapeFor(obj.class)
	switch obj.class {
	case "SynchronousMachine":
		switch obj.get("type") {
		case "SynchronousMachineKind.condenser":
			class, shape = ClassSynchronousCompensator, "174"
		case "SynchronousMachineKind.motor", "SynchronousMachineKind.motorOrCondenser":
			class, shape = ClassSynchronousMotor, "39"
		}
	case "AsynchronousMachine":
		if strings.HasSuffix(obj.get("asynchronousMachineType"), "generator") {
			class, shape = ClassGenerator, "173"
		}
	case "LinearShuntCompensator", "NonlinearShuntCompensator":
		if v, ok := obj.num("bPerSection"); ok && v < 0 {
			class, shape = ClassReactorShunt, "397"
		}
	}
	// A layout this editor exported names its shape; keep it when that shape can stand for this CIM class.
	if st := b.im.objs[do.get("DiagramObjectStyle")]; st != nil {
		if code, ok := strings.CutPrefix(st.get("name"), "shape-"); ok {
			if c, ok := b.im.opt.Classes[code]; ok && slices.Contains(b.im.classesOf(code, c), obj.class) {
				class, shape = c, code
			}
		}
	}
	if class == ClassKnifeSwitch || class == ClassKnifeSwitch3 {
		b.knifeHalf(obj, do, class, shape)
		return
	}
	pts := b.doPoints(do)
	if len(pts) == 0 {
		return
	}
	e := Element{ID: b.take(), Class: class, Shape: shape, Name: obj.get("name"), Layer: b.layer(do),
		X: pts[0].X, Y: pts[0].Y, Voltage: b.equipmentVoltage(obj)}
	if r, ok := do.num("rotation"); ok {
		e.Orient = snapOrient(r)
	}
	if l := b.im.classesOf(shape, class); len(l) > 1 && l[0] != obj.class && slices.Contains(l, obj.class) {
		e.CIM = obj.class
	}
	if no := obj.get("normalOpen"); no != "" {
		state := 1
		if no == "true" {
			state = 0
		}
		e.State = &state
	}
	b.electrical(&e, obj)
	if class == ClassBooster {
		b.boosterElectrical(&e, obj)
	}
	var cns []string
	switch {
	case class == ClassBusBarSection:
		e.Orient = 0
		if len(pts) < 2 {
			pts = []Point{{pts[0].X - 5*b.im.opt.Grid, pts[0].Y}, {pts[0].X + 5*b.im.opt.Grid, pts[0].Y}}
		}
		e.Points = pts
		e.X, e.Y = (pts[0].X+pts[len(pts)-1].X)/2, (pts[0].Y+pts[len(pts)-1].Y)/2
		if ts := b.im.terminals[obj.id]; len(ts) > 0 {
			cn := ts[0].get("ConnectivityNode")
			cns = []string{cn}
			for i := 1; i < len(pts); i++ {
				b.busbars[cn] = append(b.busbars[cn], [2]Point{pts[i-1], pts[i]})
			}
		}
	case class == ClassPowerTransformer:
		cns = b.windings(&e, obj)
	case cimAuxiliaryClasses[obj.class]:
		if t := b.im.objs[obj.get("Terminal")]; t != nil {
			cns = []string{t.get("ConnectivityNode")}
		}
	default:
		for _, t := range b.im.terminals[obj.id] {
			cns = append(cns, t.get("ConnectivityNode"))
		}
	}
	var drawn []Point
	if b.own && !cimAuxiliaryClasses[obj.class] {
		drawn = b.terminalPoints(obj, class)
		b.fit(&e, drawn, true, false)
	} else if b.foreign && !cimAuxiliaryClasses[obj.class] && class != ClassBusBarSection {
		b.baseOrient[e.ID] = e.Orient
		b.faced[e.ID] = b.face(&e, b.terminalPoints(obj, class))
	}
	ts := symbolTerminals(&e, b.im.opt.Terminals)
	// This editor's own export draws each terminal where its port Node was, which need not be the symbol's
	// terminal (two ports may share a Node); the port goes back there.
	for i := range ts {
		if i < len(drawn) && drawn[i] != (Point{}) && drawn[i] != roundPt(ts[i]) {
			ts[i] = drawn[i]
			b.portAt[e.ID] = ts
		}
	}
	b.d.Elements = append(b.d.Elements, e)
	b.itemOf[obj.id] = e.ID
	b.report.Elements++
	if cimAuxiliaryClasses[obj.class] {
		if len(cns) > 0 && cns[0] != "" {
			b.pending = append(b.pending, cimPending{id: e.ID, cn: cns[0]})
		}
		return
	}
	pc := make([]string, len(ts))
	for i, p := range ts {
		if i < len(cns) && cns[i] != "" {
			pc[i] = cns[i]
			b.attach[cns[i]] = append(b.attach[cns[i]], cimAttach{at: roundPt(p), owner: e.ID})
		}
	}
	b.portCN[e.ID] = pc
	if class == ClassBusBarSection && len(cns) > 0 {
		b.portCN[e.ID] = []string{cns[0]}
	}
}

// terminalPoints is where the layout draws each of obj's terminals (by port order; a transformer's by end number),
// a zero point for one it doesn't draw; in a foreign layout only by a polyline kept as a wire (termDrawn).
func (b *cimPageBuilder) terminalPoints(obj *cimObject, class Class) []Point {
	terms := b.portTerminals(obj, class)
	out := make([]Point, len(terms))
	for i, t := range terms {
		if t == nil {
			continue
		}
		if p, ok := b.termDrawn[t.id]; ok {
			out[i] = p
		} else if do := b.doFor[t.id]; do != nil && !b.foreign {
			if pts := b.doPoints(do); len(pts) > 0 {
				out[i] = pts[0]
			}
		}
	}
	return out
}

// portTerminals is obj's Terminals in port order: a transformer's by end number (nil for an end without one).
func (b *cimPageBuilder) portTerminals(obj *cimObject, class Class) []*cimObject {
	if class != ClassPowerTransformer {
		return b.im.terminals[obj.id]
	}
	var terms []*cimObject
	for _, end := range b.im.transformerEnds(obj) {
		terms = append(terms, b.im.objs[end.get("Terminal")])
	}
	return terms
}

// wireEndsNear is every end of cn's drawn wires within three grid steps of at.
func (b *cimPageBuilder) wireEndsNear(cn string, at Point) []Point {
	var out []Point
	r := 3 * b.im.opt.Grid
	for _, w := range b.wires[cn] {
		if len(w) < 2 {
			continue // a drawn wire left to the routing
		}
		for _, p := range []Point{w[0], w[len(w)-1]} {
			if math.Hypot(p.X-at.X, p.Y-at.Y) <= r {
				out = append(out, p)
			}
		}
	}
	return out
}

// fit sets e's mirror and lead spacing so its terminals land on targets: target i on terminal i when ordered (a
// zero target is ignored), else each target on its nearest terminal. Nothing changes unless some setting fits
// within a unit. The size step stays 0 for a device: a drawn terminal point that matches another step is usually a
// port Node lying off its terminal, and the port goes there anyway. With steps (a wire jump or junction), a size step
// that puts every terminal on a target comes first: such an element is sized to the gap its wire leaves (a jump over
// a busbar is a step larger).
func (b *cimPageBuilder) fit(e *Element, targets []Point, ordered, steps bool) {
	if len(targets) == 0 {
		return
	}
	// worst is how far the farthest target lies from its terminal; byTerminal, the farthest terminal from a target.
	worst := func(probe *Element, byTerminal bool) float64 {
		ts := symbolTerminals(probe, b.im.opt.Terminals)
		if len(ts) == 0 {
			return math.Inf(1)
		}
		nearest := func(p Point, among []Point) float64 {
			best := math.Inf(1)
			for _, q := range among {
				best = math.Min(best, math.Hypot(q.X-p.X, q.Y-p.Y))
			}
			return best
		}
		w := 0.0
		if byTerminal {
			for _, t := range ts {
				w = math.Max(w, nearest(t, targets))
			}
			return w
		}
		for i, p := range targets {
			if ordered {
				if p == (Point{}) || i >= len(ts) {
					continue
				}
				w = math.Max(w, math.Hypot(ts[i].X-p.X, ts[i].Y-p.Y))
				continue
			}
			w = math.Max(w, nearest(p, ts))
		}
		return w
	}
	spans := []float64{0}
	if HasLeads(e.Shape) && len(targets) >= 2 && targets[0] != (Point{}) && targets[1] != (Point{}) {
		spans = append(spans, math.Round(math.Hypot(targets[0].X-targets[1].X, targets[0].Y-targets[1].Y)))
	}
	search := func(scales []int, byTerminal bool) (Element, float64) {
		best, bestErr := *e, worst(e, byTerminal)
		for _, scale := range scales {
			for _, mirror := range []bool{false, true} {
				for _, span := range spans {
					probe := *e
					probe.Scale, probe.Mirror, probe.Span = scale, mirror, span
					if w := worst(&probe, byTerminal); w < bestErr-1e-9 {
						best, bestErr = probe, w
					}
				}
			}
		}
		return best, bestErr
	}
	if steps && UsesSizeStep(e.Class) {
		scales := []int{0}
		for s := minSizeStep; s <= maxSizeStep; s++ {
			if s != 0 {
				scales = append(scales, s)
			}
		}
		if best, err := search(scales, true); err <= 1 {
			*e = best
			return
		}
	}
	if best, err := search([]int{0}, false); err <= 1 {
		*e = best
	}
}

// passThrough recreates a junction, wire jump, pole or cable joint this editor exported as a point of its
// ConnectivityNode (its "shape-" style names it); its ports join the node as one connected piece.
func (b *cimPageBuilder) passThrough(cn, do *cimObject, at Point) {
	st := b.im.objs[do.get("DiagramObjectStyle")]
	if st == nil {
		return
	}
	code, ok := strings.CutPrefix(st.get("name"), "shape-")
	class, known := b.im.opt.Classes[code]
	if !ok || !known || cimClasses[class].role != rolePassThrough {
		return
	}
	e := Element{ID: b.take(), Class: class, Shape: code, Layer: b.layer(do), X: at.X, Y: at.Y}
	if r, ok := do.num("rotation"); ok {
		e.Orient = snapOrient(r)
	}
	if c := b.im.objs[cn.get("ConnectivityNodeContainer")]; c != nil {
		e.Voltage = b.voltage(b.im.containerBV(c))
	}
	b.d.Elements = append(b.d.Elements, e)
	b.report.Elements++
	b.pending = append(b.pending, cimPending{id: e.ID, cn: cn.id, optional: true})
}

// joinPending fits each junction, jump and auxiliary device to the wire and line ends around it (its ports moved
// onto them, each within three grid steps, when its symbol alone doesn't fit) and joins its ports to its node.
func (b *cimPageBuilder) joinPending() {
	for _, pd := range b.pending {
		i := slices.IndexFunc(b.d.Elements, func(e Element) bool { return e.ID == pd.id })
		e := &b.d.Elements[i]
		at := Point{X: e.X, Y: e.Y}
		near := b.wireEndsNear(pd.cn, at)
		for _, a := range b.attach[pd.cn] {
			if a.group == 0 && math.Hypot(a.at.X-at.X, a.at.Y-at.Y) <= 3*b.im.opt.Grid {
				near = append(near, a.at)
			}
		}
		b.fit(e, near, false, pd.optional)
		ts := symbolTerminals(e, b.im.opt.Terminals)
		used := map[int]bool{}
		for k, t := range ts {
			best, bd := -1, 3*b.im.opt.Grid
			for j, p := range near {
				if d := math.Hypot(p.X-t.X, p.Y-t.Y); !used[j] && d <= bd {
					best, bd = j, d
				}
			}
			if best >= 0 && bd > 1e-6 {
				used[best] = true
				ts[k] = near[best]
				b.portAt[e.ID] = ts
			} else if best >= 0 {
				used[best] = true
			}
		}
		pc := make([]string, len(ts))
		for i, p := range ts {
			pc[i] = pd.cn
			b.attach[pd.cn] = append(b.attach[pd.cn], cimAttach{at: roundPt(p), optional: pd.optional, group: e.ID})
		}
		b.portCN[e.ID] = pc
	}
}

// transformerEnds is transformer tr's PowerTransformerEnds by end number.
func (im *cimImporter) transformerEnds(tr *cimObject) []*cimObject {
	var ends []*cimObject
	for _, end := range im.byClass["PowerTransformerEnd"] {
		if end.get("PowerTransformer") == tr.id {
			ends = append(ends, end)
		}
	}
	sort.SliceStable(ends, func(i, j int) bool {
		a, _ := ends[i].num("endNumber")
		c, _ := ends[j].num("endNumber")
		return a < c
	})
	return ends
}

// knifeHalf takes one of the two Disconnectors ("<name> L"/"<name> R", pivot then contact) of a knife switch
// this editor exported, creating the switch on the first.
func (b *cimPageBuilder) knifeHalf(obj, do *cimObject, class Class, shape string) {
	pts := b.doPoints(do)
	if len(pts) == 0 {
		return
	}
	name, side := obj.get("name"), "L"
	if n, ok := strings.CutSuffix(name, " L"); ok {
		name = n
	} else if n, ok := strings.CutSuffix(name, " R"); ok {
		name, side = n, "R"
	}
	key := fmt.Sprintf("%s|%g|%g", name, pts[0].X, pts[0].Y)
	k := b.knives[key]
	if k == nil {
		e := Element{ID: b.take(), Class: class, Shape: shape, Name: name, Layer: b.layer(do), X: pts[0].X, Y: pts[0].Y,
			Voltage: b.equipmentVoltage(obj)}
		if r, ok := do.num("rotation"); ok {
			e.Orient = snapOrient(r)
		}
		if v := obj.numPtr("ratedCurrent"); v != nil {
			e.Electrical = &Electrical{RatedCurrent: v}
		}
		b.d.Elements = append(b.d.Elements, e)
		b.report.Elements++
		k = &cimKnife{id: e.ID}
		b.knives[key] = k
		b.knifeKey = append(b.knifeKey, key)
	}
	b.itemOf[obj.id] = k.id
	ts := b.im.terminals[obj.id]
	if len(ts) < 2 {
		return
	}
	k.pivot = ts[0].get("ConnectivityNode")
	closed := obj.get("normalOpen") == "false"
	contact := 1
	if side == "R" {
		k.right, k.rightClosed, contact = ts[1].get("ConnectivityNode"), closed, 2
	} else {
		k.left, k.leftClosed = ts[1].get("ConnectivityNode"), closed
	}
	if drawn := b.terminalPoints(obj, ClassDisconnector); len(drawn) >= 2 {
		k.at[0], k.at[contact] = drawn[0], drawn[1]
	}
}

// finishKnives gives each reassembled knife switch its state (the closed side) and its three connections.
func (b *cimPageBuilder) finishKnives() {
	for _, key := range b.knifeKey {
		k := b.knives[key]
		i := slices.IndexFunc(b.d.Elements, func(e Element) bool { return e.ID == k.id })
		e := &b.d.Elements[i]
		state := -1
		switch {
		case k.rightClosed:
			state = 2
		case k.leftClosed:
			state = 0
		case e.Class == ClassKnifeSwitch3:
			state = 1
		}
		if state >= 0 {
			e.State = &state
		}
		ts := symbolTerminals(e, b.im.opt.Terminals)
		for j := range ts {
			if j < 3 && k.at[j] != (Point{}) && k.at[j] != roundPt(ts[j]) {
				ts[j] = k.at[j]
				b.portAt[e.ID] = ts
			}
		}
		pc := make([]string, len(ts))
		for j, p := range ts {
			if cn := [3]string{k.pivot, k.left, k.right}[min(j, 2)]; cn != "" {
				pc[j] = cn
				b.attach[cn] = append(b.attach[cn], cimAttach{at: roundPt(p)})
			}
		}
		b.portCN[e.ID] = pc
	}
}

// boosterElectrical takes a booster's rated values and impedance from its first transformer end.
func (b *cimPageBuilder) boosterElectrical(e *Element, obj *cimObject) {
	for _, end := range b.im.byClass["PowerTransformerEnd"] {
		if end.get("PowerTransformer") == obj.id && end.get("endNumber") == "1" {
			el := Electrical{RatedS: end.numPtr("ratedS"), RatedU: end.numPtr("ratedU"), R: end.numPtr("r"), X: end.numPtr("x")}
			if el != (Electrical{}) {
				e.Electrical = &el
			}
		}
	}
}

// windings gives a transformer a winding per PowerTransformerEnd (2 to 4, by end number) and returns each one's
// ConnectivityNode.
func (b *cimPageBuilder) windings(e *Element, obj *cimObject) []string {
	ends := b.im.transformerEnds(obj)
	if len(ends) > 4 {
		b.messages = append(b.messages, fmt.Sprintf("transformer %q: only its first 4 of %d windings are drawn", e.Name, len(ends)))
		ends = ends[:4]
	}
	schemes := map[string]WindingScheme{"WindingConnection.Y": SchemeWye, "WindingConnection.Yn": SchemeWyeN, "WindingConnection.D": SchemeDelta}
	var cns []string
	for _, end := range ends {
		bv := end.get("BaseVoltage")
		if bv == "" {
			// None of its own: its terminal's voltage level's.
			if t := b.im.objs[end.get("Terminal")]; t != nil {
				if cn := b.im.objs[t.get("ConnectivityNode")]; cn != nil {
					if c := b.im.objs[cn.get("ConnectivityNodeContainer")]; c != nil {
						bv = b.im.containerBV(c)
					}
				}
			}
		}
		w := TransformerWinding{Voltage: b.voltage(bv), Scheme: schemes[end.get("connectionKind")],
			RatedU: end.numPtr("ratedU"), RatedS: end.numPtr("ratedS"), R: end.numPtr("r"), X: end.numPtr("x"),
			G: end.numPtr("g"), B: end.numPtr("b")}
		if c, ok := end.num("phaseAngleClock"); ok {
			clock := int(c)
			w.PhaseAngleClock = &clock
		}
		e.Windings = append(e.Windings, w)
		cn := ""
		if t := b.im.objs[end.get("Terminal")]; t != nil {
			cn = t.get("ConnectivityNode")
		}
		cns = append(cns, cn)
	}
	for len(e.Windings) < 2 {
		e.Windings = append(e.Windings, TransformerWinding{Scheme: SchemeWye})
	}
	return cns
}

// electrical copies obj's nameplate attributes onto e.
func (b *cimPageBuilder) electrical(e *Element, obj *cimObject) {
	var el Electrical
	set := false
	pick := func(dst **float64, names ...string) {
		for _, n := range names {
			if v := obj.numPtr(n); v != nil {
				*dst, set = v, true
				return
			}
		}
	}
	pickInt := func(dst **int, name string) {
		if v, ok := obj.num(name); ok {
			n := int(v)
			*dst, set = &n, true
		}
	}
	switch obj.class {
	case "Breaker", "DisconnectingCircuitBreaker", "Disconnector", "LoadBreakSwitch", "GroundDisconnector", "Fuse", "Switch":
		pick(&el.RatedCurrent, "ratedCurrent")
	case "BusbarSection":
		pick(&el.IpMax, "ipMax")
	case "SeriesCompensator":
		pick(&el.R, "r")
		pick(&el.X, "x")
	case "SynchronousMachine", "AsynchronousMachine":
		pick(&el.RatedS, "ratedS")
		pick(&el.RatedU, "ratedU")
		pick(&el.MinQ, "minQ")
		pick(&el.MaxQ, "maxQ")
		if gu := b.im.objs[obj.get("GeneratingUnit")]; gu != nil {
			if v := gu.numPtr("minOperatingP"); v != nil {
				el.MinP, set = v, true
			}
			if v := gu.numPtr("maxOperatingP"); v != nil {
				el.MaxP, set = v, true
			}
		}
	case "LinearShuntCompensator", "NonlinearShuntCompensator", "PetersenCoil":
		pick(&el.NomU, "nomU", "nominalU")
		pick(&el.BPerSection, "bPerSection")
		pick(&el.GPerSection, "gPerSection")
		pickInt(&el.MaximumSections, "maximumSections")
		pickInt(&el.NormalSections, "normalSections")
	case "EnergyConsumer", "ConformLoad", "NonConformLoad":
		pick(&el.P, "pfixed")
		pick(&el.Q, "qfixed")
	case "ExternalNetworkInjection", "EquivalentInjection":
		pick(&el.MinP, "minP")
		pick(&el.MaxP, "maxP")
		pick(&el.MinQ, "minQ")
		pick(&el.MaxQ, "maxQ")
	case "StaticVarCompensator":
		pick(&el.CapacitiveRating, "capacitiveRating")
		pick(&el.InductiveRating, "inductiveRating")
		pick(&el.Slope, "slope")
	}
	if set {
		e.Electrical = &el
	}
}

// line adds an AC line segment along its layout points; a "CableLine" style makes it a cable line.
func (b *cimPageBuilder) line(obj, do *cimObject) {
	pts := b.gluedEnds(do)
	if len(pts) < 2 {
		return
	}
	kind := KindOverheadLine
	if st := b.im.objs[do.get("DiagramObjectStyle")]; st != nil && st.get("name") == string(KindCableLine) {
		kind = KindCableLine
	}
	c := b.lineConnector(obj, pts, b.layer(do), kind)
	ts := b.im.terminals[obj.id]
	ends := [2]Point{pts[0], pts[len(pts)-1]}
	if len(ts) >= 2 {
		// Terminal 1 takes the end nearer what else is on its node (an auto-layout writes them in order).
		near := func(cn string, p Point) float64 {
			best := math.Inf(1)
			for _, a := range b.attach[cn] {
				best = math.Min(best, math.Hypot(a.at.X-p.X, a.at.Y-p.Y))
			}
			return best
		}
		cn1, cn2 := ts[0].get("ConnectivityNode"), ts[1].get("ConnectivityNode")
		if !b.synthetic && near(cn1, ends[1])+near(cn2, ends[0]) < near(cn1, ends[0])+near(cn2, ends[1]) {
			ends[0], ends[1] = ends[1], ends[0]
		}
		b.attach[cn1] = append(b.attach[cn1], cimAttach{at: ends[0]})
		b.attach[cn2] = append(b.attach[cn2], cimAttach{at: ends[1]})
		if ends[0] == pts[0] {
			b.connCN[c.ID] = [2]string{cn1, cn2}
		} else {
			b.connCN[c.ID] = [2]string{cn2, cn1}
		}
	}
	b.d.Connectors = append(b.d.Connectors, c)
	b.itemOf[obj.id] = c.ID
	b.report.Connectors++
}

// lineConnector is AC line segment obj as a line of kind along pts, with its name, voltage and parameters.
func (b *cimPageBuilder) lineConnector(obj *cimObject, pts []Point, layer int, kind ConnectorKind) Connector {
	c := Connector{ID: b.take(), Kind: kind, Name: obj.get("name"), Voltage: b.equipmentVoltage(obj), Layer: layer, Points: pts}
	el := Electrical{Length: obj.numPtr("length"), R: obj.numPtr("r"), X: obj.numPtr("x"), Bch: obj.numPtr("bch"),
		Gch: obj.numPtr("gch"), R0: obj.numPtr("r0"), X0: obj.numPtr("x0"), B0ch: obj.numPtr("b0ch"), G0ch: obj.numPtr("g0ch")}
	if el != (Electrical{}) {
		c.Electrical = &el
	}
	return c
}

// isObjectLink reports whether obj is a link to object this editor exported: a consumer or source drawn as a
// "LinkToObject" polyline.
func (b *cimPageBuilder) isObjectLink(obj, do *cimObject) bool {
	st := b.im.objs[do.get("DiagramObjectStyle")]
	return st != nil && st.get("name") == string(KindLinkToObject) && slices.Contains(b.im.opt.CIMClasses.linkClasses(), obj.class) &&
		len(b.im.points[do.id]) >= 2
}

// objectLink adds a link to object along its points, its first point on its terminal's node.
func (b *cimPageBuilder) objectLink(obj, do *cimObject) {
	pts := b.gluedEnds(do)
	if len(pts) < 2 {
		return
	}
	c := Connector{ID: b.take(), Kind: KindLinkToObject, Name: obj.get("name"), Voltage: b.equipmentVoltage(obj),
		Layer: b.layer(do), Points: pts}
	if obj.class != b.im.opt.CIMClasses.linkClasses()[0] {
		c.CIM = obj.class
	}
	if el := (Electrical{P: obj.numPtr("pfixed"), Q: obj.numPtr("qfixed")}); el != (Electrical{}) {
		c.Electrical = &el
	}
	if ts := b.im.terminals[obj.id]; len(ts) > 0 && ts[0].get("ConnectivityNode") != "" {
		cn := ts[0].get("ConnectivityNode")
		b.attach[cn] = append(b.attach[cn], cimAttach{at: pts[0]})
		b.connCN[c.ID] = [2]string{cn, ""}
	}
	b.d.Connectors = append(b.d.Connectors, c)
	b.itemOf[obj.id] = c.ID
	b.report.Connectors++
}

// --- wiring

// wireNodes makes the wires of every ConnectivityNode: those the layout draws, then an orthogonal wire from each
// part still apart to the nearest point of the main part (the busbar's part when there is one).
func (b *cimPageBuilder) wireNodes() {
	cns := make([]string, 0, len(b.attach))
	for cn := range b.attach {
		cns = append(cns, cn)
	}
	sort.Strings(cns)
	routed := 0
	for _, cn := range cns {
		routed += b.wireNode(cn)
	}
	if routed > 0 && !b.synthetic {
		b.messages = append(b.messages, fmt.Sprintf("%d connections were not drawn in the layout and were routed automatically", routed))
	}
	if !b.own {
		b.splitAtJunctions()
		b.addJumps()
	}
	// The connections are the model's, never the drawing's: ends and terminals share a Node only when they are on
	// the same ConnectivityNode and at the same point.
	type key struct {
		cn string
		p  Point
	}
	nodes := map[key]int{}
	nodeCN := map[int]string{}
	b.nodeCN = nodeCN
	node := func(cn string, p Point) int {
		p = roundPt(p)
		if id, ok := nodes[key{cn, p}]; ok && cn != "" {
			return id
		}
		id := b.take()
		nodes[key{cn, p}] = id
		nodeCN[id] = cn
		b.d.Nodes = append(b.d.Nodes, Node{ID: id, X: p.X, Y: p.Y})
		return id
	}
	for i := range b.d.Connectors {
		c := &b.d.Connectors[i]
		ends := b.connCN[c.ID]
		c.From, c.To = node(ends[0], c.Points[0]), node(ends[1], c.Points[len(c.Points)-1])
	}
	for i := range b.d.Elements {
		e := &b.d.Elements[i]
		if e.Class == ClassBusBarSection {
			continue
		}
		ts := b.portAt[e.ID]
		if ts == nil {
			ts = symbolTerminals(e, b.im.opt.Terminals)
		}
		for k, p := range ts {
			cn := ""
			if k < len(b.portCN[e.ID]) {
				cn = b.portCN[e.ID][k]
			}
			e.Ports = append(e.Ports, Port{Name: strconv.Itoa(k + 1), Node: node(cn, p)})
		}
	}
	// Where a busbar's end lies on another busbar or a wire of its node, the two meet at a Node there.
	for _, e := range b.d.Elements {
		if e.Class != ClassBusBarSection || len(b.portCN[e.ID]) == 0 || len(e.Points) < 2 {
			continue
		}
		cn := b.portCN[e.ID][0]
		for _, p := range []Point{e.Points[0], e.Points[len(e.Points)-1]} {
			if b.touchesOwn(e.ID, cn, p) {
				node(cn, p)
			}
		}
	}
	b.tapWires(nodeCN)
	// A busbar takes as its ports the Nodes of its own ConnectivityNode lying on its line.
	for i := range b.d.Elements {
		e := &b.d.Elements[i]
		if e.Class != ClassBusBarSection || len(b.portCN[e.ID]) == 0 {
			continue
		}
		cn := b.portCN[e.ID][0]
		for _, n := range b.d.Nodes {
			if nodeCN[n.ID] != cn {
				continue
			}
			for k := 1; k < len(e.Points); k++ {
				if onSeg(Point{X: n.X, Y: n.Y}, e.Points[k-1], e.Points[k]) {
					e.Ports = append(e.Ports, Port{Name: strconv.Itoa(len(e.Ports) + 1), Node: n.ID})
					break
				}
			}
		}
	}
	b.junctionPoints()
}

// junctionPoints puts a junction point (shape 7) on every Node where a buswork wire meets a busbar's port, unless
// one is there already, with the busbar's voltage class and layer: the rule of manual drawing
// (diagramOps.addJunctionPoints).
func (b *cimPageBuilder) junctionPoints() {
	shape := ""
	for code, c := range b.im.opt.Classes {
		if c == ClassJunctionPoint && (shape == "" || code == "7" || shape != "7" && code < shape) {
			shape = code
		}
	}
	if shape == "" {
		return
	}
	busOf := map[int]*Element{}
	dotted := map[int]bool{}
	dotAt := map[Point]bool{}
	for i := range b.d.Elements {
		e := &b.d.Elements[i]
		for _, p := range e.Ports {
			switch e.Class {
			case ClassBusBarSection:
				if busOf[p.Node] == nil {
					busOf[p.Node] = e
				}
			case ClassJunctionPoint:
				dotted[p.Node] = true
			}
		}
		if e.Class == ClassJunctionPoint {
			dotAt[roundPt(Point{X: e.X, Y: e.Y})] = true
		}
	}
	wired := map[int]bool{}
	for _, c := range b.d.Connectors {
		if c.Kind == KindBusWork {
			wired[c.From], wired[c.To] = true, true
		}
	}
	for _, n := range b.d.Nodes {
		bus := busOf[n.ID]
		if bus == nil || !wired[n.ID] || dotted[n.ID] || dotAt[roundPt(Point{X: n.X, Y: n.Y})] {
			continue
		}
		b.d.Elements = append(b.d.Elements, Element{ID: b.take(), Class: ClassJunctionPoint, Shape: shape, Layer: bus.Layer,
			Voltage: bus.Voltage, X: n.X, Y: n.Y, Ports: []Port{{Name: "1", Node: n.ID}}})
		b.report.Elements++
	}
}

// touchesOwn reports whether p lies on a busbar (other than busbar self) or a wire of ConnectivityNode cn.
func (b *cimPageBuilder) touchesOwn(self int, cn string, p Point) bool {
	on := func(pts []Point) bool {
		for k := 1; k < len(pts); k++ {
			if onSeg(p, pts[k-1], pts[k]) {
				return true
			}
		}
		return false
	}
	for _, e := range b.d.Elements {
		if e.ID != self && e.Class == ClassBusBarSection && len(b.portCN[e.ID]) > 0 && b.portCN[e.ID][0] == cn && on(e.Points) {
			return true
		}
	}
	for _, c := range b.d.Connectors {
		if c.Kind == KindBusWork && b.connCN[c.ID][0] == cn && on(c.Points) {
			return true
		}
	}
	return false
}

// tapWires splits each wire where a Node of its own ConnectivityNode lies inside it (a T-junction), so the Node
// joins it, until none is left.
func (b *cimPageBuilder) tapWires(nodeCN map[int]string) {
	for i := 0; i < len(b.d.Connectors); i++ {
		c := b.d.Connectors[i]
		cn := b.connCN[c.ID][0]
		if c.Kind != KindBusWork || cn == "" {
			continue
		}
	nodes:
		for _, n := range b.d.Nodes {
			p := Point{X: n.X, Y: n.Y}
			if nodeCN[n.ID] != cn || n.ID == c.From || n.ID == c.To {
				continue
			}
			for k := 1; k < len(c.Points); k++ {
				a, z := c.Points[k-1], c.Points[k]
				if !onSeg(p, a, z) || p == z || p == a && k == 1 {
					continue
				}
				head := slices.Clone(c.Points[:k])
				if p != a {
					head = append(head, p)
				}
				tail := append([]Point{p}, c.Points[k:]...)
				id := b.take()
				b.connCN[id] = [2]string{cn, cn}
				b.d.Connectors[i].Points, b.d.Connectors[i].To = simplifyPath(head), n.ID
				rest := c
				rest.ID, rest.From, rest.Points = id, n.ID, simplifyPath(tail)
				b.d.Connectors = append(b.d.Connectors, rest)
				i-- // the shortened wire may have more
				break nodes
			}
		}
	}
}

// splitAtJunctions cuts each wire where something of its own ConnectivityNode ends inside it (a wire end or a
// terminal), so a wire jump put in later never lands on a junction.
func (b *cimPageBuilder) splitAtJunctions() {
	own := map[string][]Point{}
	for _, c := range b.d.Connectors {
		ends := b.connCN[c.ID]
		own[ends[0]] = append(own[ends[0]], c.Points[0])
		own[ends[1]] = append(own[ends[1]], c.Points[len(c.Points)-1])
	}
	for _, e := range b.d.Elements {
		ts := b.portAt[e.ID]
		if ts == nil {
			ts = symbolTerminals(&e, b.im.opt.Terminals)
		}
		for k, p := range ts {
			if k < len(b.portCN[e.ID]) {
				own[b.portCN[e.ID][k]] = append(own[b.portCN[e.ID][k]], roundPt(p))
			}
		}
	}
	for i := 0; i < len(b.d.Connectors); i++ {
		c := b.d.Connectors[i]
		cn := b.connCN[c.ID][0]
		if c.Kind != KindBusWork || cn == "" {
			continue
		}
	points:
		for _, p := range own[cn] {
			for k := 1; k < len(c.Points); k++ {
				a, z := c.Points[k-1], c.Points[k]
				if !onSeg(p, a, z) || p == z || p == a && k == 1 {
					continue
				}
				head := slices.Clone(c.Points[:k])
				if p != a {
					head = append(head, p)
				}
				rest := c
				rest.ID, rest.Points = b.take(), simplifyPath(append([]Point{p}, c.Points[k:]...))
				b.connCN[rest.ID] = [2]string{cn, cn}
				b.d.Connectors[i].Points = simplifyPath(head)
				b.d.Connectors = append(b.d.Connectors, rest)
				i--
				break points
			}
		}
	}
}

// addJumps puts a wire jump (shape 14) into a buswork wire where, in a foreign or generated layout, it crosses a
// busbar, another buswork wire, an overhead or cable line or a link to object of a different voltage class: the rules
// of manual drawing (diagramOps.addCrossingJumps). The wire is cut around the crossing and its pieces end on the
// jump's ports; a jump over a busbar is size step 1. Each crossing gets one jump: a line already cut there is not cut
// again. A crossing too near a bend, an end or the previous jump for the jump to fit on one straight run is only
// counted in the messages.
func (b *cimPageBuilder) addJumps() {
	type obstacle struct {
		segs    [][2]Point
		voltage int
		step    int
	}
	var obs []obstacle
	for _, c := range b.d.Connectors {
		if c.Voltage == 0 || c.Kind != KindBusWork && c.Kind != KindOverheadLine && c.Kind != KindCableLine && c.Kind != KindLinkToObject {
			continue
		}
		o := obstacle{voltage: c.Voltage}
		for k := 1; k < len(c.Points); k++ {
			o.segs = append(o.segs, [2]Point{c.Points[k-1], c.Points[k]})
		}
		obs = append(obs, o)
	}
	for _, e := range b.d.Elements {
		if e.Class != ClassBusBarSection || e.Voltage == 0 {
			continue
		}
		o := obstacle{voltage: e.Voltage, step: 1}
		for k := 1; k < len(e.Points); k++ {
			o.segs = append(o.segs, [2]Point{e.Points[k-1], e.Points[k]})
		}
		obs = append(obs, o)
	}
	half := func(step int) float64 {
		probe := Element{Class: ClassNonIntersection, Shape: "14", Scale: step}
		h := 0.0
		for _, t := range symbolTerminals(&probe, b.im.opt.Terminals) {
			h = math.Max(h, math.Hypot(t.X, t.Y))
		}
		return h
	}
	if half(0) == 0 {
		return
	}
	type hit struct {
		seg   int
		at    Point
		along float64
		step  int
	}
	jumps, skipped := 0, 0
	cut := map[Point]bool{} // crossings already taken by a jump
	for i := 0; i < len(b.d.Connectors); i++ {
		w := b.d.Connectors[i]
		cn := b.connCN[w.ID][0]
		if w.Kind != KindBusWork || cn == "" || w.Voltage == 0 {
			continue
		}
		var hits []hit
		along := 0.0
		for k := 1; k < len(w.Points); k++ {
			a, z := w.Points[k-1], w.Points[k]
			for _, o := range obs {
				if o.voltage == w.Voltage {
					continue
				}
				for _, sg := range o.segs {
					if at, ok := properCrossing(a, z, sg[0], sg[1]); ok && !cut[at] {
						hits = append(hits, hit{k - 1, at, along + math.Hypot(at.X-a.X, at.Y-a.Y), o.step})
					}
				}
			}
			along += math.Hypot(z.X-a.X, z.Y-a.Y)
		}
		if len(hits) == 0 {
			continue
		}
		slices.SortFunc(hits, func(x, y hit) int { return cmp.Compare(x.along, y.along) })
		pts := w.Points
		var pieces []Connector
		rest := slices.Clone(pts)
		restSeg, restStart := 0, math.Inf(-1)
		for _, h := range hits {
			hl := half(h.step)
			a, z := pts[h.seg], pts[h.seg+1]
			segStart := h.along - math.Hypot(h.at.X-a.X, h.at.Y-a.Y)
			segEnd := segStart + math.Hypot(z.X-a.X, z.Y-a.Y)
			if h.along-hl <= math.Max(segStart, restStart)+1e-6 || h.along+hl >= segEnd-1e-6 {
				skipped++
				continue
			}
			orient := int(math.Round(math.Atan2(z.Y-a.Y, z.X-a.X) * 180 / math.Pi))
			for orient >= 90 {
				orient -= 180
			}
			for orient < -90 {
				orient += 180
			}
			j := Element{ID: b.take(), Class: ClassNonIntersection, Shape: "14", Voltage: w.Voltage, Layer: w.Layer,
				X: h.at.X, Y: h.at.Y, Orient: orient, Scale: h.step}
			ts := symbolTerminals(&j, b.im.opt.Terminals)
			if len(ts) != 2 {
				continue
			}
			cut[h.at] = true
			near, far := roundPt(ts[0]), roundPt(ts[1])
			if math.Hypot(far.X-a.X, far.Y-a.Y) < math.Hypot(near.X-a.X, near.Y-a.Y) {
				near, far = far, near
			}
			b.portAt[j.ID] = []Point{roundPt(ts[0]), roundPt(ts[1])} // port i on terminal i; the wire meets them by point
			b.portCN[j.ID] = []string{cn, cn}
			b.d.Elements = append(b.d.Elements, j)
			piece := w
			if len(pieces) > 0 {
				piece.ID = b.take()
			}
			piece.Points = simplifyPath(append(slices.Clone(rest[:h.seg-restSeg+1]), near))
			pieces = append(pieces, piece)
			rest = append([]Point{far}, pts[h.seg+1:]...)
			restSeg, restStart = h.seg, h.along+hl
			jumps++
		}
		if len(pieces) == 0 {
			continue
		}
		last := w
		last.ID = b.take()
		last.Points = simplifyPath(rest)
		pieces = append(pieces, last)
		for _, p := range pieces {
			b.connCN[p.ID] = [2]string{cn, cn}
		}
		b.d.Connectors = append(append(slices.Clone(b.d.Connectors[:i]), pieces...), b.d.Connectors[i+1:]...)
		i += len(pieces) - 1
	}
	if jumps > 0 {
		b.messages = append(b.messages, fmt.Sprintf("%d wire jumps mark where wires of different voltage classes cross", jumps))
	}
	if skipped > 0 {
		b.messages = append(b.messages, fmt.Sprintf("%d places where wires of different voltage classes cross had no room for a wire jump; they are not connected", skipped))
	}
}

// properCrossing is where segments a–z and p–q cross inside both (not at an end of either), if they do.
func properCrossing(a, z, p, q Point) (Point, bool) {
	rx, ry := z.X-a.X, z.Y-a.Y
	sx, sy := q.X-p.X, q.Y-p.Y
	den := rx*sy - ry*sx
	if math.Abs(den) < 1e-9 {
		return Point{}, false
	}
	t := ((p.X-a.X)*sy - (p.Y-a.Y)*sx) / den
	u := ((p.X-a.X)*ry - (p.Y-a.Y)*rx) / den
	const eps = 1e-6
	if t <= eps || t >= 1-eps || u <= eps || u >= 1-eps {
		return Point{}, false
	}
	return roundPt(Point{X: a.X + t*rx, Y: a.Y + t*ry}), true
}

// cimPart is a connected piece of one ConnectivityNode's drawing: its points and segments.
type cimPart struct {
	pts  []Point
	segs [][2]Point
	bus  bool
	need bool
}

func (p *cimPart) touches(q Point) bool {
	if slices.Contains(p.pts, q) {
		return true
	}
	for _, s := range p.segs {
		if onSeg(q, s[0], s[1]) {
			return true
		}
	}
	return false
}

func (p *cimPart) meets(o *cimPart) bool {
	if slices.ContainsFunc(o.pts, p.touches) {
		return true
	}
	for _, s := range o.segs {
		if p.touches(s[0]) || p.touches(s[1]) {
			return true
		}
	}
	return false
}

// nearest is the point of p closest to q (a segment's closest point when orthogonal) and its distance.
func (p *cimPart) nearest(q Point) (Point, float64) {
	best, bd := Point{}, math.Inf(1)
	try := func(c Point) {
		if d := math.Hypot(c.X-q.X, c.Y-q.Y); d < bd {
			best, bd = c, d
		}
	}
	for _, a := range p.pts {
		try(a)
	}
	for _, s := range p.segs {
		try(s[0])
		try(s[1])
		if s[0].Y == s[1].Y && q.X > math.Min(s[0].X, s[1].X) && q.X < math.Max(s[0].X, s[1].X) {
			try(Point{X: q.X, Y: s[0].Y})
		}
		if s[0].X == s[1].X && q.Y > math.Min(s[0].Y, s[1].Y) && q.Y < math.Max(s[0].Y, s[1].Y) {
			try(Point{X: s[0].X, Y: q.Y})
		}
	}
	return best, bd
}

func (b *cimPageBuilder) wireNode(cn string) int {
	var parts []*cimPart
	groups := map[int]*cimPart{}
	for _, a := range b.attach[cn] {
		if g := groups[a.group]; g != nil && a.group != 0 {
			g.pts = append(g.pts, a.at)
			g.need = g.need || !a.optional
			continue
		}
		p := &cimPart{pts: []Point{a.at}, need: !a.optional}
		parts = append(parts, p)
		if a.group != 0 {
			groups[a.group] = p
		}
	}
	for _, s := range b.busbars[cn] {
		parts = append(parts, &cimPart{segs: [][2]Point{s}, bus: true, need: true})
	}
	layer := b.cnLayer[cn]
	voltage := 0
	if o := b.im.objs[cn]; o != nil {
		if c := b.im.objs[o.get("ConnectivityNodeContainer")]; c != nil {
			voltage = b.voltage(b.im.containerBV(c))
		}
	}
	addWire := func(pts []Point) {
		id := b.take()
		b.connCN[id] = [2]string{cn, cn}
		b.d.Connectors = append(b.d.Connectors, Connector{ID: id, Kind: KindBusWork, Voltage: voltage, Layer: layer, Points: pts})
		b.report.Connectors++
	}
	for _, w := range b.wires[cn] {
		if len(w) < 2 {
			continue
		}
		p := &cimPart{pts: []Point{w[0], w[len(w)-1]}}
		for i := 1; i < len(w); i++ {
			p.segs = append(p.segs, [2]Point{w[i-1], w[i]})
		}
		parts = append(parts, p)
		addWire(w)
	}
	// Merge touching parts until none touch.
	for merged := true; merged; {
		merged = false
		for i := 0; i < len(parts) && !merged; i++ {
			for j := i + 1; j < len(parts); j++ {
				if parts[i].meets(parts[j]) || parts[j].meets(parts[i]) {
					parts[i].pts = append(parts[i].pts, parts[j].pts...)
					parts[i].segs = append(parts[i].segs, parts[j].segs...)
					parts[i].bus = parts[i].bus || parts[j].bus
					parts[i].need = parts[i].need || parts[j].need
					parts = slices.Delete(parts, j, j+1)
					merged = true
					break
				}
			}
		}
	}
	// The rest is wired to the busbar's part, else to the one with the most drawn wire, else the largest.
	hub := 0
	better := func(p, q *cimPart) bool {
		if p.bus != q.bus {
			return p.bus
		}
		if len(p.segs) != len(q.segs) {
			return len(p.segs) > len(q.segs)
		}
		return len(p.pts) > len(q.pts)
	}
	for i, p := range parts {
		if better(p, parts[hub]) {
			hub = i
		}
	}
	main := parts[hub]
	parts = slices.DeleteFunc(parts, func(p *cimPart) bool { return p != main && !p.need })
	hub = slices.Index(parts, main)
	if len(parts) < 2 {
		return 0
	}
	routed := 0
	for i, p := range parts {
		if i == hub {
			continue
		}
		from, to, bd := Point{}, Point{}, math.Inf(1)
		cands := slices.Clone(p.pts)
		for _, s := range p.segs {
			cands = append(cands, s[0], s[1])
		}
		for _, q := range cands {
			if t, d := main.nearest(q); d < bd {
				from, to, bd = q, t, d
			}
		}
		route := []Point{from}
		if from.X != to.X && from.Y != to.Y {
			route = append(route, Point{X: from.X, Y: to.Y})
		}
		route = append(route, to)
		addWire(route)
		main.pts = append(main.pts, p.pts...)
		main.segs = append(main.segs, p.segs...)
		for k := 1; k < len(route); k++ {
			main.segs = append(main.segs, [2]Point{route[k-1], route[k]})
		}
		routed++
	}
	return routed
}

// containerBV is the BaseVoltage of container c: its own, or a Bay's voltage level's.
func (im *cimImporter) containerBV(c *cimObject) string {
	if c.class == "Bay" {
		if vl := im.objs[c.get("VoltageLevel")]; vl != nil {
			return vl.get("BaseVoltage")
		}
	}
	return c.get("BaseVoltage")
}

// mridOf is CIM object o's mRID: its IdentifiedObject.mRID, else its id without the leading "_".
func mridOf(o *cimObject) string {
	if m := o.get("mRID"); m != "" {
		return m
	}
	return strings.TrimPrefix(o.id, "_")
}

// identities gives every item drawn for a CIM object that object's mRID, so an export keeps it: an element its
// equipment's (and each port its Terminal's, each winding its PowerTransformerEnd's), a line or link its own and its
// ends' Terminals', a Node its ConnectivityNode's. A changeover knife switch, two Disconnectors in one element, keeps
// none.
func (b *cimPageBuilder) identities() {
	objsOf := map[int][]*cimObject{}
	for id, item := range b.itemOf {
		if o := b.im.objs[id]; o != nil && !isNode(o) {
			objsOf[item] = append(objsOf[item], o)
		}
	}
	terms := func(o *cimObject) []string {
		var out []string
		for _, t := range b.im.terminals[o.id] {
			out = append(out, mridOf(t))
		}
		return out
	}
	for i := range b.d.Elements {
		e := &b.d.Elements[i]
		objs := objsOf[e.ID]
		if len(objs) != 1 {
			continue
		}
		o := objs[0]
		e.MRID = mridOf(o)
		ts := terms(o)
		if e.Class == ClassPowerTransformer {
			ts = ts[:0]
			for k, end := range b.im.transformerEnds(o) {
				if k < len(e.Windings) {
					e.Windings[k].MRID = mridOf(end)
				}
				t := ""
				if to := b.im.objs[end.get("Terminal")]; to != nil {
					t = mridOf(to)
				}
				ts = append(ts, t)
			}
		}
		if cimAuxiliaryClasses[o.class] {
			continue
		}
		for k := range e.Ports {
			if k < len(ts) && (e.Class != ClassBusBarSection || k == 0) {
				e.Ports[k].MRID = ts[k]
			}
		}
	}
	for i := range b.d.Connectors {
		c := &b.d.Connectors[i]
		objs := objsOf[c.ID]
		if len(objs) != 1 {
			continue
		}
		o := objs[0]
		c.MRID = mridOf(o)
		ts := b.im.terminals[o.id]
		if len(ts) > 0 {
			c.FromMRID = mridOf(ts[0])
		}
		if len(ts) > 1 {
			c.ToMRID = mridOf(ts[1])
			if ends := b.connCN[c.ID]; ends[0] != ts[0].get("ConnectivityNode") && ends[0] == ts[1].get("ConnectivityNode") {
				c.FromMRID, c.ToMRID = c.ToMRID, c.FromMRID
			}
		}
	}
	for i := range b.d.Nodes {
		if o := b.im.objs[b.nodeCN[b.d.Nodes[i].ID]]; o != nil && o.class == "ConnectivityNode" {
			b.d.Nodes[i].MRID = mridOf(o)
		}
	}
}

// nameLabels labels each named device, busbar and line with its name, as the automatic layout places them: beside an
// upright device, over a lying one, over a busbar's first end and beside a line's longest run; a name that would
// overlap one already placed, or a busbar, steps away from its item a line-height at a time, then the same ending
// at its item. Returns how many.
func (b *cimPageBuilder) nameLabels() int {
	g := b.im.opt.Grid
	box := func(at Point, text string) [4]float64 {
		return [4]float64{at.X, at.Y - 10, at.X + 6*float64(len([]rune(text))), at.Y + 2}
	}
	var placed [][4]float64
	named := map[int]bool{}
	for _, l := range b.d.Labels {
		named[l.For] = true
		placed = append(placed, box(Point{X: l.X, Y: l.Y}, l.Text))
	}
	for _, e := range b.d.Elements {
		for i := 1; e.Class == ClassBusBarSection && i < len(e.Points); i++ {
			p, q := e.Points[i-1], e.Points[i]
			placed = append(placed, [4]float64{math.Min(p.X, q.X) - 2, math.Min(p.Y, q.Y) - 2, math.Max(p.X, q.X) + 2, math.Max(p.Y, q.Y) + 2})
		}
	}
	n := 0
	// add places text at the first of at, at+step, at+2·step… (then at−step… when back is set) clear of the others.
	add := func(item, layer int, at, step Point, back bool, text string) {
		if named[item] {
			return
		}
		pick, found := at, false
		w := 6 * float64(len([]rune(text)))
		for _, dx := range []float64{0, 2*g - w} {
			for k := range 12 {
				m := float64(k)
				if back {
					m = float64((k + 1) / 2)
					if k%2 == 0 {
						m = -m
					}
				}
				cand := at.add(Point{X: dx + step.X*m, Y: step.Y * m})
				if !slices.ContainsFunc(placed, func(o [4]float64) bool { return overlaps(o, box(cand, text)) }) {
					pick, found = cand, true
					break
				}
			}
			if found {
				break
			}
		}
		placed = append(placed, box(pick, text))
		b.d.Labels = append(b.d.Labels, Label{ID: b.take(), For: item, Layer: layer, X: pick.X, Y: pick.Y, Size: 10, Text: text})
		b.report.Labels++
		n++
	}
	up, down := Point{Y: -1.5 * g}, Point{Y: 1.5 * g}
	for _, e := range b.d.Elements {
		if e.Name == "" {
			continue
		}
		if e.Class == ClassBusBarSection {
			if len(e.Points) < 2 {
				continue
			}
			a, z := e.Points[0], e.Points[len(e.Points)-1]
			if math.Abs(z.X-a.X) >= math.Abs(z.Y-a.Y) {
				add(e.ID, e.Layer, Point{X: math.Min(a.X, z.X), Y: a.Y - g}, up, false, e.Name)
			} else {
				add(e.ID, e.Layer, Point{X: a.X + g, Y: math.Min(a.Y, z.Y) - g}, up, false, e.Name)
			}
			continue
		}
		x1, y1, x2, y2 := 0.0, 0.0, 0.0, 0.0
		for i, t := range symbolTerminals(&e, b.im.opt.Terminals) {
			if i == 0 {
				x1, y1, x2, y2 = t.X, t.Y, t.X, t.Y
			}
			x1, y1, x2, y2 = math.Min(x1, t.X), math.Min(y1, t.Y), math.Max(x2, t.X), math.Max(y2, t.Y)
		}
		switch {
		case x2-x1 > y2-y1:
			add(e.ID, e.Layer, Point{X: e.X - g, Y: math.Min(y1, e.Y) - 2*g}, up, false, e.Name)
		case e.Class == ClassPowerTransformer:
			add(e.ID, e.Layer, Point{X: e.X + 5*g, Y: e.Y}, down, true, e.Name)
		default:
			add(e.ID, e.Layer, Point{X: e.X + 2*g, Y: e.Y}, down, true, e.Name)
		}
	}
	for _, c := range b.d.Connectors {
		if c.Name == "" || len(c.Points) < 2 || c.Kind == KindBusWork {
			continue
		}
		k := 1
		for i := 2; i < len(c.Points); i++ {
			if pathLength(c.Points[i-1:i+1]) > pathLength(c.Points[k-1:k+1]) {
				k = i
			}
		}
		a, z := c.Points[k-1], c.Points[k]
		mid := Point{X: (a.X + z.X) / 2, Y: (a.Y + z.Y) / 2}
		if math.Abs(z.X-a.X) >= math.Abs(z.Y-a.Y) {
			add(c.ID, c.Layer, Point{X: mid.X, Y: mid.Y - g}, up, true, c.Name)
		} else {
			add(c.ID, c.Layer, Point{X: mid.X + g, Y: mid.Y}, down, true, c.Name)
		}
	}
	return n
}

// labels turns TextDiagramObjects into labels, linked to the item they name.
func (b *cimPageBuilder) labels(dos []*cimObject) {
	for _, do := range dos {
		if do.class != "TextDiagramObject" {
			continue
		}
		pts := b.doPoints(do)
		if len(pts) == 0 || do.get("text") == "" {
			continue
		}
		b.d.Labels = append(b.d.Labels, Label{ID: b.take(), For: b.itemOf[do.get("IdentifiedObject")], Layer: b.layer(do),
			X: pts[0].X, Y: pts[0].Y, Size: 10, Text: do.get("text")})
		b.report.Labels++
	}
}

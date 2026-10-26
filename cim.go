package slddoc

import (
	"bytes"
	"crypto/sha1"
	"encoding/xml"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// CGMES 3.0 (CIM100) export: an Equipment (EQ) and a Diagram Layout (DL) profile document, as RDF/XML
// (IEC 61970-552). The whole diagram becomes one Substation with a VoltageLevel per voltage class; each
// overhead/cable line is an ACLineSegment in its own Line. Buswork wires, busbar ports and pass-through
// symbols (junctions, wire jumps, poles, cable joints, current transformers, fault indicators, blocking
// filters) are zero-impedance, so the nodes they join become one ConnectivityNode. Decorative shapes, link
// arrows and digital devices are not exported.

const (
	cimNS           = "http://iec.ch/TC57/CIM100#"
	cimProfileEQ    = "http://iec.ch/TC57/ns/CIM/CoreEquipment-EU/3.0"
	cimProfileDL    = "http://iec.ch/TC57/ns/CIM/DiagramLayout-EU/3.0"
	cimProfileSC    = "http://iec.ch/TC57/ns/CIM/ShortCircuit-EU/3.0"
	cimAuthoritySet = "urn:sld-editor"
)

// CIMOptions names the exported model: Name is the diagram's name (it seeds every mRID, so re-exporting the
// same diagram keeps its ids), Created the export time; Classes is the CIM class table (nil: the built-in lists).
type CIMOptions struct {
	Name    string
	Created time.Time
	Classes CIMClassTable
}

// CIMWarning reports something the export had to fill in or leave out. Item is "element", "connector" or
// "voltageClass" and ID its id. Kind is:
//   - "missing": required parameters (Fields, named as in Electrical/TransformerWinding, plus
//     "connectionKind") were unset and were written as 0, or as the voltage class's nominal voltage;
//   - "voltage": the voltage class's name holds no voltage, so its BaseVoltage is 0;
//   - "noTerminal": an auxiliary device (CT, VT, arrester, fault indicator, wave trap) found no equipment
//     terminal on its node to attach to;
//   - "notExported": the element has connections but no CIM counterpart;
//   - "openEnd": a link to object has something at its arrow end, which is left unconnected;
//   - "shorted": a switch or line has both ends on one node and is left out;
//   - "windings": a transformer has 4 windings, which some tools (PowSyBl among them) can't read;
//   - "guessedVoltage": nothing on or around the item has a voltage, so it was given its substation's (or the
//     diagram's) most common one;
//   - "mrid": an mRID the item keeps (its own, a terminal's or a winding's) is another item's too, or unfit for an
//     rdf:ID, and a new one was given;
//   - "mridFormat": an mRID the item keeps is written as it is but is not a UUID, which CGMES 3.0 requires.
type CIMWarning struct {
	Kind   string   `json:"kind"`
	Item   string   `json:"item"`
	ID     int      `json:"id"`
	Fields []string `json:"fields,omitempty"`
}

// CIMExport holds the two profile documents and the warnings.
type CIMExport struct {
	EQ       []byte
	DL       []byte
	Warnings []CIMWarning
}

// How each Class is exported.
type cimRole int

const (
	roleSkip        cimRole = iota
	roleEquipment           // ConductingEquipment with a Terminal per port
	rolePassThrough         // ports joined into one node, no equipment
	roleAuxiliary           // ports joined into one node, an AuxiliaryEquipment on it
	roleAuxOnPort1          // AuxiliaryEquipment on port 1's node, ports not joined
	roleBusbar              // BusbarSection: ports joined, one Terminal
	roleTransformer         // PowerTransformer + PowerTransformerEnd per winding
	roleBooster             // PowerTransformer with two ends
	roleKnife               // changeover knife switch: two Disconnectors from the pivot
	roleShunt               // one-terminal LinearShuntCompensator on port 1
)

type cimClassInfo struct {
	role  cimRole
	class string
}

var cimClasses = map[Class]cimClassInfo{
	ClassBreaker:                {roleEquipment, "Breaker"},
	ClassPowerCircuitBreaker:    {roleEquipment, "Breaker"},
	ClassDisconnector:           {roleEquipment, "Disconnector"},
	ClassLoadBreakSwitch:        {roleEquipment, "LoadBreakSwitch"},
	ClassGroundSwitch:           {roleEquipment, "GroundDisconnector"},
	ClassShortCircuiter:         {roleEquipment, "GroundDisconnector"},
	ClassShortCircuiterNoGround: {roleEquipment, "Switch"},
	ClassSectionalizer:          {roleEquipment, "Switch"},
	ClassChassis:                {roleEquipment, "Switch"},
	ClassHalfChassis:            {roleEquipment, "Switch"},
	ClassStarter:                {roleEquipment, "Switch"},
	ClassFuse:                   {roleEquipment, "Fuse"},
	ClassDisconnectorFuse:       {roleEquipment, "Fuse"},
	ClassKnifeSwitch:            {roleKnife, "Disconnector"},
	ClassKnifeSwitch3:           {roleKnife, "Disconnector"},
	ClassGround:                 {roleEquipment, "Ground"},
	ClassGenerator:              {roleEquipment, "SynchronousMachine"},
	ClassSynchronousCompensator: {roleEquipment, "SynchronousMachine"},
	ClassSynchronousMotor:       {roleEquipment, "SynchronousMachine"},
	ClassReactor:                {roleEquipment, "SeriesCompensator"},
	ClassChokeCoil:              {roleEquipment, "SeriesCompensator"},
	ClassResistor:               {roleEquipment, "SeriesCompensator"},
	ClassCapacitor:              {roleShunt, "LinearShuntCompensator"},
	ClassCapacitorBank:          {roleShunt, "LinearShuntCompensator"},
	ClassReactorShunt:           {roleShunt, "LinearShuntCompensator"},
	ClassBusBarSection:          {roleBusbar, "BusbarSection"},
	ClassPowerTransformer:       {roleTransformer, "PowerTransformer"},
	ClassBooster:                {roleBooster, "PowerTransformer"},
	ClassCurrentTransformer:     {roleAuxiliary, "CurrentTransformer"},
	ClassFaultPassageIndicator:  {roleAuxiliary, "FaultIndicator"},
	ClassBlockingFilter:         {roleAuxiliary, "WaveTrap"},
	ClassVoltageTransformer:     {roleAuxOnPort1, "PotentialTransformer"},
	ClassSurgeArrester:          {roleAuxOnPort1, "SurgeArrester"},
	ClassLoad:                   {roleEquipment, "EnergyConsumer"},
	ClassNetworkInfeed:          {roleEquipment, "ExternalNetworkInjection"},
	ClassStaticVarCompensator:   {roleEquipment, "StaticVarCompensator"},
	ClassJunctionPoint:          {rolePassThrough, ""},
	ClassConnectorPoint:         {rolePassThrough, ""},
	ClassConnectorArrow:         {rolePassThrough, ""},
	ClassFork:                   {rolePassThrough, ""},
	ClassNonIntersection:        {rolePassThrough, ""},
	ClassCableConnector:         {rolePassThrough, ""},
	ClassCableJoint:             {rolePassThrough, ""},
	ClassPowerPole:              {rolePassThrough, ""},
	ClassAnchorPole:             {rolePassThrough, ""},
}

// cimAlternatives lists, per element class, the CIM classes it may stand for besides its default.
var cimAlternatives = map[Class][]string{
	ClassBreaker:          {"DisconnectingCircuitBreaker"},
	ClassStarter:          {"LoadBreakSwitch"},
	ClassGenerator:        {"AsynchronousMachine"},
	ClassSynchronousMotor: {"AsynchronousMachine"},
	ClassReactorShunt:     {"PetersenCoil"},
	ClassLoad:             {"ConformLoad", "NonConformLoad"},
	ClassNetworkInfeed:    {"EquivalentInjection", "EnergySource"},
}

// CIMClassTable maps a symbol shape code, or LinkToObjectCode for links to object, to the CIM classes an element
// of that shape can be exported as, its default first. A nil table, or a shape missing from it, falls back to the
// built-in lists of its element class (CIMClasses).
type CIMClassTable map[string][]string

// LinkToObjectCode is the shape code of a link to object (xsde2svg 28), its CIMClassTable key.
const LinkToObjectCode = "28"

var builtinLinkCIM = []string{"EnergyConsumer", "EnergySource"}

// cimFamilies groups the CIM classes the export writes alike; an element can stand for any class of its default's
// family (a class in no family stands for itself only).
var cimFamilies = [][]string{
	{"Breaker", "DisconnectingCircuitBreaker", "Disconnector", "LoadBreakSwitch", "GroundDisconnector", "Switch", "Fuse", "Jumper"},
	{"SynchronousMachine", "AsynchronousMachine"},
	{"EnergyConsumer", "ConformLoad", "NonConformLoad", "ExternalNetworkInjection", "EquivalentInjection", "EnergySource"},
	{"LinearShuntCompensator", "PetersenCoil"},
	{"CurrentTransformer", "PotentialTransformer", "SurgeArrester", "FaultIndicator", "WaveTrap"},
}

func cimFamily(class string) []string {
	for _, f := range cimFamilies {
		if slices.Contains(f, class) {
			return f
		}
	}
	return []string{class}
}

// NewCIMClassTable builds the table of the shapes given (shape code → element class, the element library): each
// shape exported as equipment gets its class's built-in list, and overrides (shape code → CIM classes, default
// first) replace those of the shapes they name. Every override is checked: the code must be a shape exported as
// CIM equipment (or LinkToObjectCode), and every CIM class one the export writes alike for it, listed once.
func NewCIMClassTable(shapes map[string]Class, overrides map[string][]string) (CIMClassTable, error) {
	t := CIMClassTable{LinkToObjectCode: slices.Clone(builtinLinkCIM)}
	for code, c := range shapes {
		if l := CIMClasses(c); l != nil {
			t[code] = l
		}
	}
	codes := make([]string, 0, len(overrides))
	for code := range overrides {
		codes = append(codes, code)
	}
	slices.Sort(codes)
	for _, code := range codes {
		classes := overrides[code]
		def, ok := t[code]
		if !ok {
			if c, known := shapes[code]; known {
				return nil, fmt.Errorf("cim.classes: shape %q (%s) is not exported as CIM equipment", code, c)
			}
			return nil, fmt.Errorf("cim.classes: no shape %q in the element library", code)
		}
		if len(classes) == 0 {
			return nil, fmt.Errorf("cim.classes: shape %q lists no CIM class", code)
		}
		family := cimFamily(def[0])
		for i, cls := range classes {
			if !slices.Contains(family, cls) {
				return nil, fmt.Errorf("cim.classes: shape %q can't be exported as %q; choose from %s", code, cls, strings.Join(family, ", "))
			}
			if slices.Contains(classes[:i], cls) {
				return nil, fmt.Errorf("cim.classes: shape %q lists %q twice", code, cls)
			}
		}
		t[code] = slices.Clone(classes)
	}
	return t, nil
}

// Of returns the CIM classes an element of shape code can be exported as, its default first; nil when it isn't
// exported as equipment (or the table has no such shape).
func (t CIMClassTable) Of(code string) []string {
	return slices.Clone(t[code])
}

// elementClasses is the list e's shape has in t, else the built-in one of its class.
func (t CIMClassTable) elementClasses(e Element) []string {
	if l, ok := t[e.Shape]; ok {
		return l
	}
	return CIMClasses(e.Class)
}

// linkClasses is the list of links to object.
func (t CIMClassTable) linkClasses() []string {
	if l, ok := t[LinkToObjectCode]; ok {
		return l
	}
	return builtinLinkCIM
}

// CIMClasses returns the built-in CIM classes of element class c, its default first; nil when c is not exported as
// equipment.
func CIMClasses(c Class) []string {
	info, ok := cimClasses[c]
	if !ok || info.class == "" {
		return nil
	}
	return append([]string{info.class}, cimAlternatives[c]...)
}

// ConnectorCIMClasses returns the built-in CIM classes of a connector kind exported as equipment (links to object).
func ConnectorCIMClasses(k ConnectorKind) []string {
	if k == KindLinkToObject {
		return slices.Clone(builtinLinkCIM)
	}
	return nil
}

// classOf is e's CIM class: its own choice when its shape's list allows it, else that list's default.
func (t CIMClassTable) classOf(e Element) string {
	l := t.elementClasses(e)
	if e.CIM != "" && slices.Contains(l, e.CIM) {
		return e.CIM
	}
	if len(l) > 0 {
		return l[0]
	}
	return cimClasses[e.Class].class
}

// Pictures of whole installations: skipped without a warning even when they have ports.
var cimDecorWithPorts = map[Class]bool{
	ClassPowerPlant: true, ClassSubstation: true, ClassPackageSubstation: true, ClassEnclosedSubstation: true,
}

// rdfObject is one CIM object: its class, rdf:ID and properties in order.
type rdfObject struct {
	class string
	id    string
	props []rdfProp
}

type rdfProp struct {
	name  string
	value string
	kind  byte // 'l' literal, 'r' reference to an rdf:ID, 'e' enum URI
}

func (o *rdfObject) lit(name, value string) *rdfObject {
	o.props = append(o.props, rdfProp{name, value, 'l'})
	return o
}

func (o *rdfObject) num(name string, v float64) *rdfObject {
	return o.lit(name, strconv.FormatFloat(v, 'g', -1, 64))
}

func (o *rdfObject) ref(name, id string) *rdfObject {
	if id != "" {
		o.props = append(o.props, rdfProp{name, id, 'r'})
	}
	return o
}

func (o *rdfObject) enum(name, value string) *rdfObject {
	o.props = append(o.props, rdfProp{name, cimNS + value, 'e'})
	return o
}

type cimExporter struct {
	d     *Diagram
	opt   CIMOptions
	fixed map[string]string // object key → the mRID the diagram keeps for it (see fixMRIDs)
	eq    []*rdfObject
	dl    []*rdfObject
	warn  []CIMWarning

	nodes    map[int]Node
	parent   map[int]int
	cn       map[int]string // root node → ConnectivityNode id
	cnVolt   map[int]int    // root node → VoltageClass id
	classes  map[int]VoltageClass
	bv       map[int]string // VoltageClass id → BaseVoltage id
	nominal  map[int]float64
	region   string
	layerZ   map[int]int
	layerDOs map[int][]string
	eqOf     map[int]string // element/connector id → its (first) equipment id, for labels
	styles   map[string]string
	glue     map[int][]*rdfObject // node id → DiagramObjectPoints on it
	termsAt  map[int][]string     // root node → equipment Terminal ids, in order
	diagram  string
	auxLater []func()
	subFind  func(int) int
	used     map[int]int // slddoc node → how many ports and connector ends use it
	table    CIMClassTable

	subParent map[int]int    // node root → its substation's root (union-find over node roots)
	subName   map[int]string // substation root → its name
	subs      map[int]string // substation root → its Substation id
	vls       map[[2]int]string
	badClass  map[int]bool // voltage classes already reported as holding no voltage
}

// ExportCIM converts d to CGMES 3.0 EQ and DL documents.
func ExportCIM(d *Diagram, opt CIMOptions) *CIMExport {
	x := &cimExporter{
		d: d, opt: opt,
		nodes: map[int]Node{}, parent: map[int]int{}, cn: map[int]string{}, cnVolt: map[int]int{},
		classes: map[int]VoltageClass{}, bv: map[int]string{}, nominal: map[int]float64{},
		layerZ: map[int]int{}, layerDOs: map[int][]string{}, eqOf: map[int]string{}, styles: map[string]string{},
		glue: map[int][]*rdfObject{}, termsAt: map[int][]string{},
	}
	for _, n := range d.Nodes {
		x.nodes[n.ID] = n
	}
	x.table = opt.Classes
	x.used = map[int]int{}
	for _, e := range d.Elements {
		for _, p := range e.Ports {
			x.used[p.Node]++
		}
	}
	for _, c := range d.Connectors {
		x.used[c.From]++
		x.used[c.To]++
	}
	for _, l := range d.Layers {
		x.layerZ[l.ID] = l.Z
	}
	for _, vc := range d.VoltageClasses {
		x.classes[vc.ID] = vc
	}
	x.subParent, x.subName, x.subs, x.vls, x.badClass = map[int]int{}, map[int]string{}, map[int]string{}, map[[2]int]string{}, map[int]bool{}
	x.joinNodes()
	x.fixMRIDs()
	x.containers()
	x.substations()
	x.diagram = x.id("diagram")
	x.dl = append(x.dl, (&rdfObject{class: "Diagram", id: x.diagram}).
		lit("IdentifiedObject.mRID", x.mrid("diagram")).
		lit("IdentifiedObject.name", x.modelName()).
		enum("Diagram.orientation", "OrientationKind.negative").
		num("Diagram.x1InitialView", 0).num("Diagram.y1InitialView", 0).
		num("Diagram.x2InitialView", d.Width).num("Diagram.y2InitialView", d.Height))
	for _, e := range d.Elements {
		x.element(e)
	}
	for _, c := range d.Connectors {
		x.connector(c)
	}
	for _, f := range x.auxLater {
		f()
	}
	x.labels()
	x.gluePoints()
	x.layers()

	x.finish()
	eqProfiles := []string{cimProfileEQ}
	if x.shortCircuit() {
		eqProfiles = append(eqProfiles, cimProfileSC)
	}
	eqModel := x.mrid("model:EQ:" + opt.Created.UTC().Format(time.RFC3339Nano))
	dlModel := x.mrid("model:DL:" + opt.Created.UTC().Format(time.RFC3339Nano))
	return &CIMExport{
		EQ:       x.document(eqModel, eqProfiles, "", x.eq),
		DL:       x.document(dlModel, []string{cimProfileDL}, eqModel, x.dl),
		Warnings: x.warn,
	}
}

func (x *cimExporter) modelName() string {
	if x.opt.Name == "" {
		return "diagram"
	}
	return x.opt.Name[strings.LastIndex(x.opt.Name, "/")+1:]
}

// mrid is the mRID the diagram keeps for the object with the given key (an imported one), else a name-based
// (version 5) UUID of the diagram's name and key, so an object keeps its mRID across exports.
func (x *cimExporter) mrid(key string) string {
	if m, ok := x.fixed[key]; ok {
		return m
	}
	h := sha1.Sum([]byte("sld-editor/cim\x00" + x.opt.Name + "\x00" + key))
	h[6] = h[6]&0x0f | 0x50
	h[8] = h[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", h[0:4], h[4:6], h[6:8], h[8:10], h[10:16])
}

var (
	mridRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
)

// fixMRIDs takes the mRIDs the diagram keeps (Element.MRID and the like, from a CIM import) for the objects they
// stand for: an element's equipment, Terminals and PowerTransformerEnds, a line's or link's equipment and
// Terminals, and a ConnectivityNode whose Nodes keep one mRID between them (Nodes added since keep none; Nodes
// of several joined since keep several, and then it gets a new one). One taken already, or not fit for an rdf:ID, is left to a derived one, with a "mrid" warning
// for its element or connector.
func (x *cimExporter) fixMRIDs() {
	x.fixed = map[string]string{}
	taken := map[string]bool{}
	notUUID := false
	take := func(key, m string) bool {
		if m == "" {
			return true
		}
		if taken[m] || !mridRe.MatchString(m) {
			return false
		}
		taken[m] = true
		x.fixed[key] = m
		notUUID = notUUID || !uuidRe.MatchString(m)
		return true
	}
	for _, e := range x.d.Elements {
		key := fmt.Sprintf("el:%d", e.ID)
		ok := take(key, e.MRID)
		for i, p := range sortedPorts(e) {
			ok = take(fmt.Sprintf("%s:t:%d", key, i+1), p.MRID) && ok
		}
		for i, w := range e.Windings {
			ok = take(fmt.Sprintf("%s:end:%d", key, i+1), w.MRID) && ok
		}
		if !ok {
			x.warnf("mrid", "element", e.ID)
		} else if notUUID {
			x.warnf("mridFormat", "element", e.ID)
		}
		notUUID = false
	}
	for _, c := range x.d.Connectors {
		key := fmt.Sprintf("line:%d", c.ID)
		if c.Kind == KindLinkToObject {
			key = fmt.Sprintf("link:%d", c.ID)
		}
		ok := take(key, c.MRID)
		ok = take(key+":t:1", c.FromMRID) && ok
		ok = take(key+":t:2", c.ToMRID) && ok
		if !ok {
			x.warnf("mrid", "connector", c.ID)
		} else if notUUID {
			x.warnf("mridFormat", "connector", c.ID)
		}
		notUUID = false
	}
	kept := map[int]map[string]bool{}
	for _, n := range x.d.Nodes {
		r := x.find(n.ID)
		if n.MRID == "" {
			continue
		}
		if kept[r] == nil {
			kept[r] = map[string]bool{}
		}
		kept[r][n.MRID] = true
	}
	roots := slices.Sorted(maps.Keys(kept))
	for _, r := range roots {
		if len(kept[r]) == 1 {
			for m := range kept[r] {
				take(fmt.Sprintf("cn:%d", r), m)
			}
		}
	}
}

// cimShortCircuitProps are the attributes the export writes that CGMES 3.0 keeps in the ShortCircuit profile.
var cimShortCircuitProps = map[string]bool{"ACLineSegment.r0": true, "ACLineSegment.x0": true, "ACLineSegment.b0ch": true,
	"ACLineSegment.g0ch": true, "BusbarSection.ipMax": true}

// shortCircuit reports whether the EQ document holds ShortCircuit profile data (cimShortCircuitProps).
func (x *cimExporter) shortCircuit() bool {
	for _, o := range x.eq {
		for _, p := range o.props {
			if cimShortCircuitProps[p.name] {
				return true
			}
		}
	}
	return false
}

// finish writes what CGMES expects of every object once all are made: Equipment.aggregate false on each
// equipment (each stands for one real item), and a name on each diagram object, its object's (or "<class> <n>").
func (x *cimExporter) finish() {
	names := map[string]string{}
	for _, o := range x.eq {
		container := false
		for _, p := range o.props {
			switch p.name {
			case "IdentifiedObject.name":
				names[o.id] = p.value
			case "Equipment.EquipmentContainer":
				container = true
			}
		}
		if container {
			o.lit("Equipment.aggregate", "false")
		}
	}
	for i, o := range x.dl {
		if o.class != "DiagramObject" && o.class != "TextDiagramObject" {
			continue
		}
		name, named := "", false
		for _, p := range o.props {
			switch p.name {
			case "IdentifiedObject.name":
				named = true
			case "DiagramObject.IdentifiedObject":
				name = names[p.value]
			}
		}
		if named {
			continue
		}
		if name == "" {
			name = fmt.Sprintf("%s %d", o.class, i+1)
		}
		// After the mRID, where the other objects have it.
		o.props = slices.Insert(o.props, min(1, len(o.props)), rdfProp{name: "IdentifiedObject.name", value: name, kind: 'l'})
	}
}

// id is the rdf:ID of the object with the given key.
func (x *cimExporter) id(key string) string {
	return "_" + x.mrid(key)
}

func (x *cimExporter) addEQ(class, key, name string) *rdfObject {
	o := (&rdfObject{class: class, id: x.id(key)}).lit("IdentifiedObject.mRID", x.mrid(key))
	if name != "" {
		o.lit("IdentifiedObject.name", name)
	}
	x.eq = append(x.eq, o)
	return o
}

func (x *cimExporter) warnf(kind, item string, id int, fields ...string) {
	x.warn = append(x.warn, CIMWarning{Kind: kind, Item: item, ID: id, Fields: fields})
}

// --- topology

func (x *cimExporter) find(n int) int {
	p, ok := x.parent[n]
	if !ok || p == n {
		return n
	}
	r := x.find(p)
	x.parent[n] = r
	return r
}

// union keeps the smaller node id as the root, so a ConnectivityNode's key does not depend on order.
func (x *cimExporter) union(a, b int) {
	ra, rb := x.find(a), x.find(b)
	if ra == rb {
		return
	}
	if rb < ra {
		ra, rb = rb, ra
	}
	x.parent[rb] = ra
}

func sortedPorts(e Element) []Port {
	ports := slices.Clone(e.Ports)
	slices.SortStableFunc(ports, func(a, b Port) int {
		ai, _ := strconv.Atoi(a.Name)
		bi, _ := strconv.Atoi(b.Name)
		return ai - bi
	})
	return ports
}

func (x *cimExporter) joinNodes() {
	for _, c := range x.d.Connectors {
		if c.Kind == KindBusWork || c.Kind == KindBusbarWire {
			x.union(c.From, c.To)
		}
	}
	for _, e := range x.d.Elements {
		switch cimClasses[e.Class].role {
		case rolePassThrough, roleAuxiliary, roleBusbar:
			for i := 1; i < len(e.Ports); i++ {
				x.union(e.Ports[0].Node, e.Ports[i].Node)
			}
		}
	}
	// A node's voltage: the most common voltage among what touches it, the smallest class id on a tie.
	votes := map[int]map[int]int{}
	vote := func(node, v int) {
		v = x.known(v)
		if v == 0 || node == 0 {
			return
		}
		r := x.find(node)
		if votes[r] == nil {
			votes[r] = map[int]int{}
		}
		votes[r][v]++
	}
	for _, c := range x.d.Connectors {
		vote(c.From, c.Voltage)
		vote(c.To, c.Voltage)
	}
	for _, e := range x.d.Elements {
		for i, p := range sortedPorts(e) {
			v := e.Voltage
			if e.Class == ClassPowerTransformer && i < len(e.Windings) {
				v = e.Windings[i].Voltage
			}
			vote(p.Node, v)
		}
	}
	for r, m := range votes {
		best, bestN := 0, 0
		for v, n := range m {
			if n > bestN || n == bestN && v < best {
				best, bestN = v, n
			}
		}
		x.cnVolt[r] = best
	}
	// A node with no voltage of its own takes that of a node any device but a transformer joins it to.
	var pairs [][2]int
	link := func(a, b int) {
		if a != 0 && b != 0 {
			pairs = append(pairs, [2]int{x.find(a), x.find(b)})
		}
	}
	for _, e := range x.d.Elements {
		if r := cimClasses[e.Class].role; r == roleTransformer || r == roleBooster {
			continue
		}
		for i := 1; i < len(e.Ports); i++ {
			link(e.Ports[0].Node, e.Ports[i].Node)
		}
	}
	for _, c := range x.d.Connectors {
		if c.Kind == KindOverheadLine || c.Kind == KindCableLine {
			link(c.From, c.To)
		}
	}
	for changed := true; changed; {
		changed = false
		for _, p := range pairs {
			a, b := x.cnVolt[p[0]], x.cnVolt[p[1]]
			switch {
			case a == 0 && b != 0:
				x.cnVolt[p[0]], changed = b, true
			case b == 0 && a != 0:
				x.cnVolt[p[1]], changed = a, true
			}
		}
	}
}

// known is voltage class v when its name holds a voltage, else 0 (reported once as a "voltage" warning).
func (x *cimExporter) known(v int) int {
	vc, ok := x.classes[v]
	if v == 0 || !ok {
		return 0
	}
	if _, ok := ParseVoltage(vc.Name); !ok {
		if !x.badClass[v] {
			x.badClass[v] = true
			x.warnf("voltage", "voltageClass", v)
		}
		return 0
	}
	return v
}

// substations groups the nodes into substations: whatever any device but a line joins is one substation.
func (x *cimExporter) substations() {
	find := func(r int) int {
		for x.subParent[r] != 0 && x.subParent[r] != r {
			r = x.subParent[r]
		}
		return r
	}
	join := func(a, b int) {
		ra, rb := find(x.find(a)), find(x.find(b))
		if ra == rb {
			return
		}
		if rb < ra {
			ra, rb = rb, ra
		}
		x.subParent[rb] = ra
	}
	for _, e := range x.d.Elements {
		if _, ok := cimClasses[e.Class]; !ok {
			continue
		}
		for i := 1; i < len(e.Ports); i++ {
			join(e.Ports[0].Node, e.Ports[i].Node)
		}
	}
	// A line end with no equipment around it (only lines, wires, poles and the like) is no substation of its own:
	// it joins the one at the line's other end, following chains of lines until nothing changes.
	equipped := map[int]bool{}
	for _, e := range x.d.Elements {
		if info, ok := cimClasses[e.Class]; ok && info.role != rolePassThrough {
			for _, p := range e.Ports {
				equipped[find(x.find(p.Node))] = true
			}
		}
	}
	for _, c := range x.d.Connectors {
		if c.Kind == KindLinkToObject {
			equipped[find(x.find(c.From))] = true
		}
	}
	for changed := true; changed; {
		changed = false
		for _, c := range x.d.Connectors {
			if c.Kind != KindOverheadLine && c.Kind != KindCableLine {
				continue
			}
			a, b := find(x.find(c.From)), find(x.find(c.To))
			if a == b || equipped[a] && equipped[b] {
				continue
			}
			join(c.From, c.To)
			equipped[find(a)] = equipped[a] || equipped[b]
			changed = true
		}
	}
	// Named after its busbar with the most connections, else its first named device.
	best := map[int]int{}
	for _, e := range x.d.Elements {
		if len(e.Ports) == 0 || e.Name == "" {
			continue
		}
		if _, ok := cimClasses[e.Class]; !ok {
			continue
		}
		s := find(x.find(e.Ports[0].Node))
		score := 0
		if e.Class == ClassBusBarSection {
			score = 1 + len(e.Ports)
		}
		if _, named := x.subName[s]; !named || score > best[s] {
			x.subName[s], best[s] = e.Name, score
		}
	}
	x.subFind = find
}

// substation returns the Substation of slddoc node n (0: the one for equipment with no connection), creating it.
func (x *cimExporter) substation(n int) string {
	s := 0
	if n != 0 {
		s = x.subFind(x.find(n))
	}
	if id, ok := x.subs[s]; ok {
		return id
	}
	name := x.subName[s]
	if name == "" {
		name = fmt.Sprintf("%s %d", x.modelName(), len(x.subs)+1)
	}
	key := fmt.Sprintf("substation:%d", s)
	x.addEQ("Substation", key, name).ref("Substation.Region", x.region)
	x.subs[s] = x.id(key)
	return x.subs[s]
}

// node returns the ConnectivityNode of slddoc node n, creating it on first use.
func (x *cimExporter) node(n int) string {
	r := x.find(n)
	if id, ok := x.cn[r]; ok {
		return id
	}
	key := fmt.Sprintf("cn:%d", r)
	container := x.voltageLevel(x.cnVolt[r], n)
	x.addEQ("ConnectivityNode", key, fmt.Sprintf("CN %d", r)).ref("ConnectivityNode.ConnectivityNodeContainer", container)
	x.cn[r] = x.id(key)
	return x.cn[r]
}

// --- containers and voltages

var voltageRe = regexp.MustCompile("(?i)([0-9]+(?:[.,][0-9]+)?)\\s*(kv|\u043a\u0432|mv|\u043c\u0432|v|\u0432)?")

// ParseVoltage reads a voltage class name such as "110 kV", "6,3 kV" or "400 V" as kV; the unit may also be in Cyrillic.
// A bare number is kV.
func ParseVoltage(name string) (float64, bool) {
	if strings.HasPrefix(strings.TrimSpace(name), "#") {
		return 0, false // a colour, not a voltage
	}
	m := voltageRe.FindStringSubmatch(name)
	if m == nil {
		return 0, false
	}
	v, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", "."), 64)
	if err != nil {
		return 0, false
	}
	switch strings.ToLower(m[2]) {
	case "mv", "\u043c\u0432":
		v *= 1000
	case "v", "\u0432":
		v /= 1000
	}
	return v, v > 0
}

func (x *cimExporter) containers() {
	name := x.modelName()
	gr := x.addEQ("GeographicalRegion", "region:geo", name)
	x.region = x.id("region:sub")
	x.addEQ("SubGeographicalRegion", "region:sub", name).ref("SubGeographicalRegion.Region", gr.id)
}

// baseVoltage returns the BaseVoltage of voltage class v ("" for none), creating it with its VoltageLevel.
func (x *cimExporter) baseVoltage(v int) string {
	if v == 0 {
		return ""
	}
	if id, ok := x.bv[v]; ok {
		return id
	}
	vc, ok := x.classes[v]
	if !ok {
		return ""
	}
	kv, ok := ParseVoltage(vc.Name)
	if !ok {
		x.warnf("voltage", "voltageClass", v)
	}
	x.nominal[v] = kv
	key := fmt.Sprintf("bv:%d", v)
	x.addEQ("BaseVoltage", key, vc.Name).num("BaseVoltage.nominalVoltage", kv)
	x.bv[v] = x.id(key)
	return x.bv[v]
}

// voltageLevel returns the VoltageLevel of voltage class v in the substation of slddoc node n, creating it. An
// unknown voltage takes the substation's most common one; with none at all, the Substation itself is returned.
func (x *cimExporter) voltageLevel(v, n int) string {
	v = x.resolve(v, n)
	s := 0
	if n != 0 {
		s = x.subFind(x.find(n))
	}
	if v == 0 {
		return x.substation(n)
	}
	if id, ok := x.vls[[2]int{s, v}]; ok {
		return id
	}
	key := fmt.Sprintf("vl:%d:%d", s, v)
	x.addEQ("VoltageLevel", key, x.classes[v].Name).ref("VoltageLevel.Substation", x.substation(n)).ref("VoltageLevel.BaseVoltage", x.baseVoltage(v))
	x.vls[[2]int{s, v}] = x.id(key)
	return x.vls[[2]int{s, v}]
}

// resolve is voltage class v when it holds a voltage, else that of slddoc node n, else the most common one of n's
// substation, else of the whole diagram (0 only when nothing in the diagram has a voltage).
func (x *cimExporter) resolve(v, n int) int {
	if v = x.known(v); v != 0 {
		return v
	}
	if n != 0 {
		r := x.find(n)
		if v := x.cnVolt[r]; v != 0 {
			return v
		}
		if v := x.mainVoltage(x.subFind(r), false); v != 0 {
			return v
		}
	}
	return x.mainVoltage(0, true)
}

// mainVoltage is the voltage class most nodes of substation s (of the whole diagram when all) have; 0 for none.
func (x *cimExporter) mainVoltage(s int, all bool) int {
	count := map[int]int{}
	for r, v := range x.cnVolt {
		if v != 0 && (all || x.subFind(r) == s) {
			count[v]++
		}
	}
	best, bestN := 0, 0
	for v, c := range count {
		if c > bestN || c == bestN && v < best {
			best, bestN = v, c
		}
	}
	return best
}

// firstNode is the node of e's first port (0 when it has none).
func firstNode(e Element) int {
	if ps := sortedPorts(e); len(ps) > 0 {
		return ps[0].Node
	}
	return 0
}

// voltageOf is an element's own voltage, else that of its ports' nodes, else resolve's fallback.
func (x *cimExporter) voltageOf(e Element) int {
	if v := x.known(e.Voltage); v != 0 {
		return v
	}
	for _, p := range sortedPorts(e) {
		if v := x.cnVolt[x.find(p.Node)]; v != 0 {
			return v
		}
	}
	return x.resolve(0, firstNode(e))
}

// --- equipment

func elementName(e Element) string {
	if e.Name != "" {
		return e.Name
	}
	return fmt.Sprintf("%s %d", e.Class, e.ID)
}

// terminal adds the Terminal number seq of equipment eq on slddoc node n, with its DL point.
func (x *cimExporter) terminal(eqKey, eqName string, seq, n int, layer int) string {
	key := fmt.Sprintf("%s:t:%d", eqKey, seq)
	x.addEQ("Terminal", key, fmt.Sprintf("%s T%d", eqName, seq)).
		ref("Terminal.ConductingEquipment", x.id(eqKey)).
		ref("Terminal.ConnectivityNode", x.node(n)).
		lit("ACDCTerminal.sequenceNumber", strconv.Itoa(seq))
	r := x.find(n)
	x.termsAt[r] = append(x.termsAt[r], x.id(key))
	if nd, ok := x.nodes[n]; ok {
		x.diagramObject(key, x.id(key), "", 0, layer, []Point{{nd.X, nd.Y}}, n)
	}
	return x.id(key)
}

// diagramObject adds a DiagramObject for object ref (any IdentifiedObject) with its points; glueNodes gives,
// per point, the slddoc node it is glued at (0 = none).
func (x *cimExporter) diagramObject(key, ref, style string, rotation, layer int, pts []Point, glueNodes ...int) {
	doKey := "do:" + key
	o := (&rdfObject{class: "DiagramObject", id: x.id(doKey)}).
		lit("IdentifiedObject.mRID", x.mrid(doKey)).
		ref("DiagramObject.Diagram", x.diagram).
		ref("DiagramObject.IdentifiedObject", ref).
		lit("DiagramObject.drawingOrder", strconv.Itoa(x.layerZ[layer])).
		lit("DiagramObject.isPolygon", "false")
	if rotation != 0 {
		o.num("DiagramObject.rotation", float64(rotation))
	}
	if style != "" {
		o.ref("DiagramObject.DiagramObjectStyle", x.style(style))
	}
	x.dl = append(x.dl, o)
	x.layerDOs[layer] = append(x.layerDOs[layer], o.id)
	x.points(doKey, o.id, pts, glueNodes)
}

func (x *cimExporter) points(doKey, doID string, pts []Point, glueNodes []int) {
	for i, p := range pts {
		pk := fmt.Sprintf("%s:p:%d", doKey, i+1)
		po := (&rdfObject{class: "DiagramObjectPoint", id: x.id(pk)}).
			ref("DiagramObjectPoint.DiagramObject", doID).
			lit("DiagramObjectPoint.sequenceNumber", strconv.Itoa(i+1)).
			num("DiagramObjectPoint.xPosition", p.X).
			num("DiagramObjectPoint.yPosition", p.Y)
		x.dl = append(x.dl, po)
		if i < len(glueNodes) && glueNodes[i] != 0 {
			x.glue[glueNodes[i]] = append(x.glue[glueNodes[i]], po)
		}
	}
}

func (x *cimExporter) style(name string) string {
	if id, ok := x.styles[name]; ok {
		return id
	}
	key := "style:" + name
	x.dl = append(x.dl, (&rdfObject{class: "DiagramObjectStyle", id: x.id(key)}).
		lit("IdentifiedObject.mRID", x.mrid(key)).lit("IdentifiedObject.name", name))
	x.styles[name] = x.id(key)
	return x.styles[name]
}

func normalizedRotation(orient int) int {
	return (orient%360 + 360) % 360
}

func shapeStyle(e Element) string {
	return "shape-" + e.Shape
}

// param returns *p, or adds field to missing and returns fallback.
func param(p *float64, fallback float64, field string, missing *[]string) float64 {
	if p != nil {
		return *p
	}
	*missing = append(*missing, field)
	return fallback
}

func elec(e *Electrical) Electrical {
	if e == nil {
		return Electrical{}
	}
	return *e
}

func (x *cimExporter) element(e Element) {
	info, ok := cimClasses[e.Class]
	if !ok {
		if len(e.Ports) > 0 && !cimDecorWithPorts[e.Class] {
			x.warnf("notExported", "element", e.ID)
		}
		return
	}
	key := fmt.Sprintf("el:%d", e.ID)
	name := elementName(e)
	ports := sortedPorts(e)
	if info.role != rolePassThrough && len(ports) > 0 && x.known(e.Voltage) == 0 && e.Class != ClassPowerTransformer {
		guessed := true
		for _, p := range ports {
			guessed = guessed && x.cnVolt[x.find(p.Node)] == 0
		}
		if guessed {
			x.warnf("guessedVoltage", "element", e.ID)
		}
	}
	at := []Point{{e.X, e.Y}}
	rot := normalizedRotation(e.Orient)
	switch info.role {
	case rolePassThrough:
		if len(ports) > 0 {
			x.diagramObject(key, x.node(ports[0].Node), shapeStyle(e), rot, e.Layer, at)
		}
	case roleAuxiliary, roleAuxOnPort1:
		if len(ports) == 0 {
			return
		}
		node := ports[0].Node
		o := x.addEQ(x.table.classOf(e), key, name).ref("Equipment.EquipmentContainer", x.voltageLevel(x.voltageOf(e), firstNode(e)))
		x.eqOf[e.ID] = o.id
		x.diagramObject(key, o.id, shapeStyle(e), rot, e.Layer, at)
		if info.role == roleAuxOnPort1 {
			x.node(node)
		}
		// The terminal it measures or protects is chosen once every equipment terminal exists.
		x.auxLater = append(x.auxLater, func() {
			if ts := x.termsAt[x.find(node)]; len(ts) > 0 {
				o.ref("AuxiliaryEquipment.Terminal", ts[0])
			} else {
				x.warnf("noTerminal", "element", e.ID)
			}
		})
	case roleBusbar:
		v := x.voltageOf(e)
		ee := elec(e.Electrical)
		o := x.addEQ("BusbarSection", key, name).ref("Equipment.EquipmentContainer", x.voltageLevel(v, firstNode(e)))
		if ee.IpMax != nil {
			o.num("BusbarSection.ipMax", *ee.IpMax)
		}
		x.eqOf[e.ID] = o.id
		if len(ports) > 0 {
			x.terminal(key, name, 1, ports[0].Node, e.Layer)
		}
		x.diagramObject(key, o.id, shapeStyle(e), 0, e.Layer, e.Points)
	case roleTransformer, roleBooster:
		x.transformer(e, key, name, ports, info.role == roleBooster)
	case roleKnife:
		x.knifeSwitch(e, key, name, ports)
	case roleShunt:
		x.shunt(e, key, name, ports)
	default:
		class := x.table.classOf(e)
		if switchClasses[class] && len(ports) >= 2 && x.find(ports[0].Node) == x.find(ports[1].Node) {
			x.warnf("shorted", "element", e.ID) // both ends on one node: no switch at all
			return
		}
		o := x.addEQ(class, key, name).ref("Equipment.EquipmentContainer", x.voltageLevel(x.voltageOf(e), firstNode(e)))
		x.eqOf[e.ID] = o.id
		x.equipmentParams(e, o, class)
		for i, p := range ports {
			x.terminal(key, name, i+1, p.Node, e.Layer)
		}
		x.diagramObject(key, o.id, shapeStyle(e), rot, e.Layer, at)
	}
}

var switchClasses = map[string]bool{"Breaker": true, "DisconnectingCircuitBreaker": true, "Disconnector": true,
	"LoadBreakSwitch": true, "GroundDisconnector": true, "Switch": true, "Fuse": true, "Jumper": true}

func switchOpen(e Element) bool {
	return e.State != nil && *e.State != 1
}

func (x *cimExporter) equipmentParams(e Element, o *rdfObject, class string) {
	ee := elec(e.Electrical)
	var missing []string
	optional := func(name string, v *float64) {
		if v != nil {
			o.num(name, *v)
		}
	}
	switch class {
	case "Breaker", "DisconnectingCircuitBreaker", "Disconnector", "LoadBreakSwitch", "GroundDisconnector", "Switch", "Fuse", "Jumper":
		o.lit("Switch.normalOpen", strconv.FormatBool(switchOpen(e))).lit("Switch.retained", "false")
		optional("Switch.ratedCurrent", ee.RatedCurrent)
	case "SeriesCompensator":
		o.num("SeriesCompensator.r", param(ee.R, 0, "r", &missing)).
			num("SeriesCompensator.x", param(ee.X, 0, "x", &missing))
	case "SynchronousMachine", "AsynchronousMachine":
		if class == "SynchronousMachine" {
			kind := map[Class]string{ClassGenerator: "generator", ClassSynchronousCompensator: "condenser", ClassSynchronousMotor: "motor"}[e.Class]
			o.enum("SynchronousMachine.type", "SynchronousMachineKind."+kind)
		}
		o.num("RotatingMachine.ratedS", param(ee.RatedS, 0, "ratedS", &missing))
		if ee.RatedU != nil {
			o.num("RotatingMachine.ratedU", *ee.RatedU)
		} else if kv := x.nominal[x.voltageOf(e)]; kv != 0 {
			o.num("RotatingMachine.ratedU", kv)
		}
		if class == "SynchronousMachine" {
			optional("SynchronousMachine.minQ", ee.MinQ)
			optional("SynchronousMachine.maxQ", ee.MaxQ)
		}
		if e.Class == ClassGenerator {
			guKey := fmt.Sprintf("el:%d:gu", e.ID)
			gu := x.addEQ("GeneratingUnit", guKey, elementName(e)).
				num("GeneratingUnit.minOperatingP", param(ee.MinP, 0, "minP", &missing)).
				num("GeneratingUnit.maxOperatingP", param(ee.MaxP, 0, "maxP", &missing)).
				ref("Equipment.EquipmentContainer", x.voltageLevel(x.voltageOf(e), firstNode(e)))
			o.ref("RotatingMachine.GeneratingUnit", gu.id)
		}
	case "EnergyConsumer", "ConformLoad", "NonConformLoad":
		optional("EnergyConsumer.pfixed", ee.P)
		optional("EnergyConsumer.qfixed", ee.Q)
	case "ExternalNetworkInjection", "EquivalentInjection":
		if class == "ExternalNetworkInjection" {
			o.num(class+".minP", param(ee.MinP, 0, "minP", &missing)).num(class+".maxP", param(ee.MaxP, 0, "maxP", &missing))
		} else {
			optional(class+".minP", ee.MinP)
			optional(class+".maxP", ee.MaxP)
		}
		optional(class+".minQ", ee.MinQ)
		optional(class+".maxQ", ee.MaxQ)
		if class == "EquivalentInjection" {
			o.lit("EquivalentInjection.regulationCapability", "false")
		}
	case "EnergySource":
		if kv := x.nominal[x.voltageOf(e)]; kv != 0 {
			o.num("EnergySource.nominalVoltage", kv)
		}
	case "StaticVarCompensator":
		o.num("StaticVarCompensator.capacitiveRating", param(ee.CapacitiveRating, 0, "capacitiveRating", &missing)).
			num("StaticVarCompensator.inductiveRating", param(ee.InductiveRating, 0, "inductiveRating", &missing)).
			num("StaticVarCompensator.slope", param(ee.Slope, 0, "slope", &missing))
	}
	if len(missing) > 0 {
		x.warnf("missing", "element", e.ID, missing...)
	}
}

func (x *cimExporter) shunt(e Element, key, name string, ports []Port) {
	if x.table.classOf(e) == "PetersenCoil" {
		x.petersenCoil(e, key, name, ports)
		return
	}
	v := x.voltageOf(e)
	ee := elec(e.Electrical)
	var missing []string
	var nomU float64
	if ee.NomU != nil {
		nomU = *ee.NomU
	} else {
		x.baseVoltage(v)
		nomU = x.nominal[v]
		missing = append(missing, "nomU")
	}
	maxSec, normSec := 1, 1
	if ee.MaximumSections != nil {
		maxSec = *ee.MaximumSections
	} else {
		missing = append(missing, "maximumSections")
	}
	if ee.NormalSections != nil {
		normSec = *ee.NormalSections
	} else {
		missing = append(missing, "normalSections")
	}
	o := x.addEQ("LinearShuntCompensator", key, name).
		ref("Equipment.EquipmentContainer", x.voltageLevel(v, firstNode(e))).
		num("ShuntCompensator.nomU", nomU).
		lit("ShuntCompensator.maximumSections", strconv.Itoa(maxSec)).
		lit("ShuntCompensator.normalSections", strconv.Itoa(normSec)).
		num("LinearShuntCompensator.bPerSection", param(ee.BPerSection, 0, "bPerSection", &missing))
	if ee.GPerSection != nil {
		o.num("LinearShuntCompensator.gPerSection", *ee.GPerSection)
	}
	x.eqOf[e.ID] = o.id
	if len(ports) > 0 {
		x.terminal(key, name, 1, ports[0].Node, e.Layer)
	}
	x.diagramObject(key, o.id, shapeStyle(e), normalizedRotation(e.Orient), e.Layer, []Point{{e.X, e.Y}})
	if len(missing) > 0 {
		x.warnf("missing", "element", e.ID, missing...)
	}
}

// petersenCoil exports a shunt reactor chosen as an arc suppression coil: fixed mode, its nominal voltage
// from nomU or the voltage class.
func (x *cimExporter) petersenCoil(e Element, key, name string, ports []Port) {
	v := x.voltageOf(e)
	x.baseVoltage(v)
	ee := elec(e.Electrical)
	var missing []string
	o := x.addEQ("PetersenCoil", key, name).
		ref("Equipment.EquipmentContainer", x.voltageLevel(v, firstNode(e))).
		enum("PetersenCoil.mode", "PetersenCoilModeKind.fixed").
		num("PetersenCoil.nominalU", param(ee.NomU, x.nominal[v], "nomU", &missing))
	x.eqOf[e.ID] = o.id
	if len(ports) > 0 {
		x.terminal(key, name, 1, ports[0].Node, e.Layer)
	}
	x.diagramObject(key, o.id, shapeStyle(e), normalizedRotation(e.Orient), e.Layer, []Point{{e.X, e.Y}})
	if len(missing) > 0 {
		x.warnf("missing", "element", e.ID, missing...)
	}
}

// knifeSwitch exports a changeover knife switch (pivot port 1, contacts 2 and 3) as two Disconnectors from
// the pivot; State picks the closed one (KnifeSwitch: unset/0 left, 2 right; KnifeSwitch3: 0 left, 2 right,
// unset/1 neither).
func (x *cimExporter) knifeSwitch(e Element, key, name string, ports []Port) {
	if len(ports) < 3 {
		return
	}
	state := -1
	if e.State != nil {
		state = *e.State
	}
	left := state == 0 || e.Class == ClassKnifeSwitch && state == -1
	right := state == 2
	v := x.voltageOf(e)
	ee := elec(e.Electrical)
	for i, side := range []struct {
		suffix string
		port   Port
		closed bool
	}{{"L", ports[1], left}, {"R", ports[2], right}} {
		k := fmt.Sprintf("%s:%s", key, side.suffix)
		n := name + " " + side.suffix
		o := x.addEQ(x.table.classOf(e), k, n).ref("Equipment.EquipmentContainer", x.voltageLevel(v, firstNode(e))).
			lit("Switch.normalOpen", strconv.FormatBool(!side.closed)).lit("Switch.retained", "false")
		if ee.RatedCurrent != nil {
			o.num("Switch.ratedCurrent", *ee.RatedCurrent)
		}
		if i == 0 {
			x.eqOf[e.ID] = o.id
		}
		x.terminal(k, n, 1, ports[0].Node, e.Layer)
		x.terminal(k, n, 2, side.port.Node, e.Layer)
		x.diagramObject(k, o.id, shapeStyle(e), normalizedRotation(e.Orient), e.Layer, []Point{{e.X, e.Y}})
	}
}

var windingConnection = map[WindingScheme]string{SchemeWye: "Y", SchemeWyeN: "Yn", SchemeDelta: "D"}

func (x *cimExporter) transformer(e Element, key, name string, ports []Port, booster bool) {
	o := x.addEQ("PowerTransformer", key, name).ref("Equipment.EquipmentContainer", x.substation(firstNode(e)))
	if len(e.Windings) > 3 {
		x.warnf("windings", "element", e.ID) // valid CIM, but many tools read only 2 or 3 ends
	}
	x.eqOf[e.ID] = o.id
	windings := e.Windings
	if booster {
		ee := elec(e.Electrical)
		w := TransformerWinding{Voltage: e.Voltage, RatedU: ee.RatedU, RatedS: ee.RatedS, R: ee.R, X: ee.X, Scheme: SchemeWye}
		zero := 0.0
		windings = []TransformerWinding{w, {Voltage: e.Voltage, RatedU: ee.RatedU, RatedS: ee.RatedS, R: &zero, X: &zero, G: &zero, B: &zero, Scheme: SchemeWye}}
	}
	var missing []string
	for i, w := range windings {
		if i >= len(ports) {
			break
		}
		var miss []string
		v := x.resolve(w.Voltage, ports[i].Node)
		bv := x.baseVoltage(v)
		endKey := fmt.Sprintf("%s:end:%d", key, i+1)
		t := x.terminal(key, name, i+1, ports[i].Node, e.Layer)
		end := x.addEQ("PowerTransformerEnd", endKey, fmt.Sprintf("%s W%d", name, i+1)).
			ref("PowerTransformerEnd.PowerTransformer", o.id).
			ref("TransformerEnd.Terminal", t).
			ref("TransformerEnd.BaseVoltage", bv).
			lit("TransformerEnd.endNumber", strconv.Itoa(i+1)).
			num("PowerTransformerEnd.ratedU", param(w.RatedU, x.nominal[v], "ratedU", &miss))
		if w.RatedS != nil {
			end.num("PowerTransformerEnd.ratedS", *w.RatedS)
		} else {
			miss = append(miss, "ratedS")
		}
		end.num("PowerTransformerEnd.r", param(w.R, 0, "r", &miss)).
			num("PowerTransformerEnd.x", param(w.X, 0, "x", &miss)).
			num("PowerTransformerEnd.g", param(w.G, 0, "g", &miss)).
			num("PowerTransformerEnd.b", param(w.B, 0, "b", &miss))
		conn, ok := windingConnection[w.Scheme]
		if !ok {
			conn = "Y"
			miss = append(miss, "connectionKind")
		}
		end.enum("PowerTransformerEnd.connectionKind", "WindingConnection."+conn)
		if w.PhaseAngleClock != nil {
			end.lit("PowerTransformerEnd.phaseAngleClock", strconv.Itoa(*w.PhaseAngleClock))
		}
		if !booster || i == 0 {
			for _, m := range miss {
				f := m
				if !booster {
					f = fmt.Sprintf("%d:%s", i+1, m)
				}
				missing = append(missing, f)
			}
		}
	}
	x.diagramObject(key, o.id, shapeStyle(e), normalizedRotation(e.Orient), e.Layer, []Point{{e.X, e.Y}})
	if len(missing) > 0 {
		x.warnf("missing", "element", e.ID, missing...)
	}
}

func (x *cimExporter) connector(c Connector) {
	switch c.Kind {
	case KindBusWork, KindBusbarWire:
		glue := make([]int, len(c.Points))
		if len(glue) > 0 {
			glue[0], glue[len(glue)-1] = c.From, c.To
		}
		x.diagramObject(fmt.Sprintf("wire:%d", c.ID), x.node(c.From), string(c.Kind), 0, c.Layer, c.Points, glue...)
	case KindOverheadLine, KindCableLine:
		x.line(c)
	case KindLinkToObject:
		x.objectLink(c)
	}
}

// objectLink exports a link to an outside object as an EnergyConsumer (or its chosen EnergySource) with one
// Terminal on the node at its From end; the arrow end is the object itself. Its layout is the link's points, styled
// "LinkToObject".
func (x *cimExporter) objectLink(c Connector) {
	classes := x.table.linkClasses()
	class := classes[0]
	if slices.Contains(classes, c.CIM) {
		class = c.CIM
	}
	key := fmt.Sprintf("link:%d", c.ID)
	name := c.Name
	if name == "" {
		name = fmt.Sprintf("%s %d", c.Kind, c.ID)
	}
	v := x.resolve(c.Voltage, c.From)
	x.baseVoltage(v)
	o := x.addEQ(class, key, name).ref("Equipment.EquipmentContainer", x.voltageLevel(v, c.From))
	ee := elec(c.Electrical)
	switch class {
	case "EnergyConsumer":
		if ee.P != nil {
			o.num("EnergyConsumer.pfixed", *ee.P)
		}
		if ee.Q != nil {
			o.num("EnergyConsumer.qfixed", *ee.Q)
		}
	case "EnergySource":
		if kv := x.nominal[v]; kv != 0 {
			o.num("EnergySource.nominalVoltage", kv)
		}
	}
	x.eqOf[c.ID] = o.id
	x.terminal(key, name, 1, c.From, c.Layer)
	glue := make([]int, len(c.Points))
	if len(glue) > 0 {
		glue[0] = c.From
	}
	x.diagramObject(key, o.id, string(KindLinkToObject), 0, c.Layer, c.Points, glue...)
	x.auxLater = append(x.auxLater, func() {
		if x.used[c.To] > 1 {
			x.warnf("openEnd", "connector", c.ID)
		}
	})
}

func (x *cimExporter) line(c Connector) {
	if x.find(c.From) == x.find(c.To) {
		x.warnf("shorted", "connector", c.ID) // both ends on one node: nothing to model
		return
	}
	if x.known(c.Voltage) == 0 && x.cnVolt[x.find(c.From)] == 0 && x.cnVolt[x.find(c.To)] == 0 {
		x.warnf("guessedVoltage", "connector", c.ID)
	}
	key := fmt.Sprintf("line:%d", c.ID)
	name := c.Name
	if name == "" {
		name = fmt.Sprintf("%s %d", c.Kind, c.ID)
	}
	v := x.resolve(c.Voltage, c.From)
	lineID := x.addEQ("Line", key+":container", name).ref("Line.Region", x.region).id
	ee := elec(c.Electrical)
	var missing []string
	segKey := key
	o := x.addEQ("ACLineSegment", segKey, name).
		ref("Equipment.EquipmentContainer", lineID).
		ref("ConductingEquipment.BaseVoltage", x.baseVoltage(v)).
		num("ACLineSegment.r", param(ee.R, 0, "r", &missing)).
		num("ACLineSegment.x", param(ee.X, 0, "x", &missing)).
		num("ACLineSegment.bch", param(ee.Bch, 0, "bch", &missing)).
		num("Conductor.length", param(ee.Length, 0, "length", &missing))
	for _, f := range []struct {
		name string
		v    *float64
	}{{"gch", ee.Gch}, {"r0", ee.R0}, {"x0", ee.X0}, {"b0ch", ee.B0ch}, {"g0ch", ee.G0ch}} {
		if f.v != nil {
			o.num("ACLineSegment."+f.name, *f.v)
		}
	}
	x.eqOf[c.ID] = o.id
	x.terminal(segKey, name, 1, c.From, c.Layer)
	x.terminal(segKey, name, 2, c.To, c.Layer)
	glue := make([]int, len(c.Points))
	if len(glue) > 0 {
		glue[0], glue[len(glue)-1] = c.From, c.To
	}
	x.diagramObject(segKey, o.id, string(c.Kind), 0, c.Layer, c.Points, glue...)
	if len(missing) > 0 {
		x.warnf("missing", "connector", c.ID, missing...)
	}
}

// --- labels, glue points, layers

func (x *cimExporter) labels() {
	for _, l := range x.d.Labels {
		key := fmt.Sprintf("label:%d", l.ID)
		o := (&rdfObject{class: "TextDiagramObject", id: x.id(key)}).
			lit("IdentifiedObject.mRID", x.mrid(key)).
			ref("DiagramObject.Diagram", x.diagram).
			ref("DiagramObject.IdentifiedObject", x.eqOf[l.For]).
			lit("DiagramObject.drawingOrder", strconv.Itoa(x.layerZ[l.Layer])).
			lit("TextDiagramObject.text", l.Text)
		x.dl = append(x.dl, o)
		x.layerDOs[l.Layer] = append(x.layerDOs[l.Layer], o.id)
		x.points(key, o.id, []Point{{l.X, l.Y}}, nil)
	}
}

// gluePoints joins the DL points lying on one slddoc node, when there are at least two.
func (x *cimExporter) gluePoints() {
	ids := make([]int, 0, len(x.glue))
	for n := range x.glue {
		ids = append(ids, n)
	}
	slices.Sort(ids)
	for _, n := range ids {
		pts := x.glue[n]
		if len(pts) < 2 {
			continue
		}
		key := fmt.Sprintf("glue:%d", n)
		x.dl = append(x.dl, &rdfObject{class: "DiagramObjectGluePoint", id: x.id(key)})
		for _, p := range pts {
			p.ref("DiagramObjectPoint.DiagramObjectGluePoint", x.id(key))
		}
	}
}

func (x *cimExporter) layers() {
	for _, l := range x.d.Layers {
		dos := x.layerDOs[l.ID]
		if len(dos) == 0 {
			continue
		}
		key := fmt.Sprintf("layer:%d", l.ID)
		o := (&rdfObject{class: "VisibilityLayer", id: x.id(key)}).
			lit("IdentifiedObject.mRID", x.mrid(key)).
			lit("IdentifiedObject.name", l.Name).
			lit("VisibilityLayer.drawingOrder", strconv.Itoa(l.Z))
		for _, id := range dos {
			o.ref("VisibilityLayer.VisibleObjects", id)
		}
		x.dl = append(x.dl, o)
	}
}

// --- RDF/XML

func xmlText(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func (x *cimExporter) document(model string, profiles []string, dependsOn string, objs []*rdfObject) []byte {
	var b bytes.Buffer
	created := x.opt.Created.UTC().Format("2006-01-02T15:04:05Z")
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?>` + "\n")
	b.WriteString(`<rdf:RDF xmlns:cim="` + cimNS + `" xmlns:md="http://iec.ch/TC57/61970-552/ModelDescription/1#" xmlns:eu="http://iec.ch/TC57/CIM100-European#" xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">` + "\n")
	fmt.Fprintf(&b, "  <md:FullModel rdf:about=\"urn:uuid:%s\">\n", model)
	fmt.Fprintf(&b, "    <md:Model.created>%s</md:Model.created>\n", created)
	fmt.Fprintf(&b, "    <md:Model.scenarioTime>%s</md:Model.scenarioTime>\n", created)
	fmt.Fprintf(&b, "    <md:Model.description>%s</md:Model.description>\n", xmlText(x.modelName()))
	fmt.Fprintf(&b, "    <md:Model.modelingAuthoritySet>%s</md:Model.modelingAuthoritySet>\n", cimAuthoritySet)
	for _, profile := range profiles {
		fmt.Fprintf(&b, "    <md:Model.profile>%s</md:Model.profile>\n", profile)
	}
	b.WriteString("    <md:Model.version>1</md:Model.version>\n")
	if dependsOn != "" {
		fmt.Fprintf(&b, "    <md:Model.DependentOn rdf:resource=\"urn:uuid:%s\"/>\n", dependsOn)
	}
	b.WriteString("  </md:FullModel>\n")
	for _, o := range objs {
		fmt.Fprintf(&b, "  <cim:%s rdf:ID=\"%s\">\n", o.class, o.id)
		for _, p := range o.props {
			switch p.kind {
			case 'r':
				fmt.Fprintf(&b, "    <cim:%s rdf:resource=\"#%s\"/>\n", p.name, p.value)
			case 'e':
				fmt.Fprintf(&b, "    <cim:%s rdf:resource=\"%s\"/>\n", p.name, p.value)
			default:
				fmt.Fprintf(&b, "    <cim:%s>%s</cim:%s>\n", p.name, xmlText(p.value), p.name)
			}
		}
		fmt.Fprintf(&b, "  </cim:%s>\n", o.class)
	}
	b.WriteString("</rdf:RDF>\n")
	return b.Bytes()
}

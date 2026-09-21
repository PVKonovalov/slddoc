package slddoc

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
)

// BaseLayer is the implicit layer every element belongs to when a diagram
// carries no other layer for it. It is always present in a Diagram's Layers
// so "one element, exactly one layer" holds without exception.
const BaseLayer = 0

// Diagram is the root of the SLD document model.
type Diagram struct {
	XMLName xml.Name `xml:"diagram" json:"-"`

	Width  float64 `xml:"width,attr" json:"width"`
	Height float64 `xml:"height,attr" json:"height"`
	Source string  `xml:"source,attr,omitempty" json:"source,omitempty"`

	// LastID is the highest auto-assigned integer id this editor has ever
	// handed out for an Element/Node/Connector/VoltageClass in this diagram
	// (a single shared counter, not one per kind). The editor increments
	// and persists it here rather than deriving a fresh id from scratch
	// each time, so ids stay stable and never collide across a save/reopen
	// — see the frontend's diagramOps.ts, which is the only place that
	// actually assigns one. Every id is a positive integer (IdSequence
	// starts counting at 1), so 0 doubles as a safe "unset" sentinel for
	// every optional id-reference field below (Element.Voltage, Label.For,
	// ...) without needing a separate pointer/nullable type.
	LastID int `xml:"lastId,attr,omitempty" json:"lastId,omitempty"`

	// Editor holds this editor's own per-diagram preferences (grid
	// spacing/snap/background). Absent (nil) for a diagram that has never
	// been saved by this editor, or one produced by sld-svg's own tooling;
	// callers fall back to the server's configured defaults in that case.
	Editor *EditorSettings `xml:"editor,omitempty" json:"editor,omitempty"`

	Layers         []Layer         `xml:"layers>layer" json:"layers"`
	VoltageClasses []VoltageClass  `xml:"voltageClasses>class" json:"voltageClasses"`
	Nodes          []Node          `xml:"nodes>node" json:"nodes"`
	Elements       []Element       `xml:"elements>element" json:"elements"`
	Connectors     []Connector     `xml:"connectors>connector" json:"connectors"`
	Labels         []Label         `xml:"labels>label" json:"labels"`
	DigitalDevices []DigitalDevice `xml:"digitalDevices>digitalDevice" json:"digitalDevices"`
}

// EditorSettings is a diagram's own saved editing preferences. It has no
// effect on rendering an already-placed Element/Connector — only on the
// canvas UI (grid overlay, snapping, background), and, for DefaultVoltage,
// on what a *newly* placed one starts out with.
type EditorSettings struct {
	GridSpacing float64 `xml:"gridSpacing,attr,omitempty" json:"gridSpacing,omitempty"`
	Snap        bool    `xml:"snap,attr,omitempty" json:"snap,omitempty"`
	// ShowGrid was previously frontend-only (read/written via this same
	// JSON shape, but with no matching Go field) — silently lost on every
	// save/reload despite the Settings panel's own checkbox for it. Added
	// here so it actually round-trips like every other field on this
	// struct.
	ShowGrid   bool   `xml:"showGrid,attr,omitempty" json:"showGrid,omitempty"`
	Background string `xml:"background,attr,omitempty" json:"background,omitempty"`
	// DefaultVoltage references a VoltageClass.ID (0 means unset) this
	// diagram's own newly placed elements/connectors should start out
	// with, instead of no voltage at all — set from the New Diagram
	// dialog, or changed later in the Settings panel. Never assigned or
	// interpreted by the backend itself; purely round-tripped, the same
	// way Diagram.LastID is.
	DefaultVoltage int `xml:"defaultVoltage,attr,omitempty" json:"defaultVoltage,omitempty"`
	// ShowNodes toggles a debug overlay (Canvas: a small red X at every
	// Diagram.Node's own position, not just a symbol's declared
	// Terminals) — lets a diagram author see the real electrical graph
	// (where connectors/ports actually land) independent of what a
	// symbol's drawn geometry suggests.
	ShowNodes bool `xml:"showNodes,attr,omitempty" json:"showNodes,omitempty"`
}

// Layer is one entry of a diagram's visibility layers. A viewer toggles
// elements on and off by Layer.ID; every Element/Connector/Label carries
// exactly one Layer reference.
type Layer struct {
	ID   int    `xml:"id,attr" json:"id"`
	Name string `xml:"name,attr" json:"name"`
}

// VoltageClass maps a logical, real-world voltage level (e.g. "10 kV") to the
// color used to draw it in this diagram. Voltage is a logical attribute of
// an Element/Connector, not a color — VoltageClass is the only place a color
// is recorded, purely for rendering.
type VoltageClass struct {
	ID    int    `xml:"id,attr" json:"id"`
	Name  string `xml:"name,attr" json:"name"`
	Color string `xml:"color,attr" json:"color"`
}

// Node is an electrical junction: every Port and Connector endpoint that
// shares a Node.ID is electrically connected.
type Node struct {
	ID int     `xml:"id,attr" json:"id"`
	X  float64 `xml:"x,attr" json:"x"`
	Y  float64 `xml:"y,attr" json:"y"`
}

// Port is one electrical terminal of an Element, in the element's own local
// (pre-rotation) coordinate space, referencing the Node it resolves to.
type Port struct {
	Name string `xml:"name,attr" json:"name"`
	Node int    `xml:"node,attr" json:"node"`
}

// Class names an Element's real-world equipment kind, independent of the
// symbol library Shape code used to draw it.
type Class string

const (
	ClassBreaker         Class = "Breaker"
	ClassDisconnector    Class = "Disconnector"
	ClassSectionalizer   Class = "Sectionalizer"
	ClassLoadBreakSwitch Class = "LoadBreakSwitch"
	ClassGroundSwitch    Class = "GroundSwitch"
	// ClassShortCircuiter (shape 398) is a single-terminal grounding-type
	// switching device, structurally close to ClassGroundSwitch: a fixed
	// tapered earth symbol at the top, a single real electrical terminal
	// at the bottom, and a State-driven pivot rod (plus a small filled
	// arrowhead) bridging the gap between them — Closed bridges the
	// terminal straight to the earth symbol (an intentional short to
	// ground), Open pivots the rod away at the top, same pivot-circle
	// convention as ClassSectionalizer. Only two real states exist (no
	// Intermediate), same as ClassSectionalizer.
	ClassShortCircuiter     Class = "ShortCircuiter"
	ClassGround             Class = "Ground"
	ClassPowerTransformer   Class = "PowerTransformer"
	ClassCurrentTransformer Class = "CurrentTransformer"
	ClassVoltageTransformer Class = "VoltageTransformer"
	ClassChokeCoil          Class = "ChokeCoil"
	ClassReactor            Class = "Reactor"
	ClassReactorShunt       Class = "ReactorShunt"
	ClassSurgeArrester      Class = "SurgeArrester"
	ClassFuse               Class = "Fuse"
	ClassCapacitor          Class = "Capacitor"
	ClassCapacitorBank      Class = "CapacitorBank"
	ClassHalfChassis        Class = "HalfChassis"
	ClassChassis            Class = "Chassis"
	ClassStarter            Class = "Starter"
	ClassGenerator          Class = "Generator"
	ClassBusBarSection      Class = "BusBarSection"
	ClassJunctionPoint      Class = "JunctionPoint"
	ClassNonIntersection    Class = "NonIntersection"
	// ClassCableConnector (shape 56) is a real two-terminal electrical
	// device — a cable termination/splice symbol, not a decorative
	// annotation — drawn from a plain fixed local-coordinate template
	// (an open chevron flare at each end of its own stem) the same way
	// ClassJunctionPoint/ClassNonIntersection are, with real terminals at
	// (0,-10)/(0,10) rotated by its own Orient, and no State of its own
	// (real xsde2svg's own element56 never emits data-state).
	ClassCableConnector Class = "CableConnector"
	// ClassCableJoint (shape 32) is a real two-terminal electrical device —
	// a cable joint/coupling marking where two cable segments are spliced —
	// drawn from a plain fixed local-coordinate template (a vertical stem
	// split by a gap, with an unfilled triangle mark in the gap) the same
	// way ClassCableConnector is, with real terminals at (0,-12)/(0,14) —
	// asymmetric around the anchor by design, matching the real source's
	// own geometry exactly (element_32.go's own default branch draws its
	// triangle one unit below the element's true anchor, confirmed against
	// every real corpus instance found — 500+ across 17 files, all using
	// this same plain look). The real source also has an alternate
	// CustomView appearance, selected by a distinct non-English string
	// value on that field (a single line plus a differently-shaped
	// triangle), and an optional phase-color fill (yellow/green/red, or a
	// configured color) on the triangle — neither has any real corpus
	// instance to confirm against, so neither is modeled here; a real
	// instance using either would extract with this class's own plain look
	// instead. No State of its own (real xsde2svg's own element32 never
	// emits data-state for this shape).
	ClassCableJoint            Class = "CableJoint"
	ClassLamp                  Class = "Lamp"
	ClassFaultPassageIndicator Class = "FaultPassageIndicator"
	// ClassRectangle (shape 3) is a purely decorative annotation box — not
	// real electrical equipment, so unlike every class above it never has
	// Ports/a Voltage/a State of its own and never takes part in the
	// electrical topology (Extract never gives it a Port, and this
	// editor's own connectElements/routing refuse to treat one as a valid
	// endpoint). Its geometry is its own drawn Points (two opposite
	// corners), the same convention ClassBusBarSection already uses, not
	// a fixed local-coordinate template.
	ClassRectangle Class = "Rectangle"
	// ClassArrow (shape 2) is a decorative annotation line with an open
	// chevron arrowhead at one or both ends — same non-electrical status
	// as ClassRectangle (no Ports/Voltage/State, never a connectElements/
	// routing endpoint). Its geometry is its own drawn Points (start,
	// then end — order matters here, unlike a Rectangle's, since the
	// arrowhead is always drawn at the second point, or both when
	// DoubleHeaded).
	ClassArrow Class = "Arrow"
	// ClassCircle (shape 4) is a decorative annotation ellipse — same
	// non-electrical status and same two-opposite-corners Points
	// convention as ClassRectangle (order-independent, unlike
	// ClassArrow's), just rendered as an <ellipse> instead of a <rect>.
	ClassCircle Class = "Circle"
	// ClassPackageSubstation (shape 385) is a facility-level pictogram
	// (Package transformer substation, KTP) — not switchgear in the usual
	// sense, but the real source still gives it a genuine voltage-driven
	// color and exactly one real electrical terminal (see Element.NType's
	// own doc comment for its two real appearance variants, and
	// writePackageSubstation for the full geometry).
	ClassPackageSubstation Class = "PackageSubstation"
	// ClassEnclosedSubstation (shape 386) is the same kind of
	// facility-level pictogram as ClassPackageSubstation (Enclosed
	// transformer substation, ZTP) — a genuine voltage-driven color and
	// one real electrical terminal, but a single fixed appearance rather
	// than two real variants: a 36-unit square outline with a
	// downward-pointing triangle always drawn inside it (see
	// writeEnclosedSubstation for the full geometry).
	ClassEnclosedSubstation Class = "EnclosedSubstation"
)

// Element is one placed piece of equipment.
type Element struct {
	ID    int   `xml:"id,attr" json:"id"`
	Class Class `xml:"class,attr" json:"class"`
	// Shape keys the symbol library template used to draw this element
	// (see internal/elements). Two shapes can share the same electrical
	// Class (a fixed and a withdrawable breaker are both ClassBreaker) but
	// need different templates, so the renderer looks symbols up by Shape,
	// not by Class. Unlike every other id-shaped field here, Shape is a
	// symbol-library key, not a sequentially assigned identity, so it
	// stays a string.
	Shape string `xml:"shape,attr" json:"shape"`
	Name  string `xml:"name,attr,omitempty" json:"name,omitempty"`
	// Voltage references a VoltageClass.ID; 0 means unassigned.
	Voltage int `xml:"voltage,attr,omitempty" json:"voltage,omitempty"`
	// Layer references a Layer.ID; always set (defaults to BaseLayer).
	Layer int `xml:"layer,attr" json:"layer"`

	// X, Y is the element's anchor: its rotation center for a rotated
	// symbol, or the midpoint of its ports otherwise.
	X float64 `xml:"x,attr" json:"x"`
	Y float64 `xml:"y,attr" json:"y"`
	// Orient is the rotation applied to the symbol template around (X,Y),
	// in degrees (0, 90, 180, -90).
	Orient int `xml:"orient,attr,omitempty" json:"orient,omitempty"`
	// Mirror flips the symbol template horizontally (in its own local,
	// unrotated frame — applied before Orient's own rotation, matching the
	// real xsde2svg source's own xMirror convention) around (X,Y). Unlike
	// xsde2svg, which resolves it per equipment type or per placed instance
	// from the source .xsde data, this schema has no such upstream concept
	// to read it from — Extract never sets it, every already-extracted
	// element defaults to false/unmirrored — it exists purely as an
	// editor-side property a user can toggle after placement.
	Mirror bool `xml:"mirror,attr,omitempty" json:"mirror,omitempty"`
	// State carries an element's status (e.g. breaker open/closed), when
	// one applies to this class.
	State *int `xml:"state,attr,omitempty" json:"state,omitempty"`
	// Position carries a withdrawable device's own racking position
	// (Service/Normal/Test — see config.Config's PositionStates), a status
	// axis independent of State: a withdrawable breaker/disconnector can be
	// closed-and-racked-in, open-and-racked-in, or racked-out/test entirely
	// regardless of its own open/closed State. Only shapes 43/49 (Breaker/
	// Disconnector, withdrawable) currently use it.
	Position *int `xml:"position,attr,omitempty" json:"position,omitempty"`
	// FillOff/FillOn are a Lamp's (shape 106) two display colors, chosen by
	// State. Unlike the switch-like devices' state indicator (a fixed
	// red/lawngreen/yellow convention), a lamp's colors are chosen per
	// instance and carry real meaning, so they're recorded rather than
	// reduced to that convention.
	FillOff string `xml:"fillOff,attr,omitempty" json:"fillOff,omitempty"`
	FillOn  string `xml:"fillOn,attr,omitempty" json:"fillOn,omitempty"`
	// Radius is a Lamp's (shape 106) or FaultPassageIndicator's (shape
	// 320003) drawn circle radius; unlike other shapes' fixed template
	// geometry, these are meaningfully different per instance. Also used by
	// JunctionPoint (shape 7) for its own drawn dot — real corpus shows a
	// genuinely varying radius there too (2/3/4/5/8/11 all seen), unlike
	// this schema's own previous hardcoded 3. 0/unset falls back to that
	// same 3 (Render's own default, preserved so an already-placed/-saved
	// junction point's look doesn't change), not literally invisible the
	// way an unset Lamp radius would be — Extract always sets this
	// explicitly from a real instance's own r attribute instead of relying
	// on that fallback.
	Radius float64 `xml:"radius,attr,omitempty" json:"radius,omitempty"`
	// Fill is a Rectangle's (shape 3) or Circle's (shape 4) own interior
	// color — free-text like Lamp's own FillOff/FillOn, not a VoltageClass
	// reference, since a decorative annotation shape has no electrical
	// voltage of its own to resolve one from. Empty means "none"
	// (transparent), matching the real xsde2svg source's own default.
	// Unused by an Arrow (shape 2, stroke-only, no interior to fill). Also
	// used by PackageSubstation (shape 385) for its own inner rectangle's
	// interior — unlike Rectangle/Circle, this shape *does* have a real
	// Voltage of its own (its own outer outline's color), so Fill here is
	// specifically the real source's own Abonent flag, generalized from an
	// on/off toggle locked to {color} into this schema's ordinary
	// free-choice color field. Also used by JunctionPoint (shape 7) the
	// same "none" way Rectangle/Circle already use it (their own default,
	// preserved so an already-placed/-saved junction point's look doesn't
	// change), even though real corpus shows most real instances (~65%)
	// filled with their own voltage color instead — Extract sets this
	// explicitly to whatever real instance's own fill color actually is,
	// same as it does for a Rectangle, rather than leaving that majority
	// case unset just because it happens to match {color}.
	Fill string `xml:"fill,attr,omitempty" json:"fill,omitempty"`
	// Stroke is a Rectangle's/Circle's own border color, or an Arrow's own
	// line color — same free-text convention as Fill. Empty falls back to a
	// plain visible color the same way an unset Lamp color does.
	Stroke string `xml:"stroke,attr,omitempty" json:"stroke,omitempty"`
	// StrokeWidth is a Rectangle's/Circle's own border thickness, or an
	// Arrow's own line thickness, in the same local/diagram units every
	// other shape's fixed stroke-width:1 is — unlike those, meaningfully
	// different per instance the way Radius is. 0 (unset) means the real
	// xsde2svg default of 1, not literally invisible.
	StrokeWidth float64 `xml:"strokeWidth,attr,omitempty" json:"strokeWidth,omitempty"`
	// DoubleHeaded draws an Arrow's (shape 2) own open chevron arrowhead
	// at both Points, not just the second one — matching the real
	// xsde2svg source's own FDouble flag. Unused by every other class.
	DoubleHeaded bool `xml:"doubleHeaded,attr,omitempty" json:"doubleHeaded,omitempty"`
	// NType selects between PackageSubstation's (shape 385) own two real
	// appearance variants, matching the real source's own Tech.NType field
	// exactly (recovered via xsde2svg's own element_385.go, whose data
	// export attribute has gone through two names — see
	// elements.go's own substationDataProperty and parsePackageSubstation
	// doc comments — since neither variant's own drawn geometry otherwise
	// gives it away on its own): 0 (unset, the common case) draws a
	// box-in-box pictogram with a short lead stub; 1 draws a plain
	// downward-pointing triangle instead. Unused by every other class.
	NType int `xml:"nType,attr,omitempty" json:"nType,omitempty"`
	// PropertyText is a short overlay label (e.g. a transformer's own power
	// rating, "160") drawn centered on PackageSubstation's (385) or
	// EnclosedSubstation's (386) own pictogram, staying upright regardless
	// of Orient/Mirror — matches the real source's own generic ParamText/
	// SubscriptName mechanism, which drives a per-element text label (with
	// its own position/alignment/font/color options) across dozens of
	// xsde2svg shapes; this schema narrows that down to the one fixed
	// centered/white/17px-Arial style every real 385/386 corpus instance
	// actually uses, rather than modeling the mechanism generically. Empty
	// means no label, matching real instances that carry none.
	PropertyText string `xml:"propertyText,attr,omitempty" json:"propertyText,omitempty"`

	Ports []Port `xml:"port,omitempty" json:"ports,omitempty"`
	// Points holds a BusBarSection's (shape 24) own drawn geometry (its two
	// or more vertices), a Rectangle's (shape 3) or Circle's (shape 4) own
	// two opposite corners of its own bounding box (order-independent —
	// Render normalizes them into a proper top-left/width/height, or
	// center/rx/ry for a Circle, the same way the real xsde2svg source
	// does), or an Arrow's (shape 2) own start and end (order *does*
	// matter here — the arrowhead is drawn at Points[1], the second one);
	// unused by every other, template-drawn class.
	Points []Point `xml:"geometry>point,omitempty" json:"points,omitempty"`

	// Autotransformer/Windings/VectorGroupLabel are a PowerTransformer's
	// (shape 47) own nameplate/winding configuration — see TransformerWinding
	// for what each winding itself records. Unlike every other shape's fixed
	// base.xml template, a PowerTransformer's real geometry (circle count,
	// position, and per-winding connection glyph) is driven entirely by
	// these fields via writePowerTransformer, not template substitution —
	// renderElement special-cases ClassPowerTransformer the same way it
	// already does ClassBusBarSection, bypassing the template lookup
	// entirely. len(Windings) is the transformer's own winding count (2, 3,
	// or 4); each entry gets one real electrical Port, in the same order.
	Autotransformer bool                 `xml:"autotransformer,attr,omitempty" json:"autotransformer,omitempty"`
	Windings        []TransformerWinding `xml:"windings>winding,omitempty" json:"windings,omitempty"`
	// VectorGroupLabel is a freeform connection-diagram label (e.g.
	// "Yn/Δ-11"), shown when non-empty ("Show connection diagram label" in
	// Properties). Real xsde2svg markup never computes this string anywhere
	// — the clock-hour number needs a phase-displacement input the format
	// doesn't carry — so it's simply typed in and stored verbatim, not
	// derived from the windings' own Scheme.
	VectorGroupLabel string `xml:"vectorGroupLabel,attr,omitempty" json:"vectorGroupLabel,omitempty"`
}

// WindingScheme is a PowerTransformer winding's own connection scheme.
// Matches three of real xsde2svg's own TransformerWinding.WindingType
// values (wye/ЗВЕЗДА_С_НУЛЕМ/delta) — the ones with a real, distinct
// connection glyph in element_47.go; the format's rarer values (zigzag,
// open_delta, ТРИ_ЛИНИИ, ...) aren't offered here.
type WindingScheme string

const (
	SchemeWye   WindingScheme = "wye"
	SchemeWyeN  WindingScheme = "wyeN"
	SchemeDelta WindingScheme = "delta"
)

// NeutralGrounding is only meaningful when a winding's own Scheme is
// SchemeWyeN (a brought-out neutral to ground at all). Real xsde2svg only
// draws a distinct glyph for GroundingSolid (its own "neutral_ground"
// WindingType, extra ground-hatch marks on the wye-with-neutral glyph) —
// GroundingIsolated/GroundingResistor get their own small invented tick
// marks here (xsde2svg has no glyph for either).
type NeutralGrounding string

const (
	GroundingSolid    NeutralGrounding = "solid"
	GroundingIsolated NeutralGrounding = "isolated"
	GroundingResistor NeutralGrounding = "resistor"
)

// TerminalDirection is which side of a winding's own circle its lead (and
// real electrical Port) is drawn on, in the transformer's own local
// (pre-rotation) frame — this editor's own replacement for real xsde2svg's
// opaque Chassis 1-7 switch, which ties leg direction to winding
// index/count/mirroring in ways that don't map onto a per-winding "pick a
// side" control. Rotates along with the whole element via its own Orient,
// same as every other local-coordinate shape in this codebase.
type TerminalDirection string

const (
	TerminalTop    TerminalDirection = "top"
	TerminalBottom TerminalDirection = "bottom"
	TerminalLeft   TerminalDirection = "left"
	TerminalRight  TerminalDirection = "right"
)

// TransformerWinding is one winding of a PowerTransformer element (see
// Element.Windings) — HV/MV/LV1/LV2 in declaration order, each drawn as its
// own circle.
type TransformerWinding struct {
	// Voltage references a VoltageClass.ID (0 means unassigned) — this
	// winding's own rated voltage/color, matching real xsde2svg's own
	// per-winding TransformerWinding.Voltage (each winding can carry a
	// genuinely different voltage class, unlike every other shape's single
	// Element.Voltage).
	Voltage int `xml:"voltage,attr,omitempty" json:"voltage,omitempty"`
	// Scheme is this winding's own connection scheme; empty draws no
	// connection glyph at all (matching a real instance with no windingType
	// attribute).
	Scheme WindingScheme `xml:"scheme,attr,omitempty" json:"scheme,omitempty"`
	// Grounding only applies when Scheme is SchemeWyeN.
	Grounding NeutralGrounding `xml:"grounding,attr,omitempty" json:"grounding,omitempty"`
	// TapChanger marks this as the regulated winding (OLTC/off-circuit tap
	// changer) — draws the diagonal regulation arrow, centered on the
	// transformer's own anchor point. Real xsde2svg only ever draws one
	// such arrow per transformer regardless of how many windings request
	// one (each winding-loop iteration overwrites the same shared path
	// variable, so only the last one drawn survives) — writePowerTransformer
	// matches that: the last winding with TapChanger set wins.
	TapChanger bool `xml:"tapChanger,attr,omitempty" json:"tapChanger,omitempty"`
	// Terminal is which side of this winding's own circle its lead is drawn
	// on; empty falls back to this winding's own conventional default for
	// the transformer's winding count (see defaultTerminal in render.go).
	Terminal TerminalDirection `xml:"terminal,attr,omitempty" json:"terminal,omitempty"`
}

// shapeDisconnector is the Disconnector's own current Shape key.
// shapeDisconnectorLegacy was a second, byte-for-byte identical shape the
// element library used to carry alongside it (sld-svg/symbols.xml's own
// comment reads "71 / 162: Disconnector") — removed from the library as a
// confusing duplicate palette entry, but kept understood here so Load can
// still make sense of an element saved by an older version of this editor
// (or loaded from an external source) rather than erroring or rendering
// with a missing-symbol warning.
const (
	shapeDisconnector       = "162"
	shapeDisconnectorLegacy = "71"
)

// ConnectorKind names a Connector's real-world wire kind.
type ConnectorKind string

const (
	KindBusbarWire   ConnectorKind = "BusbarWire"
	KindOverheadLine ConnectorKind = "OverheadLine"
	KindCableLine    ConnectorKind = "CableLine"
	KindBusWork      ConnectorKind = "BusWork"
	// KindLinkToObject is shape 28 ("Связь с объектом"/"Object link" in the
	// xsde2svg catalog) — visually a flat, un-wrapped polyline like
	// KindBusWork, but at a heavier stroke and decorated with a directional
	// arrowhead at its own "To" end (see writeObjectLink). Its own string
	// value is deliberately not "ObjectLink" — that's kindObjectLinkLegacy's
	// own already-taken value, rewritten to KindBusWork on Load, so reusing
	// it here would make every freshly-created connector of this real kind
	// immediately rewrite itself back to KindBusWork the next time the
	// diagram loads.
	KindLinkToObject ConnectorKind = "LinkToObject"
)

// ConnectorLineStyle is a KindCableLine connector's own dash pattern
// choice, mirroring xsde2svg's own line-style switch
// (xsde2svg/internal/modus/element_23.go) exactly. Meaningless for every
// other Kind — see Connector.LineStyle's own doc comment. Empty/unset
// resolves to LineStyleDashed (render.go's resolveCableLineDash), the
// same fixed dash this editor used before this field existed, so an
// already-saved diagram keeps rendering exactly as it did before.
type ConnectorLineStyle string

const (
	LineStyleSolid   ConnectorLineStyle = "solid"
	LineStyleDashed  ConnectorLineStyle = "dashed"
	LineStyleDashDot ConnectorLineStyle = "dashDot"
	LineStyleDotted  ConnectorLineStyle = "dotted"
)

// kindObjectLinkLegacy is KindBusWork's old stored value, from before this
// editor renamed it — kept only so Load can still make sense of a
// connector saved by an older version of this editor rather than erroring
// or leaving it un-typed.
const kindObjectLinkLegacy ConnectorKind = "ObjectLink"

// Connector is a drawn wire segment: a chain of points whose two ends
// resolve to electrical Nodes.
type Connector struct {
	ID   int           `xml:"id,attr" json:"id"`
	Kind ConnectorKind `xml:"kind,attr" json:"kind"`
	// Name is optional, like Element.Name — most connector kinds render
	// with no data-name at all (writePolyline's dataAttrs never includes
	// one), but a KindOverheadLine's own <g> wrapper does, matching a real
	// xsde2svg-exported line (e.g. "Line2").
	Name    string `xml:"name,attr,omitempty" json:"name,omitempty"`
	Voltage int    `xml:"voltage,attr,omitempty" json:"voltage,omitempty"`
	Layer   int    `xml:"layer,attr" json:"layer"`
	Dashed  bool   `xml:"dashed,attr,omitempty" json:"dashed,omitempty"`
	// LineStyle is a KindCableLine connector's own dash pattern — see
	// ConnectorLineStyle's own doc comment. Ignored for every other Kind
	// (render.go's writeNamedLine only reads it when Kind is
	// KindCableLine), left as a plain unvalidated field the same way an
	// Element's State is meaningless for a non-switching-device Class but
	// still just a plain field.
	LineStyle ConnectorLineStyle `xml:"lineStyle,attr,omitempty" json:"lineStyle,omitempty"`
	From      int                `xml:"from,attr" json:"from"`
	To        int                `xml:"to,attr" json:"to"`

	Points []Point `xml:"point" json:"points"`
}

// Point is one X,Y coordinate in the diagram's own coordinate space.
type Point struct {
	X float64 `xml:"x,attr" json:"x"`
	Y float64 `xml:"y,attr" json:"y"`
}

// Label is a standalone text caption. For carries the id of the Element it
// annotates (0 means unset — a standalone label).
type Label struct {
	// ID, like every other id-shaped field in this model, is a plain
	// positive integer the frontend's own IdSequence assigns (0 means
	// unset/invalid, since a real id is never 0) — needed so a specific
	// label can be individually selected/edited/deleted, the same as an
	// Element/Connector already can be.
	ID     int     `xml:"id,attr" json:"id"`
	For    int     `xml:"for,attr,omitempty" json:"for,omitempty"`
	Layer  int     `xml:"layer,attr" json:"layer"`
	X      float64 `xml:"x,attr" json:"x"`
	Y      float64 `xml:"y,attr" json:"y"`
	Size   float64 `xml:"size,attr" json:"size"`
	Anchor string  `xml:"anchor,attr,omitempty" json:"anchor,omitempty"`
	Bold   bool    `xml:"bold,attr,omitempty" json:"bold,omitempty"`
	// Color is the label's own text fill; empty means the default white
	// writeLabel has always used, so an already-saved label with no color
	// attribute at all keeps rendering exactly as before.
	Color string `xml:"color,attr,omitempty" json:"color,omitempty"`
	// VAlign is the label's vertical anchor relative to Y — "top"/"middle",
	// or empty for the original baseline-at-Y behavior ("bottom" is never
	// actually written; an empty attribute already means that, the same
	// way Anchor's own empty value means "start").
	VAlign string `xml:"valign,attr,omitempty" json:"valign,omitempty"`
	// Font is the label's own font-family; empty means the default Arial
	// writeLabel has always used, so an already-saved label with no font
	// attribute at all keeps rendering exactly as before.
	Font string `xml:"font,attr,omitempty" json:"font,omitempty"`
	Text string `xml:",chardata" json:"text"`
}

// DigitalDevice is a live SCADA-style analog readout — shape 134 in the
// xsde2svg catalog ("Прибор цифровой", digital instrument). It shares
// Label's own text-styling fields (Anchor/Bold/Color/VAlign/Font/Size), but
// unlike Label its content isn't free text: Value is a placeholder/default
// display value (e.g. "0.00", since this editor never binds to a live data
// source, only lays out where and how one would render), Name is the SCADA
// tag/point name (written as data-name, not shown in the text itself), and
// Unit is an optional unit-of-measure suffix (e.g. "MW") rendered as its own
// inline <tspan> right after Value on the same line — unlike Label's own
// tspans, which each start a new stacked line instead.
type DigitalDevice struct {
	ID     int     `xml:"id,attr" json:"id"`
	Layer  int     `xml:"layer,attr" json:"layer"`
	X      float64 `xml:"x,attr" json:"x"`
	Y      float64 `xml:"y,attr" json:"y"`
	Size   float64 `xml:"size,attr" json:"size"`
	Anchor string  `xml:"anchor,attr,omitempty" json:"anchor,omitempty"`
	Bold   bool    `xml:"bold,attr,omitempty" json:"bold,omitempty"`
	// Color is the readout's own text fill; empty means the same default
	// white writeDigitalDevice/writeLabel have always used.
	Color string `xml:"color,attr,omitempty" json:"color,omitempty"`
	// VAlign follows Label.VAlign's own convention exactly ("top"/"middle",
	// empty for the original baseline-at-Y behavior).
	VAlign string `xml:"valign,attr,omitempty" json:"valign,omitempty"`
	// Font is the readout's own font-family; empty means the default Arial
	// writeDigitalDevice/writeLabel have always used.
	Font string `xml:"font,attr,omitempty" json:"font,omitempty"`
	// Name is the SCADA tag/point name this readout represents, written as
	// data-name — purely informational to this editor, the same way
	// Connector.Name is; it plays no part in what's actually displayed.
	Name string `xml:"name,attr,omitempty" json:"name,omitempty"`
	// Value is the placeholder/default text shown in place of a live
	// reading (e.g. "0.00").
	Value string `xml:"value,attr" json:"value"`
	// Unit is an optional unit-of-measure suffix (e.g. "MW", "kV"); empty
	// omits both the data-unit attribute and the unit <tspan> entirely.
	Unit string `xml:"unit,attr,omitempty" json:"unit,omitempty"`
}

// emptyElement matches a start tag immediately followed by its own end tag
// (encoding/xml never emits self-closing tags, even for elements with no
// content), so Save can collapse them into the shorter self-closing form.
var emptyElement = regexp.MustCompile(`<([A-Za-z][\w:.-]*)((?:\s+[A-Za-z_:][\w:.-]*="[^"]*")*)></([A-Za-z][\w:.-]*)>`)

// emptyPathWrapperLine matches a whole line consisting solely of one of
// this model's nested "parent>child" xml tags — Diagram's own
// Layers/VoltageClasses/Nodes/Elements/Connectors/Labels/DigitalDevices,
// and Element's own Points ("geometry>point") and Windings
// ("windings>winding") — immediately closed with no children, i.e. its
// slice happened to be empty. encoding/xml's own omitempty is documented
// to apply to a slice, but is silently ignored specifically for a tag with
// a chained "parent>child" path (a long-standing stdlib limitation:
// golang/go#4256), so it still writes the parent wrapper unconditionally
// regardless of omitempty — e.g. a non-BusBarSection Element, which never
// populates Points, otherwise always carried a meaningless empty
// <geometry/> (self-closing, after the emptyElement collapse below), and
// (before this line added "windings") a non-PowerTransformer Element
// carried an equally meaningless empty <windings/> the same way. None of
// these 9 wrapper tags ever carries its own attributes, so matching the
// immediately-closed (zero content) case can't mistake a populated one
// (whose own child elements/whitespace separate its open and close tags)
// for an empty one. Removed entirely, not just collapsed, since a wrapper
// with no attributes and no children carries no information Load could
// ever need — an entirely absent one round-trips identically to an
// explicit empty one (a nil slice either way). This must run before
// emptyElement's own collapse below: stripping an Element's only child
// (e.g. a Lamp with no Ports and no Points) can leave that Element's own
// tag newly empty, which emptyElement then collapses to self-closing in
// the usual way.
var emptyPathWrapperLine = regexp.MustCompile(
	`\n[ \t]*<(?:geometry|windings|layers|voltageClasses|nodes|elements|connectors|labels|digitalDevices)></[A-Za-z][\w:.-]*>`,
)

// Save writes d as indented XML.
func (d *Diagram) Save(w io.Writer) error {
	if _, err := io.WriteString(w, xml.Header); err != nil {
		return err
	}
	var buf bytes.Buffer
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")
	if err := enc.Encode(d); err != nil {
		return fmt.Errorf("slddoc: encoding diagram: %w", err)
	}
	stripped := emptyPathWrapperLine.ReplaceAll(buf.Bytes(), nil)
	out := emptyElement.ReplaceAll(stripped, []byte("<$1$2/>"))
	if _, err := w.Write(out); err != nil {
		return err
	}
	_, err := io.WriteString(w, "\n")
	return err
}

// Load reads a Diagram previously written by Save.
func Load(r io.Reader) (*Diagram, error) {
	var d Diagram
	if err := xml.NewDecoder(r).Decode(&d); err != nil {
		return nil, fmt.Errorf("slddoc: decoding diagram: %w", err)
	}
	for i, c := range d.Connectors {
		if c.Kind == kindObjectLinkLegacy {
			d.Connectors[i].Kind = KindBusWork
		}
	}
	for i, e := range d.Elements {
		if e.Shape == shapeDisconnectorLegacy {
			d.Elements[i].Shape = shapeDisconnector
		}
	}
	return &d, nil
}

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
	// been saved by this editor, or one produced by sld-svg's own tooling
	// from an SVG without a root background-color (Extract sets only
	// Background, from that style); callers fall back to the server's
	// configured defaults for anything unset.
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
	ClassShortCircuiter Class = "ShortCircuiter"
	// ClassPowerCircuitBreaker (shape 399, Автомат силовой) is a
	// two-terminal low-voltage automatic circuit breaker, drawn like a
	// Disconnector (two fixed contact bars plus a state-driven blade) with
	// a small filled square beside the blade that moves with it. Only two
	// real states exist (Closed/Open, no Intermediate), same as
	// ClassSectionalizer.
	ClassPowerCircuitBreaker Class = "PowerCircuitBreaker"
	ClassGround              Class = "Ground"
	ClassPowerTransformer    Class = "PowerTransformer"
	ClassCurrentTransformer  Class = "CurrentTransformer"
	ClassVoltageTransformer  Class = "VoltageTransformer"
	ClassChokeCoil           Class = "ChokeCoil"
	ClassReactor             Class = "Reactor"
	ClassReactorShunt        Class = "ReactorShunt"
	ClassSurgeArrester       Class = "SurgeArrester"
	ClassFuse                Class = "Fuse"
	ClassCapacitor           Class = "Capacitor"
	ClassCapacitorBank       Class = "CapacitorBank"
	ClassHalfChassis         Class = "HalfChassis"
	ClassChassis             Class = "Chassis"
	ClassStarter             Class = "Starter"
	ClassGenerator           Class = "Generator"
	ClassBusBarSection       Class = "BusBarSection"
	ClassJunctionPoint       Class = "JunctionPoint"
	// ClassFork (shape 26, "Развилка"/Fork) is a real three-terminal
	// wiring element: a "V" whose vertex (at X/Y) and two arm tips are each
	// a real electrical terminal — one wire in at the vertex, one out at
	// each tip. Radius holds its arm length (0 = forkArmLength, 10): the
	// real source scales it per element (10 * sqrt(2)^scale, so 4/7/10/14/
	// 20/28 for scale -2..3), the first shape this schema models a
	// per-element size for. Mirror is visually inert (the "V" is symmetric
	// and the real source has no mirror branch for it); no Name or State.
	ClassFork            Class = "Fork"
	ClassNonIntersection Class = "NonIntersection"
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
	// ClassButton (shape 113, "Объемная кнопка"/3D button) is a purely
	// decorative annotation widget — not real electrical equipment, same
	// non-electrical status as ClassRectangle (no Ports/Voltage/State,
	// never a connectElements/routing endpoint) — used in real diagrams as
	// a static navigation/action control (e.g. a button labeled "Журнал
	// событий"/"Event log") rather than anything reflecting live switching
	// state: real corpus never shows a data-state/data-voltage on one
	// despite the source computing both, and every real instance found
	// uses a single fixed look, not two Closed-driven variants. Its
	// geometry is its own drawn Points (two opposite corners), the same
	// convention ClassRectangle already uses, with PropertyText as its own
	// centered label (see that field's own doc comment) and TextColor/Bold
	// as that label's own per-instance color/weight — real corpus shows
	// both genuinely varying (a dark box with plain white text vs. a light
	// box with bold dark text), unlike PackageSubstation/
	// EnclosedSubstation/FaultPassageIndicator's own fixed-style overlay
	// text.
	ClassButton Class = "Button"
	// ClassRoad (shape 335, "Дорога"/Road) is a purely decorative
	// geographic background line — not real electrical equipment, same
	// non-electrical status as ClassRectangle (no Ports/Voltage/State,
	// never a connectElements/routing endpoint). Unlike Rectangle/Circle's
	// own two-opposite-corners Points convention or Arrow's own two-
	// ordered-endpoints one, a Road's geometry is an arbitrary multi-vertex
	// Points polyline, the same convention ClassBusBarSection already uses
	// (real corpus shows real instances bending through several points,
	// e.g. a 5-point road). Reuses Stroke/StrokeWidth for its own line
	// color/thickness — both genuinely vary per real instance (orangered/
	// blue/royalblue/white seen, widths 8/10/12) — with no Fill of its own
	// (an open line, like Arrow, not a closed shape).
	ClassRoad Class = "Road"
	// ClassPostPole (shape 292, "Опора стоечная"/Post-type pole) is a
	// purely decorative structural marker — a utility pole's own drawn
	// location on a pole-by-pole layout diagram, not real electrical
	// equipment (no Ports/Voltage/State, never a connectElements/routing
	// endpoint, same status as ClassRectangle). Real corpus shows it drawn
	// as either a round or a square marker (Square selects which — the
	// real source's own StyleTow "round"/"sqware", collapsed from its own
	// three-way Material/StyleTow/FillTow branching, which always reduces
	// to just those two visual shapes either way) at a genuinely varying
	// per-instance Radius/Fill/Stroke (reused from Rectangle/Circle's own
	// fields — Radius doubles as the square variant's own half-width, the
	// same numeric value the real source's own Radius/w constants always
	// share). StrokeWidth is not modeled — the real source hardcodes 1,
	// unlike Rectangle/Road's own genuinely-varying width. Orient rotates
	// around the marker's own anchor when set (matching a real instance
	// that carries one), but is visually inert either way — a circle has
	// no orientation of its own, and an axis-aligned square drawn at any
	// of this editor's own 4 supported angles (0/90/180/-90) looks
	// identical regardless — so Properties hides the field entirely, the
	// same treatment ClassLamp's own equally-inert Orientation gets.
	ClassPostPole Class = "PostPole"
	// ClassLine (shape 1, "Линия"/Line) is a purely decorative generic
	// line — not real electrical equipment (no Ports/Voltage/State, never
	// a connectElements/routing endpoint, same status as ClassRectangle).
	// Its geometry is an arbitrary multi-vertex Points polyline, the same
	// convention ClassBusBarSection/ClassRoad already use. Reuses Stroke/
	// StrokeWidth (both genuinely vary per real instance — black/gray/
	// white/red/yellow/hex colors, widths 1-10), with no Fill of its own
	// (an open line, like Arrow/Road). Unlike Road, the real source never
	// resolves its own color from anything voltage-like even loosely — it
	// comes purely from a line-style table — so there's no ambiguity
	// there. LineStyle (reusing Connector's own ConnectorLineStyle type,
	// not a dedicated one — same solid/dashed/dashDot choice, just
	// resolved through its own dash values, see writeLine) captures the
	// real source's own dashed/dash-dot line styles, confirmed genuinely
	// used in ~10% of real corpus instances found (its own third value,
	// LineStyleDotted, has no real source counterpart for this shape and
	// is never produced by Extract, only reachable by hand-editing).
	ClassLine Class = "Line"
	// ClassPolygon (shape 16, "Многоугольник"/Polygon) is a purely
	// decorative closed shape — not real electrical equipment (no Ports/
	// Voltage/State, never a connectElements/routing endpoint, same status
	// as ClassLine). Its geometry is an arbitrary Points vertex list (at
	// least 3; the real source closes it implicitly), the same convention
	// ClassLine uses. Reuses Fill (the real source's own background color,
	// "none" when unset), Stroke/StrokeWidth, and LineStyle — only
	// LineStyleDotted/LineStyleDashDot have a real counterpart for this
	// shape (see polygonDashPatterns); LineStyleDashed draws solid.
	ClassPolygon Class = "Polygon"
	// ClassArc (shape 9, "Дуга"/Arc) is a purely decorative elliptical
	// arc — not real electrical equipment (no Ports/Voltage/State, never a
	// connectElements/routing endpoint, same status as ClassLine). It is
	// stored the way an SVG arc command itself is: Points holds exactly
	// its start and end point, plus RadiusX/RadiusY/LargeArc/Sweep. The
	// real source (internal/modus/element_9.go) derives these from a
	// bounding box and two direction points and always emits
	// large-arc=1/sweep=0 with a fixed 1° x-axis rotation; an extracted
	// arc keeps exactly what the real path carried, while one drawn in
	// this editor gets whichever flags match its own bulge (so it may be
	// a shallow arc the real source itself could never produce). Reuses
	// Stroke/StrokeWidth, the latter being the drawn width (the real
	// source draws Width/4, 0.25 for every real corpus instance).
	ClassArc Class = "Arc"
	// ClassPowerflowIndicator (shape 320001, "Направление перетока"/
	// Powerflow direction) is a purely decorative annotation glyph — not
	// real electrical equipment (no Ports/Voltage, never a connectElements/
	// routing endpoint, same non-electrical status as ClassRectangle) — a
	// single bold arrow character drawn at its own anchor, rotated by
	// Orient the same generic way every other rotate-based shape is. State
	// selects which glyph: nil/0 draws "→", any other value draws "←"
	// (matching the real source's own `FState != "0"` check) — a plain
	// two-way toggle, not a switching-device-style legend. Reuses TextColor
	// (Button's own PropertyText color field) for the glyph's own fill —
	// real corpus shows this genuinely varying per instance (skyblue,
	// #C8A2C8, ...), resolved from the real source's own Color1, not from
	// any VoltageClass. Font size is not modeled per instance: real corpus
	// shows it varying only *between* diagrams (26 in one file, 18 in
	// another), consistent with the real source's own diagram-wide Scale
	// factor rather than a genuine per-instance property, so
	// writePowerflowIndicator draws it at one fixed size.
	ClassPowerflowIndicator Class = "PowerflowIndicator"
	// ClassTable (shape 312, "Таблица"/Table) is a purely decorative
	// annotation box — not real electrical equipment, same non-electrical
	// status as ClassRectangle (no Ports/Voltage/State, never a
	// connectElements/routing endpoint). The real source (element_312.go)
	// only actually draws the simple case modeled here (its own Points
	// this schema uses, the same two-opposite-corners convention
	// ClassRectangle/ClassButton already use) — a real instance with more
	// than two corners (an attempt at a real multi-cell grid via this
	// shape) is explicitly skipped by the real exporter itself with a log
	// telling the operator to use shape 313 instead, so that case has no
	// real markup to model here at all. Reuses Fill (the real source's own
	// BGColor) and Stroke (its own Color) for interior/border, StrokeWidth,
	// LineStyle (reusing Connector's own ConnectorLineStyle, the same
	// solid/dashed/dashDot choice Line's own does, resolved through its own
	// real dash values — see writeTable) for the border's own dash pattern,
	// and PropertyText/TextColor for an optional centered text label
	// (Button's own convention) — unlike Button, Orient additionally
	// rotates that label around the box's own center when set, matching
	// the real source's own ParamText.Orient (never modeled for any other
	// Points-based shape, since none of them has a text label of its own
	// to rotate).
	ClassTable Class = "Table"
	// ClassTable2 (shape 313, "Таблица 2"/Table 2) is a purely decorative
	// multi-row/multi-column grid — not real electrical equipment, same
	// non-electrical status as ClassRectangle. Unlike every other
	// decorative shape so far, its own geometry isn't Points-based: X/Y is
	// its own top-left anchor, and RowHeights/ColumnWidths (each row's/
	// column's own real size) lay out a grid from there — Cells then
	// places each real cell's own text (and, only when a real extracted
	// instance sets one, its own Fill/TextColor override — see TableCell's
	// own doc comment) at a (Row, Col) position within that grid. The real
	// source's own cell-merging (one cell spanning several grid positions
	// at once) is deliberately not modeled — a real instance using it
	// still extracts, its own would-be-merged cells just render as
	// separate ones instead of one wide/tall cell, the same kind of
	// partial-fidelity tradeoff already made for a few other shapes (e.g.
	// ClassCableConnector's own unmodeled CustomView variant) — nor is a
	// cell's own real multi-paragraph text (the real source's own literal
	// "#x9" newline-split), reduced here to a single line per cell.
	// Reuses Stroke/StrokeWidth/LineStyle for the grid's own line color/
	// thickness/dash (LineStyle resolved through its own real dash value,
	// different from every other consumer's — see writeTable2) and Fill
	// as the whole table's own default cell background, overridden per
	// cell by TableCell.Fill when a real extracted instance's own cell had
	// one. A real instance has no stable id of its own at all in the
	// format this schema was originally ported from — internal/modus/
	// element_313.go emitted one untagged, ungrouped `<path
	// data-type="313">` per cell, with no marker distinguishing one real
	// table's own cells from an unrelated adjacent one's — so, at this
	// project's own request, the real xsde2svg source (and element_312.go,
	// ClassTable's own real source, for the identical reason) was changed
	// to wrap a whole table's own output in a single `<g id
	// data-type="312"|"313">`, the same fix already made for shapes 7/106/
	// 385/386; Extract requires that wrapping `<g>` to recognize either
	// shape at all, so a real diagram exported by an xsde2svg build from
	// before that fix simply doesn't have its own table(s) recognized —
	// same "not yet understood, skipped" treatment either shape already
	// implicitly got before this schema modeled them at all, not a
	// regression.
	ClassTable2 Class = "Table2"
)

// TableCell is one real cell of a Table2 (shape 313) grid — see
// ClassTable2's own doc comment for what this schema does and doesn't
// model of the real shape. Row/Col are 0-based grid positions (matching
// however many entries RowHeights/ColumnWidths respectively give the
// owning Element); a (Row, Col) with no corresponding TableCell entry at
// all is simply not drawn, rather than defaulting to an empty cell — real
// corpus tables are usually, but not always, a fully-populated rectangle.
type TableCell struct {
	Row int `xml:"row,attr" json:"row"`
	Col int `xml:"col,attr" json:"col"`
	// Text is this cell's own content, reduced to a single line — see
	// ClassTable2's own doc comment for why a real multi-paragraph cell
	// isn't modeled.
	Text string `xml:"text,attr,omitempty" json:"text,omitempty"`
	// Fill overrides the owning Element's own Fill (its table-wide default
	// cell background) for this one cell — empty means "use the table's
	// own default", matching the real source's own cell.GridProp.BGColor-
	// falls-back-to-sde.BGColor convention exactly.
	Fill string `xml:"fill,attr,omitempty" json:"fill,omitempty"`
	// TextColor overrides this cell's own text color; empty falls back to
	// black, the real source's own default when a cell sets no color of
	// its own.
	TextColor string `xml:"textColor,attr,omitempty" json:"textColor,omitempty"`
}

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
	// in degrees (0, 90, 180, -90). Also used by Table (312), whose own
	// geometry is otherwise Points-based and has no template/anchor
	// rotation of its own to apply this to — there, Orient instead rotates
	// its own optional centered PropertyText label around the box's own
	// center, matching the real source's own ParamText.Orient (the box
	// itself never rotates).
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
	// one applies to this class. Also used by PowerflowIndicator (shape
	// 320001) for its own two-way arrow direction: nil/0 draws "→", any
	// other value draws "←" — not a real switching status, but the same
	// nil-defaults-to-0 convention.
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
	// on that fallback. Also used by PostPole (shape 292) for its own
	// drawn circle radius, doubling as the Square variant's own
	// half-width — the real source's own Radius/w constants always share
	// one value, so this schema doesn't need a second field for it.
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
	// case unset just because it happens to match {color}. PostPole (292)
	// reuses this same "none" default too, even though real corpus shows
	// both a filled and unfilled marker are common. Also a Table's (312)
	// own box interior (the real source's own BGColor) and a Table2's
	// (313) own table-wide default cell background, overridden per cell by
	// TableCell.Fill when set.
	Fill string `xml:"fill,attr,omitempty" json:"fill,omitempty"`
	// Stroke is a Rectangle's/Circle's own border color, an Arrow's/Road's/
	// Line's own line color, a Button's own box border color, a
	// PostPole's own marker border color, a Table's (312) own box border
	// color, or a Table2's (313) own grid line color — same free-text
	// convention as Fill. Empty falls back to a plain visible color the
	// same way an unset Lamp color does (PostPole's own unset fallback is
	// "gray", the real corpus's own dominant color, rather than
	// Rectangle/Arrow/Button/Line/Table/Table2's own "white"/"black" — see
	// writeLine for Line's own).
	Stroke string `xml:"stroke,attr,omitempty" json:"stroke,omitempty"`
	// StrokeWidth is a Rectangle's/Circle's own border thickness, an
	// Arrow's/Road's/Line's own line thickness, a Button's own box
	// border thickness, or a Table's/Table2's own border/grid line
	// thickness, in the same local/diagram units every other
	// shape's fixed stroke-width:1 is — unlike those, meaningfully
	// different per instance the way Radius is. 0 (unset) means the real
	// xsde2svg default of 1 for every one of these except Road, whose own
	// writeRoad instead falls back to a much thicker thumbnail width — a
	// real instance is never actually drawn at width 1 (real corpus shows
	// 8-12), so this editor's own default reflects a road's real visual
	// weight rather than that generic fallback.
	StrokeWidth float64 `xml:"strokeWidth,attr,omitempty" json:"strokeWidth,omitempty"`
	// DoubleHeaded draws an Arrow's (shape 2) own open chevron arrowhead
	// at both Points, not just the second one — matching the real
	// xsde2svg source's own FDouble flag. Unused by every other class.
	DoubleHeaded bool `xml:"doubleHeaded,attr,omitempty" json:"doubleHeaded,omitempty"`
	// Square draws a PostPole's (shape 292) own square marker instead of
	// its default round one — matching the real source's own StyleTow
	// "sqware" value (see ClassPostPole's own doc comment). Unused by
	// every other class.
	Square bool `xml:"square,attr,omitempty" json:"square,omitempty"`
	// LineStyle is a Line's (shape 1) own dash pattern, a Table's (312)
	// own box border dash pattern, or a Table2's (313) own grid line dash
	// pattern — reuses Connector's own ConnectorLineStyle type (the same
	// solid/dashed/dashDot choice), but each resolved through its own
	// consumer-specific dash values (writeLine/writeTable/writeTable2),
	// distinct from a KindCableLine connector's own (see
	// resolveCableLineDash) — every one of these real xsde2svg sources
	// uses different literal stroke-dasharray numbers for the "same"
	// named styles. Empty means solid, matching the real source's own
	// default (unlike Connector.LineStyle, whose own empty value defaults
	// to dashed for historical reasons specific to that field).
	// LineStyleDotted has no real source counterpart for any of these
	// shapes, so Extract never produces it for any of them — included
	// only because the type is shared, not because a real
	// instance can carry it.
	LineStyle ConnectorLineStyle `xml:"lineStyle,attr,omitempty" json:"lineStyle,omitempty"`
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
	// means no label, matching real instances that carry none. Also used
	// by FaultPassageIndicator (320003) for its own centered overlay text
	// — unlike 385/386, this one has no real source counterpart at all
	// (element_320.go's own custom-element case for this shape draws no
	// text of any kind; the label is purely this schema's own
	// long-standing convention), so empty here means the admin-configured
	// default (Render's own defaultFPIText param, "FPI" out of the box —
	// see config.Config.Indicators.DefaultFPIText), not "no label". Also
	// used by Button (113) for its own centered label — unlike 385/386/
	// FPI, this one carries no fixed style of its own; see TextColor/Bold.
	// Also used by Table (312) for its own optional centered cell text
	// (the real source's own Cell field) — like Button, no fixed style of
	// its own; see TextColor, and Orient for its own text-rotation reuse.
	PropertyText string `xml:"propertyText,attr,omitempty" json:"propertyText,omitempty"`
	// TextColor is a Button's (113) own PropertyText color — unlike 385/
	// 386/FPI's fixed white overlay text, real corpus shows this genuinely
	// varying per instance (plain white text on a dark box, or dark text
	// on a light box). Empty falls back to white, the more common real
	// case. Also used by PowerflowIndicator (320001) for its own arrow
	// glyph's fill color (the real source's own Color1) — empty falls back
	// to black there instead, matching Line's own default rather than
	// Button's, since a real instance is drawn directly on the canvas
	// background rather than inside its own filled box. Also used by
	// Table (312) for its own PropertyText color, empty falling back to
	// black the same way PowerflowIndicator's own does (the real source's
	// own default too). Unused by every other class — a Table2's (313)
	// own per-cell text color is TableCell.TextColor instead, since a
	// whole Table2 has many independent cells, not one shared label.
	TextColor string `xml:"textColor,attr,omitempty" json:"textColor,omitempty"`
	// Bold draws a Button's (113) own PropertyText in bold — matches the
	// real source's own ParamText.FontStyle containing "BOLD" — real
	// corpus shows both a plain and a bold real instance. Unused by every
	// other class.
	Bold bool `xml:"bold,attr,omitempty" json:"bold,omitempty"`
	// RadiusX/RadiusY/LargeArc/Sweep are an Arc's (shape 9, see
	// ClassArc) own SVG elliptical-arc parameters, stored exactly as the
	// arc's own "A rx,ry rotation large-arc sweep x,y" command carries them
	// — Points holds its start and end point.
	RadiusX  float64 `xml:"rx,attr,omitempty" json:"rx,omitempty"`
	RadiusY  float64 `xml:"ry,attr,omitempty" json:"ry,omitempty"`
	LargeArc bool    `xml:"largeArc,attr,omitempty" json:"largeArc,omitempty"`
	Sweep    bool    `xml:"sweep,attr,omitempty" json:"sweep,omitempty"`

	Ports []Port `xml:"port,omitempty" json:"ports,omitempty"`
	// Points holds a BusBarSection's (shape 24), Road's (shape 335), or
	// Line's (shape 1) own drawn geometry (its two or more vertices, in
	// drawn order — a Road/Line can genuinely bend through several, unlike
	// the fixed-two-point shapes below), a Rectangle's (shape 3), Circle's
	// (shape 4), or Button's
	// (shape 113), or Table's (shape 312) own two opposite corners of its
	// own bounding box (order-independent — Render normalizes them into a
	// proper top-left/width/height, or center/rx/ry for a Circle, the same
	// way the real xsde2svg source does), or an Arrow's (shape 2) own
	// start and end (order *does* matter here — the arrowhead is drawn at
	// Points[1], the second one); unused by every other, template-drawn
	// class — including Table2 (313), whose own geometry is X/Y plus
	// RowHeights/ColumnWidths instead, not Points (see ClassTable2's own
	// doc comment).
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

	// RowHeights/ColumnWidths/Cells are a Table2's (shape 313) own grid —
	// see ClassTable2's own doc comment for the full model. Like
	// PowerTransformer's own Windings above, a Table2's real geometry is
	// driven entirely by these fields via writeTable2, not template
	// substitution. RowHeights[i]/ColumnWidths[j] is row i's/column j's own
	// real size (local units, summed from the table's own X,Y anchor to
	// place each cell); len(RowHeights)/len(ColumnWidths) is the grid's own
	// row/column count.
	RowHeights   []float64   `xml:"rows>row,omitempty" json:"rowHeights,omitempty"`
	ColumnWidths []float64   `xml:"columns>column,omitempty" json:"columnWidths,omitempty"`
	Cells        []TableCell `xml:"cells>cell,omitempty" json:"cells,omitempty"`
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
// and Element's own Points ("geometry>point"), Windings
// ("windings>winding"), and a Table2's own RowHeights/ColumnWidths/Cells
// ("rows>row"/"columns>column"/"cells>cell") — immediately closed with no
// children, i.e. its slice happened to be empty. encoding/xml's own
// omitempty is documented to apply to a slice, but is silently ignored
// specifically for a tag with a chained "parent>child" path (a
// long-standing stdlib limitation: golang/go#4256), so it still writes the
// parent wrapper unconditionally regardless of omitempty — e.g. a
// non-BusBarSection Element, which never populates Points, otherwise
// always carried a meaningless empty <geometry/> (self-closing, after the
// emptyElement collapse below), a non-PowerTransformer Element an equally
// meaningless empty <windings/>, and a non-Table2 Element (every other
// class) three equally meaningless empty <rows/>/<columns/>/<cells/> the
// same way. None of these 12 wrapper tags ever carries its own attributes,
// so matching the immediately-closed (zero content) case can't mistake a
// populated one (whose own child elements/whitespace separate its open
// and close tags) for an empty one. Removed entirely, not just collapsed,
// since a wrapper with no attributes and no children carries no
// information Load could ever need — an entirely absent one round-trips
// identically to an explicit empty one (a nil slice either way). This must
// run before emptyElement's own collapse below: stripping an Element's
// only child (e.g. a Lamp with no Ports and no Points) can leave that
// Element's own tag newly empty, which emptyElement then collapses to
// self-closing in the usual way.
var emptyPathWrapperLine = regexp.MustCompile(
	`\n[ \t]*<(?:geometry|windings|layers|voltageClasses|nodes|elements|connectors|labels|digitalDevices|rows|columns|cells)></[A-Za-z][\w:.-]*>`,
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
		// A real data-type="21" connector (Ошиновка/Buswork) was, until
		// this bug was fixed, misread by Extract's own connectorKindByType
		// as KindBusbarWire instead of KindBusWork (KindBusbarWire's own
		// code "21" belonged to KindBusWork all along — see that map's own
		// doc comment). The frontend itself never creates a KindBusbarWire
		// connector (removed as a palette choice, no rendering distinction
		// of its own — see wireKindIcon.ts), so every one on disk is this
		// same bug's own artifact, not real data, and is rewritten here the
		// same way kindObjectLinkLegacy already is above.
		if c.Kind == KindBusbarWire {
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

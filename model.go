package slddoc

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
)

// BaseLayer is the implicit layer every element belongs to when the source
// SVG carries no data-layer attribute for it. It is always present in a
// Diagram's Layers so "one element, exactly one layer" holds without
// exception.
const BaseLayer = 0

// Diagram is the root of the SLD document model.
type Diagram struct {
	XMLName xml.Name `xml:"diagram"`

	Width  float64 `xml:"width,attr"`
	Height float64 `xml:"height,attr"`
	Source string  `xml:"source,attr,omitempty"`

	Layers         []Layer        `xml:"layers>layer"`
	VoltageClasses []VoltageClass `xml:"voltageClasses>class"`
	Nodes          []Node         `xml:"nodes>node"`
	Elements       []Element      `xml:"elements>element"`
	Connectors     []Connector    `xml:"connectors>connector"`
	Labels         []Label        `xml:"labels>label"`
}

// Layer is one entry of a diagram's visibility layers. A viewer toggles
// elements on and off by Layer.ID; every Element/Connector/Label carries
// exactly one Layer reference.
type Layer struct {
	ID   int    `xml:"id,attr"`
	Name string `xml:"name,attr"`
}

// VoltageClass maps a logical, real-world voltage level (e.g. "10 kV") to the
// color used to draw it in this diagram. Voltage is a logical attribute of
// an Element/Connector, not a color — VoltageClass is the only place a color
// is recorded, purely for rendering.
type VoltageClass struct {
	ID    int    `xml:"id,attr"`
	Name  string `xml:"name,attr"`
	Color string `xml:"color,attr"`
}

// Node is an electrical junction reconstructed from the source SVG's
// geometry: every Port and Connector endpoint that shares a Node.ID is
// electrically connected.
type Node struct {
	ID int     `xml:"id,attr"`
	X  float64 `xml:"x,attr"`
	Y  float64 `xml:"y,attr"`
}

// Port is one electrical terminal of an Element, in the element's own local
// (pre-rotation) coordinate space, referencing the Node it resolves to.
type Port struct {
	Name string `xml:"name,attr"`
	Node int    `xml:"node,attr"`
}

// Class names an Element's real-world equipment kind, independent of the
// numeric xsde2svg shape code it was parsed from.
type Class string

const (
	ClassBreaker               Class = "Breaker"
	ClassDisconnector          Class = "Disconnector"
	ClassLoadBreakSwitch       Class = "LoadBreakSwitch"
	ClassGroundSwitch          Class = "GroundSwitch"
	ClassGround                Class = "Ground"
	ClassPowerTransformer      Class = "PowerTransformer"
	ClassCurrentTransformer    Class = "CurrentTransformer"
	ClassChokeCoil             Class = "ChokeCoil"
	ClassSurgeArrester         Class = "SurgeArrester"
	ClassFuse                  Class = "Fuse"
	ClassCapacitor             Class = "Capacitor"
	ClassBusBarSection         Class = "BusBarSection"
	ClassJunctionPoint         Class = "JunctionPoint"
	ClassLamp                  Class = "Lamp"
	ClassFaultPassageIndicator Class = "FaultPassageIndicator"
)

// Element is one placed piece of equipment.
type Element struct {
	ID    int   `xml:"id,attr"`
	Class Class `xml:"class,attr"`
	// Shape is the xsde2svg numeric ObjectType code this Element was parsed
	// from (e.g. "41", "43"). Two shapes can share the same electrical
	// Class (a fixed and a withdrawable breaker are both ClassBreaker) but
	// need different symbols.xml templates to render correctly, so the
	// renderer looks symbols up by Shape, not by Class.
	Shape   string `xml:"shape,attr"`
	Name    string `xml:"name,attr,omitempty"`
	Voltage int    `xml:"voltage,attr,omitempty"`
	Layer   int    `xml:"layer,attr"`

	// X, Y is the element's anchor: its rotation center for a rotated
	// symbol, or the midpoint of its ports otherwise.
	X float64 `xml:"x,attr"`
	Y float64 `xml:"y,attr"`
	// Orient is the rotation applied to the symbol template around (X,Y),
	// in degrees, following xsde2svg's own convention (0, 90, 180, -90).
	Orient int `xml:"orient,attr,omitempty"`
	// State carries an element's data-state (e.g. breaker open/closed), when
	// the source recorded one.
	State *int `xml:"state,attr,omitempty"`
	// FillOff/FillOn are a Lamp's (shape 106) two display colors, from its
	// data-fill="0:off,1:on" attribute; State selects which one is current.
	// Unlike the switch-like devices' state indicator (rendered from a fixed
	// red/lawngreen/yellow convention, see stateFill in render.go), a lamp's
	// colors are chosen per-instance in the source and carry real meaning,
	// so they're recorded rather than reduced to that convention.
	FillOff string `xml:"fillOff,attr,omitempty"`
	FillOn  string `xml:"fillOn,attr,omitempty"`
	// Radius is a Lamp's (shape 106) drawn circle radius, from its own r
	// attribute. Unlike other shapes' fixed template geometry, real
	// instances draw meaningfully different sizes for different roles (e.g.
	// r=11 standalone "Индикатор" panel lights vs. r=5 lamps clustered in
	// triplets next to a breaker), so it's recorded per instance rather
	// than assumed constant.
	Radius float64 `xml:"radius,attr,omitempty"`

	Ports []Port `xml:"port,omitempty"`
	// Points holds a BusBarSection's own drawn geometry (its two or more
	// vertices); unused by point-symbol classes.
	Points []Point `xml:"geometry>point,omitempty"`
}

// ConnectorKind names a Connector's real-world wire kind.
type ConnectorKind string

const (
	KindBusbarWire   ConnectorKind = "BusbarWire"
	KindOverheadLine ConnectorKind = "OverheadLine"
	KindCableLine    ConnectorKind = "CableLine"
	KindObjectLink   ConnectorKind = "ObjectLink"
)

// Connector is a drawn wire segment: a chain of points whose two ends
// resolve to electrical Nodes.
type Connector struct {
	ID      int           `xml:"id,attr"`
	Kind    ConnectorKind `xml:"kind,attr"`
	Voltage int           `xml:"voltage,attr,omitempty"`
	Layer   int           `xml:"layer,attr"`
	Dashed  bool          `xml:"dashed,attr,omitempty"`
	From    int           `xml:"from,attr"`
	To      int           `xml:"to,attr"`

	Points []Point `xml:"point"`
}

// Point is one X,Y coordinate in the diagram's own coordinate space.
type Point struct {
	X float64 `xml:"x,attr"`
	Y float64 `xml:"y,attr"`
}

// Label is a standalone text caption. For carries the ID of the Element it
// was matched to by name, when unambiguous; it is empty for an unmatched or
// ambiguous label, which still renders standalone.
type Label struct {
	For    int     `xml:"for,attr,omitempty"`
	Layer  int     `xml:"layer,attr"`
	X      float64 `xml:"x,attr"`
	Y      float64 `xml:"y,attr"`
	Size   float64 `xml:"size,attr"`
	Anchor string  `xml:"anchor,attr,omitempty"`
	Bold   bool    `xml:"bold,attr,omitempty"`
	Text   string  `xml:",chardata"`
}

// emptyElement matches a start tag immediately followed by its own end tag
// (encoding/xml never emits self-closing tags, even for elements with no
// content), so Save can collapse them into the shorter self-closing form.
var emptyElement = regexp.MustCompile(`<([A-Za-z][\w:.-]*)((?:\s+[A-Za-z_:][\w:.-]*="[^"]*")*)></([A-Za-z][\w:.-]*)>`)

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
	out := emptyElement.ReplaceAll(buf.Bytes(), []byte("<$1$2/>"))
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
	return &d, nil
}

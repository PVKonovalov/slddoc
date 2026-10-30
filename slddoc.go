// The API of the internal packages, re-exported; see each for documentation.

package slddoc

import (
	"io"

	"github.com/PVKonovalov/slddoc/internal/cim"
	"github.com/PVKonovalov/slddoc/internal/extract"
	"github.com/PVKonovalov/slddoc/internal/model"
	"github.com/PVKonovalov/slddoc/internal/render"
	"github.com/PVKonovalov/slddoc/internal/repair"
)

// From internal/cim.

type (
	CIMClassTable             = cim.CIMClassTable
	CIMExport                 = cim.CIMExport
	CIMHeader                 = cim.CIMHeader
	CIMImportOptions          = cim.CIMImportOptions
	CIMLayoutKind             = cim.CIMLayoutKind
	CIMOptions                = cim.CIMOptions
	CIMPage                   = cim.CIMPage
	CIMSameVoltageTransformer = cim.CIMSameVoltageTransformer
	CIMWarning                = cim.CIMWarning
	CIMWindingVoltage         = cim.CIMWindingVoltage
)

const (
	CIMAuthoritySet    = cim.CIMAuthoritySet
	CIMLayoutForeign   = cim.CIMLayoutForeign
	CIMLayoutGenerated = cim.CIMLayoutGenerated
	CIMLayoutOwn       = cim.CIMLayoutOwn
	LinkToObjectCode   = cim.LinkToObjectCode
)

var (
	ErrNoDiagramLayout = cim.ErrNoDiagramLayout
)

func CIMHeaders(docs [][]byte) []CIMHeader { return cim.CIMHeaders(docs) }

func CIMLayoutHeader(headers []CIMHeader) (CIMHeader, bool) { return cim.CIMLayoutHeader(headers) }

func CIMSameVoltageTransformers(docs [][]byte) ([]CIMSameVoltageTransformer, error) {
	return cim.CIMSameVoltageTransformers(docs)
}

func ExportCIM(d *Diagram, opt CIMOptions) *CIMExport { return cim.ExportCIM(d, opt) }

func ImportCIM(docs [][]byte, opt CIMImportOptions) ([]CIMPage, error) {
	return cim.ImportCIM(docs, opt)
}

func NewCIMClassTable(shapes map[string]Class, export map[string][]string, draw map[string]string) (CIMClassTable, []string, error) {
	return cim.NewCIMClassTable(shapes, export, draw)
}

func ParseVoltage(name string) (float64, bool) { return cim.ParseVoltage(name) }

// From internal/extract.

type (
	Report = extract.Report
)

func Extract(raw []byte, source string, voltageHints map[string]string) (*Diagram, Report, error) {
	return extract.Extract(raw, source, voltageHints)
}

func ObjectTypeName(code string) string { return extract.ObjectTypeName(code) }

// From internal/model.

type (
	Class              = model.Class
	Connector          = model.Connector
	ConnectorKind      = model.ConnectorKind
	ConnectorLineStyle = model.ConnectorLineStyle
	Diagram            = model.Diagram
	DiagramStateColor  = model.DiagramStateColor
	DigitalDevice      = model.DigitalDevice
	EditorSettings     = model.EditorSettings
	Electrical         = model.Electrical
	Element            = model.Element
	Label              = model.Label
	Layer              = model.Layer
	NeutralGrounding   = model.NeutralGrounding
	Node               = model.Node
	Point              = model.Point
	Port               = model.Port
	Revision           = model.Revision
	SubstationSector   = model.SubstationSector
	TableCell          = model.TableCell
	TerminalDirection  = model.TerminalDirection
	TransformerWinding = model.TransformerWinding
	VoltageClass       = model.VoltageClass
	WindingScheme      = model.WindingScheme
)

const (
	BaseLayer                   = model.BaseLayer
	ClassAnchorPole             = model.ClassAnchorPole
	ClassArc                    = model.ClassArc
	ClassArrow                  = model.ClassArrow
	ClassAutomationDevice       = model.ClassAutomationDevice
	ClassBlockingFilter         = model.ClassBlockingFilter
	ClassBooster                = model.ClassBooster
	ClassBreaker                = model.ClassBreaker
	ClassBusBarSection          = model.ClassBusBarSection
	ClassButton                 = model.ClassButton
	ClassCableConnector         = model.ClassCableConnector
	ClassCableJoint             = model.ClassCableJoint
	ClassCapacitor              = model.ClassCapacitor
	ClassCapacitorBank          = model.ClassCapacitorBank
	ClassChassis                = model.ClassChassis
	ClassChokeCoil              = model.ClassChokeCoil
	ClassCircle                 = model.ClassCircle
	ClassConnectorArrow         = model.ClassConnectorArrow
	ClassConnectorPoint         = model.ClassConnectorPoint
	ClassContainer              = model.ClassContainer
	ClassCurrentTransformer     = model.ClassCurrentTransformer
	ClassDisconnector           = model.ClassDisconnector
	ClassDisconnectorFuse       = model.ClassDisconnectorFuse
	ClassEnclosedSubstation     = model.ClassEnclosedSubstation
	ClassFaultPassageIndicator  = model.ClassFaultPassageIndicator
	ClassFork                   = model.ClassFork
	ClassFuse                   = model.ClassFuse
	ClassGenerator              = model.ClassGenerator
	ClassGround                 = model.ClassGround
	ClassGroundSwitch           = model.ClassGroundSwitch
	ClassHalfChassis            = model.ClassHalfChassis
	ClassJunctionPoint          = model.ClassJunctionPoint
	ClassKnifeSwitch            = model.ClassKnifeSwitch
	ClassKnifeSwitch3           = model.ClassKnifeSwitch3
	ClassLED                    = model.ClassLED
	ClassLamp                   = model.ClassLamp
	ClassLampOnPole             = model.ClassLampOnPole
	ClassLine                   = model.ClassLine
	ClassLoad                   = model.ClassLoad
	ClassLoadBreakSwitch        = model.ClassLoadBreakSwitch
	ClassNetworkInfeed          = model.ClassNetworkInfeed
	ClassNonIntersection        = model.ClassNonIntersection
	ClassPackageSubstation      = model.ClassPackageSubstation
	ClassPicture                = model.ClassPicture
	ClassPolygon                = model.ClassPolygon
	ClassPostPole               = model.ClassPostPole
	ClassPowerCircuitBreaker    = model.ClassPowerCircuitBreaker
	ClassPowerPlant             = model.ClassPowerPlant
	ClassPowerPole              = model.ClassPowerPole
	ClassPowerTransformer       = model.ClassPowerTransformer
	ClassPowerflowIndicator     = model.ClassPowerflowIndicator
	ClassReactor                = model.ClassReactor
	ClassReactorShunt           = model.ClassReactorShunt
	ClassRectangle              = model.ClassRectangle
	ClassResistor               = model.ClassResistor
	ClassRoad                   = model.ClassRoad
	ClassSectionalizer          = model.ClassSectionalizer
	ClassShortCircuiter         = model.ClassShortCircuiter
	ClassShortCircuiterNoGround = model.ClassShortCircuiterNoGround
	ClassSmallWindow            = model.ClassSmallWindow
	ClassStarter                = model.ClassStarter
	ClassStaticVarCompensator   = model.ClassStaticVarCompensator
	ClassSubstation             = model.ClassSubstation
	ClassSurgeArrester          = model.ClassSurgeArrester
	ClassSynchronousCompensator = model.ClassSynchronousCompensator
	ClassSynchronousMotor       = model.ClassSynchronousMotor
	ClassTable                  = model.ClassTable
	ClassTable2                 = model.ClassTable2
	ClassThyristor              = model.ClassThyristor
	ClassVoltageTransformer     = model.ClassVoltageTransformer
	ClassWindowIcon             = model.ClassWindowIcon
	GroundingIsolated           = model.GroundingIsolated
	GroundingResistor           = model.GroundingResistor
	GroundingSolid              = model.GroundingSolid
	KindBusWork                 = model.KindBusWork
	KindBusbarWire              = model.KindBusbarWire
	KindCableLine               = model.KindCableLine
	KindLinkToObject            = model.KindLinkToObject
	KindOverheadLine            = model.KindOverheadLine
	LineStyleDashDot            = model.LineStyleDashDot
	LineStyleDashed             = model.LineStyleDashed
	LineStyleDotted             = model.LineStyleDotted
	LineStyleSolid              = model.LineStyleSolid
	SchemeDelta                 = model.SchemeDelta
	SchemeWye                   = model.SchemeWye
	SchemeWyeN                  = model.SchemeWyeN
	TerminalBottom              = model.TerminalBottom
	TerminalLeft                = model.TerminalLeft
	TerminalRight               = model.TerminalRight
	TerminalTop                 = model.TerminalTop
)

func Load(r io.Reader) (*Diagram, error) { return model.Load(r) }

func LoadVoltageHints(r io.Reader) (map[string]string, error) { return model.LoadVoltageHints(r) }

func ScaledTerminal(t Point, step int) Point { return model.ScaledTerminal(t, step) }

func SizeFactor(step int) float64 { return model.SizeFactor(step) }

func UsesSizeStep(c Class) bool { return model.UsesSizeStep(c) }

// From internal/render.

type (
	RenderMode    = render.RenderMode
	StateColor    = render.StateColor
	SymbolLibrary = render.SymbolLibrary
)

const (
	Interactive = render.Interactive
	Static      = render.Static
)

func HasLeads(shape string) bool { return render.HasLeads(shape) }

func LeadTerminal(e Element, t Point) Point { return render.LeadTerminal(e, t) }

func LoadSymbolLibrary(r io.Reader) (*SymbolLibrary, error) { return render.LoadSymbolLibrary(r) }

func NewSymbolLibrary(templates map[string]string) *SymbolLibrary {
	return render.NewSymbolLibrary(templates)
}

func Render(d *Diagram, lib *SymbolLibrary, w io.Writer, mode RenderMode, defaultFPIText string, fpiStateColorLegend []StateColor, stateColorLegend ...StateColor) error {
	return render.Render(d, lib, w, mode, defaultFPIText, fpiStateColorLegend, stateColorLegend...)
}

func RenderFragments(d *Diagram, lib *SymbolLibrary, ids []int, mode RenderMode, defaultFPIText string, fpiStateColorLegend []StateColor, stateColorLegend ...StateColor) (map[int]string, error) {
	return render.RenderFragments(d, lib, ids, mode, defaultFPIText, fpiStateColorLegend, stateColorLegend...)
}

func SeedStateColors(d *Diagram, legend []StateColor) { render.SeedStateColors(d, legend) }

// From internal/repair.

var (
	SizeSteps = repair.SizeSteps
)

func EnsureIDs(d *Diagram) bool { return repair.EnsureIDs(d) }

func InferSizeSteps(d *Diagram, terminals map[string][]Point) bool {
	return repair.InferSizeSteps(d, terminals)
}

func MostUsedVoltage(d *Diagram) int { return repair.MostUsedVoltage(d) }

func Repair(d *Diagram, terminals map[string][]Point) bool { return repair.Repair(d, terminals) }

package slddoc

// Electrical is an element's or line's nameplate data, used by the CIM
// export. Names and units follow CIM (IEC 61970-301); each class uses only
// its own subset, and nil means unset:
//   - OverheadLine/CableLine (ACLineSegment): Length, R, X, Bch, Gch, R0, X0, B0ch, G0ch
//   - Booster: RatedS, RatedU, R, X
//   - Generator/SynchronousCompensator/SynchronousMotor (SynchronousMachine):
//     RatedS, RatedU, MinP, MaxP, MinQ, MaxQ
//   - Capacitor/CapacitorBank/ReactorShunt (LinearShuntCompensator):
//     NomU, BPerSection, GPerSection, MaximumSections, NormalSections
//   - Reactor/ChokeCoil (SeriesCompensator): R, X
//   - switching devices and fuses: RatedCurrent
//   - BusBarSection: IpMax
//   - Load (EnergyConsumer): P, Q
//   - NetworkInfeed (ExternalNetworkInjection and the like): MinP, MaxP, MinQ, MaxQ
//   - StaticVarCompensator: CapacitiveRating, InductiveRating
//
// A PowerTransformer keeps its parameters on each TransformerWinding.
type Electrical struct {
	// Length is in km; R, X, R0, X0 in Ω; Bch, Gch, B0ch, G0ch,
	// BPerSection, GPerSection in S (a line's values are for its whole length).
	Length *float64 `xml:"length,attr,omitempty" json:"length,omitempty"`
	R      *float64 `xml:"r,attr,omitempty" json:"r,omitempty"`
	X      *float64 `xml:"x,attr,omitempty" json:"x,omitempty"`
	Bch    *float64 `xml:"bch,attr,omitempty" json:"bch,omitempty"`
	Gch    *float64 `xml:"gch,attr,omitempty" json:"gch,omitempty"`
	R0     *float64 `xml:"r0,attr,omitempty" json:"r0,omitempty"`
	X0     *float64 `xml:"x0,attr,omitempty" json:"x0,omitempty"`
	B0ch   *float64 `xml:"b0ch,attr,omitempty" json:"b0ch,omitempty"`
	G0ch   *float64 `xml:"g0ch,attr,omitempty" json:"g0ch,omitempty"`

	// RatedS is in MVA, RatedU and NomU in kV, P in MW, Q in Mvar.
	RatedS *float64 `xml:"ratedS,attr,omitempty" json:"ratedS,omitempty"`
	RatedU *float64 `xml:"ratedU,attr,omitempty" json:"ratedU,omitempty"`
	MinP   *float64 `xml:"minP,attr,omitempty" json:"minP,omitempty"`
	MaxP   *float64 `xml:"maxP,attr,omitempty" json:"maxP,omitempty"`
	MinQ   *float64 `xml:"minQ,attr,omitempty" json:"minQ,omitempty"`
	MaxQ   *float64 `xml:"maxQ,attr,omitempty" json:"maxQ,omitempty"`

	NomU            *float64 `xml:"nomU,attr,omitempty" json:"nomU,omitempty"`
	BPerSection     *float64 `xml:"bPerSection,attr,omitempty" json:"bPerSection,omitempty"`
	GPerSection     *float64 `xml:"gPerSection,attr,omitempty" json:"gPerSection,omitempty"`
	MaximumSections *int     `xml:"maximumSections,attr,omitempty" json:"maximumSections,omitempty"`
	NormalSections  *int     `xml:"normalSections,attr,omitempty" json:"normalSections,omitempty"`

	// P (MW) and Q (Mvar) are a load's fixed active and reactive power.
	P *float64 `xml:"p,attr,omitempty" json:"p,omitempty"`
	Q *float64 `xml:"q,attr,omitempty" json:"q,omitempty"`

	// CapacitiveRating and InductiveRating are an SVC's reactances (Ω) at its limits.
	CapacitiveRating *float64 `xml:"capacitiveRating,attr,omitempty" json:"capacitiveRating,omitempty"`
	InductiveRating  *float64 `xml:"inductiveRating,attr,omitempty" json:"inductiveRating,omitempty"`

	// RatedCurrent and IpMax are in A.
	RatedCurrent *float64 `xml:"ratedCurrent,attr,omitempty" json:"ratedCurrent,omitempty"`
	IpMax        *float64 `xml:"ipMax,attr,omitempty" json:"ipMax,omitempty"`
}

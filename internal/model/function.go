package model

import (
	"fmt"
	"strings"
)

// FunctionKind is the kind of a functional unit (Function.Kind).
type FunctionKind string

const (
	FunctionVoltageLevel   FunctionKind = "voltageLevel"
	FunctionIncomingFeeder FunctionKind = "incomingFeeder"
	FunctionOutgoingFeeder FunctionKind = "outgoingFeeder"
	FunctionGenerator      FunctionKind = "generator"
	FunctionTransformer    FunctionKind = "transformer"
	FunctionBusCoupler     FunctionKind = "busCoupler"
	FunctionBusbar         FunctionKind = "busbar"
	FunctionMeasuring      FunctionKind = "measuring"
	FunctionOther          FunctionKind = "other"
)

// FunctionKinds lists every unit kind, voltage level first.
var FunctionKinds = []FunctionKind{
	FunctionVoltageLevel, FunctionIncomingFeeder, FunctionOutgoingFeeder, FunctionGenerator, FunctionTransformer,
	FunctionBusCoupler, FunctionBusbar, FunctionMeasuring, FunctionOther,
}

// Function is one node of a diagram's functional hierarchy (IEC 81346): a voltage level, or a unit (bay) in one.
// Its designation is its parent's followed by "=" Code Number, Number zero-padded to Digits.
type Function struct {
	ID     int          `xml:"id,attr" json:"id"`
	Parent int          `xml:"parent,attr,omitempty" json:"parent,omitempty"`
	Kind   FunctionKind `xml:"kind,attr" json:"kind"`
	Code   string       `xml:"code,attr" json:"code"`
	Number int          `xml:"number,attr" json:"number"`
	Digits int          `xml:"digits,attr,omitempty" json:"digits,omitempty"`
	// Voltage references the VoltageClass.ID of a voltage level; 0 for a unit.
	Voltage int `xml:"voltage,attr,omitempty" json:"voltage,omitempty"`
	// Pair references the unit on a transformer's other side (both ways), so the two read as one transformer.
	Pair int `xml:"pair,attr,omitempty" json:"pair,omitempty"`
	// Guessed marks a feeder whose incoming/outgoing kind was guessed and not yet confirmed.
	Guessed bool `xml:"guessed,attr,omitempty" json:"guessed,omitempty"`
}

// Own returns f's own designation part, e.g. "=H03".
func (f Function) Own() string {
	return fmt.Sprintf("=%s%0*d", f.Code, max(f.Digits, 1), f.Number)
}

// FunctionDesignation returns the full designation of function id in fs, e.g. "=E1=H03"; "" for an unknown id.
// A parent cycle is cut where it closes.
func FunctionDesignation(fs []Function, id int) string {
	byID := make(map[int]Function, len(fs))
	for _, f := range fs {
		byID[f.ID] = f
	}
	var parts []string
	seen := map[int]bool{}
	for id != 0 && !seen[id] {
		f, ok := byID[id]
		if !ok {
			break
		}
		seen[id] = true
		parts = append(parts, f.Own())
		id = f.Parent
	}
	var b strings.Builder
	for i := len(parts) - 1; i >= 0; i-- {
		b.WriteString(parts[i])
	}
	return b.String()
}

// ProductPart returns the product part of designation name, from its first "-" (e.g. "-QA1"); "" when it has none.
func ProductPart(name string) string {
	if i := strings.IndexByte(name, '-'); i >= 0 {
		return name[i:]
	}
	return ""
}

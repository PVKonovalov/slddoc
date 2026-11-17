package model

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"time"
)

// BindingKind is what a Binding supplies to its item, matching the tabs of RSDU's scheme editor.
type BindingKind string

const (
	BindingEquipment BindingKind = "equipment" // the equipment the item stands for
	BindingMeas1     BindingKind = "meas1"     // main parameter ("Parameters")
	BindingMeas2     BindingKind = "meas2"     // additional parameter ("Additional parameters")
	BindingJump      BindingKind = "jump"      // another scheme, panel or report opened from the item
)

// Binding links a diagram item to an object of a SCADA data provider.
type Binding struct {
	// Source names the provider, e.g. "rsdu".
	Source string      `xml:"source,attr" json:"source"`
	Kind   BindingKind `xml:"kind,attr" json:"kind"`
	// Ref is the provider's key of the object, opaque outside the provider (RSDU: "tableId:id").
	Ref string `xml:"ref,attr" json:"ref"`
	// Option says how the value is read (RSDU: SYS_GTOPT.DEFINE_ALIAS).
	Option string `xml:"option,attr,omitempty" json:"option,omitempty"`
	// Scale multiplies the value; 0 means 1, a negative value inverts a discrete one.
	Scale int `xml:"scale,attr,omitempty" json:"scale,omitempty"`
	// Name, Type and UID are cached from the provider for display without a connection.
	Name string `xml:"name,attr,omitempty" json:"name,omitempty"`
	Type string `xml:"type,attr,omitempty" json:"type,omitempty"`
	UID  string `xml:"uid,attr,omitempty" json:"uid,omitempty"`
}

// SchemeRef identifies the scheme a diagram is exported to at a provider (RSDU: VS_FORM.ID).
type SchemeRef struct {
	Source string `xml:"source,attr" json:"source"`
	Ref    string `xml:"ref,attr" json:"ref"`
	// Exported is when the diagram was last exported to the scheme; nil = never.
	Exported *time.Time `xml:"exported,attr,omitempty" json:"exported,omitempty"`
	// Registered tells that the scheme was registered by an export of this diagram, so its file name follows the
	// diagram's path; false for a scheme linked from the provider.
	Registered bool `xml:"registered,attr,omitempty" json:"registered,omitempty"`
}

// ReadScheme reads only the <scheme> of the .xsld in r, without parsing the rest: it stops at the first child of
// <diagram> that Save writes after <scheme>. It returns nil when the diagram has no scheme.
func ReadScheme(r io.Reader) (*SchemeRef, error) {
	dec := xml.NewDecoder(r)
	depth := 0
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			if _, end := tok.(xml.EndElement); end {
				depth--
			}
			continue
		}
		depth++
		if depth == 1 {
			if start.Name.Local != "diagram" {
				return nil, fmt.Errorf("not a diagram: <%s>", start.Name.Local)
			}
			continue
		}
		switch start.Name.Local {
		case "scheme":
			var s SchemeRef
			if err := dec.DecodeElement(&s, &start); err != nil {
				return nil, err
			}
			return &s, nil
		case "editor":
			if err := dec.Skip(); err != nil {
				return nil, err
			}
			depth--
		default:
			return nil, nil
		}
	}
}

var bindingKinds = map[BindingKind]bool{
	BindingEquipment: true, BindingMeas1: true, BindingMeas2: true, BindingJump: true,
}

// ValidateBindings checks every binding of d: a known kind, a non-empty source and ref, and at most one binding of
// each kind per item. It returns all problems found joined into one error, or nil.
func ValidateBindings(d *Diagram) error {
	var errs []error
	check := func(item string, bs []Binding) {
		seen := map[BindingKind]bool{}
		for _, b := range bs {
			if err := validateBinding(b); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", item, err))
				continue
			}
			if seen[b.Kind] {
				errs = append(errs, fmt.Errorf("%s: more than one %q binding", item, b.Kind))
			}
			seen[b.Kind] = true
		}
	}
	if d.Object != nil {
		check("diagram object", []Binding{*d.Object})
	}
	if d.Scheme != nil && (d.Scheme.Source == "" || d.Scheme.Ref == "") {
		errs = append(errs, errors.New("diagram scheme: empty source or ref"))
	}
	for _, e := range d.Elements {
		check(fmt.Sprintf("element %d", e.ID), e.Bindings)
	}
	for _, dd := range d.DigitalDevices {
		check(fmt.Sprintf("digital device %d", dd.ID), dd.Bindings)
	}
	return errors.Join(errs...)
}

func validateBinding(b Binding) error {
	if !bindingKinds[b.Kind] {
		return fmt.Errorf("unknown binding kind %q", b.Kind)
	}
	if b.Source == "" || b.Ref == "" {
		return fmt.Errorf("%q binding: empty source or ref", b.Kind)
	}
	return nil
}

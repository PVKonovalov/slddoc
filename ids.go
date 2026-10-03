package slddoc

// assignUniqueIDs makes every id Extract produced unique across the shared
// id space (Node, VoltageClass, Element, Connector, Label, DigitalDevice,
// Layer). Nodes and voltage classes are Extract's own (numbered 1..N by
// buildTopology/buildVoltageClasses, which collides with the source's own
// small ids), so they are all renumbered from next, every reference
// following. Then an element, connector, digital device or label whose id
// is unset or already taken (the source writes a Powerflow arrow and its
// reading under one id) gets a fresh one; elements come first, so a label's
// For keeps pointing at the element it was matched to. Layer ids are kept
// and never reused. Returns the next free id.
func assignUniqueIDs(d *Diagram, next int) int {
	used := map[int]bool{}
	for _, l := range d.Layers {
		used[l.ID] = true
	}
	fresh := func() int {
		for used[next] {
			next++
		}
		id := next
		used[id] = true
		next++
		return id
	}

	nodeID := make(map[int]int, len(d.Nodes))
	for i := range d.Nodes {
		id := fresh()
		nodeID[d.Nodes[i].ID] = id
		d.Nodes[i].ID = id
	}
	for i := range d.Elements {
		for j := range d.Elements[i].Ports {
			if id, ok := nodeID[d.Elements[i].Ports[j].Node]; ok {
				d.Elements[i].Ports[j].Node = id
			}
		}
	}
	for i := range d.Connectors {
		if id, ok := nodeID[d.Connectors[i].From]; ok {
			d.Connectors[i].From = id
		}
		if id, ok := nodeID[d.Connectors[i].To]; ok {
			d.Connectors[i].To = id
		}
	}

	classID := make(map[int]int, len(d.VoltageClasses))
	for i := range d.VoltageClasses {
		id := fresh()
		classID[d.VoltageClasses[i].ID] = id
		d.VoltageClasses[i].ID = id
	}
	remap := func(v *int) {
		if id, ok := classID[*v]; ok && *v != 0 {
			*v = id
		}
	}
	for i := range d.Elements {
		e := &d.Elements[i]
		remap(&e.Voltage)
		for j := range e.Windings {
			remap(&e.Windings[j].Voltage)
		}
		for j := range e.Sectors {
			remap(&e.Sectors[j].Voltage)
		}
	}
	for i := range d.Connectors {
		remap(&d.Connectors[i].Voltage)
	}
	if d.Editor != nil {
		remap(&d.Editor.DefaultVoltage)
	}

	unique := func(id *int) {
		if *id == 0 || used[*id] {
			*id = fresh()
			return
		}
		used[*id] = true
	}
	for i := range d.Elements {
		unique(&d.Elements[i].ID)
	}
	for i := range d.Connectors {
		unique(&d.Connectors[i].ID)
	}
	for i := range d.DigitalDevices {
		unique(&d.DigitalDevices[i].ID)
	}
	for i := range d.Labels {
		unique(&d.Labels[i].ID)
	}
	return next
}

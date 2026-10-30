package cim

import (
	"fmt"
	"slices"
	"strconv"
)

// CIMSameVoltageTransformer is a transformer whose windings all have one voltage, which no real transformer has: the
// model most likely wrote one BaseVoltage for every end. Its windings are in end-number order.
type CIMSameVoltageTransformer struct {
	MRID     string              `json:"mrid"`
	Name     string              `json:"name"`
	Windings []CIMWindingVoltage `json:"windings"`
}

// CIMWindingVoltage is one winding of a CIMSameVoltageTransformer: its PowerTransformerEnd's mRID, end number, name,
// the voltage the model gives it (kV) and its rated voltage (kV, nil when the model has none).
type CIMWindingVoltage struct {
	MRID   string   `json:"mrid"`
	End    int      `json:"end"`
	Name   string   `json:"name,omitempty"`
	KV     float64  `json:"kv"`
	RatedU *float64 `json:"ratedU,omitempty"`
}

// CIMSameVoltageTransformers lists the transformers of CIM documents docs whose windings (two or more) all have one
// voltage, for CIMImportOptions.WindingVoltages to be asked before the import.
func CIMSameVoltageTransformers(docs [][]byte) ([]CIMSameVoltageTransformer, error) {
	im, err := newCIMImporter(docs, CIMImportOptions{})
	if err != nil {
		return nil, err
	}
	return im.sameVoltageTransformers(), nil
}

func (im *cimImporter) sameVoltageTransformers() []CIMSameVoltageTransformer {
	var out []CIMSameVoltageTransformer
	for _, tr := range im.byClass["PowerTransformer"] {
		ends := im.transformerEnds(tr)
		if len(ends) < 2 {
			continue
		}
		kvs := make([]float64, len(ends))
		for i, end := range ends {
			kvs[i] = im.endKV(end)
		}
		if kvs[0] <= 0 || slices.ContainsFunc(kvs, func(kv float64) bool { return kv != kvs[0] }) {
			continue
		}
		t := CIMSameVoltageTransformer{MRID: mridOf(tr), Name: tr.get("name")}
		for i, end := range ends {
			n, _ := end.num("endNumber")
			t.Windings = append(t.Windings, CIMWindingVoltage{MRID: mridOf(end), End: int(n), Name: end.get("name"),
				KV: kvs[i], RatedU: end.numPtr("ratedU")})
		}
		out = append(out, t)
	}
	return out
}

// endBV is transformer end end's BaseVoltage: its own, else its terminal's voltage level's.
func (im *cimImporter) endBV(end *cimObject) string {
	if bv := end.get("BaseVoltage"); bv != "" {
		return bv
	}
	if t := im.objs[end.get("Terminal")]; t != nil {
		if cn := im.objs[t.get("ConnectivityNode")]; cn != nil {
			if c := im.objs[cn.get("ConnectivityNodeContainer")]; c != nil {
				return im.containerBV(c)
			}
		}
	}
	return ""
}

func (im *cimImporter) endKV(end *cimObject) float64 {
	kv, _ := im.objs[im.endBV(end)].num("nominalVoltage")
	return kv
}

// applyWindingVoltages gives each transformer end named (by mRID) in kvs that voltage, and everything joined to it
// up to the next transformer along with it: its nodes and equipment move to a voltage level of that voltage beside
// their own, a line takes it as its base voltage. A side joined back to another winding of its own transformer
// keeps its voltage (only the winding takes the new one), noted in im.notes.
func (im *cimImporter) applyWindingVoltages(kvs map[string]float64) {
	if len(kvs) == 0 {
		return
	}
	atCN := map[string][]*cimObject{}
	for _, list := range im.terminals {
		for _, t := range list {
			if cn := t.get("ConnectivityNode"); cn != "" {
				atCN[cn] = append(atCN[cn], t)
			}
		}
	}
	for _, tr := range im.byClass["PowerTransformer"] {
		ends := im.transformerEnds(tr)
		for _, end := range ends {
			kv, ok := kvs[mridOf(end)]
			if !ok || kv <= 0 || kv == im.endKV(end) {
				continue
			}
			bv := im.baseVoltageOf(kv)
			end.props["BaseVoltage"] = []string{bv}
			t := im.objs[end.get("Terminal")]
			if t == nil {
				continue
			}
			own := map[string]bool{}
			for _, other := range ends {
				if other != end {
					if ot := im.objs[other.get("Terminal")]; ot != nil {
						own[ot.get("ConnectivityNode")] = true
					}
				}
			}
			cns, equipment, ok := im.side(t.get("ConnectivityNode"), own, atCN)
			if !ok {
				im.notes = append(im.notes, fmt.Sprintf("transformer %q: winding %q is joined to another of its windings "+
					"without a transformer between, so only the winding takes %s kV", tr.get("name"),
					cmpName(end), formatKV(kv)))
				continue
			}
			for _, cn := range cns {
				if c := im.objs[cn.get("ConnectivityNodeContainer")]; c != nil && im.containerBV(c) != bv {
					cn.props["ConnectivityNodeContainer"] = []string{im.voltageLevelBeside(c, bv, kv)}
				}
			}
			for _, eq := range equipment {
				if eq.get("BaseVoltage") != "" {
					eq.props["BaseVoltage"] = []string{bv}
				}
				if c := im.objs[eq.get("EquipmentContainer")]; c != nil && (c.class == "VoltageLevel" || c.class == "Bay") &&
					im.containerBV(c) != bv {
					eq.props["EquipmentContainer"] = []string{im.voltageLevelBeside(c, bv, kv)}
				}
			}
		}
	}
}

// side is what is joined to node cn without passing a transformer: its nodes and equipment. ok is false when it
// reaches a node in stop (another winding of the same transformer).
func (im *cimImporter) side(cn string, stop map[string]bool, atCN map[string][]*cimObject) (cns, equipment []*cimObject, ok bool) {
	seen := map[string]bool{cn: true}
	queue := []string{cn}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if stop[id] {
			return nil, nil, false
		}
		if o := im.objs[id]; o != nil {
			cns = append(cns, o)
		}
		for _, t := range atCN[id] {
			eq := im.objs[t.get("ConductingEquipment")]
			if eq == nil || eq.class == "PowerTransformer" || seen[eq.id] {
				continue
			}
			seen[eq.id] = true
			equipment = append(equipment, eq)
			for _, et := range im.terminals[eq.id] {
				if next := et.get("ConnectivityNode"); next != "" && !seen[next] {
					seen[next] = true
					queue = append(queue, next)
				}
			}
		}
	}
	return cns, equipment, true
}

// baseVoltageOf is the id of a BaseVoltage of kv kV: the model's own, else one made for it.
func (im *cimImporter) baseVoltageOf(kv float64) string {
	for _, bv := range im.byClass["BaseVoltage"] {
		if v, _ := bv.num("nominalVoltage"); v == kv {
			return bv.id
		}
	}
	id := "_winding-bv-" + formatKV(kv)
	if _, ok := im.objs[id]; !ok {
		im.add(&cimObject{id: id, class: "BaseVoltage", props: map[string][]string{"nominalVoltage": {formatKV(kv)},
			"name": {formatKV(kv) + " kV"}}})
	}
	return id
}

// voltageLevelBeside is the id of a voltage level of base voltage bv (kv kV) in the substation of container c (a
// voltage level or a bay), made on first use and named after its voltage.
func (im *cimImporter) voltageLevelBeside(c *cimObject, bv string, kv float64) string {
	vl := c
	if c.class == "Bay" {
		if v := im.objs[c.get("VoltageLevel")]; v != nil {
			vl = v
		}
	}
	id := vl.id + "-winding-" + formatKV(kv)
	if _, ok := im.objs[id]; !ok {
		im.add(&cimObject{id: id, class: "VoltageLevel", props: map[string][]string{"name": {formatKV(kv) + " kV"},
			"Substation": {vl.get("Substation")}, "BaseVoltage": {bv}}})
	}
	return id
}

func (im *cimImporter) add(o *cimObject) {
	im.objs[o.id] = o
	im.byClass[o.class] = append(im.byClass[o.class], o)
}

func formatKV(kv float64) string { return strconv.FormatFloat(kv, 'f', -1, 64) }

func cmpName(o *cimObject) string {
	if n := o.get("name"); n != "" {
		return n
	}
	return mridOf(o)
}

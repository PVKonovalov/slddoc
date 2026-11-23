package designate

import (
	"slices"
	"testing"

	"github.com/PVKonovalov/slddoc/internal/model"
)

var classOf = map[string]model.Class{
	"41": model.ClassBreaker, "162": model.ClassDisconnector, "54": model.ClassGroundSwitch,
	"47": model.ClassPowerTransformer, "55": model.ClassVoltageTransformer, "24": model.ClassBusBarSection,
	"173": model.ClassGenerator, "330001": model.ClassLoad, "330002": model.ClassNetworkInfeed,
}

var testOptions = Options{
	LetterCodes: map[string]string{
		"41": "QA", "162": "QB", "54": "QC", "47": "TA", "55": "BA", "24": "WC", "173": "GA",
	},
	VoltageLevels:       []VoltageCode{{MaxKV: 1, Code: "M"}, {MaxKV: 10, Code: "J"}, {MaxKV: 110, Code: "E"}},
	FallbackVoltageCode: "U",
	Units: map[model.FunctionKind]UnitCode{
		model.FunctionIncomingFeeder: {"N", 2}, model.FunctionOutgoingFeeder: {"H", 2},
		model.FunctionGenerator: {"G", 1}, model.FunctionTransformer: {"T", 1}, model.FunctionBusCoupler: {"Z", 1},
		model.FunctionBusbar: {"W", 1}, model.FunctionMeasuring: {"Y", 1}, model.FunctionOther: {"X", 2},
	},
	Grid: 10,
}

type builder struct {
	d    *model.Diagram
	next int
}

func newBuilder() *builder {
	return &builder{d: &model.Diagram{Width: 1000, Height: 1000, VoltageClasses: []model.VoltageClass{
		{ID: 1, Name: "110 kV", Color: "#ff0000"}, {ID: 2, Name: "10 kV", Color: "#00ff00"},
	}}, next: 10}
}

func (b *builder) id() int {
	b.next++
	b.d.LastID = b.next
	return b.next
}

func (b *builder) node(x, y float64) int {
	id := b.id()
	b.d.Nodes = append(b.d.Nodes, model.Node{ID: id, X: x, Y: y})
	return id
}

func (b *builder) element(shape string, x, y float64, voltage int, nodes ...int) int {
	e := model.Element{ID: b.id(), Class: classOf[shape], Shape: shape, X: x, Y: y, Voltage: voltage}
	for i, n := range nodes {
		e.Ports = append(e.Ports, model.Port{Name: string(rune('1' + i)), Node: n})
	}
	b.d.Elements = append(b.d.Elements, e)
	return e.ID
}

// busbar draws a horizontal busbar at y from x1 to x2 with a connection point at each of xs; returns their nodes.
func (b *builder) busbar(x1, x2, y float64, voltage int, xs ...float64) []int {
	var nodes []int
	for _, x := range xs {
		nodes = append(nodes, b.node(x, y))
	}
	id := b.element("24", (x1+x2)/2, y, voltage, nodes...)
	b.d.Elements[len(b.d.Elements)-1].Points = []model.Point{{X: x1, Y: y}, {X: x2, Y: y}}
	_ = id
	return nodes
}

// chain hangs two-terminal devices downward from node `from` at x, 40 apart; returns the last node.
func (b *builder) chain(from int, x, y float64, voltage int, shapes ...string) int {
	for _, s := range shapes {
		to := b.node(x, y+40)
		b.element(s, x, y+20, voltage, from, to)
		from, y = to, y+40
	}
	return from
}

func (b *builder) wire(kind model.ConnectorKind, from, to int) {
	b.d.Connectors = append(b.d.Connectors, model.Connector{ID: b.id(), Kind: kind, From: from, To: to})
}

func (b *builder) line(from int, voltage int) {
	to := b.node(0, 0)
	b.d.Connectors = append(b.d.Connectors, model.Connector{ID: b.id(), Kind: model.KindOverheadLine, From: from, To: to, Voltage: voltage})
}

// apply writes p to d the way the editor does: new function ids from d.LastID.
func apply(d *model.Diagram, p Plan) {
	ids := map[int]int{}
	for _, f := range p.Functions {
		if f.ID < 0 {
			d.LastID++
			ids[f.ID] = d.LastID
		}
	}
	remap := func(id int) int {
		if n, ok := ids[id]; ok {
			return n
		}
		return id
	}
	d.Functions = nil
	for _, f := range p.Functions {
		f.ID, f.Parent, f.Pair = remap(f.ID), remap(f.Parent), remap(f.Pair)
		d.Functions = append(d.Functions, f)
	}
	for _, r := range p.Rows {
		for i := range d.Elements {
			if d.Elements[i].ID == r.Element {
				d.Elements[i].Name, d.Elements[i].Function = r.Name, remap(r.Function)
			}
		}
	}
}

func names(d *model.Diagram) map[string]bool {
	m := map[string]bool{}
	for _, e := range d.Elements {
		if e.Name != "" {
			m[e.Name] = true
		}
	}
	return m
}

func expect(t *testing.T, d *model.Diagram, want ...string) {
	t.Helper()
	got := names(d)
	for _, w := range want {
		if !got[w] {
			t.Errorf("missing %s", w)
		}
	}
	if len(got) != len(want) {
		var all []string
		for n := range got {
			all = append(all, n)
		}
		slices.Sort(all)
		t.Errorf("got %d names, want %d: %v", len(got), len(want), all)
	}
}

func function(d *model.Diagram, designation string) *model.Function {
	for i, f := range d.Functions {
		if model.FunctionDesignation(d.Functions, f.ID) == designation {
			return &d.Functions[i]
		}
	}
	return nil
}

// substation: a 110 kV busbar with a line feeder and a transformer bay; on 10 kV the transformer's side, a line
// feeder, a load feeder and a VT bay.
func substation() (*builder, int) {
	b := newBuilder()
	hv := b.busbar(0, 400, 100, 1, 100, 300)
	b.line(b.chain(hv[0], 100, 100, 1, "162", "41"), 1)
	t := b.chain(hv[1], 300, 100, 1, "162", "41")
	lv := b.node(300, 300)
	b.element("47", 300, 220, 0, t, lv)
	lvBus := b.busbar(0, 400, 400, 2, 100, 200, 300, 350)
	b.wire(model.KindBusWork, b.chain(lv, 300, 300, 2, "41"), lvBus[2])
	b.line(b.chain(lvBus[0], 100, 400, 2, "162", "41"), 2)
	b.element("330001", 200, 500, 2, b.chain(lvBus[1], 200, 400, 2, "162", "41"))
	b.chain(lvBus[3], 350, 400, 2, "162", "55")
	return b, t
}

func TestDesignate_SingleBusbar(t *testing.T) {
	b, _ := substation()
	p := Designate(b.d, testOptions)
	if len(p.Unplaced) != 0 {
		t.Errorf("unplaced %v", p.Unplaced)
	}
	apply(b.d, p)
	expect(t, b.d,
		"=E1=W1-WC1", "=E1=N01-QB1", "=E1=N01-QA1", "=E1=T1-QB1", "=E1=T1-QA1", "=E1=T1-TA1",
		"=J1=T1-QA1", "=J1=W1-WC1", "=J1=H01-QB1", "=J1=H01-QA1", "=J1=H02-QB1", "=J1=H02-QA1",
		"=J1=Y1-QB1", "=J1=Y1-BA1",
	)
	if f := function(b.d, "=E1=N01"); f == nil || !f.Guessed || f.Kind != model.FunctionIncomingFeeder {
		t.Errorf("the 110 kV line feeder should be a guessed incoming feeder: %+v", f)
	}
	if f := function(b.d, "=J1=H02"); f == nil || f.Guessed {
		t.Errorf("the load feeder should be a sure outgoing feeder: %+v", f)
	}
	hv, lv := function(b.d, "=E1=T1"), function(b.d, "=J1=T1")
	if hv == nil || lv == nil || hv.Pair != lv.ID || lv.Pair != hv.ID {
		t.Errorf("transformer units should be paired: %+v %+v", hv, lv)
	}
	if e1 := function(b.d, "=E1"); e1 == nil || e1.Voltage != 1 || e1.Kind != model.FunctionVoltageLevel {
		t.Errorf("110 kV level: %+v", e1)
	}
}

func TestDesignate_Stable(t *testing.T) {
	b, _ := substation()
	apply(b.d, Designate(b.d, testOptions))
	if p := Designate(b.d, testOptions); len(p.Rows) != 0 || len(p.Functions) != len(b.d.Functions) {
		t.Fatalf("a second run should change nothing: %+v", p.Rows)
	}
	var qa *model.Element
	for i := range b.d.Elements {
		if b.d.Elements[i].Name == "=J1=H02-QA1" {
			qa = &b.d.Elements[i]
		}
	}
	n1, n2 := qa.Ports[0].Node, qa.Ports[1].Node
	first := b.element("54", 220, 440, 2, n1)
	b.element("54", 220, 480, 2, n2)
	p := Designate(b.d, testOptions)
	if len(p.Rows) != 2 || p.Rows[0].Name != "=J1=H02-QC1" || p.Rows[1].Name != "=J1=H02-QC2" {
		t.Fatalf("the new ground switches, along the feeder: %+v", p.Rows)
	}
	apply(b.d, p)
	b.d.Elements = slices.DeleteFunc(b.d.Elements, func(e model.Element) bool { return e.ID == first })
	if p := Designate(b.d, testOptions); len(p.Rows) != 0 {
		t.Fatalf("deleting an element must not renumber the others: %+v", p.Rows)
	}
	again := b.element("54", 220, 440, 2, n1)
	p = Designate(b.d, testOptions)
	if len(p.Rows) != 1 || p.Rows[0].Name != "=J1=H02-QC3" {
		t.Fatalf("a new element takes the next free number: %+v", p.Rows)
	}
	apply(b.d, p)
	renumber := testOptions
	renumber.Renumber = true
	p = Designate(b.d, renumber)
	if len(p.Rows) != 1 || p.Rows[0].Element != again || p.Rows[0].Name != "=J1=H02-QC1" {
		t.Errorf("renumbering closes the gap: %+v", p.Rows)
	}
	if len(p.Functions) != len(b.d.Functions) {
		t.Errorf("renumbering should keep the units: %d, was %d", len(p.Functions), len(b.d.Functions))
	}
}

func TestDesignate_Flags(t *testing.T) {
	b, _ := substation()
	b.d.Elements[1].Name, b.d.Elements[1].MRID = "Q1", "m1"
	b.d.Elements[2].Bindings = []model.Binding{{Source: "rsdu", Kind: model.BindingEquipment, Ref: "1:1"}}
	p := Designate(b.d, testOptions)
	flags := map[int]Row{}
	for _, r := range p.Rows {
		flags[r.Element] = r
	}
	if r := flags[b.d.Elements[1].ID]; !r.MRID || r.OldName != "Q1" {
		t.Errorf("mrid row: %+v", r)
	}
	if r := flags[b.d.Elements[2].ID]; !r.Bound || !r.Exported {
		t.Errorf("bound row: %+v", r)
	}
}

func TestDesignate_DoubleBusbar(t *testing.T) {
	b := newBuilder()
	w1a := b.busbar(0, 200, 100, 2, 100, 180)
	w1b := b.busbar(220, 400, 100, 2, 240, 300)
	w2 := b.busbar(0, 400, 200, 2, 100, 300)
	mid := b.node(210, 100)
	b.element("162", 195, 100, 2, w1a[1], mid)
	b.element("162", 225, 100, 2, mid, w1b[0])
	// A feeder on both systems through two busbar selectors.
	j := b.node(100, 300)
	b.element("162", 90, 150, 2, w1a[0], j)
	b.element("162", 110, 250, 2, w2[0], j)
	b.element("330001", 100, 400, 2, b.chain(j, 100, 300, 2, "41"))
	// The coupler.
	b.wire(model.KindBusWork, b.chain(w1b[1], 300, 100, 2, "162", "41"), w2[1])
	apply(b.d, Designate(b.d, testOptions))
	expect(t, b.d,
		"=J1=W1-WC1", "=J1=W1-WC2", "=J1=W2-WC1",
		"=J1=Z1-QB1", "=J1=Z1-QB2", "=J1=Z2-QB1", "=J1=Z2-QA1",
		"=J1=H01-QB1", "=J1=H01-QB2", "=J1=H01-QA1",
	)
}

func TestDesignate_Generator(t *testing.T) {
	b := newBuilder()
	bus := b.busbar(0, 400, 100, 2, 100, 200)
	b.element("173", 100, 200, 2, b.chain(bus[0], 100, 100, 2, "162", "41"))
	b.element("330002", 200, 200, 2, b.chain(bus[1], 200, 100, 2, "41"))
	apply(b.d, Designate(b.d, testOptions))
	expect(t, b.d, "=J1=W1-WC1", "=J1=G1-QB1", "=J1=G1-QA1", "=J1=G1-GA1", "=J1=N01-QA1")
	if f := function(b.d, "=J1=N01"); f == nil || f.Guessed {
		t.Errorf("a network infeed feeder is surely incoming: %+v", f)
	}
}

func TestDesignate_Scope(t *testing.T) {
	b, _ := substation()
	opt := testOptions
	opt.Scope = []int{b.d.Elements[1].ID}
	p := Designate(b.d, opt)
	if len(p.Rows) != 1 || p.Rows[0].Name != "=E1=N01-QB1" {
		t.Errorf("only the selected element: %+v", p.Rows)
	}
	created := 0
	for _, f := range p.Functions {
		if f.ID < 0 {
			created++
		}
	}
	if created != 2 {
		t.Errorf("only its unit and level should be created, got %d", created)
	}
}

func TestRowRanks(t *testing.T) {
	ps := []model.Point{{X: 50, Y: 102}, {X: 10, Y: 100}, {X: 0, Y: 200}, {X: 30, Y: 98}}
	if got := rowRanks(ps, 5); !slices.Equal(got, []int{2, 0, 3, 1}) {
		t.Errorf("rowRanks = %v", got)
	}
}

func TestDesignate_UniqueNumbers(t *testing.T) {
	b, _ := substation()
	opt := testOptions
	opt.UniqueNumbers = true
	apply(b.d, Designate(b.d, opt))
	expect(t, b.d,
		"=E1=W1-WC1", "=E1=N01-QB1", "=E1=N01-QA1", "=E1=T1-QB2", "=E1=T1-QA2", "=E1=T1-TA1",
		"=J1=T1-QA5", "=J1=W1-WC2", "=J1=H01-QB3", "=J1=H01-QA3", "=J1=H02-QB4", "=J1=H02-QA4",
		"=J1=Y1-QB5", "=J1=Y1-BA1",
	)
	if p := Designate(b.d, opt); len(p.Rows) != 0 {
		t.Errorf("a second run should change nothing: %+v", p.Rows)
	}
	// Per-unit numbers repeat a product part: renumbering makes them unique, keeping the units.
	perUnit, _ := substation()
	apply(perUnit.d, Designate(perUnit.d, testOptions))
	missing := Designate(perUnit.d, opt)
	renumber := opt
	renumber.Renumber = true
	if len(missing.Rows) == 0 {
		t.Error("repeated product parts should be renumbered even when only missing ones are asked for")
	}
	apply(perUnit.d, Designate(perUnit.d, renumber))
	seen := map[string]bool{}
	for _, e := range perUnit.d.Elements {
		if part := model.ProductPart(e.Name); part != "" {
			if seen[part] {
				t.Errorf("product part %s repeats", part)
			}
			seen[part] = true
		}
	}
}

func TestDesignate_Isolated(t *testing.T) {
	b, _ := substation()
	a, z := b.node(600, 100), b.node(600, 140)
	b.element("41", 600, 120, 2, a, z)
	isolated := 0
	for _, r := range Designate(b.d, testOptions).Rows {
		if r.Isolated {
			isolated++
		}
	}
	if isolated != 1 {
		t.Errorf("only the breaker joined to nothing is isolated, got %d", isolated)
	}
	g := newBuilder()
	n1, n2 := g.node(0, 0), g.node(0, 40)
	g.element("41", 0, 20, 2, n1, n2)
	if r := Designate(g.d, testOptions).Rows; len(r) != 1 || r[0].Isolated {
		t.Errorf("a diagram without busbars warns of nothing: %+v", r)
	}
}

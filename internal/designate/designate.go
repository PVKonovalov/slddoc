// Package designate builds a diagram's functional hierarchy (IEC 81346) from its topology and gives its equipment
// reference designations such as =E1=T1-QA1.
package designate

import (
	"cmp"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/PVKonovalov/slddoc/internal/cim"
	"github.com/PVKonovalov/slddoc/internal/model"
)

// VoltageCode gives the voltage levels up to MaxKV (and above the previous range) the letter Code.
type VoltageCode struct {
	MaxKV float64
	Code  string
}

// UnitCode is a unit kind's letter Code and the zero-padded width of its number.
type UnitCode struct {
	Code   string
	Digits int
}

// Options steers Designate.
//   - LetterCodes: equipment shape → IEC 81346-2 letter code; a shape not listed is not designated.
//   - VoltageLevels: voltage level letters, ascending by MaxKV; a level matching none gets FallbackVoltageCode.
//   - Units: each unit kind's code.
//   - Scope: the element ids to designate; empty means all.
//   - Renumber: renumber every element in scope (and, for the whole diagram, every unit) instead of only the ones
//     without a designation.
//   - UniqueNumbers: number each letter code across the whole diagram, not per unit, so the product part alone
//     (-QA7) names an element.
//   - Grid: the grid step; reading-order rows are half of it high.
type Options struct {
	LetterCodes         map[string]string
	VoltageLevels       []VoltageCode
	FallbackVoltageCode string
	Units               map[model.FunctionKind]UnitCode
	Scope               []int
	Renumber            bool
	UniqueNumbers       bool
	Grid                float64
}

// Row is one element's proposed designation. Bound, MRID and Exported tell that renaming it may break something
// outside the editor: it has SCADA bindings, a CIM identity, or bindings already exported. Isolated tells that its unit
// reaches no busbar in a diagram that has busbars: most likely a wire not joined to it.
type Row struct {
	Element   int    `json:"element"`
	Function  int    `json:"function"`
	OldName   string `json:"oldName,omitempty"`
	Name      string `json:"name"`
	Bound     bool   `json:"bound,omitempty"`
	MRID      bool   `json:"mrid,omitempty"`
	Exported  bool   `json:"exported,omitempty"`
	Duplicate bool   `json:"duplicate,omitempty"`
	Isolated  bool   `json:"isolated,omitempty"`
}

// LevelMember is an element in no unit (no letter code, or no unit found) and the voltage level Function it lies in.
type LevelMember struct {
	Element  int `json:"element"`
	Function int `json:"function"`
}

// Plan is Designate's result: the whole new function list (functions it creates have negative ids, for the caller
// to replace with its own), a row per element whose name or function changes, the elements in scope no unit
// could be found for, and the elements in scope in no unit whose voltage level has a function.
type Plan struct {
	Functions []model.Function `json:"functions"`
	Rows      []Row            `json:"rows"`
	Unplaced  []int            `json:"unplaced"`
	Levels    []LevelMember    `json:"levels"`
}

var designationRe = regexp.MustCompile(`^(?:=[A-Z]+[0-9]+)+-([A-Z]+)([0-9]+)$`)

var sourceCIM = map[string]bool{"ExternalNetworkInjection": true, "EquivalentInjection": true, "EnergySource": true}

var measuringClasses = map[model.Class]bool{
	model.ClassVoltageTransformer: true, model.ClassSurgeArrester: true, model.ClassFuse: true,
	model.ClassGroundSwitch: true, model.ClassDisconnector: true, model.ClassSectionalizer: true,
	model.ClassLoadBreakSwitch: true, model.ClassKnifeSwitch: true, model.ClassKnifeSwitch3: true,
	model.ClassDisconnectorFuse: true, model.ClassBreaker: true, model.ClassPowerCircuitBreaker: true,
	model.ClassShortCircuiter: true, model.ClassShortCircuiterNoGround: true,
}

type level struct {
	comp    int
	voltage int
	kv      float64
	code    string
	at      model.Point
	network int
	fn      int
}

type unit struct {
	kind        model.FunctionKind
	guessed     bool
	comp        int
	at          model.Point
	members     []int
	transformer int
	hv          bool
	fn          int
	needed      bool
	isolated    bool
}

type designator struct {
	d      *model.Diagram
	opt    Options
	tol    float64
	g      *graph
	comps  unionFind
	levels map[int]*level
	units  []*unit
	unitOf map[int]*unit
	dist   map[int]int
	fs     []model.Function
	byID   map[int]int
	nextID int
	full   bool
}

// Designate proposes designations for d's equipment (see Options); d is not changed.
func Designate(d *model.Diagram, opt Options) Plan {
	x := &designator{d: d, opt: opt, tol: opt.Grid / 2, unitOf: map[int]*unit{}, dist: map[int]int{}, byID: map[int]int{}}
	if x.tol <= 0 {
		x.tol = 5
	}
	x.full = opt.Renumber && len(opt.Scope) == 0
	x.g = newGraph(d)
	x.comps = x.g.components()
	parts := buildParts(x.g, x.comps, x.tol)
	x.buildLevels()
	x.buildUnits(parts)
	x.fs = slices.Clone(d.Functions)
	for i, f := range x.fs {
		x.byID[f.ID] = i
	}
	return x.plan()
}

func (x *designator) code(e *model.Element) string {
	if len(e.Ports) == 0 {
		return ""
	}
	return x.opt.LetterCodes[e.Shape]
}

func (x *designator) compOf(e *model.Element) int {
	for _, p := range e.Ports {
		if !isTransformer(e.Class) {
			return x.comps.find(p.Node)
		}
	}
	return -1
}

func pointLess(a, b model.Point) bool { return a.Y < b.Y || a.Y == b.Y && a.X < b.X }

// buildLevels makes a voltage level of each component holding equipment, voltage by majority.
func (x *designator) buildLevels() {
	x.levels = map[int]*level{}
	votes := map[int]map[int]int{}
	vote := func(comp, voltage int) {
		if votes[comp] == nil {
			votes[comp] = map[int]int{}
		}
		if voltage != 0 {
			votes[comp][voltage]++
		}
	}
	for i := range x.d.Elements {
		e := &x.d.Elements[i]
		comp := x.compOf(e)
		if comp < 0 {
			continue
		}
		vote(comp, e.Voltage)
		if l := x.levels[comp]; l == nil {
			x.levels[comp] = &level{comp: comp, at: position(e)}
		} else if p := position(e); pointLess(p, l.at) {
			l.at = p
		}
	}
	for _, c := range x.d.Connectors {
		if !isLine(c.Kind) {
			vote(x.comps.find(c.From), c.Voltage)
		}
	}
	names := map[int]string{}
	for _, vc := range x.d.VoltageClasses {
		names[vc.ID] = vc.Name
	}
	codes := slices.Clone(x.opt.VoltageLevels)
	slices.SortStableFunc(codes, func(a, b VoltageCode) int { return cmp.Compare(a.MaxKV, b.MaxKV) })
	for comp, l := range x.levels {
		best := 0
		for v, n := range votes[comp] {
			if n > votes[comp][best] || n == votes[comp][best] && (best == 0 || v < best) {
				best = v
			}
		}
		l.voltage, l.code = best, x.opt.FallbackVoltageCode
		if kv, ok := cim.ParseVoltage(names[best]); ok {
			l.kv = kv
			for _, c := range codes {
				if kv <= c.MaxKV {
					l.code = c.Code
					break
				}
			}
		}
	}
	networks := unionFind{}
	for _, e := range x.d.Elements {
		if isTransformer(e.Class) {
			for i := 1; i < len(e.Ports); i++ {
				networks.union(x.comps.find(e.Ports[0].Node), x.comps.find(e.Ports[i].Node))
			}
		}
	}
	for comp, l := range x.levels {
		l.network = networks.find(comp)
	}
}

func (x *designator) networkMaxKV(network int) float64 {
	m := 0.0
	for _, l := range x.levels {
		if l.network == network {
			m = math.Max(m, l.kv)
		}
	}
	return m
}

func (x *designator) classify(p *part) (model.FunctionKind, bool) {
	has := func(classes ...model.Class) bool {
		for _, id := range p.elements {
			if slices.Contains(classes, x.g.elements[id].Class) {
				return true
			}
		}
		return false
	}
	source := p.sourceLinks > 0 || has(model.ClassNetworkInfeed)
	for _, id := range p.elements {
		source = source || sourceCIM[x.g.elements[id].CIM]
	}
	sink := has(model.ClassLoad, model.ClassSynchronousMotor)
	ends := p.lines > 0 || source || sink
	measuring := has(model.ClassVoltageTransformer)
	for _, id := range p.elements {
		if e := x.g.elements[id]; x.code(e) != "" && !measuringClasses[e.Class] {
			measuring = false
		}
	}
	switch {
	case len(p.transformers) > 0:
		return model.FunctionTransformer, false
	case has(model.ClassGenerator, model.ClassSynchronousCompensator):
		return model.FunctionGenerator, false
	case len(p.busbars) >= 2 && !ends:
		return model.FunctionBusCoupler, false
	case measuring:
		return model.FunctionMeasuring, false
	case source:
		return model.FunctionIncomingFeeder, false
	case sink:
		return model.FunctionOutgoingFeeder, false
	case p.lines > 0:
		if l := x.levels[p.comp]; l != nil && l.kv > 0 && l.kv >= x.networkMaxKV(l.network) {
			return model.FunctionIncomingFeeder, true
		}
		return model.FunctionOutgoingFeeder, true
	}
	return model.FunctionOther, false
}

// busbarAxis returns whether busbar e runs horizontally, and its y (horizontal) or x.
func busbarAxis(e *model.Element) (bool, float64) {
	if len(e.Points) < 2 {
		return true, e.Y
	}
	a, z := e.Points[0], e.Points[len(e.Points)-1]
	if math.Abs(z.X-a.X) >= math.Abs(z.Y-a.Y) {
		return true, a.Y
	}
	return false, a.X
}

func (x *designator) buildUnits(parts []*part) {
	busbars := unionFind{}
	for _, p := range parts {
		kind, guessed := x.classify(p)
		if kind == model.FunctionBusCoupler || len(p.elements) == 0 && len(p.transformers) == 0 {
			for i := range p.busbars {
				for j := i + 1; j < len(p.busbars); j++ {
					a, b := x.g.elements[p.busbars[i]], x.g.elements[p.busbars[j]]
					ha, va := busbarAxis(a)
					hb, vb := busbarAxis(b)
					if ha == hb && math.Abs(va-vb) <= x.tol {
						busbars.union(a.ID, b.ID)
					}
				}
			}
		}
		u := &unit{kind: kind, guessed: guessed, comp: p.comp, at: p.at, isolated: len(p.busbars) == 0 && len(x.g.busbarAt) > 0}
		for _, id := range p.elements {
			x.dist[id] = p.dist[id]
			if x.code(x.g.elements[id]) != "" {
				u.members = append(u.members, id)
				x.unitOf[id] = u
			}
		}
		if len(p.transformers) > 0 {
			u.transformer = p.transformers[0]
		}
		x.units = append(x.units, u)
	}
	x.placeTransformers(parts)
	systems := map[int]*unit{}
	var sections []*model.Element
	for i := range x.d.Elements {
		if e := &x.d.Elements[i]; e.Class == model.ClassBusBarSection && x.code(e) != "" {
			sections = append(sections, e)
		}
	}
	sortByReading(sections, x.tol)
	for _, e := range sections {
		root := busbars.find(e.ID)
		u := systems[root]
		if u == nil {
			u = &unit{kind: model.FunctionBusbar, comp: x.compOf(e), at: position(e)}
			systems[root] = u
			x.units = append(x.units, u)
		}
		u.members = append(u.members, e.ID)
		x.unitOf[e.ID] = u
		x.dist[e.ID] = 0
	}
	x.units = slices.DeleteFunc(x.units, func(u *unit) bool { return len(u.members) == 0 && u.transformer == 0 })
}

// placeTransformers puts each transformer in the unit of its higher-voltage side.
func (x *designator) placeTransformers(parts []*part) {
	for i, e := range x.d.Elements {
		if !isTransformer(e.Class) || x.code(&x.d.Elements[i]) == "" {
			continue
		}
		var best *part
		var bestUnit *unit
		for j, p := range parts {
			if !slices.Contains(p.transformers, e.ID) {
				continue
			}
			if best == nil || x.kvOf(p.comp) > x.kvOf(best.comp) {
				best, bestUnit = p, x.units[j]
			}
		}
		if best == nil {
			continue
		}
		bestUnit.members = append(bestUnit.members, e.ID)
		bestUnit.hv = true
		bestUnit.transformer = e.ID
		x.unitOf[e.ID] = bestUnit
		x.dist[e.ID] = best.dist[e.ID]
	}
	for j, p := range parts {
		if u := x.units[j]; !u.hv && len(p.transformers) > 0 {
			u.transformer = p.transformers[0]
		}
	}
}

func (x *designator) kvOf(comp int) float64 {
	if l := x.levels[comp]; l != nil {
		return l.kv
	}
	return 0
}

func (x *designator) inScope() func(id int) bool {
	if len(x.opt.Scope) == 0 {
		return func(int) bool { return true }
	}
	set := map[int]bool{}
	for _, id := range x.opt.Scope {
		set[id] = true
	}
	return func(id int) bool { return set[id] }
}

func (x *designator) fn(id int) *model.Function {
	if i, ok := x.byID[id]; ok && id != 0 {
		return &x.fs[i]
	}
	return nil
}

func (x *designator) rootLevel(id int) int {
	seen := map[int]bool{}
	for f := x.fn(id); f != nil && !seen[f.ID]; f = x.fn(f.Parent) {
		seen[f.ID] = true
		if f.Kind == model.FunctionVoltageLevel {
			return f.ID
		}
	}
	return 0
}

// pick returns the function most of ids vote for that isn't taken, the lowest id on a tie; 0 for none.
func pick(votes map[int]int, taken map[int]bool) int {
	best := 0
	for id, n := range votes {
		if taken[id] {
			continue
		}
		if best == 0 || n > votes[best] || n == votes[best] && id < best {
			best = id
		}
	}
	return best
}

func (x *designator) add(f model.Function) int {
	x.nextID--
	f.ID = x.nextID
	x.byID[f.ID] = len(x.fs)
	x.fs = append(x.fs, f)
	return f.ID
}

func (x *designator) unitCode(kind model.FunctionKind) UnitCode {
	c := x.opt.Units[kind]
	if c.Code == "" {
		c.Code = "X"
	}
	return c
}

func (x *designator) plan() Plan {
	in := x.inScope()
	x.matchLevels()
	x.matchUnits()

	target := map[int]int{}
	var unplaced []int
	for i := range x.d.Elements {
		e := &x.d.Elements[i]
		if x.code(e) == "" || !in(e.ID) {
			continue
		}
		if x.fn(e.Function) != nil {
			target[e.ID] = e.Function
			continue
		}
		u := x.unitOf[e.ID]
		if u == nil || x.levels[u.comp] == nil {
			unplaced = append(unplaced, e.ID)
			continue
		}
		u.needed = true
	}
	if x.full {
		for _, u := range x.units {
			u.needed = u.needed || u.fn != 0
		}
	}
	x.numberLevels()
	x.numberUnits()
	for i := range x.d.Elements {
		e := &x.d.Elements[i]
		if _, ok := target[e.ID]; ok || x.code(e) == "" || !in(e.ID) {
			continue
		}
		if u := x.unitOf[e.ID]; u != nil && u.fn != 0 {
			target[e.ID] = u.fn
		}
	}
	return Plan{Functions: x.fs, Rows: x.number(target, in), Unplaced: unplaced, Levels: x.levelMembers(target, in)}
}

// levelMembers returns the elements in scope (in) with no target unit whose component's voltage level has a function.
func (x *designator) levelMembers(target map[int]int, in func(int) bool) []LevelMember {
	var out []LevelMember
	for i := range x.d.Elements {
		e := &x.d.Elements[i]
		if _, ok := target[e.ID]; ok || !in(e.ID) {
			continue
		}
		if l := x.levels[x.compOf(e)]; l != nil && l.fn != 0 {
			out = append(out, LevelMember{Element: e.ID, Function: l.fn})
		}
	}
	return out
}

func (x *designator) matchLevels() {
	taken := map[int]bool{}
	for _, l := range x.sortedLevels() {
		votes := map[int]int{}
		for i := range x.d.Elements {
			e := &x.d.Elements[i]
			if x.compOf(e) == l.comp {
				if r := x.rootLevel(e.Function); r != 0 {
					votes[r]++
				}
			}
		}
		if l.fn = pick(votes, taken); l.fn != 0 {
			taken[l.fn] = true
		}
	}
}

func (x *designator) sortedLevels() []*level {
	ls := make([]*level, 0, len(x.levels))
	ps := []model.Point{}
	for _, l := range x.levels {
		ls = append(ls, l)
	}
	slices.SortFunc(ls, func(a, b *level) int { return cmp.Compare(a.comp, b.comp) })
	for _, l := range ls {
		ps = append(ps, l.at)
	}
	rank := rowRanks(ps, x.tol)
	order := map[*level]int{}
	for i, l := range ls {
		order[l] = rank[i]
	}
	slices.SortStableFunc(ls, func(a, b *level) int { return cmp.Or(cmp.Compare(b.kv, a.kv), cmp.Compare(order[a], order[b])) })
	return ls
}

func (x *designator) sortedUnits() []*unit {
	ps := make([]model.Point, len(x.units))
	for i, u := range x.units {
		ps[i] = u.at
	}
	rank := rowRanks(ps, x.tol)
	us := make([]*unit, len(x.units))
	for i, u := range x.units {
		us[rank[i]] = u
	}
	return us
}

func (x *designator) matchUnits() {
	taken := map[int]bool{}
	for _, u := range x.sortedUnits() {
		votes := map[int]int{}
		for _, id := range u.members {
			if f := x.fn(x.g.elements[id].Function); f != nil && f.Kind != model.FunctionVoltageLevel {
				votes[f.ID]++
			}
		}
		if u.fn = pick(votes, taken); u.fn != 0 {
			taken[u.fn] = true
		}
	}
}

// numbers hands out numbers per (parent, code): in order from 1 when renumbering, else after the highest in use.
type numbers struct {
	used map[string]map[int]bool
	full bool
}

func key(parent int, code string) string { return strconv.Itoa(parent) + "\x00" + code }

func (n *numbers) reserve(parent int, code string, num int) {
	k := key(parent, code)
	if n.used[k] == nil {
		n.used[k] = map[int]bool{}
	}
	n.used[k][num] = true
}

func (n *numbers) next(parent int, code string) int {
	used := n.used[key(parent, code)]
	num := 1
	if !n.full {
		for u := range used {
			num = max(num, u+1)
		}
	}
	for used[num] {
		num++
	}
	n.reserve(parent, code, num)
	return num
}

// reserveExisting reserves the numbers of the functions keep says are kept.
func (x *designator) reserveExisting(n *numbers, keep func(f *model.Function) bool) {
	for i := range x.fs {
		if f := &x.fs[i]; keep(f) {
			n.reserve(f.Parent, f.Code, f.Number)
		}
	}
}

func (x *designator) numberLevels() {
	n := &numbers{used: map[string]map[int]bool{}, full: x.full}
	matched := map[int]bool{}
	for _, l := range x.levels {
		matched[l.fn] = true
	}
	x.reserveExisting(n, func(f *model.Function) bool {
		return f.Kind == model.FunctionVoltageLevel && (!x.full || !matched[f.ID])
	})
	needed := map[int]bool{}
	for _, u := range x.units {
		if u.needed && (u.fn == 0 || x.full) {
			needed[u.comp] = true
		}
	}
	for _, l := range x.sortedLevels() {
		switch {
		case l.fn != 0 && x.full:
			f := x.fn(l.fn)
			f.Code, f.Voltage, f.Number = l.code, l.voltage, n.next(0, l.code)
		case l.fn == 0 && needed[l.comp]:
			l.fn = x.add(model.Function{Kind: model.FunctionVoltageLevel, Code: l.code, Number: n.next(0, l.code), Voltage: l.voltage})
		}
	}
}

func (x *designator) numberUnits() {
	n := &numbers{used: map[string]map[int]bool{}, full: x.full}
	matched := map[int]bool{}
	for _, u := range x.units {
		matched[u.fn] = true
	}
	x.reserveExisting(n, func(f *model.Function) bool {
		return f.Kind != model.FunctionVoltageLevel && (!x.full || !matched[f.ID])
	})
	tnum := x.transformerNumbers()
	hvOf := map[int]*unit{}
	for _, u := range x.units {
		if u.hv {
			hvOf[u.transformer] = u
		}
	}
	for _, u := range x.sortedUnits() {
		l := x.levels[u.comp]
		if !u.needed || l == nil || u.fn != 0 && !x.full {
			continue
		}
		parent := l.fn
		kind, guessed := u.kind, u.guessed
		if u.fn != 0 {
			if f := x.fn(u.fn); !f.Guessed {
				kind, guessed = f.Kind, false
			}
		}
		c := x.unitCode(kind)
		num := 0
		if kind == model.FunctionTransformer && u.transformer != 0 {
			num = tnum[u.transformer]
			n.reserve(parent, c.Code, num)
		} else {
			num = n.next(parent, c.Code)
		}
		f := model.Function{Parent: parent, Kind: kind, Code: c.Code, Number: num, Digits: c.Digits, Guessed: guessed}
		if u.fn != 0 {
			f.ID, f.Pair = u.fn, x.fn(u.fn).Pair
			*x.fn(u.fn) = f
		} else {
			u.fn = x.add(f)
		}
	}
	for _, u := range x.units {
		hv := hvOf[u.transformer]
		if u.fn == 0 || u.hv || hv == nil || hv.fn == 0 || u.kind != model.FunctionTransformer {
			continue
		}
		if f := x.fn(u.fn); f.Pair == 0 {
			f.Pair = hv.fn
		}
		if f := x.fn(hv.fn); f.Pair == 0 {
			f.Pair = u.fn
		}
	}
}

// transformerNumbers numbers the transformers diagram-wide: a kept unit's number, else after the highest in use
// (from 1 in reading order when renumbering).
func (x *designator) transformerNumbers() map[int]int {
	tnum := map[int]int{}
	highest := 0
	if !x.full {
		for _, f := range x.fs {
			if f.Kind == model.FunctionTransformer {
				highest = max(highest, f.Number)
			}
		}
		for _, u := range x.units {
			if f := x.fn(u.fn); f != nil && u.transformer != 0 && f.Kind == model.FunctionTransformer &&
				(u.hv || tnum[u.transformer] == 0) {
				tnum[u.transformer] = f.Number
			}
		}
	}
	var ts []*model.Element
	for i := range x.d.Elements {
		if e := &x.d.Elements[i]; isTransformer(e.Class) {
			ts = append(ts, e)
		}
	}
	sortByReading(ts, x.tol)
	for _, e := range ts {
		if tnum[e.ID] == 0 {
			highest++
			tnum[e.ID] = highest
		}
	}
	return tnum
}

// number gives each element in scope a number per letter code within its target function (with UniqueNumbers, within
// the diagram), in unit order, then along the energy flow, then in reading order.
func (x *designator) number(target map[int]int, in func(int) bool) []Row {
	ps := make([]model.Point, len(x.d.Elements))
	for i := range x.d.Elements {
		ps[i] = position(&x.d.Elements[i])
	}
	rank := rowRanks(ps, x.tol)
	path := map[int]string{}
	designation := func(fn int) string {
		if p, ok := path[fn]; ok {
			return p
		}
		path[fn] = model.FunctionDesignation(x.fs, fn)
		return path[fn]
	}
	n := &numbers{used: map[string]map[int]bool{}, full: x.full}
	within := func(fn int) int {
		if x.opt.UniqueNumbers {
			return 0
		}
		return fn
	}
	parse := func(e *model.Element, fn int) (int, bool) {
		rest, ok := strings.CutPrefix(e.Name, designation(fn)+"-"+x.code(e))
		if !ok || rest == "" || strings.TrimLeft(rest, "0123456789") != "" {
			return 0, false
		}
		num, err := strconv.Atoi(rest)
		return num, err == nil && num > 0
	}
	var todo []int
	for _, scoped := range []bool{false, true} {
		for i := range x.d.Elements {
			e := &x.d.Elements[i]
			code := x.code(e)
			if code == "" || in(e.ID) != scoped {
				continue
			}
			fn, ok := target[e.ID]
			if !scoped {
				fn, ok = e.Function, x.fn(e.Function) != nil
			}
			if !ok {
				continue
			}
			if num, ok := parse(e, fn); ok && (!scoped || !x.opt.Renumber && e.Function == fn && !n.used[key(within(fn), code)][num]) {
				n.reserve(within(fn), code, num)
				continue
			}
			if scoped {
				todo = append(todo, i)
			}
		}
	}
	order := map[int]int{}
	for i, u := range x.sortedUnits() {
		if _, ok := order[u.fn]; u.fn != 0 && !ok {
			order[u.fn] = i
		}
	}
	unitOrder := func(fn int) int {
		if i, ok := order[fn]; ok {
			return i
		}
		return len(order)
	}
	slices.SortStableFunc(todo, func(a, b int) int {
		fa, fb := target[x.d.Elements[a].ID], target[x.d.Elements[b].ID]
		return cmp.Or(cmp.Compare(unitOrder(fa), unitOrder(fb)), cmp.Compare(fa, fb),
			cmp.Compare(x.distOf(x.d.Elements[a].ID), x.distOf(x.d.Elements[b].ID)), cmp.Compare(rank[a], rank[b]))
	})
	names := map[string]int{}
	for _, e := range x.d.Elements {
		names[e.Name]++
	}
	var rows []Row
	for _, i := range todo {
		e := &x.d.Elements[i]
		fn, code := target[e.ID], x.code(e)
		num := 0
		if m := designationRe.FindStringSubmatch(e.Name); m != nil && !x.opt.Renumber && m[1] == code {
			if old, _ := strconv.Atoi(m[2]); old > 0 && !n.used[key(within(fn), code)][old] {
				num = old
				n.reserve(within(fn), code, num)
			}
		}
		if num == 0 {
			num = n.next(within(fn), code)
		}
		name := fmt.Sprintf("%s-%s%d", designation(fn), code, num)
		if name == e.Name && fn == e.Function {
			continue
		}
		names[e.Name]--
		names[name]++
		exported := false
		for _, b := range e.Bindings {
			exported = exported || b.Status == ""
		}
		rows = append(rows, Row{
			Element: e.ID, Function: fn, OldName: e.Name, Name: name,
			Bound: len(e.Bindings) > 0, MRID: e.MRID != "", Exported: exported,
			Isolated: x.unitOf[e.ID] != nil && x.unitOf[e.ID].isolated,
		})
	}
	for i := range rows {
		rows[i].Duplicate = names[rows[i].Name] > 1
	}
	return rows
}

func (x *designator) distOf(id int) int {
	if d, ok := x.dist[id]; ok {
		return d
	}
	return math.MaxInt32
}

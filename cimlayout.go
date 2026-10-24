package slddoc

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
)

// Auto-layout for a CIM model without a Diagram Layout: one page per Substation, plus one for equipment outside
// any substation, on the model of PowSyBl's single-line layout. Each voltage level is a band, highest voltage at the
// top. Its busbars sharing a bay are parallel and drawn as full-width rows one under another; busbars sharing none sit
// side by side. Each bay (a cell: the devices between the busbars and its feeders) takes columns of its own and is
// drawn as a tree rooted at its node farthest from the busbars: every device that reaches a busbar has a column and
// drops straight onto its row (crossing the rows before it, where the import puts wire jumps), each other device
// stands over the devices below it, a level per device, and the feeders (lines leaving the page, transformers, end
// devices) lie beyond the root. A bay reaching a transformer of a higher voltage, on the top band one reaching a line,
// and a coupler (no feeder) go above the busbars; the rest below. Transformers sit on a row between the band of their
// highest winding and the next, linked through corridors. The layout is written as ordinary DiagramObjects of a
// generated Diagram, so page builds it like any other layout, wiring included. A voltage level without a busbar
// hangs its bays off its busiest node instead. Each band is labelled with its voltage level's name.

const (
	layoutColumns = 6 // grid steps between bay columns
	layoutLevel   = 8 // grid steps between devices down a bay
)

// layLink is a connection the layout draws itself through a corridor: from at (a bay end) to a transformer winding
// on node cn, or, closing a ring, to node cn where it was first drawn.
type layLink struct {
	cn       string
	at       Point
	trans    *cimObject
	loop     bool
	corridor float64
	band     *layBand // whose corridor it takes
}

type cimLayout struct {
	im       *cimImporter
	grid     float64
	page     map[string]bool         // equipment ids on this page
	cnPage   map[string]bool         // ConnectivityNode ids on this page
	atCN     map[string][]*cimObject // ConnectivityNode → equipment with a terminal on it
	visited  map[string]bool
	busCN    map[string]bool
	vlKV     map[string]float64 // voltage level key → kV
	diagram  *cimObject
	transTop map[string]string // transformer id → voltage level key of its highest winding
	links    []layLink
	busRow   map[string]int           // busbar node → its row in its group
	cur      *layRegion               // the region being drawn, which collects what is drawn
	transX   map[string]float64       // transformer id → its centre's x as arranged (none: the links' mean)
	cnAt     map[string]Point         // node → the point it was first drawn at
	cnOut    map[string]float64       // node → the side (+1 below, -1 above) its own wires leave that point
	sideUsed map[float64][][2]float64 // x → the y spans of the vertical runs links take there
	margin   float64
}

// autoLayout returns a generated Diagram per page.
func (im *cimImporter) autoLayout() []*cimObject {
	atCN := map[string][]*cimObject{}
	var equipment []*cimObject
	for _, list := range im.byClass {
		for _, o := range list {
			if _, isEq := im.terminals[o.id]; !isEq {
				continue
			}
			if !im.drawable(o.class) && o.class != "ACLineSegment" {
				continue
			}
			equipment = append(equipment, o)
			for _, t := range im.terminals[o.id] {
				if cn := t.get("ConnectivityNode"); cn != "" {
					atCN[cn] = append(atCN[cn], o)
				}
			}
		}
	}
	for _, o := range im.auxiliary() {
		equipment = append(equipment, o)
		if t := im.objs[o.get("Terminal")]; t != nil && t.get("ConnectivityNode") != "" {
			cn := t.get("ConnectivityNode")
			atCN[cn] = append(atCN[cn], o)
		}
	}
	byName := func(a, b *cimObject) int {
		return cmp.Or(cmp.Compare(a.get("name"), b.get("name")), cmp.Compare(a.id, b.id))
	}
	slices.SortFunc(equipment, byName)
	for _, list := range atCN {
		slices.SortFunc(list, byName)
	}

	// Pages: a substation's equipment; a line belongs to the substations at its ends, drawn there as a stub.
	pages := map[string]map[string]bool{}
	var order []string
	add := func(page, eq string) {
		if pages[page] == nil {
			pages[page] = map[string]bool{}
			order = append(order, page)
		}
		pages[page][eq] = true
	}
	for _, o := range equipment {
		subs := map[string]bool{}
		for _, cn := range im.equipmentCNs(o) {
			subs[im.substationOfCN(cn)] = true
		}
		if s := im.substationOf(o); s != "" && o.class != "ACLineSegment" {
			subs = map[string]bool{s: true}
		}
		if len(subs) == 0 {
			subs[""] = true
		}
		for s := range subs {
			add(s, o.id)
		}
	}
	slices.SortStableFunc(order, func(a, b string) int {
		return cmp.Compare(im.objs[a].get("name"), im.objs[b].get("name"))
	})
	var diagrams []*cimObject
	for _, sub := range order {
		title := "Network"
		if o := im.objs[sub]; o != nil && o.get("name") != "" {
			title = o.get("name")
		}
		l := &cimLayout{im: im, grid: im.opt.Grid, page: pages[sub], cnPage: map[string]bool{}, atCN: atCN,
			visited: map[string]bool{}, busCN: map[string]bool{}, vlKV: map[string]float64{},
			transTop: map[string]string{}, busRow: map[string]int{}, cnAt: map[string]Point{}, cnOut: map[string]float64{},
			sideUsed: map[float64][][2]float64{}}
		for cn := range atCN {
			if im.substationOfCN(cn) == sub {
				l.cnPage[cn] = true
			}
		}
		l.diagram = im.synthetic("Diagram", map[string]string{"name": title, "synthetic": "true"})
		l.layout()
		diagrams = append(diagrams, l.diagram)
	}
	return diagrams
}

// auxiliary is every auxiliary device (attached to a terminal rather than having its own).
func (im *cimImporter) auxiliary() []*cimObject {
	var out []*cimObject
	for class := range cimAuxiliaryClasses {
		out = append(out, im.byClass[class]...)
	}
	return out
}

func (im *cimImporter) equipmentCNs(o *cimObject) []string {
	var out []string
	if t := im.objs[o.get("Terminal")]; t != nil && cimAuxiliaryClasses[o.class] {
		return []string{t.get("ConnectivityNode")}
	}
	for _, t := range im.terminals[o.id] {
		if cn := t.get("ConnectivityNode"); cn != "" {
			out = append(out, cn)
		}
	}
	return out
}

// containerSubstation follows a container (Bay → VoltageLevel → Substation) to its substation id.
func (im *cimImporter) containerSubstation(id string) string {
	for range 4 {
		o := im.objs[id]
		if o == nil {
			return ""
		}
		switch o.class {
		case "Substation":
			return o.id
		case "VoltageLevel":
			id = o.get("Substation")
		case "Bay":
			id = o.get("VoltageLevel")
		default:
			return ""
		}
	}
	return ""
}

func (im *cimImporter) substationOf(o *cimObject) string {
	return im.containerSubstation(o.get("EquipmentContainer"))
}

func (im *cimImporter) substationOfCN(cn string) string {
	if o := im.objs[cn]; o != nil {
		return im.containerSubstation(o.get("ConnectivityNodeContainer"))
	}
	return ""
}

// synthetic adds a generated object of class with literal properties.
func (im *cimImporter) synthetic(class string, props map[string]string) *cimObject {
	o := &cimObject{id: fmt.Sprintf("auto-%d", len(im.objs)), class: class, props: map[string][]string{}}
	for k, v := range props {
		o.props[k] = []string{v}
	}
	im.objs[o.id] = o
	im.byClass[class] = append(im.byClass[class], o)
	return o
}

// --- voltage levels

// vlOf is the key of o's voltage level: its container's (or its bay's), else its base voltage's, else that of its
// first node.
func (l *cimLayout) vlOf(o *cimObject) string {
	im := l.im
	if c := im.objs[o.get("EquipmentContainer")]; c != nil {
		if c.class == "Bay" {
			c = im.objs[c.get("VoltageLevel")]
		}
		if c != nil && c.class == "VoltageLevel" {
			return l.vlKey(c.id, c.get("BaseVoltage"))
		}
	}
	if bv := o.get("BaseVoltage"); bv != "" {
		return l.vlKey("bv:"+bv, bv)
	}
	for _, cn := range im.equipmentCNs(o) {
		if k := l.cnVL(cn); k != "" {
			return k
		}
	}
	return ""
}

func (l *cimLayout) cnVL(cn string) string {
	o := l.im.objs[cn]
	if o == nil {
		return ""
	}
	c := l.im.objs[o.get("ConnectivityNodeContainer")]
	if c != nil && c.class == "Bay" {
		c = l.im.objs[c.get("VoltageLevel")]
	}
	if c == nil || c.class != "VoltageLevel" {
		return ""
	}
	return l.vlKey(c.id, c.get("BaseVoltage"))
}

func (l *cimLayout) vlKey(key, bv string) string {
	if _, ok := l.vlKV[key]; !ok {
		kv, _ := l.im.objs[bv].num("nominalVoltage")
		l.vlKV[key] = kv
	}
	return key
}

// --- cells

// layTree is a device of a cell hanging from node up, its parent's, towards the busbars: its other terminal continues
// the cell at node down, lands on busbar node bus or closes a ring at loop (none of them for a single-terminal one).
// A transformer or a line leaving the page met inside a cell is a leaf too.
type layTree struct {
	eq       *cimObject
	up       string
	down     string
	bus      string
	loop     string
	children []*layTree
	height   int
	width    int
	x        float64
	far      Point // its terminal on up, as placed
}

// layCell is a bay: the devices between the busbars and its feeders (lines leaving the page, transformers and end
// devices), laid out as a tree rooted at its node farthest from the busbars, the feeders beyond that node.
type layCell struct {
	root    string // the far node ("" for a device straight between two busbars)
	devices []*layTree
	feeders []*cimObject
	buses   []string   // the busbar nodes it reaches, in row order
	direct  *cimObject // a device whose terminals are all on busbars
	onBus   string     // the busbar node its feeders sit on when it has no devices
	up      bool
	width   int
	height  int // levels below the root, plus one for the feeders
	col     int
}

// layGroup is a set of parallel busbars (sharing a bay), drawn as full-width rows one under another.
type layGroup struct {
	buses []*cimObject // nil entries stand for a busbar-less hub node
	cns   []string     // each row's node
	cells []*layCell
	col   int
	cols  int
}

// isFeeder reports whether eq ends a bay at node cn: a transformer, a line leaving the page, or a device with no
// other terminal.
func (l *cimLayout) isFeeder(eq *cimObject) bool {
	if eq.class == "PowerTransformer" {
		return true
	}
	cns := l.im.equipmentCNs(eq)
	if eq.class == "ACLineSegment" {
		return len(cns) < 2 || !l.cnPage[cns[0]] || !l.cnPage[cns[1]]
	}
	return len(cns) < 2 || cimAuxiliaryClasses[eq.class]
}

// cell collects the bay starting at device first on hub node hub: every device reached through nodes other than
// busbars', its feeders at the node farthest from the busbars, and the tree of its devices from there.
func (l *cimLayout) cell(first *cimObject, hub string) *layCell {
	c := &layCell{}
	cns := map[string]bool{}
	var order []string
	buses := map[string]bool{}
	feedersAt := map[string][]*cimObject{}
	devices := map[string]bool{}
	queue := []*cimObject{first}
	if l.isFeeder(first) {
		l.visited[first.id] = first.class != "PowerTransformer"
		c.feeders, c.onBus, c.buses = []*cimObject{first}, hub, []string{hub}
		return c
	}
	l.visited[first.id] = true
	devices[first.id] = true
	for len(queue) > 0 {
		eq := queue[0]
		queue = queue[1:]
		for _, cn := range l.im.equipmentCNs(eq) {
			if l.busCN[cn] {
				buses[cn] = true
				continue
			}
			if cns[cn] {
				continue
			}
			cns[cn] = true
			order = append(order, cn)
			for _, o := range l.atCN[cn] {
				if !l.page[o.id] || o.class == "BusbarSection" || devices[o.id] || l.visited[o.id] && o.class != "PowerTransformer" {
					continue
				}
				if l.isFeeder(o) {
					if o.class != "PowerTransformer" {
						l.visited[o.id] = true
					}
					feedersAt[cn] = append(feedersAt[cn], o)
					continue
				}
				l.visited[o.id] = true
				devices[o.id] = true
				queue = append(queue, o)
			}
		}
	}
	for cn := range buses {
		c.buses = append(c.buses, cn)
	}
	slices.SortFunc(c.buses, func(a, b string) int { return cmp.Compare(l.busRow[a], l.busRow[b]) })
	if len(order) == 0 {
		c.direct = first
		return c
	}

	// The root: the node farthest from the busbars, one with feeders first.
	dist := map[string]int{}
	var frontier []string
	for _, cn := range order {
		for _, eq := range l.atCN[cn] {
			if devices[eq.id] && slices.ContainsFunc(l.im.equipmentCNs(eq), func(x string) bool { return l.busCN[x] }) {
				if _, ok := dist[cn]; !ok {
					dist[cn] = 1
					frontier = append(frontier, cn)
				}
			}
		}
	}
	for len(frontier) > 0 {
		cn := frontier[0]
		frontier = frontier[1:]
		for _, eq := range l.atCN[cn] {
			if !devices[eq.id] {
				continue
			}
			for _, next := range l.im.equipmentCNs(eq) {
				if _, ok := dist[next]; !ok && cns[next] {
					dist[next] = dist[cn] + 1
					frontier = append(frontier, next)
				}
			}
		}
	}
	c.root = order[0]
	score := func(cn string) [2]int {
		f := 0
		if len(feedersAt[cn]) > 0 {
			f = 1
		}
		return [2]int{f, dist[cn]}
	}
	for _, cn := range order {
		if s, b := score(cn), score(c.root); s[0] > b[0] || s[0] == b[0] && s[1] > b[1] {
			c.root = cn
		}
	}
	c.feeders = feedersAt[c.root]
	inTree := map[string]bool{c.root: true}
	placed := map[string]bool{}
	c.devices = l.tree(c.root, nil, devices, feedersAt, inTree, placed)
	return c
}

// tree returns the devices hanging from node cn towards the busbars (parent excluded), each with its own subtree.
func (l *cimLayout) tree(cn string, parent *cimObject, devices map[string]bool, feedersAt map[string][]*cimObject,
	inTree, placed map[string]bool) []*layTree {
	var out []*layTree
	for _, eq := range l.atCN[cn] {
		if !devices[eq.id] || eq == parent || placed[eq.id] {
			continue
		}
		placed[eq.id] = true
		t := &layTree{eq: eq, up: cn}
		for _, other := range l.im.equipmentCNs(eq) {
			switch {
			case other == cn:
			case l.busCN[other]:
				t.bus = other
			case inTree[other]:
				t.loop = other
			default:
				t.down = other
				inTree[other] = true
				t.children = l.tree(other, eq, devices, feedersAt, inTree, placed)
				for _, f := range feedersAt[other] {
					t.children = append(t.children, &layTree{eq: f, up: other})
				}
			}
			if t.down != "" || t.bus != "" || t.loop != "" {
				break
			}
		}
		out = append(out, t)
	}
	// Legs to the upper busbar rows first, so they cross fewer of the others.
	slices.SortStableFunc(out, func(a, b *layTree) int { return cmp.Compare(l.firstRow(a), l.firstRow(b)) })
	return out
}

// firstRow is the topmost busbar row t's subtree lands on (a large number for none).
func (l *cimLayout) firstRow(t *layTree) int {
	r := math.MaxInt
	if t.bus != "" {
		r = l.busRow[t.bus]
	}
	for _, c := range t.children {
		r = min(r, l.firstRow(c))
	}
	return r
}

func measureTree(t *layTree) {
	t.width, t.height = 0, 0
	for _, c := range t.children {
		measureTree(c)
		t.width += c.width
		t.height = max(t.height, c.height)
	}
	t.width = max(t.width, 1)
	t.height++
}

func (l *cimLayout) measureCell(c *layCell) {
	c.width, c.height = 0, 0
	for _, t := range c.devices {
		measureTree(t)
		c.width += t.width
		c.height = max(c.height, t.height)
	}
	if c.direct != nil {
		c.width = 1
	}
	if len(c.feeders) > 0 {
		c.height++
	}
	c.width = max(c.width, len(c.feeders), 1)
}

// goesUp reports whether cell c of band b is drawn above its busbars: one reaching a transformer of a higher voltage,
// on the top band one reaching a line, and one with no feeder (a coupler).
func (l *cimLayout) goesUp(c *layCell, b *layBand, top bool) bool {
	if c.direct != nil {
		return true
	}
	feeders := slices.Clone(c.feeders)
	var walk func(ts []*layTree)
	walk = func(ts []*layTree) {
		for _, t := range ts {
			if l.isFeeder(t.eq) {
				feeders = append(feeders, t.eq)
			}
			walk(t.children)
		}
	}
	walk(c.devices)
	if len(feeders) == 0 {
		return true
	}
	for _, f := range feeders {
		if f.class == "PowerTransformer" {
			top := l.transTop[f.id]
			if top != b.vl && l.vlKV[top] > l.vlKV[b.vl] {
				return true
			}
		}
		if f.class == "ACLineSegment" && top {
			return true
		}
	}
	return false
}

// --- the page

type layBand struct {
	vl                 string
	groups             []*layGroup
	upDepth, downDepth int
	upLinks, downLinks int
	top, busY, bottom  float64
	rows               int
	main               int     // the group the voltage level's name stands beside
	nameW              float64 // room for that name
	nextUp, nextDown   int     // corridors taken
}

const layoutBusGap = 4 // grid steps between parallel busbars

// mainGroup is the index of the group with the most busbars, then the most cells.
func mainGroup(groups []*layGroup) int {
	busbars := func(gr *layGroup) int {
		n := 0
		for _, bus := range gr.buses {
			if bus != nil {
				n++
			}
		}
		return n
	}
	main := 0
	for i, gr := range groups {
		if m := groups[main]; busbars(gr) > busbars(m) || busbars(gr) == busbars(m) && len(gr.cells) > len(m.cells) {
			main = i
		}
	}
	return main
}

func (l *cimLayout) layout() {
	im := l.im
	vls := map[string]*layBand{}
	band := func(vl string) *layBand {
		if vls[vl] == nil {
			vls[vl] = &layBand{vl: vl}
		}
		return vls[vl]
	}
	var buses []*cimObject
	for _, eq := range im.byClass["BusbarSection"] {
		if l.page[eq.id] {
			buses = append(buses, eq)
			for _, cn := range im.equipmentCNs(eq) {
				l.busCN[cn] = true
			}
		}
	}
	slices.SortFunc(buses, func(a, b *cimObject) int { return cmp.Compare(a.get("name"), b.get("name")) })
	var trs []*cimObject
	for _, tr := range im.byClass["PowerTransformer"] {
		if !l.page[tr.id] {
			continue
		}
		trs = append(trs, tr)
		top, kv := "", math.Inf(-1)
		for _, cn := range im.equipmentCNs(tr) {
			if vl := l.cnVL(cn); vl != "" && l.vlKV[vl] > kv {
				top, kv = vl, l.vlKV[vl]
			}
		}
		if top == "" {
			top = l.vlOf(tr)
		}
		l.transTop[tr.id] = top
		band(top)
	}

	// Busbars and their bays; then whatever no busbar reaches hangs off its busiest node, repeatedly.
	type hub struct {
		bus *cimObject
		cn  string
		vl  string
	}
	var hubs []hub
	for i, bus := range buses {
		l.visited[bus.id] = true
		for _, cn := range im.equipmentCNs(bus) {
			l.busRow[cn] = i
			hubs = append(hubs, hub{bus: bus, cn: cn, vl: l.vlOf(bus)})
			break
		}
	}
	cellsOf := map[string][]*layCell{}
	grow := func(h hub) {
		for _, eq := range l.atCN[h.cn] {
			if !l.page[eq.id] || eq.class == "BusbarSection" || l.visited[eq.id] && eq.class != "PowerTransformer" {
				continue
			}
			if eq.class == "PowerTransformer" && slices.ContainsFunc(cellsOf[h.vl], func(c *layCell) bool { return slices.Contains(c.feeders, eq) }) {
				continue
			}
			cellsOf[h.vl] = append(cellsOf[h.vl], l.cell(eq, h.cn))
		}
	}
	for _, h := range hubs {
		grow(h)
	}
	for {
		best, bestN := "", 0
		for cn, list := range l.atCN {
			if l.busCN[cn] || !l.cnPage[cn] && !l.anyOnPage(list) {
				continue
			}
			n := 0
			for _, eq := range list {
				if !l.visited[eq.id] && l.page[eq.id] && eq.class != "PowerTransformer" && eq.class != "BusbarSection" {
					n++
				}
			}
			if n > bestN || n == bestN && n > 0 && cn < best {
				best, bestN = cn, n
			}
		}
		if bestN == 0 {
			break
		}
		vl := l.cnVL(best)
		if vl == "" {
			for _, eq := range l.atCN[best] {
				if l.page[eq.id] {
					vl = l.vlOf(eq)
					break
				}
			}
		}
		l.busCN[best] = true
		l.busRow[best] = len(buses) + len(hubs)
		h := hub{cn: best, vl: vl}
		hubs = append(hubs, h)
		grow(h)
	}

	// Groups: the busbars sharing a bay are parallel, drawn one under another.
	for _, h := range hubs {
		band(h.vl)
	}
	groupOf := map[string]*layGroup{}
	for _, h := range hubs {
		b := band(h.vl)
		var g *layGroup
		for _, c := range cellsOf[h.vl] {
			if slices.Contains(c.buses, h.cn) {
				for _, cn := range c.buses {
					if groupOf[cn] != nil {
						g = groupOf[cn]
					}
				}
			}
		}
		if g == nil {
			g = &layGroup{}
			b.groups = append(b.groups, g)
		}
		g.buses = append(g.buses, h.bus)
		g.cns = append(g.cns, h.cn)
		groupOf[h.cn] = g
	}
	for vl, cells := range cellsOf {
		b := band(vl)
		for _, c := range cells {
			g := groupOf[c.onBus]
			if len(c.buses) > 0 {
				g = groupOf[c.buses[0]]
			}
			if g == nil && len(b.groups) > 0 {
				g = b.groups[0]
			}
			if g == nil {
				g = &layGroup{}
				b.groups = append(b.groups, g)
			}
			g.cells = append(g.cells, c)
		}
	}
	for _, b := range vls {
		for _, g := range b.groups {
			for i, cn := range g.cns {
				l.busRow[cn] = i
			}
		}
	}

	bands := make([]*layBand, 0, len(vls))
	for _, b := range vls {
		bands = append(bands, b)
	}
	slices.SortFunc(bands, func(a, b *layBand) int {
		return cmp.Or(cmp.Compare(l.vlKV[b.vl], l.vlKV[a.vl]), cmp.Compare(a.vl, b.vl))
	})

	g := l.grid
	colW, levelH, busGap := layoutColumns*g, layoutLevel*g, layoutBusGap*g
	margin := 6 * g
	l.margin = margin
	colX := func(col int) float64 { return margin + float64(col)*colW + colW/2 }

	// Columns, directions, depths and corridors: couplers first, then the bays, each in columns of its own; before
	// the band's main busbar group, room for the voltage level's name.
	for bi, b := range bands {
		col := 0
		b.main = mainGroup(b.groups)
		for gi, gr := range b.groups {
			if _, name := l.vlName(b.vl); gi == b.main && name != "" {
				b.nameW = 6*float64(len([]rune(name))) + 2*g
				col += int(math.Ceil(b.nameW / (layoutColumns * g)))
			}
			b.rows = max(b.rows, len(gr.cns))
			slices.SortStableFunc(gr.cells, func(x, y *layCell) int {
				return cmp.Compare(min(len(x.feeders), 1), min(len(y.feeders), 1))
			})
			gr.col = col
			for _, c := range gr.cells {
				l.measureCell(c)
				c.up = l.goesUp(c, b, bi == 0)
				c.col = col
				col += c.width
				links := l.cellLinks(c)
				if c.up {
					b.upDepth, b.upLinks = max(b.upDepth, c.height), b.upLinks+links
				} else {
					b.downDepth, b.downLinks = max(b.downDepth, c.height), b.downLinks+links
				}
			}
			gr.cols = max(col-gr.col, 1)
			col = gr.col + gr.cols + 1
		}
	}

	// Bands top to bottom, each with a corridor row per link above and below it, and a transformer row after it.
	y := margin
	maxX := margin
	rows := map[string]float64{}
	var regions []*layRegion
	for _, b := range bands {
		bandStart := y
		var bandRegions []*layRegion
		for _, gr := range b.groups {
			bandRegions = append(bandRegions, &layRegion{group: gr, band: b})
		}
		b.top = y + float64(b.upLinks+1)*g
		b.busY = b.top + float64(b.upDepth)*levelH + 2*g
		last := b.busY + float64(max(b.rows-1, 0))*busGap
		b.bottom = last + float64(b.downDepth)*levelH + 2*g
		rowY := func(cn string) float64 { return b.busY + float64(l.busRow[cn])*busGap }
		for gi, gr := range b.groups {
			r := bandRegions[gi]
			l.cur = r
			firstLink := len(l.links)
			seen := make(map[string]bool, len(l.cnAt))
			for cn := range l.cnAt {
				seen[cn] = true
			}
			x1, x2 := colX(gr.col)-colW/2+g, colX(gr.col+gr.cols-1)+colW/2-g
			// The voltage level's name, as PowSyBl writes it, left of its main busbars, half way down their rows.
			if o, name := l.vlName(b.vl); o != nil && gi == b.main {
				l.text(o, name, Point{X: x1 - b.nameW, Y: b.busY + float64(len(gr.cns)-1)*busGap/2 + 3})
				r.name, r.nameDX = r.dos[len(r.dos)-1], x2+g-(x1-b.nameW)
			}
			for i, bus := range gr.buses {
				yy := b.busY + float64(i)*busGap
				switch {
				case bus != nil:
					l.object(bus, []Point{{X: x1, Y: yy}, {X: x2, Y: yy}}, 0)
					if name := bus.get("name"); name != "" {
						l.text(bus, name, Point{X: x1, Y: yy - g})
					}
				case im.objs[gr.cns[i]] != nil:
					// A drawn wire stands in for the busbar, so the bays drop straight onto it.
					l.object(im.objs[gr.cns[i]], []Point{{X: x1 + g, Y: yy}, {X: x2 - g, Y: yy}}, 0)
				}
			}
			for _, c := range gr.cells {
				base, out := b.busY, -1.0
				if !c.up {
					base, out = last, 1
				}
				l.placeCell(c, b, base, out, rowY, colX)
			}
			maxX = math.Max(maxX, colX(gr.col+gr.cols))
			for i := firstLink; i < len(l.links); i++ {
				l.links[i].band = b
				r.links = append(r.links, i)
			}
			for cn := range l.cnAt {
				if !seen[cn] {
					r.cns = append(r.cns, cn)
				}
			}
			// Its box: its columns, room for the names right of the last, and the band with its corridors.
			r.box = [4]float64{x1 - g, bandStart, x2 + 13*g, b.bottom + float64(b.downLinks+1)*g}
		}
		l.cur = nil
		regions = append(regions, bandRegions...)
		// Room for a transformer's leads and the wires coming in to them, clear of the corridors.
		rows[b.vl] = b.bottom + float64(b.downLinks+1)*g + 12*g
		y = rows[b.vl] + 13*g
	}
	l.arrange(regions, trs, rows)
	l.transformers(trs, rows, y, maxX, colW)
}

// vlName is the object and name to label voltage level key vl with: the VoltageLevel's name, else its base voltage
// ("110 kV"); nil for none.
func (l *cimLayout) vlName(vl string) (*cimObject, string) {
	o := l.im.objs[vl]
	if o == nil {
		o = l.im.objs[strings.TrimPrefix(vl, "bv:")]
	}
	if o == nil {
		return nil, ""
	}
	if name := o.get("name"); name != "" && o.class == "VoltageLevel" {
		return o, name
	}
	if kv := l.vlKV[vl]; kv > 0 {
		return o, strconv.FormatFloat(kv, 'f', -1, 64) + " kV"
	}
	return nil, ""
}

// cellLinks counts the links cell c takes through a corridor: transformers and rings.
func (l *cimLayout) cellLinks(c *layCell) int {
	n := 0
	for _, f := range c.feeders {
		if f.class == "PowerTransformer" {
			n++
		}
	}
	var walk func(ts []*layTree)
	walk = func(ts []*layTree) {
		for _, t := range ts {
			if t.loop != "" || t.eq.class == "PowerTransformer" {
				n++
			}
			walk(t.children)
		}
	}
	walk(c.devices)
	return n
}

// placeCell draws cell c from base (its busbar row nearest it) outwards (out -1 up, +1 down): its legs in columns
// of their own from c.col, each device over the legs below it, a level per device, the feeders beyond the root.
func (l *cimLayout) placeCell(c *layCell, b *layBand, base, out float64, rowY func(string) float64, colX func(int) float64) {
	g := l.grid
	levelH := layoutLevel * g
	corridor := func() float64 {
		if out > 0 {
			b.nextDown++
			return b.bottom + float64(b.nextDown)*g
		}
		b.nextUp++
		return b.top - float64(b.nextUp)*g
	}
	if c.direct != nil {
		l.placeDirect(c, colX(c.col), rowY)
		return
	}
	col := c.col
	var assign func(t *layTree)
	assign = func(t *layTree) {
		if len(t.children) == 0 {
			t.x = colX(col)
			col++
			return
		}
		for _, ch := range t.children {
			assign(ch)
		}
		t.x = math.Round((t.children[0].x+t.children[len(t.children)-1].x)/2/g) * g
	}
	for _, t := range c.devices {
		assign(t)
	}

	rootX := colX(c.col)
	if n := len(c.devices); n > 0 {
		rootX = math.Round((c.devices[0].x+c.devices[n-1].x)/2/g) * g
	}
	rootH := c.height
	if len(c.feeders) > 0 {
		rootH--
	}
	rootY := base + out*(float64(rootH)*levelH+g)
	if len(c.devices) == 0 {
		rootY = rowY(c.onBus)
	}
	if c.root != "" {
		if _, seen := l.cnAt[c.root]; !seen {
			l.cnAt[c.root], l.cnOut[c.root] = Point{X: rootX, Y: rootY}, out
		}
	}
	for _, t := range c.devices {
		l.placeTree(t, b, base, out, rowY, corridor)
		l.wire(t.up, []Point{{X: rootX, Y: rootY}, {X: t.far.X, Y: rootY}, t.far})
	}

	// The feeders beyond the root, side by side.
	hubCN := c.root
	if hubCN == "" {
		hubCN = c.onBus
	}
	for i, f := range c.feeders {
		x := rootX
		if len(c.feeders) > 1 {
			x = colX(c.col + i)
		}
		at := Point{X: x, Y: rootY}
		if x != rootX {
			l.wire(hubCN, []Point{{X: rootX, Y: rootY}, at})
		}
		l.feeder(f, hubCN, at, out, corridor)
	}
}

// feeder draws feeder f on node cn, leaving at outwards: a transformer through a corridor, a line as a stub, an end
// device facing at.
func (l *cimLayout) feeder(f *cimObject, cn string, at Point, out float64, corridor func() float64) {
	g := l.grid
	switch {
	case f.class == "PowerTransformer":
		l.links = append(l.links, layLink{cn: cn, at: at, trans: f, corridor: corridor()})
	case f.class == "ACLineSegment":
		end := Point{X: at.X, Y: at.Y + out*4*g}
		pts := []Point{at, end}
		if cns := l.im.equipmentCNs(f); len(cns) > 1 && cns[1] == cn {
			pts[0], pts[1] = pts[1], pts[0] // the points run from terminal 1 to terminal 2
		}
		l.object(f, pts, 0)
		if name := f.get("name"); name != "" {
			l.text(f, name, Point{X: end.X + g, Y: end.Y + out*g})
		}
	default:
		near := at.Y + out*g
		ts, k := l.device(f, cn, at.X, near, at.Y+out*3*g, out)
		if k >= 0 {
			l.wire(cn, []Point{at, ts[k]})
		}
		if name := f.get("name"); name != "" {
			l.text(f, name, Point{X: at.X + 2*g, Y: at.Y + out*3*g})
		}
	}
}

// placeTree draws device t at the level of its height, its terminal towards the busbars down, and its subtree below.
func (l *cimLayout) placeTree(t *layTree, b *layBand, base, out float64, rowY func(string) float64, corridor func() float64) {
	g := l.grid
	levelH := layoutLevel * g
	level := float64(t.height)
	if l.isFeeder(t.eq) {
		// A feeder met inside the bay: from its node towards the busbars.
		at := Point{X: t.x, Y: base + out*(levelH-g)}
		t.far = at
		l.feeder(t.eq, t.up, at, -out, corridor)
		return
	}
	near := base + out*((level-1)*levelH+2*g)
	centre := base + out*((level-0.5)*levelH)
	// Names of devices side by side would run into each other: each column's at a height of its own.
	col := int(math.Round((t.x - l.margin) / (layoutColumns * g)))
	label := centre + float64(col%3-1)*1.5*g
	facingCN := cmp.Or(t.down, t.bus, t.loop)
	if facingCN == "" {
		// One terminal only: it faces the node it hangs from.
		ts, k := l.device(t.eq, t.up, t.x, base+out*levelH, centre, -out)
		if k >= 0 {
			t.far = ts[k]
		}
		l.name(t.eq, t.x, label)
		return
	}
	ts, k := l.device(t.eq, facingCN, t.x, near, centre, out)
	l.name(t.eq, t.x, label)
	if k < 0 {
		t.far = Point{X: t.x, Y: centre}
		return
	}
	facing := ts[k]
	t.far = facing
	for i, cn := range l.im.equipmentCNs(t.eq) {
		if cn == t.up && i < len(ts) {
			t.far = ts[i]
		}
	}
	switch {
	case t.bus != "":
		l.wire(t.bus, []Point{facing, {X: facing.X, Y: rowY(t.bus)}})
	case t.loop != "":
		start := Point{X: facing.X, Y: facing.Y - out*g}
		l.wire(t.loop, []Point{facing, start})
		l.links = append(l.links, layLink{cn: t.loop, at: start, loop: true, corridor: corridor()})
	default:
		jy := facing.Y - out*g
		if _, seen := l.cnAt[t.down]; !seen {
			l.cnAt[t.down], l.cnOut[t.down] = Point{X: facing.X, Y: jy}, -out
		}
		for _, ch := range t.children {
			l.placeTree(ch, b, base, out, rowY, corridor)
			l.wire(t.down, []Point{facing, {X: facing.X, Y: jy}, {X: ch.far.X, Y: jy}, ch.far})
		}
	}
}

// placeDirect draws a device whose terminals are all on busbars between their rows, in its own column at x.
func (l *cimLayout) placeDirect(c *layCell, x float64, rowY func(string) float64) {
	eq := c.direct
	cns := l.im.equipmentCNs(eq)
	if len(cns) < 2 {
		return
	}
	upper, lower := cns[0], cns[1]
	if rowY(upper) > rowY(lower) {
		upper, lower = lower, upper
	}
	y1, y2 := rowY(upper), rowY(lower)
	if y1 == y2 {
		y1 -= 4 * l.grid
	}
	ts, k := l.device(eq, upper, x, y1+l.grid, (y1+y2)/2, 1)
	l.name(eq, x, (y1+y2)/2)
	if k < 0 {
		return
	}
	l.wire(upper, []Point{ts[k], {X: x, Y: rowY(upper)}})
	for i, cn := range cns {
		if cn == lower && i < len(ts) {
			l.wire(lower, []Point{ts[i], {X: x, Y: rowY(lower)}})
		}
	}
}

func (l *cimLayout) name(eq *cimObject, x, y float64) {
	if name := eq.get("name"); name != "" {
		l.text(eq, name, Point{X: x + 2*l.grid, Y: y})
	}
}

// transformers places each transformer on the row under the band of its highest winding (rows; y below the last
// band), above the bays that reach it, turned so winding 1 is on top, and draws the links to it and the rings.
func (l *cimLayout) transformers(trs []*cimObject, rows map[string]float64, y, maxX, colW float64) {
	im := l.im
	g := l.grid
	xs := map[string][]float64{}
	for _, k := range l.links {
		if k.trans != nil {
			xs[k.trans.id] = append(xs[k.trans.id], k.at.X)
		}
	}
	slices.SortFunc(trs, func(a, b *cimObject) int {
		return cmp.Or(cmp.Compare(meanOr(xs[a.id], math.Inf(1)), meanOr(xs[b.id], math.Inf(1))), cmp.Compare(a.get("name"), b.get("name")))
	})
	last := map[float64]float64{}
	terms := map[string][]Point{}  // transformer id → its winding terminals as placed
	centres := map[string]Point{}  // transformer id → its centre
	approaches := map[string]int{} // transformer id → links already brought in
	for _, tr := range trs {
		rowY, ok := rows[l.transTop[tr.id]]
		if !ok {
			rowY = y
		}
		probe := l.transformerProbe(tr, 0, rowY)
		best := probe.Orient
		// Its terminals' reach left and right of its centre, plus room for the wires coming in beside them.
		left, right := 0.0, 0.0
		for _, t := range symbolTerminals(&probe, l.im.opt.Terminals) {
			left, right = math.Min(left, t.X), math.Max(right, t.X)
		}
		room := 6 * g
		x := math.Round(meanOr(xs[tr.id], maxX+colW)/g) * g
		if ax, arranged := l.transX[tr.id]; arranged {
			x = ax
		} else if prev, ok := last[rowY]; ok && x+left-room < prev {
			x = prev - left + room
		}
		last[rowY] = x + right + room
		maxX = math.Max(maxX, x+right+colW)
		probe.X = x
		terms[tr.id] = symbolTerminals(&probe, l.im.opt.Terminals)
		centres[tr.id] = Point{X: x, Y: rowY}
		l.object(tr, []Point{{X: x, Y: rowY}}, float64(best))
		if name := tr.get("name"); name != "" {
			l.text(tr, name, Point{X: x + 5*g, Y: rowY})
		}
	}

	var leads []corrLead
	for i, k := range l.links {
		if k.trans == nil {
			continue
		}
		if w := l.winding(k.trans, k.cn); w >= 0 && w < len(terms[k.trans.id]) {
			leads = append(leads, corrLead{i, k.band, k.at, terms[k.trans.id][w], k.corridor})
		}
	}
	for i, y := range corridors(leads) {
		l.links[i].corridor = y
	}

	// The links: down (or up) the bay's column into its corridor, along it, and into the winding or the ring's node.
	for _, k := range l.links {
		switch {
		case k.loop:
			to, ok := l.cnAt[k.cn]
			if !ok {
				continue
			}
			// In from the side the node's own wires leave, where only they lie.
			ay := to.Y + l.cnOut[k.cn]*g
			side := l.side(to.X, k.corridor, ay, colW)
			l.wire(k.cn, []Point{k.at, {X: k.at.X, Y: k.corridor}, {X: side, Y: k.corridor}, {X: side, Y: ay}, {X: to.X, Y: ay}, to})
		case k.trans != nil:
			ts := terms[k.trans.id]
			w := -1
			for i, end := range im.transformerEnds(k.trans) {
				if t := im.objs[end.get("Terminal")]; t != nil && t.get("ConnectivityNode") == k.cn && i < len(ts) {
					w = i
				}
			}
			if w < 0 {
				continue
			}
			end := ts[w]
			c := centres[k.trans.id]
			if math.Abs(end.Y-c.Y) >= math.Abs(end.X-c.X) && math.Abs(k.at.X-end.X) < 1e-6 {
				// Lined up under (or over) its bay: one straight lead.
				l.wire(k.cn, []Point{k.at, end})
				continue
			}
			if math.Abs(end.Y-c.Y) >= math.Abs(end.X-c.X) {
				// A lead up or down: come in vertically, from outside, at a height of its own.
				out := math.Copysign(1, end.Y-c.Y)
				approaches[k.trans.id]++
				ay := end.Y + out*float64(1+approaches[k.trans.id])*g
				side := l.side(end.X, k.corridor, ay, colW)
				l.wire(k.cn, []Point{k.at, {X: k.at.X, Y: k.corridor}, {X: side, Y: k.corridor}, {X: side, Y: ay}, {X: end.X, Y: ay}, end})
				continue
			}
			// A lead to the side: come down beyond it, at a distance of its own, and in horizontally.
			out := math.Copysign(1, end.X-c.X)
			ax := end.X
			for {
				approaches[k.trans.id]++
				ax = end.X + out*float64(1+approaches[k.trans.id])*g
				if !l.columnX(ax) && l.free(ax, k.corridor, end.Y) {
					break
				}
			}
			l.wire(k.cn, []Point{k.at, {X: k.at.X, Y: k.corridor}, {X: ax, Y: k.corridor}, {X: ax, Y: end.Y}, end})
		}
	}
}

// columnX reports whether x is a bay column's centre, where devices' terminals lie.
func (l *cimLayout) columnX(x float64) bool {
	colW := layoutColumns * l.grid
	return math.Mod(math.Abs(x-l.margin-colW/2), colW) == 0
}

// free reports whether a vertical run at x from lo to hi overlaps no other run there, and if so takes it.
func (l *cimLayout) free(x, y1, y2 float64) bool {
	lo, hi := math.Min(y1, y2), math.Max(y1, y2)
	for _, span := range l.sideUsed[x] {
		if span[0] <= hi && lo <= span[1] {
			return false
		}
	}
	l.sideUsed[x] = append(l.sideUsed[x], [2]float64{lo, hi})
	return true
}

// side picks an x within half a column of x, where no terminal lies, for a link's vertical run from y1 to y2 that
// no other run overlaps; the run then touches nothing on its way, nor does its short last leg to x.
func (l *cimLayout) side(x, y1, y2, colW float64) float64 {
	g := l.grid
	lo, hi := math.Min(y1, y2), math.Max(y1, y2)
	for _, off := range []float64{-3, 3, -2, 2, -4, 4, -1, 1, -5, 5} {
		cand := x + off*g
		if math.Abs(off*g) >= colW/2+g || l.columnX(cand) {
			continue
		}
		if l.free(cand, lo, hi) {
			return cand
		}
	}
	return x - colW/2
}

func (l *cimLayout) anyOnPage(list []*cimObject) bool {
	for _, eq := range list {
		if l.page[eq.id] {
			return true
		}
	}
	return false
}

func meanOr(xs []float64, or float64) float64 {
	if len(xs) == 0 {
		return or
	}
	s := 0.0
	for _, x := range xs {
		s += x
	}
	return s / float64(len(xs))
}

// wire adds a drawn wire of node cn along pts.
func (l *cimLayout) wire(cn string, pts []Point) {

	pts = simplifyPath(pts)
	if o := l.im.objs[cn]; o != nil && len(pts) >= 2 {
		l.object(o, pts, 0)
	}
}

// device places eq: turned (0° or 180°) so its terminal on node facing is on the busbar side for dir (+1: the
// busbars are above), and moved so that terminal sits at near; one without symbol terminals is centred at centre.
// Returns its terminals as placed and the facing one's index (-1 for none).
func (l *cimLayout) device(eq *cimObject, facingCN string, x, near, centre, dir float64) ([]Point, int) {
	defClass, defShape, _ := l.im.shapeFor(eq.class)
	ts := l.im.opt.Terminals[defShape]
	facing := 0
	if !cimAuxiliaryClasses[eq.class] {
		facing = max(slices.Index(l.im.equipmentCNs(eq), facingCN), 0)
	}
	if facing >= len(ts) {
		l.object(eq, []Point{{X: x, Y: centre}}, 0)
		return nil, -1
	}
	ty := ts[facing].Y
	if ty == 0 && len(ts) == 1 {
		ty = -1 // a terminal at the anchor: the body is below it
	}
	rot := 0
	if dir > 0 && ty > 0 || dir < 0 && ty < 0 {
		rot = 180
	}
	probe := Element{Class: defClass, Shape: defShape, Orient: rot}
	placed := symbolTerminals(&probe, l.im.opt.Terminals)
	anchor := Point{X: x, Y: near - placed[facing].Y}
	l.object(eq, []Point{anchor}, float64(rot))
	for i := range placed {
		placed[i] = roundPt(Point{X: placed[i].X + anchor.X, Y: placed[i].Y + anchor.Y})
	}
	return placed, facing
}

// object adds a DiagramObject for eq with its points.
func (l *cimLayout) object(eq *cimObject, pts []Point, rotation float64) {
	do := l.im.synthetic("DiagramObject", map[string]string{
		"Diagram": l.diagram.id, "IdentifiedObject": eq.id, "rotation": strconv.FormatFloat(rotation, 'f', -1, 64)})
	l.im.dos[l.diagram.id] = append(l.im.dos[l.diagram.id], do)
	if l.cur != nil {
		l.cur.dos = append(l.cur.dos, do)
	}
	for i, p := range pts {
		pt := l.im.synthetic("DiagramObjectPoint", map[string]string{"DiagramObject": do.id, "sequenceNumber": strconv.Itoa(i + 1),
			"xPosition": strconv.FormatFloat(p.X, 'f', -1, 64), "yPosition": strconv.FormatFloat(p.Y, 'f', -1, 64)})
		l.im.points[do.id] = append(l.im.points[do.id], pt)
	}
}

func (l *cimLayout) text(eq *cimObject, text string, at Point) {
	do := l.im.synthetic("TextDiagramObject", map[string]string{"Diagram": l.diagram.id, "IdentifiedObject": eq.id, "text": text})
	l.im.dos[l.diagram.id] = append(l.im.dos[l.diagram.id], do)
	if l.cur != nil {
		l.cur.dos = append(l.cur.dos, do)
	}
	pt := l.im.synthetic("DiagramObjectPoint", map[string]string{"DiagramObject": do.id,
		"xPosition": strconv.FormatFloat(at.X, 'f', -1, 64), "yPosition": strconv.FormatFloat(at.Y, 'f', -1, 64)})
	l.im.points[do.id] = append(l.im.points[do.id], pt)
}

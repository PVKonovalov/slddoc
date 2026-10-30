package cim

import (
	"cmp"
	"maps"
	"math"
	"slices"
	"strconv"

	"github.com/PVKonovalov/slddoc/internal/model"
	"github.com/PVKonovalov/slddoc/internal/repair"
)

// Arranging a generated layout: after the bands are drawn, each busbar group with its bays (a region) may move as a
// whole, its inner layout untouched, to straighten the links to transformers, and the regions of a voltage class
// move together as a block on one row, never split by another class. The moves are kept only when they lower the
// layout's error: crossings first, then the links' bends and length.

// layRegion is what one busbar group drew: its DiagramObjects, links and the nodes it first drew, and its box.
type layRegion struct {
	group *layGroup
	band  *layBand
	dos   []*cimObject
	links []int    // indexes into cimLayout.links
	cns   []string // nodes whose cnAt it set
	box   [4]float64
	parts [][4]float64 // a box per drawn object
	d     model.Point  // its move
	cols  []float64    // its bay columns' x, before its move

	name    *cimObject    // its band's name, left of its busbars
	nameDX  float64       // the name's move to the right of its busbars
	flipped bool          // whether box and parts are those with the name moved
	other   [2][4]float64 // box and the name's part of the other side
	namePt  int           // the name's index in parts
}

// flip puts the region's name, in its box and parts, right of its busbars or back left.
func (r *layRegion) flip(right bool) {
	if r.name == nil || r.flipped == right {
		return
	}
	r.box, r.other[0] = r.other[0], r.box
	r.parts[r.namePt], r.other[1] = r.other[1], r.parts[r.namePt]
	r.flipped = right
}

// transformerProbe is transformer tr centred at (x, y), turned so winding 1 is on top.
func (l *cimLayout) transformerProbe(tr *cimObject, x, y float64) model.Element {
	probe := model.Element{Class: model.ClassPowerTransformer, Shape: "47", X: x, Y: y,
		Windings: make([]model.TransformerWinding, min(max(len(l.im.transformerEnds(tr)), 2), 4))}
	best, bestScore := 0, math.Inf(-1)
	for _, o := range []int{0, 90, 180, -90} {
		probe.Orient = o
		ts := repair.SymbolTerminals(&probe, l.im.opt.Terminals)
		score := 0.0
		for _, t := range ts[1:] {
			score += t.Y - ts[0].Y
		}
		if score > bestScore+1e-9 {
			best, bestScore = o, score
		}
	}
	probe.Orient = best
	return probe
}

// winding is the index of transformer tr's winding on node cn (-1 for none).
func (l *cimLayout) winding(tr *cimObject, cn string) int {
	w := -1
	for i, end := range l.im.transformerEnds(tr) {
		if t := l.im.objs[end.get("Terminal")]; t != nil && t.get("ConnectivityNode") == cn {
			w = i
		}
	}
	return w
}

// layBlock is a voltage class's regions: they move together, side by side, so a voltage class is never split.
type layBlock struct {
	kv      float64
	regions []*layRegion
}

// arrange places the transformers (l.transX) and moves the regions where that lowers the layout's error, starting
// from the layout as drawn. The regions of a voltage class form a block, kept in one piece: each of its regions goes
// under the transformers it links to, in their order, the others close beside, and the block rises beside the one
// above where nothing is in the way, never above it. Each transformer may go under or over any bay linked to it, or
// beside another block; a block may also step clear of a transformer's leads. Moves are tried until none lowers the
// error; regions never overlap, nor do transformers on a row, and all stays right of the page margin. rows are the
// transformer rows by band.
func (l *cimLayout) arrange(regions []*layRegion, trs []*cimObject, rows map[string]float64) {
	g := l.grid
	if len(regions) < 2 || len(trs) == 0 {
		return
	}
	for _, r := range regions {
		l.measureRegion(r)
	}
	var blocks []*layBlock
	for _, r := range regions {
		kv := l.vlKV[r.band.vl]
		if n := len(blocks); n == 0 || blocks[n-1].kv != kv {
			blocks = append(blocks, &layBlock{kv: kv})
		}
		blocks[len(blocks)-1].regions = append(blocks[len(blocks)-1].regions, r)
	}
	if len(blocks) < 2 {
		return
	}
	region := map[int]*layRegion{}
	for _, r := range regions {
		for _, i := range r.links {
			region[i] = r
		}
	}
	row := func(tr *cimObject) (float64, bool) {
		y, ok := rows[l.transTop[tr.id]]
		return y, ok
	}
	// A transformer's x where winding w's terminal lies at x.
	centreFor := func(tr *cimObject, w int, x float64) (float64, bool) {
		y, ok := row(tr)
		ts := repair.SymbolTerminals(new(l.transformerProbe(tr, 0, y)), l.im.opt.Terminals)
		if !ok || w < 0 || w >= len(ts) {
			return 0, false
		}
		return math.Round((x-ts[w].X)/g) * g, true
	}

	// The start: the layout as drawn, transformers where the links' mean puts them.
	transX := map[string]float64{}
	xs := map[string][]float64{}
	for _, k := range l.links {
		if k.trans != nil {
			xs[k.trans.id] = append(xs[k.trans.id], k.at.X)
		}
	}
	for _, tr := range trs {
		if _, ok := row(tr); ok && len(xs[tr.id]) > 0 {
			transX[tr.id] = math.Round(meanOr(xs[tr.id], 0)/g) * g
		}
	}
	l.spaceRows(trs, transX, rows)
	still := map[*layRegion]model.Point{}
	start := l.layoutError(regions, still, transX, rows)
	best, bestT, bestS := start, transX, map[*layBlock]float64{}
	var bestD map[*layRegion]model.Point
	bestF := map[*layRegion]bool{}
	setFlips := func(f map[*layRegion]bool) {
		for _, r := range regions {
			r.flip(f[r])
		}
	}

	top := map[*layRegion]bool{}
	for _, r := range blocks[0].regions {
		top[r] = true
	}
	try1 := func(nt map[string]float64, ns map[*layBlock]float64) (map[*layRegion]model.Point, bool) {
		l.spaceRows(trs, nt, rows)
		d := l.placeBlocks(blocks, nt, ns, rows)
		if !l.fits(regions, d, nt, trs, rows) {
			return d, false
		}
		if e := l.layoutError(regions, d, nt, rows); e < best-1e-6 {
			best, bestT, bestS, bestD = e, nt, ns, d
			bestF = map[*layRegion]bool{}
			for _, r := range regions {
				bestF[r] = r.flipped
			}
			return d, true
		}
		return d, false
	}
	// try also has every transformer follow what hangs under it, as placed, so a voltage class kept together lines up
	// with its transformers.
	try := func(nt map[string]float64, ns map[*layBlock]float64) bool {
		d, ok := try1(nt, ns)
		follow := maps.Clone(nt)
		for i, k := range l.links {
			r := region[i]
			if k.trans == nil || r == nil || top[r] || l.transTop[k.trans.id] == r.band.vl {
				continue
			}
			if x, ok := centreFor(k.trans, l.winding(k.trans, k.cn), k.at.X+d[r].X); ok {
				follow[k.trans.id] = x
			}
		}
		if maps.Equal(follow, nt) {
			return ok
		}
		_, ok2 := try1(follow, maps.Clone(ns))
		return ok || ok2
	}
	// Starts: the transformers as drawn, and each lined up with its leads from the top block.
	up := maps.Clone(transX)
	for i, k := range l.links {
		if k.trans != nil && top[region[i]] {
			if x, ok := centreFor(k.trans, l.winding(k.trans, k.cn), k.at.X); ok {
				up[k.trans.id] = x
			}
		}
	}
	type state struct {
		e float64
		t map[string]float64
		s map[*layBlock]float64
		d map[*layRegion]model.Point
		f map[*layRegion]bool
	}
	won := state{e: start, t: transX}
	for _, st := range []map[string]float64{transX, up} {
		best, bestT, bestS, bestD, bestF = math.Inf(1), st, map[*layBlock]float64{}, nil, map[*layRegion]bool{}
		try(maps.Clone(st), map[*layBlock]float64{})
		for range 8 {
			improved := false
			setFlips(bestF)
			cur := bestD
			if cur == nil {
				cur = still
			}
			for _, tr := range trs {
				if _, ok := bestT[tr.id]; !ok {
					continue
				}
				var cands []float64
				for i, k := range l.links {
					if k.trans == tr {
						if x, ok := centreFor(tr, l.winding(tr, k.cn), k.at.X+cur[region[i]].X); ok {
							cands = append(cands, x)
						}
					}
				}
				for _, b := range blocks[1:] {
					u := union(b.regions, cur)
					cands = append(cands, math.Round((u[0]-4*layoutColumns*g)/g)*g, math.Round((u[2]+4*layoutColumns*g)/g)*g)
				}
				for _, x := range cands {
					if x == bestT[tr.id] {
						continue
					}
					nt := maps.Clone(bestT)
					nt[tr.id] = x
					improved = try(nt, maps.Clone(bestS)) || improved
				}
			}
			for _, b := range blocks[1:] {
				u := union(b.regions, cur)
				cands := []float64{0}
				for _, tr := range trs {
					tx, ok := bestT[tr.id]
					y, ok2 := row(tr)
					if !ok || !ok2 {
						continue
					}
					left, right := l.reach(tr, y)
					cands = append(cands, bestS[b]+tx+right-u[0], bestS[b]+tx+left-u[2])
				}
				for _, sh := range cands {
					sh = math.Round(sh/g) * g
					if sh == bestS[b] {
						continue
					}
					ns := maps.Clone(bestS)
					ns[b] = sh
					improved = try(maps.Clone(bestT), ns) || improved
				}
			}
			if !improved {
				break
			}
		}
		if bestD != nil && best < won.e-1e-6 {
			won = state{best, bestT, bestS, bestD, bestF}
		}
	}
	best, bestT, bestD, bestF = won.e, won.t, won.d, won.f
	if bestD == nil || best >= start-1e-6 {
		setFlips(nil)
		return
	}
	setFlips(bestF)
	for _, r := range regions {
		if r.flipped {
			for _, p := range l.im.points[r.name.id] {
				p.props["xPosition"] = []string{strconv.FormatFloat(pointOf(p).X+r.nameDX, 'f', -1, 64)}
			}
		}
		r.d = bestD[r]
		l.shiftRegion(r)
	}
	l.transX = bestT
}

// placeBlocks moves the blocks below the first, given the transformers at transX and each block's extra shift: its
// regions on one row, side by side in the order their leads up to transformers want them (the others with the
// block's median), each as close to that as being clear of the others and at most layoutBlockGap columns further
// allows, a band's name going right of its busbars when only it is in the way; then the block rises beside the one
// above as far as that block's top, what is drawn over it and the transformers allow.
func (l *cimLayout) placeBlocks(blocks []*layBlock, transX map[string]float64, shift map[*layBlock]float64, rows map[string]float64) map[*layRegion]model.Point {
	g := l.grid
	d := map[*layRegion]model.Point{}
	for _, b := range blocks {
		for _, r := range b.regions {
			r.flip(false)
		}
	}
	var done []*layRegion
	for _, r := range blocks[0].regions {
		d[r] = model.Point{}
		done = append(done, r)
	}
	for _, b := range blocks[1:] {
		// A voltage class's regions stay on the row of bands they were drawn on.
		rise := map[*layRegion]float64{}
		type item struct {
			r    *layRegion
			want float64
			has  bool
		}
		var items []item
		var all []float64
		for _, r := range b.regions {
			var dxs []float64
			for _, i := range r.links {
				k := l.links[i]
				if k.trans == nil || l.transTop[k.trans.id] == r.band.vl {
					continue
				}
				tx, ok := transX[k.trans.id]
				y, ok2 := rows[l.transTop[k.trans.id]]
				if !ok || !ok2 {
					continue
				}
				w := l.winding(k.trans, k.cn)
				ts := repair.SymbolTerminals(new(l.transformerProbe(k.trans, tx, y)), l.im.opt.Terminals)
				if w < 0 || w >= len(ts) || ts[w].Y <= y {
					continue
				}
				dxs = append(dxs, ts[w].X-k.at.X)
			}
			it := item{r: r}
			if len(dxs) > 0 {
				slices.Sort(dxs)
				it.want, it.has = r.box[0]+dxs[len(dxs)/2], true
				all = append(all, dxs[len(dxs)/2])
			}
			items = append(items, it)
		}
		mid := 0.0
		if len(all) > 0 {
			slices.Sort(all)
			mid = all[len(all)/2]
		}
		for i := range items {
			if !items[i].has {
				items[i].want = items[i].r.box[0] + mid
			}
		}
		// Moves on the grid, so a region lined up under its transformer stays exactly under it.
		slices.SortStableFunc(items, func(a, c item) int { return cmp.Compare(a.want, c.want) })
		most := layoutBlockGap * layoutColumns * g // boxes already hold a margin and the names
		lay := func(first float64) []float64 {
			moves := make([]float64, len(items))
			for i, it := range items {
				if i == 0 {
					moves[i] = math.Round(first/g) * g
					continue
				}
				least := func() float64 {
					lo := math.Inf(-1)
					for j := range i {
						lo = math.Max(lo, clearOf(it.r, rise[it.r], items[j].r, model.Point{X: moves[j], Y: rise[items[j].r]}))
					}
					return math.Ceil(lo/g) * g
				}
				want := math.Round((it.want-it.r.box[0])/g) * g
				lo := least()
				// The name goes right of the busbars when only it keeps the region from lining up.
				if it.has && want < lo && it.r.name != nil {
					it.r.flip(true)
					if l2 := least(); l2 < lo {
						lo = l2
					} else {
						it.r.flip(false)
					}
				}
				moves[i] = lo
				if it.has {
					moves[i] = math.Min(math.Max(want, lo), lo+most)
				}
			}
			return moves
		}
		first := 0.0
		if items[0].has {
			first = items[0].want - items[0].r.box[0]
		} else if len(all) > 0 {
			first = mid
		}
		for i, m := range lay(first) {
			d[items[i].r] = model.Point{X: m + shift[b], Y: rise[items[i].r]}
		}
		u := union(b.regions, d)
		if edge := l.margin / 2; u[0] < edge {
			for _, r := range b.regions {
				d[r] = model.Point{X: d[r].X + math.Ceil((edge-u[0])/g)*g, Y: d[r].Y}
			}
			u = union(b.regions, d)
		}
		// Clear of what is already placed on its row: further right, never up or down.
		for range 100 {
			push := 0.0
			for _, o := range done {
				for _, r := range b.regions {
					for _, p := range r.parts {
						p = shifted(p, d[r])
						for _, q := range o.parts {
							if q = shifted(q, d[o]); overlaps([4]float64{p[0] - g, p[1], p[2] + g, p[3]}, q) {
								push = math.Max(push, q[2]+2*g-p[0])
							}
						}
					}
				}
			}
			if push <= 0 {
				break
			}
			for _, r := range b.regions {
				d[r] = model.Point{X: d[r].X + math.Ceil(push/g)*g, Y: d[r].Y}
			}
		}
		done = append(done, b.regions...)
	}
	return d
}

// layoutBlockGap is how many columns may part two regions of a voltage class, to line them up under their
// transformers.
const layoutBlockGap = 2

// spaceRows pushes apart, in their order along each row, transformers at transX that would overlap.
func (l *cimLayout) spaceRows(trs []*cimObject, transX map[string]float64, rows map[string]float64) {
	var placed []*cimObject
	for _, tr := range trs {
		if _, ok := transX[tr.id]; ok {
			placed = append(placed, tr)
		}
	}
	slices.SortStableFunc(placed, func(a, b *cimObject) int { return cmp.Compare(transX[a.id], transX[b.id]) })
	last := map[float64]float64{}
	for _, tr := range placed {
		y, ok := rows[l.transTop[tr.id]]
		if !ok {
			continue
		}
		left, right := l.reach(tr, y)
		x := transX[tr.id]
		if prev, ok := last[y]; ok && x+left < prev {
			x = math.Ceil((prev-left)/l.grid) * l.grid
		}
		transX[tr.id] = x
		last[y] = x + right
	}
}

// union is the box around regions moved by d.
func union(regions []*layRegion, d map[*layRegion]model.Point) [4]float64 {
	u := [4]float64{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}
	for _, r := range regions {
		b := shifted(r.box, d[r])
		u = [4]float64{math.Min(u[0], b[0]), math.Min(u[1], b[1]), math.Max(u[2], b[2]), math.Max(u[3], b[3])}
	}
	return u
}

// measureRegion sets region r's box sideways from what it drew, names included (its height comes from its band).
func (l *cimLayout) measureRegion(r *layRegion) {
	x1, x2 := math.Inf(1), math.Inf(-1)
	for _, do := range r.dos {
		for _, p := range l.im.points[do.id] {
			x := pointOf(p).X
			x1, x2 = math.Min(x1, x), math.Max(x2, x)
			if do.class == "TextDiagramObject" {
				x2 = math.Max(x2, x+6*float64(len([]rune(do.get("text")))))
			}
		}
	}
	if x1 <= x2 {
		r.box[0], r.box[2] = x1-l.grid, x2+l.grid
	}
	r.parts = r.parts[:0]
	for _, do := range r.dos {
		b := [4]float64{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}
		for _, p := range l.im.points[do.id] {
			q := pointOf(p)
			b = [4]float64{math.Min(b[0], q.X), math.Min(b[1], q.Y), math.Max(b[2], q.X), math.Max(b[3], q.Y)}
		}
		if b[0] > b[2] {
			continue
		}
		if do.class == "TextDiagramObject" {
			b = [4]float64{b[0], b[1] - 10, b[2] + 6*float64(len([]rune(do.get("text")))), b[3] + 2}
		} else if len(l.im.points[do.id]) > 1 {
			b = [4]float64{b[0] - l.grid/2, b[1] - l.grid/2, b[2] + l.grid/2, b[3] + l.grid/2}
		} else {
			b = [4]float64{b[0] - 1.5*l.grid, b[1] - 1.5*l.grid, b[2] + 1.5*l.grid, b[3] + 1.5*l.grid}
		}
		if do == r.name {
			r.namePt = len(r.parts)
		}
		r.parts = append(r.parts, [4]float64{b[0] - l.grid/2, b[1], b[2] + l.grid/2, b[3]})
	}
	if r.name != nil {
		r.flipped = false
		np := shifted(r.parts[r.namePt], model.Point{X: r.nameDX})
		box := r.box
		box[0] = math.Inf(1)
		for i, p := range r.parts {
			if i != r.namePt {
				box[0] = math.Min(box[0], p[0])
			}
		}
		box[2] = math.Max(box[2], np[2]+l.grid)
		r.other = [2][4]float64{box, np}
	}
}

// clearOf is the least move of region a, raised by ay, that puts each of its parts right of every part of region b
// (moved by bd) beside it.
func clearOf(a *layRegion, ay float64, b *layRegion, bd model.Point) float64 {
	lo := math.Inf(-1)
	for _, p := range a.parts {
		for _, q := range b.parts {
			if spanOverlap(p[1]+ay, p[3]+ay, q[1]+bd.Y, q[3]+bd.Y) {
				lo = math.Max(lo, q[2]+bd.X-p[0])
			}
		}
	}
	return lo
}

// collide reports whether any part of region a, moved by da, overlaps one of region b, moved by db.
func collide(a, b *layRegion, da, db model.Point) bool {
	if !overlaps(shifted(a.box, da), shifted(b.box, db)) {
		return false
	}
	for _, p := range a.parts {
		for _, q := range b.parts {
			if overlaps(shifted(p, da), shifted(q, db)) {
				return true
			}
		}
	}
	return false
}

// fits reports whether everything stays right of the page's margin, and no two regions overlap, nor two transformers
// on a row, nor a transformer and a region.
func (l *cimLayout) fits(regions []*layRegion, d map[*layRegion]model.Point, transX map[string]float64, trs []*cimObject, rows map[string]float64) bool {
	edge := l.margin / 2
	for _, r := range regions {
		if r.box[0]+d[r].X < edge {
			return false
		}
	}
	for i, a := range regions {
		for _, b := range regions[i+1:] {
			if collide(a, b, d[a], d[b]) {
				return false
			}
		}
	}
	var boxes [][4]float64
	for _, tr := range trs {
		tx, ok := transX[tr.id]
		y, ok2 := rows[l.transTop[tr.id]]
		if !ok || !ok2 {
			continue
		}
		left, right := l.reach(tr, y)
		tb := [4]float64{tx + left, y - 6*l.grid, tx + right, y + 6*l.grid}
		if tb[0] < edge {
			return false
		}
		for _, o := range boxes {
			if overlaps(tb, o) {
				return false
			}
		}
		for _, r := range regions {
			if overlaps(tb, shifted(r.box, d[r])) {
				return false
			}
		}
		boxes = append(boxes, tb)
	}
	return true
}

// reach is how far transformer tr, its terminals and its name, reaches left and right of its centre on row y.
func (l *cimLayout) reach(tr *cimObject, y float64) (float64, float64) {
	left, right := 0.0, 0.0
	for _, t := range repair.SymbolTerminals(new(l.transformerProbe(tr, 0, y)), l.im.opt.Terminals) {
		left, right = math.Min(left, t.X), math.Max(right, t.X)
	}
	right = math.Max(right+3*l.grid, 5*l.grid+6*float64(len([]rune(tr.get("name")))))
	return left - 3*l.grid, right
}

// layoutError estimates the layout with regions moved by d and transformers at transX (nil: where the links' mean
// puts them): crossings of links with each other and with busbars and wires of another voltage class, and runs of a
// link along a busbar or wire (a million each), then each bend of a link (a thousand, a quarter more below its transformer), then the links' length.
func (l *cimLayout) layoutError(regions []*layRegion, d map[*layRegion]model.Point, transX map[string]float64, rows map[string]float64) float64 {
	type seg struct {
		a, z model.Point
		vl   string
	}
	var segs []seg
	regionOf := map[int]*layRegion{}
	for _, r := range regions {
		for _, i := range r.links {
			regionOf[i] = r
		}
		for _, do := range r.dos {
			o := l.im.objs[do.get("IdentifiedObject")]
			if o == nil || do.class != "DiagramObject" {
				continue
			}
			vl := ""
			switch {
			case isNode(o):
				vl = l.cnVL(o.id)
			case o.class == "BusbarSection":
				vl = l.vlOf(o)
			default:
				continue
			}
			pts := l.im.points[do.id]
			for k := 1; k < len(pts); k++ {
				segs = append(segs, seg{pointOf(pts[k-1]).Add(d[r]), pointOf(pts[k]).Add(d[r]), vl})
			}
		}
	}
	xs := map[string][]float64{}
	for i, k := range l.links {
		if k.trans != nil {
			xs[k.trans.id] = append(xs[k.trans.id], k.at.X+d[regionOf[i]].X)
		}
	}
	var leads []corrLead
	for i, k := range l.links {
		if k.trans == nil {
			continue
		}
		y, ok := rows[l.transTop[k.trans.id]]
		if !ok {
			continue
		}
		x, ok := transX[k.trans.id]
		if !ok {
			x = math.Round(meanOr(xs[k.trans.id], 0)/l.grid) * l.grid
		}
		w := l.winding(k.trans, k.cn)
		ts := repair.SymbolTerminals(new(l.transformerProbe(k.trans, x, y)), l.im.opt.Terminals)
		if w < 0 || w >= len(ts) {
			continue
		}
		leads = append(leads, corrLead{i, k.band, k.at.Add(d[regionOf[i]]), ts[w], k.corridor + d[regionOf[i]].Y})
	}
	levels := corridors(leads)
	// The page's size counts too: a straighter lead is not worth a page much wider or taller.
	errSum := 0.0
	page := [4]float64{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}
	for _, r := range regions {
		b := shifted(r.box, d[r])
		page = [4]float64{math.Min(page[0], b[0]), math.Min(page[1], b[1]), math.Max(page[2], b[2]), math.Max(page[3], b[3])}
	}
	for _, x := range transX {
		page[0], page[2] = math.Min(page[0], x), math.Max(page[2], x)
	}
	if page[2] > page[0] {
		errSum += layoutPageWeight * (page[2] - page[0] + page[3] - page[1])
	}
	var linkSegs []seg
	for _, ld := range leads {
		k, at, end := l.links[ld.i], ld.at, ld.end
		pts := []model.Point{at, end}
		if math.Abs(at.X-end.X) > 1e-6 {
			corr := cmp.Or(levels[ld.i], ld.y)
			pts = repair.SimplifyPath([]model.Point{at, {X: at.X, Y: corr}, {X: end.X, Y: corr}, end})
		}
		bend := 1e3
		if end.Y < at.Y {
			bend = 1.25e3 // what hangs under a transformer, like a generator, best lines up with it
		}
		errSum += bend * float64(max(len(pts)-2, 0))
		vl := l.cnVL(k.cn)
		for j := 1; j < len(pts); j++ {
			errSum += math.Abs(pts[j].X-pts[j-1].X) + math.Abs(pts[j].Y-pts[j-1].Y)
			s := seg{pts[j-1], pts[j], vl}
			for _, o := range segs {
				if _, hit := properCrossing(s.a, s.z, o.a, o.z); hit && o.vl != vl || alongSeg(s.a, s.z, o.a, o.z) {
					errSum += 1e6
				}
			}
			for _, o := range linkSegs {
				if _, hit := properCrossing(s.a, s.z, o.a, o.z); hit {
					errSum += 1e6
				}
			}
			linkSegs = append(linkSegs, s)
		}
	}
	return errSum
}

// layoutPageWeight is what a unit of the page's width or height costs in layoutError, against 1 for a unit of lead
// and 1000 for a bend.
const layoutPageWeight = 3

// shiftRegion moves everything region r drew by r.d.
func (l *cimLayout) shiftRegion(r *layRegion) {
	if r.d == (model.Point{}) {
		return
	}
	for _, do := range r.dos {
		for _, p := range l.im.points[do.id] {
			q := pointOf(p).Add(r.d)
			p.props["xPosition"] = []string{strconv.FormatFloat(q.X, 'f', -1, 64)}
			p.props["yPosition"] = []string{strconv.FormatFloat(q.Y, 'f', -1, 64)}
		}
	}
	for _, i := range r.links {
		l.links[i].at = l.links[i].at.Add(r.d)
		l.links[i].corridor += r.d.Y
	}
	for _, cn := range r.cns {
		l.cnAt[cn] = l.cnAt[cn].Add(r.d)
	}
}

func pointOf(p *cimObject) model.Point {
	x, _ := p.num("xPosition")
	y, _ := p.num("yPosition")
	return model.Point{X: x, Y: y}
}

func shifted(b [4]float64, d model.Point) [4]float64 {
	return [4]float64{b[0] + d.X, b[1] + d.Y, b[2] + d.X, b[3] + d.Y}
}

func overlaps(a, b [4]float64) bool {
	return spanOverlap(a[0], a[2], b[0], b[2]) && spanOverlap(a[1], a[3], b[1], b[3])
}

func spanOverlap(a1, a2, b1, b2 float64) bool { return a1 < b2 && b1 < a2 }

// corrLead is a lead through a corridor: link i from at, along the corridor at y, to end.
type corrLead struct {
	i       int
	band    *layBand
	at, end model.Point
	y       float64
}

// corridors gives the leads leaving each side of a band its corridors again, so that none crosses another: a lead
// whose run along the corridor passes another's bay turns farther out than that one, and one passing another's way
// on to its transformer nearer. A side whose leads can't all be so keeps its corridors. Returns the corridor by link.
func corridors(leads []corrLead) map[int]float64 {
	type side struct {
		band *layBand
		down bool
	}
	sides := map[side][]corrLead{}
	var order []side
	for _, k := range leads {
		if math.Abs(k.at.X-k.end.X) < 1e-6 {
			continue
		}
		s := side{k.band, k.y > k.at.Y}
		if sides[s] == nil {
			order = append(order, s)
		}
		sides[s] = append(sides[s], k)
	}
	out := map[int]float64{}
	for _, s := range order {
		ks := sides[s]
		near := func(a, b float64) bool { return a < b == s.down }
		slices.SortStableFunc(ks, func(a, b corrLead) int {
			if a.y == b.y {
				return 0
			}
			if near(a.y, b.y) {
				return -1
			}
			return 1
		})
		inside := func(x float64, k corrLead) bool {
			return x > math.Min(k.at.X, k.end.X)+1e-6 && x < math.Max(k.at.X, k.end.X)-1e-6
		}
		// before[a][b]: a must turn nearer than b.
		before := make([][]bool, len(ks))
		ins := make([]int, len(ks))
		for a := range ks {
			before[a] = make([]bool, len(ks))
		}
		for a := range ks {
			for b := range ks {
				if a == b {
					continue
				}
				if inside(ks[b].at.X, ks[a]) && !before[b][a] {
					before[b][a] = true
					ins[a]++
				}
				if inside(ks[b].end.X, ks[a]) && !before[a][b] {
					before[a][b] = true
					ins[b]++
				}
			}
		}
		var seq []int
		used := make([]bool, len(ks))
		for len(seq) < len(ks) {
			next := -1
			for a := range ks {
				if !used[a] && ins[a] == 0 {
					next = a
					break
				}
			}
			if next < 0 {
				break
			}
			used[next] = true
			seq = append(seq, next)
			for b := range ks {
				if before[next][b] {
					ins[b]--
				}
			}
		}
		if len(seq) < len(ks) {
			continue
		}
		for n, a := range seq {
			out[ks[a].i] = ks[n].y
		}
	}
	return out
}

// alongSeg reports whether segments a–b and c–d lie on one line and share more than a point.
func alongSeg(a, b, c, d model.Point) bool {
	switch {
	case a.X == b.X && c.X == d.X && a.X == c.X:
		return math.Min(math.Max(a.Y, b.Y), math.Max(c.Y, d.Y))-math.Max(math.Min(a.Y, b.Y), math.Min(c.Y, d.Y)) > 1e-6
	case a.Y == b.Y && c.Y == d.Y && a.Y == c.Y:
		return math.Min(math.Max(a.X, b.X), math.Max(c.X, d.X))-math.Max(math.Min(a.X, b.X), math.Min(c.X, d.X)) > 1e-6
	}
	return false
}

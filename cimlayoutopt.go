package slddoc

import (
	"maps"
	"math"
	"strconv"
)

// Arranging a generated layout: after the bands are drawn, each busbar group with its bays (a region) may move as a
// whole, its inner layout untouched, to straighten the links to transformers. Each transformer goes under the bay
// that feeds it from its highest voltage; each region of a lower voltage slides sideways so its bays to transformers
// lie under them, and rises beside the band above where nothing is in the way, higher voltages staying above lower
// ones. The moves are kept only when they lower the layout's error: crossings between voltage classes first, then
// the links' bends and length.

// layRegion is what one busbar group drew: its DiagramObjects, links and the nodes it first drew, and its box.
type layRegion struct {
	group *layGroup
	band  *layBand
	dos   []*cimObject
	links []int    // indexes into cimLayout.links
	cns   []string // nodes whose cnAt it set
	box   [4]float64
	d     Point // its move
}

// transformerProbe is transformer tr centred at (x, y), turned so winding 1 is on top.
func (l *cimLayout) transformerProbe(tr *cimObject, x, y float64) Element {
	probe := Element{Class: ClassPowerTransformer, Shape: "47", X: x, Y: y,
		Windings: make([]TransformerWinding, min(max(len(l.im.transformerEnds(tr)), 2), 4))}
	best, bestScore := 0, math.Inf(-1)
	for _, o := range []int{0, 90, 180, -90} {
		probe.Orient = o
		ts := symbolTerminals(&probe, l.im.opt.Terminals)
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

// arrange moves the regions and places the transformers (l.transX) where that lowers the layout's error, starting
// from the layout as drawn: each transformer may go under or over any bay linked to it, each region of a lower
// voltage under or over any transformer it links to, or just clear of another transformer's leads, rising beside the
// band above where nothing is in the way. Moves are tried until none lowers the error; regions never overlap, nor
// do transformers on a row. rows are the transformer rows by band.
func (l *cimLayout) arrange(regions []*layRegion, trs []*cimObject, rows map[string]float64) {
	g := l.grid
	if len(regions) < 2 || len(trs) == 0 {
		return
	}
	for _, r := range regions {
		l.measureRegion(r)
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
		ts := symbolTerminals(new(l.transformerProbe(tr, 0, y)), l.im.opt.Terminals)
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
	d := map[*layRegion]Point{}
	start := l.layoutError(regions, d, transX, rows)
	best := start

	try := func(nd map[*layRegion]Point, nt map[string]float64) bool {
		l.lift(regions, nd, nt, rows)
		if !l.fits(regions, nd, nt, trs, rows) {
			return false
		}
		if e := l.layoutError(regions, nd, nt, rows); e < best-1e-6 {
			best, d, transX = e, nd, nt
			return true
		}
		return false
	}
	for range 8 {
		improved := false
		for _, tr := range trs {
			if _, ok := transX[tr.id]; !ok {
				continue
			}
			for i, k := range l.links {
				if k.trans != tr {
					continue
				}
				x, ok := centreFor(tr, l.winding(tr, k.cn), k.at.X+d[region[i]].X)
				if !ok || x == transX[tr.id] {
					continue
				}
				nt := maps.Clone(transX)
				nt[tr.id] = x
				improved = try(maps.Clone(d), nt) || improved
			}
		}
		for _, r := range regions {
			if r.band == regions[0].band {
				continue
			}
			var cands []float64
			for _, i := range r.links {
				k := l.links[i]
				if k.trans == nil {
					continue
				}
				tx, ok := transX[k.trans.id]
				if !ok {
					continue
				}
				y, _ := row(k.trans)
				ts := symbolTerminals(new(l.transformerProbe(k.trans, tx, y)), l.im.opt.Terminals)
				if w := l.winding(k.trans, k.cn); w >= 0 && w < len(ts) {
					cands = append(cands, ts[w].X-k.at.X)
				}
			}
			for _, tr := range trs {
				tx, ok := transX[tr.id]
				y, ok2 := row(tr)
				if !ok || !ok2 {
					continue
				}
				left, right := l.reach(tr, y)
				cands = append(cands, tx+right-r.box[0], tx+left-r.box[2])
			}
			cands = append(cands, 0)
			for _, dx := range cands {
				dx = math.Round(dx/g) * g
				if dx == d[r].X {
					continue
				}
				nd := maps.Clone(d)
				nd[r] = Point{X: dx}
				improved = try(nd, maps.Clone(transX)) || improved
			}
		}
		if !improved {
			break
		}
	}
	if best >= start-1e-6 {
		return
	}
	for _, r := range regions {
		r.d = d[r]
		l.shiftRegion(r)
	}
	l.transX = transX
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
}

// lift raises each region of a lower voltage (d's moves sideways given) beside the band above as far as that band's
// top, the regions over it and the transformers allow, setting d's moves up.
func (l *cimLayout) lift(regions []*layRegion, d map[*layRegion]Point, transX map[string]float64, rows map[string]float64) {
	g := l.grid
	var done []*layRegion
	prevTop := math.Inf(-1)
	for i := 0; i < len(regions); {
		band := regions[i].band
		curTop := math.Inf(1)
		for ; i < len(regions) && regions[i].band == band; i++ {
			r := regions[i]
			m := d[r]
			m.Y = 0
			if band != regions[0].band {
				newTop := prevTop
				for _, o := range done {
					if o.band == band {
						continue
					}
					ob := shifted(o.box, d[o])
					if spanOverlap(r.box[0]+m.X, r.box[2]+m.X, ob[0], ob[2]) {
						newTop = math.Max(newTop, ob[3]+2*g)
					}
				}
				for id, tx := range transX {
					tr := l.im.objs[id]
					y, ok := rows[l.transTop[id]]
					if !ok {
						continue
					}
					left, right := l.reach(tr, y)
					if spanOverlap(r.box[0]+m.X, r.box[2]+m.X, tx+left, tx+right) {
						newTop = math.Max(newTop, y+8*g)
					}
				}
				if newTop < r.box[1] {
					m.Y = math.Round((newTop-r.box[1])/g) * g
				}
			}
			d[r] = m
			curTop = math.Min(curTop, r.box[1]+m.Y)
			done = append(done, r)
		}
		prevTop = curTop
	}
}

// fits reports whether everything stays right of the page's margin, and no two regions overlap, nor two transformers
// on a row, nor a transformer and a region.
func (l *cimLayout) fits(regions []*layRegion, d map[*layRegion]Point, transX map[string]float64, trs []*cimObject, rows map[string]float64) bool {
	edge := l.margin / 2
	for _, r := range regions {
		if r.box[0]+d[r].X < edge {
			return false
		}
	}
	for i, a := range regions {
		for _, b := range regions[i+1:] {
			if overlaps(shifted(a.box, d[a]), shifted(b.box, d[b])) {
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

// reach is how far transformer tr's terminals reach left and right of its centre on row y.
func (l *cimLayout) reach(tr *cimObject, y float64) (float64, float64) {
	left, right := 0.0, 0.0
	for _, t := range symbolTerminals(new(l.transformerProbe(tr, 0, y)), l.im.opt.Terminals) {
		left, right = math.Min(left, t.X), math.Max(right, t.X)
	}
	return left - 3*l.grid, right + 3*l.grid
}

// layoutError estimates the layout with regions moved by d and transformers at transX (nil: where the links' mean
// puts them): crossings of links with busbars, wires and other links of another voltage class, then the links' bends
// and length.
func (l *cimLayout) layoutError(regions []*layRegion, d map[*layRegion]Point, transX map[string]float64, rows map[string]float64) float64 {
	type seg struct {
		a, z Point
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
				segs = append(segs, seg{pointOf(pts[k-1]).add(d[r]), pointOf(pts[k]).add(d[r]), vl})
			}
		}
	}
	xs := map[string][]float64{}
	for i, k := range l.links {
		if k.trans != nil {
			xs[k.trans.id] = append(xs[k.trans.id], k.at.X+d[regionOf[i]].X)
		}
	}
	errSum := 0.0
	var linkSegs []seg
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
		ts := symbolTerminals(new(l.transformerProbe(k.trans, x, y)), l.im.opt.Terminals)
		if w < 0 || w >= len(ts) {
			continue
		}
		at := k.at.add(d[regionOf[i]])
		end := ts[w]
		pts := []Point{at, end}
		if math.Abs(at.X-end.X) > 1e-6 {
			corr := k.corridor + d[regionOf[i]].Y
			pts = []Point{at, {X: at.X, Y: corr}, {X: end.X, Y: corr}, end}
			errSum += 2e3
		}
		vl := l.cnVL(k.cn)
		for j := 1; j < len(pts); j++ {
			errSum += math.Abs(pts[j].X-pts[j-1].X) + math.Abs(pts[j].Y-pts[j-1].Y)
			s := seg{pts[j-1], pts[j], vl}
			for _, o := range segs {
				if o.vl != vl {
					if _, hit := properCrossing(s.a, s.z, o.a, o.z); hit {
						errSum += 1e6
					}
				}
			}
			for _, o := range linkSegs {
				if o.vl != vl {
					if _, hit := properCrossing(s.a, s.z, o.a, o.z); hit {
						errSum += 1e6
					}
				}
			}
			linkSegs = append(linkSegs, s)
		}
	}
	return errSum
}

// shiftRegion moves everything region r drew by r.d.
func (l *cimLayout) shiftRegion(r *layRegion) {
	if r.d == (Point{}) {
		return
	}
	for _, do := range r.dos {
		for _, p := range l.im.points[do.id] {
			q := pointOf(p).add(r.d)
			p.props["xPosition"] = []string{strconv.FormatFloat(q.X, 'f', -1, 64)}
			p.props["yPosition"] = []string{strconv.FormatFloat(q.Y, 'f', -1, 64)}
		}
	}
	for _, i := range r.links {
		l.links[i].at = l.links[i].at.add(r.d)
		l.links[i].corridor += r.d.Y
	}
	for _, cn := range r.cns {
		l.cnAt[cn] = l.cnAt[cn].add(r.d)
	}
}

func pointOf(p *cimObject) Point {
	x, _ := p.num("xPosition")
	y, _ := p.num("yPosition")
	return Point{X: x, Y: y}
}

func (p Point) add(q Point) Point { return Point{X: p.X + q.X, Y: p.Y + q.Y} }

func shifted(b [4]float64, d Point) [4]float64 {
	return [4]float64{b[0] + d.X, b[1] + d.Y, b[2] + d.X, b[3] + d.Y}
}

func overlaps(a, b [4]float64) bool {
	return spanOverlap(a[0], a[2], b[0], b[2]) && spanOverlap(a[1], a[3], b[1], b[3])
}

func spanOverlap(a1, a2, b1, b2 float64) bool { return a1 < b2 && b1 < a2 }

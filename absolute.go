package slddoc

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Static output follows real xsde2svg's own placement convention: every
// coordinate is absolute, and a rotated symbol carries
// transform="rotate(angle,x,y)" around its own anchor (see
// PS_110kV_Valdai.svg's breakers and transformer). The symbol templates and
// the hand-written emitters in render.go instead draw in the symbol's local
// frame, placed by transform="translate(x,y) rotate(angle)[ scale(-1,1)]" —
// the form the live canvas (Interactive) needs, since it drags an element by
// rewriting that translate and measures getBBox() in the local frame.
// absolutize converts one rendered fragment from the local form into the
// absolute one, so Render can emit both from the same code.

var (
	tagRe           = regexp.MustCompile(`<(/?)([A-Za-z][\w:-]*)((?:\s+[\w:-]+="[^"]*")*)\s*(/?)>`)
	attrRe          = regexp.MustCompile(`(\s+)([\w:-]+)="([^"]*)"`)
	transformRe     = regexp.MustCompile(`\s*([a-zA-Z]+)\s*\(([^)]*)\)`)
	numberRe        = regexp.MustCompile(`[+-]?(?:\d+\.?\d*|\.\d+)(?:[eE][+-]?\d+)?`)
	tagOpenRe       = regexp.MustCompile(`</?[A-Za-z]`)
	transformAttrRe = regexp.MustCompile(`\stransform="([^"]*)"`)
	pathTokenAll    = regexp.MustCompile(`[A-Za-z]|[+-]?(?:\d+\.?\d*|\.\d+)(?:[eE][+-]?\d+)?`)
)

// transformOp is one item of an SVG transform list, e.g. rotate(90,10,20).
type transformOp struct {
	name string
	args []float64
}

func parseTransformList(s string) ([]transformOp, error) {
	var ops []transformOp
	rest := strings.TrimSpace(s)
	for rest != "" {
		loc := transformRe.FindStringSubmatchIndex(rest)
		if loc == nil || loc[0] != 0 {
			return nil, fmt.Errorf("slddoc: transform %q: unparseable", s)
		}
		name := rest[loc[2]:loc[3]]
		var args []float64
		for _, n := range numberRe.FindAllString(rest[loc[4]:loc[5]], -1) {
			v, err := strconv.ParseFloat(n, 64)
			if err != nil {
				return nil, fmt.Errorf("slddoc: transform %q: %w", s, err)
			}
			args = append(args, v)
		}
		ops = append(ops, transformOp{name, args})
		rest = strings.TrimLeft(rest[loc[1]:], " ,\t\n")
	}
	return ops, nil
}

func (op transformOp) String() string {
	parts := make([]string, len(op.args))
	for i, a := range op.args {
		parts[i] = fmtNum(a)
	}
	return op.name + "(" + strings.Join(parts, ",") + ")"
}

// conjugate rewrites one transform op that applied inside a frame placed at
// (tx,ty) so it means the same once that frame's own content is shifted by
// (tx,ty) and the frame's translate is gone: T·op·T⁻¹.
func conjugate(op transformOp, tx, ty float64) ([]transformOp, error) {
	switch op.name {
	case "translate":
		return []transformOp{op}, nil
	case "rotate":
		switch len(op.args) {
		case 1:
			return []transformOp{{"rotate", []float64{op.args[0], tx, ty}}}, nil
		case 3:
			return []transformOp{{"rotate", []float64{op.args[0], op.args[1] + tx, op.args[2] + ty}}}, nil
		}
	case "scale":
		if len(op.args) == 1 || len(op.args) == 2 {
			return []transformOp{{"translate", []float64{tx, ty}}, op, {"translate", []float64{-tx, -ty}}}, nil
		}
	case "matrix":
		if len(op.args) == 6 {
			a, b, c, d, e, f := op.args[0], op.args[1], op.args[2], op.args[3], op.args[4], op.args[5]
			return []transformOp{{"matrix", []float64{a, b, c, d, e + tx - a*tx - c*ty, f + ty - b*tx - d*ty}}}, nil
		}
	}
	return nil, fmt.Errorf("slddoc: transform %s: not supported", op)
}

// isNoOp reports a translate by zero or a rotate by zero degrees.
func isNoOp(op transformOp) bool {
	switch op.name {
	case "translate":
		for _, a := range op.args {
			if a != 0 {
				return false
			}
		}
		return true
	case "rotate":
		return len(op.args) > 0 && op.args[0] == 0
	}
	return false
}

func joinOps(ops []transformOp) string {
	parts := make([]string, len(ops))
	for i, op := range ops {
		parts[i] = op.String()
	}
	return strings.Join(parts, " ")
}

// placement recognizes a fragment's own outer local-frame placement,
// "translate(x,y) [rotate(a)] [scale(-1,1)]", returning its translation and
// the transform the absolute form uses instead ("" when there is none left).
func placement(transform string) (tx, ty float64, abs string, ok bool) {
	ops, err := parseTransformList(transform)
	if err != nil || len(ops) == 0 || ops[0].name != "translate" || len(ops[0].args) == 0 || len(ops[0].args) > 2 {
		return 0, 0, "", false
	}
	tx = ops[0].args[0]
	if len(ops[0].args) == 2 {
		ty = ops[0].args[1]
	}
	var out []transformOp
	for _, op := range ops[1:] {
		switch {
		case op.name == "rotate" && len(op.args) == 1:
			if op.args[0] != 0 {
				out = append(out, transformOp{"rotate", []float64{op.args[0], tx, ty}})
			}
		case op.name == "scale" && len(op.args) == 2 && op.args[0] == -1 && op.args[1] == 1:
			// A mirror about the anchor: x -> 2tx - x.
			out = append(out, transformOp{"translate", []float64{2 * tx, 0}}, transformOp{"scale", []float64{-1, 1}})
		default:
			return 0, 0, "", false
		}
	}
	return tx, ty, joinOps(out), true
}

// shiftPathData adds (tx,ty) to every absolute coordinate of path data d.
// Relative commands are left alone, except that a path's very first
// moveto is absolute even when written "m".
func shiftPathData(d string, tx, ty float64) (string, error) {
	toks := pathTokenAll.FindAllString(d, -1)
	out := make([]string, 0, len(toks))
	cmd := ""
	arg := 0
	firstMove := true
	for _, tok := range toks {
		if c := tok[0]; (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			if !strings.Contains("MmLlHhVvCcSsQqTtAaZz", tok) || len(tok) != 1 {
				return "", fmt.Errorf("slddoc: path %q: unsupported command %q", d, tok)
			}
			if cmd != "" {
				firstMove = false
			}
			cmd, arg = tok, 0
			out = append(out, tok)
			continue
		}
		v, err := strconv.ParseFloat(tok, 64)
		if err != nil {
			return "", fmt.Errorf("slddoc: path %q: %w", d, err)
		}
		shift := 0.0
		switch cmd {
		case "M", "L", "C", "S", "Q", "T":
			shift = pick(arg%2 == 0, tx, ty)
		case "m":
			if firstMove && arg < 2 {
				shift = pick(arg%2 == 0, tx, ty)
			}
		case "H":
			shift = tx
		case "V":
			shift = ty
		case "A":
			switch arg % 7 {
			case 5:
				shift = tx
			case 6:
				shift = ty
			}
		case "":
			return "", fmt.Errorf("slddoc: path %q: number before any command", d)
		}
		arg++
		if shift == 0 {
			out = append(out, tok)
		} else {
			out = append(out, fmtNum(v+shift))
		}
	}
	return strings.Join(out, " "), nil
}

func pick(first bool, a, b float64) float64 {
	if first {
		return a
	}
	return b
}

func shiftNumber(s string, by float64) (string, error) {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return "", fmt.Errorf("slddoc: coordinate %q: %w", s, err)
	}
	return fmtNum(v + by), nil
}

func shiftPoints(s string, tx, ty float64) (string, error) {
	nums := numberRe.FindAllString(s, -1)
	if len(nums)%2 != 0 {
		return "", fmt.Errorf("slddoc: points %q: odd coordinate count", s)
	}
	pairs := make([]string, 0, len(nums)/2)
	for i := 0; i < len(nums); i += 2 {
		x, err := shiftNumber(nums[i], tx)
		if err != nil {
			return "", err
		}
		y, err := shiftNumber(nums[i+1], ty)
		if err != nil {
			return "", err
		}
		pairs = append(pairs, x+","+y)
	}
	return strings.Join(pairs, " "), nil
}

// shiftAttrs moves one tag's own geometry by (tx,ty) and conjugates its own
// transform, if any. outer is the replacement for the fragment root's own
// placement transform ("" drops it), used only when isRoot.
func shiftAttrs(tag, attrs string, tx, ty float64, isRoot bool, outer string) (string, error) {
	var err error
	res := attrRe.ReplaceAllStringFunc(attrs, func(m string) string {
		if err != nil {
			return m
		}
		sm := attrRe.FindStringSubmatch(m)
		space, name, val := sm[1], sm[2], sm[3]
		var nv string
		var e error
		switch {
		case name == "transform":
			if isRoot {
				if outer == "" {
					return ""
				}
				return space + `transform="` + outer + `"`
			}
			ops, pe := parseTransformList(val)
			if pe != nil {
				e = pe
				break
			}
			var conj []transformOp
			for _, op := range ops {
				c, ce := conjugate(op, tx, ty)
				if ce != nil {
					e = ce
					break
				}
				for _, o := range c {
					if !isNoOp(o) {
						conj = append(conj, o)
					}
				}
			}
			if e == nil && len(conj) == 0 {
				// e.g. a template's translate({positionOffset},0) at
				// offset 0, or rotate({counterRotate}) at orient 0.
				return ""
			}
			nv = joinOps(conj)
		case name == "d" && tag == "path":
			nv, e = shiftPathData(val, tx, ty)
		case name == "points" && (tag == "polyline" || tag == "polygon"):
			nv, e = shiftPoints(val, tx, ty)
		case (name == "x" || name == "x1" || name == "x2" || name == "cx") && tag != "svg":
			nv, e = shiftNumber(val, tx)
		case (name == "y" || name == "y1" || name == "y2" || name == "cy") && tag != "svg":
			nv, e = shiftNumber(val, ty)
		default:
			return m
		}
		if e != nil {
			err = e
			return m
		}
		return space + name + `="` + nv + `"`
	})
	return res, err
}

// absolutize rewrites every top-level element of frag that is placed in a
// local frame (see placement) into absolute coordinates, leaving the rest
// untouched. It returns an error, and the caller keeps frag as it was, for
// any transform or path form it doesn't know how to carry over exactly.
func absolutize(frag string) (string, error) {
	if n := len(tagOpenRe.FindAllStringIndex(frag, -1)); n != len(tagRe.FindAllStringIndex(frag, -1)) {
		return frag, fmt.Errorf("slddoc: fragment has a tag absolutize can't read")
	}
	type frame struct {
		tx, ty float64
		placed bool
	}
	var stack []frame
	var err error
	out := tagRe.ReplaceAllStringFunc(frag, func(m string) string {
		if err != nil {
			return m
		}
		sm := tagRe.FindStringSubmatch(m)
		closing, tag, attrs, selfClose := sm[1] == "/", sm[2], sm[3], sm[4] == "/"
		if closing {
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			return m
		}
		isRoot := len(stack) == 0
		var f frame
		outer := ""
		if isRoot {
			if tm := transformAttrRe.FindStringSubmatch(attrs); tm != nil {
				if tx, ty, abs, ok := placement(tm[1]); ok {
					f, outer = frame{tx, ty, true}, abs
				}
			}
		} else {
			f = stack[len(stack)-1]
		}
		if !selfClose {
			stack = append(stack, f)
		}
		if !f.placed {
			return m
		}
		na, e := shiftAttrs(tag, attrs, f.tx, f.ty, isRoot, outer)
		if e != nil {
			err = e
			return m
		}
		end := ">"
		if selfClose {
			end = " />"
		}
		return "<" + tag + na + end
	})
	if err != nil {
		return frag, err
	}
	return out, nil
}

package extract

import (
	"strconv"

	"github.com/PVKonovalov/slddoc/internal/model"
	"github.com/PVKonovalov/slddoc/internal/svgtree"
)

// sourceMirrorShapes are the shapes whose Mirror is xsde2svg's xMirror: drawn as the same symbol flipped about its
// axis, checked against xsde2svg's drawing. Another shape's xMirror is ignored by xsde2svg, drawn differently
// (Thyristor 157 turns half size) or not checked yet (173, 360, 397).
var sourceMirrorShapes = map[string]bool{
	"19": true, "29": true, "32": true, "33": true, "34": true, "37": true, "38": true, "43": true, "47": true,
	"50": true, "51": true, "163": true, "164": true, "166": true, "168": true, "398": true, "399": true,
}

// sourceScale reads the size step xsde2svg writes on an element's node (data-scale, since 1.4.28), clamped to the
// editor's range. ok is false when the node has none: an older xsde2svg, or step 0.
func sourceScale(n *svgtree.RawNode) (step int, ok bool) {
	s, err := strconv.Atoi(n.Attr("data-scale"))
	if err != nil {
		return 0, false
	}
	return clampStep(s), true
}

// sourceMirror reports whether xsde2svg marks an element's node as mirrored (data-mirror="1", since 1.4.28).
func sourceMirror(n *svgtree.RawNode) bool {
	return n.Attr("data-mirror") == "1"
}

// applySourceAttrs sets el's Scale from data-scale (for a class with a size step) and Mirror from data-mirror (for a
// shape in sourceMirrorShapes), where n carries them. Without them el keeps what was read from the drawing.
func applySourceAttrs(el *model.Element, n *svgtree.RawNode) {
	if n == nil {
		return
	}
	if s, ok := sourceScale(n); ok && model.UsesSizeStep(el.Class) {
		el.Scale = s
	}
	if sourceMirror(n) && sourceMirrorShapes[el.Shape] {
		el.Mirror = true
	}
}

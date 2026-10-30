package svgtree

import (
	"bytes"
	"encoding/xml"
	"io"
)

// RawNode is a minimal in-memory DOM built from an SVG file, used only
// during extraction to inspect element structure and geometry. It does not
// aim to be a general XML DOM: attributes are flattened to their local name
// (no namespace handling beyond what encoding/xml already strips), and
// mixed text/element content only tracks the concatenated text.
type RawNode struct {
	Tag      string
	Attrs    map[string]string
	Children []*RawNode
	Text     string
}

func (n *RawNode) Attr(name string) string {
	if n == nil {
		return ""
	}
	return n.Attrs[name]
}

func (n *RawNode) ChildrenTagged(tag string) []*RawNode {
	var out []*RawNode
	for _, c := range n.Children {
		if c.Tag == tag {
			out = append(out, c)
		}
	}
	return out
}

// Descendants returns every node in n's subtree (n excluded) matching tag,
// in document order.
func (n *RawNode) Descendants(tag string) []*RawNode {
	var out []*RawNode
	var walk func(*RawNode)
	walk = func(cur *RawNode) {
		for _, c := range cur.Children {
			if c.Tag == tag {
				out = append(out, c)
			}
			walk(c)
		}
	}
	walk(n)
	return out
}

// FirstAttrDescendant returns the value of attr on the first descendant (n
// included) that carries it, or "" if none do.
func (n *RawNode) FirstAttrDescendant(attr string) string {
	if v, ok := n.Attrs[attr]; ok {
		return v
	}
	for _, c := range n.Children {
		if v := c.FirstAttrDescendant(attr); v != "" {
			return v
		}
	}
	return ""
}

// ParseRawTree parses raw SVG bytes into a RawNode tree rooted at the <svg>
// element.
func ParseRawTree(raw []byte) (*RawNode, error) {
	dec := xml.NewDecoder(bytes.NewReader(raw))

	var root *RawNode
	var stack []*RawNode

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			n := &RawNode{Tag: t.Name.Local, Attrs: map[string]string{}}
			for _, a := range t.Attr {
				n.Attrs[a.Name.Local] = a.Value
			}
			if len(stack) > 0 {
				parent := stack[len(stack)-1]
				parent.Children = append(parent.Children, n)
			} else {
				root = n
			}
			stack = append(stack, n)
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) > 0 {
				stack[len(stack)-1].Text += string(t)
			}
		}
	}
	return root, nil
}

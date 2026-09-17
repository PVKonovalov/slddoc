package slddoc

import (
	"bytes"
	"encoding/xml"
	"io"
)

// rawNode is a minimal in-memory DOM built from an SVG file, used only
// during extraction to inspect element structure and geometry. It does not
// aim to be a general XML DOM: attributes are flattened to their local name
// (no namespace handling beyond what encoding/xml already strips), and
// mixed text/element content only tracks the concatenated text.
type rawNode struct {
	Tag      string
	Attrs    map[string]string
	Children []*rawNode
	Text     string
}

func (n *rawNode) attr(name string) string {
	if n == nil {
		return ""
	}
	return n.Attrs[name]
}

func (n *rawNode) childrenTagged(tag string) []*rawNode {
	var out []*rawNode
	for _, c := range n.Children {
		if c.Tag == tag {
			out = append(out, c)
		}
	}
	return out
}

// descendants returns every node in n's subtree (n excluded) matching tag,
// in document order.
func (n *rawNode) descendants(tag string) []*rawNode {
	var out []*rawNode
	var walk func(*rawNode)
	walk = func(cur *rawNode) {
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

// firstAttrDescendant returns the value of attr on the first descendant (n
// included) that carries it, or "" if none do.
func (n *rawNode) firstAttrDescendant(attr string) string {
	if v, ok := n.Attrs[attr]; ok {
		return v
	}
	for _, c := range n.Children {
		if v := c.firstAttrDescendant(attr); v != "" {
			return v
		}
	}
	return ""
}

// parseRawTree parses raw SVG bytes into a rawNode tree rooted at the <svg>
// element.
func parseRawTree(raw []byte) (*rawNode, error) {
	dec := xml.NewDecoder(bytes.NewReader(raw))

	var root *rawNode
	var stack []*rawNode

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
			n := &rawNode{Tag: t.Name.Local, Attrs: map[string]string{}}
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

package extract

import (
	"testing"

	"github.com/PVKonovalov/slddoc/internal/model"
)

// TestExtractZoneDivision reads real xsde2svg Zone divisions (element_60.go): a bare path followed by its untyped
// magenta "Z". The first sits inline between two buswork wires, which connect to its two terminals; the second is
// rotated with one end free.
func TestExtractZoneDivision(t *testing.T) {
	const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="400" height="400">
<polyline points="100,40 100,50" style="fill:none;stroke:#555555;stroke-width:1 " data-type="21" id="11" data-voltage="#555555" />
<path d="M 100 50 v 10 " style="fill:none;stroke:#555555;stroke-width:1" id="12" data-type="60" data-voltage="#555555" />
<path d="M 95 50 h 10 l -10 10 h 10" style="fill:none;stroke:magenta;stroke-width:2" />
<polyline points="100,60 100,80" style="fill:none;stroke:#555555;stroke-width:1 " data-type="21" id="13" data-voltage="#555555" />
<path d="M 200 50 v 10 " style="fill:none;stroke:teal;stroke-width:1" transform="rotate(90,200,50)" id="14" data-type="60" data-voltage="teal" />
<path d="M 195 50 h 10 l -10 10 h 10" style="fill:none;stroke:magenta;stroke-width:2" transform="rotate(90,200,50)" />
</svg>`
	d, report, err := Extract([]byte(svg), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Failed) != 0 || len(report.Skipped) != 0 || len(d.Elements) != 2 {
		t.Fatalf("report = %+v, elements = %+v", report, d.Elements)
	}
	zd, rotated := d.Elements[0], d.Elements[1]
	if zd.Class != model.ClassZoneDivision || zd.Shape != "60" || zd.X != 100 || zd.Y != 50 || zd.Orient != 0 || len(zd.Ports) != 2 {
		t.Errorf("inline = %+v", zd)
	}
	if rotated.X != 200 || rotated.Y != 50 || rotated.Orient != 90 {
		t.Errorf("rotated = %+v", rotated)
	}
	nodes := map[int]bool{}
	for _, c := range d.Connectors {
		nodes[c.From], nodes[c.To] = true, true
	}
	for _, p := range zd.Ports {
		if p.Node == 0 || !nodes[p.Node] {
			t.Errorf("port %s node %d not on a wire (connectors %+v)", p.Name, p.Node, d.Connectors)
		}
	}
	if zd.Ports[0].Node == zd.Ports[1].Node {
		t.Errorf("both ports on node %d", zd.Ports[0].Node)
	}
}

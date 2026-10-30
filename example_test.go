package slddoc_test

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/PVKonovalov/slddoc"
)

// feeder is a 10 kV busbar feeding a line through a breaker. The breaker has no ports yet: Repair adds them.
func feeder() *slddoc.Diagram {
	return &slddoc.Diagram{
		Width: 200, Height: 160,
		VoltageClasses: []slddoc.VoltageClass{{ID: 1, Name: "10kV", Color: "#962896"}},
		Nodes: []slddoc.Node{
			{ID: 6, X: 100, Y: 40}, {ID: 7, X: 100, Y: 70}, {ID: 8, X: 100, Y: 90}, {ID: 9, X: 100, Y: 140},
		},
		Elements: []slddoc.Element{
			{ID: 2, Class: slddoc.ClassBusBarSection, Shape: "24", Name: "Bus 1", Voltage: 1,
				Points: []slddoc.Point{{X: 20, Y: 40}, {X: 180, Y: 40}}},
			{ID: 3, Class: slddoc.ClassBreaker, Shape: "41", Name: "CB 1", Voltage: 1, X: 100, Y: 80},
		},
		Connectors: []slddoc.Connector{
			{ID: 4, Kind: slddoc.KindBusWork, Voltage: 1, From: 6, To: 7, Points: []slddoc.Point{{X: 100, Y: 40}, {X: 100, Y: 70}}},
			{ID: 5, Kind: slddoc.KindBusWork, Voltage: 1, From: 8, To: 9, Points: []slddoc.Point{{X: 100, Y: 90}, {X: 100, Y: 140}}},
		},
		LastID: 9,
	}
}

// terminals are each shape's port positions in its local coordinates.
var terminals = map[string][]slddoc.Point{"41": {{X: 0, Y: -10}, {X: 0, Y: 10}}}

var symbols = slddoc.NewSymbolLibrary(map[string]string{
	"41": `<path d="M -7 -7 h 14 v 14 h -14 z M 0 -10 v 3 M 0 7 v 3" style="fill:{fill};stroke:{color};stroke-width:1" />`,
})

var stateColors = []slddoc.StateColor{
	{State: 0, Label: "Open", Color: "red"},
	{State: 1, Label: "Close", Color: "lawngreen"},
}

// Repair builds the electrical topology from the geometry: nodes where wires meet ports and busbars.
func ExampleRepair() {
	d := feeder()
	slddoc.Repair(d, terminals)
	for _, c := range d.Connectors {
		fmt.Printf("connector %d: node %d -> node %d\n", c.ID, c.From, c.To)
	}
	for _, p := range d.Elements[1].Ports {
		fmt.Printf("%s port %s: node %d\n", d.Elements[1].Name, p.Name, p.Node)
	}
	// Output:
	// connector 4: node 6 -> node 10
	// connector 5: node 11 -> node 9
	// CB 1 port 1: node 10
	// CB 1 port 2: node 11
}

// A diagram is saved as an .xsld document and loaded back.
func ExampleLoad() {
	var buf bytes.Buffer
	if err := feeder().Save(&buf); err != nil {
		panic(err)
	}
	d, err := slddoc.Load(&buf)
	if err != nil {
		panic(err)
	}
	fmt.Println(len(d.Elements), "elements,", len(d.Connectors), "connectors")
	// Output: 2 elements, 2 connectors
}

// Render draws the diagram as an xsde2svg-compatible SVG.
func ExampleRender() {
	var buf bytes.Buffer
	if err := slddoc.Render(feeder(), symbols, &buf, slddoc.Static, "", nil, stateColors...); err != nil {
		panic(err)
	}
	svg := buf.String()
	fmt.Println(strings.Contains(svg, "<svg"), strings.Contains(svg, "CB 1"))
	// Output: true true
}

// Extract reads a diagram back from an SVG.
func ExampleExtract() {
	var buf bytes.Buffer
	if err := slddoc.Render(feeder(), symbols, &buf, slddoc.Static, "", nil, stateColors...); err != nil {
		panic(err)
	}
	d, report, err := slddoc.Extract(buf.Bytes(), "feeder.svg", nil)
	if err != nil {
		panic(err)
	}
	fmt.Println(report.Elements, "elements,", report.Connectors, "connectors,", d.VoltageClasses[0].Color)
	// Output: 2 elements, 2 connectors, #962896
}

// ExportCIM writes CGMES 3.0 Equipment and Diagram Layout documents; ImportCIM reads them back.
func ExampleExportCIM() {
	classes, _, err := slddoc.NewCIMClassTable(
		map[string]slddoc.Class{"24": slddoc.ClassBusBarSection, "41": slddoc.ClassBreaker},
		map[string][]string{"24": {"BusbarSection"}, "41": {"Breaker"}},
		map[string]string{"BusbarSection": "24", "Breaker": "41"},
	)
	if err != nil {
		panic(err)
	}
	d := feeder()
	slddoc.Repair(d, terminals)
	out := slddoc.ExportCIM(d, slddoc.CIMOptions{Name: "Feeder", Created: time.Unix(0, 0).UTC(), Classes: classes})

	pages, err := slddoc.ImportCIM([][]byte{out.EQ, out.DL}, slddoc.CIMImportOptions{
		Terminals:  terminals,
		Classes:    map[string]slddoc.Class{"24": slddoc.ClassBusBarSection, "41": slddoc.ClassBreaker},
		CIMClasses: classes,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	for _, p := range pages {
		fmt.Println(p.Layout, len(p.Diagram.Elements), "elements")
	}
	// Output: own 2 elements
}

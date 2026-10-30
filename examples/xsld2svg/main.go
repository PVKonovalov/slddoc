// Command xsld2svg renders an .xsld document as an xsde2svg-compatible SVG (sld-svg's `svg-sld render`).
//
//	go run ./examples/xsld2svg -in examples/testdata/feeder.xsld -symbols examples/testdata/symbols.xml -out feeder.svg
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/PVKonovalov/slddoc"
)

var stateColors = []slddoc.StateColor{
	{State: 0, Label: "Open", Color: "red"},
	{State: 1, Label: "Close", Color: "lawngreen"},
	{State: 2, Label: "Intermediate", Color: "yellow"},
}

func main() {
	in := flag.String("in", "", "source .xsld (required)")
	symbols := flag.String("symbols", "", "symbol library .xml (required)")
	out := flag.String("out", "", "output SVG (required)")
	flag.Parse()
	if *in == "" || *symbols == "" || *out == "" {
		flag.Usage()
		os.Exit(2)
	}
	if err := run(*in, *symbols, *out); err != nil {
		fmt.Fprintln(os.Stderr, "xsld2svg:", err)
		os.Exit(1)
	}
}

func run(in, symbols, out string) error {
	sf, err := os.Open(symbols)
	if err != nil {
		return err
	}
	lib, err := slddoc.LoadSymbolLibrary(sf)
	sf.Close()
	if err != nil {
		return err
	}
	df, err := os.Open(in)
	if err != nil {
		return err
	}
	d, err := slddoc.Load(df)
	df.Close()
	if err != nil {
		return err
	}
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := slddoc.Render(d, lib, f, slddoc.Static, "", nil, stateColors...); err != nil {
		return err
	}
	fmt.Printf("%s -> %s: %d elements, %d connectors\n", in, out, len(d.Elements), len(d.Connectors))
	return nil
}

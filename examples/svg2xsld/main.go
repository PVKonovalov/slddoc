// Command svg2xsld extracts a diagram from an xsde2svg SVG into an .xsld document (sld-svg's `svg-sld extract`).
//
//	go run ./examples/svg2xsld -in diagram.svg -out diagram.xsld [-voltage-hints hints.csv]
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/PVKonovalov/slddoc"
)

func main() {
	in := flag.String("in", "", "source SVG (required)")
	out := flag.String("out", "", "output .xsld (required)")
	hintsPath := flag.String("voltage-hints", "", "optional hexColor;name CSV of known voltage-class colors")
	flag.Parse()
	if *in == "" || *out == "" {
		flag.Usage()
		os.Exit(2)
	}
	if err := run(*in, *out, *hintsPath); err != nil {
		fmt.Fprintln(os.Stderr, "svg2xsld:", err)
		os.Exit(1)
	}
}

func run(in, out, hintsPath string) error {
	var hints map[string]string
	if hintsPath != "" {
		f, err := os.Open(hintsPath)
		if err != nil {
			return err
		}
		hints, err = slddoc.LoadVoltageHints(f)
		f.Close()
		if err != nil {
			return err
		}
	}
	raw, err := os.ReadFile(in)
	if err != nil {
		return err
	}
	d, report, err := slddoc.Extract(raw, filepath.Base(in), hints)
	if err != nil {
		return err
	}
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := d.Save(f); err != nil {
		return err
	}
	fmt.Printf("%s -> %s: %d elements, %d connectors, %d nodes, %d labels\n",
		in, out, report.Elements, report.Connectors, report.Nodes, report.Labels)
	if len(report.Skipped) > 0 {
		fmt.Printf("skipped types: %v\n", report.Skipped)
	}
	return nil
}

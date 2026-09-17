// Package slddoc models a single-line diagram (SLD) as a standalone XML
// document — elements, their coordinates, and the electrical topology
// connecting them — extracted from an xsde2svg-generated SVG, and later
// rendered back into a fresh SVG using a symbol library.
//
// This package does not aim for byte-identical round-tripping of the
// source SVG: it builds a new object model from the SVG's geometry and
// re-renders from scratch, so exact original formatting is not preserved.
//
// This is a standalone module shared by two sibling projects: sld-svg
// (github.com/PVKonovalov/sld-svg), which owns Extract (reconstructing a
// Diagram from a real xsde2svg-generated SVG) and is the reference for
// corpus-faithful behavior here; and sld-editor
// (github.com/PVKonovalov/sld-editor), an interactive diagram editor built
// on top of Render, which needs to load the same XML format Extract
// produces. Every id (Element/Node/Connector/VoltageClass/Layer, and every
// field referencing one) is a plain int, never a string; 0 doubles as
// "unset" for an optional reference since a real id from a source SVG is
// never 0.
package slddoc

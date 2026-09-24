# slddoc

A Go module modeling a single-line diagram (SLD) — the elements, their
coordinates, and the electrical topology connecting them — as a standalone
XML document, plus the SVG rendering that draws it back from a symbol
library.

It's a shared library, not an application: two sibling projects import it
rather than each keeping their own copy of this model.

- **[sld-svg](https://github.com/PVKonovalov/sld-svg)** owns `Extract`,
  reconstructing a `Diagram` from a real
  [xsde2svg](https://github.com/PVKonovalov/xsde2svg)-generated SVG, and is
  the reference for corpus-faithful behavior here.
- **[sld-editor](https://github.com/PVKonovalov/sld-editor)**, an
  interactive diagram editor, builds on `Render` and needs to load the same
  XML format `Extract` produces. Its own extensions to the model and
  renderer (JSON tags for its REST API, per-diagram editor settings, a
  `RenderMode` for interactivity-only markup, ...) live here too — see
  `doc.go`.

This package does not aim for byte-identical round-tripping of a source
SVG: `Extract` builds a fresh object model from the SVG's geometry, and
`Render` re-renders it from scratch, so exact original formatting is never
preserved.

## XML format

The diagram XML format (every section, attribute, id rule, the topology
model and the element/connector code tables) is described in
[`FORMAT.md`](FORMAT.md).

## Install

```
go get github.com/PVKonovalov/slddoc
```

## Layout

A single flat package at the module root — `Diagram`/`Element`/`Connector`/
`Node`/`Label`/... in `model.go`, XML `Load`/`Save` alongside them,
`Extract` and its shape-specific parsers in `extract.go`/`elements.go`/
`topology.go`/`layers.go`/`voltage.go`/`labels.go`, SVG `Render` in
`render.go`, and the `SymbolLibrary` it draws from in `symbols.go`. Every
file has its own `_test.go` sibling.

## Development

Both consuming repos build against a local checkout of this module via a
`go.mod` `replace` directive (a sibling `../slddoc` checkout), rather than
a tagged release, while the three are developed together.

```
go build ./...
go vet ./...
go test ./...
```

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
[`XSLD_FORMAT.md`](XSLD_FORMAT.md).

## Install

```
go get github.com/PVKonovalov/slddoc
```

## Layout

Consumers import only the root package `slddoc`; `slddoc.go` re-exports the
API from the internal packages that implement it:

| Package | Contents |
|---|---|
| `internal/model` | `Diagram`/`Element`/`Connector`/`Node`/`Label`/..., XSLD `Load`/`Save`, ids, style, geometry, voltage, size steps, electrical parameters |
| `internal/svgtree` | the raw SVG tree `Extract` reads |
| `internal/render` | SVG `Render`/`RenderFragments`, `SymbolLibrary`, absolute coordinates, leads, layer metadata |
| `internal/extract` | `Extract` and its shape-specific parsers, topology, layers, labels, taps |
| `internal/repair` | `Repair`, `EnsureIDs`, `InferSizeSteps` |
| `internal/cim` | CIM export/import, foreign layouts, automatic layout and lead routing |

Each package depends only on the ones above it. Every file has its own
`_test.go` sibling.

## Examples

- `example_test.go` — runnable godoc examples: build, `Repair`, `Save`/`Load`,
  `Render`, `Extract`, `ExportCIM`/`ImportCIM`.
- `examples/xsld2svg` — render an `.xsld` to SVG:
  `go run ./examples/xsld2svg -in examples/testdata/feeder.xsld -symbols examples/testdata/symbols.xml -out feeder.svg`
- `examples/svg2xsld` — extract an `.xsld` from an SVG:
  `go run ./examples/svg2xsld -in feeder.svg -out feeder.xsld`

Both are trimmed versions of sld-svg's `svg-sld render`/`extract`.

## Development

Both consuming repos build against a local checkout of this module via a
`go.mod` `replace` directive (a sibling `../slddoc` checkout), rather than
a tagged release, while the three are developed together.

```
go build ./...
go vet ./...
go test ./...
```

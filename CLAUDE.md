# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository. It is an
index plus what the code can't tell you; read the referenced files and their doc comments for how things work.
History lives in RELEASE.md.

## Common requirements

- Before implementing anything, always first briefly describe what you'll be doing and ask for my consent.
- After implementation, write down in RELEASE.md file what was implemented in the format "Date: Implementation
  Description" (and in the consuming repo's RELEASE.md when the change is made for it, e.g. sld-editor's)
- Use Go 1.26
- Don't use a lot of code comments. For function describe arguments and results. The best comments is a code.
- use go fix ./...
- Do not use actual diagram names in comments or in files saved in Git

## What this is

`github.com/PVKonovalov/slddoc`: a shared Go library (no application; one public package, `slddoc`, over
internal packages) modeling a single-line diagram — elements, coordinates, electrical topology — as an `.xsld` XML
document, with SVG rendering, extraction from xsde2svg SVGs, topology repair, and CIM (CGMES 3.0 EQ + DL)
export/import with automatic layout.

## Sibling repositories

All under `~/repo/PVKonovalov/`; consumers build against this checkout via a `go.mod` `replace` (`../slddoc` or
`../../slddoc`), not a tagged release, so **a change here is live in every consumer at once — build and test them too.**

| Repo | Relationship |
|---|---|
| `sld-editor` | Main consumer (`backend/go.mod` replace). Owns this library's extensions (JSON tags, `Editor` settings, label id/color/font, `RenderMode.Interactive`, `History`, CIM). Its `frontend/src/lib/diagramOps.ts` fixed-ports/topology functions are what `Repair` ports **rule for rule — keep the two in step**. Its RELEASE.md describes most changes made here. |
| `sld-svg` | Consumer (`go.mod` replace) and owner of `Extract`'s behavior; the reference for corpus-faithful output (`svg-sld` CLI). |
| `EMA/xsde2svg` | Produces the SVGs `Extract` reads; `RenderMode.Static` must stay faithful to its output. |
| `OrxaGridMicroservices/rdss-dms` | Final consumer of rendered SVGs (via sld-editor). Its `assets/sld/*.svg` are the real xsde2svg corpus to verify extraction/rendering against. |
| SOPTIM `CIMDiagramLayoutEditor`, PowSyBl | CIM interop checks (sld-editor's `docs/CIM_INTEROP.md`, `tools/cim-tools`). |

## Commands

```
go build ./... && go vet ./... && go test ./...
go test ./... -run TestName
go fix ./...
```

## Index

- `XSLD_FORMAT.md` — the format reference (ids, sections, topology, class attributes, code tables, legacy rewrites).
  Update it with any model/format change. `README.md`, `doc.go` — overview.
- `slddoc.go` — the public API: aliases and wrappers re-exporting the internal packages. Add a new exported name
  there too. Imports only go one way: model, svgtree ← render ← extract, repair ← cim.
- `internal/model/` — `model.go` (types, XML `Load`/`Save`), `ids.go`, `style.go`, `geom.go`, `points.go`,
  `voltage.go`, `sizestep.go`, `electrical.go`.
- `internal/svgtree/` — `rawtree.go`, the raw SVG tree.
- `internal/render/` — `render.go` (`Render`, `RenderFragments`, `RenderMode`), `absolute.go` (Static absolute
  coordinates), `symbols.go` (`SymbolLibrary`), `leads.go`, `layers.go` (layer metadata).
- `internal/extract/` — `extract.go` (+ `elements.go`, `topology.go`, `layers.go`, `labels.go`, `leads.go`, `tap.go`)
  — SVG → Diagram.
- `internal/repair/` — `repair.go`: `Repair`, `EnsureIDs`, `InferSizeSteps`.
- `internal/cim/` — `cim.go` (export, `NewCIMClassTable`), `cimimport.go`, `cimforeign.go` (another tool's DL),
  `cimheader.go`, `cimwindings.go`, `cimlayout.go` + `cimlayoutopt.go` + `cimroute.go` (automatic layout and lead
  routing).
- Every file has a `_test.go` sibling; corpus-like cases are named after the device (`substation_test.go`, ...). A
  test that needs a higher package (e.g. a render test that extracts) lives in that package.
- `example_test.go` — godoc examples; `examples/` — runnable `xsld2svg`/`svg2xsld` with synthetic `testdata/`.

## Rules not derivable from code

- Byte-identical round-tripping of a source SVG is not a goal: `Extract` builds a fresh model, `Render` redraws it.
- Ids are plain ints in one shared space; 0 = unset. This library never assigns ids in an editing flow (sld-editor's
  frontend does); `EnsureIDs` only fills missing ones.
- `Load` rewrites legacy values (connector `ObjectLink` → BusWork, shape `71` → `162`); drop the 71→162 rewrite once
  shape 71 is ported as its own type.
- `Repair` never moves an existing node (an imported diagram may use one node for a whole busbar).
- `RenderMode.Static` (saved `.svg`) is xsde2svg-faithful: absolute coordinates, `rotate(a,cx,cy)`. `Interactive` is
  for sld-editor's canvas only. Connector kinds render as type codes 21/22/23/28. The `data-*` attributes are read by
  rdss-dms — changing them is an API change there.
- CIM: export must keep importing cleanly into PowSyBl and round-tripping through our own import; anything not known
  to be true is left out rather than guessed.

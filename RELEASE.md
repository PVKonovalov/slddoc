# Release notes

Earlier history is in the commit messages and in sld-editor's RELEASE.md.

2026-10-27: Added `CLAUDE.md` (an index plus the rules the code can't show, with a sibling-repository table:
sld-editor, sld-svg, xsde2svg, rdss-dms, CIM interop tools) and this `RELEASE.md`.

2026-10-27: Auto-layout transformer leads keep two rules (`transformers()` and `leadClear` in `cimlayout.go`). First, a lead never crosses or runs along its own node's wires, or itself. Second, it passes over no symbol, and reaches its transformer only in its last segment, from outside. The straight and one-jog shortcuts now take that check, and a lead coming from the far side goes around. `route` treats the lead's own node's wires as solid, and `symbolBox` makes a transformer's box across its axis half its terminal span plus half the reach. A lead with no clear route is drawn anyway and noted in `CIMPage.Warnings` (`cimImporter.layoutWarnings`). Test: `TestAutoLayoutTransformerLeads`.

2026-10-27: Transformer winding voltages (`cimwindings.go`). `CIMSameVoltageTransformers(docs)` lists the transformers whose windings (two or more) all have one voltage, giving each end's mRID, end number, name, kV and `ratedU`. `CIMImportOptions.WindingVoltages` (end mRID → kV) is applied before anything is drawn. The end takes a BaseVoltage of that voltage (the model's own, else a new one). Everything joined to it without passing a transformer moves to a VoltageLevel of that voltage in the same substation, and a line with its own BaseVoltage takes the new one. A side that reaches another winding of the same transformer keeps its voltage, with a warning on the first page. `ImportCIM`'s parsing and indexing moved to `newCIMImporter`. Test: `TestCIMWindingVoltages`.

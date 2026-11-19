package extract

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/PVKonovalov/slddoc/internal/model"
	"github.com/PVKonovalov/slddoc/internal/render"
)

// table2XsdeSVG is a 3x3 Table2 as xsde2svg draws it: cell (0,2) spans two rows, (2,0) spans two columns, and the
// cells use every alignment, a bigger bold font, italic, underline and several lines.
const table2XsdeSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="600" height="400">
<g id="7" data-type="313" >
<path d="M 100 100 h 110 v 50 h -110 v -50 " style="fill:lightyellow;stroke:red;stroke-width:1 " data-voltage="red" />
<text x="102" y="111" style="fill:gray;text-anchor:start;dominant-baseline:middle;font-size:11px;font-family:Arial " >A</text>
<path d="M 100 150 h 110 v 50 h -110 v -50 " style="fill:lightcyan;stroke:red;stroke-width:1 " data-voltage="red" />
<text x="155" y="175" style="fill:black;text-anchor:middle;dominant-baseline:middle;font-size:14px;font-family:Arial ;font-weight: bold" >B</text>
<path d="M 100 200 h 220 v 50 h -220 v -50 " style="fill:lightcyan;stroke:red;stroke-width:1 " data-voltage="red" />
<text x="318" y="239" style="fill:darkred;text-anchor:end;dominant-baseline:middle;font-size:11px;font-family:Arial ;text-decoration: underline;font-style:italic" >C</text>
<path d="M 210 100 h 110 v 50 h -110 v -50 " style="fill:lightcyan;stroke:red;stroke-width:1 " data-voltage="red" />
<path d="M 210 150 h 110 v 50 h -110 v -50 " style="fill:lightcyan;stroke:red;stroke-width:1 " data-voltage="red" />
<text x="265" y="161" style="fill:navy;text-anchor:middle;dominant-baseline:middle;font-size:11px;font-family:Arial " >D<tspan style="fill:navy;text-anchor:middle;dominant-baseline:middle;font-size:11px;font-family:Arial " x="265" dy="25" > </tspan></text>
<path d="M 320 100 h 110 v 100 h -110 v -100 " style="fill:lightcyan;stroke:red;stroke-width:1 " data-voltage="red" />
<text x="322" y="111" style="fill:navy;text-anchor:start;dominant-baseline:middle;font-size:11px;font-family:Arial " >E<tspan style="fill:navy;text-anchor:start;dominant-baseline:middle;font-size:11px;font-family:Arial " x="322" dy="50" >F</tspan></text>
<path d="M 320 200 h 110 v 50 h -110 v -50 " style="fill:lightcyan;stroke:red;stroke-width:1 " data-voltage="red" />
</g>
</svg>`

var table2XsdeCells = []model.TableCell{
	{Row: 0, Col: 0, Text: "A", Fill: "lightyellow", TextColor: "gray", AlignH: model.AlignLeft, AlignV: model.AlignTop},
	{Row: 1, Col: 0, Text: "B", FontSize: 14, Bold: true},
	{Row: 2, Col: 0, ColSpan: 2, Text: "C", TextColor: "darkred", Italic: true, Underline: true, AlignH: model.AlignRight, AlignV: model.AlignBottom},
	{Row: 0, Col: 1},
	{Row: 1, Col: 1, Text: "D\n", TextColor: "navy"},
	{Row: 0, Col: 2, RowSpan: 2, Text: "E\nF", TextColor: "navy", AlignH: model.AlignLeft},
	{Row: 2, Col: 2},
}

// TestExtractTable2Cells reads merged cells, text lines, font and alignment from xsde2svg's Table2 markup.
func TestExtractTable2Cells(t *testing.T) {
	d, report, err := Extract([]byte(table2XsdeSVG), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Failed) != 0 || len(d.Elements) != 1 {
		t.Fatalf("report = %+v, elements = %+v", report, d.Elements)
	}
	el := d.Elements[0]
	if !reflect.DeepEqual(el.RowHeights, []float64{50, 50, 50}) || !reflect.DeepEqual(el.ColumnWidths, []float64{110, 110, 110}) {
		t.Errorf("grid = %v x %v", el.RowHeights, el.ColumnWidths)
	}
	if !reflect.DeepEqual(el.Cells, table2XsdeCells) {
		t.Errorf("cells =\n%+v\nwant\n%+v", el.Cells, table2XsdeCells)
	}
}

// TestRender_Table2RoundTrip checks that a Table2 renders its cells where xsde2svg puts them and imports back the
// same.
func TestRender_Table2RoundTrip(t *testing.T) {
	d, _, err := Extract([]byte(table2XsdeSVG), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := render.Render(d, render.NewSymbolLibrary(map[string]string{}), &buf, render.Static, "", nil); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<path d="M 320 100 h 110 v 100 h -110 v -100 "`,
		`<text x="318" y="239" style="fill:darkred;text-anchor:end;dominant-baseline:middle;font-size:11px;font-family:Arial;text-decoration: underline;font-style:italic">C</text>`,
		`>E<tspan style="fill:navy;text-anchor:start;dominant-baseline:middle;font-size:11px;font-family:Arial" x="322" dy="50">F</tspan></text>`,
		`font-size:14px;font-family:Arial;font-weight: bold">B</text>`,
	} {
		if !bytes.Contains(buf.Bytes(), []byte(want)) {
			t.Errorf("missing %s in\n%s", want, buf.String())
		}
	}
	back, _, err := Extract(buf.Bytes(), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Elements) != 1 || !reflect.DeepEqual(back.Elements[0].Cells, table2XsdeCells) {
		t.Errorf("round trip cells = %+v", back.Elements)
	}
}

// TestTable2CellsXSLD checks that the new cell attributes, a line break included, survive .xsld save and load.
func TestTable2CellsXSLD(t *testing.T) {
	d, _, err := Extract([]byte(table2XsdeSVG), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := d.Save(&buf); err != nil {
		t.Fatal(err)
	}
	back, err := model.Load(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(back.Elements[0].Cells, table2XsdeCells) {
		t.Errorf("cells after save/load = %+v", back.Elements[0].Cells)
	}
}

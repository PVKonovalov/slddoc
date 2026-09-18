package slddoc

import "testing"

const testDiagramSVG = `<?xml version="1.0"?>
<svg width="2000" height="1000" style='stroke-width: 0px; background-color: #12161d;' xmlns="http://www.w3.org/2000/svg">
<metadata>
{"layers":[{"id":"10","label":"Контейнеры"}]}
</metadata>
<polyline points="810,240 1320,240" style="fill:none;stroke:#326400;stroke-width:2" data-voltage="#326400" data-name="СШ 6 кВ" data-type="24" id="1" />
<circle cx="900" cy="240" r="3" style="fill:#12161d;stroke:#326400;stroke-width:1" data-type="7" id="2" data-voltage="#12161d" />
<polyline points="900,240 900,280" style="fill:none;stroke:#326400;stroke-width:1" data-type="21" id="3" data-voltage="#326400" />
<g id="4" data-voltage="#326400" data-type="41" data-name="Breaker 1" >
<g>
<path d="M 893 283 h 14 v 14 h -14 z" style="fill:lawngreen;stroke:#326400;stroke-width:1" />
<path d="M 900 285 v 10" data-state="1" style="stroke:#326400;stroke-width:1" />
<path d="M 900 280 v 3 M 900 300 v -3" style="stroke:#326400;stroke-width:1" />
</g>
</g>
<g data-type="5" data-name="Breaker 1" data-event="dc" id="2674">
<text x="910" y="293" style="fill:white;text-anchor:start;font-size:13px;font-family:Arial;white-space: pre;" >Breaker 1</text>
</g>
<text x="1876" y="759" style="fill:darkturquoise;text-anchor:end;dominant-baseline:middle;font-size:16px;font-family:Arial ;font-weight: bold" data-type="134" id="148704875" data-name="R T-1 10" data-unit="МВт" >0.00 <tspan style="fill:darkturquoise;text-anchor:end;dominant-baseline:middle;font-size:16px;font-family:Arial ;font-weight: bold" >MW</tspan></text>
<polyline points="0,0 5,0" style="fill:none;stroke:black;stroke-width:1" data-type="1" id="border" />
</svg>
`

func TestExtract_EndToEnd(t *testing.T) {
	d, report, err := Extract([]byte(testDiagramSVG), "test.svg", map[string]string{"#326400": "6кВ"})
	if err != nil {
		t.Fatal(err)
	}

	if len(d.Layers) != 2 || d.Layers[1].ID != 10 {
		t.Errorf("layers = %+v", d.Layers)
	}
	if len(d.VoltageClasses) != 1 || d.VoltageClasses[0].Name != "6кВ" {
		t.Errorf("voltage classes = %+v", d.VoltageClasses)
	}
	if report.Elements != 3 {
		t.Errorf("report.Elements = %d, want 3 (bus, point, breaker)", report.Elements)
	}
	if report.Connectors != 1 {
		t.Errorf("report.Connectors = %d, want 1", report.Connectors)
	}
	if report.Labels != 1 {
		t.Errorf("report.Labels = %d, want 1", report.Labels)
	}
	if report.Nodes != 3 {
		t.Errorf("report.Nodes = %d, want 3 (bus+point+wire-start merged, wire-end+port1 merged, port2 alone)", report.Nodes)
	}

	var breaker Element
	for _, e := range d.Elements {
		if e.Class == ClassBreaker {
			breaker = e
		}
	}
	if breaker.ID == 0 {
		t.Fatalf("no breaker in %+v", d.Elements)
	}
	if breaker.Ports[0].Node == breaker.Ports[1].Node {
		t.Errorf("breaker's two ports must resolve to different nodes: %+v", breaker.Ports)
	}

	if len(d.Labels) != 1 || d.Labels[0].For != breaker.ID {
		t.Errorf("label should match the breaker by name: %+v", d.Labels)
	}
	if d.Labels[0].ID != 2674 {
		t.Errorf("label should keep its own real source id, got %+v", d.Labels[0])
	}
	if d.Labels[0].Color != "white" || d.Labels[0].Font != "Arial" || d.Labels[0].VAlign != "" {
		t.Errorf("label should keep its own real fill/font/baseline, got %+v", d.Labels[0])
	}

	if report.DigitalDevices != 1 || len(d.DigitalDevices) != 1 {
		t.Errorf("report.DigitalDevices = %d, len(d.DigitalDevices) = %d, want 1 each", report.DigitalDevices, len(d.DigitalDevices))
	} else {
		dd := d.DigitalDevices[0]
		if dd.ID != 148704875 || dd.Name != "R T-1 10" || dd.Value != "0.00" || dd.Unit != "МВт" || dd.Anchor != "end" || !dd.Bold {
			t.Errorf("digital device = %+v", dd)
		}
		// A real digital device's own per-instance color/baseline matter a
		// lot (status indication) — dropping these silently defaulted every
		// extracted one to white/bottom, which is what surfaced this bug.
		if dd.Color != "darkturquoise" || dd.Font != "Arial" || dd.VAlign != "middle" {
			t.Errorf("digital device should keep its own real fill/font/baseline, got %+v", dd)
		}
	}

	// The decorative border (data-type="1", a plain line) is a recognized
	// SVG shape but not one v1 understands electrically; it must be
	// reported, not silently absorbed into an element.
	if report.Skipped["1"] != 1 {
		t.Errorf("expected the border polyline (data-type=1) to be skipped and counted, got %+v", report.Skipped)
	}
}

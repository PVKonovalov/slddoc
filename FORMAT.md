# slddoc XML format

This document describes the XML format of a single-line diagram (SLD) as read by
`slddoc.Load` and written by `(*Diagram).Save`. The authoritative definition is
the `xml` struct tags in [`model.go`](model.go); this document explains them.

## 1. Overview

A `.xml` diagram file is the **source of truth** for one SLD. It records:

- what equipment is on the diagram, where, and how it is drawn;
- the **electrical topology** — which terminals are connected to which;
- voltage classes, visibility layers, text labels and readouts.

The SVG is always *derived* from it (`slddoc.Render`), never the other way
round — except for `slddoc.Extract`, which reconstructs a diagram from an
existing xsde2svg-generated SVG as a one-time import.

| Producer / consumer | Role |
|---|---|
| `slddoc.Extract` (sld-svg `svg-sld extract`, sld-editor `.svg` import) | writes diagrams reconstructed from xsde2svg SVGs |
| sld-editor | reads and writes diagrams interactively |
| `slddoc.Render` (sld-svg `svg-sld render`, sld-editor) | reads a diagram, produces an xsde2svg-compatible SVG |

Diagrams produced by `Extract` leave every editor-only field (e.g. `<editor>`,
label `id`s on old files) at its zero value; readers must treat every optional
field as possibly absent.

### General conventions

- Encoding is UTF-8 with a standard `<?xml version="1.0" encoding="UTF-8"?>` header.
- **Coordinates** are floating-point numbers in diagram units (the same units as
  the SVG's user space). X grows to the right, Y grows **down**.
- **Booleans** are written as `true`; `false` is written by omitting the attribute.
- **Optional attributes** are omitted when they hold their zero value (`0`, `""`,
  `false`). An absent attribute and its zero value mean the same thing.
- **Colors** are free-text CSS/SVG colors: a name (`red`, `lawngreen`), `#rrggbb`,
  or `none`.
- Empty collections are omitted entirely (no empty `<labels/>` wrapper).
- Unknown elements and attributes are ignored by `Load`.

## 2. Ids

Every `Layer`, `VoltageClass`, `Node`, `Element`, `Connector`, `Label` and
`DigitalDevice` carries an `id`:

- an id is a **plain positive integer**, never a string;
- there is **one shared id space** for elements, connectors, labels and digital
  devices — no two of them share an id (the SVG uses the same bare integer as
  its DOM `id`);
- `0` means **"unset"** in every field that *references* an id (`voltage`, `for`,
  `defaultVoltage`, ...), since a real id is never 0. The one exception is
  layer `0`, which is the real, always-present base layer (see §4.1).

`<diagram lastId="…">` is the high-water mark of the editor's autoincrement
counter: the next new object gets `lastId + 1`. A diagram without `lastId` gets
one computed (the highest existing id) when the editor opens it.

The element `shape` attribute is **not** an id — it is a symbol-library key
(an xsde2svg ObjectType code such as `"41"`) and stays a string.

## 3. Document structure

```xml
<?xml version="1.0" encoding="UTF-8"?>
<diagram width="1800" height="1200" source="PS_110kV.svg" lastId="42">
  <editor gridSpacing="10" snap="true" showGrid="true" background="#12161d"
          defaultVoltage="1" showNodes="true"/>
  <layers>          <layer .../>          ... </layers>
  <voltageClasses>  <class .../>          ... </voltageClasses>
  <nodes>           <node .../>           ... </nodes>
  <elements>        <element ...>...      ... </elements>
  <connectors>      <connector ...>...    ... </connectors>
  <labels>          <label ...>text       ... </labels>
  <digitalDevices>  <digitalDevice .../>  ... </digitalDevices>
</diagram>
```

Child sections always appear in this order when written; each one is optional.

### 3.1 `<diagram>`

| Attribute | Type | Req. | Meaning |
|---|---|---|---|
| `width` | number | yes | Canvas width in diagram units |
| `height` | number | yes | Canvas height in diagram units |
| `source` | string | no | File the diagram was extracted from (e.g. `PS_110kV.svg`); informational |
| `lastId` | int | no | Id high-water mark (see §2) |

### 3.2 `<editor>` (optional)

The editor's per-diagram preferences. Only `background` affects rendering (the
SVG background color); everything else is editor UI state. Absent for diagrams
never saved by sld-editor — readers fall back to their own defaults.

| Attribute | Type | Meaning |
|---|---|---|
| `gridSpacing` | number | Grid step in diagram units |
| `snap` | bool | Snap placement/drag to the grid |
| `showGrid` | bool | Draw the grid overlay |
| `background` | color | Canvas background color |
| `defaultVoltage` | id → `class` | Voltage class newly placed objects start with (0 = none) |
| `showNodes` | bool | Draw a debug marker at every `<node>` |

## 4. Sections

### 4.1 `<layers>` / `<layer>`

Visibility layers — a viewer can toggle everything on one layer at once.

| Attribute | Type | Req. | Meaning |
|---|---|---|---|
| `id` | int | yes | Layer id. `0` is the base layer |
| `name` | string | yes | Display name |

Every element, connector, label and digital device carries exactly one `layer`
reference (default `0`).

### 4.2 `<voltageClasses>` / `<class>`

A logical voltage level and the color it is drawn with. Voltage is a property of
an element/connector; **color is recorded only here**.

| Attribute | Type | Req. | Meaning |
|---|---|---|---|
| `id` | int | yes | Class id |
| `name` | string | yes | Display name, e.g. `10 kV`. Extract uses the color itself as the name when it has no voltage hint for it |
| `color` | color | yes | Drawing color |

### 4.3 `<nodes>` / `<node>`

An **electrical junction**. Every element port and connector end that references
the same node id is electrically connected — this is the whole topology model.

| Attribute | Type | Req. | Meaning |
|---|---|---|---|
| `id` | int | yes | Node id |
| `x`, `y` | number | yes | Position of the junction (informational: used for display/editing, not for connectivity) |

### 4.4 `<elements>` / `<element>`

One placed object: electrical equipment (breaker, transformer, busbar, ...) or a
decorative shape (rectangle, line, table, ...).

**Common attributes**

| Attribute | Type | Req. | Meaning |
|---|---|---|---|
| `id` | int | yes | Element id |
| `class` | string | yes | Equipment kind (see §7.1), independent of how it is drawn |
| `shape` | string | yes | Symbol-library key / xsde2svg ObjectType code (see §7.1) |
| `name` | string | no | Dispatch name, e.g. `В-10 Л-11`. Rendered as `data-name` |
| `voltage` | id → `class` | no | Voltage class (0 = unassigned) |
| `layer` | id → `layer` | yes | Layer (default 0) |
| `x`, `y` | number | yes | Anchor: the symbol's rotation center (for points-based shapes, informational — the midpoint of its geometry) |
| `orient` | int | no | Rotation around the anchor, in degrees, clockwise (SVG `rotate`). Multiples of 90; negative values and ±270 occur in extracted files |
| `mirror` | bool | no | Flip the symbol horizontally in its local frame, before rotation. Editor-only; Extract never sets it |

**Children**

```xml
<element ...>
  <port name="1" node="7"/>          <!-- zero or more -->
  <geometry><point x=".." y=".."/>...</geometry>   <!-- points-based shapes only -->
  <windings>...</windings>           <!-- PowerTransformer only -->
  <rows>/<columns>/<cells>           <!-- Table2 only -->
</element>
```

**`<port>`** — one electrical terminal.

| Attribute | Type | Meaning |
|---|---|---|
| `name` | string | Terminal name: `"1"`, `"2"`, ... in order |
| `node` | id → `node` | The node this terminal is connected to |

Decorative shapes (Rectangle, Arrow, Circle, Line, Road, Button, PostPole,
Table, Table2, PowerflowIndicator) never have ports. See §5 for how ports
form the topology and §6 for per-class attributes.

### 4.5 `<connectors>` / `<connector>`

A drawn wire: a polyline whose two ends are nodes.

| Attribute | Type | Req. | Meaning |
|---|---|---|---|
| `id` | int | yes | Connector id |
| `kind` | string | yes | Wire kind (see §7.2) |
| `name` | string | no | Line name, e.g. `Line2`. Rendered as `data-name` for `OverheadLine`/`CableLine` only |
| `voltage` | id → `class` | no | Voltage class |
| `layer` | id → `layer` | yes | Layer |
| `dashed` | bool | no | Draw dashed |
| `lineStyle` | enum | no | `CableLine` only: `solid`, `dashed` (default), `dashDot`, `dotted` |
| `from` | id → `node` | yes | Node at the first point |
| `to` | id → `node` | yes | Node at the last point |

Children: two or more `<point x=".." y=".."/>`, **directly** inside
`<connector>` (no `<geometry>` wrapper), in drawing order. The first point
coincides with node `from`, the last with node `to`. Editor-drawn wires are
orthogonal (horizontal/vertical segments only), but that is not a format rule.

### 4.6 `<labels>` / `<label>`

A standalone text caption. The text is the element's character content; a
newline (`&#xA;` or a literal line break) starts a new line.

| Attribute | Type | Req. | Meaning |
|---|---|---|---|
| `id` | int | yes | Label id. Older files may have `0` on every label; the editor assigns fresh ids on open |
| `for` | id → `element` | no | Element this label annotates. Informational only — it is not a live link and the label does not follow the element |
| `layer` | id → `layer` | yes | Layer |
| `x`, `y` | number | yes | Text anchor point |
| `size` | number | yes | Font size in diagram units |
| `anchor` | enum | no | Horizontal anchor: `start` (default), `middle`, `end` |
| `valign` | enum | no | Vertical anchor: `top`, `middle`; absent = text baseline at `y` |
| `bold` | bool | no | Bold text |
| `color` | color | no | Text color (default `white`) |
| `font` | string | no | Font family (default `Arial`) |

```xml
<label id="2674" for="4500" layer="0" x="1110" y="223" size="13" anchor="start">1 сш 10 кВ</label>
```

### 4.7 `<digitalDevices>` / `<digitalDevice>`

A SCADA analog readout (xsde2svg shape 134, digital instrument). Has the same
text-style attributes as `<label>` (`id`, `layer`, `x`, `y`, `size`, `anchor`,
`valign`, `bold`, `color`, `font`), plus:

| Attribute | Type | Req. | Meaning |
|---|---|---|---|
| `name` | string | no | SCADA tag/point name (rendered as `data-name`, not displayed) |
| `value` | string | yes | Placeholder value shown in place of a live reading, e.g. `0.00` |
| `unit` | string | no | Unit suffix, e.g. `MW`, drawn right after the value |

## 5. Topology

The electrical graph is expressed **only through node ids**:

```
 element port ──node──┐
 connector from/to ───┤  same node id  ⇒  electrically connected
 element port ──node──┘
```

- An element terminal is a `<port node="N">`.
- A connector joins its `from` node to its `to` node.
- A **junction** (a T-tap into a wire) is a single node referenced by three or
  more connector ends. The editor splits the tapped wire into two connectors
  at the tap so that the junction is always at a connector end, never mid-span.
- An **unconnected wire end** is a node that no port references and only one
  connector end uses. This is legal (a wire left to be finished later).
- **Busbars** (`BusBarSection`) are one electrical node along their whole
  length:
  - in editor-drawn diagrams, each wire attached to a busbar adds a
    `<port name="k" node="N"/>` to the busbar (k = 1, 2, ...);
  - in `Extract` output, a busbar has **no ports**; every wire end lying on the
    busbar's geometry is merged into one shared node whose position is the
    busbar's first point.
  A consumer that builds a network model should therefore treat a busbar as
  connected to (a) every node in its own ports and (b) every node that lies on
  its geometry.
- Switching devices are **not** open in the topology: a breaker in state `0`
  still has both ports connected to their nodes. Whether current flows is
  determined by the device's `state`, not by the graph.
- Node `x`/`y` and connector points are geometry only; connectivity is never
  inferred from coordinates when loading a saved file (only `Extract` does
  that, once, when building the file).

## 6. Class-specific attributes

These attributes live on `<element>` and are meaningful only for the listed
classes; elsewhere they are ignored.

### 6.1 Switching devices — `state`, `position`

| Attribute | Classes | Values |
|---|---|---|
| `state` | Breaker, Disconnector, LoadBreakSwitch, GroundSwitch, Sectionalizer, ShortCircuiter | `0` Open, `1` Closed, `2` Intermediate (Sectionalizer/ShortCircuiter: 0/1 only). Absent = unknown |
| `state` | FaultPassageIndicator | `0` not triggered, `1` triggered |
| `state` | Lamp | `0` off, `1` on |
| `state` | PowerflowIndicator | `0`/absent draws `→`, any other value draws `←` |
| `position` | withdrawable Breaker (43), withdrawable Disconnector (49) | `0` Service, `1` Normal, `2` Test — the racking position, independent of `state` |

The state → color legend (`0:red, 1:lawngreen, 2:yellow` for switching devices)
is a renderer setting, not stored in the diagram.

### 6.2 Points-based shapes — `<geometry>`

These shapes are drawn from their own `<geometry><point/>...</geometry>`
instead of a symbol template around `x`/`y`:

| Class (shape) | Points |
|---|---|
| BusBarSection (24) | 2+ vertices, polyline |
| Line (1), Road (335) | 2+ vertices, polyline |
| Rectangle (3), Circle (4), Button (113), Table (312) | 2 opposite corners of the bounding box, any order |
| Arrow (2) | start, end — order matters, the arrowhead is at the second point |

### 6.3 Decorative styling

| Attribute | Type | Used by | Meaning |
|---|---|---|---|
| `fill` | color | Rectangle, Circle, JunctionPoint, PostPole, PackageSubstation (inner box), Table, Table2 (default cell fill) | Interior color; empty = `none` |
| `stroke` | color | Rectangle, Circle, Arrow, Line, Road, Button, PostPole, Table, Table2 | Border / line color |
| `strokeWidth` | number | Rectangle, Circle, Arrow, Line, Road, Button, Table, Table2 | Line width; 0 = default (1, or a thick default for Road) |
| `lineStyle` | enum | Line, Table, Table2 | `solid` (default), `dashed`, `dashDot` |
| `doubleHeaded` | bool | Arrow | Arrowhead at both ends |
| `square` | bool | PostPole | Square marker instead of round |
| `radius` | number | Lamp, FaultPassageIndicator, JunctionPoint (default 3), PostPole | Circle radius (PostPole square: half-width) |
| `propertyText` | string | PackageSubstation, EnclosedSubstation (e.g. rating `160`), FaultPassageIndicator (default `FPI`), Button, Table | Centered overlay text |
| `textColor` | color | Button (default white), PowerflowIndicator, Table (default black) | Text / glyph color |
| `bold` | bool | Button | Bold overlay text |
| `fillOff`, `fillOn` | color | Lamp | Colors for `state` 0 / 1 |
| `nType` | int | PackageSubstation | Appearance: `0` box-in-box (default), `1` triangle |
| `orient` | int | Table | Rotates only the overlay text, not the box |

### 6.4 PowerTransformer (47) — `<windings>`

```xml
<element id="20" class="PowerTransformer" shape="47" x="1560" y="890"
         autotransformer="true" vectorGroupLabel="Yn/Δ-11">
  <port name="1" node="166"/>
  <port name="2" node="163"/>
  <windings>
    <winding voltage="1" scheme="wyeN" grounding="solid" terminal="top"/>
    <winding voltage="2" scheme="delta" tapChanger="true"/>
  </windings>
</element>
```

- 2 to 4 `<winding>`s in order HV, MV, LV1, LV2. Winding *i* uses port *i*.
- `autotransformer` (bool), `vectorGroupLabel` (free text, shown when non-empty).

| `<winding>` attribute | Values |
|---|---|
| `voltage` | id → `class`; each winding has its own voltage |
| `scheme` | `wye`, `wyeN` (wye with neutral), `delta`; absent = no glyph |
| `grounding` | `wyeN` only: `solid`, `isolated`, `resistor` |
| `tapChanger` | bool — regulated winding (only one arrow is drawn; the last wins) |
| `terminal` | Side of the lead in the local frame: `top`, `bottom`, `left`, `right`; absent = default for the winding count |

### 6.5 Table2 (313) — grid

`x`/`y` is the top-left corner; no `<geometry>`.

```xml
<element id="30" class="Table2" shape="313" layer="0" x="100" y="100" stroke="black">
  <rows><row>20</row><row>20</row></rows>
  <columns><column>80</column><column>40</column></columns>
  <cells>
    <cell row="0" col="0" text="P, MW"/>
    <cell row="0" col="1" text="12.5" fill="yellow" textColor="red"/>
  </cells>
</element>
```

- `<row>` / `<column>` hold each row height / column width.
- `<cell>`: `row`, `col` (0-based), `text` (single line), optional `fill` and
  `textColor` overrides. A grid position with no `<cell>` is not drawn.
- Cell merging is not modeled.

## 7. Code tables

### 7.1 Element classes and shapes

`shape` is the xsde2svg ObjectType code. The shapes available for rendering
come from the element library (`sld-editor/backend/assets/elements/base.xml`,
extendable per site); this is the default set.

| Class | Shape(s) | Ports | Notes |
|---|---|---|---|
| Breaker | 41, 43 (withdrawable) | 2 | `state`; 43 also `position` |
| Disconnector | 162, 49 (withdrawable) | 2 | `state`; 49 also `position` |
| LoadBreakSwitch | 42 | 2 | `state` |
| Sectionalizer | 164 | 2 | `state` (0/1) |
| GroundSwitch | 54 | 1 | `state` |
| ShortCircuiter | 398 | 1 | `state` (0/1) |
| Fuse | 203, 154 (withdrawable) | 2 | |
| Chassis / HalfChassis | 51 / 52 | 2 / 1 | |
| Starter | 76 | 2 | |
| PowerTransformer | 47 | 2–4 | `<windings>` (§6.4) |
| CurrentTransformer | 34 | 2 | |
| VoltageTransformer | 55 | 1 | |
| ChokeCoil / Reactor / ReactorShunt | 33 / 37 / 397 | 2 / 2 / 1 | |
| SurgeArrester | 35, 29, 168 (grounded) | 1–2 | |
| Capacitor / CapacitorBank | 388 / 172 | 2 / 1 | |
| Generator | 173 | 1 | |
| Ground | 31 | 1 | |
| CableConnector / CableJoint | 56 / 32 | 2 | |
| JunctionPoint | 7 | 1 | `radius`, `fill` |
| NonIntersection (wire jump) | 14 | 2 | |
| BusBarSection | 24 | 0+ | `<geometry>`; see §5 |
| PackageSubstation / EnclosedSubstation | 385 / 386 | 1 | `propertyText`; 385 also `nType`, `fill` |
| Lamp | 106 | 0 | `state`, `fillOff`, `fillOn`, `radius`; no voltage |
| FaultPassageIndicator | 320003 | 0 | `state`, `radius`, `propertyText` |
| PowerflowIndicator | 320001 | — | decorative; `state` = direction, `textColor` |
| Line / Road | 1 / 335 | — | decorative; `<geometry>` |
| Rectangle / Circle / Arrow | 3 / 4 / 2 | — | decorative; `<geometry>` |
| Button | 113 | — | decorative; `<geometry>`, `propertyText` |
| PostPole | 292 | — | decorative; `radius`, `square` |
| Table / Table2 | 312 / 313 | — | decorative; §6.2, §6.5 |

The "Ports" column is the usual count (measured over the existing corpus); the format itself does not enforce it.

### 7.2 Connector kinds

| `kind` | xsde2svg code | Meaning / rendering |
|---|---|---|
| `BusWork` | 21 | Buswork / generic wire between equipment (default) |
| `OverheadLine` | 22 | Overhead line; `<g>`-wrapped with `data-name`. In the editor it can only be tapped at its two ends |
| `CableLine` | 23 | Cable line; `<g>`-wrapped with `data-name`, dash pattern from `lineStyle` |
| `LinkToObject` | 28 | Object link; heavier line with an arrowhead at the `to` end |

Labels (`<label>`) are xsde2svg code 5 and digital devices code 134; they have
their own sections rather than being elements.

## 8. Legacy values rewritten on load

`Load` silently upgrades a few values older files may still contain; the new
value is written on the next save.

| Where | Old value | New value |
|---|---|---|
| `connector/@kind` | `ObjectLink` | `BusWork` |
| `connector/@kind` | `BusbarWire` (an old Extract bug for code 21) | `BusWork` |
| `element/@shape` | `71` (duplicate Disconnector) | `162` |

## 9. Complete example

Two busbar sections joined by a disconnector and a breaker, with an overhead
line leaving the second bus and a label on the breaker. The Breaker (41) and
Disconnector (162) symbols have their terminals at local `(0,-10)` / `(0,10)`,
so an unrotated one at `y` has terminals at `y-10` and `y+10`.

```xml
<?xml version="1.0" encoding="UTF-8"?>
<diagram width="400" height="300" lastId="16">
  <editor gridSpacing="10" snap="true" showGrid="true" background="#12161d" defaultVoltage="1"/>
  <layers>
    <layer id="0" name="Base"/>
  </layers>
  <voltageClasses>
    <class id="1" name="110 kV" color="#00b4ff"/>
  </voltageClasses>
  <nodes>
    <node id="2" x="200" y="100"/>
    <node id="3" x="200" y="120"/>
    <node id="4" x="200" y="140"/>
    <node id="5" x="200" y="160"/>
    <node id="6" x="200" y="200"/>
    <node id="7" x="120" y="200"/>
    <node id="8" x="300" y="280"/>
  </nodes>
  <elements>
    <element id="9" class="BusBarSection" shape="24" name="1 bus 110 kV" voltage="1" layer="0" x="175" y="100">
      <port name="1" node="2"/>
      <geometry>
        <point x="100" y="100"/>
        <point x="250" y="100"/>
      </geometry>
    </element>
    <element id="10" class="BusBarSection" shape="24" name="2 bus 110 kV" voltage="1" layer="0" x="175" y="200">
      <port name="1" node="6"/>
      <port name="2" node="7"/>
      <geometry>
        <point x="100" y="200"/>
        <point x="250" y="200"/>
      </geometry>
    </element>
    <element id="11" class="Disconnector" shape="162" name="QS-1" voltage="1" layer="0" x="200" y="110" state="1">
      <port name="1" node="2"/>
      <port name="2" node="3"/>
    </element>
    <element id="12" class="Breaker" shape="41" name="Q-1" voltage="1" layer="0" x="200" y="150" state="0">
      <port name="1" node="4"/>
      <port name="2" node="5"/>
    </element>
  </elements>
  <connectors>
    <connector id="13" kind="BusWork" voltage="1" layer="0" from="3" to="4">
      <point x="200" y="120"/>
      <point x="200" y="140"/>
    </connector>
    <connector id="14" kind="BusWork" voltage="1" layer="0" from="5" to="6">
      <point x="200" y="160"/>
      <point x="200" y="200"/>
    </connector>
    <connector id="15" kind="OverheadLine" name="Overhead line-15" voltage="1" layer="0" from="7" to="8">
      <point x="120" y="200"/>
      <point x="120" y="280"/>
      <point x="300" y="280"/>
    </connector>
  </connectors>
  <labels>
    <label id="16" for="12" layer="0" x="215" y="155" size="12">Q-1</label>
  </labels>
</diagram>
```

Reading the topology:

- node 2 — bus 1 and QS-1 terminal 1 (the disconnector sits directly on the bus);
- QS-1 (closed) — node 2 ↔ node 3; wire 13 — node 3 ↔ node 4;
- Q-1 (open) — node 4 ↔ node 5; wire 14 — node 5 ↔ node 6, a tap on bus 2;
- overhead line 15 — leaves bus 2 at node 7 (a second tap, joined to node 6
  through the busbar itself) and ends unconnected at node 8.

---
name: render-data-as-ui
description: >-
  Turn any data a sidecar produces into a rendered UI page — KPIs, charts
  (line/bar/area/pie/donut), tables, timelines, rich text — by emitting a
  page DSL tree to the display-ui-component tool. Use when the user says
  "show me X as a page / dashboard / with graphs / visualize this". Works
  over the already-shipped 22-node display DSL; no custom code, no TSX.
icon: "📊"
iconName: chart-line
hashtags: [ui, display, dashboard, charts, visualization, render]
category: lifecycle
---

# Render data as a UI page

This skill turns **data you already have** (a service result, a list of
records, a rollup) into a **rendered page** via one `display-ui-component`
call with `kind: "page"` and a `dslTree`. The display engine ships 22 node
kinds — including a full chart suite — so most dashboards need zero custom
code.

> **The rule:** compose only the shipped DSL nodes. Never reach for custom TSX
> (the tier-2 escape hatch is not needed for KPIs/charts/tables). If a chart
> shape isn't expressible, say so and pick the closest node — don't invent one.

## The node palette (what you can compose)

**Layout** — `vbox` / `hbox` (stacks; `gap`, `align`, `justify`), `grid`
(`cols` + `children`, row-major), `box` / `card` / `section` (titled),
`tabs` / `accordion`.

**Atomic** —
- `heading` { text, level 1-6 }
- `text` / `markdown` { content }
- `kpi` { label, value, trend?, trendColor?, description?, icon? } — a stat card
- `chart` (see below)
- `timeline`, `badge`, `link`, `divider`, `image`/`video`/`audio`/`file`, `code-block`

**Schema-bound** — `form` / `view` / `table` / `wizard` (render from a
registered data-type; see the `make-data-collection` + `author-schema` skills).
A `table` node's `schema` MUST have a `root` that resolves to an **`object`
with `fields`** (or an array whose `items` object has `fields`) and that names a
type present in `types` — a fieldless/placeholder/missing row type renders
nothing. Prefer a `markdown` table for ad-hoc rows you don't want to type a
schema for.

### The chart node

```jsonc
{
  "kind": "chart",
  "chartKind": "line",            // line | bar | area | pie | donut
  "data": [ {row}, {row}, ... ],  // array of row objects
  // line/bar/area:
  "dataKey": "date",              // categorical x-axis key on each row
  "series": [                     // numeric series to plot
    { "name": "tempMax", "label": "High °C", "color": "red.6" },
    { "name": "tempMin", "label": "Low °C",  "color": "blue.5" }
  ],
  "curveType": "monotone",        // line/area curve
  // pie/donut: use "nameKey" + "valueKey" instead of dataKey/series
  // shared:
  "title": "Temperature",
  "h": 260,                        // height px (default 240)
  "withLegend": true, "withTooltip": true
}
```

- `series[].name` and `dataKey` are **keys into each row object** — bind them
  to the field names in your data.
- With no data the engine renders a "No data" placeholder. **Never fabricate
  rows** to fill a chart — pass the real data or show empty.

## Procedure

1. **Get the data.** Call the service/tool that returns it (e.g. a sidecar's
   `get-forecast`, a `list-records`, a coverage rollup). Keep the row shape.
2. **Pick the visualization** for the user's intent:
   - trend over a sequence → `line` / `area` (x = the sequence key)
   - compare categories / magnitudes → `bar`
   - share of a whole → `pie` / `donut`
   - headline numbers → `kpi` cards in a `grid`
   - the raw rows → `table` (needs a registered schema) or a `markdown` table
3. **Compose the page** — a `vbox` root with a `heading`, a KPI `grid`, then
   the chart(s). Keep it to what answers the question; don't over-stuff.
4. **Emit it**: `display-ui-component({ kind: "page", title, dslTree, data })`.
   Put the rows inline in each chart's `data`, OR put one outer object in
   `data` and have nodes reference it via `dataField: "$.path"`.
5. **Stop.** The page renders in its own tab. Offer one refinement ("want it
   as a bar chart instead / grouped by week?").

## Worked example — a weather forecast page (the proof case)

Given `get-forecast` → `{ city, days: [{date, tempMax, tempMin, precipitationMm, condition}, ...] }`:

```jsonc
{
  "kind": "page",
  "title": "Phoenix — 7-day forecast",
  "dslTree": {
    "kind": "vbox", "gap": "md", "p": "md",
    "children": [
      { "kind": "heading", "text": "Phoenix — 7-day forecast", "level": 2 },
      { "kind": "grid", "cols": 3, "gap": "sm", "children": [
        { "kind": "kpi", "label": "Today high", "value": "34°C", "icon": "temperature-sun" },
        { "kind": "kpi", "label": "Today low",  "value": "21°C", "icon": "temperature" },
        { "kind": "kpi", "label": "Rain (7d)",  "value": "12 mm", "icon": "droplet" }
      ]},
      { "kind": "chart", "chartKind": "line", "title": "Temperature (°C)",
        "dataKey": "date",
        "series": [
          { "name": "tempMax", "label": "High", "color": "red.6" },
          { "name": "tempMin", "label": "Low",  "color": "blue.5" }
        ],
        "data": [ /* the days[] rows from get-forecast */ ] },
      { "kind": "chart", "chartKind": "bar", "title": "Precipitation (mm)",
        "dataKey": "date",
        "series": [ { "name": "precipitationMm", "label": "Rain", "color": "cyan.5" } ],
        "data": [ /* the same days[] rows */ ] }
    ]
  },
  "data": { "city": "Phoenix" }
}
```

That is a complete, valid page: a heading, three KPI cards, a temperature
line chart (two series), and a precipitation bar chart — rendered from the raw
forecast rows, no custom code.

## When NOT to use this skill

- You need an editable record form → use `make-data-collection` / `author-schema`
  (schema-bound `form`/`table`), not a hand-built page.
- The data doesn't exist yet → fetch it first (this skill renders, it doesn't query).
- A chart shape genuinely isn't in the palette → say so; don't invent a node kind.

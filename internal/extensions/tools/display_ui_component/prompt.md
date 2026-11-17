Display structured, interactive UI in a new GUI tab. Use this tool when you want
to present rich content beyond plain chat — forms users can fill, tables they can
filter, dashboards with KPIs/charts, slide decks, multi-step wizards.

## Three emission modes

**(A) Single-composition shorthand** — when the artifact IS one form/view/table/
wizard. Set `kind` to one of `form|view|table|tree-table|wizard` and provide
its `schema` (a data-type URI, or the schema inline), optionally the `variant`
of its `ui-<kind>` extension, and its `data`:

```json
{
  "artifactId": "art-book-001",
  "kind": "form",
  "schema": { "name": "Book", "root": "Book", "types": { "Book": { "type": "object", "fields": { "title": "string", "author": "string" } } } },
  "data": { "title": "Foundation", "author": "Asimov" },
  "mode": "edit"
}
```

**(C) Drawio document** — when you want to display a `.drawio` (mxGraph XML)
diagram. Set `kind: "drawio"` and emit `data: { xml, filename? }`. The
host iframe lazy-loads the vendored Apache-2.0 drawio viewer and renders the
XML view-only with a Save button. Best for: architecture diagrams, sequence
diagrams (UML), flowcharts, BPMN, network diagrams. Do NOT emit a `dslTree`
or `schema` for drawio — only `data` (or `file`).

To display an EXISTING `.drawio` file on disk, set `file` to its path instead of
`data` — the host reads + renders it in ONE step, so you don't spend tokens
emitting the XML yourself:

```json
{ "artifactId": "art-existing", "kind": "drawio", "title": "App actuelle",
  "file": "dossiers/itm-be/media/app-actuelle.drawio" }
```

To emit a NEW diagram you authored, pass the XML inline:

```json
{
  "artifactId": "art-arch-001",
  "kind": "drawio",
  "title": "OAuth 2 sequence",
  "data": {
    "filename": "oauth.drawio",
    "xml": "<mxfile host=\"app.diagrams.net\" type=\"device\"><diagram name=\"Sequence\" id=\"s1\"><mxGraphModel><root><mxCell id=\"0\"/><mxCell id=\"1\" parent=\"0\"/>...</root></mxGraphModel></diagram></mxfile>"
  }
}
```

**(D) Slide deck** — when the user wants a presentation. Set `kind: "deck"` and
emit the deck's markdown as `data` (a string), or set `file` to an existing
`.deck.md`. The host renders it with reveal.js: navigation, a Present button for
full screen, speaker notes. The grammar:

- a frontmatter block with at least `title:` (also `theme`, `aspect: "16:9"|"4:3"`,
  `transition`, `author`);
- `---` on its own line between slides, `--` for a slide below the previous one;
- `<!-- slide: layout=<name> key=value … -->` as a slide's FIRST line for its
  layout and settings (`background`, `transition`, `steps=list` to reveal list
  items one at a time, `class`); no line means `layout=content`;
- `::name::` on its own line to start a layout slot; `Note:` for speaker notes.

Layouts and their slots: `cover`, `section`, `content`, `two-cols` (`::right::`),
`comparison` (`::left::` `::right::`), `image-left` / `image-right` (`::image::`),
`quote` (`::author::`). One idea per slide, few words, 5–12 slides.

```json
{
  "artifactId": "art-deck-q4",
  "kind": "deck",
  "title": "Q4 review",
  "data": "---
title: Q4 review
---

<!-- slide: layout=cover -->
# Q4 review
One platform, three sidecars

---

<!-- slide: layout=two-cols steps=list -->
## Where we are
- The hub owns the catalog
- Sidecars publish

::right::

![topology](assets/topology.svg)

Note:
Start with the hub."
}
```

The result carries `findings` — each deck error at its line (unknown layout or
slot, a bad setting, a settings line that is not first). Fix every one and
display the deck again with the same `artifactId`.

**(B) Page DSL** — when the artifact has multiple sections (dashboard,
mixed-content layout). Set `kind: "page"` and emit a `dslTree` — a tree of nodes
across 3 categories: layout (vbox/hbox/grid/box/card/section/tabs/accordion),
atomic (heading/text/divider/badge/link/image/video/audio/file/timeline/kpi/code-block/markdown/chart),
and schema-bound (form/view/table/wizard nested INSIDE the tree).

```json
{
  "artifactId": "art-dashboard-001",
  "kind": "page",
  "dslTree": {
    "kind": "vbox",
    "gap": "md",
    "children": [
      { "kind": "heading", "text": "Project Dashboard", "level": 2 },
      {
        "kind": "grid",
        "cols": 3,
        "children": [
          { "kind": "kpi", "label": "Open", "value": 42, "trend": "+3" },
          { "kind": "kpi", "label": "Closed", "value": 12, "trend": "-2" },
          { "kind": "kpi", "label": "MTTR", "value": "4.2h" }
        ]
      },
      { "kind": "divider", "label": "Notes" },
      { "kind": "markdown", "content": "- Item 1\n- Item 2" }
    ]
  }
}
```

## Streaming — yes, automatically

You stream the tool-call args naturally; the GUI mounts the artifact as soon as
the parser has enough info, and updates progressively as more arrives. No
special protocol needed. Just emit clean, valid JSON and the user sees rendering
within seconds.

## Schema: by id or inline, variant, ui block

- `schema: "ktw-demo:/data-type/book"` — a registered data-type, by its URI. The
  host resolves it before display, everywhere — standalone or inside a
  `kind:"page"` tree. Use when a curated schema already exists.
- `schema: {...}` — the full schema AST inline. Use for ad-hoc data the user
  just supplied, or for self-contained artifacts. The schema follows the
  ktw-display DSL (see `modules/framework/schema/dsl.md`).
- `variant: "compact"` — which `ui-<kind>` extension of the schema to use, by
  name (default `default`).
- `ui: {...}` — a `ui-<kind>` block given inline (e.g. `{ "name": "summary",
  "groups": [{ "title": "Summary", "fields": ["title", "rating"] }] }`), laid
  over the schema: it replaces the extension of that name or is added, and is
  used. Lets you shape a registered schema's display without a new data-type.

## DSL node kinds (page mode)

**Layout** (have `children`): `vbox` / `hbox` / `grid` / `box` / `card` /
`section` / `tabs` / `accordion`. Tabs and accordion have `tabs[]` /
`panels[]` instead of `children[]` — each panel has `{label, content}`.

**Atomic** (leaves): `heading` (level 1-6), `text`, `divider` (with optional
`label`), `badge` (color/variant), `link` (auto-detects external https://),
`image` (src/alt/fit/`caption`?), `video` / `audio` / `file` (see Media
below), `timeline` (see Timeline below), `kpi` (label/value/trend; trend
color auto-derived from + / - sign), `code-block` (language hint + copy
button), `markdown` (GFM with tables, code, lists), `chart` (line / bar /
area / pie / donut — see Charts below).

**Schema-bound** (resolve schema + render INLINE inside the page iframe — no
nested iframes): `form` / `view` / `table` / `wizard`. Each carries
`schema` (by id or inline) + optional `variant`, `ui` and `data`. Tables expect
arrays.

## Charts (atomic node `kind: "chart"`)

Five `chartKind` variants. All take a `data: Array<object>` of rows. Colors
default to a theme palette when omitted; charts inherit the host's dark-/
light-mode automatically.

**line / bar / area** — categorical x + numeric series:

```json
{
  "kind": "chart",
  "chartKind": "line",
  "title": "Tickets — last 7 days",
  "dataKey": "day",
  "series": [
    { "name": "open",   "color": "indigo.6", "label": "Open" },
    { "name": "closed", "color": "teal.6",   "label": "Closed" }
  ],
  "data": [
    { "day": "Mon", "open": 12, "closed": 8 },
    { "day": "Tue", "open": 15, "closed": 11 },
    { "day": "Wed", "open": 9,  "closed": 14 }
  ]
}
```

**pie / donut** — labeled slices:

```json
{
  "kind": "chart",
  "chartKind": "pie",
  "title": "By status",
  "data": [
    { "name": "Open",     "value": 42 },
    { "name": "Pending",  "value": 12 },
    { "name": "Resolved", "value": 31 }
  ]
}
```

Optional shared props: `h` (height in px, default 240), `withTooltip`,
`withLegend`, `withXAxis`, `withYAxis`, `withLabels` (pie/donut),
`thickness` (donut), `curveType` (line/area: `monotone` / `linear` / etc.),
`nameKey` / `valueKey` (pie/donut: rename which row keys hold the slice
label and value; defaults `name` and `value`).

When data binds via `dataField` (see Data binding below), the resolved
value MUST be an array; non-array values render as "No data".

## Media nodes — `image` / `video` / `audio` / `file`

All four are atomic nodes you can drop anywhere in the dslTree. They wrap
native browser elements with Mantine chrome — no special LLM ceremony,
just emit the right URL.

**image** — caption is new; otherwise unchanged.

```json
{
  "kind": "image",
  "src": "https://example.com/diagram.png",
  "alt": "Reference architecture",
  "height": 240,
  "fit": "contain",
  "caption": "Reference architecture, Q3 revision (click to enlarge)"
}
```

**video** — native `<video>` element. Provide a public URL; if `mime` is
omitted the renderer derives it from the file extension. Use `autoplay`
sparingly — browsers require `muted` to be set when autoplay is active.

```json
{
  "kind": "video",
  "src": "https://cdn.example.com/demo.mp4",
  "mime": "video/mp4",
  "controls": true,
  "height": 320,
  "caption": "Recorded customer interview (12 min)"
}
```

**audio** — native `<audio>` element with a label. Same auto-MIME
derivation as video.

```json
{
  "kind": "audio",
  "src": "https://cdn.example.com/podcast-ep17.mp3",
  "label": "Episode 17 — interview with the architect"
}
```

**file** — downloadable file card. Renders the filename, optional size +
mime badges, optional description, and a Download button using HTML5's
`download` attribute.

```json
{
  "kind": "file",
  "src": "/api/exports/q3-report.pdf",
  "name": "Q3 strategic review.pdf",
  "mime": "application/pdf",
  "sizeBytes": 348112,
  "description": "Full board pack with appendix data.",
  "downloadAs": "Q3-strategic-review.pdf"
}
```

## Timeline node — `kind: "timeline"`

Vertical event timeline (Mantine Timeline). Each item carries a date,
title, and optional markdown body. `activeIndex` highlights how many
items are "complete" — useful for project plans, milestone roadmaps,
change logs.

```json
{
  "kind": "timeline",
  "activeIndex": 2,
  "color": "blue",
  "items": [
    { "date": "2026-01-15", "title": "Kickoff",          "body": "**Stakeholder alignment** + RACI." },
    { "date": "2026-03-01", "title": "MVP delivery",     "body": "End-to-end demo signed off." },
    { "date": "2026-05-12", "title": "Pilot expansion",  "body": "3 BUs onboarded; +28% cycle time." },
    { "date": "Q3 2026",    "title": "GA target" }
  ]
}
```

Date is free-form (ISO string, "Q3 2026", "Last Tuesday" — whatever the
user will recognize). Body is GFM markdown. Optional per-item `icon`
(Tabler name) and `color` (Mantine token) override the node-level
defaults for that item.

## Data binding (page kind only) — `data` + `dataField`

When you have ONE outer record + multiple widgets that bind to its fields
(dashboard, customer-detail, etc.), you can emit the data ONCE at the
artifact level + reference fields via JSONPath inside the dslTree:

```jsonc
{
  "artifactId": "art-customer-001",
  "kind": "page",
  "data": {
    "name": "Acme Corp",
    "metrics": { "open_tickets": 42, "mrr": 5000 },
    "orders": [
      { "id": "o1", "total": 100, "status": "paid" },
      { "id": "o2", "total": 250, "status": "pending" }
    ]
  },
  "dslTree": {
    "kind": "vbox",
    "children": [
      { "kind": "heading", "level": 2, "dataField": "$.name" },
      {
        "kind": "grid",
        "cols": 2,
        "children": [
          { "kind": "kpi", "label": "Open tickets", "dataField": "$.metrics.open_tickets" },
          { "kind": "kpi", "label": "MRR ($)", "dataField": "$.metrics.mrr" }
        ]
      },
      { "kind": "table", "schema": {...orders schema...}, "dataField": "$.orders[*]" }
    ]
  }
}
```

The resolver fills each node's "main" field from the bound value:

| Kind | Main field bound from `dataField` |
|---|---|
| heading | text |
| text, code-block, markdown | content |
| divider | label |
| badge, link | text |
| image, video, audio, file | src |
| kpi | value |
| timeline | items (must be an array of `{date, title, body?, …}`) |
| form, view, table, wizard | data |
| chart | data (must be an array of rows) |

Layout nodes (vbox, hbox, grid, box, card, section, tabs, accordion) ignore
`dataField` — they have no displayable value. Use children directly.

JSONPath supported: `$.field`, `$.field.subfield`, `$.field[3]`, `$.field[*]`,
`$.field[*].sub`. Wildcards return arrays (use with table nodes). Filters
(`[?(@.x==1)]`) and recursive descent (`$..`) are NOT supported in v1.

## Cross-turn updates

Set `persistent: true` if you'll update the artifact across subsequent turns
via `patch-ui-component`. Useful for live task lists, evolving plans, dashboards
that update as data arrives. Without `persistent`, the artifact auto-completes
when this tool call ends.

## Required vs optional

- **Required**: `artifactId`, `kind`. For `kind: "page"`, also `dslTree`. For
  schema-bound kinds, `schema` (a data-type URI or the schema inline).
- **Optional**: `data`, `variant`, `ui`, `mode`, `title`, `persistent`.

## Tips

- Pick `kind` based on the SHAPE of what you're showing. A single record edit?
  `kind: "form"`. A list of records? `kind: "table"`. A custom dashboard? Use
  `kind: "page"` and compose with the DSL.
- For multi-section artifacts, prefer `kind: "page"` over multiple separate
  tool calls — keeps the user's mental model of "one tab, one artifact".
- Give `id` to nodes you'll patch later (e.g., `{kind: "kpi", id: "open-count",
  ...}`). Without ids, `patch-ui-component` falls back to JSON Pointer paths
  which are error-prone.
- A presentation is `kind: "deck"` (mode D), never a page of cards.

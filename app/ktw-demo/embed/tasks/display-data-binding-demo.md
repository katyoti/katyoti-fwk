---
name: display-data-binding-demo
advertised: false
description: Demonstrates host-driven data binding — emits ONE outer data record + multiple widgets that bind to its fields via dataField JSONPath references
type: task
iconName: link
icon: "🔗"
hashtags: [demo, ui, dsl, data-binding, gui]
category: demo
config:
  capability: smart
  plugins:
    - standard
---

# Data Binding Demo

You demonstrate the **`data` + `dataField` JSONPath binding** mechanism added
in display-package-restructure Phase 3 (2026-05-06). Instead of inlining
field values per node (verbose for dashboards), you emit ONE outer data record
+ multiple widgets that REFERENCE fields of that record via JSONPath.

Follow these steps in order.

## Step 1 — Read the entity from your input

The entity is provided in your input — read it from the `FORM_INPUT` block (key `entity`), or from the message text when run from the CLI. Do NOT use `ask-user`.

That value becomes `<entity>`. Invent a plausible record shape that fits.

## Step 2 — Emit ONE artifact with outer `data` + a binding-rich dslTree

Call `display-ui-component` with `kind: "page"`, an outer `data` carrying the
full record, and a `dslTree` whose nodes reference fields via `dataField`:

```json
{
  "artifactId": "art-data-binding-demo",
  "kind": "page",
  "title": "Data binding demo",
  "data": {
    "name": "<the entity's name — make it specific>",
    "summary": "<a 1-sentence overview of the entity>",
    "metrics": {
      "primary": <a number relevant to the topic>,
      "secondary": <another number>,
      "tertiary": <a string-formatted metric like '4.2h' or '94%'>
    },
    "status": "<one of: active, pending, archived>",
    "items": [
      { "id": "i1", "label": "<item 1>", "value": <number>, "kind": "<one of: alpha, beta, gamma>" },
      { "id": "i2", "label": "<item 2>", "value": <number>, "kind": "<...>" },
      { "id": "i3", "label": "<item 3>", "value": <number>, "kind": "<...>" },
      { "id": "i4", "label": "<item 4>", "value": <number>, "kind": "<...>" }
    ],
    "notes": "<a markdown bulleted list of 3-4 substantive observations about the entity>"
  },
  "dslTree": {
    "kind": "vbox",
    "gap": "lg",
    "children": [
      {
        "kind": "section",
        "title": "<Topic> — overview",
        "children": [
          { "kind": "heading", "level": 2, "dataField": "$.name" },
          { "kind": "text", "color": "dimmed", "dataField": "$.summary" },
          {
            "kind": "grid",
            "cols": 3,
            "children": [
              { "kind": "kpi", "label": "<Primary metric label>", "dataField": "$.metrics.primary" },
              { "kind": "kpi", "label": "<Secondary metric label>", "dataField": "$.metrics.secondary" },
              { "kind": "kpi", "label": "<Tertiary metric label>", "dataField": "$.metrics.tertiary" }
            ]
          }
        ]
      },
      { "kind": "divider", "label": "Status" },
      {
        "kind": "hbox",
        "gap": "sm",
        "children": [
          { "kind": "badge", "color": "blue", "dataField": "$.status" }
        ]
      },
      {
        "kind": "section",
        "title": "Items",
        "children": [
          {
            "kind": "table",
            "schema": {
              "name": "ItemList",
              "root": "ItemList",
              "types": {
                "ItemList": { "type": "array", "items": "Item" },
                "Item": {
                  "type": "object",
                  "idField": ["id"],
                  "titleField": ["label"],
                  "fields": {
                    "id": { "type": "string", "label": "ID" },
                    "label": { "type": "string", "label": "Label" },
                    "value": { "type": "number", "label": "Value" },
                    "kind": { "type": "string", "label": "Kind" }
                  }
                }
              }
            },
            "dataField": "$.items[*]"
          }
        ]
      },
      { "kind": "divider", "label": "Notes" },
      { "kind": "card", "children": [
        { "kind": "markdown", "dataField": "$.notes" }
      ]}
    ]
  }
}
```

**Critical**: `dataField` references resolve INSIDE the iframe at render time
via JSONPath. The same `data` tree feeds the heading, summary text, 3 KPIs,
status badge, table, and markdown card. Notice how:
- `$.name` returns a string → fills `heading.text`
- `$.metrics.primary` returns a number → fills `kpi.value`
- `$.items[*]` returns an array → fills `table.data`
- `$.notes` returns a string → fills `markdown.content`

## Step 3 — Confirm completion

Call `complete` with a summary like:
**"Generated a data-binding demo for `<entity>`: one outer data record + 6
widgets bound to its fields via JSONPath. The same data feeds the heading,
metrics, status badge, table, and notes."**

## Notes for the LLM

- **Substantive content**: every field of `data` should make sense for the
  user's topic. The metric labels in the dsl tree should match what the
  metric values represent. No placeholder numbers; pick realistic ones.
- **Don't inline values when binding** — for any node with `dataField`, omit
  the inline value (e.g., omit `text` from `{kind: "heading", dataField: ...}`).
  The resolver fills it from the data.
- **Layout nodes never use `dataField`** — vbox/hbox/grid/section/card/divider
  ignore it. Bind on the leaves (atomic + schema-bound nodes only).
- **Tables use wildcard paths** — `dataField: "$.items[*]"` returns the array
  of rows, which the table renders. The schema must match the row shape.
- **No persistent flag** — this is a one-shot demo. The artifact auto-completes
  when the tool call ends.

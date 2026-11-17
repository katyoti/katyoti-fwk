---
name: make-data-collection
description: >-
  Stand up a working data collection from a sentence — a record type with
  Create/Read/Update/Delete plus a schema-bound table/form/view UI.
  Composes author-schema + the memory CRUD connection (or CWM objects) + the
  record tools + the form/view/table widgets. Use when the user wants to
  "manage / track / keep a list of X" with edits. NOT for a read-only
  chart page (render-data-as-ui) and NOT for just defining a shape
  (author-schema alone).
icon: "🗃️"
iconName: database
hashtags: [crud, collection, records, data-type, ui, form, table]
category: lifecycle
---

# Make a data collection (CRUD + UI) from a sentence

Composes already-shipped pieces into a mini-app: a schema (the record shape),
a CRUD connection (where rows live), the `records` tool plugin (the verbs),
and the schema-bound widgets (the UI). No Go code.

> Requires the `records` plugin in the task config (`plugins: [..., records]`)
> — it provides `load-record` / `list-records` / `save-record` / `delete-record`.

## The pieces

- **Schema** — author it with `author-schema` (a data-type with `ui-form` /
  `ui-view` / `ui-table`). Its URI is `<provider>:/data-type/<name>`.
- **Connection** — pick by durability:
  - `memory` — in-process, resets on restart. URI `memory:/<type>[/<id>]`. For scratch/demo.
  - persistent data is not a connection: record it as CWM objects of a type whose
    content follows the schema (`cwm-object-create` / `cwm-object-list`), visible in
    the GUI and committed with the module.
  State which you picked and why.
- **Tools** — `list-records` (read) · `load-record` (one) · `save-record`
  (create/update; pass `schema_uri` to validate — failures come back as
  per-field violations, not a tool error) · `delete-record` (destructive — ask first).

## Procedure

1. **Schema**: run `author-schema` (or reuse a registered data-type URI).
2. **Connection**: choose memory or CWM objects per durability; for memory, build the URI prefix.
3. **Seed** (optional): `save-record` a few rows so the table isn't empty.
4. **Mount the table**: `display-ui-component { kind: "table", schema: <data-type URI or inline>, data: <rows> }`.
   Optionally add a declarative row-click action (Mode B) to open a detail view
   without an agent round-trip.
5. **Interaction loop**: row click → `load-record` → `view`; "edit" →
   `form` → `save-record` (validate) → refresh table; "delete" → confirm →
   `delete-record`.

## Done when
- The collection lists its rows, a new row saves (bad fields → per-field
  violations), an edit round-trips, a delete (confirmed) removes it.

## Don't
- Don't `delete-record` without asking — it is destructive.
- Don't pick `memory` for data the user expects to persist — say it resets.
- Don't hand-build a form/table — the schema's ui-* extensions render it.

## Known automation gaps (state them if relevant)
- No natural-language → connection auto-discovery yet (you choose the prefix).
- The one CRUD connection is `memory`, code-registered; "connections from
  config" is not implemented.

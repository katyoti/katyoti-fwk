---
name: compose-ui-live
description: >-
  Show the user a live, schema-bound UI component (form / view / table) RIGHT
  NOW, in the running session — no file written, no rebuild, no sidecar. You
  author an inline ktw-schema/v1 shape + UI extension and mount it via
  display-ui-component with schema + data. Use when the user says
  "show me a form / table / detail for X" and wants it immediately. For
  persisted, restart-surviving data-types use author-schema's persisted mode +
  make-data-collection.
icon: "⚡"
iconName: bolt
hashtags: [ui, live, form, view, table, schema, inline]
category: lifecycle
---

# Compose a live UI component (in-session, no files)

This is the **live UI mode**: you hand the display engine an inline schema and
inline value and it renders a schema-bound widget in its own tab THIS session.
Nothing touches disk, nothing registers by URI, no rebuild. It is the fast path
for "let me see/edit X now" — the opposite of authoring a persisted data-type.

> **Live vs persisted — pick on durability:**
> - **Live (this skill)** — inline shape + value, mounted now, gone on restart.
>   No file, no URI, no `make-data-collection`.
> - **Persisted** — write the DSL to a sidecar's `embed/data-types/`, register
>   by URI, add CRUD via `make-data-collection`. Survives restart, visible in
>   the GUI. Use that when the user wants to KEEP the data.

## The two inline parts

1. **`schema`** — a full `ktw-schema/v1` shape (the same object
   `author-schema` produces) WITH the UI extension for the format you're
   mounting. Author each part with the format skills:
   - `ktw-methods:/skill/author-schema` — the data shape (fields, id, title).
   - `ktw-methods:/skill/author-ui-form` — add `ui-form` for an editable form.
   - `ktw-methods:/skill/author-ui-view` — add `ui-view` for a read-only detail.
   - `ktw-methods:/skill/author-ui-table` — add `ui-table` for a row list.
2. **`data`** — the data to render: one record object for `form`/`view`,
   an array of rows for `table`. Use the user's real data; never fabricate rows.

## Mount it

```jsonc
display-ui-component({
  "kind": "table",            // form | view | table
  "schema": { /* ktw-schema/v1 shape with the matching ui-* extension */ },
  "data":  [ /* rows (table) or one record (form/view) */ ]
})
```

The SchemaService validates `schema` at mount time — a bad shape comes
back as an error, fix it and re-mount.

## Procedure

1. **Pick the format** from intent: edit → `form`, read one → `view`,
   browse many → `table`.
2. **Author the inline shape** (author-schema) + the ONE matching UI extension
   (author-ui-form / -view / -table). You usually need just one extension here.
3. **Gather the value** — the real record(s). If there's no data yet, mount the
   empty `form` to capture it, or mount the `table` empty (the widget shows a
   "no data" state — don't invent rows).
4. **Mount** with `display-ui-component({ kind, schema, data })`.
5. **Iterate in-session**: re-mount with an updated `data` after the user
   edits or after you fetch more rows. Offer one refinement, then stop.

## When to escalate to persisted
- The user wants the data to survive restart or appear in the GUI → switch to
  `author-schema` persisted mode + `make-data-collection` (CRUD over a `memory`
  or `cwm-items` connection).
- The user wants a multi-panel dashboard / charts (not a single record widget)
  → `render-data-as-ui` (a `page` DSL tree).

## Don't
- Don't write a `.dsl.yaml` file or trigger a rebuild for a live component — the
  whole point is inline.
- Don't fabricate rows/values to fill a widget — pass real data or render empty.
- Don't reach for persisted CRUD when the user only asked to "see" something.

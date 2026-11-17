---
name: author-ui-view
description: >-
  Author the `ui-view` extension for a ktw-schema/v1 data-type — the read-only
  detail pane the shipped view widget renders. Use after the data shape exists
  (author-schema) when the user needs to READ a single record nicely. This is
  the VIEW format only; see author-ui-form (create/edit) and author-ui-table
  (lists). compose-ui-live mounts the result inline.
icon: "👁️"
iconName: eye
hashtags: [ui, view, extension, schema, detail]
category: lifecycle
---

# Author a ui-view extension

> Needs a data shape first — run `ktw-methods:/skill/author-schema` (or reuse a
> registered data-type). Load `ktw-methods:/reference/schema/dsl.md` §5 for the
> exact `extensions:` grammar before writing.

A `ui-view` adds ONE entry to the type's `extensions:` list. The shipped view
widget reads it to render a **read-only detail pane** for a single record —
fields shown as labelled, formatted values (no inputs). The content-types
decide how each value formats (markdown renders, dates format, ratings show
stars).

## The block

```yaml
extensions:
  - type: ui-view
    name: default            # view id; "default" unless you author variants
    layout: detail-pane      # detail-pane | single
    groups:
      - { title: "Overview", cols: 2, fields: [title, status, owner] }
      - { title: "Notes",    cols: 1, fields: [notes] }
      - { title: "Audit",    fields: [id, created_at, updated_at] }
```

- **`layout: detail-pane`** is the standard two-column detail; `single` is a
  flat single column for short records.
- **`groups`** mirror the form's grouping but here EVERYTHING is shown —
  there is no edit, so audit fields belong in their own group, not hidden.
- A view is read-only: it never submits. Pair it with a `ui-form` when the
  user also needs to edit.

## Procedure

1. **Confirm the shape**; for fields meant to render rich (long text, links,
   markdown) make sure the shape carries the matching content-type
   (`string@markdown`, `string@url`, …) — the view honours it.
2. **Group for reading**, lead with the title/identity fields, push system
   fields to a trailing group.
3. **Validate**: `ktw-compose schema parse <file>` → exit 0 and the AST carries
   the `ui-view` extension. (Live mode: SchemaService validates at mount.)

## Don't
- Don't add inputs/submit — that's `author-ui-form`.
- Don't reference undeclared fields — parse fails.
- Don't duplicate the table's column list here — a view shows the whole record.

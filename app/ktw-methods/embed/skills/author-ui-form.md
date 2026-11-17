---
name: author-ui-form
description: >-
  Author the `ui-form` extension for a ktw-schema/v1 data-type — the grouped
  create/edit form that the shipped form widget renders. Use after the data
  shape exists (author-schema) when the user needs to CREATE or EDIT records.
  This is the FORM format only; see author-ui-view (read-only detail) and
  author-ui-table (lists). compose-ui-live mounts the result inline.
icon: "📝"
iconName: form
hashtags: [ui, form, extension, schema, edit]
category: lifecycle
---

# Author a ui-form extension

> Needs a data shape first — run `ktw-methods:/skill/author-schema` (or reuse a
> registered data-type). Load `ktw-methods:/reference/schema/dsl.md` §5 for the
> exact `extensions:` grammar before writing.

A `ui-form` adds ONE entry to the type's `extensions:` list. The shipped form
widget reads it to render a grouped create/edit form bound to the fields. The
field **content-types drive the input widgets** (the shape's job); the form
extension only decides grouping, columns, labels, and which fields hide.

## The block

```yaml
extensions:
  - type: ui-form
    name: default            # form id; "default" unless you author variants
    layout: groups           # groups | single
    submit_label: "Save"
    groups:
      - { title: "Main",  cols: 2, fields: [title, status, owner] }
      - { title: "Detail", cols: 1, fields: [notes] }
      - { title: "Audit", visibility: [view], fields: [id, created_at] }
```

- **`layout`**: `groups` (titled sections) or `single` (one flat column).
- **`groups[].cols`**: 1–3 — how many fields per row in that group.
- **`groups[].fields`**: field names from the shape, in display order.
- **`visibility: [view]`** on a group (or on a field in the shape) hides it
  from edit — use it for `id` / `created_at` / computed audit fields.

## Procedure

1. **Confirm the shape** has the fields you reference; pick the tightest
   content-type on each (that, not the form, chooses the widget).
2. **Group by meaning**, not by type — related fields together; primary fields
   first; audit/system fields in a trailing `visibility: [view]` group.
3. **Set `cols`** to keep a group scannable (2 is the safe default; 1 for long
   text / markdown fields).
4. **Validate**: `ktw-compose schema parse <file>` → exit 0 and the AST carries
   the `ui-form` extension. (Live mode: SchemaService validates at mount.)

## Don't
- Don't reference a field the shape doesn't declare — parse fails.
- Don't put long markdown/text fields in a multi-col group — give them `cols: 1`.
- Don't hand-build form HTML — the widget renders from this block.

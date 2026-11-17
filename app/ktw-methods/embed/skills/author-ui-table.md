---
name: author-ui-table
description: >-
  Author the `ui-table` extension for a ktw-schema/v1 data-type — the row grid
  the shipped table widget renders. Use after the data shape exists
  (author-schema) when the user needs to LIST / browse records. This is the
  TABLE format only; see author-ui-form (create/edit) and author-ui-view
  (read-only detail). compose-ui-live mounts the result inline.
icon: "🧮"
iconName: table
hashtags: [ui, table, extension, schema, list]
category: lifecycle
---

# Author a ui-table extension

> Needs a data shape first — run `ktw-methods:/skill/author-schema` (or reuse a
> registered data-type). Load `ktw-methods:/reference/schema/dsl.md` §5 for the
> exact `extensions:` grammar before writing.

A `ui-table` adds ONE entry to the type's `extensions:` list. The shipped table
widget reads it to render a paginated, sortable row grid. Unlike form/view it
shows a **subset of fields as columns** — pick the ones that identify a row at
a glance, not the whole record.

> **Invariant (load-bearing):** the table's **row type must be an `object` with
> `fields`**, and every `columns[].id` must be one of those field names. The row
> type is the schema `root` object, or the `items` object when `root` is an
> array. A table whose `root` is missing from `types`, is a fieldless object, or
> is an unfilled placeholder (`<EntityList>`) has no columns to render — declare
> the `fields` first (see `author-schema`).

## The block

```yaml
extensions:
  - type: ui-table
    name: default            # table id; "default" unless you author variants
    columns:
      - { id: title,  label: "Title",  sortable: true }
      - { id: status, label: "Status", align: center, width: 90 }
      - { id: owner,  label: "Owner",  sortable: true }
    pagination: true
    pageSize: 15
    striped: true
    highlightOnHover: true
```

- **`columns[].id`** is a field name from the shape; `label` overrides the
  header text; `align` (`left|center|right`); `width` is px (omit to auto-size).
- **`sortable: true`** per column enables header-click sort.
- **`pagination` + `pageSize`** page the grid; drop pagination for short lists.
- Keep it to 3–6 columns — the row click opens the `ui-view` for the full
  record. Don't try to show every field.

## Procedure

1. **Confirm the shape**; choose the columns that let a user scan and find a
   row (title/identity, one status/category, one date or owner).
2. **Mark the sortable columns** (usually title + a date), set sensible
   `pageSize`.
3. **Validate**: `ktw-compose schema parse <file>` → exit 0 and the AST carries
   the `ui-table` extension. (Live mode: SchemaService validates at mount.)

## Don't
- Don't list every field as a column — pick the identifying few; the view shows
  the rest.
- Don't reference an undeclared field id — parse fails.
- Don't fabricate rows to fill the grid — show the real rows or empty.

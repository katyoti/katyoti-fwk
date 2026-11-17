---
name: author-schema
description: >-
  Turn a plain-language description of a record type ("a maintenance log
  with asset, date, severity, notes") into a valid ktw-schema/v1 data shape
  — the fields, content-types, id/title, and references. Use when the user
  wants to DEFINE a data shape / record type / "a schema for X". This is the
  SHAPE layer only; the per-format skills author-ui-form / author-ui-view /
  author-ui-table add the matching UI extensions, and compose-ui-live mounts
  it inline for a session. NOT for authoring a skill (use the compose
  create-skill flow) and NOT for a one-off read-only chart page
  (use render-data-as-ui).
icon: "🧩"
iconName: schema
hashtags: [schema, data-type, dsl, shape]
category: lifecycle
---

# Author a schema (data shape) from a description

> **Load the grammar reference** `ktw-methods:/reference/schema/dsl.md` before
> writing — it is the authoring spec for the `ktw-schema/v1` DSL (structure,
> shorthand, content-types, the filename rule). The same DSL is used for a
> task's input / output contract (`<task>.input.dsl.yaml` / `<task>.output.dsl.yaml`
> beside it, `root: Input`/`Output`)
> — NOT JSON Schema (`fields:`, never `properties:`).

From one sentence you produce the **data shape**: a valid `ktw-schema/v1`
data-type whose fields carry content-type hints, an id field, and a title
field. The shape is the foundation; the UI extensions that make it
renderable are authored by the per-format skills — one each:

> - `ktw-methods:/skill/author-ui-form` — the `ui-form` extension (grouped create/edit).
> - `ktw-methods:/skill/author-ui-view` — the `ui-view` extension (read-only detail).
> - `ktw-methods:/skill/author-ui-table` — the `ui-table` extension (columns, sort, pagination).
>
> **Two modes consume the result:**
> - **Live (in-session)** — `ktw-methods:/skill/compose-ui-live` mounts the shape
>   + chosen UI extension INLINE (`schema` / `data`) for the running
>   session. Nothing is written to disk; no rebuild. This is the default for
>   "show me a form/table for X right now".
> - **Persisted** — write the DSL to a sidecar's `embed/data-types/` so it
>   registers by URI (`<provider>:/data-type/<name>`) and survives restart.
>   Pairs with `make-data-collection` (adds CRUD) and `render-data-as-ui`
>   (read-only dashboards).

## The shape

```yaml
$schema: ktw-schema/v1
name: <kebab-name>          # the registered name; URI = <provider>:/data-type/<name>
version: 1
root: <RootType>
types:
  <RootType>:
    type: object
    label: <Human label>
    idField: [id]            # which field is the record id
    titleField: [<field>]    # which field titles a row/detail
    fields:
      <field>:
        type: <type>         # see content-types below
        label: <Human label>
        optional: true|false
        default: <value>
        description: <one line>
    extensions:
      - { type: ui-form,  name: default, layout: groups, groups: [...] }
      - { type: ui-view,  name: default, layout: detail-pane, groups: [...] }
      - { type: ui-table, name: default, columns: [{id, label, sortable?, align?, width?}], pagination: true }
```

> **Load-bearing invariant — a table/list MUST have a fielded row object.**
> A `ui-table` (and any list view) renders the **`fields`** of its *row type* —
> the `root` object, or the `items` object when `root` is an array. So the
> `root` you emit MUST resolve to an **`object` with a non-empty `fields` map**:
> - `root` must NAME a type that exists in `types` (never a placeholder like
>   `<EntityList>` left unfilled, never a name absent from `types`).
> - For a list, make `root` an array (`MyRowList` → `type: array, items: MyRow`)
>   whose `items` type is an object WITH `fields`; or make `root` the object
>   directly. Either way the row object needs ≥1 field.
> - An `object` type with no `fields` (or `fields: {}`) has nothing to show — the
>   renderer can't derive columns. Always declare the columns' fields.

## Content-type shorthands (pick the tightest one — it drives the input widget)

- `string`, `number`, `boolean`
- `string@email`, `string@markdown`, `string@date`, `string@date-time`, `string@url`
- `string@enum(a, b, c)` — a select
- `number@rating(max:5)` — stars · `number@slider(min:0, max:100)` — slider
- `<Type>[]` — array of a defined type · `->Other` — a reference

## Procedure

1. **Clarify the shape (don't over-model).** Extract the fields the user
   *implied*; for each pick the tightest content-type. If a field's type or
   meaning is unclear, ASK — do not guess. Confirm id + title fields.
2. **Add the UI extensions via the per-format skills.** Pick the format(s) the
   user needs and follow the matching skill: `author-ui-form` for create/edit,
   `author-ui-view` for read-only detail, `author-ui-table` for lists. Each adds
   one `extensions:` entry to this same data-type. You rarely need all three.
3. **(Persisted mode only) Write the DSL** at the owning sidecar's
   `embed/data-types/<name>.dsl.yaml` (the canonical home; consolidated packs use
   their `local/embed/cwm/data-types/`). **Naming rule (load-bearing):** the
   registered data-type id + URI segment is derived from the **FILE NAME** (minus
   `.dsl.yaml`), NOT the inner `name:` field. So `<filename-stem>` MUST equal the
   `name:` you declare and the `<provider>:/data-type/<name>` URI consumers
   reference — e.g. a type named `forecast-day` lives in `forecast-day.dsl.yaml`,
   never `forecast.dsl.yaml` (that would register as `forecast` and the URI
   silently wouldn't resolve). For **live mode** skip this step — hand the inline
   shape to `compose-ui-live` instead.
4. **Validate**: `ktw-compose schema parse <file>` → exit 0, AST carries the
   fields + extensions. Fix any parse error before declaring done. (Live mode:
   the SchemaService validates the inline shape at mount time.)
5. **Hand off**: live UI right now → `compose-ui-live`; persisted records + CRUD
   UI → `make-data-collection`; read-only dashboard → `render-data-as-ui`.

## Don't
- Don't invent fields the user didn't imply — ask instead (over-modelling is the
  common failure).
- Don't leave a field with no nameable type — flag it and ask.
- Don't emit a `root` that names a type missing from `types`, or a row `object`
  with an empty/absent `fields` map — a table built on it can't derive its
  columns. Every table/list root must resolve to an object with ≥1 field.
- Don't author a *skill* with this (that's `ktw-compose:/skill/create-skill`).

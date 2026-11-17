# ktw-schema DSL — authoring reference

> Agent-facing reference for writing a `ktw-schema/v1` DSL — used for **data-type
> assets** (`<name>.dsl.yaml`) AND for a **task's input / output contract**
> (`<task>.input.dsl.yaml` / `<task>.output.dsl.yaml` beside the task, same grammar). Loaded on demand by `author-schema`. The exhaustive
> spec (imports, inheritance, codegen targets, every content-type) is
> `modules/framework/schema/dsl.md` — cite it for edge cases; this covers
> the 95% you need to author one correctly.

## 1. File / block structure

```yaml
$schema: ktw-schema/v1     # required for a standalone data-type file (omit in a task's contract file)
name: <kebab-name>         # required — the schema/type name
version: 1                 # integer, monotonic (optional)
description: "..."         # optional
root: <RootType>           # required — the entry type
types:
  <RootType>:
    type: object
    label: <Human label>
    idField: [id]          # (records) which field is the id
    titleField: [title]    # (records) which field titles a row/detail
    fields:
      <field>: <shorthand>            # or the long form (below)
```

**For a task** the same shape is a file beside it — `<task>.input.dsl.yaml` /
`<task>.output.dsl.yaml` (`TASK.input.dsl.yaml` in a folder task), no `$schema:`
line needed — with `root: Input` / `root: Output` by convention:

```yaml
name: <task>-input
root: Input
types:
  Input:
    type: object
    fields:
      city:
        type: string
        description: "..."
        optional: true
```

> **NOT JSON Schema.** Use `types: { T: { fields: {...} } }` — never
> `{ type: object, properties: {...} }`. `fields:` not `properties:`.

## 2. Shorthand: `type[@content][?]`

```yaml
fields:
  id:       string@uuid           # content type uuid, required
  email:    string@email?         # optional (trailing ?)
  bio:      string@markdown
  score:    number@rating(max:5)  # inline params (1–3) in parens
  pct:      number@slider(min:0, max:100, step:5)
  active:   boolean@switch
  when:     string@date-time
  tags:     string[]              # array of strings
  items:    Item[]                # array of a user type
  owner:    ->User                # link many2one
  members:  ->User[]              # link many2many
  parent:   ~Category             # tree (self-referential)
  role:     string@enum(admin, member, guest)   # closed value set
  pick:     string | number | null               # inline choice (union)
```

- `?` = optional (equivalent to long-form `optional: true`).
- `@enum(...)` = a value restricted to a fixed set (string/number only).
  `a | b | null` = a value that can BE several shapes (a union). Different things.

## 3. Content-type catalog (the `@content`)

`string`: `@text(min,max,pattern,transform)` · `@email` · `@url` · `@uuid` ·
`@markdown` · `@date` · `@date-time` · `@label(color)` · `@image(max_size)` ·
`@enum(...)`.
`number`: `@rating(max,half)` · `@slider(min,max,step)` · `@currency(code)` ·
`@enum(...)`.
`boolean`: `@switch`.
Pick the **tightest** one — it chooses the input widget.

## 4. Long form (when >3 params, nesting, or metadata)

```yaml
title:
  type: string
  content: { type: text, min: 1, max: 255, transformation: capitalize }
  label: Title
  description: "Main title"
  optional: true
  default: ""
```

Field options: `label`, `description`, `optional` (bool), `default`,
`placeholder`, `visibility: [view]` (hide from edit), plus `content:` (long-form
content params) and `enum: [...]` (combine with a content type — the only way to
have both).

## 5. UI extensions (`extensions:` on a type — drive the rendered widgets)

```yaml
extensions:
  - type: ui-form                 # create/edit
    name: default
    layout: groups                # or single
    submit_label: "Save"
    groups:
      - { title: "Main", cols: 2, fields: [title, status] }
      - { title: "Audit", visibility: [view], fields: [id, created_at] }
  - type: ui-view                 # read-only detail
    name: default
    layout: detail-pane
    groups: [ ... ]
  - type: ui-table                # row grid
    name: default
    columns:
      - { id: title, label: "Title", sortable: true }
      - { id: status, label: "Status", align: center, width: 90 }
    pagination: true
    pageSize: 10
```

## 6. Naming rule (load-bearing for data-type FILES)

A standalone data-type registers under its **FILE NAME** (minus `.dsl.yaml`),
NOT the inner `name:`. So `<filename-stem>` MUST equal `name:` and the
`<provider>:/data-type/<name>` URI consumers use — e.g. type `forecast-day` →
file `forecast-day.dsl.yaml` (never `forecast.dsl.yaml`, which registers as
`forecast` and the URI silently won't resolve). (Inline task schemas have no
file, so this is data-type-files only.)

## 7. Validate

`ktw-agents schema parse <file>` → exit 0; the AST must carry your fields +
extensions. For a task, boot the host — its contract files are published as the
data-types `<provider>:/data-type/task/<task>/input|output` (declared, not
enforced: caller input is not validated before dispatch).

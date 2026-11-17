# Authoring drawio notations, viewpoints and templates

The drawio engine draws a **LogicalDiagram** (typed nodes, edges and zones)
through a **viewpoint**. Three contribution kinds, each a file a sidecar
publishes, say everything about how a diagram looks and what it may contain:

| Kind | Says | File (in ktw-docs: `embed/drawio/…`) | URI |
|---|---|---|---|
| `notation` | the element kinds, relationship kinds and rules of a language | `notations/<id>.yaml` | `<provider>:/notation/<id>` |
| `viewpoint` | the type of drawing: a notation, the kinds it shows, its templates, its layout | `viewpoints/<id>.yaml` | `<provider>:/viewpoint/<id>` |
| `drawio-template` | the looks: draw.io templates plus the bindings they read | `drawio-templates/<id>.yaml` + `<id>.drawio` | `<provider>:/drawio-template/<id>` |

A sidecar publishes its own with `drawiocontrib.ContributeDrawio(provider, loader, root)`
over a folder laid out the same way. Every kind is resolved through the universe
catalog, so a viewpoint in one sidecar may use a notation from another.

Schemas: `ktw-docs:/data-type/notation`, `…/viewpoint`, `…/drawio-template`.

## Notation

```yaml
id: archimate3
displayName: ArchiMate 3
version: "3.2"
elements:
  BusinessActor: {label: Business Actor, category: business, description: …}
  ApplicationComponent: {label: Application Component, category: application}
relationships:
  serving: {label: Used By (Serving)}
  triggering: {label: Triggering}
rules:            # optional; absent = any relationship joins any elements
  allowed:
    - {relationship: triggering, sources: [category:business], targets: [BusinessProcess]}
    - {relationship: serving, sources: ["*"], targets: ["*"]}
```

- A node's `kind` must be an element kind, an edge's a relationship kind:
  anything else is a validation **error** (`unknown-kind`, `unknown-edge-kind`).
- With `rules.allowed`, an edge is correct only when some rule allows its
  (source kind, relationship, target kind). An entry is a kind,
  `category:<name>` or `*`. A forbidden edge is an **error**
  (`forbidden-relationship`) naming the relationships that are allowed.

## Viewpoint

```yaml
id: archimate3-application-cooperation
displayName: ArchiMate — application cooperation
notation: ktw-docs:/notation/archimate3
templates: ktw-docs:/drawio-template/archimate3
page: default                     # a page template of the library; empty = no frame
elements: [ApplicationComponent, ApplicationService, DataObject]   # empty = all
relationships: [flow, serving, access]                             # empty = all
layout: {algorithm: layered, direction: RIGHT}   # or {preset: archi-layered}, elkOptions: {…}
```

`lanes: true` in the layout draws the zones as lanes — bands across the
reading direction that every flow crosses, a zone's inner zones (`parent`)
stacked inside it (BPMN pools and lanes); otherwise zones are partitions along
the reading direction. `laneHeader: top` puts each zone's label above its
content and nests the zones as boxes inside boxes (AWS groups); the default,
`side`, puts it on the band's leading edge.

A kind the notation knows but the viewpoint leaves out renders, with a lint
**warning** (`outside-viewpoint`). The diagram's own `layoutHints` win field by
field over the viewpoint's `layout`.

## Template library

Two files with the same name. The YAML carries the constants and bindings:

```yaml
id: app-flux
displayName: Application flux
notation: <sidecar>:/notation/app-flux
constants:
  domainPalette:
    reference: {fill: "#d5e8d4", stroke: "#82B366"}
bindings:
  appFill:   {lookup: attributes.domain, table: domainPalette, field: fill, default: "#FFFFFF"}
  appStroke: {expr: "'xls' in (attributes.markers ?? []) ? '#107C41' : '#000000'"}
  flowCodes: {expr: "join(map(attributes.interface_codes ?? [], {'<a href=\"' + #.url + '\">' + #.code + '</a>'}), ' / ')"}
```

The `.drawio` file holds the templates. **Edit it in draw.io.** On any page
not named `page:<name>`:

| Mark (Edit Data on the cell) | Meaning |
|---|---|
| `ktw-template="<Kind>"` | this cell (and everything grouped inside it) draws element kind `<Kind>` |
| `ktw-template="edge:<kind>"` | this edge draws relationship kind `<kind>` |
| `ktw-template="*"` / `"edge:*"` | the default for kinds without a template |
| `ktw-template="zone:<kind>"` | draws a zone of that kind (`"kind"` on the zone: an AWS `Region`, a `VPC`) |
| `ktw-template="zone"` / `"zone:nested"` | draws a zone / a zone inside another (a BPMN pool / lane) when no `zone:<kind>` applies; without them a zone is a grey dashed box |
| `ktw-style="fillColor=appFill;strokeColor=appStroke"` | style keys a binding overrides; the style holds a sample value so the template previews |
| `ktw-when="'xls' in (attributes.markers ?? [])"` | on a child cell: kept only when the expression holds |
| `ktw-autosize="1"` | the element's height grows to fit its label (one line per `<br>` / `<hr>`) |

**Placeholders** — `%name%` in a label, a tooltip or a link — are draw.io's own:
set `placeholders="1"` and sample properties on the template, and draw.io
previews them. At render, a placeholder is filled from, in order: a binding of
that name, the element's attribute of that name, `name` / `id` / `kind`, a page
variable. Model values are HTML-escaped inside a label; binding values are
trusted HTML (a binding may compose links). Markup a missing value leaves empty
(`<i></i>`, a trailing `<br>`) is removed. An instance never keeps the
template's sample properties.

A page named `page:<name>` is a **page template**: its cells frame the drawing
(`%title%`, `%date%` placeholders); a cell with `ktw-slot="content"` places the
drawing at its corner, one with `ktw-slot="legend"` places the legend. The viewpoint names its page (`page:`); a diagram can pick another page of
the library and fill any of its placeholders with `"page": {"name": "dossier",
"values": {"project_code": "…"}}`.

## Bindings

A binding computes one value from the element's scope — `name`, `id`,
`kind`, `attributes`, `constants`:

| Primitive | Shape | Value |
|---|---|---|
| scalar | `"#FFFFFF"` / `3` / `true` | itself |
| const | `{const: domainPalette.reference.fill}` | a path into `constants` |
| lookup | `{lookup: attributes.domain, table: domainPalette, field: fill, default: "#FFF"}` | `constants[table][value][field]`, else `default` |
| match | `{match: attributes.status, cases: {live: green}, default: grey}` | the case for the value, else `default` |
| coalesce | `{coalesce: [{…}, {…}]}` | the first non-empty |
| expr | `{expr: "<el expression>"}` | an expression over the scope (`??`, `?.`, `map`, `join`, slicing) |

A binding that fails leaves the template's own value and is reported as a
warning of the render.

## What a drawn cell carries

Every element, relationship and zone is written as a draw.io `UserObject`
carrying its model identity (`ktw-role`, `ktw-id`, `ktw-kind`, `ktw-label`,
`ktw-attributes`, and each scalar attribute as a plain property). That is what
lets `ktw-docs drawio read` bring a drawing — and what a person changed in it —
back into its LogicalDiagram.

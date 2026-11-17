---
name: create-object-panel
description: "Author an object panel: a tab on the objects a filter admits: static content, a source's page, or a page declared over a source's data. Load to add a tab or a Global view."
advertised: false
hashtags: [cwm, object-panel, page, display, pane, authoring, create-items]
category: guide
---

# Authoring an object panel

The object pane's tab strip is **declared**: each `object-panel` whose filter
admits an object is one tab on it. A panel is ONE YAML file in a pack,
`embed/cwm/object-panels/<name>.yaml`, read by the binary's pack with no
wiring (a ktw-host content folder reads its `cwm/` folder the same way). Never ask for GUI code keyed by a type — a panel is the way. The field
and block tables are `ktw-compose:/reference/artifact/page-dsl.md`; worked
examples: ktw-demo's `object-panels/signoff-board.yaml` (a source answering a
page) and `contests.yaml` (a tab on an edge's target), ktw-methods'
`composition-overview.yaml` (a page declared over a source's data).

## 1. Is it a tab at all?

| You want… | Use, not a panel |
|---|---|
| how a type's Information tab looks | the type's `pane.view` |
| header chips | a capability's `badges` or the type's `pane.badges` |
| a capability's fields shown | nothing: Information draws them as a section |
| the children of a type listed | nothing: each undrawn child type gets a tab (`label_plural`) |

A panel is right for a view across objects, a dashboard, acts on the object, or
a tab on what points at it.

## 2. Choose who it shows on — the filter

```yaml
filter:
  types: [signoff-item]          # or "*"
  capabilities: [signoff]        # carried (effective set); naming one takes its fields out of Information
  incoming: [contests]           # has an incoming edge of this kind — what points AT it
  outgoing: [derived-from]
  when: "object.lifecycle.state != 'draft'"
```

Fields AND, a list is any-of. Select by capability when the tab belongs to an
ability, by type when it belongs to a kind of thing. A relation is
`incoming` / `outgoing`, never a graph read inside `when`. A misspelt key
refuses the panel.

## 3. Choose what it shows — three shapes

**Static content** (no method): a render definition whose `${…}` are resolved
over the object — a document kind from the object's fields or body.

```yaml
content:
  kind: markdown
  data: "**Verdict**: ${current.signoff.verdict}"
```

**A source answering the display**: the action is called with `{id, language}`
(plus the panel's `input` and the page's last query) and answers a whole
render definition — a page built in Go with the `render` builders. This needs
a method: reuse one, or hand the method to the code track.

```yaml
source: ":signoff/board"
refresh: {scope: module}
```

**A declared page over a source's data** — files-only when the source exists:
the source answers data, `content` is the page that binds it.

```yaml
source: "ktw-methods:method-assistant/composition"
refresh: {scope: children, types: [goal, use-case, deliverable]}
content:
  kind: page
  dslTree:
    kind: section
    title: Goals
    icon: bulb
    dataField: "$.goals"             # counts it; shows `empty` when it has none
    empty: No goals yet.
    aside:
      - {kind: button, label: Add, icon: plus, action: {label: Add a goal, create: {type: goal}}}
    children:
      - kind: table
        schema: "ktw-methods:/data-type/use-case"
        dataField: "$.goals"
        open: {navigate: {object: "${item.id}"}}
```

A row source answering a list fills a `table` content directly
(`content: {kind: table, schema: ":/data-type/contest-row"}`).

## 4. Acts on the page

Every action names exactly one verb — `uri` (+ `args`), `navigate`, `chat`,
`create`, `query` — on a button, a row, a link, a badge or a kpi; a form's
`submit` hands its value (`${value.<field>}`) to a `uri`. The host runs it,
honours the result convention `{ok, message, open?}` and re-reads the panel.
A picker for another object is a form field typed
`string@object-ref(types:…)`. Filter chips and saved views are `query`
actions over the panel's `input`.

## 5. Placement and refresh

- `order`: lower first, 100 when absent; the base tabs sit at 1 (Information),
  5 (Lifecycle), 850–900 (Links, Actions, Agent context).
- `refresh`: which object changes re-read it while shown — `self`,
  `children`, `module`, `universe`, `none`, narrowed by `types`. Pick the
  smallest scope that sees every change the page shows (a challenge written on
  ANOTHER object needs `module`).
- `global: true` adds it to the Global strip, its source called with
  `{target}`; with an empty filter it shows on no object. `input` carries its
  own parameters (saved views).
- `replaces_children: [<type>]` makes it stand for that type's children tab.

## 6. Name and translate

`name` is the tab label, unique across shipped panels (two "Overview" tabs
cannot be told apart — a repo test refuses a duplicate). Only `name` and
`description` are translated (`<name>.i18n.yaml`); texts inside a declared
page are shown as written, so a page that must follow the person's language is
answered by its source from `language`. Run
`ktw-compose:/skill/translate-contributions`.

## 7. Validate

1. Rebuild or restart, then ask the pane: its `Detail` for an admitted object
   (`ktw-compose scenario call <case> ktw-agents:object-pane/Detail
   '{"id":"<id>"}'`) lists the tab as `panel:<file name>`; an object the
   filter refuses does not get it.
2. Every schema the content names resolves (a dead data-type URI fails the
   panel), every action has one verb, every `${item.*}` path exists in the
   rows.
3. Look at it: `ktw-compose:/skill/test-gui` on a scenario case's universe —
   the tab, the counts, each act and the re-read after it.
4. Pin it in a scenario case stage — `expect` a `call` to
   `ktw-agents:object-pane/Detail` that `contains: "panel:<file name>"` — as
   `demo-showcase` does.

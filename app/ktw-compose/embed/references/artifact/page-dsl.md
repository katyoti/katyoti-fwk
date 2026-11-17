# Object panels and the page DSL — the lookup tables

What an `object-panel` declaration carries and what a `page` it shows may hold,
condensed. The grammar's source of truth is the display module
(`modules/display/page-dsl.md`) and the object pane
(`modules/object-pane/concepts.md`) in the ktw-agents repository.

## The panel declaration (`object-panels/<file>.yaml`)

| Field | Meaning |
|---|---|
| `name`, `description` | the tab label (unique across shipped panels) and its tooltip — the two translated texts |
| `icon` | `{tabler: <name>, emoji: <char>}` |
| `order` | sort key, lower first; 100 when absent (Information 1, Lifecycle 5, Links 850, Actions 860, Agent context 900, children tabs 1000) |
| `filter` | which objects show it: `{types, capabilities, incoming, outgoing, when}` — naming a capability takes its fields out of Information |
| `content` | a render definition; its `${…}` strings are resolved over the object once per pane |
| `source` | an action URI called with `{id, language, …the page's last query}`; its answer is the display and its data |
| `content` + `source` | the source answers the data, the content is the display it fills (a page binds it with `dataField`) |
| `refresh` | `{scope: self\|children\|module\|universe\|none, types}` — which object changes re-read it while shown; default `module` with a source, `none` for static content |
| `input` | laid under what the source is called with: the panel's own parameters (saved views) |
| `global`, `targets` | also a tab of the Global strip, its source called with `{target}` (empty = every module); `targets` is the action listing the choices |
| `replaces_children` | child types whose children tab this panel stands for |

A content's `kind`: `form`, `view`, `table`, `tree-table`, `wizard` (bound to a
`schema`: a data-type URI, `{action: <uri>}` for a form over an action's
input, or an AST inline), `page` (a `dslTree`), a document kind (`markdown`,
`html`, `mermaid`, `drawio`, `deck`), or `extension` (a registered view by
name). The pane resolves every schema before drawing: a dead data-type URI
fails the panel, naming it.

## Page blocks

- **Layout**: `vbox`, `hbox`, `grid`, `box`, `card`, `section`, `tabs`, `accordion`
- **Atomic**: `heading`, `text`, `divider`, `badge`, `link`, `image`, `video`, `audio`, `file`, `timeline`, `kpi`, `code-block`, `markdown`, `chart`
- **Schema-bound** (drawn inline): `form`, `view`, `table`, `wizard`
- **Interactive**: `button`, `list` (one `item` template per element, `$.item` inside), `tree`, `lanes` (drop runs `move`), `gauge`, `gantt`, `graph`

The blocks a panel reaches a registered view's quality with:

| Block | Draws |
|---|---|
| `section` | `title`, `icon` (`color`), a count badge, `aside` nodes (an Add button), its children or its `empty` text (`emptyColor: green` for an all-clear); `size: sm\|md\|lg`; bound by `dataField` to a collection, it counts it |
| `badge` | a pill: `color` or `colors` by value, `icon`, `tooltip`, `mono`; with an `action` it is a chip button |
| `text` | `size`, `color`, `fw`, `fs`; `lineClamp: 2` holds it to two lines, `mono` the monospace font, `expandable` (with `lineClamp`) a « Afficher plus » / « Afficher moins » toggle drawn only when it overflows (`text:<id>:toggle`) |
| `gauge` | `value` / `max`, or stacked `segments: [{value, color, label}]` with `legend` (`true` chips, `swatch` a coloured square per part) |
| `markdown` | GFM; `maxHeight` folds it behind *Show more* |
| `kpi`, `card`, `divider` | a headline figure, a frame (`dashed: true` its border dashed), a separator (`variant: dashed`) |
| `grid` | `cols` columns; `ratio: [2, 1]` by share, or `widths: [4, 150, 0, 210]` in pixels (0 shares the rest); `dividers: [2, 3]` a line left of those columns (0-based column indexes, the first (0) excluded); a number `gap` in pixels |
| `accordion` | `panels: [{label, icon, badge, color, content}]`, `multiple`, `defaultOpen`, `look: rule` (no frame: a caret, the label a tag in its `color`, its count, a rule) |
| `table` | rows by its schema; `open` (a row click) and row `actions` (a last column) |

## Data binding

A page carries one `data` object; a node binds with `dataField: "$.path"`
(`$.a`, `$.a.b`, `$.a[3]`, `$.a[*]`, `$.a[*].b` — no filters). The bound field:
`text` (heading, badge, link), `content` (text, markdown, code-block), `value`
(kpi, gauge), `data` (form, view, table, wizard, list, tree, lanes, gantt,
chart, graph), `label` (button, divider), `src` (media). A `form` bound by
`dataField` is two-way; nothing reaches the producer until it submits.

## Actions — exactly one verb each

| Verb | Does |
|---|---|
| `uri` (+ `args`) | runs a method; a form's `submit` with no `args` passes the whole value |
| `navigate: {object, panel?, query?}` | opens an object, on one of its panels; `query` opens that panel as a `query` action would (the Files tab on one file: `query: {file: "${item.path}"}`) |
| `chat: {task, target?}` | opens a chat running the task on an object |
| `create: {type?, parent?}` | opens the new-object dialog (under the panel's object by default) |
| `query: {args?}` | re-reads the panel's source with `args` over its input (a filter chip, a saved view) |

Plus `label`, `icon`, `confirm`, `dangerous`. An action sits on a `button`,
`kpi`, `link`, `badge` (`action`), a list / tree / lane / table row
(`actions`, `open`), a drop (`move`). Its strings may hold `${item.<path>}`,
`${value.<path>}`, `${lane}`, `${index}`, and the host's `${id}` /
`${target}`. A method may answer the result convention `{ok, message, open?}`:
`ok: false` shows a refusal, `open` (`<id>` or `<id>#<panel>`) is navigated to;
the panel is re-read after every action. An action with no verb or two is
refused and drawn disabled.

## Reference fields

In a data-type, `string@object-ref(types:signoff-item)` (an object id) or
`string@panel-ref(…)` (`<id>#<panel>`), their params an object filter, are drawn
as a picker searching the objects the filter admits; shown, they are links.

## Languages

Only the panel's `name` and `description` are translated (`<file>.i18n.yaml`).
The texts written inside a declared `content` are shown as written. A page
that must speak the person's language is answered by its source, which
receives `language` (`en`, `fr`) and builds its texts in it — ktw-demo's
`signoff/board` does.

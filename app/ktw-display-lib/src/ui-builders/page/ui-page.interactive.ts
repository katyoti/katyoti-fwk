// SPDX-License-Identifier: Apache-2.0
// Interactive nodes of the page DSL — the components a declared panel is
// composed of beyond the layout and atomic ones: a button, a list (one
// template repeated per element), a tree, lanes (cards in columns, a drag runs
// an action), a gauge, a Gantt and a graph (boxes and edges).
//
// Every node is GENERIC: a data contract and nothing of any consumer; a
// sidecar answers data in the contract and gets the component. An action is
// DECLARATIVE — a method URI and its arguments — because a sidecar cannot send
// a callback: the HOST (the panel the page runs in) resolves the arguments
// against the item and the page's scope, asks a confirmation when declared,
// runs it and refreshes. A page with no host (a chat artifact) shows its
// actions disabled.

import type { UIPageNode, UIPageNodeBase } from './ui-page.types.ts'
import { refTarget } from '../../ui-core/host/index.tsx'
import { resolveJSONPath } from '../../utils/json-path.ts'
import { isPanelId } from './panel.ts'

// =================================================================================================
// Actions
// =================================================================================================

/** A declarative action. It does EXACTLY ONE thing, named by its verb field:
 *
 *  - `uri` runs a method (`<provider>:<slot>/<method>`) with `args`; its
 *    answer may follow the result convention (`UIActionResult`);
 *  - `navigate` opens an object, on one of its panels when `panel` is given;
 *  - `chat` opens a chat running `task` on `target` (an object id);
 *  - `create` opens the host's "new object" dialog (`type` preset, under
 *    `parent`), then re-reads the panel;
 *  - `query` re-reads the panel's source with `args` laid over its own input
 *    (a filter, a saved view) — each query replaces the last; a form's submit
 *    with no `args` lays the form's whole value;
 *  - `tab` shows a tab of the page, `select` sets a key of the page's
 *    selection — both run by the page itself.
 *
 *  String values of `args` and of the verb fields may hold `${item.<path>}`
 *  (the element the action is on — a list item, a card, a row), `${lane}` (the
 *  lane a card is dropped on), `${value.<path>}` (a submitted form's value),
 *  `${selection.<key>}` (the page's selection) or `${<name>}` from the host's
 *  scope (`${target}`, `${id}`). */
export interface UIPageAction {
  /** Identifier, unique among a node's actions. */
  id?: string
  label?: string
  /** Tabler icon name. */
  icon?: string
  /** Mantine color token. */
  color?: string
  /** Run a method: an action URI (`<provider>:<slot>/<method>`). */
  uri?: string
  args?: Record<string, unknown>
  /** Open an object, on one of its panels when `panel` is given. */
  navigate?: UINavigateTarget
  /** Open a chat running a task on an object. */
  chat?: { task: string; target?: string }
  /** Open the host's new-object dialog: the type preset, under the parent
   *  (the panel's object when absent). */
  create?: { type?: string; parent?: string }
  /** Re-read the panel's source with these args over its input; no args on a
   *  form's submit = the form's value. A page whose host has no source to
   *  re-read does nothing with it. */
  query?: { args?: Record<string, unknown> }
  /** Show a tab of this page: the id of its panel. Run by the page itself,
   *  never handed to the host. */
  tab?: string
  /** With `tab`: an element of that tab to bring into view and highlight
   *  - an element id (`picker:sources`), a
   *  row's or a card's name, a section's id or a DOM id. */
  focus?: string
  /** Set a key of this page's selection (`value` resolved like args:
   *  `${item.id}`): run by the page itself, never handed to the host; the
   *  sections whose load args read `${selection.<key>}` re-read. */
  select?: { key: string; value?: unknown }
  /** Open a side drawer: `node`, a page node the page draws
   *  itself - an action's form, a section -, an action run from it closing
   *  the drawer; or `panel`, a panel of `object` (the page's object when
   *  absent) the host draws. */
  drawer?: UIDrawerTarget
  /** Open a centred modal holding `node`, a page node the page draws
   *  - a confirmation, a short form; an action run from it
   *  that the host takes closes it. */
  modal?: UIModalTarget
  /** Open a display panel by its id (dp-…) in a tab of its own - a
   *  conversation of « À faire » opens its chat; handed to the
   *  host (HostServices.openPanel). */
  panel?: string
  /** Open one of the host's own screens by its name: `todo`, « À faire » -
   *  what the rail's To do opens; handed to the host. */
  screen?: 'todo'
  /** Close what the page is drawn in: the host's dialog (the create dialog's
   *  « Annuler »), the page's own drawer or modal; handed to the host, which
   *  ignores it where the page fills a pane. */
  close?: boolean
  /** On an element's action: the element's field that must be set for the
   *  action to show (a retry only on what has a step to retry), or
   *  `field=value` - the field must hold that value (one action per stage:
   *  `next_step=freeze`). */
  when?: string
  /** On an element's action: the element's field holding why the action is
   *  not available yet (a validation until its hand tests passed). While the
   *  element sets it, the action shows disabled, that reason as its tooltip. */
  disabledWhen?: string
  /** Ask this question before running (yes / no). */
  confirm?: string
  /** A destructive action: shown red, and confirmed even without `confirm`. */
  dangerous?: boolean
  /** On a button: the person first chooses a file on their machine; its
   *  `name` and `content` (base64) are laid over the args and the action runs.
   *  Cancelled, nothing runs. `accept` filters the choice
   *  (`.pdf,.docx`); `maxSize` (bytes, FILE_MAX_SIZE when absent and never
   *  above it) refuses a larger file. */
  file?: UIFileChoice
}

/** What a file-choosing action accepts. */
export interface UIFileChoice {
  accept?: string
  maxSize?: number
}

/** The largest file an action hands: the call crosses the hub in one message
 *  of at most 8 MiB, and base64 adds a third. */
export const FILE_MAX_SIZE = 5 * 1024 * 1024

/** A file the person chose: its name, its content in base64, its size. */
export interface ChosenFile {
  name: string
  content: string
  size: number
}

/** withFile answers the action to run with a chosen file - its args `name`
 *  and `content` the exact templates `${file.name}` / `${file.content}`, the
 *  file in the event's extra, so the host lays the values themselves and a
 *  name holding `${…}` is never templated; the other args kept, `file`
 *  dropped - or why the file is refused (too large), in `language` (`fr`,
 *  else English). */
export function withFile(action: UIPageAction, f: ChosenFile, language = 'en'): { action: UIPageAction; extra: Record<string, unknown> } | { error: string } {
  const max = Math.min(action.file?.maxSize && action.file.maxSize > 0 ? action.file.maxSize : FILE_MAX_SIZE, FILE_MAX_SIZE)
  if (f.size > max) {
    return { error: tooLarge(f.name, megabytes(f.size, language), megabytes(max, language), language) }
  }
  const { file: _file, ...rest } = action
  return {
    action: { ...rest, args: { ...(action.args ?? {}), name: '${file.name}', content: '${file.content}' } },
    extra: { file: { name: f.name, content: f.content } },
  }
}

/** The refusal of a file too large, in the person's language. */
function tooLarge(name: string, size: string, max: string, language: string): string {
  return language.toLowerCase().startsWith('fr')
    ? `${name} est trop volumineux (${size} Mo, ${max} Mo au plus).`
    : `${name} is too large (${size} MB, at most ${max} MB).`
}

function megabytes(n: number, language: string): string {
  const s = (n / (1024 * 1024)).toFixed(1).replace(/\.0$/, '')
  return language.toLowerCase().startsWith('fr') ? s.replace('.', ',') : s
}

/** An object to open, on one of its panels when `panel` is given; `query`
 *  opens that panel as a `query` action would — its source read with these
 *  args over its input (the Files tab with one file selected); `element`
 *  names an element of that panel the host brings into view and highlights
 *  once drawn, addressed as a tab action's `focus`. */
export interface UINavigateTarget {
  object: string
  panel?: string
  query?: Record<string, unknown>
  element?: string
}

/** What a modal action opens: a page node, centred, with its title. */
export interface UIModalTarget {
  title?: string
  node: UIPageNode
  size?: 'xs' | 'sm' | 'md' | 'lg' | 'xl'
}

/** What a drawer action opens: a page node, a panel of an object, or a file
 *  of an object read in place - a markdown section, a draw.io diagram, a
 *  document -, its path module-relative. */
export interface UIDrawerTarget {
  title?: string
  node?: UIPageNode
  panel?: string
  file?: string
  object?: string
}

/** The verbs of an action — the fields of which exactly one is set. */
export const ACTION_VERBS = ['uri', 'navigate', 'chat', 'create', 'query', 'tab', 'select', 'drawer', 'modal', 'panel', 'screen', 'close'] as const
export type UIActionVerb = typeof ACTION_VERBS[number]

/** actionVerb names what an action does: its one verb field, or undefined
 *  when it has none or more than one (a malformed action, shown disabled). */
export function actionVerb(a: UIPageAction | undefined): UIActionVerb | undefined {
  if (!a) return undefined
  const set = ACTION_VERBS.filter((v) => a[v] !== undefined && a[v] !== null && a[v] !== '' && a[v] !== false)
  return set.length === 1 ? set[0] : undefined
}

/** pressOnly runs a button's own press and stops its click there:
 *  a button inside a clickable cell - a chip in a row that
 *  opens its own object - runs its action alone, never the cell's after it. */
export function pressOnly(e: { stopPropagation(): void }, press: () => void): void {
  e.stopPropagation()
  press()
}

/** resolveAction fills every template of an action — its args and its verb
 *  fields — against the item and the scope (resolveActionArgs). */
export function resolveAction(action: UIPageAction, item: unknown, scope: Record<string, unknown> = {}): UIPageAction {
  const fill = (v: Record<string, unknown> | undefined) => (v === undefined ? undefined : resolveActionArgs(v, item, scope))
  const out: UIPageAction = { ...action }
  if (action.args) out.args = fill(action.args)
  else if (action.uri && scope.value && typeof scope.value === 'object' && !Array.isArray(scope.value)) {
    // A submitted form's method with no args takes the form's value as its
    // input, as typed — never templated.
    out.args = { ...(scope.value as Record<string, unknown>) }
  }
  if (action.navigate) out.navigate = fill(action.navigate as unknown as Record<string, unknown>) as unknown as UINavigateTarget
  if (action.chat) out.chat = fill(action.chat) as UIPageAction['chat']
  if (action.create) out.create = fill(action.create) as UIPageAction['create']
  // A drawer the host draws (a panel, a file) names its object and file by
  // templates; a node the page draws keeps its own (bound when drawn).
  if (action.drawer && !action.drawer.node) out.drawer = fill(action.drawer as unknown as Record<string, unknown>) as unknown as UIDrawerTarget
  if (action.panel) out.panel = String(resolveActionArgs({ panel: action.panel }, item, scope).panel ?? '')
  if (action.select) out.select = { key: action.select.key, value: resolveActionArgs({ value: action.select.value }, item, scope).value }
  if (action.query) {
    // A query with no args on a submitted form lays the form's whole value.
    out.query = action.query.args
      ? { args: fill(action.query.args) }
      : (scope.value && typeof scope.value === 'object' && !Array.isArray(scope.value)
        ? { args: { ...(scope.value as Record<string, unknown>) } }
        : { args: {} })
  }
  return out
}

/** The result convention of a method run from a page: `ok` false is a
 *  refusal shown as such, `message` is shown, `open` navigates afterwards
 *  (an object id, `<id>#<panel>`, or `{object, panel}`), `undo` is the action
 *  taking the change back - its args written out - which the host offers
 *  with the message as « Undo », `project` a project folder
 *  the universe is to serve - the host has the hub switch to it and restarts
 *  the universe, `panel` the display panel the work runs in
 *  - the screen that ran it shows it in place (a `panel` node) with « Ouvrir
 *  dans un onglet ». A method answering anything else is a
 *  plain success. */
export interface UIActionResult {
  ok?: boolean
  message?: string
  open?: string | UINavigateTarget
  undo?: UIPageAction
  project?: string
  panel?: string
}

/** parseActionResult reads a method's answer (its JSON text or the value)
 *  as the result convention: undefined when it does not follow it. An `undo`
 *  that names no method (`uri`) is left out - only a method takes a change
 *  back. */
export function parseActionResult(value: unknown): { ok: boolean; message?: string; open?: UINavigateTarget; undo?: UIPageAction; project?: string; panel?: string } | undefined {
  let v = value
  if (typeof v === 'string') {
    if (v.trim() === '') return undefined
    try { v = JSON.parse(v) } catch { return undefined }
  }
  if (!v || typeof v !== 'object' || Array.isArray(v)) return undefined
  const r = v as Record<string, unknown>
  const has = (k: string) => Object.prototype.hasOwnProperty.call(r, k)
  if (!has('ok') && !has('message') && !has('open') && !has('undo') && !has('project') && !has('panel')) return undefined
  if (has('ok') && typeof r.ok !== 'boolean') return undefined
  if (has('message') && typeof r.message !== 'string') return undefined
  const open = navigateTarget(r.open)
  if (has('open') && r.open !== undefined && r.open !== null && !open) return undefined
  const u = r.undo as UIPageAction | undefined
  const undo = u && typeof u === 'object' && !Array.isArray(u) && typeof u.uri === 'string' && u.uri !== '' ? u : undefined
  const out: { ok: boolean; message?: string; open?: UINavigateTarget; undo?: UIPageAction; project?: string; panel?: string } = { ok: r.ok !== false, message: r.message as string | undefined, open }
  if (undo && out.ok) out.undo = undo
  // A project folder the universe is to serve: the host
  // switches the universe to it.
  if (out.ok && typeof r.project === 'string' && r.project.trim() !== '') out.project = r.project
  // The display panel the work runs in, shown in place.
  if (isPanelId(r.panel)) out.panel = r.panel
  return out
}

/** resultPanel is the node a screen draws in place - under the button, in
 *  the row - for an action's result that runs in a display panel: a `panel`
 *  node embedding it; undefined for a result with none. */
export function resultPanel(result: { panel?: string } | undefined, title?: string): UIPanelNode | undefined {
  if (!result || !isPanelId(result.panel)) return undefined
  return title ? { kind: 'panel', id: `result:${result.panel}`, panelId: result.panel, title } : { kind: 'panel', id: `result:${result.panel}`, panelId: result.panel }
}

/** navigateTarget reads an object reference: `{object, panel}`, an id, or
 *  `<id>#<panel>` (the panel-ref value). */
export function navigateTarget(v: unknown): UINavigateTarget | undefined {
  if (typeof v === 'string') return refTarget(v)
  if (v && typeof v === 'object' && typeof (v as UINavigateTarget).object === 'string' && (v as UINavigateTarget).object !== '') {
    const t = v as UINavigateTarget
    const out: UINavigateTarget = { object: t.object }
    if (t.panel) out.panel = t.panel
    if (t.panel && t.query && typeof t.query === 'object' && !Array.isArray(t.query)) out.query = t.query
    if (t.panel && typeof t.element === 'string' && t.element.trim() !== '') out.element = t.element.trim()
    return out
  }
  return undefined
}

/** What the host's handler receives: the action, the element it is on, and
 *  any extra values (the lane of a drop, the value of a submitted form). */
export interface UIPageActionEvent {
  action: UIPageAction
  item?: unknown
  extra?: Record<string, unknown>
}

const TEMPLATE = /\$\{([^}]+)\}/g

/** resolveActionArgs fills an action's `${...}` templates: `item.<path>`
 *  against the item, any other name against `scope` (the host's values and
 *  the event's extras). A string that is exactly one template takes the
 *  value itself (a number stays a number); one embedded in text is
 *  stringified. An unknown name resolves to the empty string. Nested objects
 *  and arrays are resolved too. A name `scope` lacks is read in `around`,
 *  first found wins: the working copies of the pages a page-own action runs
 *  in, nearest first. */
export function resolveActionArgs(
  args: Record<string, unknown> | undefined,
  item: unknown,
  scope: Record<string, unknown> = {},
  around: unknown[] = [],
): Record<string, unknown> {
  const lookup = (name: string): unknown => {
    const n = name.trim()
    if (n === 'item') return item
    if (n.startsWith('item.')) return pathOf(item, n.slice(5))
    const v = pathOf(scope, n)
    if (v !== undefined) return v
    for (const d of around) {
      const w = pathOf(d, n)
      if (w !== undefined) return w
    }
    return undefined
  }
  const fill = (v: unknown): unknown => {
    if (typeof v === 'string') {
      const whole = /^\$\{([^}]+)\}$/.exec(v)
      if (whole) {
        // The value as it is — an array, an object, null —; only an absent one is ''.
        const got = lookup(whole[1])
        return got === undefined ? '' : got
      }
      return v.replace(TEMPLATE, (_, name: string) => {
        const got = lookup(name)
        return got === undefined || got === null ? '' : typeof got === 'object' ? JSON.stringify(got) : String(got)
      })
    }
    if (Array.isArray(v)) return v.map(fill)
    if (v && typeof v === 'object') {
      return Object.fromEntries(Object.entries(v as Record<string, unknown>).map(([k, x]) => [k, fill(x)]))
    }
    return v
  }
  return fill(args ?? {}) as Record<string, unknown>
}

/** pathOf reads a dotted path (`a.b.0.c`) in a value; undefined when absent. */
export function pathOf(value: unknown, path: string): unknown {
  let cur: unknown = value
  for (const seg of path.split('.')) {
    if (seg === '') continue
    if (cur === null || cur === undefined || typeof cur !== 'object') return undefined
    cur = (cur as Record<string, unknown>)[seg]
  }
  return cur
}

// =================================================================================================
// Nodes
// =================================================================================================

/** A button running one action. */
export interface UIButtonNode extends UIPageNodeBase {
  kind: 'button'
  label: string
  action: UIPageAction
  /** Tabler icon name; the action's when absent. */
  icon?: string
  /** The icon's side: before the label (left, the default) or after it
   *  (right) - « Suivante › ». */
  iconPosition?: 'left' | 'right'
  color?: string
  variant?: 'filled' | 'light' | 'outline' | 'subtle' | 'default'
  size?: 'xs' | 'sm' | 'md'
  /** Page selection keys that must be set for the button to run:
   *  while one is unset, empty or blank it shows
   *  disabled, `requiresHint` its tooltip - « Créer » until a choice and a
   *  name are given. */
  requires?: string[]
  requiresHint?: string
}

/** A list: ONE template repeated for each element of `data` — not a table,
 *  no columns. Inside the template `$.item` is the element (`$.index` its
 *  position) and every other path is the page's data. */
export interface UIListNode extends UIPageNodeBase {
  kind: 'list'
  /** Items drawn per page on a progressive page: the first
   *  page, then « Afficher plus » adds the next one. Default 50. */
  pageSize?: number
  /** The elements (bind with `dataField`). */
  data?: unknown[]
  /** What each element shows: any node, a whole form included. Leave it out
   *  when `itemType` names the elements' data type: each is then drawn by
   *  that type's `itemVariant` view. */
  item?: UIPageNode
  /** The data type each element is of - a data-type URI, or its schema
   *  inline - so the list draws each one by that type's own definition of
   *  `itemVariant` (its `ui-view` extension of that name) through the generic
   *  view, never a template written for this list. */
  itemType?: string | Record<string, unknown>
  /** The variant of `itemType` each element is drawn as: the name of its
   *  `ui-view` extension. Default `card`. */
  itemVariant?: string
  layout?: 'vertical' | 'horizontal' | 'grid'
  /** Columns of the grid layout (default 3). */
  columns?: number
  /** A spacing token, or pixels. */
  gap?: 'xs' | 'sm' | 'md' | 'lg' | 'xl' | number
  /** Shown when there is no element. */
  empty?: string
  /** Actions on each element, shown at its right. */
  /** A click on an element runs it, on that element. */
  open?: UIPageAction
  actions?: UIPageAction[]
  /** `cards` draws each element in a card (default `plain`). */
  variant?: 'plain' | 'cards'
  /** The padding of each element's card (a spacing token or pixels; default
   *  `sm`): 0 for a row whose cells pad themselves. */
  cardPadding?: string | number
  /** The element's field whose value picks its accent color (`toneColors`). */
  toneField?: string
  toneColors?: Record<string, string>
  /** Values of the tone field whose elements pulse — work happening now. */
  pulse?: string[]
  /** An inline editor each element can open: a remark typed
   *  in the card, with its own confirm and cancel. */
  editor?: UIListEditor
  /** The element's field the list groups its elements by:
   *  a header with a count per group - a click shows only that group, a
   *  second click shows all -, the elements drawn under their group's
   *  heading. */
  groupBy?: string
  /** The groups' order and labels; a value not named follows, in the order
   *  first seen, labelled by itself. */
  groups?: UIListGroup[]
  /** The element's field set (true, or any value but false and empty) when
   *  it is overdue: the header adds an overdue count that filters the same
   *  way. */
  overdueField?: string
  /** The overdue count's label (« en retard » / « overdue » by default). */
  overdueLabel?: string
  /** The element's field the headings are drawn by when it is not the
   *  counted one: the header counts and filters by `groupBy` (the kind), the
   *  elements are drawn under this field's headings (the holder) - each kept
   *  under a filter, « Nothing of this kind. » when nothing is left. */
  sectionBy?: string
  /** The `sectionBy` headings' order and labels; a value not named follows,
   *  in the order first seen, labelled by itself. */
  sectionGroups?: UIListGroup[]
  /** false draws the elements in their own order, without a heading per
   *  group: the header alone groups them. Default true. */
  sections?: boolean
  /** Actions run on several elements of one group at once: each element
   *  ticked gets its own run of the action - one answer each, never one for
   *  all -, skipping an element where the action's `when` does not hold or
   *  its `disabledWhen` is set. A tick in another group starts a new
   *  selection there. */
  bulk?: UIPageAction[]
  /** The header's own words: `filterLabel` names the counts
   *  for a screen reader (« Filter by group » by default), `filterHint` is
   *  the note under them while no count filters (« Click a count to show
   *  only that group. » by default) - a page saying « kind » sets its own. */
  filterLabel?: string
  filterHint?: string
  /** false draws the headings without their count: the header counts. */
  sectionCounts?: boolean
  /** The element's field holding the node it is drawn as, in place of the
   *  `item` template: a producer that draws each element itself - each card
   *  its own controls - still gets the list's grouping, filters, ticks and
   *  actions. An element without one is drawn by the template, or not at
   *  all. */
  nodeField?: string
  /** `tiles` draws the header's counts as KPI tiles - each its group's label,
   *  count, hint, icon and colour, a click filtering as a count does -
   *  `chips` (the default) as buttons. `allLabel` labels a first tile
   *  counting every element, a click on it showing all. */
  filterStyle?: 'chips' | 'tiles'
  allLabel?: string
  /** `card` draws each heading as a bordered card titled « <label> — <n> »
   *  with its group's icon, colour and hint, its elements inside; `heading`
   *  (the default) as a small upper-case label. */
  sectionStyle?: 'heading' | 'card'
  /** The element field naming its place: a dimmed line names it inside a
   *  section each time it differs from the element before (« Pour moi »,
   *  « Liste X · membres »); `places` give the values' labels. */
  placeField?: string
  places?: UIListGroup[]
}

/** A group of a grouped list: the value of its `groupBy` field and its label.
 *  A heading (`sectionGroups`) naming `only` holds those groups' elements
 *  alone: it is drawn with no filter or under one of theirs, and under one of
 *  theirs it is the only kind of heading drawn - the conversations apart
 *  from the questions. */
export interface UIListGroup {
  key: string
  label?: string
  only?: string[]
  /** Drawn as a tile (filterStyle: tiles) and a card (sectionStyle: card):
   *  a tabler icon, a colour, a one-line hint. */
  icon?: string
  color?: string
  hint?: string
}

/** elementNode is the node an element draws itself as - its `nodeField`'s
 *  value when that is a node -, else the list's template. */
export function elementNode(node: Pick<UIListNode, 'nodeField'>, el: unknown, template: UIPageNode): UIPageNode {
  if (!node.nodeField || !el || typeof el !== 'object') return template
  const own = (el as Record<string, unknown>)[node.nodeField]
  return own && typeof own === 'object' && typeof (own as { kind?: unknown }).kind === 'string' ? own as UIPageNode : template
}

/** The variant a list draws its typed elements as when it names none. */
export const DEFAULT_ITEM_VARIANT = 'card'

/** listItemNode is what a list draws for each element: its own `item`
 *  template, else - its elements' data type named by `itemType` - that
 *  type's `itemVariant` (default `card`) drawn by the generic view, bound to
 *  the element (`$.item`): one definition per kind of card, in the data
 *  type, never a template per list. Undefined when the list
 *  gives neither - nor `nodeField`, each element's own node (elementNode). */
export function listItemNode(node: Pick<UIListNode, 'id' | 'item' | 'itemType' | 'itemVariant' | 'nodeField'>): UIPageNode | undefined {
  if (node.item) return node.item
  // Each element draws its own node: an element without one draws nothing.
  if (!node.itemType && node.nodeField) return { kind: 'text', content: '' } as UIPageNode
  if (!node.itemType) return undefined
  return {
    kind: 'view',
    ...(node.id ? { id: `${node.id}:item` } : {}),
    schema: node.itemType as never,
    variant: node.itemVariant || DEFAULT_ITEM_VARIANT,
    dataField: '$.item',
  } as UIPageNode
}

/** An inline editor in a list card: its button opens a text
 *  box in the card; « Annuler » closes it, its confirm runs `confirm` on the
 *  element with `${value}` the text. */
export interface UIListEditor {
  /** The button opening it (« Refaire avec une remarque »). */
  label: string
  icon?: string
  /** What confirming runs on the element; its label is the confirm button's. */
  confirm: UIPageAction
  /** The cancel button's label. Default « Annuler ». */
  cancelLabel?: string
  placeholder?: string
  /** The element's field the text starts from (a remark kept). */
  field?: string
  /** The confirm waits for a text. Default true. */
  required?: boolean
  /** Shown only on the elements setting this field (`field=value` too). */
  when?: string
}

/** editorStart is the text an element's editor opens with. */
export function editorStart(editor: Pick<UIListEditor, 'field'>, item: unknown): string {
  if (!editor.field || !item || typeof item !== 'object') return ''
  const v = (item as Record<string, unknown>)[editor.field]
  return v === undefined || v === null ? '' : String(v)
}

/** editorReady says whether the confirm can run with this text. */
export function editorReady(editor: Pick<UIListEditor, 'required'>, text: string): boolean {
  return editor.required === false || text.trim() !== ''
}

/** editorEvent is what confirming sends the host: the confirm action on the
 *  element, `${value}` the text typed (trimmed). */
export function editorEvent(editor: Pick<UIListEditor, 'confirm'>, item: unknown, text: string): UIPageActionEvent {
  return { action: editor.confirm, item, extra: { value: text.trim() } }
}

/** A tree of rows linked by their parent — features and their sub-features,
 *  a demand and its parts. A row whose parent is absent or unknown is a root. */
export interface UITreeNode extends UIPageNodeBase {
  kind: 'tree'
  /** Items drawn per page on a progressive page: the first
   *  page, then « Afficher plus » adds the next one. Default 50. */
  pageSize?: number
  /** The rows, flat (bind with `dataField`). */
  data?: Array<Record<string, unknown>>
  idField?: string
  parentField?: string
  /** The row's label when no template is given. */
  titleField?: string
  /** A field shown as a badge at the row's right (a state). */
  badgeField?: string
  /** Badge colors by value (`{done: 'green'}`). */
  badgeColors?: Record<string, string>
  /** What each row shows instead of its title: any node, `$.item` the row. */
  item?: UIPageNode
  /** Open every row at first (default true for the first level only). */
  expanded?: boolean
  /** A click on a row runs it, on that row. */
  open?: UIPageAction
  actions?: UIPageAction[]
  empty?: string
  /** The row's field whose value picks its accent color (`toneColors`). */
  toneField?: string
  toneColors?: Record<string, string>
  /** Values of the tone field whose elements pulse — work happening now. */
  pulse?: string[]
}

export interface UILane {
  id: string
  title: string
  /** A line under the title. */
  meta?: string
  color?: string
  /** Lane ids a card may be dropped into from this lane (default: any). */
  accepts?: string[]
  /** What dropping a card INTO this lane runs, instead of the node's `move`
   *  (a backlog that untargets, beside releases that target). */
  move?: UIPageAction
}

/** Lanes: cards in columns — phases of the work, releases beside a backlog, a
 *  kanban. Dropping a card on another lane runs `move` with `${item.*}` and
 *  `${lane}`; without `move` the cards stay put. */
export interface UILanesNode extends UIPageNodeBase {
  kind: 'lanes'
  /** Items drawn per page on a progressive page: the first
   *  page, then « Afficher plus » adds the next one. Default 50. */
  pageSize?: number
  lanes: UILane[]
  /** The cards (bind with `dataField`). */
  data?: Array<Record<string, unknown>>
  /** The field naming a card's lane (default `lane`). */
  laneField?: string
  /** What each card shows: any node, `$.item` the card; its `title` otherwise. */
  item?: UIPageNode
  titleField?: string
  move?: UIPageAction
  /** A click on a card runs it, on that card. */
  open?: UIPageAction
  /** Actions on each card. */
  actions?: UIPageAction[]
  /** Minimum width of a lane, in pixels (default 220). */
  laneWidth?: number
  /** Maximum height of a lane's cards, in pixels: past it they scroll inside the
   *  lane, so what follows the lanes stays on screen (default: no limit).
   *  `fill`: the lanes take the window's height below them, each scrolling
   *  inside itself. */
  laneHeight?: number | 'fill'
  /** The card's field whose value picks its accent color (`toneColors`). */
  toneField?: string
  toneColors?: Record<string, string>
  /** Values of the tone field whose elements pulse — work happening now. */
  pulse?: string[]
}

/** One part of a stacked gauge: how many, its color, what it counts. */
export interface UIGaugeSegment {
  value: number
  /** Mantine color token. */
  color?: string
  /** What it counts (`pass`); the legend shows `<value> <label>`. */
  label?: string
  /** What it counts, listed when the segment is hovered, each opening its
   *  object: a goal's requests in that state. */
  items?: { id: string; title?: string }[]
}

/** A gauge: a value against its maximum, as a bar — a group's load, a
 *  release's progress, a tier reached. With `segments` it is a stacked bar
 *  (pass / deferred / failing of a total), each part colored, its parts
 *  listed on hover and, with `legend`, as colored chips (or swatches) under it. */
export interface UIGaugeNode extends UIPageNodeBase {
  kind: 'gauge'
  /** The label above the bar; no heading row without it. */
  label?: string
  /** The value (single bar); the sum of the segments when they are given. */
  value?: number
  /** The bar's full length. Default: 100, or the segments' sum. */
  max?: number
  color?: string
  /** A line under the bar; `{value}` and `{max}` are replaced. */
  hint?: string
  /** The stacked parts, in drawing order. */
  segments?: UIGaugeSegment[]
  /** The legend under a stacked bar, one entry per non-zero part: `true`
   *  colored chips; `'swatch'` a small square in the part's own color beside
   *  its dimmed count and label, wrapping. */
  legend?: boolean | 'swatch'
  /** The text at the right of the label; `{value}` `{max}` `{percent}` are
   *  replaced. Default `{value} / {max}`; empty hides it. */
  valueText?: string
  /** Bar thickness. Default: 'md'. */
  size?: 'xs' | 'sm' | 'md' | 'lg' | 'xl'
  /** A click on the gauge runs it. */
  action?: UIPageAction
}

export interface UIGanttTask {
  id: string
  title: string
  /** Its start on the axis, in units from 0 (a relative scale). */
  start?: number
  /** Its length, in units (a relative scale). */
  length?: number
  /** Its first day, YYYY-MM-DD (a calendar scale). */
  from?: string
  /** Its last day, YYYY-MM-DD (a calendar scale); its first day when absent. */
  to?: string
  critical?: boolean
  /** Ids it waits on. */
  after?: string[]
  color?: string
}

/** A Gantt: tasks as bars on a time axis, the critical path highlighted. */
export interface UIGanttNode extends UIPageNodeBase {
  kind: 'gantt'
  /** The tasks (bind with `dataField`). */
  data?: UIGanttTask[]
  /** The axis' unit, shown on its last tick (`h`, `d`). */
  unit?: string
  /** `calendar`: the tasks give `from` / `to` dates, the axis shows months and
   *  `today`; `relative` (the default): `start` / `length` in units. */
  scale?: 'relative' | 'calendar'
  /** The day the today line marks on a calendar scale, YYYY-MM-DD. */
  today?: string
  /** Clicking a bar runs it with `${item.*}` the task. */
  open?: UIPageAction
}

export interface UIGraphBox {
  id: string
  title: string
  meta?: string
  /** A Mantine color token; `critical` draws it accented. */
  tone?: string
  critical?: boolean
  /** The box whose frame it sits in: a box others name as
   *  their group is drawn as a frame titled by it around them, laid out with
   *  them; an edge to or from it orders all of them. */
  group?: string
  /** `dotted` draws the box, or the frame it is, dotted (work outside, an
   *  external dependency). */
  frame?: 'solid' | 'dotted'
  /** Its state in the shared status colours: done, running, waiting,
   *  blocked, todo (any planning status word). */
  status?: string
  /** A short line of badges under its title (« ready to run », who holds it). */
  badges?: string[]
}

export interface UIGraphEdge {
  from: string
  to: string
  /** `dashed` draws it dashed (a reads relation, an optional path). */
  style?: 'solid' | 'dashed'
  label?: string
}

/** A graph: boxes and edges, laid out in layers left to right (each box after
 *  the ones it comes from) — a PERT, a workflow's states, the module graph, a
 *  split demand. */
export interface UIGraphNode extends UIPageNodeBase {
  kind: 'graph'
  /** `{nodes, edges}` (bind with `dataField`). */
  data?: { nodes: UIGraphBox[]; edges: UIGraphEdge[] }
  /** Clicking a box runs it with `${item.*}` the box. */
  open?: UIPageAction
  /** Height in pixels (default: fits the layout). */
  height?: number
}

/** An item a picker offers or holds: its key and title, where it sits (a
 *  parent's key, in a tree), its declared badges, its state once applied
 *  (read, waiting, removed, failed - with why), unreachable when its source
 *  cannot be reached. */
export interface UIPickerItem {
  key: string
  title: string
  parent?: string
  kind?: string
  badges?: string[]
  state?: 'read' | 'waiting' | 'removed' | 'failed'
  why?: string
  unreachable?: boolean
  has_children?: boolean
  /** The connector's default scope (Confluence's default space): « Parcourir »
   *  opens it on its first draw. */
  open?: boolean
  /** The titles above it - where it is: a search hit nests under them.
 */
  path?: string[]
  /** Picked with what sits under it (a tree's « avec les sous-éléments »). */
  with_children?: boolean
  /** Unticked in a tickable picker: kept in « Sélection », struck through,
   *  never handed back. */
  off?: boolean
  /** false: chosen earlier but not proposed now - the proposal region of a
   *  picker declaring `proposals` draws it only as a chip, gone when
   *  unticked, never as a proposal row. Default proposed. */
  proposed?: boolean
  /** The group its row shows under in a tickable picker, each with its own
   *  « Tout cocher ». */
  group?: string
  /** The rest of its source's item roles: one line saying
   *  what it is, its attributes by name (an application's code), when it
   *  last changed at its source (RFC 3339). */
  detail?: string
  attrs?: Record<string, string>
  modified?: string
  /** Where it is at its source - a web link, or the source's own URI. */
  uri?: string
  /** How many items sit under it: a tree proposal row says « avec n
   *  sous-éléments » (the node's `belowOne` / `belowMany`). */
  below?: number
  /** A marker drawn at the right of its proposal row: its label, colour and
   *  icon name (a from-source marker). */
  pill?: { label: string; color?: string; icon?: string }
}

/** The sizes an item card is drawn at: a search hit, a
 *  browsed level's row, a held selection's row, a summary of the item. */
export type UIItemCardSize = 'search' | 'browse' | 'selection' | 'summary'

/** A slot an item kind declares: one of its attributes drawn on its card
 *  with its label, at the sizes it names (default every size). */
export interface UIItemSlot {
  attr: string
  label?: string
  sizes?: UIItemCardSize[]
}

/** What a source declares of one item kind: its label and
 *  the slots its cards draw - data, never code per kind. */
export interface UIItemKind {
  label?: string
  slots?: UIItemSlot[]
}

/** The item kinds a source declares, by kind. */
export type UIItemKinds = Record<string, UIItemKind>

/** Where a picker's source stands (its status method's answer): configured,
 *  reachable, why not, and where it is configured. */
export interface UIPickerStatus {
  configured?: boolean
  reachable?: boolean
  message?: string
  configure?: { label: string; hint?: string; keys?: string[] }
}

/** A badge a picker declares: its label, colour and what it means. */
export interface UIPickerBadge {
  label: string
  color?: string
  description?: string
  /** Drawn on a proposed row only while that row is unticked in the draft
   *  (mock-up v6's « nouvelle » pill): ticking it hides the badge. */
  unticked?: boolean
}

/** A generic picker: a search run on Enter through the
 *  `search` method, its matches, « Sélection » as a list or a tree,
 *  « Retirer », declared badges, a state per item, unreachable items marked
 *  (« Réessayer » asks its status again); tickable, each item ticked or
 *  unticked in place with why it was found, « Tout cocher » per group, the
 *  items already held checked and disabled;
 *  `apply` hands the selection back. Driven only by the method URIs it is
 *  given - the display library calls them through the host's callMethod -
 *  so any pack can use it. */
/** One place a picker's source searches in. */
export interface UIPickerScope {
  key: string
  title?: string
  /** It comes from the source's configuration. */
  default?: boolean
}

/** A picker's scopes. */
export interface UIPickerScopes {
  list: string
  unit?: string
  add?: boolean
  save?: string
}

/** A picker's proposal region. */
export interface UIPickerProposals {
  title: string
  /** A page node drawn under the title (the keyword chips). */
  sub?: UIPageNode
  /** Said when nothing is proposed. */
  none?: string
  /** A tree picker's proposals (`layout: 'tree'`): drawn above its search,
   *  each ticked through the tree's selection - taken with what sits under
   *  it, or included by a taken ancestor. */
  items?: UIPickerItem[]
  /** A tree picker's label above its search and tree, under the proposals
   *  (« Ou choisissez dans l'arbre de … »). */
  choose?: string
  /** The groups the proposals are drawn by, in this order (an item's
   *  `group` names its group's `key`): each a head with its check box, then
   *  its proposals - or its `empty` line when it holds none. A group found on
   *  an item but not declared follows, the items in no group last. */
  groups?: UIPickerGroup[]
}

/** A group of a picker's proposal region: its head's words. */
export interface UIPickerGroup {
  /** The `group` its items carry. */
  key: string
  title: string
  /** A short mark before the title (« LX »). */
  badge?: { label: string; color?: string }
  /** Drawn in monospace after the title. */
  code?: string
  /** A tag chip after the code. */
  chip?: string
  /** Names of the node's `badges` drawn on the head. */
  badges?: string[]
  /** Said when the group holds no proposal. */
  empty?: string
}

/** A picker's save bar. */
export interface UIPickerSavebar {
  one: string
  many: string
  idle?: string
  empty?: string
  cancel?: string
  save?: string
}

export interface UIPickerNode extends UIPageNodeBase {
  kind: 'picker'
  /** The method searching: `{object?, query}` -> `{items: UIPickerItem[]}`;
   *  none: the picker draws no search field - it browses, or holds what was
   *  proposed. */
  search?: string
  /** The method taking the selection: `{object?, items: [{key, title,
   *  parent?}]}` -> `{ok, message?}`; none: the picker only shows it. */
  apply?: string
  /** `change`: each change of the selection is handed to `apply` as it is
   *  made - after a short calm, one call at a time - and no « Appliquer » is
   *  drawn: a wizard step holding what is ticked. */
  applyOn?: 'change'
  /** The method listing a level to browse, « Parcourir »:
   *  `{object?, parent?}` -> `{items}`, each saying `has_children`. */
  browse?: string
  /** The method saying whether the source is configured and reachable
   *  (`{...args}` -> UIPickerStatus): a banner says so when it is not, the
   *  selection still shown. */
  status?: string
  /** What is read with each item without being picked - a note beside the
   *  selection. */
  derived?: string
  /** « Rechercher et ajouter » beside « Sélection », two columns.
 */
  columns?: boolean
  /** The source's scopes (shown by a host showing the source
   *  model): `list` answers them (`{...args, object?}` -> `{scopes: [{key,
   *  title?, default?}]}`, the default ones from the configuration and those
   *  the carrier added), drawn as chips ticked for the search; `add`: « +
   *  Ajouter » adds one for the carrier, kept by `save` (`{...args, object?,
   *  scope}`) when the node gives it; `unit` names one (« espace »). */
  scopes?: UIPickerScopes
  /** Search while typing, from two letters - ignored by a
   *  host showing the source model: there a search runs on Enter or the
   *  field's search icon only. */
  live?: boolean
  /** Shown before the first search: what to type. */
  hint?: string
  /** A declared badge the selection can be rid of at once: « dont N IA »
   *  and « Retirer les IA ». */
  removeBadge?: string
  /** Each item of « Sélection » is ticked or unticked in place - an
   *  unticked one struck through, kept, never handed back - with its why
   *  line, and « Tout cocher » per group: the picker of
   *  proposed items. */
  tickable?: boolean
  /** The proposal region (mock-up v6), tickable only: ONE
   *  column - a head titled `title` with the producer's `sub` node and «
   *  Tout cocher » while a proposal is unticked, one row per proposed item
   *  (`selected`: the check box at the left, its slots, detail « · » why,
   *  badges at the right; `none` when none), the search below whose hits
   *  are ticked the same way, every chosen item as a removable chip. A
   *  check-box tree (`layout: 'tree'`) instead draws `proposals.items` above
   *  its search, each row ticked as the tree ticks it, then `choose`. */
  proposals?: UIPickerProposals
  /** The save bar, with `proposals`: « N <one|many> »
   *  counting the ticked items; while they differ from the choice held «
   *  Annuler » (`cancel`) and the apply button (`save`, « Enregistrer et lire
   *  »), else `idle` (bound by the producer) or `empty` when nothing is
   *  ticked. A successful apply makes the choice the one held. */
  savebar?: UIPickerSavebar
  /** The keys of the items already held: checked and disabled where the
   *  picker lists them (its tree, its matches), « déjà retenue ».
 */
  held?: string[]
  /** The object the methods are for (templated). */
  object?: string
  /** Laid over every call's input - what the methods need beside the object
   *  and the query (a connector's name). */
  args?: Record<string, unknown>
  /** How « Sélection » shows: a list (default) or a tree by parent. */
  layout?: 'list' | 'tree'
  /** `single`: one item chosen at a time - picking one,
   *  in the tree browsed level by level or among the matches, replaces the
   *  one held, never with what sits under it; `apply` is handed that one.
   *  Default `multiple`. */
  choice?: 'single' | 'multiple'
  /** The badges its items may carry, by name. */
  badges?: Record<string, UIPickerBadge>
  /** Its source's item kinds: each kind's label and the slots its cards
   *  draw. */
  kinds?: UIItemKinds
  /** The items selected before (bind with `dataField`). */
  selected?: UIPickerItem[]
  /** The words it shows, overriding the defaults. */
  labels?: Partial<Record<'search' | 'selection' | 'remove' | 'add' | 'choose' | 'apply' | 'none' | 'empty' | 'unreachable' | 'noHost' | 'browse' | 'top' | 'withChildren' | 'self' | 'included' | 'treeHelp' | 'treeHelpWith' | 'treeHelpIncluded' | 'treeHelpMixed' | 'treeNotice' | 'notConfigured' | 'notReachable' | 'searchTitle' | 'removeAll' | 'of' | 'read' | 'waiting' | 'removed' | 'failed' | 'tickAll' | 'untickAll' | 'kept' | 'held' | 'retry' | 'untickBadge' | 'results' | 'alreadyProposed' | 'noneChosen' | 'cancel' | 'saveRead', string>>
  /** Height in pixels of the matches (default 260). */
  height?: number
}

/** An option of a select or of a menu's choice: the value its action gets as
 *  `${value}`, and what it shows (the value when absent). */
export interface UIChoiceOption {
  value: string
  label?: string
}

/** Where a choice takes its options: `options` inline, or `optionsField` — a
 *  path in the element (a row's own options) or else in the page's data
 *  holding `[{value, label}]`. */
export interface UIChoiceSource {
  options?: UIChoiceOption[]
  optionsField?: string
}

/** A chip of a chip group: its value (the selection key's value it sets),
 *  its label, its count, a palette token or CSS colour for its dot. */
export interface UIChipOption { value: string; label?: string; count?: number; color?: string }

/** A chip group: a click on a chip sets the selection's `key`
 *  to its value, the chip of the current value pressed. `all` names a first
 *  chip clearing the key (every row); `empty` is the value a « none » chip
 *  carries (a filter clause with the same `empty` keeps the empty rows). A
 *  chip shows its `count`, else the rows of `countRows` (bind with
 *  `dataField`) whose `countField` is its value. */
export interface UIChipsNode extends UIPageNodeBase {
  kind: 'chips'
  key: string
  options?: UIChipOption[]
  all?: string
  empty?: string
  countRows?: Array<Record<string, unknown>>
  countField?: string
  /** Its accessible name. */
  label?: string
}

/** A search box: what is typed sets the selection's `key`
 *  (after a short pause), so a `contains` filter clause narrows the rows. */
export interface UISearchNode extends UIPageNodeBase {
  kind: 'search'
  key: string
  placeholder?: string
  label?: string
  /** Its width in pixels. Default: 220. */
  width?: number
}

/** The terms a search was run with: each an editable chip -
 *  « × » removes it, the box beside adds one -, and « Chercher encore » runs
 *  `action` with `${terms}` the terms shown (an array). */
export interface UITermsNode extends UIPageNodeBase {
  kind: 'terms'
  /** The terms the search was run with (bind with `dataField`). */
  terms?: string[]
  /** The method run again with the terms: its args read `${terms}`. */
  action: UIPageAction
  /** Above the chips. Default « Recherche faite avec ». */
  title?: string
  /** Under the chips: where the terms come from. */
  note?: string
  /** Shown, never run. */
  disabled?: boolean
  /** The words it shows, overriding the defaults. */
  labels?: Partial<Record<'again' | 'add' | 'remove' | 'none' | 'replaced', string>>
}

/** addTerm adds a term once (case and spaces aside); an empty one is never
 *  added. */
export function addTerm(terms: string[], term: string): string[] {
  const t = term.trim().replace(/\s+/g, ' ')
  if (!t || terms.some((x) => x.toLowerCase() === t.toLowerCase())) return terms
  return [...terms, t]
}

/** uniqueTerms is a list of terms as the node shows them: each once (case
 *  and spaces aside), never empty - so each chip is its own. */
export function uniqueTerms(terms: unknown[] | undefined): string[] {
  return (terms ?? []).reduce<string[]>((acc, t) => addTerm(acc, String(t ?? '')), [])
}

/** termsToRun is what « Chercher encore » runs with: the terms shown and the
 *  one being typed, not yet confirmed with Enter. */
export function termsToRun(terms: string[], typed: string): string[] {
  return addTerm(terms, typed)
}

/** Where a terms node stands: the chips, the bound terms
 *  they came from (as a key), an edit not run, an edit the producer
 *  replaced, a run waiting for the producer's answer. */
export interface TermsState {
  terms: string[]
  bound: string
  edited: boolean
  replaced: boolean
  busy: boolean
}

/** What moves a terms node: the producer's terms (bound), an edit, a run,
 *  the run's wait running out. */
export type TermsMove =
  | { type: 'bound'; terms: unknown[] | undefined }
  | { type: 'edit'; terms: string[] }
  | { type: 'run'; terms: string[]; wait?: boolean }
  | { type: 'timeout' }

const boundKey = (terms: string[]) => JSON.stringify(terms.map((t) => t.toLowerCase()))

/** termsInit is a terms node as first drawn: its bound terms. */
export function termsInit(bound: unknown[] | undefined): TermsState {
  const terms = uniqueTerms(bound)
  return { terms, bound: boundKey(terms), edited: false, replaced: false, busy: false }
}

/** termsReduce moves a terms node. A run waits (`wait` false: never - an
 *  action the page runs itself). The producer has answered only when the
 *  bound terms differ from those the chips came from: then they replace the
 *  chips, end a run, and say an edit not run was replaced. The same terms
 *  sent again change nothing - a run then ends at its timeout. A run or an
 *  edit while a run waits is refused. */
export function termsReduce(s: TermsState, m: TermsMove): TermsState {
  switch (m.type) {
    case 'bound': {
      const terms = uniqueTerms(m.terms)
      const key = boundKey(terms)
      if (key === s.bound) return s
      return { terms, bound: key, edited: false, replaced: s.edited && !s.busy, busy: false }
    }
    case 'edit':
      return s.busy ? s : { ...s, terms: m.terms, edited: true, replaced: false }
    case 'run':
      // A run the page runs itself (a select, a tab) has no producer to wait
      // for: its terms are set at once.
      if (s.busy || m.terms.length === 0) return s
      return { ...s, terms: m.terms, edited: false, replaced: false, busy: m.wait !== false }
    case 'timeout':
      return s.busy ? { ...s, busy: false } : s
  }
}

/** removeTerm removes a term. */
export function removeTerm(terms: string[], term: string): string[] {
  return terms.filter((x) => x !== term)
}

/** termsEvent is what « Chercher encore » sends the host: the action with
 *  `${terms}` the terms shown. */
export function termsEvent(action: UIPageAction, item: unknown, terms: string[]): UIPageActionEvent {
  return { action, item, extra: { terms: [...terms] } }
}

/** One stage of a run, as a progress list shows it. */
export interface UIProgressStage {
  id: string
  label: string
  /** What is done of it, out of `total` (the bar and « 3/7 »). */
  done?: number
  total?: number
  state?: 'waiting' | 'running' | 'done' | 'failed' | 'skipped'
  /** Why it failed, or what it is doing. */
  message?: string
  /** How long it took, in milliseconds, once ended. */
  ms?: number
}

/** A live progress list: one row per stage of a run - its
 *  bar, its count, its tone (running, success, failure) -, updated as the
 *  run reports: `poll` calls the run's progress method through the host
 *  until the run is over, the rows following each answer, never the panel
 *  read again. */
export interface UIProgressListNode extends UIPageNodeBase {
  kind: 'progress-list'
  /** The stages as the page was read (bind with `dataField`). */
  stages?: UIProgressStage[]
  /** The run's progress method: called with `args` every `every` ms
   *  (default 1000); its answer holds the stages at `field` (default
   *  `stages`) and `done: true` once the run is over. */
  poll?: { uri: string; args?: Record<string, unknown>; field?: string; every?: number }
  title?: string
  /** Shown while no stage is known. */
  empty?: string
  /** `cards` (the default): a card and a bar per stage; `rows`: one bordered
   *  list, each stage its label and its state in words - the generic steps
   *  panel type. */
  layout?: 'cards' | 'rows'
  /** One bar above the stages for the stages ended, « 2 / 5 » under it. */
  bar?: boolean
  /** What the stages are (« pages », « programmes »), after « 2 / 5 ». */
  unit?: string
  /** One more line per source its reading syncs, after the
   *  stages: followed from the `source.sync` events the sync raises on each
   *  source through the host's `followEvents` - its count
   *  n/m while it reads, then done, or failed with why. `sources` names the
   *  sources the run reads, a waiting line each from the start (any source
   *  then ignored); without it a line shows for each source whose sync
   *  starts while the list is shown. `labels` their names by id;
   *  `states` where a source already stands when the list is drawn (its
   *  reading ended: done, or failed with why, its count), its line starting
   *  there instead of waiting. */
  follow?: UIProgressFollow
}

/** What a progress list follows besides its stages. */
export interface UIProgressFollow {
  event: 'source.sync'
  sources?: string[]
  labels?: Record<string, string>
  states?: Record<string, Pick<UIProgressStage, 'state' | 'done' | 'total' | 'message'>>
}

/** A `source.sync` event's payload: where a source's sync
 *  stands - started, items read of total, then failed with why or ended. */
export interface SourceSyncProgress {
  source: string
  phase: string
  read?: number
  total?: number
  error?: string
}

/** sourceLines are a followed run's lines before any event: a line per
 *  source it names, waiting, or where its `states` says the
 *  source stands. */
export function sourceLines(follow: UIProgressFollow | undefined): UIProgressStage[] {
  return (follow?.sources ?? []).map((id) => ({ id, label: follow?.labels?.[id] ?? id, state: 'waiting', ...follow?.states?.[id] }))
}

/** applySourceSync folds one `source.sync` event into a run's per-source
 *  lines: the source's line - added at its first event -
 *  running with its count while it reads, then done, or failed with its
 *  why. An event of a source the follow does not name, or one without a
 *  source, changes nothing; a count arriving after its line ended is late
 *  and dropped, a new `started` reads the source again. */
export function applySourceSync(lines: UIProgressStage[], ev: unknown, follow?: UIProgressFollow): UIProgressStage[] {
  if (!ev || typeof ev !== 'object') return lines
  const p = ev as Partial<SourceSyncProgress>
  if (typeof p.source !== 'string' || p.source === '') return lines
  if (follow?.sources && !follow.sources.includes(p.source)) return lines
  const at = lines.findIndex((l) => l.id === p.source)
  const was = at >= 0 ? lines[at] : undefined
  const ended = was?.state === 'done' || was?.state === 'failed'
  const count = { done: p.read ?? 0, ...(p.total ? { total: p.total } : {}) }
  const base = { id: p.source, label: follow?.labels?.[p.source] ?? was?.label ?? p.source }
  let line: UIProgressStage
  switch (p.phase) {
    case 'started':
      line = { ...base, ...count, state: 'running' }
      break
    case 'items':
      if (ended) return lines
      line = { ...base, ...count, state: 'running' }
      break
    case 'failed':
      line = { ...base, ...count, state: 'failed', message: p.error || 'La lecture a échoué.' }
      break
    case 'ended':
      line = { ...base, ...count, state: 'done' }
      break
    default:
      return lines
  }
  return at >= 0 ? lines.map((l, i) => (i === at ? line : l)) : [...lines, line]
}

/** progressRow is a stage drawn: its state (said, else read from its
 *  count), its percent, its count text and its tone. */
export function progressRow(s: UIProgressStage): { state: NonNullable<UIProgressStage['state']>; percent: number; count: string; tone: 'gray' | 'blue' | 'teal' | 'red' } {
  const total = s.total ?? 0
  const done = Math.min(s.done ?? 0, total || (s.done ?? 0))
  const state = s.state ?? (total > 0 && done >= total ? 'done' : done > 0 ? 'running' : 'waiting')
  const percent = state === 'done' ? 100 : total > 0 ? Math.round((done / total) * 100) : 0
  const tone = state === 'failed' ? 'red' : state === 'done' ? 'teal' : state === 'running' ? 'blue' : 'gray' // waiting, skipped
  return { state, percent, count: total > 0 ? `${done}/${total}` : '', tone }
}

/** progressOver says whether a run is over: its answer says so, or every
 *  stage ended (done or failed). */
export function progressOver(answer: unknown, stages: UIProgressStage[]): boolean {
  if (answer && typeof answer === 'object' && (answer as Record<string, unknown>).done === true) return true
  return stages.length > 0 && stages.every((s) => s.state === 'done' || s.state === 'failed' || s.state === 'skipped')
}

/** progressEnded counts the stages ended (done or skipped) - the steps
 *  panel's bar, « 2 / 5 ». */
export function progressEnded(stages: readonly UIProgressStage[]): { ended: number; total: number } {
  return { ended: stages.filter((s) => s.state === 'done' || s.state === 'skipped').length, total: stages.length }
}

/** pollProgress asks the run's progress method once: the
 *  stages its answer holds and whether the run is over; a call that fails
 *  keeps the stages and says why. */
export async function pollProgress(call: (uri: string, input?: unknown) => Promise<unknown>, poll: NonNullable<UIProgressListNode['poll']>, stages: UIProgressStage[]): Promise<{ stages: UIProgressStage[]; over: boolean; error?: string }> {
  try {
    const answer = await call(poll.uri, poll.args ?? {})
    const got = resolveJSONPath(answer, poll.field ?? 'stages')
    const next = Array.isArray(got) ? got as UIProgressStage[] : stages
    return { stages: next, over: progressOver(answer, next) }
  } catch (e) {
    return { stages, over: false, error: e instanceof Error ? e.message : String(e) }
  }
}

/** One part of an action bar's summary: a count read live
 *  from the page's working data, or an estimate made from one. */
export interface UIActionBarPart {
  /** Its name: the bar's buttons read its value as `${item.<id>}`, and their
   *  `when` / `disabledWhen` name it. */
  id: string
  /** Where it counts, in the page's working data (what pickers, ticks and
   *  forms write): an array counts its elements, a number is taken as is,
   *  an object its keys. */
  count: string
  /** Counts only the elements whose fields hold these values. */
  where?: Record<string, unknown>
  /** Its text: `{n}` is the value, put first when the text names none
   *  (« éléments retenus » → « 3 éléments retenus »). */
  label: string
  /** An estimate: the count times `per`, rounded, at least `min`
   *  (« environ {n} min »). */
  per?: number
  min?: number
  /** The value shown bold. */
  strong?: boolean
  /** Left out of the summary while its value is 0. */
  hideZero?: boolean
  /** While its value is 0, why the bar's buttons are not available: they read
   *  it through `disabledWhen: missing`. */
  required?: string
}

/** A sticky bar at the bottom of a page: a one-line summary
 *  computed live from the other nodes' state, and its buttons, the last the
 *  main one. Each button runs with the summary's values as its element. */
export interface UIActionBarNode extends UIPageNodeBase {
  kind: 'action-bar'
  parts: UIActionBarPart[]
  actions?: UIPageAction[]
  /** Shown only while this part's value is not 0. */
  showWhen?: string
  /** Kept in view at the bottom while the page scrolls. Default true. */
  sticky?: boolean
}

/** barCount is what a part counts in the page's data. */
export function barCount(data: unknown, part: Pick<UIActionBarPart, 'count' | 'where'>): number {
  const v = resolveJSONPath(data, part.count)
  if (typeof v === 'number' && Number.isFinite(v)) return v
  if (Array.isArray(v)) {
    const where = Object.entries(part.where ?? {})
    if (!where.length) return v.length
    return v.filter((e) => e && typeof e === 'object' && where.every(([k, want]) => (e as Record<string, unknown>)[k] === want)).length
  }
  if (v && typeof v === 'object') return Object.keys(v).length
  return 0
}

/** barValue is a part's value: its count, or the estimate made from it. */
export function barValue(data: unknown, part: UIActionBarPart): number {
  const n = barCount(data, part)
  if (part.per === undefined) return n
  return Math.max(part.min ?? 0, Math.round(n * part.per))
}

/** barSummary is what the bar shows and hands its buttons: each part's value
 *  by id, `missing` the first required part's reason while its value is 0,
 *  and the parts shown with their text split around the value. */
export function barSummary(node: Pick<UIActionBarNode, 'parts'>, data: unknown): {
  values: Record<string, unknown>
  shown: { id: string; before: string; value: number; after: string; strong: boolean }[]
} {
  const values: Record<string, unknown> = {}
  const shown: { id: string; before: string; value: number; after: string; strong: boolean }[] = []
  for (const p of node.parts) {
    const value = barValue(data, p)
    values[p.id] = value
    if (value === 0 && p.required && values.missing === undefined) values.missing = p.required
    if (value === 0 && p.hideZero) continue
    const at = p.label.indexOf('{n}')
    const [before, after] = at < 0 ? ['', ' ' + p.label] : [p.label.slice(0, at), p.label.slice(at + 3)]
    shown.push({ id: p.id, before, value, after, strong: !!p.strong })
  }
  return { values, shown }
}

/** One option of a set of choice cards. */
export interface UIChoiceCard {
  /** The value the selection's key takes when the card is chosen. */
  value: string
  label?: string
  /** A dimmed line under the label. */
  description?: string
  /** The badge's colour (a Mantine colour, `teal`, `blue.6`); gray by default. */
  color?: string
  /** The badge's letters (« LX »); the label's initials when absent. */
  badge?: string
  /** A Tabler icon drawn in the badge in place of its letters. */
  icon?: string
  /** The card cannot be chosen: drawn dimmed, its `pill` beside its label. */
  disabled?: boolean
  /** A short word beside a disabled card's label (« déjà dans l'étude »). */
  pill?: string
  /** Why, shown on hover. */
  tooltip?: string
  /** Other selection keys the card sets when chosen (`{name: 'LeanIX'}`: the
   *  Nom prefilled) - each only while the person has not changed it: unset,
   *  or still what the card chosen before set. */
  sets?: Record<string, string>
}

/** A set of choice cards: one card per option, one chosen at
 *  a time - a radio group - the chosen card setting the page selection's
 *  `key` to its value; a disabled card cannot be chosen. */
export interface UIChoiceCardsNode extends UIPageNodeBase {
  kind: 'choice-cards'
  key: string
  /** The group's accessible name. */
  label?: string
  /** The options (bind with `dataField`). */
  options?: UIChoiceCard[]
  /** Cards per row; 2 by default. */
  cols?: number
}

/** A labelled text field setting the page selection's `key`
 *  to what is typed. Its starting `value` (bind with `dataField`) is the
 *  key's while the selection holds none. `when` - `key=value`, or `key`
 *  set - draws it only while the selection holds it (Confluence's « Espace »
 *  only when the choice is Confluence). */
export interface UITextFieldNode extends UIPageNodeBase {
  kind: 'text-field'
  key: string
  label: string
  placeholder?: string
  value?: string
  when?: string
  /** Marked required (an asterisk); a button's `requires` holds it. */
  required?: boolean
  description?: string
  /** The values it may take: drawn as a drop-down of them instead of free
   *  text (Confluence's « Espace »: the spaces joined in its connection). */
  options?: { value: string; label?: string }[]
  /** Multi-line: a text area showing this many lines,
   *  growing with what is typed - a remark box. */
  rows?: number
}

/** A switch: sets the selection's `key` to true or false.
 *  With `action` it is an action switch instead: drawn at `checked`, a toggle
 *  runs `action` on the element it is on with `${value}` the new state
 *  (`true` / `false`) - a choice its producer keeps (a section's « À écrire »). */
export interface UISwitchNode extends UIPageNodeBase {
  kind: 'switch'
  key: string
  label: string
  checked?: boolean
  action?: UIPageAction
}

/** An inline select: a compact drop-down whose choice runs
 *  `action` at once, with `${value}` the option chosen and `${item.*}` the
 *  element it is on (a row). Shown disabled when the action cannot run. */
export interface UISelectNode extends UIPageNodeBase, UIChoiceSource {
  kind: 'select'
  label?: string
  /** The value shown (bind with `dataField`). */
  value?: string
  placeholder?: string
  size?: 'xs' | 'sm'
  action: UIPageAction
}

/** An entry of an action menu: a plain entry runs its action on the element;
 *  a CHOICE entry lists its options and runs its action with `${value}` the
 *  option chosen; a PROMPT entry first asks a short text (its `prompt` the
 *  question) and runs its action with `${value}` that text — an empty text
 *  runs nothing. `choice` is `{options}` / `{optionsField}`, or a string:
 *  `P0|P1|P2` the options themselves (value = label), a word with no `|` the
 *  element's field holding them. `confirm` / `dangerous` / `when` act as on
 *  an action. */
export interface UIMenuItem {
  id: string
  label: string
  icon?: string
  action?: UIPageAction
  confirm?: string
  dangerous?: boolean
  when?: string
  prompt?: string
  choice?: UIChoiceSource | string
  /** In a row's menu: drawn as its own small button in the row (a choice
   *  « <label> ▾ » opening its options) rather than under ⋯. */
  button?: boolean
  /** In an action bar: the element's main action, drawn filled. */
  primary?: boolean
  /** In an action bar: drawn under its ⋯ rather than inline. */
  menu?: boolean
}

/** An action menu: a small button opening its entries. */
export interface UIMenuNode extends UIPageNodeBase {
  kind: 'menu'
  /** The button's text (`⋯` when absent). */
  label?: string
  /** Tabler icon name. */
  icon?: string
  variant?: 'filled' | 'light' | 'outline' | 'subtle' | 'default'
  /** The entries (bind with `dataField`). */
  items: UIMenuItem[]
}

/** A mark on an object card: a CWM badge as the row carries it - the label,
 *  its tabler icon, its colour, its tooltip. */
export interface UIObjectCardBadge {
  id?: string
  label: string
  icon?: string
  color?: string
  tooltip?: string
}

/** The row an object card is filled from: what CWM answers
 *  for an object - its title, id and type, its lifecycle state, the badges
 *  its type and capabilities declare, its one next action and the entries of
 *  its « ⋯ » menu. A producer passes this row, never nodes of its own. */
export interface UIObjectCardRow {
  id: string
  title: string
  type?: string
  /** A tabler icon of its type. */
  icon?: string
  state?: UIObjectCardBadge
  badges?: UIObjectCardBadge[]
  /** Its one next action, drawn as the card's button. */
  next?: UIMenuItem
  /** The entries of its « ⋯ » menu. */
  menu?: UIMenuItem[]
}

/** The one object card: every object drawn
 *  as a card - a holder's children, a study's sources - is this node filled
 *  from its CWM row; its actions run on the row. */
export interface UIObjectCardNode extends UIPageNodeBase {
  kind: 'object-card'
  /** The object's row (bind with `dataField`, `$.item` in a list). */
  row?: UIObjectCardRow
  /** A click on the card runs it on the row (open the object). */
  open?: UIPageAction
}

/** objectCardRow is the row a card draws: a row with an id and a title, else
 *  undefined - a card is never drawn from a partial row. */
export function objectCardRow(value: unknown): UIObjectCardRow | undefined {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return undefined
  const r = value as Record<string, unknown>
  if (typeof r.id !== 'string' || r.id === '') return undefined
  const title = typeof r.title === 'string' && r.title !== '' ? r.title : r.id
  return { ...(r as object), id: r.id, title } as UIObjectCardRow
}

/** choiceOptions are the options a choice offers: its inline `options`, else
 *  the array at `optionsField` in the element, else in the page's data (a
 *  leading `$.` is allowed). A choice given as a string is `a|b|c` the
 *  options themselves, or a field name (no `|`). A string entry is an option
 *  of that value; an entry with no value is dropped. */
export function choiceOptions(choice: UIChoiceSource | string | undefined, item?: unknown, data?: unknown): UIChoiceOption[] {
  if (typeof choice === 'string') {
    if (!choice.includes('|')) return choice.trim() ? choiceOptions({ optionsField: choice.trim() }, item, data) : []
    choice = { options: choice.split('|').map((s) => s.trim()).filter(Boolean).map((v) => ({ value: v, label: v })) }
  }
  if (!choice) return []
  let raw: unknown = choice.options
  if (raw === undefined && choice.optionsField) {
    const path = choice.optionsField.replace(/^\$\.?/, '')
    raw = pathOf(item, path)
    if (!Array.isArray(raw)) raw = pathOf(data, path)
  }
  if (!Array.isArray(raw)) return []
  return raw.flatMap((o): UIChoiceOption[] => {
    if (typeof o === 'string' || typeof o === 'number') return [{ value: String(o), label: String(o) }]
    if (!o || typeof o !== 'object') return []
    const r = o as Record<string, unknown>
    if (r.value === undefined || r.value === null || r.value === '') return []
    return [{ value: String(r.value), label: r.label === undefined || r.label === null ? String(r.value) : String(r.label) }]
  })
}

/** choiceEvent is what choosing an option sends the host: the action, the
 *  element it is on, and the option as `value` (`${value}` in its args). */
export function choiceEvent(action: UIPageAction, item: unknown, value: string): UIPageActionEvent {
  return { action, item, extra: { value } }
}

/** promptEvent is what answering a prompt entry sends the host: the action
 *  with `${value}` the text typed (trimmed), or undefined for an empty text —
 *  nothing runs. */
export function promptEvent(action: UIPageAction, item: unknown, text: string): UIPageActionEvent | undefined {
  const t = text.trim()
  return t ? choiceEvent(action, item, t) : undefined
}

/** menuItemAction is the action a menu entry runs: its `action`, or the
 *  entry itself when it names a verb; its entry's `confirm` and `dangerous`
 *  laid over; undefined when it has none. */
export function menuItemAction(entry: UIMenuItem): UIPageAction | undefined {
  // An entry with no `action` may carry its verb itself (a ui-action:
  // `{id, label, uri, args}`).
  const own = entry.action ?? (actionVerb(entry as UIPageAction) ? (() => {
    const { choice: _c, prompt: _p, ...rest } = entry as UIMenuItem & UIPageAction
    void _c; void _p
    return rest as UIPageAction
  })() : undefined)
  if (!own) return undefined
  const out: UIPageAction = { ...own }
  if (entry.confirm !== undefined) out.confirm = entry.confirm
  if (entry.dangerous !== undefined) out.dangerous = entry.dangerous
  return out
}

/** A step of a stepper. */
export interface UIStepperStep {
  id: string
  label: string
  description?: string
  /** The step's own state (a planning status word); else done before the
   *  current step, running at it, todo after. */
  status?: string
  /** A path in the page's data whose value is the step's label once set
   *  (« Sources proposées (12) »). */
  labelField?: string
}

/** A stepper: the states of a workflow in order, the steps
 *  before `current` done, `current` active; `action` runs on a click on a
 *  step, `${item.id}` the step. An object's header shows its workflow with
 *  it. */
export interface UIStepperNode extends UIPageNodeBase {
  kind: 'stepper'
  steps: UIStepperStep[]
  /** The current step's id (bind with `dataField`); none: nothing done yet. */
  current?: string
  /** Every step done (the workflow ended). */
  completed?: boolean
  action?: UIPageAction
  size?: 'xs' | 'sm' | 'md' | 'lg' | 'xl'
  /** `dots`: a row of dots, each step's state in the shared status colours
   *  and its name on hover - small enough for a list row. A
   *  `dataField` bound to a list gives the steps, to the current step's id
   *  gives `current`. */
  variant?: 'steps' | 'dots'
}

/** A row of a board: its key (what selecting it sets), and what it shows. */
export interface UIBoardRow {
  key: string
  label?: string
  icon?: string
  /** A number or a short text at its right (`3`, `12 pages`). */
  count?: number | string
  /** Chips after its label: a text, or `{text, color}`. */
  chips?: Array<string | { text: string; color?: string }>
  /** The switch's state - drawn only when the board declares `toggle`. */
  on?: boolean
  // What a card shows (`layout: cards`): the colour of its
  // icon, its state, a line under its head, what it picks and a remark.
  /** The colour of its icon tile on a card (a Mantine colour). */
  color?: string
  /** Its state on a card - a text, or `{text, color}` (« à jour », « 2 erreurs »). */
  state?: string | { text: string; color?: string }
  /** A dimmed line under a card's head (« 12 pages lues sur 14 · il y a 1 h »). */
  meta?: string
  /** What it picks, by title: a card lists the first `itemsMax`, then « and N more ». */
  items?: string[]
  /** A remark on a card saying what to do (« Connectez-vous… puis « Actualiser » »). */
  note?: string
  /** The remark's colour (yellow by default; red for a failure). */
  noteColor?: string
  /** Its own open button's label on a card, in place of the board's
   *  `openLabel` (« Déposer… » on a card whose panel deposits). */
  openLabel?: string
}

/** A board: a list of items beside the selected item's panel.
 *  Each row shows its icon, label, count, chips and - with `toggle` - a
 *  switch; the head shows the title and how many rows; a click on a row sets
 *  the page selection's `select` key to its key (the first row selected when
 *  none is), and `panel` - any node, usually a section loading with
 *  `${selection.<select>}` - is drawn beside. Generic: the rows are data. */
export interface UIBoardNode extends UIPageNodeBase {
  kind: 'board'
  /** The rows (bind with `dataField`). */
  data?: UIBoardRow[]
  title?: string
  /** The selection key a click on a row sets. */
  select: string
  /** What is drawn beside the list for the selected row. */
  panel: UIPageNode
  /** Runs when a row's switch flips, on that row, with `${value}` the new
   *  state (`true` or `false`). */
  toggle?: UIPageAction
  /** Shown when there is no row. */
  empty?: string
  /** The list's width in pixels (280 by default); one column under 720px. */
  listWidth?: number
  /** `cards` lays the rows out as a grid of cards - each its
   *  icon, label, state, `meta` line, the first `itemsMax` of its `items`
   *  (« and N more »), its `note`, the `openLabel` button, its `actions` and the
   *  switch; opening a card sets the selection's `select` key to its key and
   *  draws `panel` in place of the grid, with `backLabel` coming back. Drawn as
   *  cards where the host shows the source model (IN-045's switch), else as
   *  the list. */
  layout?: 'list' | 'cards'
  /** How many items a card lists before « and N more » (5 by default). */
  itemsMax?: number
  /** The text after the items a card does not list, `{n}` their number
   *  (« and {n} more » by default). */
  moreLabel?: string
  /** The label of the button opening a card (« Open… » by default). */
  openLabel?: string
  /** The label of the way back from an opened card (« ← All » by default). */
  backLabel?: string
  /** The buttons of each card, run on its row (`${item.<path>}`). */
  actions?: UIPageAction[]
}

/** boardCardItems is what a card lists of its items: the first `max` (5 by
 *  default), and how many it leaves out. */
export function boardCardItems(items: unknown, max = 5): { shown: string[]; more: number } {
  const all = Array.isArray(items) ? items.filter((x): x is string => typeof x === 'string' && x !== '') : []
  const n = max > 0 ? max : 5
  return { shown: all.slice(0, n), more: Math.max(0, all.length - n) }
}

/** boardMoreText is the text after a card's items, `{n}` the ones left out. */
export function boardMoreText(more: number, label?: string): string {
  return (label || 'and {n} more').replace('{n}', String(more))
}

/** boardRows keeps the rows a board can draw: objects with a key. */
export function boardRows(data: unknown): UIBoardRow[] {
  if (!Array.isArray(data)) return []
  return data.filter((r): r is UIBoardRow => !!r && typeof r === 'object' && typeof (r as UIBoardRow).key === 'string' && (r as UIBoardRow).key !== '')
}

/** boardSelected is the row a board shows selected: the selection's value
 *  for its key when a row holds it, else the first row's key; undefined with
 *  no row. */
export function boardSelected(rows: UIBoardRow[], selection: Record<string, unknown>, key: string): string | undefined {
  const v = selection[key]
  if (v !== undefined && v !== null && rows.some((r) => r.key === String(v))) return String(v)
  return rows[0]?.key
}

/** boardToggleEvent is what flipping a row's switch sends: the board's toggle
 *  action on the row, `${value}` the new state. */
export function boardToggleEvent(action: UIPageAction, row: UIBoardRow): UIPageActionEvent {
  return { action, item: row, extra: { value: row.on ? 'false' : 'true' } }
}

/** A row of a grid board: its id (what a card's row field holds) and title. */
export interface UIGridRow {
  id: string
  title: string
  /** A line under the title (a stream's mode and next release). */
  meta?: string
}

/** A column of a grid board. A column with `prompt` asks a short text in the
 *  card before a card moves into it - « Why is it on hold? » - and its move
 *  runs with `${value}` that text; its own `move` replaces the board's. */
export interface UIGridColumn {
  id: string
  title: string
  color?: string
  prompt?: string
  move?: UIPageAction
}

/** A grid board: cards in the cells of rows by one field and
 *  columns by another - counts per row and per column; dropping a card in
 *  another column of its row runs `move` with `${row}` / `${column}` the
 *  target cell (and `${value}` a prompted column's text); a drop in another
 *  row is refused with `crossRow`; Alt+Left / Alt+Right moves a focused card
 *  to the column beside. Generic: rows, columns and cards are data. */
export interface UIGridBoardNode extends UIPageNodeBase {
  kind: 'grid-board'
  /** Items drawn per page on a progressive page: the first
   *  page, then « Afficher plus » adds the next one. Default 50. */
  pageSize?: number
  rows: UIGridRow[]
  columns: UIGridColumn[]
  /** The cards (bind with `dataField`). */
  data?: Array<Record<string, unknown>>
  /** The card field naming its row (default `row`). */
  rowField?: string
  /** The card field naming its column (default `column`). */
  columnField?: string
  /** What each card shows: any node, `$.item` the card; its title otherwise. */
  item?: UIPageNode
  titleField?: string
  move?: UIPageAction
  open?: UIPageAction
  actions?: UIPageAction[]
  /** Said when a card is dropped in another row (refused). */
  crossRow?: string
  /** Minimum width of a column, in pixels (default 180). */
  columnWidth?: number
}

/** gridCell is a cell's cards: those whose row and column fields hold the
 *  cell's ids. */
export function gridCell(node: Pick<UIGridBoardNode, 'data' | 'rowField' | 'columnField'>, row: string, column: string): Array<Record<string, unknown>> {
  const rf = node.rowField ?? 'row'
  const cf = node.columnField ?? 'column'
  return (node.data ?? []).filter((c) => String(c[rf]) === row && String(c[cf]) === column)
}

/** gridCounts are how many cards each row and each column holds. */
export function gridCounts(node: Pick<UIGridBoardNode, 'data' | 'rowField' | 'columnField'>): { rows: Record<string, number>; columns: Record<string, number> } {
  const rf = node.rowField ?? 'row'
  const cf = node.columnField ?? 'column'
  const rows: Record<string, number> = {}
  const columns: Record<string, number> = {}
  for (const c of node.data ?? []) {
    rows[String(c[rf])] = (rows[String(c[rf])] ?? 0) + 1
    columns[String(c[cf])] = (columns[String(c[cf])] ?? 0) + 1
  }
  return { rows, columns }
}

/** What a move of a card to a cell does: run its event, ask the column's
 *  prompt first, be refused saying why, or nothing (the same cell, or no
 *  move declared). */
export type GridMove =
  | { run: UIPageActionEvent }
  | { prompt: string; column: string }
  | { refused: string }
  | { none: true }

/** gridMove decides a card's move to a cell; `text` is a prompted column's
 *  answer (the move then runs with it). */
export function gridMove(node: UIGridBoardNode, card: Record<string, unknown>, row: string, column: string, text?: string): GridMove {
  const rf = node.rowField ?? 'row'
  const cf = node.columnField ?? 'column'
  if (String(card[rf]) !== row) return { refused: node.crossRow ?? 'A card moves within its own row only.' }
  if (String(card[cf]) === column) return { none: true }
  const col = node.columns.find((c) => c.id === column)
  const action = col?.move ?? node.move
  if (!col || !action) return { none: true }
  if (col.prompt && text === undefined) return { prompt: col.prompt, column }
  const extra: Record<string, unknown> = { row, column }
  if (col.prompt) {
    const t = (text ?? '').trim()
    if (!t) return { none: true }
    extra.value = t
  }
  return { run: { action, item: card, extra } }
}

/** gridKeyTarget is the column Alt+Left (-1) / Alt+Right (+1) moves a card
 *  to: the one beside its own, or undefined at an edge. */
export function gridKeyTarget(node: Pick<UIGridBoardNode, 'columns' | 'columnField'>, card: Record<string, unknown>, dir: -1 | 1): string | undefined {
  const i = node.columns.findIndex((c) => c.id === String(card[node.columnField ?? 'column']))
  if (i < 0) return undefined
  return node.columns[i + dir]?.id
}

/** A display panel embedded by its id: the screen follows
 *  it through the host (HostServices.followPanel) and draws it as its events
 *  build it - its page, its progress, its question, its end -, with « Ouvrir
 *  dans un onglet » (HostServices.openPanel). */
export interface UIPanelNode extends UIPageNodeBase {
  kind: 'panel'
  /** The panel's id: dp- and its UUID. */
  panelId: string
  /** Shown until the panel's open gives its own. */
  title?: string
  /** Its page alone, with no frame, title or controls of its own: a
   *  question's card placed in a conversation. */
  bare?: boolean
}

/** One source's answer in a multi-source search (sources.SearchSources'
 *  SourceSearch): where it stands and what it proposes. */
export interface UISourceAnswer {
  connector: string
  label?: string
  icon?: string
  color?: string
  /** searching, waiting, found, nothing-new, unreachable or not-configured. */
  state: string
  /** The connector it waits on, and that source's label. */
  waiting?: string
  waiting_label?: string
  found?: number
  held?: number
  items?: UIPickerItem[]
  /** Why it is unreachable or not configured. */
  message?: string
  errors?: string[]
  configure?: { label: string; hint?: string; keys?: string[] }
  /** Its item kinds as the source declares them, laid over
   *  the node's. */
  kinds?: UIItemKinds
}

/** A multi-source search panel: one group per source as it
 *  answers, laid over by the search's per-source states (`data.sources`, the
 *  merge op sources.SearchSources sends to the node whose id is
 *  `source-search`). Each found item says why, with a tick (ticked when
 *  proposed); « Tout cocher » per group; « Réessayer » on an unreachable one
 *  runs `retry` for that source alone; « Configurer » on one not configured;
 *  « Ajouter depuis <source>… » runs `add` on the group (that source's own
 *  panel in a modal). What is ticked is the page selection's `key`:
 *  `{selected: [{connector, items}], unreachable: [{connector, why}]}` - a
 *  form's submit reads it (`${selection.<key>.selected}`). When the values
 *  `watch` names change after a search, it says its proposals are out of
 *  date; the ticks are kept. */
export interface UISourceSearchNode extends UIPageNodeBase {
  kind: 'source-search'
  /** The search's answers as the producer lays them over the node. */
  data?: { sources?: UISourceAnswer[] }
  /** The page selection's key it hands what is ticked to. Default: its id,
   *  else `source-search`. */
  key?: string
  /** Run on one unreachable source (« Réessayer »): its item is the group
   *  (`${item.connector}`), its extras `{only}` the connector - the search
   *  run again for that source alone. */
  retry?: UIPageAction
  /** Run on a source not configured (« Configurer »), its item the group. */
  configure?: UIPageAction
  /** Run on a group (« Ajouter depuis <source>… »), its item the group:
   *  that source's own panel - a modal action. It wins over `pickers`. */
  add?: UIPageAction
  /** The method listing the connectors as pickers (`{}` -> `{connectors}`,
   *  sources' Pickers - the source-set field's `connectors`). With it and no
   *  `add`, « Ajouter depuis <source>… » shows on a group found or with
   *  nothing new whose connector it lists with a search or a browse, and
   *  opens that connector's generic picker in a modal: what is picked joins
   *  the group, ticked. */
  pickers?: string
  /** The page data paths the search ran with (`value.name`): when they
   *  change after a search, the proposals are out of date. Each path is read
   *  in the panel's page's data, else in the pages around it - a panel drawn
   *  in a loaded section watches the form of the page holding the section.
 */
  watch?: string[]
  /** Above the groups. Default « Sources proposées ». */
  title?: string
  /** Under the title. */
  note?: string
  /** Shown, never run. */
  disabled?: boolean
  /** The item kinds its sources' items are drawn with; a
   *  source's own laid over them. */
  kinds?: UIItemKinds
  /** The words it shows, overriding the defaults. */
  labels?: Partial<Record<'title' | 'searching' | 'waiting' | 'found' | 'nothingNew' | 'unreachable' | 'notConfigured' | 'retry' | 'configure' | 'all' | 'none' | 'add' | 'addConfirm' | 'cancel' | 'kept' | 'stale' | 'empty', string>>
}

/** One group of a multi-source search panel: a source's answer, the order
 *  it answered in and the keys of its items ticked. */
export interface SourceGroup {
  answer: UISourceAnswer
  /** The answer's order: 1 for the first source that answered; 0 while it
   *  searches or waits. */
  order: number
  ticked: string[]
}

/** Where a multi-source search panel stands: its groups by
 *  connector in the order they were first asked, the next answer order, the
 *  connectors a « Réessayer » runs again, and the watched values the last
 *  search ran with. */
export interface SourceSearchState {
  groups: SourceGroup[]
  next: number
  retrying: string[]
  ranWith?: string
}

/** What moves a multi-source search panel: the producer's answers (with the
 *  watched values as the page holds them now), a tick, « Tout cocher », a
 *  « Réessayer », the items picked through « Ajouter depuis <source>… ». */
export type SourceSearchEvent =
  | { type: 'answers'; sources: UISourceAnswer[] | undefined; watched?: string }
  | { type: 'tick'; connector: string; key: string; on: boolean }
  | { type: 'all'; connector: string; on: boolean }
  | { type: 'retry'; connector: string }
  | { type: 'add'; connector: string; items: UIPickerItem[] }

export const SOURCE_SEARCH_EMPTY: SourceSearchState = { groups: [], next: 1, retrying: [] }

/** sourceAnswered says whether a source has answered: any state but
 *  searching and waiting. */
export function sourceAnswered(a: UISourceAnswer): boolean {
  return a.state !== 'searching' && a.state !== 'waiting'
}

/** withChildrenOf is the item held with (or without) what sits under it. */
function withChildrenOf<T extends { with_children?: boolean }>(item: T, on: boolean | undefined): T {
  const { with_children: _, ...rest } = item
  return (on ? { ...rest, with_children: true } : rest) as T
}

/** sourceSearchReduce moves a multi-source search panel. Answers fold in by
 *  connector: a source they do not name keeps its group (a « Réessayer »
 *  answers its source alone). A new search records the watched values it
 *  ran with: every source named searching or waiting, not only those a
 *  « Réessayer » runs again - or a whole answer, every source named
 *  answered, while no group searches, waits or is run again (a section read
 *  again, « Chercher encore »). A source that answers takes its answer order;
 *  its new items are ticked (an unreachable item never), the ticks given
 *  before are kept, and an item ticked before that the answer leaves out
 *  stays, ticked. */
export function sourceSearchReduce(state: SourceSearchState, e: SourceSearchEvent): SourceSearchState {
  switch (e.type) {
    case 'answers': {
      const sources = (e.sources ?? []).filter((s) => s && typeof s.connector === 'string' && s.connector !== '')
      if (sources.length === 0) return state
      const pending = sources.filter((s) => !sourceAnswered(s))
      const whole = pending.length === 0 && state.retrying.length === 0 && state.groups.every((g) => sourceAnswered(g.answer))
      const fresh = whole || (pending.length === sources.length && !pending.every((s) => state.retrying.includes(s.connector)))
      let next = state.next
      const groups = [...state.groups]
      for (const s of sources) {
        const at = groups.findIndex((g) => g.answer.connector === s.connector)
        const before = at >= 0 ? groups[at] : undefined
        let g: SourceGroup
        if (!sourceAnswered(s)) {
          g = { answer: { ...s, items: before?.answer.items ?? [] }, order: 0, ticked: before?.ticked ?? [] }
        } else {
          const was = new Set(before?.ticked ?? [])
          // An item held « avec les sous-éléments » that the answer proposes
          // again keeps its sub-pages.
          const deep = new Set((before?.answer.items ?? []).filter((i) => i.with_children && was.has(i.key)).map((i) => i.key))
          const items = (s.items ?? []).map((i) => (deep.has(i.key) && !i.with_children ? withChildrenOf(i, true) : i))
          const shown = new Set(items.map((i) => i.key))
          const seen = new Set((before?.answer.items ?? []).map((i) => i.key))
          for (const i of before?.answer.items ?? []) {
            if (was.has(i.key) && !shown.has(i.key)) items.push(i)
          }
          const ticked = items.filter((i) => was.has(i.key) || (!seen.has(i.key) && !i.unreachable)).map((i) => i.key)
          g = { answer: { ...s, items }, order: before && before.order > 0 ? before.order : next++, ticked }
        }
        if (at >= 0) groups[at] = g
        else groups.push(g)
      }
      const retrying = state.retrying.filter((c) => !sources.some((s) => s.connector === c && sourceAnswered(s)))
      return { groups, next, retrying, ranWith: fresh ? e.watched : state.ranWith }
    }
    case 'tick':
    case 'all': {
      const groups = state.groups.map((g) => {
        if (g.answer.connector !== e.connector) return g
        const keys = e.type === 'all' ? (g.answer.items ?? []).filter((i) => !i.unreachable).map((i) => i.key) : [e.key]
        const set = new Set(g.ticked)
        for (const k of keys) {
          if (e.on) set.add(k)
          else set.delete(k)
        }
        return { ...g, ticked: (g.answer.items ?? []).map((i) => i.key).filter((k) => set.has(k)) }
      })
      return { ...state, groups }
    }
    case 'retry':
      return state.retrying.includes(e.connector) ? state : { ...state, retrying: [...state.retrying, e.connector] }
    case 'add': {
      // The items picked join their connector's group, ticked: one already
      // there is ticked, none twice, an unreachable one never; a later answer
      // keeps them ticked (an item ticked that an answer leaves out stays).
      // A pick of an item already there takes the pick's « avec les
      // sous-éléments ».
      const picked = (e.items ?? []).filter((i) => i && typeof i.key === 'string' && i.key !== '' && !i.unreachable)
      if (picked.length === 0) return state
      const groups = state.groups.map((g) => {
        if (g.answer.connector !== e.connector) return g
        const items = [...(g.answer.items ?? [])]
        const at = new Map(items.map((i, n) => [i.key, n]))
        for (const i of picked) {
          const n = at.get(i.key)
          if (n !== undefined) {
            if (!!items[n].with_children !== !!i.with_children) items[n] = withChildrenOf(items[n], i.with_children)
            continue
          }
          at.set(i.key, items.length)
          items.push(i)
        }
        const on = new Set([...g.ticked, ...picked.map((i) => i.key)])
        return { ...g, answer: { ...g.answer, items }, ticked: items.map((i) => i.key).filter((k) => on.has(k)) }
      })
      return { ...state, groups }
    }
  }
  return state
}

/** sourceGroupsShown are the groups as the panel draws them: those that
 *  answered in the order they answered, then those still searching or
 *  waiting in the order they were asked. */
export function sourceGroupsShown(state: SourceSearchState): SourceGroup[] {
  const answered = state.groups.filter((g) => g.order > 0).sort((a, b) => a.order - b.order)
  return [...answered, ...state.groups.filter((g) => g.order === 0)]
}

/** The value a multi-source search panel hands its page. */
export interface SourceSearchValue {
  selected: { connector: string; items: UIPickerItem[] }[]
  unreachable: { connector: string; why: string }[]
}

/** sourceSearchValue is what the panel hands its page: each source's ticked
 *  items as `selected: [{connector, items}]` (kept while it searches again; a
 *  source with none ticked left out), each unreachable source as `unreachable: [{connector,
 *  why}]`. */
export function sourceSearchValue(state: SourceSearchState): SourceSearchValue {
  const out: SourceSearchValue = { selected: [], unreachable: [] }
  for (const g of sourceGroupsShown(state)) {
    if (g.answer.state === 'unreachable') {
      out.unreachable.push({ connector: g.answer.connector, why: g.answer.message ?? '' })
      continue
    }
    const on = new Set(g.ticked)
    const items = (g.answer.items ?? []).filter((i) => on.has(i.key))
    if (items.length > 0) out.selected.push({ connector: g.answer.connector, items })
  }
  return out
}

/** watchedValues are the values a panel's `watch` paths hold in the page's
 *  data, as one comparable string; undefined when it watches none. A path
 *  the data does not hold is read in `around`, the data of the pages around
 *  it, nearest first. */
export function watchedValues(watch: string[] | undefined, data: unknown, around: unknown[] = []): string | undefined {
  if (!watch || watch.length === 0) return undefined
  const read = (p: string): unknown => {
    for (const d of [data, ...around]) {
      const v = pathOf(d, p)
      if (v !== undefined) return v
    }
    return null
  }
  return JSON.stringify(watch.map((p) => read(p) ?? null))
}

/** sourceSearchStale says whether the values the last search ran with
 *  changed since: the panel then says its proposals are out of date. */
export function sourceSearchStale(state: SourceSearchState, watched: string | undefined): boolean {
  return state.ranWith !== undefined && watched !== undefined && watched !== state.ranWith
}

/** One item opened (sources.SourceFetch): the item by its
 *  roles, its data and the data-type that shapes it, its markdown text -
 *  either or both -, and the objects it is used by when its producer says. */
export interface UIItemOpened {
  item?: UIPickerItem
  data?: Record<string, unknown>
  data_type?: string
  text?: string
  used?: UIItemUse[]
}

/** An object an item is used by: opened on a click. */
export interface UIItemUse {
  id: string
  name?: string
  type?: string
}

/** An item's detail: what its source's fetch opens - the item
 *  drawn by its roles (the item card's summary), its link, its data drawn by
 *  its data-type's view, its markdown text with its images, and where it is
 *  used. `data` is the opened item, bound by `dataField` or laid by its
 *  producer; with `fetch`, the node opens it itself (its args read `${…}` in
 *  the page's data, as an action's). The data-type's view is resolved through
 *  `resolve` (the object pane's resolve-part). Drawn where the host shows the
 *  source model (IN-045's switch); elsewhere it draws nothing. */
export interface UIItemDetailNode extends UIPageNodeBase {
  kind: 'item-detail'
  /** The item opened. */
  data?: UIItemOpened
  /** The method opening the item - sources' `ktw-agents:sources/source-fetch`
   *  with `{id: <source>, key: <item key>}` -; its answer laid over `data`
   *  (the producer's `used` kept). */
  fetch?: { method: string; args?: Record<string, unknown> }
  /** The method resolving a data-type's view: `{kind: 'view', schema:
   *  <data-type URI>}` -> `{part}`. Default ITEM_DETAIL_RESOLVE. */
  resolve?: string
  /** The item kinds its source declares - their labels and slots. */
  kinds?: UIItemKinds
  /** Its words, the node's own when it gives them. */
  labels?: Partial<Record<'link' | 'data' | 'text' | 'used' | 'unused' | 'empty' | 'loading' | 'failed', string>>
}

/** The method resolving a data-type's view (object-pane/ResolvePart). */
export const ITEM_DETAIL_RESOLVE = 'ktw-agents:object-pane/resolve-part'

/** The words an item's detail shows. */
export const ITEM_DETAIL_LABELS = {
  link: 'Ouvrir à la source',
  data: 'Données',
  text: 'Texte',
  used: 'Utilisé par',
  unused: 'Utilisé par aucun objet.',
  empty: 'Cet élément n\'apporte ni données ni texte.',
  loading: 'Ouverture de l\'élément…',
  failed: 'L\'élément n\'a pas pu être ouvert',
}

/** What an item's detail draws, in order: its card, its link, its data (by
 *  the data-type's view when it names one, else field by field), its text,
 *  where it is used - each part only when the item carries it. */
export interface ItemDetailParts {
  item?: UIPickerItem
  /** Its link, when it is one a browser opens (http, https). */
  href?: string
  /** Where it is, when it is no web link (confluence://…): shown as text. */
  uri?: string
  data?: Record<string, unknown>
  dataType?: string
  /** The data's fields as text, drawn when no data-type view is. */
  dataRows: { field: string; value: string }[]
  text?: string
  used?: UIItemUse[]
  /** It brings neither data nor text. */
  empty: boolean
}

/** itemDataRows is an item's data field by field: a nested value as JSON. */
export function itemDataRows(data: Record<string, unknown> | undefined): { field: string; value: string }[] {
  if (!data) return []
  return Object.entries(data)
    .filter(([, v]) => v !== undefined && v !== null && v !== '')
    .map(([field, v]) => ({ field, value: typeof v === 'object' ? JSON.stringify(v) : String(v) }))
}

/** itemDetailParts reads an opened item as what its detail draws. */
export function itemDetailParts(opened: UIItemOpened | undefined): ItemDetailParts {
  const o = opened ?? {}
  const data = o.data && typeof o.data === 'object' && Object.keys(o.data).length > 0 ? o.data : undefined
  const text = typeof o.text === 'string' && o.text.trim() !== '' ? o.text : undefined
  const uri = o.item?.uri
  const web = typeof uri === 'string' && /^https?:\/\//i.test(uri)
  return {
    item: o.item,
    ...(web ? { href: uri } : uri ? { uri } : {}),
    ...(data ? { data, dataType: o.data_type || undefined } : {}),
    dataRows: itemDataRows(data),
    ...(text ? { text } : {}),
    ...(Array.isArray(o.used) ? { used: o.used.filter((u) => u && typeof u.id === 'string' && u.id !== '') } : {}),
    empty: !data && !text,
  }
}

export type UIInteractiveNode =
  | UIItemDetailNode
  | UISourceSearchNode
  | UIPanelNode
  | UIChipsNode
  | UITermsNode
  | UIActionBarNode
  | UIProgressListNode
  | UISearchNode
  | UISwitchNode
  | UIChoiceCardsNode
  | UITextFieldNode
  | UIGridBoardNode
  | UIBoardNode
  | UIStepperNode
  | UISelectNode
  | UIMenuNode
  | UIObjectCardNode
  | UIPickerNode
  | UIButtonNode
  | UIListNode
  | UITreeNode
  | UILanesNode
  | UIGaugeNode
  | UIGanttNode
  | UIGraphNode

// =================================================================================================
// Layout helpers — pure, so they are tested apart from rendering
// =================================================================================================

export interface TreeRow<T> {
  row: T
  id: string
  depth: number
  children: TreeRow<T>[]
}

/** nestRows turns flat rows into a forest: each row under its parent, a row
 *  whose parent is absent or unknown a root, in the rows' order. A cycle is
 *  broken where it closes. */
export function nestRows<T extends Record<string, unknown>>(rows: T[], idField = 'id', parentField = 'parent'): TreeRow<T>[] {
  const byId = new Map<string, TreeRow<T>>()
  for (const r of rows) {
    const id = String(r[idField] ?? '')
    if (id !== '' && !byId.has(id)) byId.set(id, { row: r, id, depth: 0, children: [] })
  }
  const roots: TreeRow<T>[] = []
  for (const r of rows) {
    const node = byId.get(String(r[idField] ?? ''))
    if (!node || node.row !== r) continue
    const pid = r[parentField] === undefined || r[parentField] === null ? '' : String(r[parentField])
    const parent = pid !== '' ? byId.get(pid) : undefined
    if (parent && !reaches(parent, node, byId, parentField)) parent.children.push(node)
    else roots.push(node)
  }
  const depth = (ns: TreeRow<T>[], d: number) => ns.forEach((n) => { n.depth = d; depth(n.children, d + 1) })
  depth(roots, 0)
  return roots
}

/** reaches: walking up from `from` meets `target` (attaching would close a cycle). */
function reaches<T extends Record<string, unknown>>(from: TreeRow<T>, target: TreeRow<T>, byId: Map<string, TreeRow<T>>, parentField: string): boolean {
  const seen = new Set<string>()
  let cur: TreeRow<T> | undefined = from
  while (cur && !seen.has(cur.id)) {
    if (cur === target) return true
    seen.add(cur.id)
    const pid: unknown = cur.row[parentField]
    cur = pid === undefined || pid === null ? undefined : byId.get(String(pid))
  }
  return false
}

/** ganttSpan is the axis length: the latest end, at least 1. */
export function ganttSpan(tasks: UIGanttTask[]): number {
  return Math.max(1, ...tasks.map((t) => (Number(t.start) || 0) + (Number(t.length) || 0)))
}

const DAY_MS = 86_400_000
const dayOf = (d?: string): number | undefined => {
  if (!d || !/^\d{4}-\d{2}-\d{2}/.test(d)) return undefined
  const t = Date.UTC(Number(d.slice(0, 4)), Number(d.slice(5, 7)) - 1, Number(d.slice(8, 10)))
  return Number.isNaN(t) ? undefined : t / DAY_MS
}
const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']

export interface GanttCalendar {
  /** The tasks placed in days from the axis' first day (`start`, `length`). */
  tasks: UIGanttTask[]
  /** The axis' length in days. */
  span: number
  /** A tick at the first day of each month the axis crosses. */
  ticks: { at: number; label: string }[]
  /** Where today falls, in days from the axis' first day; absent when outside it. */
  today?: number
}

/** ganttCalendar places dated tasks on a calendar axis: from the first month
 *  any task (or today) touches to the end of the last, a tick per month, each
 *  task a whole number of days (its last day included). A task with no valid
 *  `from` is left out. */
export function ganttCalendar(tasks: UIGanttTask[], today?: string): GanttCalendar {
  const dated = tasks.flatMap((t) => {
    const a = dayOf(t.from)
    if (a === undefined) return []
    const b = Math.max(a, dayOf(t.to) ?? a)
    return [{ t, a, b }]
  })
  const now = dayOf(today)
  const days = [...dated.flatMap((d) => [d.a, d.b]), ...(now === undefined ? [] : [now])]
  if (dated.length === 0) return { tasks: [], span: 1, ticks: [] }
  const first = new Date(Math.min(...days) * DAY_MS)
  const last = new Date(Math.max(...days) * DAY_MS)
  const start = Date.UTC(first.getUTCFullYear(), first.getUTCMonth(), 1) / DAY_MS
  const end = Date.UTC(last.getUTCFullYear(), last.getUTCMonth() + 1, 1) / DAY_MS
  const ticks: { at: number; label: string }[] = []
  let y = first.getUTCFullYear()
  let m = first.getUTCMonth()
  while (Date.UTC(y, m, 1) / DAY_MS < end) {
    ticks.push({ at: Date.UTC(y, m, 1) / DAY_MS - start, label: m === 0 || ticks.length === 0 ? `${MONTHS[m]} ${y}` : MONTHS[m] })
    if (m === 11) { y++; m = 0 } else { m++ }
  }
  return {
    tasks: dated.map(({ t, a, b }) => ({ ...t, start: a - start, length: b - a + 1 })),
    span: end - start,
    ticks,
    today: now !== undefined && now >= start && now <= end ? now - start : undefined,
  }
}

export interface PlacedBox extends UIGraphBox {
  layer: number
  /** Its row, counted over the whole graph. */
  slot: number
}

/** A group's frame: the box it is drawn from, the layers and the rows it
 *  spans (slotTo excluded) - its title on its first row, its members on the
 *  rows after. */
export interface PlacedFrame extends UIGraphBox {
  layerFrom: number
  layerTo: number
  slotFrom: number
  slotTo: number
}

/** layerGraph places each box in a layer one after the deepest box it comes
 *  from (longest path), and in a slot within its layer, in input order. Edges
 *  naming an unknown box are ignored; a cycle is broken at the box first met.
 *  A box others name as their `group` is a frame: its members
 *  keep together in one band of rows - the frame's title on the band's first
 *  row -, the bands in the order they are first met, the boxes in no group in
 *  one band of their own; an edge from or to a frame orders all its members. */
export function layerGraph(nodes: UIGraphBox[], edges: UIGraphEdge[]): { boxes: PlacedBox[]; layers: number; frames: PlacedFrame[] } {
  const ids = new Set(nodes.map((n) => n.id))
  const members = new Map<string, string[]>()
  for (const n of nodes) {
    if (n.group && n.group !== n.id && ids.has(n.group)) members.set(n.group, [...(members.get(n.group) ?? []), n.id])
  }
  const laid = nodes.filter((n) => !members.has(n.id))
  const ends = (id: string) => members.get(id) ?? [id]
  const incoming = new Map<string, string[]>()
  for (const e of edges) {
    if (!ids.has(e.from) || !ids.has(e.to) || e.from === e.to) continue
    for (const a of ends(e.from)) {
      for (const b of ends(e.to)) {
        if (a !== b) incoming.set(b, [...(incoming.get(b) ?? []), a])
      }
    }
  }
  const layer = new Map<string, number>()
  const visiting = new Set<string>()
  const depthOf = (id: string): number => {
    const known = layer.get(id)
    if (known !== undefined) return known
    if (visiting.has(id)) return 0
    visiting.add(id)
    const d = Math.max(-1, ...(incoming.get(id) ?? []).map(depthOf)) + 1
    visiting.delete(id)
    layer.set(id, d)
    return d
  }
  laid.forEach((n) => depthOf(n.id))
  const bandOf = (n: UIGraphBox) => (n.group && members.has(n.group) ? n.group : '')
  const bands: string[] = []
  for (const n of laid) if (!bands.includes(bandOf(n))) bands.push(bandOf(n))
  const boxes: PlacedBox[] = []
  const frames: PlacedFrame[] = []
  let cursor = 0
  for (const band of bands) {
    const head = band === '' ? 0 : 1
    const slots = new Map<number, number>()
    for (const n of laid) {
      if (bandOf(n) !== band) continue
      const l = layer.get(n.id) ?? 0
      const s = slots.get(l) ?? 0
      slots.set(l, s + 1)
      boxes.push({ ...n, layer: l, slot: cursor + head + s })
    }
    const rows = Math.max(1, ...slots.values())
    if (band !== '') {
      const ls = boxes.filter((b) => bandOf(b) === band).map((b) => b.layer)
      const g = nodes.find((n) => n.id === band)!
      frames.push({ ...g, layerFrom: Math.min(...ls), layerTo: Math.max(...ls), slotFrom: cursor, slotTo: cursor + head + rows })
    }
    cursor += head + rows
  }
  const order = new Map(laid.map((n, i) => [n.id, i]))
  boxes.sort((a, b) => (order.get(a.id) ?? 0) - (order.get(b.id) ?? 0))
  return { boxes, layers: Math.max(0, ...boxes.map((b) => b.layer)) + 1, frames }
}

export interface GaugePart extends UIGaugeSegment {
  /** Its share of the bar, 0–100. */
  percent: number
}

/** gaugeBar measures a gauge: its value, its maximum and its parts as shares
 *  of the bar. A single bar is one part; stacked segments sum to the value,
 *  the maximum their sum unless declared. A negative or non-numeric value
 *  counts 0; shares are clamped so the bar never overflows. */
export function gaugeBar(node: UIGaugeNode): { value: number; max: number; parts: GaugePart[] } {
  const num = (v: unknown) => Math.max(0, Number(v) || 0)
  if (node.segments?.length) {
    const segs = node.segments.map((s) => ({ ...s, value: num(s.value) }))
    const value = segs.reduce((a, s) => a + s.value, 0)
    const max = node.max && node.max > 0 ? node.max : value
    const parts = segs.map((s) => ({ ...s, percent: max > 0 ? Math.min(100, (s.value / max) * 100) : 0 }))
    return { value, max, parts }
  }
  const value = num(node.value)
  const max = node.max && node.max > 0 ? node.max : 100
  return { value, max, parts: [{ value, color: node.color, label: node.label, percent: Math.min(100, (value / max) * 100) }] }
}

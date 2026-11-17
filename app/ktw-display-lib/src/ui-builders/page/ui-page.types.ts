// SPDX-License-Identifier: Apache-2.0
// UIPage runtime types — the JSON DSL emitted by the LLM (or a skill, or a
// sidecar template) that describes a page as a tree of composable nodes.
//
// Per the display module's decision on where the UI lives (modules/display/decisions/), this is the
// default extension surface. New visual concepts ship as DSL nodes added
// HERE, not as sidecar TSX bundles. Sidecars contribute schemas + DSL
// templates only.
//
// Three node categories, dispatched uniformly by `<UIPageView>`:
//   1. Layout — has `children: UIPageNode[]` (or kind-specific child shape
//      for tabs/accordion). Recurses into children.
//   2. Schema-bound — resolves a schema (named or inline) and renders the
//      corresponding composition (form/view/table/wizard) INLINE inside
//      the page iframe. NO nested iframe.
//   3. Atomic — leaf node with self-contained props.

import type { Schema } from '../../schema/index.ts'

// =================================================================================================
// Common — shared by every node
// =================================================================================================

/** Stable id for cross-turn `patch-ui-component` addressing. Optional at
 *  emit time; the runtime auto-assigns `<kind>-<index>` when missing.
 *  Ids must be unique within an artifact tree. */
import type { UIInteractiveNode, UIPageAction } from './ui-page.interactive.ts'
import { listItemNode } from './ui-page.interactive.ts'
import type { UILogNode, UIProgressNode } from './progress-log.ts'

export interface UIPageNodeBase {
  /** Stable identifier for cross-turn id-based patching. Optional. */
  id?: string

  /** Optional JSONPath reference into the artifact's outer `data` object
   *  (set at the artifact level via `display-ui-component`'s `data` field).
   *  When present, the resolver fills the node's "main" field from the
   *  bound value, replacing any inline value. The "main" field per kind:
   *  heading→text, text→content, divider→label, badge→text, link→text,
   *  image→src, kpi→value, code-block→content, markdown→content,
   *  form/view/table/wizard→data; a section bound to a collection counts it
   *  in its badge and shows its empty state when it has none. The other
   *  layout nodes (vbox/hbox/etc) silently ignore dataField. Wildcards (`$.items[*]`) return arrays —
   *  best used with table.dataField.
   *
   *  Wired by display-package-restructure Phase 3 (2026-05-06). */
  dataField?: string

  /** Optional JSONPath into the same data: its value is appended to the
   *  node's id (`<id>:<value>`, the value alone when the node has no id), so
   *  a node in a row template takes a per-row id from its row (`$.item.id`)
   *  and every element id drawn from it - a text's toggle
   *  `text:<id>:<value>:toggle` - differs from row to row. */
  idField?: string

  /** On a node holding rows in `data` (a table, a list, lanes, a grid
   *  board…): only the rows passing every clause are drawn, read against the
   *  page selection the page filters set. */
  filter?: UIFilterClause[]

  /** Any node: drawn only while the page selection holds what it names -
   *  `key=value` the key holds that value, `key` the key is set (a create
   *  page's note under the card chosen: `connector=leanix-application`);
   *  none always drawn. */
  when?: string
}

/** A clause of a node's `filter`, read against the page selection's `key`:
 *  `equals` (the default) keeps the rows whose `field` is the key's value -
 *  the key's value `empty` keeping the rows whose field is empty; `contains`
 *  keeps the rows one of whose `fields` holds the key's text, ignoring case;
 *  `unless` shows a row whose `field` is set only while the key is on. An
 *  unset key keeps every row (an `unless` clause hides its rows then). */
export interface UIFilterClause {
  key: string
  field?: string
  fields?: string[]
  match?: 'equals' | 'contains' | 'unless'
  empty?: string
}

// =================================================================================================
// Layout nodes (8) — containers with children
// =================================================================================================

/** Common layout knobs shared by the simple stack/row containers. */
interface UILayoutCommon {
  /** Mantine theme size token: 'xs' | 'sm' | 'md' | 'lg' | 'xl' OR a px number. */
  gap?: string | number
  /** Mantine padding token. */
  p?: string | number
}

export interface UIVBoxNode extends UIPageNodeBase, UILayoutCommon {
  kind: 'vbox'
  children: UIPageNode[]
  align?: 'stretch' | 'flex-start' | 'center' | 'flex-end'
  /** Where the children sit along the height the vbox is given - `center`
   *  centres them in a cell stretched to its row. */
  justify?: 'flex-start' | 'center' | 'flex-end' | 'space-between'
}

export interface UIHBoxNode extends UIPageNodeBase, UILayoutCommon {
  kind: 'hbox'
  children: UIPageNode[]
  align?: 'stretch' | 'flex-start' | 'center' | 'flex-end'
  justify?: 'flex-start' | 'center' | 'flex-end' | 'space-between' | 'space-around'
  /** Wrap onto multiple lines when overflowing. */
  wrap?: boolean
}

export interface UIGridNode extends UIPageNodeBase, UILayoutCommon {
  kind: 'grid'
  /** Number of columns. Children fill row-major. */
  cols: number
  children: UIPageNode[]
  /** The columns' widths by ratio: `[2, 1]` gives the first
   *  twice the second; its length is the column count. */
  ratio?: number[]
  /** The columns' widths in pixels: `[4, 150, 0, 210]`, 0 a
   *  column sharing what is left (by `ratio` when given); its length is
   *  `cols`. The cells stretch to their row's height. */
  widths?: number[]
  /** The columns drawn with a 1px line on their left: 0-based column indexes, the first (0) excluded. */
  dividers?: number[]
}

export interface UIBoxNode extends UIPageNodeBase, UILayoutCommon {
  kind: 'box'
  children: UIPageNode[]
  /** Optional CSS background color (Mantine token or raw value). */
  bg?: string
  /** Border radius token. */
  radius?: string | number
  /** A callout in this tone (a Mantine colour): its light background and a
   *  border of it. */
  callout?: string
  /** The callout's border dashed - a remark waiting, a draft. */
  dashed?: boolean
}

export interface UICardNode extends UIPageNodeBase, UILayoutCommon {
  kind: 'card'
  children: UIPageNode[]
  /** Show border (default true). */
  withBorder?: boolean
  /** Shadow token: 'xs' | 'sm' | 'md' | 'lg' | 'xl'. */
  shadow?: string
  /** Radius token. */
  radius?: string | number
  /** Its border drawn dashed, in its normal border colour - a card set apart
   *  (a delivery constraint's goal card). */
  dashed?: boolean
  /** A stripe of this tone (a Mantine colour) down its left edge - a row's
   *  kind, as the increment's Blockers tab colours each blocker. */
  stripe?: string  /** `fill`: the card takes the window's height below it and its last child
   *  scrolls inside it - boxes side by side in a grid row each scroll their
   *  own content, the page does not. */
  height?: 'fill'
}

export interface UISectionNode extends UIPageNodeBase, UILayoutCommon {
  kind: 'section'
  /** Section heading rendered before children. */
  title?: string
  /** Optional dimmed sub-heading. */
  description?: string
  /** A Tabler icon before the title (tinted by `color` at size md / lg). */
  icon?: string
  /** Mantine color token of the icon. */
  color?: string
  /** The heading's weight: `sm` a compact label row (a sub-part of a card),
   *  `md` a card's title, `lg` (default) a page section. */
  size?: 'sm' | 'md' | 'lg'
  /** A count or a word shown beside the title (`3 blocking`). */
  badge?: string | number
  /** Mantine color token of the badge (default gray). */
  badgeColor?: string
  /** Nodes drawn at the right of the heading row — badges, a total, a button. */
  aside?: UIPageNode[]
  children: UIPageNode[]
  /** Shown instead of the children when there is none (the empty state) —
   *  or, for a section bound by `dataField` to the collection it presents,
   *  when the collection has no element (its count is then the badge). */
  empty?: string
  /** Mantine color token of the empty text (default dimmed; `green` for an all-clear). */
  emptyColor?: string
}

/** One panel inside a `tabs` or `accordion` node. The content is itself
 *  a DSL sub-tree, so panels can hold any composition. */
export interface UIPanelSpec {
  /** Stable id (used to set the active tab via cross-turn patches). */
  id?: string
  /** Panel label shown in the tab strip / accordion header. */
  label: string
  /** Optional Mantine icon name (Tabler icon). */
  icon?: string
  /** A count or a word shown beside the label. */
  badge?: string | number
  /** A path in the page's data whose value is the badge - it follows the
   *  data. */
  badgeField?: string
  /** The badge's tone (a Mantine colour); grey by default. */
  badgeColor?: string
  /** The fold's colour (a Mantine colour): an accordion with look `rule`
   *  draws its label as a tag of it. */
  color?: string
  /** The panel body. */
  content: UIPageNode
  /** Its own `data-*` keys, written on the fold for a probe. */
  [dataKey: `data-${string}`]: unknown
}

export interface UITabsNode extends UIPageNodeBase {
  kind: 'tabs'
  tabs: UIPanelSpec[]
  /** Initial active tab id (matches `UIPanelSpec.id`). Default: first. */
  defaultTab?: string
  /** Mantine variant: 'default' | 'outline' | 'pills'. */
  variant?: 'default' | 'outline' | 'pills'
  /** Run when a tab is chosen, `${item.id}` its id: a page that builds only
   *  the tab shown re-reads its source for it (a `query` with `{tab}`). */
  select?: UIPageAction
}

export interface UIAccordionNode extends UIPageNodeBase {
  kind: 'accordion'
  panels: UIPanelSpec[]
  /** Allow more than one open at a time. Default: false (single). */
  multiple?: boolean
  /** Ids of initially-open panels. */
  defaultOpen?: string[]
  /** `rule` draws each fold with no frame: a caret, its label
   *  (an uppercase tag in its `color`, else a dimmed line), its badge and a
   *  rule to the end of the line - groups of rows under their heads. */
  look?: 'rule'
}

/** A split layout: a main column and a side panel beside it (sticky by
 *  default) — one column under 1000px wide. */
export interface UISplitNode extends UIPageNodeBase {
  kind: 'split'
  main: UIPageNode
  side: UIPageNode
  /** The side panel's width in px. Default: 380. */
  sideWidth?: number
  /** The side panel stays in view as the main column scrolls. Default: true. */
  sticky?: boolean
}

export type UILayoutNode =
  | UISplitNode
  | UIVBoxNode
  | UIHBoxNode
  | UIGridNode
  | UIBoxNode
  | UICardNode
  | UISectionNode
  | UITabsNode
  | UIAccordionNode

// =================================================================================================
// Atomic nodes (9 — chart deferred to dsl-chart-node todo)
// =================================================================================================

export interface UIHeadingNode extends UIPageNodeBase {
  kind: 'heading'
  text: string
  /** Heading level (1-6). Default: 2. */
  level?: 1 | 2 | 3 | 4 | 5 | 6
  /** Mantine color token. */
  color?: string
  /** A Tabler icon shown before the text, in a tinted square. */
  icon?: string
  /** `label`: a section's small label - mono, upper case, spaced, muted
   *  (the planning views' « SHIPS IN THIS RELEASE (8) »); its level and
   *  icon are then ignored. Default: a title. */
  variant?: 'title' | 'label'
}

export interface UITextNode extends UIPageNodeBase {
  kind: 'text'
  content: string
  /** Mantine size token: 'xs' | 'sm' | 'md' | 'lg' | 'xl'. */
  size?: string
  /** Mantine color token (use 'dimmed' for muted text). */
  color?: string
  /** Render as italics / bold / etc. */
  fw?: number | 'normal' | 'bold'
  fs?: 'normal' | 'italic'
  /** Clamp to this many lines (a whole number of 1 or more), an ellipsis
   *  ending the last one. */
  lineClamp?: number
  /** Draw in the monospace font, its case kept (an id, a path). */
  mono?: boolean
  /** With lineClamp: a toggle « Afficher plus » / « Afficher moins » under
   *  the text, drawn only when it overflows its lines, opens and folds it
   *  (`text:<id>:toggle` its element id). */
  expandable?: boolean
}

export interface UIDividerNode extends UIPageNodeBase {
  kind: 'divider'
  orientation?: 'horizontal' | 'vertical'
  /** Centered label rendered on the line. */
  label?: string
  /** Mantine color token. */
  color?: string
  /** Line style. Default: 'solid'. */
  variant?: 'solid' | 'dashed' | 'dotted'
}

export interface UIBadgeNode extends UIPageNodeBase {
  kind: 'badge'
  text: string
  /** A two-part chip: the kind before the value, small and
   *  muted (« application » Arbor). */
  prefix?: string
  /** Mantine color token. */
  color?: string
  /** The color by the text's value (`{blocked: 'red'}`) — for a bound text. */
  colors?: Record<string, string>
  variant?: 'filled' | 'light' | 'outline' | 'dot' | 'transparent' | 'white' | 'default'
  size?: 'xs' | 'sm' | 'md' | 'lg' | 'xl'
  /** A Tabler icon before the text (a state's sign: `circle-check`). */
  icon?: string
  /** Shown on hover. */
  tooltip?: string
  /** Wrap the tooltip inside a box of `tooltipWidth` (default 320) pixels —
   *  for a tooltip that holds several lines or a long sentence. */
  tooltipMultiline?: boolean
  /** The wrapped tooltip's width in pixels; applies when `tooltipMultiline`. */
  tooltipWidth?: number
  /** Monospace text, case kept — a path, an identifier. */
  mono?: boolean
  /** The text's own case, not upper-cased — a title, a name. */
  keepCase?: boolean
  /** Clicking the badge runs it (open the object it names, …). */
  action?: UIPageAction
}

/** A link: an `href` (a URL), or an `action` the host runs — open an object
 *  or one of its panels (`navigate`), a chat, a dialog, a method. One of the
 *  two. */
export interface UILinkNode extends UIPageNodeBase {
  kind: 'link'
  text: string
  href?: string
  /** What clicking runs, instead of following an href. */
  action?: UIPageAction
  /** Open in a new tab + add rel=noopener. Default: true for external (http(s)). */
  external?: boolean
  /** Mantine size token of the text ('xs' … 'xl'); the body size when absent. */
  size?: string
  /** Font weight (500, 600 for a title). */
  fw?: number
}

/** UILegendNode — the planning palette's legend: a small square in each
 *  word's tone and its label (`done`, `running`, `waiting`, `todo`,
 *  `blocked`, `planned` …, as the status content type colours them). */
export interface UILegendNode extends UIPageNodeBase {
  kind: 'legend'
  /** The words, each a status word or `{word, label}` to name it otherwise
   *  (`{word: waiting, label: awaiting a person}`). */
  items: (string | { word: string; label?: string })[]
}

/** UIDrawioNode — a draw.io drawing (mxGraph XML), view-only, fitted to its
 *  box. The same file can be published to Confluence's draw.io macro. */
export interface UIDrawioNode extends UIPageNodeBase {
  kind: 'drawio'
  /** mxGraph XML (`<mxfile>…</mxfile>`). */
  xml: string
  /** Minimum height in pixels. Default: 520. */
  height?: number
  /** Drawn as a small thumbnail - `true` (the box's width, landscape 16:10)
   *  or its height - that opens the diagram in full: a
   *  sketch of its boxes and arrows in the page's colours. */
  thumbnail?: boolean | number
  /** The full view's title. */
  title?: string
}

export interface UIImageNode extends UIPageNodeBase {
  kind: 'image'
  src: string
  alt: string
  width?: number | string
  height?: number | string
  /** object-fit value. Default: 'cover'. */
  fit?: 'cover' | 'contain' | 'fill' | 'none' | 'scale-down'
  /** Mantine radius token. */
  radius?: string | number
  /** Optional caption rendered below the image (small dimmed text). */
  caption?: string
}

/** UIVideoNode — embedded video player. Maps directly to a native <video>
 *  element wrapped in a Mantine container so spacing/radius matches the
 *  page DSL idiom. Source is fetched by the iframe (cross-origin allowed
 *  via standard HTTP semantics). */
export interface UIVideoNode extends UIPageNodeBase {
  kind: 'video'
  src: string
  /** MIME type hint to feed `<source>`. Default: derived from src extension
   *  (mp4 → video/mp4, webm → video/webm). */
  mime?: string
  /** Optional poster image URL (the still frame shown before play). */
  poster?: string
  /** Container width — number = px, string = CSS value, omitted = 100%. */
  width?: number | string
  /** Container height — number = px, string = CSS value. Omitted = auto. */
  height?: number | string
  /** Show native player controls. Default: true. */
  controls?: boolean
  /** Autoplay (browsers require muted for autoplay to work). Default: false. */
  autoplay?: boolean
  /** Loop on completion. Default: false. */
  loop?: boolean
  /** Mute by default. Default: false (but auto-true when autoplay=true). */
  muted?: boolean
  /** Mantine radius token applied to the player wrapper. */
  radius?: string | number
  /** Optional caption rendered below the player. */
  caption?: string
}

/** UIAudioNode — embedded audio player. Native <audio> element with
 *  controls. */
export interface UIAudioNode extends UIPageNodeBase {
  kind: 'audio'
  src: string
  /** MIME type hint. Default: derived from src extension. */
  mime?: string
  /** Show native player controls. Default: true. */
  controls?: boolean
  autoplay?: boolean
  loop?: boolean
  muted?: boolean
  /** Optional label rendered above the player. */
  label?: string
}

/** UIFileNode — downloadable file. Renders as a card with the file's
 *  name, size, type, and a download button. Does NOT embed the content
 *  inline (use `UIImageNode` / `UIVideoNode` / etc. for that). */
export interface UIFileNode extends UIPageNodeBase {
  kind: 'file'
  /** URL the download button hits. Browser navigates to it. */
  src: string
  /** Display name (typically the actual filename). */
  name: string
  /** Optional MIME type — surfaced as a label next to the filename. */
  mime?: string
  /** Optional size in bytes — surfaced as a human-readable label
   *  (e.g. "12.4 KB", "3.2 MB"). */
  sizeBytes?: number
  /** Optional description rendered below the filename. */
  description?: string
  /** `download` attribute hint — when set, browsers save with this filename
   *  instead of the URL-derived one. Defaults to `name`. */
  downloadAs?: string
}

/** UITimelineNode — vertical event timeline. Items are rendered top to
 *  bottom with timestamps on one side and title + body on the other.
 *  Each item can carry markdown so embedded code/lists render naturally. */
export interface UITimelineNode extends UIPageNodeBase {
  kind: 'timeline'
  /** The events to render, in display order (no automatic sort — caller
   *  supplies). */
  items: UITimelineItem[]
  /** Bullet style for the connector line. Default: 'dot'. */
  bullet?: 'dot' | 'check' | 'cross' | 'star' | 'icon'
  /** Active item index (1-based, 0 = none). Used to highlight progress
   *  through a sequence. Default: items.length (all "done"). */
  activeIndex?: number
  /** Connector line color — Mantine color token. Default: 'blue'. */
  color?: string
}

export interface UITimelineItem {
  /** ISO timestamp OR free-form label ("2 days ago", "Q3 2025"). */
  date: string
  /** Short headline. */
  title: string
  /** Optional markdown body (renders below the title). */
  body?: string
  /** Optional longer markdown, folded under the body behind a "Details…"
   *  switch (a step's detailed result under its one-line summary). */
  details?: string
  /** Optional Tabler icon name (overrides the node-level bullet style
   *  for THIS item — useful for "milestone" emphasis). */
  icon?: string
  /** Optional accent color for this item's bullet — Mantine token. */
  color?: string
}

export interface UIKPINode extends UIPageNodeBase {
  kind: 'kpi'
  label: string
  /** Stringified for display. Numbers formatted by the renderer. */
  value: string | number
  /** Optional trend indicator (e.g. '+12%', '-3', 'stable'). */
  trend?: string
  /** Color hint for the trend chip. Default: derived from trend sign if numeric. */
  trendColor?: 'green' | 'red' | 'yellow' | 'gray' | 'blue'
  /** Optional sub-label / context. */
  description?: string
  /** Mantine icon name (Tabler). */
  icon?: string
  /** Clicking the card runs it (a declared panel's host) — e.g. filter the
   *  panel to what the count counts. */
  action?: UIPageAction
  /** Mantine color token: tints the card, its icon and its value. */
  color?: string
  /** The tile is the one chosen (the filter its action set): marked. */
  selected?: boolean
  /** Drawn in the tile under its caption (a button opening what the count
   *  counts); a tile with children is not itself clickable. */
  children?: UIPageNode[]
  /** `inline`: the label bold with the count on its line, no icon, a soft
   *  ring in its colour — a call to act (« Needs you ») beside measures. */
  variant?: 'inline'
}

export interface UICodeBlockNode extends UIPageNodeBase {
  kind: 'code-block'
  content: string
  /** Highlighter language hint. Default: 'text'. */
  language?: string
  /** Show copy button. Default: true. */
  withCopyButton?: boolean
}

export interface UIMarkdownNode extends UIPageNodeBase {
  kind: 'markdown'
  /** GFM-flavored markdown source. Sanitized at render time. */
  content: string
  /** Collapsible: the text shows up to this height in pixels, then a
   *  "Show more" toggle opens the rest. Absent = always whole. */
  maxHeight?: number
}

/** A series spec for line / bar / area charts — names a numeric key in
 *  each row of `data` and assigns a color. Optional `label` overrides
 *  the default (which uses `name`). */
export interface UIChartSeries {
  /** Key name in each data row pointing to the numeric value. */
  name: string
  /** Mantine color token (e.g. `'indigo.6'`, `'teal.5'`). When omitted,
   *  Mantine assigns one from the theme palette. */
  color?: string
  /** Optional human-readable legend label. Defaults to `name`. */
  label?: string
}

/** Chart atomic node — wraps `@mantine/charts` (recharts under the hood).
 *  v1 supports line / bar / area / pie / donut. radar / scatter / treemap
 *  deferred to follow-up todos when use cases emerge.
 *
 *  Data shape: an array of objects (rows). For line/bar/area, each row
 *  has one categorical key (`dataKey`) plus N numeric keys (the series).
 *  For pie/donut, each row has `name` + `value` + optional `color`.
 *
 *  Reactive: when the host pushes a `value` postMessage that updates the
 *  artifact's tree, this node's `data` prop changes and the chart
 *  re-renders. Empty `data` shows a "No data" placeholder.
 *
 *  Per dsl-chart-node todo (todo/ms06, 2026-05-07). */
export interface UIChartNode extends UIPageNodeBase {
  kind: 'chart'
  /** Chart variant. Required. */
  chartKind: 'line' | 'bar' | 'area' | 'pie' | 'donut'
  /** Row data — pass-through to Mantine. Each row is an object. */
  data: Array<Record<string, unknown>>

  // ── line / bar / area: x-axis + numeric series ──────────────────────
  /** Categorical key on each row (e.g. `'date'` or `'category'`). */
  dataKey?: string
  /** Numeric series to plot. */
  series?: UIChartSeries[]
  /** Curve type for line/area. Default: `'monotone'`. */
  curveType?: 'bump' | 'linear' | 'natural' | 'monotone' | 'step' | 'stepBefore' | 'stepAfter'

  // ── pie / donut: per-row name + value ───────────────────────────────
  /** Row key for slice label (pie/donut). Default: `'name'`. */
  nameKey?: string
  /** Row key for slice value (pie/donut). Default: `'value'`. */
  valueKey?: string
  /** Display labels on slices (pie/donut). */
  withLabels?: boolean
  /** Donut: thickness of the ring (px). Default Mantine default. */
  thickness?: number

  // ── shared options ──────────────────────────────────────────────────
  /** Chart container height in px. Default 240. */
  h?: number
  /** Show tooltip on hover. Default true. */
  withTooltip?: boolean
  /** Show legend. Default true when series.length > 1, else false. */
  withLegend?: boolean
  /** Show X axis (line/bar/area). Default true. */
  withXAxis?: boolean
  /** Show Y axis (line/bar/area). Default true. */
  withYAxis?: boolean
  /** Optional title shown above the chart (Mantine Text). */
  title?: string
}

/** A coloured letter monogram: a source's, a connector's
 *  mark - one or two letters (from `text`) on a square of `color`. */
export interface UIMonogramNode extends UIPageNodeBase {
  kind: 'monogram'
  /** The letters, or a name they are taken from (« LeanIX » → « LE »). */
  text: string
  color?: string
  size?: number
  tooltip?: string
}

export type UIAtomicNode =
  | UIMonogramNode
  | UIHeadingNode
  | UITextNode
  | UIDividerNode
  | UIBadgeNode
  | UILinkNode
  | UIImageNode
  | UIDrawioNode
  | UILegendNode
  | UIVideoNode
  | UIAudioNode
  | UIFileNode
  | UITimelineNode
  | UIKPINode
  | UICodeBlockNode
  | UIMarkdownNode
  | UIChartNode
  | UIProgressNode
  | UILogNode

// =================================================================================================
// Schema-bound nodes (4) — resolve a schema + value, render via the
// existing UIxxxView components INLINE (no nested iframe)
// =================================================================================================

/** Common shape for all 4 schema-bound nodes — the one component vocabulary
 *  (render.Def): the schema, the variant of its `ui-<kind>` extension, an
 *  optional inline `ui` block, and the data. A schema named by id (a data-type
 *  URI) or by an action's input and a `ui` block are resolved by the host
 *  before the page renders (Go, the object pane's ResolvePanel), so a renderer
 *  only ever sees an inline schema. */
interface UISchemaBoundCommon {
  /** The schema: its canonical AST inline, a data-type URI, or `{action: <uri>}`
   *  (the input of an action) — the last two resolved by the host. */
  schema?: Schema | string | { action: string }
  /** Which `ui-<kind>` extension of the schema to use, by name. Default: 'default'. */
  variant?: string
  /** An inline `ui-<kind>` extension block laid over the schema (resolved by the host). */
  ui?: Record<string, unknown>
  /** The value the component shows or edits. */
  data?: unknown
}

export interface UIFormNode extends UIPageNodeBase, UISchemaBoundCommon {
  kind: 'form'
  /** CRUD mode. Default: 'edit'. */
  mode?: 'create' | 'edit' | 'update' | 'view'
  /** Draw the form without its own heading and toolbar — the page around it
   *  titles it (a form answering a question in a section). Default: false. */
  hideTitle?: boolean
  /** What submitting runs, through the host: `${value.<path>}` is the form's
   *  value; a method (`uri`) with no `args` takes the whole value as its
   *  input. Without it the page's own `submit` gets the value. */
  submit?: UIPageAction
}

export interface UIViewNode extends UIPageNodeBase, UISchemaBoundCommon {
  kind: 'view'
  /** Read-only. Mode is always 'view'. */
  mode?: 'view'
  /** Empty-value rendering strategy. */
  emptyStrategy?: 'omit' | 'em-dash' | 'explicit-empty'
}

export interface UITableNode extends UIPageNodeBase, UISchemaBoundCommon {
  kind: 'table'
  /** Tables are read-only (writes go through row-click → mounted form). */
  mode?: 'view'
  /** Named filter variant. Default: 'default'. */
  filterName?: string
  /** A click on a row runs it, `${item.*}` the row — open the object it is
   *  (`navigate`), its panel, a dialog. */
  open?: UIPageAction
  /** Actions on each row, drawn in a last column. */
  actions?: UIPageAction[]
}

export interface UIWizardNode extends UIPageNodeBase, UISchemaBoundCommon {
  kind: 'wizard'
  /** CRUD mode. Default: 'create'. */
  mode?: 'create' | 'edit'
  /** What finishing runs: the action with the steps'
   *  accumulated value as its element (`${item.<field>}`) and `${value}`;
   *  its label the button's. */
  complete?: UIPageAction
  /** What leaving runs - « Abandonner » -, `confirm` asked first. */
  cancel?: UIPageAction
  /** `stacked`: every step drawn together, in order, each marked done,
   *  current or next; `paged` (the default): the current one only. */
  layout?: 'paged' | 'stacked'
  /** The current step's id, as the producer says it - its data moves with
   *  events -; or `currentField`, a path in the page's data holding it.
   *  Either way the stepper follows it and no « Suivant » is drawn. */
  current?: string
  currentField?: string
}

/** UITimelineLanesNode — lanes on a shared date axis: the
 *  schema is the LANE type, its `ui-timeline-lanes` variant says which field
 *  holds the items, their date, the header and card views; `data` is the
 *  lanes. A click on a card runs the page's `select` verb with the variant's
 *  `select` key. */
export interface UITimelineLanesNode extends UIPageNodeBase, UISchemaBoundCommon {
  kind: 'timeline-lanes'
}

export type UISchemaBoundNode =
  | UIFormNode
  | UIViewNode
  | UITableNode
  | UIWizardNode
  | UITimelineLanesNode

// =================================================================================================
// The discriminated union — what `<UIPageView node={...}>` accepts
// =================================================================================================

/** A DSL node — one of its kinds across 4 categories. */
export type UIPageNode = UILayoutNode | UIAtomicNode | UISchemaBoundNode | UIInteractiveNode

/** Discriminator string union (useful for switch-case exhaustiveness). */
export type UIPageNodeKind = UIPageNode['kind']

/** All recognized kinds, grouped by category. The runtime dispatcher uses
 *  these sets to decide whether a node has children, has schema/value
 *  resolution, or is a leaf. */
export const LAYOUT_KINDS = [
  'vbox', 'hbox', 'grid', 'box', 'card', 'section', 'tabs', 'accordion', 'split',
] as const satisfies readonly UIPageNodeKind[]

export const ATOMIC_KINDS = [
  'heading', 'text', 'divider', 'badge', 'link', 'image', 'video', 'audio', 'file', 'timeline', 'kpi', 'code-block', 'markdown', 'chart', 'drawio', 'legend', 'monogram', 'progress', 'log',
] as const satisfies readonly UIPageNodeKind[]

export const SCHEMA_BOUND_KINDS = [
  'form', 'view', 'table', 'wizard', 'timeline-lanes',
] as const satisfies readonly UIPageNodeKind[]

/** The interactive kinds (ui-page.interactive.ts): they act through the host. */
export const INTERACTIVE_KINDS = [
  'button', 'list', 'tree', 'lanes', 'gauge', 'gantt', 'graph', 'picker', 'select', 'menu', 'stepper', 'board', 'grid-board',
  'chips', 'search', 'switch', 'choice-cards', 'text-field', 'terms', 'source-search', 'item-detail', 'action-bar', 'progress-list', 'panel',
  'object-card',
] as const satisfies readonly UIPageNodeKind[]

export const ALL_KINDS: readonly UIPageNodeKind[] = [
  ...LAYOUT_KINDS, ...ATOMIC_KINDS, ...SCHEMA_BOUND_KINDS, ...INTERACTIVE_KINDS,
]

// =================================================================================================
// Type guards
// =================================================================================================

export function isLayoutNode(node: UIPageNode): node is UILayoutNode {
  return (LAYOUT_KINDS as readonly string[]).includes(node.kind)
}

export function isAtomicNode(node: UIPageNode): node is UIAtomicNode {
  return (ATOMIC_KINDS as readonly string[]).includes(node.kind)
}

export function isSchemaBoundNode(node: UIPageNode): node is UISchemaBoundNode {
  return (SCHEMA_BOUND_KINDS as readonly string[]).includes(node.kind)
}

export function isInteractiveNode(node: UIPageNode): node is UIInteractiveNode {
  return (INTERACTIVE_KINDS as readonly string[]).includes(node.kind)
}

/** Walks a node's direct children. Layout nodes vary by kind: vbox/hbox/
 *  grid/box/card/section have a `children` array; tabs/accordion have
 *  panels/tabs each carrying a `content` node. Atomic + schema-bound
 *  nodes have no children. */
export function childrenOf(node: UIPageNode): UIPageNode[] {
  switch (node.kind) {
    case 'vbox':
    case 'hbox':
    case 'grid':
    case 'box':
    case 'card':
      return node.children
    case 'section':
      return [...(node.aside ?? []), ...(node.children ?? [])]
    case 'tabs':
      return node.tabs.map((t) => t.content)
    case 'accordion':
      return node.panels.map((p) => p.content)
    case 'split':
      return [node.main, node.side]
    case 'board':
      return [node.panel]
    case 'grid-board':
      return node.item ? [node.item] : []
    case 'list': {
      // Its template, or its elements' type drawn as its card.
      const item = listItemNode(node)
      return item ? [item] : []
    }
    case 'tree':
    case 'lanes':
      return node.item ? [node.item] : []
    default:
      return []
  }
}

// =================================================================================================
// Exhaustiveness helper — use in switch defaults
// =================================================================================================

/** Compile-time exhaustiveness check. Used in the renderer's switch
 *  default arm: when a new kind is added to UIPageNodeKind, calls to
 *  this with `node.kind` (which would now widen to a union containing
 *  the new kind) become a TS error until the switch handles it. */
export function assertNeverNode(node: never): never {
  throw new Error(`unhandled UIPageNode kind: ${JSON.stringify(node)}`)
}

/** hasSchemaBound reports whether a tree holds a schema-bound node (a form, a
 *  view, a table, a wizard) or a list naming its elements' data type
 *  (`itemType`, drawn by that type's view) — what a host resolves (schemas
 *  named by id or by an action's input, ui blocks, options) before the page
 *  renders. It never looks inside a node's own `schema`, `itemType`, `ui` or
 *  `data` — save the node each element of a list holds in its `nodeField`, a
 *  node the page draws. */
export function hasSchemaBound(v: unknown): boolean {
  if (Array.isArray(v)) return v.some(hasSchemaBound)
  if (!v || typeof v !== 'object') return false
  const n = v as Record<string, unknown>
  if (typeof n.kind === 'string' && (SCHEMA_BOUND_KINDS as readonly string[]).includes(n.kind)) return true
  if ((typeof n.itemType === 'string' && n.itemType !== '') || (!!n.itemType && typeof n.itemType === 'object')) return true
  const field = n.nodeField
  if (typeof field === 'string' && field !== '' && Array.isArray(n.data) &&
    n.data.some(el => !!el && typeof el === 'object' && hasSchemaBound((el as Record<string, unknown>)[field]))) return true
  return Object.entries(n).some(([k, c]) => !NODE_PAYLOAD.has(k) && hasSchemaBound(c))
}

/** The fields of a node that are its own payload, never nested nodes. */
const NODE_PAYLOAD = new Set(['schema', 'itemType', 'ui', 'data'])

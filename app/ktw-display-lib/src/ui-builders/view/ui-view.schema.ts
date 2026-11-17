// SPDX-License-Identifier: Apache-2.0
// UIViewDef — the DSL `ui-view` extension shape.
//
// Ported from _ktw_workspace/.../cpm-ui/ui-view/ui-view.schema.ts during
// gui-view Subtask 1 (2026-05-04). DSL-aligned renames vs the source:
//   - `nbCols` → `cols` (group-level)
//   - `defaultCollapsed` → `collapsed` (group-level)
//   - `notNew`/`notEdit` → `visibility?: SchemaFieldVisibility[]`
//     (matches the gui-form negative→positive migration; for view-side,
//     the `'view'` token is what's HONORED; `'create'`/`'update'` tokens
//     mean "form-only", so a group declared `visibility: ['create']` is
//     hidden in view mode)
//
// Layout enum: 5 NEW spatial layouts vs old code's 3. Detail-pane is the
// new default (was effectively 'single' in old code).

import type { SchemaFieldVisibility, SchemaTypeExtension } from '../../schema/index.ts'
import type { UIFieldRef } from '../fields/index.ts'
import type { UIActionRef } from '../../ui-core/action/index.ts'

/** Group definition — one section inside a view. */
export type UIViewGroupDef = {
  /** Stable name of the group: what its title and description are translated
   *  by (`types.<T>.extensions.<name>.groups.<id>.title`). A group without one
   *  is shown in its own language. The renderer does not read it. */
  id?: string
  title?: string
  description?: string

  /** Mantine Grid span (1-12) — how wide this group sits in the view's grid. */
  md?: number

  /** Number of columns inside this group's SimpleGrid (was `nbCols`). */
  cols?: number

  /** Which CRUD modes show this group. View mode honors the `'view'`
   *  token; groups declared as `['create']` or `['update']` only are
   *  hidden in view. Aligns with the schema-side `SchemaFieldVisibility[]`. */
  visibility?: SchemaFieldVisibility[]

  /** Field refs in this group — string (id) or full UIFieldDef. */
  fields?: UIFieldRef[]

  /** Renders the group as a collapsible section (only honored by layouts
   *  that support it — `card` does, `inline` may not). */
  collapsible?: boolean

  /** Initial collapsed state when `collapsible: true`. Was `defaultCollapsed`. */
  collapsed?: boolean
}

/** An action of a `slots` view: a menu entry's shape `{id, label, uri, args,
 *  confirm?, choice?, prompt?}` (page/ui-page.interactive.ts UIMenuItem); a
 *  choice entry is drawn as a compact select whose current value is the
 *  element's field `value`. */
export type UIViewSlotAction = {
  id: string
  label: string
  uri?: string
  args?: Record<string, unknown>
  confirm?: string
  dangerous?: boolean
  when?: string
  prompt?: string
  choice?: string | { options?: { value: string; label?: string }[]; optionsField?: string }
  value?: string
  [k: string]: unknown
}

/** The slots of the `slots` view layout. Each names a field by its path — a
 *  dotted path reaches into a nested object (`release.gates`) — drawn by its
 *  content renderer's view; a plain-text field is drawn as text in the
 *  slot's typography (a badge as a neutral chip). */
/** A slot's field: its path, or its path with a tone — a palette token
 *  (done, run, wait, pend, block, plan) or a status word (`planned`) — its
 *  text is drawn in. */
export type UIViewSlotField = string | { field: string; tone?: string }

export type UIViewSlots = {
  /** A small mono line above the title (an id). */
  eyebrow?: string
  /** The bold title. */
  title?: string
  /** An id beside the title, mono and dimmed. */
  titleId?: string
  /** Chips after the title. */
  badges?: UIViewSlotField[]
  /** One dimmed line, the non-empty values joined by « · ». */
  meta?: UIViewSlotField[]
  /** Full-width rows under the meta line (a stepper, gate dots). */
  footer?: UIViewSlotField[]
  /** The element's actions, drawn inline: a `primary` one filled, the
   *  `menu` ones under a ⋯. */
  actions?: UIViewSlotAction[]
  /** Where the actions go: `bottom` (default) a row under the rest, `top`
   *  the header's top right, beside the eyebrow, title and meta. */
  actionsAt?: 'top' | 'bottom'
}

/**
 * UIViewDef — the schema `ui-view` extension. Authors attach via
 * `extensions: [{ type: 'ui-view', name: 'default', layout: 'detail-pane', ... }]`.
 */
export type UIViewDef = SchemaTypeExtension<{
  type: 'ui-view'

  /** View name in case of multi-view. Defaults to `'default'` when omitted —
   *  the view widget exposes `?view=<name>` query param to pick a non-default
   *  view, falling back to `'default'`. */
  name?: string

  /** Layout strategy. Looked up against `getViewLayoutRegistry()`; default
   *  comes from `LAYOUT_DEFAULTS.view` ('detail-pane'). */
  layout?: 'detail-pane' | 'card' | 'inline' | 'two-column' | 'tabs' | 'slots'

  /** The `slots` layout's slots: which field goes where in a
   *  compact card or header. Ignored by the other layouts. */
  slots?: UIViewSlots

  /** View sections. When omitted, the builder synthesizes a default group
   *  containing all fields. */
  groups?: UIViewGroupDef[]

  /** Action refs (string id, UIActionDef, or full UIAction). When omitted,
   *  the view's standard action set is used (defaultViewActions: close/edit/delete). */
  actions?: UIActionRef[]
}>

/** UIViewRef — schema attaches this to ui-view extension lookups.
 *  - undefined: pick the default view
 *  - string: pick the named view
 *  - UIViewDef: use this object directly */
export type UIViewRef = string | UIViewDef

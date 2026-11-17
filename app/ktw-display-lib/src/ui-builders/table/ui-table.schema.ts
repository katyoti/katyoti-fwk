// SPDX-License-Identifier: Apache-2.0
// UITableDef — the DSL `ui-table` extension shape.
//
// Ported from _ktw_workspace/.../cpm-ui/ui-table/ui-table.schema.ts during
// gui-table Subtask 2 (2026-05-04). No DSL renames required; the shape
// already used snake-case-friendly names (sortable, filterable, pagination,
// pageSize). Kept camelCase for `pageSize` since it is
// programmatic (not DSL-author-facing).

import type { SchemaTypeExtension } from '../../schema/index.ts'
import type { UIColumnRef } from '../fields/index.ts'
import type { UIActionRef } from '../../ui-core/action/index.ts'
import type { UIFilterRef } from '../filter/index.ts'

export type UITableAttributes = {
  // Features
  sortable?: boolean
  filterable?: boolean

  // Display features
  withTableBorder?: boolean
  withColumnBorders?: boolean
  withRowBorders?: boolean
  highlightOnHover?: boolean
  striped?: boolean

  /** `compact`: dense aligned rows — no header, no zebra,
   *  1px separators, a rounded border around the list, a hover — that select
   *  or open; `normal` (the default) is the spreadsheet table. */
  density?: 'compact' | 'normal'
  /** Draw the header row (default true; a compact table draws none unless
   *  set true). */
  header?: boolean
  /** The header row stays in view while the rows scroll under it - in the
   *  table's own box when `maxHeight` is set, else in the page. */
  stickyHeader?: boolean
  /** The rows scroll in a box at most this high (pixels or a CSS length). */
  maxHeight?: number | string

  // Window: the first pageSize rows (default 10) and
  // « Afficher plus » for the next page; pagination false draws every row
  pagination?: boolean
  pageSize?: number

  // Optional template name (room for future per-table layout strategies;
  // unused in v1)
  template?: string
}

/** A row-menu entry — the shape of a page menu's entry (page/ui-page.interactive.ts
 *  UIMenuItem), declared here so the table depends on no page module. */
export type UITableMenuEntry = {
  id: string
  label: string
  uri?: string
  args?: Record<string, unknown>
  confirm?: string
  dangerous?: boolean
  when?: string
  prompt?: string
  choice?: string | { options?: { value: string; label?: string }[]; optionsField?: string }
  /** Drawn as its own small button in the row rather than under ⋯. */
  button?: boolean
  [k: string]: unknown
}

export type UITableDef = SchemaTypeExtension<{
  type: 'ui-table'

  /** Table name (multi-table support). Defaults to 'default'. */
  name?: string

  /** Columns to display. When undefined, all top-level fields render. */
  columns?: UIColumnRef[]

  /**
   * Filter integration:
   *  - false / undefined: no filters
   *  - true: look up the unnamed `ui-filter` extension on this schema
   *  - string: look up the named `ui-filter` extension
   *  - UIFilterRef[]: inline list of filters (no separate extension needed)
   */
  filters?: boolean | string | UIFilterRef[]

  /** Action refs (string id, UIActionDef, or UIAction). v1 keeps these
   *  but only emits them as `action` postMessages — bulk/row/toolbar menu
   *  rendering is deferred to v1.5. */
  actions?: UIActionRef[]

  /** A click on a row sets this key of the page's selection to the row's id
   *  (the page's `select` verb); the selected row is highlighted. On a page
   *  only. */
  select?: string

  /** The row's menu, drawn as its last cell: menu entries
   *  `{id, label, uri, args, confirm?, choice?, prompt?}` on the row —
   *  `${item.id}`, `${value}` the option chosen or the text typed, a choice
   *  from the row's field or `P0|P1`. On a page only. */
  menu?: UITableMenuEntry[]
}> &
  UITableAttributes

export type UITableRef = string | UITableDef

// SPDX-License-Identifier: Apache-2.0
// UITable runtime types — what the table rendering pipeline operates on.
//
// Ported from _ktw_workspace/.../cpm-ui/ui-table/ui-table.types.ts during
// gui-table Subtask 2 (2026-05-04). v1 trim: dropped the bulkMenu /
// rowMenu / toolboxMenu fields (those depend on the cpm-core/actions/menus
// renderers we cut in gui-form Subtask 1; deferred to gui-table v1.5).

import type { Schema } from '../../schema/index.ts'
import type { UIColumnDef, UIColumnRenderer, UIHeaderRenderer } from '../fields/index.ts'
import type { UIAction } from '../../ui-core/action/index.ts'
import type { UIFilters } from '../filter/index.ts'
import type { UITexts } from '../../ui-core/i18n/index.ts'
import type { UITableAttributes, UITableDef } from './ui-table.schema.ts'
import type { UIColumnExtra } from './ui-table.utils.tsx'

/**
 * Built column — what `buildColumn` returns. Carries the renderer pair
 * (header + cell) plus the per-column display attributes (label, width,
 * align, sortable, etc.) lifted from UIColumnDef.
 */
export type UIColumn<ItemData = any> = Omit<UIColumnDef, 'name' | 'type'> & {
  id: string
  renderHeader: UIHeaderRenderer
  renderCell: UIColumnRenderer<ItemData>
  /** The column field's content-type name (`text` for a plain string): a
   *  compact table draws plain text in its own row typography. */
  content?: string
  /** The fields drawn before / after the column's own value (its
   *  `prepend` / `append`); `renderCell` already draws them. */
  before?: UIColumnExtra<ItemData>[]
  after?: UIColumnExtra<ItemData>[]
  /** The fields drawn on a second line under the column's own value (its
   *  `below`); `renderCell` already draws them. */
  under?: UIColumnExtra<ItemData>[]
}

/**
 * UITable<TData> — the composed table runtime. Returned by `buildUITable()`,
 * consumed by `<UITableView>`. v1 omits bulk/row/toolbox menu renderers.
 */
export type UITable<TData = any> = UITableAttributes & {
  schema: Schema

  definition: UITableDef

  rowId: (row: TData) => string
  rowTitle: (row: TData) => string

  texts: UITexts

  actions: UIAction[]
  filters?: UIFilters
  columns: UIColumn<TData>[]
}

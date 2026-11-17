// SPDX-License-Identifier: Apache-2.0
// UITreeTable runtime types — what the tree-table rendering pipeline operates on.
// Reuses the ui-table `UIColumn` model; adds the tree linkage + display knobs.

import type { Schema } from '../../schema/index.ts'
import type { UITexts } from '../../ui-core/i18n/index.ts'
import type { UIColumn } from '../table/index.ts'
import type { UITreeTableDef } from './ui-tree-table.schema.ts'

/** UITreeTable<TData> — the composed tree-table runtime. Returned by
 *  `buildUITreeTable()`, consumed by `<UITreeTableView>`. */
export type UITreeTable<TData = any> = {
  schema: Schema
  definition: UITreeTableDef
  columns: UIColumn<TData>[]

  idField: string
  parentField: string
  stateField: string
  treeColumn?: string
  defaultExpanded: boolean
  indentPx: number

  texts: UITexts

  // Display flags (subset of UITableAttributes, resolved with defaults).
  withTableBorder?: boolean
  withColumnBorders?: boolean
  highlightOnHover?: boolean
  striped?: boolean
}

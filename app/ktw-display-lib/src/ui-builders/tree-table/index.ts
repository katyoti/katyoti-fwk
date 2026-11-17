// SPDX-License-Identifier: Apache-2.0
// Public surface of the tree-table package.

// DSL extension types
export type { UITreeTableDef, UITreeTableRef } from './ui-tree-table.schema.ts'

// Runtime types
export type { UITreeTable } from './ui-tree-table.types.ts'

// Builders
export {
  buildUITreeTable,
  buildUITreeTables,
  toUITreeTableDef,
  type BuildUITreeTableParams,
  type BuildUITreeTablesParams,
} from './ui-tree-table.builder.ts'

// React component
export { UITreeTableView, type UITreeTableViewProps } from './ui-tree-table.view.tsx'

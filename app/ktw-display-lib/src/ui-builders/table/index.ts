// SPDX-License-Identifier: Apache-2.0
// Public surface of the table package.

// DSL extension types
export type {
  UITableAttributes,
  UITableDef,
  UITableRef,
  UITableMenuEntry,
} from './ui-table.schema.ts'

// Runtime types
export type { UIColumn, UITable } from './ui-table.types.ts'

// Builders
export {
  buildUITable,
  buildUITables,
  buildColumns,
  toUITable,
  toUITableDef,
  type BuildUITableParams,
  type BuildUITablesParams,
  type UITableOverrides,
} from './ui-table.builder.ts'

// React component
export { UITableView, type UITableViewProps } from './ui-table.view.tsx'

// The compact rows (density: compact) and their pure helpers.
export { CompactRow, compactGrid, compactRoles, rowClickKind, type CompactRole } from './ui-table.compact.tsx'

// Cell composition utilities (for layouts that want to merge multiple
// fields into one cell, e.g. avatar + name).
export { joinCell, wrapColumnRenderers, wrapHeaderRenderers, type UIColumnExtra } from './ui-table.utils.tsx'

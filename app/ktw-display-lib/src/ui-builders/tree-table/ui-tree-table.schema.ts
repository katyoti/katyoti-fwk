// SPDX-License-Identifier: Apache-2.0
// UITreeTableDef — the DSL `ui-tree-table` extension shape.
//
// A tree-table (treegrid) renders a flat node list linked by a parent-id field
// as nested, indented rows with expand/collapse — the confluence-TUI page table
// with status columns. Standalone subsystem (mirrors `ui-table`; reuses its
// column model) so the shipped table widget carries no regression risk.

import type { SchemaTypeExtension } from '../../schema/index.ts'
import type { UIColumnRef } from '../fields/index.ts'
import type { UITableAttributes } from '../table/index.ts'

export type UITreeTableDef = SchemaTypeExtension<{
  type: 'ui-tree-table'

  /** Tree-table name (multi-variant support). Defaults to 'default'. */
  name?: string

  /** Columns to display. When undefined, all top-level fields render. */
  columns?: UIColumnRef[]

  /** Field holding each node's id. Default 'id'. */
  idField?: string

  /** Field linking to the parent node's id (empty / unknown ⇒ root). Default 'parent_id'. */
  parentField?: string

  /** Field whose value renders as a colored status badge. Default 'state'. */
  stateField?: string

  /** Column id that carries the tree indent + expand/collapse chevron. Default
   *  the first column. Set this when the hierarchy should render on a column
   *  other than the leftmost (e.g. version/direction columns come first, the
   *  title column holds the tree). */
  treeColumn?: string

  /** Start with all nodes expanded. Default true. */
  defaultExpanded?: boolean

  /** Indent per depth level, in px. Default 20. */
  indentPx?: number
}> &
  UITableAttributes

export type UITreeTableRef = string | UITreeTableDef

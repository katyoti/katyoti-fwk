// SPDX-License-Identifier: Apache-2.0
// UIFilterDef + UIFilterToolboxDef — the DSL shapes for the `ui-filter` extension.
//
// Ported from _ktw_workspace/.../cpm-ui/ui-filter/ui-filter.schema.ts during
// gui-table Subtask 1 (2026-05-04). No DSL renames needed.
//
// Schema authors attach via:
//   extensions: [
//     { type: 'ui-filter', name: 'default', tabs: 'status', filters: [...] }
//   ]
// then reference from the table:
//   extensions: [{ type: 'ui-table', filters: true }]   // → look up unnamed
//   extensions: [{ type: 'ui-table', filters: 'mySearch' }]  // → look up named
//   extensions: [{ type: 'ui-table', filters: ['name', { id: 'role' }] }]  // → inline

import type {
  SchemaFieldName,
  SchemaTypeExtension,
  SchemaTypeName,
} from '../../schema/index.ts'

/** Per-filter declarative shape. `id` picks the schema field; `name`
 *  optionally renames the RHF field key (lets the same field appear
 *  as multiple filters with different renderers). */
export type UIFilterDef = {
  id: string

  /** Defaults to id, but you can specify a different field name so a
   *  field can be displayed multiple times with different renderers. */
  name?: SchemaFieldName

  /** Schema typeName — normally we use the field's type, but you can
   *  define a specific type to display the field in a different way. */
  type?: SchemaTypeName

  label?: string
}

export type UIFilterRef = string | UIFilterDef

/**
 * UIFilterToolboxDef — the schema's `ui-filter` extension. Authors can
 * declare multiple named filter toolboxes per schema; the table extension
 * picks one by name (or the unnamed default).
 */
export type UIFilterToolboxDef = SchemaTypeExtension<{
  type: 'ui-filter'

  /** Filter-toolbox name in case of multi-toolbox. */
  name?: string

  /**
   * Field name to use as a key for tabs. Renders as a top-row Mantine
   * Tabs component; clicking a tab filters the dataset by that value.
   * Default: undefined (no tabs).
   */
  tabs?: UIFilterRef

  /**
   * Field names to use as toolbar filters. Renders below the tabs
   * (when `tabs:` is set) or directly as the toolbar.
   * Default: undefined (no filters).
   */
  filters?: UIFilterRef[]
}>

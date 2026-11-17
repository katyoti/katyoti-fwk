// SPDX-License-Identifier: Apache-2.0
// UIFilter runtime types — what the filter pipeline operates on.
//
// Ported from _ktw_workspace/.../cpm-ui/ui-filter/ui-filter.types.ts during
// gui-table Subtask 1 (2026-05-04). No DSL renames needed (filter shape
// pre-dates the snake_case migration; nothing to rename).

import type { ReactNode } from 'react'
import type { FieldOption } from '../../schema/index.ts'

/**
 * Info used to create filters — populated by the builder from the schema's
 * field type + the filter ref's id/name. Drives the per-kind renderer
 * factory (search / options / tabs).
 */
export type UIFilterInfo = {
  name: string
  label: string
  type: 'tabs' | 'options' | 'search'
  options?: FieldOption[]
  allOption?: FieldOption
}

/**
 * Per-filter handler — knows the default value, predicates the
 * "this row matches the filter" check, and applies the filter to a row
 * array. Implemented per kind (option-handler for tabs+options,
 * string-handler for search).
 */
export type UIFilterHandler<DataType = any, T = any> = {
  defaultValue: DataType
  isFiltered(value: DataType): boolean
  filter(value: DataType, inputData: T[]): T[]
}

/** Map of field-name → current filter value (driven by react-hook-form). */
export type UIFilterValues = Record<string, any>

/** Filter renderer — emits the controlled input(s) for one filter. */
export type UIFilterRenderer = () => ReactNode

/**
 * Built filter — what `buildUIFilter()` returns. Carries the per-instance
 * `name` (RHF field name), the renderer thunk, and the matching handler.
 */
export type UIFilter<DataType extends string | number | boolean = any> = {
  name: string
  renderer: UIFilterRenderer
  handler: UIFilterHandler<DataType>
}

/**
 * Top-level filter bundle — what the table builder passes into the
 * filter hook. Splits a top-row tab filter (single-select discriminator
 * over the full dataset) from a toolbar of additional filters.
 */
export type UIFilters = {
  filterResetButton?: string
  tabFilter?: UIFilter
  toolbarFilters?: UIFilter[]
}

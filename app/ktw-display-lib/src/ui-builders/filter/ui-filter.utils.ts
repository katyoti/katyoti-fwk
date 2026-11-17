// SPDX-License-Identifier: Apache-2.0
// Filter pipeline utilities — apply / merge / default-value helpers.
//
// Ported from _ktw_workspace/.../cpm-ui/ui-filter/ui-filter.utils.ts during
// gui-table Subtask 1 (2026-05-04). No semantic changes.

import type { JsonObject } from '../../types/json-types.ts'
import type { UIFilter, UIFilterValues } from './ui-filter.types.ts'

export function mergeFilters(tabFilter?: UIFilter, toolbarFilters?: UIFilter[]): UIFilter[] {
  return [...(tabFilter ? [tabFilter] : []), ...(toolbarFilters ? toolbarFilters : [])]
}

export function createFilterDefaultValue(filters: UIFilter[]): UIFilterValues {
  const defaultValue: UIFilterValues = {}
  for (const filter of filters) {
    defaultValue[filter.name] = filter.handler.defaultValue
  }
  return defaultValue
}

/**
 * Apply tab + toolbar filters to a row array. Each filter's handler
 * narrows the input; the optional `comparator` performs a stable
 * pre-sort BEFORE filtering.
 */
export function applyFilter({
  values,
  inputData,
  comparator,
  filters,
}: {
  values: UIFilterValues
  inputData: JsonObject[]
  comparator?: (a: any, b: any) => number
  filters: UIFilter[]
}): JsonObject[] {
  if (comparator) {
    const stabilized = inputData.map((el, index) => [el, index] as const)
    stabilized.sort((a, b) => {
      const order = comparator(a[0], b[0])
      if (order !== 0) return order
      return a[1] - b[1]
    })
    inputData = stabilized.map((el) => el[0])
  }

  for (const filter of filters) {
    const value = values[filter.name]
    inputData = filter.handler.filter(value, inputData)
  }
  return inputData
}

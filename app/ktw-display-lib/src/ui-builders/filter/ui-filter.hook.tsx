// SPDX-License-Identifier: Apache-2.0
// useUIFilters — React hook that wires a UIFilters bundle through
// react-hook-form, exposes the filtering callback, the resetFilters
// action, the isFiltered flag, and a memoized FilterToolbar component.
//
// Ported from _ktw_workspace/.../cpm-ui/ui-filter/ui-filter.hook.tsx during
// gui-table Subtask 1 (2026-05-04). Adaptations:
//   - `@core/cpm-core/forms` → `'react-hook-form'`
//   - Reset wiring: hook owns the reset logic (was inline-implemented in toolbar);
//     toolbar receives `onResetFilter` callback for parity with the form-side pattern.

import { useCallback, useEffect, useMemo, useState } from 'react'
import type { ComponentType, ReactNode } from 'react'

import { useForm } from 'react-hook-form'
import { UIFilterToolbar } from './ui-filter.toolbar.tsx'
import type { UIFilterValues, UIFilters } from './ui-filter.types.ts'
import { applyFilter, createFilterDefaultValue, mergeFilters } from './ui-filter.utils.ts'

export interface UseUIFiltersResult {
  /** Apply current filter values to a row array; returns the filtered subset. */
  filter: (inputData: any[]) => any[]
  /** Reset all filter values to their defaults. */
  resetFilters: () => void
  /** True iff any toolbar filter currently differs from its default. */
  isFiltered: boolean
  /** The toolbar component, memoized against filter identities. Mount it
   *  above the table to render the filter chrome. */
  FilterToolbar: ComponentType<{ children?: ReactNode }>
}

export function useUIFilters({
  filters,
  comparator,
}: {
  filters?: UIFilters
  comparator?: (a: any, b: any) => number
}): UseUIFiltersResult {
  const { tabFilter, toolbarFilters = [], filterResetButton } = filters || {}
  const allFilters = mergeFilters(tabFilter, toolbarFilters)

  const defaultValues: UIFilterValues = useMemo(
    () => createFilterDefaultValue(allFilters),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [...allFilters],
  )
  const [isFiltered, setIsFiltered] = useState(false)

  const methods = useForm<UIFilterValues>({ defaultValues })
  const { watch, getValues, setValue, reset } = methods

  const updateIsFiltered = useCallback(() => {
    const values = getValues()
    let result = false
    for (const filter of toolbarFilters) {
      const value = values[filter.name]
      if (filter.handler.isFiltered(value)) {
        result = true
        break
      }
    }
    setIsFiltered((prev) => (prev !== result ? result : prev))
  }, [getValues, toolbarFilters])

  const resetFilters = useCallback(() => {
    for (const filter of toolbarFilters) {
      setValue(filter.name, filter.handler.defaultValue)
    }
    if (tabFilter) {
      setValue(tabFilter.name, tabFilter.handler.defaultValue)
    }
    updateIsFiltered()
  }, [toolbarFilters, tabFilter, setValue, updateIsFiltered])

  const values = watch()

  useEffect(() => {
    updateIsFiltered()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [JSON.stringify(values)])

  const filter = useCallback(
    (inputData: any[]) =>
      applyFilter({ values, inputData, comparator, filters: allFilters }),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [JSON.stringify(values), comparator, ...allFilters],
  )

  const FilterToolbar: ComponentType<{ children?: ReactNode }> = useMemo(
    () => {
      return function FilterToolbarComponent({ children }: { children?: ReactNode }) {
        return (
          <UIFilterToolbar
            key="ui-filter-toolbar"
            methods={methods}
            tabFilter={tabFilter}
            toolbarFilters={toolbarFilters}
            isFiltered={isFiltered}
            filterResetButton={filterResetButton}
            onResetFilter={() => {
              reset()
              resetFilters()
            }}
          >
            {children}
          </UIFilterToolbar>
        )
      }
    },
    [methods, tabFilter, toolbarFilters, isFiltered, filterResetButton, reset, resetFilters],
  )

  return {
    filter,
    resetFilters,
    isFiltered,
    FilterToolbar,
  }
}

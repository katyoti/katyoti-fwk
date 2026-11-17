// SPDX-License-Identifier: Apache-2.0
// Public surface of the filter package — DSL extension types, runtime
// types, builders, the React hook, the toolbar component, and the
// per-kind renderers.

export type {
  UIFilter,
  UIFilterHandler,
  UIFilterInfo,
  UIFilterRenderer,
  UIFilterValues,
  UIFilters,
} from './ui-filter.types.ts'

export type { UIFilterDef, UIFilterRef, UIFilterToolboxDef } from './ui-filter.schema.ts'

export {
  buildUIFilters,
  buildUIFilterToolbox,
  buildUIFilterList,
  buildUITabFilter,
  buildUIToolbarFilter,
} from './ui-filter.builder.ts'

export { createFilter } from './ui-filter.factory.ts'

export {
  useUIFilters,
  type UseUIFiltersResult,
} from './ui-filter.hook.tsx'

export { UIFilterToolbar } from './ui-filter.toolbar.tsx'

// Re-export per-kind renderers so authors writing custom filter renderers
// can compose them.
export {
  SearchFilterRenderer,
  OptionFilterRenderer,
  TabsFilterRenderer,
  createFilterRenderer,
} from './renderers/index.ts'

export { createFilterHandler } from './handlers/index.ts'

export { applyFilter, createFilterDefaultValue, mergeFilters } from './ui-filter.utils.ts'

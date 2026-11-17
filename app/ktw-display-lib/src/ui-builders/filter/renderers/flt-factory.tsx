// SPDX-License-Identifier: Apache-2.0
// Filter renderer factory — switches on UIFilterInfo.type to produce the
// right thunk renderer (search / options / tabs).

import type { FieldOption } from '../../../schema/index.ts'
import type { UIFilterInfo, UIFilterRenderer } from '../ui-filter.types.ts'

import { OptionFilterRenderer } from './flt-options.tsx'
import { SearchFilterRenderer } from './flt-search.tsx'
import { TabsFilterRenderer } from './flt-tabs.tsx'

type UIFilterRendererFactory = (def: UIFilterInfo) => UIFilterRenderer

const flt_tabs: UIFilterRendererFactory = ({ name, allOption, options }) => {
  if (!options) throw new Error(`${name}: options must be defined for 'tabs' filter`)
  if (!allOption) throw new Error(`${name}: allOption must be defined for 'tabs' filter`)
  return () => (
    <TabsFilterRenderer
      fieldName={name}
      allOption={allOption as FieldOption<string | number>}
      options={options as FieldOption<string | number>[]}
    />
  )
}

const flt_options: UIFilterRendererFactory = ({ name, label, allOption, options }) => {
  if (!options) throw new Error(`${name}: options must be defined for 'options' filter`)
  if (!allOption) throw new Error(`${name}: allOption must be defined for 'options' filter`)
  return () => <OptionFilterRenderer fieldName={name} fieldLabel={label} allOption={allOption} options={options} />
}

const flt_search: UIFilterRendererFactory = ({ name, label }) => {
  return () => <SearchFilterRenderer fieldName={name} fieldLabel={label} />
}

const factories: Record<UIFilterInfo['type'], UIFilterRendererFactory> = {
  tabs: flt_tabs,
  options: flt_options,
  search: flt_search,
}

export function createFilterRenderer(def: UIFilterInfo): UIFilterRenderer {
  const factory = factories[def.type]
  if (factory) return factory(def)
  throw new Error(`Unknown filter renderer ${def.type} for field ${def.name}`)
}

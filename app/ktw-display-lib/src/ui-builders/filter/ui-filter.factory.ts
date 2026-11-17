// SPDX-License-Identifier: Apache-2.0
// Per-filter factory — composes a UIFilter from a UIFilterInfo by
// pairing the renderer (UI layer) with the handler (logic layer).

import { createFilterHandler } from './handlers/index.ts'
import { createFilterRenderer } from './renderers/index.ts'
import type { UIFilter, UIFilterInfo } from './ui-filter.types.ts'

export function createFilter(def: UIFilterInfo): UIFilter {
  return {
    name: def.name,
    renderer: createFilterRenderer(def),
    handler: createFilterHandler(def),
  }
}

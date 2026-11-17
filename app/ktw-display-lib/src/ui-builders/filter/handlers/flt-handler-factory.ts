// SPDX-License-Identifier: Apache-2.0
// Per-filter handler factory — switches on UIFilterInfo.type to produce
// the right handler (tabs/options → OptionFilterHandler; search → StringFilterHandler).
//
// Ported from _ktw_workspace/.../cpm-ui/ui-filter/handlers/flt-handler-factory.ts during
// gui-table Subtask 1 (2026-05-04). No semantic changes.

import type { FieldOptionKey } from '../../../schema/index.ts'
import type { JsonObject } from '../../../types/json-types.ts'

import type { UIFilterHandler, UIFilterInfo } from '../ui-filter.types.ts'

export function createFilterHandler(def: UIFilterInfo): UIFilterHandler {
  switch (def.type) {
    case 'options':
    case 'tabs':
      if (!def.allOption) throw new Error(`Missing allOption for filter ${def.name}`)
      return new OptionFilterHandler(def.name, def.allOption.value)
    case 'search':
      return new StringFilterHandler(def.name)
    default:
      throw new Error(`Unknown filter type: ${(def as { type: string }).type}`)
  }
}

class OptionFilterHandler<FilterType extends FieldOptionKey> implements UIFilterHandler<FilterType> {
  constructor(
    public readonly fieldName: string,
    public readonly defaultValue: FilterType,
  ) {}

  public isFiltered(value: FilterType): boolean {
    return value !== this.defaultValue
  }

  public filter(value: FilterType, inputData: JsonObject[]): JsonObject[] {
    if (value !== this.defaultValue) {
      inputData = inputData.filter((item) => item[this.fieldName] === value)
    }
    return inputData
  }
}

class StringFilterHandler implements UIFilterHandler<string> {
  public readonly defaultValue: string = ''

  constructor(public readonly fieldName: string) {}

  public filter(value: string, inputData: JsonObject[]): JsonObject[] {
    if (value) {
      inputData = inputData.filter((item) =>
        ('' + (item[this.fieldName] || '')).toLowerCase().indexOf(value.toLowerCase()) !== -1,
      )
    }
    return inputData
  }

  public isFiltered(value: string): boolean {
    return value !== undefined && value.trim() !== ''
  }
}

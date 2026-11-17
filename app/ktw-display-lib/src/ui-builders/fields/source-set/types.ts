// SPDX-License-Identifier: Apache-2.0
// SourceSetContent - `content: {type: 'source-set', connectors, object}`: an
// array of the items picked, each with its connector, the pickers driven by
// the connectors the `connectors` method lists.

import type { ContentType } from '../../../schema/schema-contents.ts'
import type { UIPickerBadge, UIPickerItem } from '../../page/ui-page.interactive.ts'

export interface SourceSetContent extends ContentType {
  type: 'source-set'
  /** The method listing the connectors: `{object?}` -> `{connectors}`. */
  connectors: string
  /** The object the sources are for. */
  object?: string
  /** A note under the field in a form - what happens at creation.
 */
  note?: string
}

/** A connector as the `connectors` method answers it. */
export interface SourceConnector {
  name: string
  label?: string
  /** Its search method: `{object?, query}` -> `{items}` - where it declares
   *  a search. */
  search?: string
  /** Its browse method, « Parcourir »: `{object?, parent?}` -> `{items}` -
   *  where it browses. */
  browse?: string
  badges?: Record<string, UIPickerBadge>
  layout?: 'list' | 'tree'
  /** Laid over every call of its picker (sources' Pickers: the connector). */
  args?: Record<string, unknown>
  /** What one item is (application, page) and what is read with each. */
  unit?: string
  derived?: string
  /** Its status method and where it is configured. */
  status?: string
  configure?: { label: string; hint?: string; keys?: string[] }
  /** Its icon and colour, as every view draws it. */
  icon?: string
  color?: string
}

/** An item of the value: a picked item and its connector. */
export interface SourceSetItem extends UIPickerItem {
  connector: string
}

// SPDX-License-Identifier: Apache-2.0
// Shared utilities for renderer factories — column text/style/header helpers.
// Ported from _ktw_workspace/old_ui_main/.../renderer-utils.tsx with the
// param-shape unification (`{ def, info, texts }`).

import { Text, Tooltip } from '@mantine/core'
import type { JsonObject } from '../../types/json-types.ts'
import type { CrudFieldInfo } from './info.ts'
import type { HeaderRendererFactory } from './renderer.ts'
import type {
  ContentColor,
  ContentColorPredefined,
  LabelVariant,
  UIColumnStyle,
} from './types.ts'

export const COLOR_MAP: Record<ContentColorPredefined, ContentColor> = {
  primary: 'blue',
  secondary: 'gray',
  info: 'yellow',
  success: 'green',
  warning: 'orange',
  error: 'red',
}

export function translateColor(color: ContentColor | undefined): ContentColor | undefined {
  if (!color) return undefined
  if (COLOR_MAP[color as ContentColorPredefined]) return COLOR_MAP[color as ContentColorPredefined]
  return color
}

/** Default header renderer — common implementation reused by every renderer
 *  that doesn't need a custom header. Reads label + tooltip from
 *  `info.label` / `def.tooltip`. */
export const createHeaderRenderer: HeaderRendererFactory<any, any> = ({ def, info }) => {
  const { label } = info
  const { tooltip } = def
  if (tooltip) {
    return () => (
      <Tooltip label={tooltip}>
        <Text fw={600}>{label}</Text>
      </Tooltip>
    )
  }
  return () => <Text fw={600}>{label}</Text>
}

/** Resolve cell text from the row, honoring the info's `options` map. */
export function columnText(
  info: CrudFieldInfo,
  row: JsonObject,
  fieldName?: string,
): string {
  const key = fieldName ?? info.name
  const raw = row[key]
  const value = raw === undefined || raw === null ? '' : String(raw)
  if (info.options && info.options[value] !== undefined) return info.options[value]
  return value
}

/** Resolve cell text + per-row color/variant from a colorMap/variantMap. */
export function columnTextWithColor(
  info: CrudFieldInfo,
  row: JsonObject,
  colorField: string | undefined,
  colorMap: Record<string, ContentColor | undefined> | undefined,
  variantField?: string,
  variantMap?: Record<string, LabelVariant | undefined>,
): {
  value: string
  color: ContentColor | undefined
  variant: LabelVariant | undefined
} {
  const raw = row[info.name]
  const value = raw === undefined || raw === null ? '' : String(raw)
  const colorValue = colorField
    ? row[colorField] === undefined || row[colorField] === null
      ? ''
      : String(row[colorField])
    : value
  const color = colorMap ? colorMap[colorValue.toLowerCase()] || colorMap['*'] : undefined
  const variantValue = variantField
    ? row[variantField] === undefined || row[variantField] === null
      ? ''
      : String(row[variantField])
    : value
  const variant = variantMap
    ? variantMap[variantValue.toLowerCase()] || variantMap['*']
    : undefined
  const realValue = info.options ? info.options[value] ?? value : value
  return {
    value: realValue,
    color: translateColor(color),
    variant,
  }
}

/** Convert UIColumnStyle[] to React.CSSProperties. */
export function columnStyles(info: CrudFieldInfo): React.CSSProperties | undefined {
  const effects: React.CSSProperties = {}
  if (info.styles) {
    for (const style of info.styles) {
      switch (style as UIColumnStyle) {
        case 'bold':
          effects.fontWeight = 'bold'
          break
        case 'italic':
          effects.fontStyle = 'italic'
          break
        case 'underline':
          effects.textDecoration = 'underline'
          break
        case 'strike':
          effects.textDecoration = 'line-through'
          break
        case 'overline':
          effects.textDecoration = 'overline'
          break
        case 'upper-first':
          effects.textTransform = 'capitalize'
          break
        case 'uppercase':
          effects.textTransform = 'uppercase'
          break
        case 'lowercase':
          effects.textTransform = 'lowercase'
          break
        case 'capitalize':
          effects.textTransform = 'capitalize'
          break
      }
    }
  }
  return Object.keys(effects).length > 0 ? effects : undefined
}

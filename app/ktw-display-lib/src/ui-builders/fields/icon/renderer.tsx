// SPDX-License-Identifier: Apache-2.0
// icon content renderer — the value is a Tabler icon name drawn as that icon
// (a table cell, a column's `prepend` mark such as a row's open mark); an
// unknown name draws a dot, an empty value nothing. The form edits the name as
// text.

import { IconPointFilled } from '@tabler/icons-react'
import TextRenderer from '../text/renderer.tsx'
import {
  createHeaderRenderer,
  type ColumnRendererFactory,
  type FieldRendererFactory,
  type FormFieldRendererFactory,
  type ViewFieldRendererFactory,
} from '../index.ts'
import type { JsonObject } from '../../../types/json-types.ts'
import { tablerIcon } from '../../../ui-core/components/icons/byName.tsx'
import type { IconContent } from './types.ts'

/** iconColor is the icon's color from the content's `colorMap`, keyed by the
 *  `colorField` sibling's value (or the icon's own); a Mantine color name
 *  becomes its shade-6 variable. */
export function iconColor(content: IconContent, item: JsonObject, value: string): string | undefined {
  if (!content.colorMap) return undefined
  let map: Record<string, string>
  try { map = JSON.parse(content.colorMap) } catch { return undefined }
  const key = content.colorField ? item?.[content.colorField] : value
  const c = key === undefined || key === null ? undefined : map[String(key)]
  if (!c) return undefined
  return /^[a-z]+$/.test(c) ? `var(--mantine-color-${c}-6)` : c
}

function IconCell({ content, item, name, testId }: { content: IconContent; item: JsonObject; name: string; testId: string }) {
  const value = item?.[name]
  if (typeof value !== 'string' || value.trim() === '') return null
  const Icon = tablerIcon(value.trim(), IconPointFilled)
  return <Icon size={16} stroke={1.75} color={iconColor(content, item, value)} aria-hidden data-testid={testId} style={{ verticalAlign: 'middle', flexShrink: 0 }} />
}

const view: ViewFieldRendererFactory<any, IconContent> = ({ info }) => {
  return (item: JsonObject) => <IconCell content={info.contentType} item={item} name={info.name} testId={'view.' + info.name} />
}

const form: FormFieldRendererFactory<any, IconContent> = (p) => TextRenderer.form(p as never)

const column: ColumnRendererFactory<any, IconContent> = ({ info }) => {
  return (item: JsonObject) => <IconCell content={info.contentType} item={item} name={info.name} testId={'column.' + info.name} />
}

const IconRenderer: FieldRendererFactory<any, IconContent> = {
  view,
  form,
  header: createHeaderRenderer,
  column,
}

export default IconRenderer

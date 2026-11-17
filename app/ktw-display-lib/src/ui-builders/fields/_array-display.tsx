// SPDX-License-Identifier: Apache-2.0
// Shared plumbing for array-of-objects field renderers (grid | table | tree).
//
// An array field's `content` picks how its item objects display:
//   grid  → object cards (the object renderer)
//   table → flat table  (ui-table)
//   tree  → tree-table   (ui-tree-table)
//
// table/tree resolve a display extension from `content.extension`:
//   string  → the ITEM object type's named extension of the matching kind
//   object  → an inline extension spec
//   absent  → a synthesized default (columns discovered from the item fields)
//
// FORM always delegates to the object renderer (basic card editor) for now —
// hierarchical/tabular data isn't hand-edited inline yet (enhance later).

import { Text } from '@mantine/core'
import type { ReactNode } from 'react'

import {
  createHeaderRenderer,
  type ColumnRendererFactory,
  type FieldRendererFactory,
  type ViewFieldRendererFactory,
} from './index.ts'
import type { CrudFieldInfo } from './info.ts'
import ObjectRenderer from './object/renderer.tsx'
import type { JsonObject } from '../../types/json-types.ts'
import type { Schema, SchemaType } from '../../schema/schema-types.ts'
import type { ContentType } from '../../schema/schema-contents.ts'
import type { UITexts } from '../../ui-core/i18n/types.ts'

export function arrayRows(item: JsonObject, name: string): JsonObject[] {
  const v = (item ?? {})[name]
  return Array.isArray(v) ? (v as JsonObject[]) : []
}

/** Resolve the display-extension def for an array field (extType e.g.
 *  'ui-tree-table' / 'ui-table'): inline object, a named extension on the item
 *  object type, or a synthesized default. */
export function resolveArrayExt(info: CrudFieldInfo, extType: string): Record<string, unknown> {
  const ext = (info.contentType as { extension?: unknown }).extension
  if (ext && typeof ext === 'object' && !Array.isArray(ext)) {
    return { type: extType, name: 'default', ...(ext as Record<string, unknown>) }
  }
  if (typeof ext === 'string') {
    const named = ((info.type as { extensions?: Array<Record<string, unknown>> }).extensions ?? []).find(
      (e) => e.type === extType && e.name === ext,
    )
    if (named) return named
  }
  return { type: extType, name: 'default' }
}

/** Wrap the item object type in a synthetic array schema carrying the resolved
 *  display extension, so the standalone ui-table / ui-tree-table builders run
 *  unchanged. */
function miniSchema(info: CrudFieldInfo, extDef: Record<string, unknown>): Schema {
  return {
    name: 'array-field',
    version: 1,
    root: '__arr_root__',
    types: {
      __arr_root__: { type: 'array', items: '__arr_item__', extensions: [extDef] },
      __arr_item__: info.type,
    },
  } as unknown as Schema
}

/** Build a FieldRendererFactory for an array-of-objects display kind: the view
 *  builds + renders via the given builder/view; the form delegates to the
 *  object renderer (basic grid editor); column is a compact count. */
export function makeArrayFieldRenderer(opts: {
  extType: string
  /** `ref` is the resolved extension's name: the builder must look it up by
   *  name (an unnamed lookup misses it and falls back to every item field). */
  build: (p: { schema: Schema; texts: UITexts; data: unknown[]; ref: string }) => unknown
  render: (built: unknown, rows: JsonObject[]) => ReactNode
}): FieldRendererFactory<SchemaType, ContentType> {
  const view: ViewFieldRendererFactory<SchemaType, ContentType> =
    ({ info, texts }) =>
    (item: JsonObject) => {
      const rows = arrayRows(item, info.name)
      if (rows.length === 0) return <Text c="dimmed">(none)</Text>
      const extDef = resolveArrayExt(info, opts.extType)
      const built = opts.build({ schema: miniSchema(info, extDef), texts, data: rows, ref: String(extDef.name) })
      return opts.render(built, rows)
    }

  const column: ColumnRendererFactory<SchemaType, ContentType> =
    ({ info }) =>
    (item: JsonObject) => {
      const n = arrayRows(item, info.name).length
      return (
        <Text size="sm" c="dimmed">
          {n} item{n === 1 ? '' : 's'}
        </Text>
      )
    }

  return { view, form: ObjectRenderer.form, header: createHeaderRenderer, column }
}

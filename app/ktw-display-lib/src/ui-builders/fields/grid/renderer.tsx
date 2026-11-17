// SPDX-License-Identifier: Apache-2.0
// grid content renderer — an array-of-objects FIELD renders as a grid of object
// cards. The DEFAULT array-of-objects display (info.ts routes here when no
// content, or `content:{type:'grid'}`).
//
// `content.extension` selects how each card renders, via a `ui-view` of the item
// type (per the grid↔ui-view mapping):
//   string  → the item type's named `ui-view`
//   object  → an inline ui-view spec
//   absent  → the object renderer's generic card list (unchanged default)
//
// FORM delegates to the object renderer (basic card editor).

import { Card, Stack, Text } from '@mantine/core'

import { buildUIViews, UIViewView } from '../../view/index.ts'
import {
  createHeaderRenderer,
  type FieldRendererFactory,
  type ViewFieldRendererFactory,
} from '../index.ts'
import ObjectRenderer from '../object/renderer.tsx'
import { arrayRows } from '../_array-display.tsx'
import type { JsonObject } from '../../../types/json-types.ts'
import type { Schema, SchemaType } from '../../../schema/schema-types.ts'
import type { ContentType } from '../../../schema/schema-contents.ts'

const view: ViewFieldRendererFactory<SchemaType, ContentType> = (params) => {
  const { info, texts } = params
  const ext = (info.contentType as { extension?: unknown }).extension

  // No extension → the object renderer's generic card list (default, no change).
  if (ext === undefined || ext === null) {
    return ObjectRenderer.view(params)
  }

  return (item: JsonObject) => {
    const rows = arrayRows(item, info.name)
    if (rows.length === 0) return <Text c="dimmed">(none)</Text>

    let itemType = info.type as unknown as Record<string, unknown>
    let viewName = 'default'
    if (typeof ext === 'string') {
      viewName = ext
    } else if (typeof ext === 'object') {
      itemType = {
        ...itemType,
        extensions: [
          ...((itemType.extensions as unknown[]) ?? []),
          { type: 'ui-view', name: 'default', ...(ext as object) },
        ],
      }
    }

    const schema = {
      name: 'grid-item',
      version: 1,
      root: '__item__',
      types: { __item__: itemType },
    } as unknown as Schema
    const views = buildUIViews({ schema, texts })
    const v = views[viewName] ?? views['default']

    return (
      <Stack gap="xs">
        {rows.map((r, i) => (
          <Card key={i} withBorder padding="sm" radius="sm">
            <UIViewView view={v} item={r as Record<string, unknown>} />
          </Card>
        ))}
      </Stack>
    )
  }
}

const GridRenderer: FieldRendererFactory<SchemaType, ContentType> = {
  view,
  form: ObjectRenderer.form,
  header: createHeaderRenderer,
  column: ObjectRenderer.column,
}

export default GridRenderer

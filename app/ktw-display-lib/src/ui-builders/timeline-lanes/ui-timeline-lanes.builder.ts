// SPDX-License-Identifier: Apache-2.0
// UITimelineLanes builder — composes a lane schema and its
// `ui-timeline-lanes` extension into what the view draws: the lane header
// view (a ui-view variant of the lane type), the item card view (a ui-view
// variant of the items field's element type), the backlog's, and how each
// names its id and title.

import i18next from 'i18next'

import type { Schema } from '../../schema/index.ts'
import { SchemaQuery } from '../../schema/index.ts'
import type { UITexts } from '../../ui-core/i18n/index.ts'
import { makeUITexts } from '../../ui-core/i18n/index.ts'
import { buildUIView, type UIView } from '../view/index.ts'
import type { UIViewDef } from '../view/index.ts'
import type { UITimelineLanesDef, UITimelineLanesRef } from './ui-timeline-lanes.schema.ts'
import { elementSchema } from './ui-timeline-lanes.utils.ts'

type Row = Record<string, unknown>

/** The built lane timeline. */
export interface UITimelineLanes {
  schema: Schema
  definition: UITimelineLanesDef
  laneId: (lane: Row) => string
  laneTitle: (lane: Row) => string
  /** The lane header's view — absent when the def names none or the lane
   *  type declares no such variant. */
  header?: UIView
  /** The items' element type, as its own schema. */
  itemSchema?: Schema
  itemId: (item: Row) => string
  itemTitle: (item: Row) => string
  /** The item card's view. */
  itemView?: UIView
  backlogId: (entry: Row) => string
  /** The backlog entries' view (the chip's hover card). */
  backlogView?: UIView
}

const byId = (row: Row) => String(row?.id ?? '')

/** toUITimelineLanesDef resolves a ref — a variant name, absent for the
 *  default one, or a definition inline — to the extension of the root type. */
export function toUITimelineLanesDef(schema: Schema, ref?: UITimelineLanesRef): UITimelineLanesDef | undefined {
  if ((ref === undefined || ref === null || typeof ref === 'string') && schema !== undefined) {
    const name = ref ?? 'default'
    const all = SchemaQuery.getSchemaExtension<UITimelineLanesDef>(schema, undefined, 'ui-timeline-lanes')
    return all.find((e) => (e.name || 'default') === name)
  }
  if (ref && typeof ref === 'object' && ref.type === 'ui-timeline-lanes') return ref
  return undefined
}

/** namedView builds a declared ui-view variant of a schema's root type;
 *  undefined when the variant is not declared (no synthesized default: a
 *  card is never every field). */
function namedView(schema: Schema | undefined, variant: string | undefined, texts: UITexts): UIView | undefined {
  if (!schema || !variant) return undefined
  let def: UIViewDef | undefined
  try {
    def = SchemaQuery.getSchemaExtension<UIViewDef>(schema, undefined, 'ui-view').find((e) => (e.name || 'default') === variant)
  } catch { return undefined }
  if (!def) return undefined
  return buildUIView({ schema, viewRef: def, texts, actionRefs: [] })
}

function idAndTitle(schema: Schema | undefined): { id: (r: Row) => string; title: (r: Row) => string } {
  if (!schema) return { id: byId, title: byId }
  try {
    const { idField, titleField } = SchemaQuery.getIdAndTitleField<Row>(schema as never)
    return { id: idField, title: titleField }
  } catch {
    return { id: byId, title: byId }
  }
}

/** toUITimelineLanes builds a definition's views on the schema. */
export function toUITimelineLanes(schema: Schema, definition: UITimelineLanesDef, texts: UITexts): UITimelineLanes {
  const lane = idAndTitle(schema)
  const itemSchema = elementSchema(schema, definition.items)
  const item = idAndTitle(itemSchema)
  const backlogSchema = definition.backlog ? elementSchema(schema, definition.backlog) : undefined
  const backlog = idAndTitle(backlogSchema)
  return {
    schema,
    definition,
    laneId: lane.id,
    laneTitle: (r) => {
      const n = r?.name ?? r?.title
      return n !== undefined && n !== null && n !== '' ? String(n) : lane.title(r)
    },
    header: namedView(schema, definition.laneHeader, texts),
    itemSchema,
    itemId: item.id,
    itemTitle: (r) => {
      const n = r?.label ?? r?.title ?? r?.name
      return n !== undefined && n !== null && n !== '' ? String(n) : item.title(r)
    },
    itemView: namedView(itemSchema, definition.itemView, texts),
    backlogId: backlog.id,
    backlogView: namedView(backlogSchema, definition.backlogView, texts),
  }
}

export type BuildUITimelineLanesParams = {
  schema: Schema
  /** The variant (default 'default'), or a definition inline. */
  ref?: UITimelineLanesRef
  texts?: UITexts
}

/** buildUITimelineLanes builds one variant; undefined when the schema
 *  declares no such `ui-timeline-lanes`. */
export function buildUITimelineLanes({ schema, ref, texts }: BuildUITimelineLanesParams): UITimelineLanes | undefined {
  const def = toUITimelineLanesDef(schema, ref)
  if (!def) return undefined
  return toUITimelineLanes(schema, def, texts ?? makeUITexts(i18next, 'view'))
}

/** buildUITimelineLanesVariants builds every `ui-timeline-lanes` variant the
 *  root type declares, by name. */
export function buildUITimelineLanesVariants({ schema, texts }: Omit<BuildUITimelineLanesParams, 'ref'>): Record<string, UITimelineLanes> {
  const resolved = texts ?? makeUITexts(i18next, 'view')
  const out: Record<string, UITimelineLanes> = {}
  for (const def of SchemaQuery.getSchemaExtension<UITimelineLanesDef>(schema, undefined, 'ui-timeline-lanes')) {
    out[def.name || 'default'] = toUITimelineLanes(schema, def, resolved)
  }
  return out
}

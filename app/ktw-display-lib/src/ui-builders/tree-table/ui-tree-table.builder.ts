// SPDX-License-Identifier: Apache-2.0
// UITreeTable builder — composes a schema + `ui-tree-table` extension into a
// UITreeTable runtime. Reuses the ui-table column builder (`buildColumns`) so
// column discovery/renderers stay identical; adds the tree linkage + knobs.

import i18next from 'i18next'

import type { Schema } from '../../schema/index.ts'
import { SchemaQuery } from '../../schema/index.ts'
import type { JsonObject } from '../../types/json-types.ts'
import { type UITexts, makeUITexts } from '../../ui-core/i18n/index.ts'

import { buildColumns } from '../table/index.ts'

import type { UITreeTableDef, UITreeTableRef } from './ui-tree-table.schema.ts'
import type { UITreeTable } from './ui-tree-table.types.ts'

export function toUITreeTableDef(schema: Schema, ref?: UITreeTableRef): UITreeTableDef {
  if ((ref === undefined || ref === null || typeof ref === 'string') && schema !== undefined) {
    ref = SchemaQuery.getNamedSchemaExtension<UITreeTableDef>(schema, undefined, 'ui-tree-table', ref)
  }
  if (ref && typeof ref === 'object' && 'type' in ref && ref.type === 'ui-tree-table') {
    return ref as UITreeTableDef
  }
  return { type: 'ui-tree-table' }
}

function toUITreeTable<TData = any>(
  schema: Schema,
  def: UITreeTableDef,
  texts: UITexts,
  data?: unknown[],
): UITreeTable<TData> {
  const sampleRow = Array.isArray(data)
    ? (data.find((r) => r && typeof r === 'object') as JsonObject | undefined)
    : undefined
  const columns = buildColumns<JsonObject>(schema, def.columns, texts, sampleRow) as UITreeTable<TData>['columns']
  return {
    schema,
    definition: def,
    columns,
    idField: def.idField ?? 'id',
    parentField: def.parentField ?? 'parent_id',
    stateField: def.stateField ?? 'state',
    treeColumn: def.treeColumn,
    defaultExpanded: def.defaultExpanded ?? true,
    indentPx: def.indentPx ?? 20,
    texts,
    withTableBorder: def.withTableBorder,
    withColumnBorders: def.withColumnBorders,
    highlightOnHover: def.highlightOnHover,
    striped: def.striped,
  }
}

export type BuildUITreeTableParams<TData = any> = {
  schema: Schema
  treeRef?: UITreeTableRef
  texts?: UITexts
  data?: unknown[]
}

export function buildUITreeTable<TData = any>({
  schema,
  treeRef,
  texts,
  data,
}: BuildUITreeTableParams<TData>): UITreeTable<TData> {
  const resolvedTexts = texts ?? makeUITexts(i18next, 'table')
  return toUITreeTable<TData>(schema, toUITreeTableDef(schema, treeRef), resolvedTexts, data)
}

export type BuildUITreeTablesParams<TData = any> = Omit<BuildUITreeTableParams<TData>, 'treeRef'>

/** Discover every `ui-tree-table` extension on the schema and build one per
 *  name. Always synthesizes a 'default' when no extension claims that name. */
export function buildUITreeTables<TData = any>({
  schema,
  texts,
  data,
}: BuildUITreeTablesParams<TData>): Record<string, UITreeTable<TData>> {
  const resolvedTexts = texts ?? makeUITexts(i18next, 'table')
  const tables: Record<string, UITreeTable<TData>> = {}
  const defs = SchemaQuery.getSchemaExtension<UITreeTableDef>(schema, undefined, 'ui-tree-table')
  for (const def of defs) {
    tables[def.name || 'default'] = toUITreeTable<TData>(schema, def, resolvedTexts, data)
  }
  if (tables['default'] === undefined) {
    tables['default'] = toUITreeTable<TData>(schema, { type: 'ui-tree-table' }, resolvedTexts, data)
  }
  return tables
}

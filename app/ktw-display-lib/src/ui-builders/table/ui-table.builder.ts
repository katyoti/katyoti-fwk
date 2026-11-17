// SPDX-License-Identifier: Apache-2.0
// UITable builder — composes a schema + DSL table-extension into a
// UITable runtime.
//
// Ported from _ktw_workspace/.../cpm-ui/ui-table/ui-table.builder.tsx
// during gui-table Subtask 2 (2026-05-04). Adaptations vs the source:
//
//   - Array-root unwrap (Q1 ANALYZE decision): when `schema.types[schema.root]`
//     is an Array type, we unwrap to its `items` to discover columns. Tables
//     are inherently row-collections, so the Array IS implicit; columns come
//     from the items type. Object-root tables still work (single-row table).
//
//   - v1 trim (Q4): NO bulkMenu / rowMenu / toolboxMenu builders. Actions
//     are kept on the UITable runtime but row-click action emission is
//     handled by the harness, not by per-row menu renderers.
//
//   - i18n: optional UITexts param + i18next fallback (matches gui-form/
//     gui-view pattern).
//
//   - DSL renames are not needed (UITableDef field names already match
//     the canonical-AST conventions).

import i18next from 'i18next'

import type { Schema, SchemaType, SchemaTypeRef } from '../../schema/index.ts'
import { SchemaQuery } from '../../schema/index.ts'
import type { JsonObject } from '../../types/json-types.ts'
import { type Overrides, doOverrides } from '../../utils/overrides.ts'

import type { ActionMode, UIAction, UIActionRef } from '../../ui-core/action/index.ts'
import { mapToUIActions, mergeActions, toUIActions } from '../../ui-core/action/index.ts'
import { createTableRenderers, getInfo } from '../fields/index.ts'
import type {
  UIColumnDef,
  UIColumnRef,
  UIColumnRenderer,
  UIHeaderRenderer,
} from '../fields/index.ts'
import { type UIFilters, buildUIFilters } from '../filter/index.ts'
import { type UITexts, makeUITexts } from '../../ui-core/i18n/index.ts'

import type { UITableDef, UITableRef } from './ui-table.schema.ts'
import type { UIColumn, UITable } from './ui-table.types.ts'
import { joinCell, wrapColumnRenderers, wrapHeaderRenderers, type UIColumnExtra } from './ui-table.utils.tsx'

// =================================================================================================
// Array-root unwrap (Q1 of ANALYZE 2026-05-04)
// =================================================================================================

/**
 * Resolve the items type of a table-shaped schema. When the root is an
 * Array, returns the resolved items type (suitable for `getFieldInfos` +
 * `getIdAndTitleField`). When the root is already an Object, returns it
 * directly (single-row table is well-defined).
 *
 * @internal — gui-table-only helper; if a 3rd composition needs the same
 * pattern, lift into `SchemaQuery`.
 */
function resolveRowType(schema: Schema): SchemaType {
  const rootRef = schema.root
  const rootType = SchemaQuery.getType(schema, rootRef)
  if (rootType.type === 'array') {
    const itemsRef = (rootType as { items: SchemaTypeRef }).items
    return SchemaQuery.getType(schema, itemsRef)
  }
  return rootType
}

// =================================================================================================
// Column building
// =================================================================================================

function buildColumnRenderer<TData extends JsonObject = JsonObject>(
  schema: Schema,
  rowType: SchemaType,
  columnDef: UIColumnDef,
  texts: UITexts,
): { renderHeader: UIHeaderRenderer; renderCell: UIColumnRenderer<TData> } {
  const { id, name } = columnDef
  if (Array.isArray(name)) {
    const renderers = name.map((onename) =>
      buildColumnRenderer<TData>(schema, rowType, { ...columnDef, name: onename }, texts),
    )
    return {
      renderHeader: wrapHeaderRenderers(renderers.map((r) => r.renderHeader)),
      renderCell: wrapColumnRenderers(renderers.map((r) => r.renderCell)),
    }
  }
  const fieldName = (name as string) || id
  return createTableRenderers<TData>(schema, fieldName, columnDef, texts, rowType) as {
    renderHeader: UIHeaderRenderer
    renderCell: UIColumnRenderer<TData>
  }
}

function buildColumn<TData extends JsonObject = JsonObject>(
  schema: Schema,
  rowType: SchemaType,
  columnDef: UIColumnDef,
  texts: UITexts,
): UIColumn<TData> {
  const { id, name: _n, type: _t, append, prepend, below, ...defs } = columnDef
  const renderer = buildColumnRenderer<TData>(schema, rowType, columnDef, texts)
  const content = columnContent(schema, rowType, columnDef)
  // The fields drawn beside the column's own value, each by its own column
  // renderer.
  const extra = (names: string[] | undefined): UIColumnExtra<TData>[] =>
    (names ?? []).filter((n) => typeof n === 'string' && n !== '').map((n) => {
      const def: UIColumnDef = { id: n }
      return {
        id: n,
        content: columnContent(schema, rowType, def),
        renderCell: buildColumnRenderer<TData>(schema, rowType, def, texts).renderCell,
      }
    })
  const before = extra(prepend)
  const after = extra(append)
  const under = extra(below)
  return {
    id,
    ...defs,
    ...renderer,
    renderCell: joinCell<TData>(before, renderer.renderCell, after, under),
    content,
    before,
    after,
    under,
  }
}

/** columnContent is a column field's content-type name (`enum` for an
 *  enumerated string); undefined for a composite or unknown column. */
function columnContent(schema: Schema, rowType: SchemaType, def: UIColumnDef): string | undefined {
  if (Array.isArray(def.name)) return undefined
  try {
    const info = getInfo(schema, (def.name as string) || def.id, def, rowType)
    return Array.isArray((info.type as { enum?: unknown[] }).enum) ? 'enum' : info.contentType.type
  } catch {
    return undefined
  }
}

/** Title-case a data key for a column header: "risk_level" → "Risk Level". */
function humanizeKey(k: string): string {
  return k
    .replace(/[_-]+/g, ' ')
    .replace(/([a-z])([A-Z])/g, '$1 $2')
    .replace(/\b\w/g, (c) => c.toUpperCase())
    .trim()
}

function buildColumnList(
  schema: Schema,
  rowType: SchemaType,
  columns: UIColumnRef[] | undefined,
  sampleRow?: JsonObject,
): UIColumnDef[] {
  if (columns === undefined || columns.length === 0) {
    // Discover columns from the schema's declared fields. A thin or placeholder
    // schema (row type with no `fields`) makes getFieldInfos throw — don't let
    // that white-screen the table; fall back to the data's own keys so the rows
    // still render (cells render generically via the field renderer fallback).
    let fields: ReturnType<typeof SchemaQuery.getFieldInfos> = []
    try {
      fields = SchemaQuery.getFieldInfos(schema, rowType)
    } catch {
      fields = []
    }
    if (fields.length > 0) {
      return fields.map((field) => ({
        id: field.fieldName,
        label: (field.fieldType as { label?: string }).label,
      }))
    }
    if (sampleRow && typeof sampleRow === 'object') {
      return Object.keys(sampleRow).map((k) => ({ id: k, label: humanizeKey(k) }))
    }
    return []
  }

  return columns.map((column) => {
    const columnDef: UIColumnDef = typeof column === 'string' ? { id: column } : { ...column }
    if (columnDef.label === undefined) {
      try {
        const field = SchemaQuery.getFieldInfo(schema, rowType, columnDef.id)
        columnDef.label = field?.fieldType
          ? (field.fieldType as { label?: string }).label || columnDef.id
          : columnDef.id
      } catch {
        columnDef.label = columnDef.id
      }
    }
    return columnDef
  })
}

export function buildColumns<TData extends JsonObject = JsonObject>(
  schema: Schema,
  columns: UIColumnRef[] | undefined,
  texts: UITexts,
  sampleRow?: JsonObject,
): UIColumn<TData>[] {
  const rowType = resolveRowType(schema)
  const fields = buildColumnList(schema, rowType, columns, sampleRow)
  return fields.map((field) => buildColumn<TData>(schema, rowType, field, texts))
}

// =================================================================================================
// Table building
// =================================================================================================

export function toUITableDef(schema: Schema, tableRef?: UITableRef): UITableDef {
  if ((tableRef === undefined || tableRef === null || typeof tableRef === 'string') && schema !== undefined) {
    tableRef = SchemaQuery.getNamedSchemaExtension<UITableDef>(schema, undefined, 'ui-table', tableRef)
  }
  if (tableRef && typeof tableRef === 'object' && 'type' in tableRef && tableRef.type === 'ui-table') {
    return tableRef as UITableDef
  }
  return { type: 'ui-table' }
}

export function toUITable<TData = any>(
  schema: Schema,
  tableDef: UITableDef,
  texts: UITexts,
  actions: UIAction[],
  actionMode: ActionMode = 'append',
  data?: unknown[],
): UITable<TData> {
  const { type: _t, name: _n, columns, filters, actions: rowActions, ...attributes } = tableDef
  const rowType = resolveRowType(schema)
  // First object row drives data-derived columns when the schema declares no fields.
  const sampleRow = Array.isArray(data)
    ? (data.find((r) => r && typeof r === 'object') as JsonObject | undefined)
    : undefined

  const filterDef: UIFilters = buildUIFilters(schema, filters, texts, rowType)
  const { idField, titleField } = SchemaQuery.getIdAndTitleField(schema, rowType)

  const xrowActions = mapToUIActions(rowActions, 'table', texts)
  const tableActions = mergeActions(xrowActions, actionMode, actions)

  return {
    ...attributes,
    schema,
    definition: tableDef,
    rowId: idField as (row: TData) => string,
    rowTitle: titleField as (row: TData) => string,
    actions: tableActions,
    filters: filterDef,
    texts,
    columns: buildColumns<JsonObject>(schema, columns, texts, sampleRow) as UIColumn<TData>[],
  }
}

/** Overrides applicable to a built UITable. */
export type UITableOverrides<TData = any> = Overrides<UITable<TData>>

export type BuildUITableParams<TData = any> = {
  schema: Schema
  tableRef?: UITableRef
  actionRefs?: UIActionRef[]
  actionMode?: ActionMode
  texts?: UITexts
  overrides?: UITableOverrides<TData>
  /** Row data — used ONLY to derive columns from the data keys when the schema
   *  declares no fields (thin/placeholder schema). Never affects a well-typed
   *  schema's columns. */
  data?: unknown[]
}

export function buildUITable<TData = any>({
  schema,
  tableRef,
  actionRefs,
  actionMode,
  texts,
  overrides,
  data,
}: BuildUITableParams<TData>): UITable<TData> {
  const resolvedTexts = texts ?? makeUITexts(i18next, 'table')
  const tableDef = toUITableDef(schema, tableRef)
  const actions = toUIActions<TData>(actionRefs)

  const defaultTable = toUITable<TData>(schema, tableDef, resolvedTexts, actions, actionMode, data)
  return overrides ? doOverrides(defaultTable, overrides) : defaultTable
}

export type BuildUITablesParams<TData = any> = Omit<BuildUITableParams<TData>, 'tableRef' | 'overrides'>

/**
 * Discover every `ui-table` extension on the schema and build a UITable
 * for each. Always synthesizes a `'default'` entry when no extension
 * claims that name.
 */
export function buildUITables<TData = any>({
  schema,
  actionRefs,
  actionMode,
  texts,
  data,
}: BuildUITablesParams<TData>): Record<string, UITable<TData>> {
  const resolvedTexts = texts ?? makeUITexts(i18next, 'table')
  const actions = toUIActions<TData>(actionRefs)

  const tables: Record<string, UITable<TData>> = {}
  const tableDefs = SchemaQuery.getSchemaExtension<UITableDef>(schema, undefined, 'ui-table')
  for (const tableDef of tableDefs) {
    const key = tableDef.name || 'default'
    tables[key] = toUITable<TData>(schema, tableDef, resolvedTexts, actions, actionMode, data)
  }
  if (tables['default'] === undefined) {
    tables['default'] = toUITable<TData>(schema, { type: 'ui-table' }, resolvedTexts, actions, actionMode, data)
  }
  return tables
}

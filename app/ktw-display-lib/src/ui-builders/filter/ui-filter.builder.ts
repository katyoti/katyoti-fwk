// SPDX-License-Identifier: Apache-2.0
// UIFilter builder — composes a schema + DSL filter-extension into a
// UIFilters runtime bundle (tabFilter + toolbarFilters).
//
// Ported from _ktw_workspace/.../cpm-ui/ui-filter/ui-filter.builder.ts during
// gui-table Subtask 1 (2026-05-04). Adaptations vs the source:
//   - `@core/cpm-ui/ui-text` (toUITexts) → optional UITexts param + i18next fallback
//   - `texts.buttons.all` → `texts.t('buttons.all', { defaultValue: 'All' })`
//   - `texts.buttons.filterReset` → `texts.t('buttons.filterReset', { defaultValue: 'Reset filters' })`

import i18next from 'i18next'

import type { Schema, SchemaTypeRef } from '../../schema/index.ts'
import { SchemaQuery } from '../../schema/index.ts'
import { type UITexts, makeUITexts } from '../../ui-core/i18n/index.ts'

import { createFilter } from './ui-filter.factory.ts'
import type { UIFilterRef, UIFilterToolboxDef } from './ui-filter.schema.ts'
import type { UIFilter, UIFilterInfo, UIFilters } from './ui-filter.types.ts'

/**
 * Build a single filter from a field ref. Inspects the field's content
 * type for `options` — if present, dispatches to tabs (`isTab=true`) or
 * options (`isTab=false`); otherwise falls back to a search filter.
 *
 * `typeRef` (when supplied) is the type to look field info up on. Required
 * when the schema root is an Array (no fields directly on the root); in
 * that case callers pass the resolved items type. When undefined, defaults
 * to the schema root.
 * @internal
 */
function buildFilterInternal(
  schema: Schema,
  fieldRef: UIFilterRef,
  texts: UITexts,
  isTab: boolean,
  typeRef?: SchemaTypeRef,
): UIFilter {
  const fieldDef = typeof fieldRef === 'string' ? { id: fieldRef } : fieldRef
  const fieldName = fieldDef.name || fieldDef.id
  const { fieldType } = SchemaQuery.getMandatoryFieldInfo(schema, typeRef, fieldDef.id)
  const label = fieldDef.label || (fieldType as { label?: string }).label || fieldName
  const name = fieldName

  const { optionList, allOption } = SchemaQuery.getTypeOptions(schema, fieldType)
  const def: UIFilterInfo = optionList
    ? {
        name,
        label,
        type: isTab ? 'tabs' : 'options',
        options: optionList,
        allOption: allOption || { label: texts.t('buttons.all', { defaultValue: 'All' }), value: 'all' },
      }
    : {
        name,
        label,
        type: 'search',
      }
  return createFilter(def)
}

/** Build the optional top-row tab filter. Returns undefined when fieldRef is undefined. */
export function buildUITabFilter(
  schema: Schema,
  fieldRef: UIFilterRef | undefined,
  texts: UITexts,
  typeRef?: SchemaTypeRef,
): UIFilter | undefined {
  if (!fieldRef) return undefined
  return buildFilterInternal(schema, fieldRef, texts, true, typeRef)
}

/** Build a toolbar filter (search or options-select). fieldRef is required. */
export function buildUIToolbarFilter(
  schema: Schema,
  fieldRef: UIFilterRef,
  texts: UITexts,
  typeRef?: SchemaTypeRef,
): UIFilter {
  return buildFilterInternal(schema, fieldRef, texts, false, typeRef)
}

export function buildUIFilterList(
  schema: Schema,
  texts: UITexts,
  fieldNames: UIFilterRef[] | undefined,
  typeRef?: SchemaTypeRef,
): UIFilter[] | undefined {
  if (!fieldNames || fieldNames.length === 0) return undefined
  return fieldNames.map((fn) => buildUIToolbarFilter(schema, fn, texts, typeRef))
}

/**
 * Build a UIFilters bundle from a named (or unnamed) `ui-filter`
 * extension on the schema's root type. Returns an empty bundle if the
 * extension isn't found (no filtering).
 *
 * `typeRef` lets table builders pass the resolved items type when the
 * schema root is an Array (the field-info lookup needs the row type).
 */
export function buildUIFilterToolbox(
  schema: Schema,
  filterName?: string,
  texts?: UITexts,
  typeRef?: SchemaTypeRef,
): UIFilters {
  const filterExtension = SchemaQuery.getNamedSchemaExtension<UIFilterToolboxDef>(
    schema,
    undefined,
    'ui-filter',
    filterName,
  )
  const resolvedTexts = texts ?? makeUITexts(i18next, 'filter')
  return {
    filterResetButton: resolvedTexts.t('buttons.filterReset', { defaultValue: 'Reset filters' }),
    tabFilter: buildUITabFilter(schema, filterExtension?.tabs, resolvedTexts, typeRef),
    toolbarFilters: buildUIFilterList(schema, resolvedTexts, filterExtension?.filters, typeRef),
  }
}

/**
 * Top-level filter resolver — what the table builder calls. Accepts
 * the table's `filters: boolean | string | UIFilterRef[]` field and
 * dispatches:
 *   - boolean `true`  → look up the unnamed `ui-filter` extension
 *   - string         → look up the named `ui-filter` extension
 *   - UIFilterRef[]  → inline list (no separate ui-filter extension)
 *
 * `typeRef` (optional) — pass the resolved row type when the schema root
 * is an Array. Otherwise field-info lookups default to root.
 */
export function buildUIFilters(
  schema: Schema,
  filters?: boolean | string | UIFilterRef[],
  texts?: UITexts,
  typeRef?: SchemaTypeRef,
): UIFilters {
  const resolvedTexts = texts ?? makeUITexts(i18next, 'filter')

  if (filters === true) return buildUIFilterToolbox(schema, undefined, resolvedTexts, typeRef)
  if (typeof filters === 'string') return buildUIFilterToolbox(schema, filters, resolvedTexts, typeRef)
  if (Array.isArray(filters)) {
    return {
      toolbarFilters: buildUIFilterList(schema, resolvedTexts, filters, typeRef),
    }
  }
  return {}
}

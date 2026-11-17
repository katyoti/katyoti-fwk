// SPDX-License-Identifier: Apache-2.0
// UIForm builder — composes a schema + DSL form-extension into a UIForm runtime.
//
// Ported from _ktw_workspace/.../cpm-ui/ui-form/ui-form.builder.ts during
// gui-form Subtask 3 (2026-05-03). Adaptations vs the source:
//
//   - Negative-form `notNew`/`notEdit` predicates → positive `visibility[]`
//     filtering. A field/group with `visibility: ['create']` is shown only
//     in create mode; one with `['create', 'update']` (or no `visibility`
//     at all) shows in both. Aligns with `SchemaFieldVisibility[]` already
//     used field-side since the M7 cleanup.
//
//   - DSL renames per modules/framework/schema/dsl-ui-extensions.md: `nbCols` → `cols`
//     (already applied at the schema-type level — this file just propagates).
//
//   - Layout-factory dispatch is now registry-driven. The legacy
//     `createFormLayout(groups, layoutName)` switch was the only place
//     that knew the four layouts existed; we replace it with a lookup
//     against `getFormLayout(name)` from `runtime/registry-wire.ts`,
//     falling back to `DEFAULT_FORM_LAYOUT` ('groups'). Adding a 5th
//     layout requires zero edits to this file — drop a folder under
//     `content-types/form-layouts/<name>/` + add an entry in
//     `LAYOUT_FACTORIES` (see Subtask 3.11).
//
//   - i18n: the legacy `toUITexts(textsRef, schema)` resolver from
//     `cpm-ui/ui-text` is gone (UITexts is now backed by react-i18next).
//     The builder accepts an optional `texts: UITexts` directly; when
//     omitted it builds one off the global i18next instance scoped to the
//     `'form'` namespace. Render-time renderers can still call
//     `useTexts(...)` to bind to React context.
//
//   - Out of scope (dropped from old code): `useCrudBase` integration,
//     `cpm-ui/ui-form/ui-form.{hook,section,page}.tsx` (CRUD-page concerns).

import i18next from 'i18next'

import type { Schema, SchemaFieldVisibility } from '../../schema/index.ts'
import { SchemaQuery } from '../../schema/index.ts'
import type { JsonObject } from '../../types/json-types.ts'
import { type Overrides, doOverrides } from '../../utils/overrides.ts'
import { DEFAULT_FORM_LAYOUT, getFormLayout } from './layouts/factory.ts'

import type { ActionMode, UIAction, UIActionRef } from '../../ui-core/action/index.ts'
import { mapToUIActions, mergeActions, toUIActions } from '../../ui-core/action/index.ts'
import type { UIFieldDef, UIFieldRef, UIFormFieldRenderer } from '../fields/index.ts'
import { createFieldRenderer, createRowRenderers, type UIFormRowRenderers } from '../fields/index.ts'
import type { UITexts } from '../../ui-core/i18n/index.ts'
import { makeUITexts } from '../../ui-core/i18n/index.ts'

import { formActionsOf } from './form-line.ts'
import { fol_buttons_layout, fol_group_layout, fol_toolbar_layout } from './layouts/_shared/index.ts'
import type { UIFormDef, UIFormGroupDef, UIFormRef } from './ui-form.schema.ts'
import type {
  UIForm,
  UIFormButtonsRenderer,
  UIFormFields,
  UIFormGroup,
  UIFormGroups,
  UIFormRenderer,
} from './ui-form.types.ts'

// =================================================================================================
// Visibility filtering — replaces the legacy `notNew`/`notEdit` predicates
// =================================================================================================

/** A render-time item paired with its CRUD-mode visibility list. Empty/absent
 *  lists mean "show in every mode". */
type VisibilityItem<T> = { visibility?: SchemaFieldVisibility[]; item: T }

/** Map `isNew` (the form's CRUD mode) to the corresponding visibility token.
 *  `view` and `filter` are not produced here — those tokens are honored only
 *  when authors emit them on read-only surfaces. */
function visibilityFor(isNew: boolean): SchemaFieldVisibility {
  return isNew ? 'create' : 'update'
}

function filterByVisibility<T>(items: VisibilityItem<T>[], isNew: boolean): T[] {
  const mode = visibilityFor(isNew)
  return items
    .filter((entry) => {
      if (!entry.visibility || entry.visibility.length === 0) return true
      return entry.visibility.includes(mode)
    })
    .map((entry) => entry.item)
}

// =================================================================================================
// Field building
// =================================================================================================

function toUIFieldDef(field: string | UIFieldRef): UIFieldDef {
  if (typeof field === 'string') return { id: field }
  return field
}

function buildFieldsRenderers(
  schema: Schema,
  fields: UIFieldRef[] = [],
  texts: UITexts,
): VisibilityItem<UIFormFieldRenderer>[] {
  if (!fields || fields.length === 0) return []
  return fields.map(toUIFieldDef).map((field) => ({
    visibility: field.visibility,
    item: createFieldRenderer(schema, field.name || field.id, field, texts),
  }))
}

function buildGetFieldsToRender(
  schema: Schema,
  fields: UIFieldRef[] = [],
  texts: UITexts,
): (isNew: boolean, values: JsonObject) => UIFormFieldRenderer[] {
  const renderers = buildFieldsRenderers(schema, fields, texts)
  return (isNew: boolean, _values: JsonObject): UIFormFieldRenderer[] => {
    return filterByVisibility(renderers, isNew)
  }
}

function buildGetRowsToRender(
  schema: Schema,
  fields: UIFieldRef[] = [],
  texts: UITexts,
): (isNew: boolean, values: JsonObject) => UIFormRowRenderers[] {
  const rows = (fields ?? []).map(toUIFieldDef).map((field) => ({
    visibility: field.visibility,
    item: createRowRenderers(schema, field.name || field.id, field, texts),
  }))
  return (isNew: boolean): UIFormRowRenderers[] => filterByVisibility(rows, isNew)
}

/** The schema fields a group shows (a nested one by its dotted path), and
 *  those that are arrays. */
function groupFieldNames(schema: Schema, fields: UIFieldRef[] = []): { fields: string[]; arrayFields: string[] } {
  const names = (fields ?? []).map(toUIFieldDef).map((f) => f.name || f.id)
  const arrayFields = names.filter((n) => {
    try {
      const fi = (n.includes('.') ? SchemaQuery.getFieldInfoAt(schema, schema.root, n) : SchemaQuery.getFieldInfo(schema, schema.root, n)) as { isArray?: boolean; fieldType?: { type?: string } } | undefined
      return fi?.isArray === true || fi?.fieldType?.type === 'array'
    } catch {
      return false
    }
  })
  return { fields: names, arrayFields }
}

// =================================================================================================
// Group building
// =================================================================================================

function buildGroupRenderer(schema: Schema, group: UIFormGroupDef, texts: UITexts, formEdit?: 'row'): UIFormFields {
  const { title, cols, collapsible, collapsed } = group
  const edit = group.edit ?? formEdit
  return {
    title,
    cols,
    collapsible,
    collapsed,
    edit,
    texts,
    getFieldsToRender: buildGetFieldsToRender(schema, group.fields, texts),
    ...(edit === 'row' ? { getRowsToRender: buildGetRowsToRender(schema, group.fields, texts) } : {}),
  }
}

function createDefaultGroups(schema: Schema): UIFormGroupDef[] {
  return [
    {
      fields: SchemaQuery.getFieldInfos(schema, undefined).map((field) => ({ id: field.fieldName })),
    },
  ]
}

function buildGroupsRenderers(
  schema: Schema,
  formDef: UIFormDef,
  texts: UITexts,
): VisibilityItem<UIFormGroup>[] {
  const groups = !formDef.groups || formDef.groups.length === 0 ? createDefaultGroups(schema) : formDef.groups
  // Frame each group only when there's more than one — multiple groups
  // need a visual separator; a lone group renders flush (the host frames it).
  const framed = groups.length > 1
  return groups.map((group) => ({
    visibility: group.visibility,
    item: {
      md: group.md,
      title: group.title,
      ...groupFieldNames(schema, group.fields),
      ...(group.count !== undefined ? { count: group.count } : {}),
      renderer: fol_group_layout(buildGroupRenderer(schema, group, texts, formDef.edit), framed),
    },
  }))
}

function buildGetGroupsToRender(
  schema: Schema,
  formDef: UIFormDef,
  texts: UITexts,
): (isNew: boolean, values: JsonObject) => UIFormGroup<any>[] {
  const renderers = buildGroupsRenderers(schema, formDef, texts)
  return (isNew: boolean, _values: JsonObject): UIFormGroup<any>[] => {
    return filterByVisibility(renderers, isNew)
  }
}

// =================================================================================================
// Buttons building
// =================================================================================================

function buildGetButtonsToRender(
  texts: UITexts,
  actions: UIAction[],
): (isNew: boolean, values: JsonObject) => UIFormButtonsRenderer {
  const buttons = fol_buttons_layout(texts, actions)
  return (_isNew: boolean, _values: JsonObject): UIFormButtonsRenderer => buttons
}

// =================================================================================================
// Form building
// =================================================================================================

/** @internal Exposed for unit testing of the visibility filter pipeline.
 *  Composes the schema + DSL form-extension into the runtime `UIFormGroups`
 *  callback bundle that layout factories consume. Production callers should
 *  use `buildUIForm()` instead. */
export function buildFormGroups(
  schema: Schema,
  formDef: UIFormDef,
  texts: UITexts,
  actions: UIAction[],
): UIFormGroups {
  return {
    getGroupsToRender: buildGetGroupsToRender(schema, formDef, texts),
    getButtonsToRender: buildGetButtonsToRender(texts, actions),
  }
}

/** Resolve a layout name to its renderer, honoring `DEFAULT_FORM_LAYOUT`
 *  ('groups') when the layout is unspecified or unwired. The factory
 *  shape is `(groupsDef) => UIFormRenderer`; we narrow at the boundary. */
function resolveLayoutRenderer(groups: UIFormGroups, layoutName: string | undefined): UIFormRenderer {
  const name = layoutName || DEFAULT_FORM_LAYOUT
  const factory = getFormLayout(name) ?? getFormLayout(DEFAULT_FORM_LAYOUT)
  if (!factory) {
    throw new Error(
      `[ui-form] no layout factory registered for "${name}" and no default ("${DEFAULT_FORM_LAYOUT}") wired — ` +
        `did you forget to call wireFormLayouts() at runtime bootstrap?`,
    )
  }
  return factory(groups) as UIFormRenderer
}

/**
 * Resolve a `UIFormRef` (string | UIFormDef | undefined) against `schema` to
 * a concrete `UIFormDef`. Falls back to a synthesized
 * `{ type: 'ui-form' }` (which yields a single auto-built group containing
 * every field) when the named extension isn't found.
 */
export function toUIFormDef(schema: Schema, formRef?: UIFormRef): UIFormDef {
  if ((formRef === undefined || formRef === null || typeof formRef === 'string') && schema !== undefined) {
    formRef = SchemaQuery.getNamedSchemaExtension<UIFormDef>(schema, undefined, 'ui-form', formRef)
  }
  if (formRef && typeof formRef === 'object' && 'type' in formRef && formRef.type === 'ui-form') {
    return formRef as UIFormDef
  }
  return { type: 'ui-form' }
}

export function toUIForm<Item = any>(
  schema: Schema,
  formDef: UIFormDef,
  texts: UITexts,
  actions: UIAction[],
  actionMode: ActionMode = 'append',
): UIForm<Item> {
  const defaultValue = SchemaQuery.getDefaultValue(schema) as JsonObject
  const { idField, titleField } = SchemaQuery.getIdAndTitleField(schema)

  const formActions = mapToUIActions<Item>(formDef.actions, 'form', texts)
  const allActions = formActionsOf(formDef, mergeActions(formActions, actionMode, actions))

  const groups = buildFormGroups(schema, formDef, texts, allActions)

  return {
    schema,
    definition: formDef,
    rowId: idField as (row: Item) => string,
    rowTitle: titleField as (row: Item) => string,
    actions: allActions,
    texts,
    getDefaultValue: (item?: Item) => (item ? item : ({ ...defaultValue } as unknown as Item)),
    renderer: resolveLayoutRenderer(groups, formDef.layout),
    toolbar: fol_toolbar_layout(texts, allActions),
  }
}

/** Overrides applicable to a built UIForm — every property may be replaced
 *  with either a direct value or a `(default) => override` function. */
export type UIFormOverrides<Item = any> = Overrides<UIForm<Item>>

export type BuildUIFormParams<Item = any> = {
  schema: Schema
  formRef?: UIFormRef
  actionRefs?: UIActionRef[]
  actionMode?: ActionMode
  /** UITexts to use. Defaults to a fresh adapter on the global i18next
   *  instance scoped to the `'form'` namespace. */
  texts?: UITexts
  overrides?: UIFormOverrides<Item>
}

export function buildUIForm<Item = any>({
  schema,
  formRef,
  actionRefs,
  actionMode,
  texts,
  overrides,
}: BuildUIFormParams<Item>): UIForm<Item> {
  const resolvedTexts = texts ?? makeUITexts(i18next, 'form')
  const formDef = toUIFormDef(schema, formRef)
  const actions = toUIActions<Item>(actionRefs)

  const defaultForm = toUIForm<Item>(schema, formDef, resolvedTexts, actions, actionMode)
  return overrides ? doOverrides(defaultForm, overrides) : defaultForm
}

export type BuildUIFormsParams<Item = any> = Omit<BuildUIFormParams<Item>, 'formRef' | 'overrides'>

/**
 * Discover every `ui-form` extension declared on the schema and build a
 * `UIForm` for each. The returned record is keyed by the extension's `name`
 * (or `'default'` when unnamed). A `'default'` entry is always synthesized
 * from `{ type: 'ui-form' }` when no extension claims it — guarantees the
 * caller can always look up `forms['default']`.
 */
export function buildUIForms<Item = any>({
  schema,
  actionRefs,
  actionMode,
  texts,
}: BuildUIFormsParams<Item>): Record<string, UIForm<Item>> {
  const resolvedTexts = texts ?? makeUITexts(i18next, 'form')
  const actions = toUIActions<Item>(actionRefs)

  const forms: Record<string, UIForm<Item>> = {}
  const formDefs = SchemaQuery.getSchemaExtension<UIFormDef>(schema, undefined, 'ui-form')
  for (const formDef of formDefs) {
    const key = formDef.name || 'default'
    forms[key] = toUIForm<Item>(schema, formDef, resolvedTexts, actions, actionMode)
  }
  if (forms['default'] === undefined) {
    forms['default'] = toUIForm<Item>(schema, { type: 'ui-form' }, resolvedTexts, actions, actionMode)
  }
  return forms
}

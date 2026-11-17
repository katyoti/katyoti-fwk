// SPDX-License-Identifier: Apache-2.0
// UIView builder — composes a schema + DSL view-extension into a UIView runtime.
//
// Ported from _ktw_workspace/.../cpm-ui/ui-view/ui-view.builder.ts during
// gui-view Subtask 1 (2026-05-04). Adaptations vs the source:
//
//   - Negative-form `notNew`/`notEdit` predicates → positive `visibility[]`
//     filtering. View-side honors the `'view'` token: a group/field with
//     `visibility: ['view']` is shown; one with `['create']` or `['update']`
//     only is HIDDEN (form-only). Empty/absent visibility shows everywhere.
//
//   - Detail-level filtering: `getFieldsToRender(item, detailLevel)` skips
//     fields whose `info.fieldOptions.detail > detailLevel`. Per-field
//     declaration in the schema; per-view runtime control via the toggle
//     in UIViewView's header.
//
//   - DSL renames per modules/display/compositions.md (`ui-view`): `nbCols` → `cols`
//     (already applied at the schema-type level — this file just propagates).
//
//   - Layout-factory dispatch is registry-driven via `getViewLayout(name)`
//     (see Subtask 2's runtime/registry-wire.ts changes), falling back to
//     `DEFAULT_VIEW_LAYOUT` ('detail-pane'). Adding a 6th layout requires
//     zero edits to this file.
//
//   - i18n: `texts: UITexts` parameter (or auto-built from i18next), same
//     as gui-form's builder.
//
//   - Out of scope (dropped): `useCrudBase`; `ui-view.{hook,page,section}.tsx`.

import i18next from 'i18next'

import type { Schema, SchemaFieldVisibility } from '../../schema/index.ts'
import { SchemaQuery } from '../../schema/index.ts'
import type { JsonObject } from '../../types/json-types.ts'
import { type Overrides, doOverrides } from '../../utils/overrides.ts'
import { DEFAULT_VIEW_LAYOUT, getViewLayout, type ViewLayoutContext } from './layouts/factory.ts'

import type { ActionMode, UIAction, UIActionRef } from '../../ui-core/action/index.ts'
import { mapToUIActions, mergeActions, toUIActions } from '../../ui-core/action/index.ts'
import type { UIFieldDef, UIFieldRef, UIViewFieldRenderer } from '../fields/index.ts'
import { createViewRenderer } from '../fields/index.ts'
import type { UITexts } from '../../ui-core/i18n/index.ts'
import { makeUITexts } from '../../ui-core/i18n/index.ts'

import { vol_buttons_layout, vol_group_layout } from './layouts/_shared/index.ts'
import type { UIViewDef, UIViewGroupDef, UIViewRef } from './ui-view.schema.ts'
import type {
  UIView,
  UIViewButtonsRenderer,
  UIViewFields,
  UIViewGroup,
  UIViewGroups,
  UIViewRenderer,
} from './ui-view.types.ts'

// =================================================================================================
// Visibility filtering — mirrors gui-form's pattern, but the matched
// token is `'view'` (form is `'create'` or `'update'`).
// =================================================================================================

type VisibilityItem<T> = { visibility?: SchemaFieldVisibility[]; item: T }

/** A view-side renderer is shown when its visibility list contains `'view'`,
 *  is empty, or is absent. `'create'`-only or `'update'`-only items are
 *  considered form-side and hidden in view. */
function filterByViewVisibility<T>(items: VisibilityItem<T>[]): T[] {
  return items
    .filter((entry) => {
      if (!entry.visibility || entry.visibility.length === 0) return true
      return entry.visibility.includes('view')
    })
    .map((entry) => entry.item)
}

// =================================================================================================
// Field building (with detail-level filtering)
// =================================================================================================

function toUIFieldDef(field: string | UIFieldRef): UIFieldDef {
  if (typeof field === 'string') return { id: field }
  return field
}

function buildFieldsRenderers(
  schema: Schema,
  fields: UIFieldRef[] = [],
  texts: UITexts,
): { detail: number | undefined; visibility?: SchemaFieldVisibility[]; renderer: UIViewFieldRenderer }[] {
  if (!fields || fields.length === 0) return []
  return fields.map(toUIFieldDef).map((field) => ({
    detail: field.detail,
    visibility: field.visibility,
    renderer: createViewRenderer(schema, field.name || field.id, field, texts),
  }))
}

function buildGetFieldsToRender(
  schema: Schema,
  fields: UIFieldRef[] = [],
  texts: UITexts,
): (values: JsonObject, detailLevel: number) => UIViewFieldRenderer[] {
  const all = buildFieldsRenderers(schema, fields, texts)
  return (_values, detailLevel) => {
    // First filter by visibility ('view' token), then by detail level.
    const visibilityFiltered = filterByViewVisibility(
      all.map((entry) => ({ visibility: entry.visibility, item: entry })),
    )
    return visibilityFiltered
      .filter((entry) => (entry.detail ?? 1) <= detailLevel)
      .map((entry) => entry.renderer)
  }
}

// =================================================================================================
// Group building
// =================================================================================================

function buildGroupRenderer(schema: Schema, group: UIViewGroupDef, texts: UITexts): UIViewFields {
  const { title, cols } = group
  return {
    title,
    cols,
    getFieldsToRender: buildGetFieldsToRender(schema, group.fields, texts),
  }
}

function createDefaultGroups(schema: Schema): UIViewGroupDef[] {
  return [
    {
      fields: SchemaQuery.getFieldInfos(schema, undefined).map((field) => ({ id: field.fieldName })),
    },
  ]
}

function buildGroupsRenderers(
  schema: Schema,
  viewDef: UIViewDef,
  texts: UITexts,
): VisibilityItem<UIViewGroup>[] {
  const groups = !viewDef.groups || viewDef.groups.length === 0 ? createDefaultGroups(schema) : viewDef.groups
  return groups.map((group) => ({
    visibility: group.visibility,
    item: {
      md: group.md,
      title: group.title,
      renderer: vol_group_layout(buildGroupRenderer(schema, group, texts)),
    },
  }))
}

function buildGetGroupsToRender(
  schema: Schema,
  viewDef: UIViewDef,
  texts: UITexts,
): (values: JsonObject) => UIViewGroup<any>[] {
  const renderers = buildGroupsRenderers(schema, viewDef, texts)
  return () => filterByViewVisibility(renderers)
}

// =================================================================================================
// Buttons building
// =================================================================================================

function buildGetButtonsToRender(
  texts: UITexts,
  actions: UIAction[],
): (values: JsonObject) => UIViewButtonsRenderer {
  const buttons = vol_buttons_layout(texts, actions) ?? (() => null)
  return () => buttons
}

// =================================================================================================
// View building
// =================================================================================================

/** @internal Exposed for unit testing of the visibility + detail-level
 *  filter pipeline. Production callers should use `buildUIView()`. */
export function buildViewGroups(
  schema: Schema,
  viewDef: UIViewDef,
  texts: UITexts,
  actions: UIAction[],
): UIViewGroups {
  return {
    getGroupsToRender: buildGetGroupsToRender(schema, viewDef, texts),
    getButtonsToRender: buildGetButtonsToRender(texts, actions),
  }
}

/** Resolve a layout name to its renderer, honoring `DEFAULT_VIEW_LAYOUT`
 *  ('detail-pane') when the layout is unspecified or unwired. */
function resolveLayoutRenderer(groups: UIViewGroups, layoutName: string | undefined, ctx: ViewLayoutContext): UIViewRenderer {
  const name = layoutName || DEFAULT_VIEW_LAYOUT
  const factory = getViewLayout(name) ?? getViewLayout(DEFAULT_VIEW_LAYOUT)
  if (!factory) {
    throw new Error(
      `[ui-view] no layout factory registered for "${name}" and no default ("${DEFAULT_VIEW_LAYOUT}") wired — ` +
        `did you forget to call wireViewLayouts() at runtime bootstrap?`,
    )
  }
  return factory(groups, ctx) as UIViewRenderer
}

/** Resolve a `UIViewRef` to a concrete `UIViewDef`. Falls back to a
 *  synthesized `{ type: 'ui-view' }` (which yields a single auto-built
 *  group containing every field) when the named extension isn't found. */
export function toUIViewDef(schema: Schema, viewRef?: UIViewRef): UIViewDef {
  if ((viewRef === undefined || viewRef === null || typeof viewRef === 'string') && schema !== undefined) {
    viewRef = SchemaQuery.getNamedSchemaExtension<UIViewDef>(schema, undefined, 'ui-view', viewRef)
  }
  if (viewRef && typeof viewRef === 'object' && 'type' in viewRef && viewRef.type === 'ui-view') {
    return viewRef as UIViewDef
  }
  return { type: 'ui-view' }
}

export function toUIView<Item = any>(
  schema: Schema,
  viewDef: UIViewDef,
  texts: UITexts,
  actions: UIAction[],
  actionMode: ActionMode = 'append',
): UIView<Item> {
  const { idField, titleField } = SchemaQuery.getIdAndTitleField(schema)

  const viewActions = mapToUIActions<Item>(viewDef.actions, 'view', texts)
  const allActions = mergeActions(viewActions, actionMode, actions)

  const groups = buildViewGroups(schema, viewDef, texts, allActions)

  return {
    schema,
    definition: viewDef,
    rowId: idField as (row: Item) => string,
    rowTitle: titleField as (row: Item) => string,
    actions: allActions,
    texts,
    renderer: resolveLayoutRenderer(groups, viewDef.layout, { schema, definition: viewDef, texts }),
  }
}

/** Overrides applicable to a built UIView. */
export type UIViewOverrides<Item = any> = Overrides<UIView<Item>>

export type BuildUIViewParams<Item = any> = {
  schema: Schema
  viewRef?: UIViewRef
  actionRefs?: UIActionRef[]
  actionMode?: ActionMode
  /** UITexts to use. Defaults to a fresh adapter on the global i18next
   *  instance scoped to the `'view'` namespace. */
  texts?: UITexts
  overrides?: UIViewOverrides<Item>
}

export function buildUIView<Item = any>({
  schema,
  viewRef,
  actionRefs,
  actionMode,
  texts,
  overrides,
}: BuildUIViewParams<Item>): UIView<Item> {
  const resolvedTexts = texts ?? makeUITexts(i18next, 'view')
  const viewDef = toUIViewDef(schema, viewRef)
  const actions = toUIActions<Item>(actionRefs)

  const defaultView = toUIView<Item>(schema, viewDef, resolvedTexts, actions, actionMode)
  return overrides ? doOverrides(defaultView, overrides) : defaultView
}

export type BuildUIViewsParams<Item = any> = Omit<BuildUIViewParams<Item>, 'viewRef' | 'overrides'>

/**
 * Discover every `ui-view` extension declared on the schema and build a
 * `UIView` for each. Always synthesizes a `'default'` entry (from
 * `{ type: 'ui-view' }`) when no extension claims that name.
 */
export function buildUIViews<Item = any>({
  schema,
  actionRefs,
  actionMode,
  texts,
}: BuildUIViewsParams<Item>): Record<string, UIView<Item>> {
  const resolvedTexts = texts ?? makeUITexts(i18next, 'view')
  const actions = toUIActions<Item>(actionRefs)

  const views: Record<string, UIView<Item>> = {}
  const viewDefs = SchemaQuery.getSchemaExtension<UIViewDef>(schema, undefined, 'ui-view')
  for (const viewDef of viewDefs) {
    const key = viewDef.name || 'default'
    views[key] = toUIView<Item>(schema, viewDef, resolvedTexts, actions, actionMode)
  }
  if (views['default'] === undefined) {
    views['default'] = toUIView<Item>(schema, { type: 'ui-view' }, resolvedTexts, actions, actionMode)
  }
  return views
}

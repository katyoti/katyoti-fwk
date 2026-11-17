// SPDX-License-Identifier: Apache-2.0
// UIView runtime types — what the view rendering pipeline operates on.
//
// Ported from _ktw_workspace/.../cpm-ui/ui-view/ui-view.types.ts during
// gui-view Subtask 1 (2026-05-04). Renames vs the source:
//   - `nbCols` (group-level) → `cols` (DSL-aligned, matches §9.2)
//
// Crucial difference vs ui-form: NO `isNew` anywhere — view is read-only.
// CRUD-mode is implicit (`'view'` for the visibility filter), no submit
// state, no toolbar/buttons-row split.
//
// The `detailLevel` field on RendererParams is gui-view-specific: each
// layout receives the current detail level (1 = minimal, 2 = medium,
// 3 = detailed) and clamps which fields render based on the field's
// `info.fieldOptions.detail` declaration.

import type { ReactNode } from 'react'
import type { Schema } from '../../schema/index.ts'
import type { UIAction, UIActionProcessor } from '../../ui-core/action/index.ts'
import type { UIViewFieldRenderer } from '../fields/index.ts'
import type { UITexts } from '../../ui-core/i18n/index.ts'
import type { UIViewDef } from './ui-view.schema.ts'

// =================================================================================================
// Render closures — the layered pipeline produces these
// =================================================================================================

/** Empty-value render strategy for fields with null/undefined values. */
export type UIViewEmptyStrategy = 'omit' | 'em-dash' | 'explicit-empty'

export type UIViewRendererParams<Item = any> = {
  item?: Item
  actionProcessor?: UIActionProcessor
  /** Current detail level (1 = minimal, 2 = medium, 3 = detailed). The
   *  layout passes this through to the group renderer; the per-field
   *  predicate (`info.fieldOptions.detail <= detailLevel`) gates which
   *  fields actually render. */
  detailLevel?: 1 | 2 | 3
  /** How to render fields whose value is null/undefined/empty. */
  defaultEmptyStrategy?: UIViewEmptyStrategy
}

export type UIViewGroupRenderer<Item = any> = (props: UIViewRendererParams<Item>) => ReactNode
export type UIViewButtonsRenderer<Item = any> = (props: UIViewRendererParams<Item>) => ReactNode
export type UIViewRenderer<Item = any> = (props: UIViewRendererParams<Item>) => ReactNode

// =================================================================================================
// Building types — what the builder pipeline returns
// =================================================================================================

export type UIViewFields<T = any> = {
  title?: string
  /** Number of columns in the SimpleGrid laying out this group's fields.
   *  DSL-aligned name (was `nbCols` in old code). */
  cols?: number
  getFieldsToRender(item: T, detailLevel: number): UIViewFieldRenderer[]
}

export type UIViewGroup<T = any> = {
  /** Mantine Grid span (1-12) — controls the group's horizontal share. */
  md?: number
  title?: string
  renderer: UIViewGroupRenderer<T>
}

export type UIViewGroups<T = any> = {
  getGroupsToRender(item: T): UIViewGroup<T>[]
  getButtonsToRender(item: T): UIViewButtonsRenderer
}

// =================================================================================================
// The composed view — what `buildUIView()` returns
// =================================================================================================

export type UIView<Item = any> = {
  schema: Schema

  definition: UIViewDef

  rowId: (row: Item) => string
  rowTitle: (row: Item) => string

  texts: UITexts

  actions: UIAction[]

  renderer: UIViewRenderer<Item>
}

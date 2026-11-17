// SPDX-License-Identifier: Apache-2.0
// UIForm runtime types — what the form rendering pipeline operates on.
//
// Ported from _ktw_workspace/.../cpm-ui/ui-form/ui-form.types.ts during
// gui-form Subtask 3 (2026-05-03). Renames vs the source:
//   - `nbCols` (group-level) → `cols` (DSL-aligned, matches §9.1)
//
// The render-shape (UIFormRenderer / UIFormGroupRenderer / etc.) is
// unchanged — these are internal closure types built by the factory
// pipeline, not exposed in the DSL.

import type { ReactNode } from 'react'
import type { Schema } from '../../schema/index.ts'
import type { UIAction, UIActionProcessor } from '../../ui-core/action/index.ts'
import type { UIFormFieldRenderer, UIFormRowRenderers } from '../fields/index.ts'
import type { UITexts } from '../../ui-core/i18n/index.ts'
import type { UIFormDef } from './ui-form.schema.ts'

// =================================================================================================
// Render closures — the layered pipeline produces these
// =================================================================================================

export type UIFormRendererParams<Item = any> = {
  item?: Item
  isNew: boolean
  isSubmitting: boolean
  actionProcessor?: UIActionProcessor
}

export type UIFormGroupRenderer<Item = any> = (props: UIFormRendererParams<Item>) => ReactNode
export type UIFormButtonsRenderer<Item = any> = (props: UIFormRendererParams<Item>) => ReactNode
export type UIFormRenderer<Item = any> = (props: UIFormRendererParams<Item>) => ReactNode
export type UIFormToolbarRenderer<Item = any> = (props: UIFormRendererParams<Item>) => ReactNode

// =================================================================================================
// Building types — what the builder pipeline returns
// =================================================================================================

export type UIFormFields<T = any> = {
  title?: string
  /** Number of columns in the SimpleGrid laying out this group's fields.
   *  DSL-aligned name (was `nbCols` in old code). */
  cols?: number
  /** The group folds under its title (a click on the title shows / hides
   *  its fields). Honoured only for a group with a title. */
  collapsible?: boolean
  /** A collapsible group is drawn folded at first. */
  collapsed?: boolean
  /** `row`: the fields drawn as read rows edited behind a pencil
   *  (getRowsToRender), not as inputs. */
  edit?: 'row'
  /** The form's texts (the read rows' OK / Annuler / pencil). */
  texts?: UITexts
  getFieldsToRender(isNew: boolean, item: T): UIFormFieldRenderer[]
  getRowsToRender?(isNew: boolean, item: T): UIFormRowRenderers[]
}

export type UIFormGroup<T = any> = {
  /** Mantine Grid span (1-12) — controls the group's horizontal share. */
  md?: number
  title?: string
  /** The schema fields the group shows (its tab's alert mark). */
  fields?: string[]
  /** Those that are arrays (its tab's count: their rows). */
  arrayFields?: string[]
  /** The group's declared count (UIFormGroupDef.count): a path or `false`. */
  count?: string | false
  renderer: UIFormGroupRenderer<T>
}

export type UIFormGroups<T = any> = {
  getGroupsToRender(isNew: boolean, item: T): UIFormGroup<T>[]
  getButtonsToRender(isNew: boolean, item: T): UIFormButtonsRenderer
}

// =================================================================================================
// The composed form — what `buildUIForm()` returns
// =================================================================================================

export type UIForm<Item = any> = {
  schema: Schema

  definition: UIFormDef

  rowId: (row: Item) => string
  rowTitle: (row: Item) => string

  texts: UITexts

  actions: UIAction[]

  getDefaultValue(item?: Item): any | undefined

  renderer: UIFormRenderer
  toolbar: UIFormToolbarRenderer | undefined
}

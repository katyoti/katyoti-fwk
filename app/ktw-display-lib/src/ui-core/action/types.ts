// SPDX-License-Identifier: Apache-2.0
// UI Action runtime + DSL extension types.
//
// Merged port of:
//   - _ktw_workspace/.../cpm-core/actions/types.ts (UIAction runtime types)
//   - _ktw_workspace/.../cpm-ui/ui-action/types.ts (UIActionDef DSL extension)
//
// Dropped from old code (out of gui-form scope):
//   - cpm-core/actions/menus/ (ActionButtons, ActionIcons, ActionMenu, etc.)
//   - cpm-core/actions/quick-actions/ (QuickActionButton, QuickActionsGrid)
//   - UIRowMenuRenderer / UIBulkMenuRenderer / UIToolboxMenuRenderer types
//     (table/list-page concerns, will land in gui-table todo if needed)
//
// Port date: 2026-05-03 (gui-form Subtask 1).

import type { ReactNode } from 'react'
import type { IconName } from '../components/icons/index.ts'
import type { SchemaTypeExtension } from '../../schema/index.ts'
import type { ValueOrFunction } from '../../types/utils-types.ts'

// =================================================================================================
// Runtime types — used by buildUIForm() and the form-action renderers
// =================================================================================================

export type UIActionScope = 'global' | 'item' | 'items'

export type UIActionParams<T = any, TScope extends UIActionScope = any> = TScope extends 'item'
  ? { action: UIAction<T, 'item'>; item: T }
  : TScope extends 'items'
    ? { action: UIAction<T, 'items'>; selectedItems: T[] }
    : TScope extends 'global'
      ? { action: UIAction<T, 'global'> }
      : never

export type UIActionValue<TResult, T = any, TScope extends UIActionScope = UIActionScope> = ValueOrFunction<
  TResult,
  [UIActionParams<T, TScope>]
>

export type UIActionIdsPredefined = 'delete' | 'edit' | 'new' | 'view'
export type UIActionId = UIActionIdsPredefined | string

export type UIAction<T = any, TScope extends UIActionScope = any> = {
  id: UIActionId
  scope: TScope

  /** Callback when action is triggered */
  onAction?: (params: UIActionParams<T, TScope>) => Promise<void>

  // all other properties are optional and can be set to a function or a value
  label?: UIActionValue<string, T, TScope>
  icon?: UIActionValue<IconName, T, TScope>
  color?: UIActionValue<string, T, TScope>
  description?: UIActionValue<string, T, TScope>
  dangerous?: UIActionValue<boolean, T, TScope>

  /** Whether the action is enabled */
  enabled?: UIActionValue<boolean, T, TScope>

  /** Whether the action is visible */
  visible?: UIActionValue<boolean, T, TScope>

  /** Render custom content instead of default */
  render?: (params: UIActionParams<T, TScope>) => ReactNode
}

// =================================================================================================
// Menu-item types (separators) — used by mergeActions / splitActions
// =================================================================================================

/** Separator for menu sections. */
export interface UISeparator {
  UISeparator: true
  id: string
}

/** Menu item can be an action or a separator. */
export type UIActionMenuItem<T = any, TScope extends UIActionScope = any> = UIAction<T, TScope> | UISeparator

/** Action processor receives params and dispatches to the action's handler. */
export type UIActionProcessor<T = any> = (params: UIActionParams<T>) => Promise<void>

// =================================================================================================
// Menu renderer types — produced by `buildXxxMenuRenderer` and consumed
// by table/view/dashboard surfaces. Restored 2026-05-04 (gui-action
// Subtask 3) after being dropped in gui-form Subtask 1's trim.
// =================================================================================================

/** Per-row menu (icon row + overflow); renders for one item at a time. */
export type UIRowMenuRenderer<T = any> = (
  item: T,
  index: number,
  actionProcessor?: UIActionProcessor,
) => ReactNode

/** Bulk-selection menu; renders when the user selects multiple rows. */
export type UIBulkMenuRenderer<T = any> = (
  selectedItems: T[],
  actionProcessor?: UIActionProcessor,
) => ReactNode

/** Toolbox menu (table-level / global actions, no per-item context). */
export type UIToolboxMenuRenderer = (actionProcessor?: UIActionProcessor) => ReactNode

// =================================================================================================
// DSL extension type — schemas declare `extensions: [{ type: 'ui-action', ... }]`
// =================================================================================================

export type UIActionDef = SchemaTypeExtension<{
  type: 'ui-action'
  id: UIActionId
  scope: UIActionScope
  label?: string
  icon?: IconName
  color?: string
  description?: string
  dangerous?: boolean
}>

/** A UIActionRef can be: a runtime UIAction object, a DSL UIActionDef, or
 *  a string (action code like 'submit(item)' or a predefined-action id). */
export type UIActionRef<T = any, TDefaults extends string = UIActionId> = UIAction<T> | UIActionDef | TDefaults

// SPDX-License-Identifier: Apache-2.0
// Public surface of the action package — UIAction runtime types,
// DSL extension types, factories, action-mode merging, the i18n-backed
// attribute resolver, and the default action sets for form/table/view.

export type {
  UIAction,
  UIActionDef,
  UIActionId,
  UIActionIdsPredefined,
  UIActionMenuItem,
  UIActionParams,
  UIActionProcessor,
  UIActionRef,
  UIActionScope,
  UIActionValue,
  UIBulkMenuRenderer,
  UIRowMenuRenderer,
  UIToolboxMenuRenderer,
  UISeparator,
} from './types.ts'

export {
  // Mode + merge
  type ActionMode,
  mergeActions,
  splitActions,
  findActionsByType,
  toActionCode,
  fromActionCode,
  // Menu predicates + factories
  isUISeparator,
  createSeparator,
  createAction,
  createActionMap,
  // Ref resolution
  isUIActionDef,
  isUIAction,
  toUIAction,
  toUIActions,
  createActionProcessor,
  // Display helpers
  resolveUIActionValue,
  getActionAttributes,
  getattributs, // back-compat alias for the typo'd export name
  // Menu renderer builders (gui-action Subtask 3)
  buildToolboxMenuRenderer,
  buildBulkMenuRenderer,
  buildRowMenuRenderer,
} from './utils.tsx'

export {
  ROW_CLICK_ACTION,
  ROW_DOUBLE_CLICK_ACTION,
  defaultTableActions,
  defaultFormActions,
  defaultViewActions,
  mapToUIActions,
} from './defaults.ts'

// Menu components (gui-action Subtask 1)
export {
  ActionButtons,
  ActionIcons,
  ActionMenu,
  type ActionMenuProps,
  ContextualMenu,
  type ContextualMenuProps,
} from './menus/index.ts'

// Quick-actions (gui-action Subtask 2 — dashboard primary-action grid)
export { QuickActionButton, QuickActionsGrid } from './quick-actions/index.ts'

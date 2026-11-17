// SPDX-License-Identifier: Apache-2.0
// Default action factories for form / table / view contexts.
//
// Ported from _ktw_workspace/.../cpm-ui/ui-action/defaults.ts during
// gui-form Subtask 1 (2026-05-03).
//
// Adapted for i18n: old `texts.buttons.save` (destructured from a bag)
// became `texts.t('buttons.save', { defaultValue: 'Save' })` (i18n facade
// with a sensible English fallback). Translations live in the form
// namespace's `i18n/<locale>.json` files (loaded by initI18n at runtime).
//
// Scope: only `defaultFormActions` is needed by gui-form — but the
// table/view variants are kept here because they're tiny and future
// gui-table / gui-view todos will consume them. The `mapToUIActions`
// dispatcher accepts a 'table' | 'form' | 'view' kind so all three
// surfaces share the same string→UIAction resolution path.

import type { UITexts } from '../i18n/index.ts'
import { createAction, createActionMap, toUIAction } from './utils.tsx'
import type { UIAction, UIActionRef } from './types.ts'

/** Sentinel: row-click action (table-side; consumer wires it up). */
export const ROW_CLICK_ACTION = createAction({ id: 'click', scope: 'item' })

/** Sentinel: row-double-click action (table-side; consumer wires it up). */
export const ROW_DOUBLE_CLICK_ACTION = createAction({ id: 'doubleClick', scope: 'item' })

/** Default actions for a list/table page (out-of-scope for gui-form, kept
 *  for future gui-table consumption). */
export function defaultTableActions<T = any>(texts: UITexts): UIAction<T>[] {
  return [
    createAction({
      id: 'create',
      scope: 'global',
      label: texts.t('buttons.new', { defaultValue: 'New' }),
      icon: 'IconPlus',
    }),
    createAction({
      id: 'edit',
      scope: 'item',
      label: texts.t('buttons.edit', { defaultValue: 'Edit' }),
      icon: 'IconEdit',
    }),
    createAction({
      id: 'view',
      scope: 'item',
      label: texts.t('buttons.view', { defaultValue: 'View' }),
      icon: 'IconEye',
    }),
    createAction({
      id: 'delete',
      scope: 'item',
      label: texts.t('buttons.delete', { defaultValue: 'Delete' }),
      icon: 'IconTrash',
      color: 'red',
      dangerous: true,
    }),
    createAction<T, 'items'>({
      id: 'delete',
      scope: 'items',
      label: ({ selectedItems }) =>
        `${texts.t('buttons.delete', { defaultValue: 'Delete' })} ${selectedItems.length} ${texts.t('selected', { defaultValue: 'selected' })}`,
      icon: 'IconTrash',
      color: 'red',
      dangerous: true,
    }),
  ]
}

/** Default actions for a form: submit / cancel / reset / delete. */
export function defaultFormActions<T = any>(texts: UITexts): UIAction<T>[] {
  return [
    createAction({
      id: 'submit',
      scope: 'item',
      label: texts.t('buttons.save', { defaultValue: 'Save' }),
      icon: 'IconDeviceFloppy',
    }),
    createAction({
      id: 'cancel',
      scope: 'item',
      label: texts.t('buttons.cancel', { defaultValue: 'Cancel' }),
      icon: 'IconX',
    }),
    createAction({
      id: 'reset',
      scope: 'item',
      label: texts.t('buttons.reset', { defaultValue: 'Reset' }),
      icon: 'IconRefresh',
    }),
    createAction({
      id: 'delete',
      scope: 'item',
      label: texts.t('buttons.delete', { defaultValue: 'Delete' }),
      icon: 'IconTrash',
      color: 'red',
      dangerous: true,
    }),
  ]
}

/** Default actions for a read-only view: close / edit / delete (out-of-scope
 *  for gui-form, kept for future gui-view consumption). */
export function defaultViewActions<T = any>(texts: UITexts): UIAction<T>[] {
  return [
    createAction({
      id: 'close',
      scope: 'item',
      label: texts.t('buttons.close', { defaultValue: 'Close' }),
      icon: 'IconX',
    }),
    createAction({
      id: 'edit',
      scope: 'item',
      label: texts.t('buttons.edit', { defaultValue: 'Edit' }),
      icon: 'IconPencil',
    }),
    createAction({
      id: 'delete',
      scope: 'item',
      label: texts.t('buttons.delete', { defaultValue: 'Delete' }),
      icon: 'IconTrash',
      color: 'red',
      dangerous: true,
    }),
  ]
}

/**
 * Resolve a list of action refs against the surface defaults. String entries
 * are looked up in the default action map; UIAction / UIActionDef refs go
 * through `toUIAction()`. Used by `buildUIForm()` to wire `actions:` from
 * the DSL ui-form extension.
 */
export function mapToUIActions<T = any>(
  actions: UIActionRef[] | undefined,
  type: 'table' | 'form' | 'view',
  texts: UITexts,
): UIAction<T>[] {
  const defaultActions =
    type === 'table' ? defaultTableActions<T>(texts)
    : type === 'form' ? defaultFormActions<T>(texts)
    : defaultViewActions<T>(texts)

  if (actions === undefined) return defaultActions
  const actionMap = createActionMap<T>(defaultActions)
  return actions.map((action) => {
    if (typeof action === 'string' && actionMap[action] !== undefined) {
      return actionMap[action]
    }
    return toUIAction<T>(action)
  })
}

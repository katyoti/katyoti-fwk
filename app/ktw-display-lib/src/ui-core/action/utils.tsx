// SPDX-License-Identifier: Apache-2.0
// UI Action utilities — predicates, factories, action-mode merging,
// and the i18n-backed `getattributs` resolver used by FormButton.
//
// Merged port of:
//   - _ktw_workspace/.../cpm-core/actions/utils.tsx (mergeActions, splitActions,
//     createAction, getattributs, etc.)
//   - _ktw_workspace/.../cpm-ui/ui-action/utils.tsx (toUIAction, toUIActions,
//     createActionProcessor, isUIActionDef, isUIAction)
//
// Dropped from old code (out of gui-form scope):
//   - buildToolboxMenuRenderer / buildBulkMenuRenderer / buildRowMenuRenderer
//     (depend on ActionButtons / ActionIcons from cpm-core/actions/menus —
//     those are table/list-page concerns; will land in gui-table todo if needed)
//
// Adapted for i18n:
//   - Old `texts.buttons[action.id]` (bag indexing) → `texts.t('buttons.${id}', { defaultValue: id })`
//     (i18n-facade key resolution with sensible fallback to the action id)
//
// Port date: 2026-05-03 (gui-form Subtask 1).

import type { UITexts } from '../i18n/index.ts'
import type { IconName } from '../components/icons/index.ts'
import { resolveValueOrFunction } from '../../types/utils-types.ts'
import type { MaybePromise } from '../../types/utils-types.ts'
import { resolveMaybePromise } from '../../types/utils-types.ts'
import type {
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
  UISeparator,
} from './types.ts'

// =================================================================================================
// Action mode + merge
// =================================================================================================

export type ActionMode = 'append' | 'replace' | 'prepend' | 'substitute'

export function mergeActions<T = any, TScope extends UIActionScope = UIActionScope>(
  actions: UIAction<T, TScope>[],
  actionMode: ActionMode = 'append',
  additionalActions?: UIAction<T, TScope>[],
): UIAction<T, TScope>[] {
  if (actionMode === 'replace') return additionalActions ?? []
  if (actionMode === 'prepend') return [...(additionalActions ?? []), ...actions]
  if (actionMode === 'append') return [...actions, ...(additionalActions ?? [])]

  if (actionMode === 'substitute') {
    return actions.map((action) => {
      const newAction = additionalActions?.find((a) => a.id === action.id && a.scope === action.scope)
      return newAction ?? action
    })
  }
  return actions
}

export function splitActions<T = any>(actions: UIAction<T, UIActionScope>[]) {
  const globalActions: UIAction<T, 'global'>[] = []
  const itemActions: UIAction<T, 'item'>[] = []
  const itemsActions: UIAction<T, 'items'>[] = []

  const alreadyProcessed = new Set<string>()

  actions.forEach((action) => {
    const actionId = `${action.scope}-${action.id}`
    if (alreadyProcessed.has(actionId)) return
    alreadyProcessed.add(actionId)

    switch (action.scope) {
      case 'global':
        globalActions.push(action as UIAction<T, 'global'>)
        break
      case 'item':
        itemActions.push(action as UIAction<T, 'item'>)
        break
      case 'items':
        itemsActions.push(action as UIAction<T, 'items'>)
        break
      default:
        // eslint-disable-next-line no-console
        console.warn(`Unhandled action scope: ${(action as UIAction).scope}`)
        break
    }
  })

  return { globalActions, itemActions, itemsActions }
}

export function findActionsByType<T extends { id: UIActionId; scope: UIActionScope }>(
  action: UIActionIdsPredefined,
  scope: UIActionScope,
  actions?: T[],
): T | undefined {
  const result = actions?.filter((a) => a.id === action && a.scope === scope) ?? []
  return result.length > 0 ? result[0] : undefined
}

export function toActionCode<T extends { id: UIActionId; scope: UIActionScope }>(action: T): string {
  return `${action.id}(${action.scope})`
}

export function fromActionCode(actionString: string): { id: UIActionId; scope: UIActionScope } {
  const cleanString = actionString.toLowerCase().trim().replace(')', '')
  const [id, scope = 'item'] = cleanString.split('(')
  return { id, scope: scope as UIActionScope }
}

// =================================================================================================
// Action menu predicates + factories
// =================================================================================================

/** Type guard: is the menu item a separator? */
export function isUISeparator(item: UIActionMenuItem): item is UISeparator {
  return typeof item === 'object' && 'UISeparator' in item && item.UISeparator === true
}

/** Helper to create a separator. */
export function createSeparator(id: string): UISeparator {
  return { UISeparator: true, id }
}

/** Create a UIAction with sensible defaults (id='', scope='item'). */
export function createAction<T = any, TScope extends UIActionScope = any>(
  action: Partial<UIAction<T, TScope>>,
): UIAction<T, TScope> {
  return {
    id: action.id ?? '',
    scope: action.scope ?? ('item' as TScope),
    ...action,
  }
}

/** Create a Record<actionCode, action> map from an action list. */
export function createActionMap<T = any>(actions: UIAction<T>[]): Record<string, UIAction<T>> {
  return actions.reduce(
    (acc, action) => {
      acc[`${action.id}(${action.scope})`] = action
      return acc
    },
    {} as Record<string, UIAction<T>>,
  )
}

// =================================================================================================
// UIActionRef resolution (string code | UIActionDef | UIAction → UIAction)
// =================================================================================================

export function isUIActionDef(action: UIActionRef): action is UIActionDef {
  return !!action && typeof action === 'object' && 'type' in action && action.type === 'ui-action'
}

export function isUIAction(action: UIActionRef): action is UIAction {
  return !!action && typeof action === 'object' && !('type' in action)
}

export function toUIAction<T = any>(action: UIActionRef<T>): UIAction<T> {
  if (isUIAction(action)) return action

  function newUIAction(actionString: string): UIActionDef {
    return { type: 'ui-action', ...fromActionCode(actionString) }
  }

  const actionDef: UIActionDef = isUIActionDef(action) ? action : newUIAction(action as string)

  return {
    id: actionDef.id,
    scope: actionDef.scope,
    label: actionDef.label,
    icon: actionDef.icon,
    // Preserve explicit colors; default 'red' for delete actions when no color set.
    color: actionDef.color ?? (actionDef.id === 'delete' ? 'red' : undefined),
    description: actionDef.description,
    dangerous: actionDef.dangerous,
  }
}

export function toUIActions<T = any>(actions?: UIActionRef[]): UIAction<T>[] {
  if (actions === undefined) return []
  return actions.map((action) => toUIAction(action))
}

/** Build an action processor from a code-keyed dispatch map. */
export function createActionProcessor<T = any>(
  actionMap: Record<string, (params: UIActionParams) => MaybePromise<void>>,
): UIActionProcessor<T> {
  return (params: UIActionParams<T>): Promise<void> => {
    const actionCode = toActionCode(params.action)
    const action = actionMap[actionCode]
    if (action) {
      return resolveMaybePromise(action(params))
    }
    return Promise.resolve()
  }
}

// =================================================================================================
// Display helpers (i18n-backed)
// =================================================================================================

/**
 * Evaluate a UIActionValue (T | function) with the standard params bag.
 * Returns defaultValue when the input is undefined.
 */
export function resolveUIActionValue<TResult, T = any, TScope extends UIActionScope = any>(
  value: UIActionValue<TResult, T, TScope> | undefined,
  params: UIActionParams<T, TScope>,
  defaultValue: TResult,
): TResult {
  return resolveValueOrFunction<TResult, [UIActionParams<T, TScope>]>(value, [params], defaultValue)
}

/** Default icon names for predefined action ids. */
const ICON_MAP: Record<UIActionIdsPredefined, IconName> = {
  new: 'IconPlus',
  delete: 'IconTrash',
  edit: 'IconPencil',
  view: 'IconEye',
}

/**
 * Resolve all displayable attributes for an action — icon, label (i18n),
 * color, description, dangerous, enabled, visible. Used by FormButton and
 * any other action-rendering component to get the final values to render.
 *
 * i18n adaptation: the old code used `texts.buttons[action.id]` (bag indexing).
 * This version uses `texts.t('buttons.${id}', { defaultValue: id })` — the
 * caller should pass a UITexts scoped to a namespace that has translations
 * under the `buttons.` prefix. Falls back to the action id if unset.
 *
 * Signature note: drops the per-scope generic parameter from the original
 * old-UI version. The action+params are widened to `UIActionScope` (the
 * union) at the boundary because the resolver dispatches by scope at
 * runtime — propagating the precise TScope generic created variance
 * issues under strict TS settings without buying any runtime safety.
 */
export function getActionAttributes<T = any>(
  params: UIActionParams<T, UIActionScope>,
  texts: UITexts,
): {
  icon: IconName
  label: string
  color?: string
  description?: string
  dangerous: boolean
  enabled: boolean
  visible: boolean
} {
  // Cast widening: each `action.X` field is typed as a union over
  // possible scope arities; resolveUIActionValue dispatches on whether
  // it's a function or value at runtime. The cast is a no-op semantically.
  const action = params.action as UIAction<T, UIActionScope>
  return {
    icon: resolveUIActionValue<string, T, UIActionScope>(
      action.icon,
      params,
      ICON_MAP[action.id as UIActionIdsPredefined] ?? 'IconQuestionMark',
    ),
    label: resolveUIActionValue<string, T, UIActionScope>(
      action.label,
      params,
      texts.t(`buttons.${action.id}`, { defaultValue: action.id }),
    ),
    color: resolveUIActionValue<string | undefined, T, UIActionScope>(action.color, params, undefined),
    description: resolveUIActionValue<string | undefined, T, UIActionScope>(action.description, params, undefined),
    dangerous: resolveUIActionValue<boolean, T, UIActionScope>(action.dangerous, params, false),
    enabled: resolveUIActionValue<boolean, T, UIActionScope>(action.enabled, params, true),
    visible: resolveUIActionValue<boolean, T, UIActionScope>(action.visible, params, true),
  }
}

/** Backward-compatibility alias for the old typo'd export name `getattributs`.
 *  Kept exported because the ported ui-form code still references it; can be
 *  dropped once all consumers migrate to `getActionAttributes`. */
export const getattributs = getActionAttributes

// =================================================================================================
// Menu renderer builders — restored 2026-05-04 (gui-action Subtask 3) after
// being dropped in gui-form Subtask 1's trim. Tables (gui-table v1.5),
// dashboards, and any composition with bulk/row/toolbar action surfaces
// consume these.
// =================================================================================================

import { ActionButtons, ActionIcons } from './menus/index.ts'
import type {
  UIBulkMenuRenderer,
  UIRowMenuRenderer,
  UIToolboxMenuRenderer,
} from './types.ts'

/** Build a toolbox-menu renderer (table-level / global-scope actions —
 *  e.g. "Create new", "Export all" buttons above the table). */
export function buildToolboxMenuRenderer(
  actions?: UIAction<any, 'global'>[],
  texts?: UITexts,
): UIToolboxMenuRenderer {
  if (actions === undefined || actions.length === 0) return () => null
  return (actionProcessor?: UIActionProcessor) => (
    <ActionButtons
      params={{} as never}
      actions={actions}
      texts={texts as UITexts}
      actionProcessor={actionProcessor}
    />
  )
}

/** Build a bulk-menu renderer (multi-selection action bar —
 *  e.g. "Delete selected", "Export selected" buttons that appear when
 *  rows are checked). */
export function buildBulkMenuRenderer<T = any>(
  actions?: UIAction<T, 'items'>[],
  texts?: UITexts,
): UIBulkMenuRenderer<T> {
  if (actions === undefined || actions.length === 0) return () => null
  return (selectedItems: T[], actionProcessor?: UIActionProcessor) => (
    <ActionButtons
      actions={actions}
      params={{ selectedItems }}
      texts={texts as UITexts}
      actionProcessor={actionProcessor}
    />
  )
}

/** Build a row-menu renderer (per-row icon strip, typically the rightmost
 *  table column — e.g. "Edit", "Delete", "Duplicate" icons). */
export function buildRowMenuRenderer<T = any>(
  actions?: UIAction<T, 'item'>[],
  texts?: UITexts,
): UIRowMenuRenderer<T> {
  if (actions === undefined || actions.length === 0) return () => null
  return (item: T, _index: number, actionProcessor?: UIActionProcessor) => (
    <ActionIcons
      actions={actions}
      params={{ item }}
      texts={texts as UITexts}
      actionProcessor={actionProcessor}
    />
  )
}

// SPDX-License-Identifier: Apache-2.0
// Form buttons + toolbar — shared infrastructure used by all 4 form layouts.
//
// Merged port of:
//   - _ktw_workspace/.../cpm-ui/ui-form/layouts/fol-buttons-cpm.tsx (raw
//     React components: FormButton, FormButtons, FormToolbar)
//   - _ktw_workspace/.../cpm-ui/ui-form/layouts/fol-buttons.tsx (factory
//     wrappers: fol_buttons_layout, fol_toolbar_layout, filterActions)
//
// Adapted for the new src/ui/ layout (2026-05-03):
//   - `@core/cpm-core/actions/{types,utils}` → relative imports into `../../../action/index.ts`
//   - `@core/cpm-core/icons` → `../../../components/icons/index.ts`
//   - `@core/cpm-core/texts` → `../../../i18n/index.ts` (UITexts now i18n-backed)
//   - `getattributs` retained as the back-compat alias; new code can use
//     `getActionAttributes` directly

import { Button, Group } from '@mantine/core'
import type { ButtonHTMLAttributes } from 'react'
import { Icon } from '../../../../ui-core/components/icons/index.ts'
import {
  getActionAttributes,
  type UIAction,
  type UIActionParams,
  type UIActionProcessor,
} from '../../../../ui-core/action/index.ts'
import type { UITexts } from '../../../../ui-core/i18n/index.ts'
import type { UIFormButtonsRenderer, UIFormRendererParams, UIFormToolbarRenderer } from '../../ui-form.types.ts'

// ============================================================================
// Action id → button HTML type mapping
// ============================================================================

const buttonTypeOf: Record<string, ButtonHTMLAttributes<HTMLButtonElement>['type']> = {
  submit: 'submit',
  save: 'submit',
  create: 'submit',
  reset: 'reset',
}

// ============================================================================
// FormButton — renders a single action as a Mantine Button
// ============================================================================

export type FormButtonProps<Item = any> = {
  action: UIAction<Item, 'item'>
  texts: UITexts
  isNew?: boolean
  isSubmitting?: boolean
  item?: Item
  actionProcessor?: UIActionProcessor
}

export function FormButton<Item = any>({
  action,
  isSubmitting = false,
  item,
  texts,
  actionProcessor,
}: FormButtonProps<Item>) {
  const params: UIActionParams<Item, 'item'> = { action, item: item as Item }
  const { label, icon, enabled, visible, dangerous, color } = getActionAttributes(params, texts)

  if (visible === false) return null

  const buttonType = buttonTypeOf[action.id] ?? 'button'
  const isSubmit = buttonType === 'submit'
  const isLoading = isSubmit && isSubmitting

  const handleClick = () => {
    if (!enabled) return
    // For submit-typed buttons, the native form submission is the
    // canonical path: <button type="submit"> triggers the form's
    // onSubmit (validated through methods.handleSubmit). Calling the
    // actionProcessor here would fire onSubmit a SECOND time on the
    // same click — surfaced as the "wizard completes on step 1" bug
    // during the gui-action probe (2026-05-04). Custom action handlers
    // (action.onAction) still fire because they're explicit overrides.
    if (action.onAction) {
      void action.onAction(params)
      return
    }
    if (isSubmit) return // native form submit takes over
    void actionProcessor?.(params)
  }

  if (action.render) {
    return <>{action.render(params)}</>
  }

  return (
    <Button
      type={buttonType}
      variant="filled"
      loading={isLoading}
      disabled={!enabled}
      color={dangerous ? 'red' : color}
      leftSection={icon ? <Icon icon={icon} size={18} /> : null}
      onClick={handleClick}
    >
      {label}
    </Button>
  )
}

// ============================================================================
// FormButtons / FormToolbar — group of FormButton in a right-aligned row
// ============================================================================

type FormButtonsProps = UIFormRendererParams & {
  actions: UIAction<any, 'item'>[]
  texts: UITexts
  actionProcessor?: UIActionProcessor
}

function renderActionGroup({ actions, texts, actionProcessor, ...props }: FormButtonsProps) {
  const { item = {} } = props

  // Filter actions based on visibility and enabled state
  const displayedActions = actions.filter((action) => {
    const params: UIActionParams = { action, item }
    const { enabled, visible } = getActionAttributes(params, texts)
    return enabled !== false && visible !== false
  })

  if (displayedActions.length === 0) return null

  return (
    <Group justify="flex-end">
      {displayedActions.map((action, index) => (
        <FormButton
          key={`btn-${action.id || index}`}
          action={action}
          texts={texts}
          actionProcessor={actionProcessor}
          {...props}
        />
      ))}
    </Group>
  )
}

export function FormButtons(props: FormButtonsProps) {
  return renderActionGroup(props)
}

export function FormToolbar(props: FormButtonsProps) {
  return renderActionGroup(props)
}

// ============================================================================
// Layout factory wrappers — produce UIFormButtonsRenderer / UIFormToolbarRenderer
// ============================================================================

/** Action ids that go into the TOP TOOLBAR (right side of the title)
 *  rather than the bottom button row. Includes destructive actions like
 *  `delete` so the user can't accidentally hit them mid-submit. */
const TOOLBAR_ACTION_IDS: string[] = ['delete']

function filterActions(actions: UIAction[] | undefined, toolbar: boolean): UIAction[] {
  return (
    actions?.filter((action) => {
      // Non-item-scope actions (global / items) only go in the toolbar
      if (action.scope !== 'item') return toolbar
      return TOOLBAR_ACTION_IDS.includes(action.id) === toolbar
    }) ?? []
  )
}

/** Build the bottom-button-row renderer. Includes all `item`-scope actions
 *  EXCEPT those routed to the toolbar (delete etc.). */
export function fol_buttons_layout(texts: UITexts, actionsDef: UIAction[]): UIFormButtonsRenderer {
  const actions = filterActions(actionsDef, false)
  return (props) => <FormButtons texts={texts} {...props} actions={actions} />
}

/** Build the top-toolbar renderer. Returns undefined when there are no
 *  toolbar-eligible actions — the form view skips the toolbar Group entirely
 *  in that case (no empty render). */
export function fol_toolbar_layout(texts: UITexts, actionsDef: UIAction[]): UIFormToolbarRenderer | undefined {
  const actions = filterActions(actionsDef, true)
  if (actions.length === 0) return undefined
  return (props) => <FormToolbar texts={texts} {...props} actions={actions} />
}

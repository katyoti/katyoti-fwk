// SPDX-License-Identifier: Apache-2.0
// View buttons row — bottom-of-view actions (close / edit / delete /
// export / etc). Unlike gui-form's `_shared/buttons.tsx` which splits
// destructive actions into a top toolbar, the view surface keeps ALL
// actions in a single inline row — view actions are typically navigation
// (close, edit) plus destructive (delete) actions; no submit/cancel/reset
// noise to filter out.
//
// Reuses `FormButton` from `src/ui-builders/form/_shared/buttons.tsx` for the
// per-action rendering (Mantine Button + icon + color + dangerous-flag
// + visibility/enabled checks). The view-side factory just maps over all
// actions and produces a horizontal Group.
//
// Subtask 1 of gui-view (2026-05-04). The cross-package import (form's
// _shared) is intentional: factoring `FormButton` higher (e.g. into
// `src/ui/_shared/`) is a follow-up if a 3rd composition (gui-table)
// also needs it.

import { Group } from '@mantine/core'
import { FormButton } from '../../../form/index.ts'
import type { UIAction, UIActionParams, UIActionProcessor } from '../../../../ui-core/action/index.ts'
import { getActionAttributes } from '../../../../ui-core/action/index.ts'
import type { UITexts } from '../../../../ui-core/i18n/index.ts'
import type { UIViewButtonsRenderer, UIViewRendererParams } from '../../ui-view.types.ts'

type ViewButtonsProps = UIViewRendererParams & {
  actions: UIAction<any, 'item'>[]
  texts: UITexts
  actionProcessor?: UIActionProcessor
}

function ViewButtons({ actions, texts, actionProcessor, ...props }: ViewButtonsProps) {
  const { item = {} } = props

  const displayed = actions.filter((action) => {
    const params: UIActionParams = { action, item }
    const { enabled, visible } = getActionAttributes(params, texts)
    return enabled !== false && visible !== false
  })

  if (displayed.length === 0) return null

  return (
    <Group justify="flex-end">
      {displayed.map((action, index) => (
        <FormButton
          key={`view-btn-${action.id || index}`}
          action={action}
          texts={texts}
          item={item}
          actionProcessor={actionProcessor}
        />
      ))}
    </Group>
  )
}

/** Build the view's bottom button-row renderer. Returns undefined when
 *  there are no actions — UIViewView skips the button strip in that case. */
export function vol_buttons_layout(texts: UITexts, actionsDef: UIAction[]): UIViewButtonsRenderer | undefined {
  const itemActions = actionsDef.filter((a) => a.scope === 'item') as UIAction<any, 'item'>[]
  if (itemActions.length === 0) return undefined
  return (props) => <ViewButtons texts={texts} actions={itemActions} {...props} />
}

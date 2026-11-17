// SPDX-License-Identifier: Apache-2.0
// ActionButtons — horizontal/vertical row of Mantine Buttons, one per
// action. Used by `buildToolboxMenuRenderer` (top-of-table global actions)
// and `buildBulkMenuRenderer` (multi-select bulk-action bar).
//
// Ported from _ktw_workspace/.../cpm-core/actions/menus/ActionButtons.tsx
// during gui-action Subtask 1 (2026-05-04). Adaptations:
//   - `@core/cpm-core/icons` → `'../../components/icons/index.ts'`
//   - `@core/cpm-core/texts` → drop; accept UITexts as a prop
//   - `getattributs` → kept as the back-compat alias for `getActionAttributes`
//     (already exported from gui-form Subtask 1's ui-action package)

import { Button, Group, Stack } from '@mantine/core'
import { Icon } from '../../components/icons/index.ts'
import type { UITexts } from '../../i18n/index.ts'
import type { UIAction, UIActionParams, UIActionProcessor, UIActionScope } from '../types.ts'
import { getattributs } from '../utils.tsx'

type Props<T = any, TScope extends UIActionScope = any> = {
  actions: UIAction<T, TScope>[]
  params: Omit<UIActionParams<T, TScope>, 'action'>
  texts: UITexts
  actionProcessor?: UIActionProcessor
  horizontal?: boolean
}

export function ActionButtons<T = any, TScope extends UIActionScope = any>({
  actions,
  params,
  texts,
  actionProcessor,
  horizontal = true,
}: Props<T, TScope>) {
  if (actions.length === 0) return null

  const renderButtons = () => (
    <>
      {actions.map((action, index) => {
        const fullParams = { action, ...params } as unknown as UIActionParams<T, TScope>
        const { label, icon } = getattributs(fullParams, texts)
        const onAction = () => {
          if (action.onAction) void action.onAction(fullParams)
          else void actionProcessor?.(fullParams)
        }
        return (
          <Button key={`ctb-${index}`} leftSection={icon ? <Icon icon={icon} size={16} /> : null} onClick={onAction}>
            {label}
          </Button>
        )
      })}
    </>
  )

  return horizontal ? <Group gap="xs">{renderButtons()}</Group> : <Stack gap="xs">{renderButtons()}</Stack>
}

// SPDX-License-Identifier: Apache-2.0
// ActionIcons — horizontal row of icon-only ActionIcon buttons + an
// overflow Menu when actions exceed `maxIcons`. Used by
// `buildRowMenuRenderer` (per-row table actions in tight cells).
//
// Ported from _ktw_workspace/.../cpm-core/actions/menus/ActionIcons.tsx
// during gui-action Subtask 1 (2026-05-04). Adaptations same as
// ActionButtons.tsx (icon path, drop UITexts default).

import { ActionIcon, Group, Menu, Stack, Tooltip } from '@mantine/core'
import { IconDots } from '@tabler/icons-react'

import { Icon } from '../../components/icons/index.ts'
import type { UITexts } from '../../i18n/index.ts'
import type { UIAction, UIActionParams, UIActionProcessor, UIActionScope } from '../types.ts'
import { getattributs } from '../utils.tsx'

type Props<T = any, TScope extends UIActionScope = any> = {
  actions: UIAction<T, TScope>[]
  params: Omit<UIActionParams<T, TScope>, 'action'>
  texts: UITexts
  actionProcessor?: UIActionProcessor
  maxIcons?: number
  horizontal?: boolean
  iconSize?: number
}

export function ActionIcons<T = any, TScope extends UIActionScope = any>({
  actions,
  params,
  texts,
  actionProcessor,
  maxIcons = 3,
  horizontal = true,
  iconSize = 16,
}: Props<T, TScope>) {
  const renderIcons = () => (
    <>
      {actions.slice(0, maxIcons).map((action, idx) => {
        const fullParams = { action, ...params } as unknown as UIActionParams<T, TScope>
        const { label, icon, color, enabled } = getattributs(fullParams, texts)
        const onAction = () => {
          if (action.onAction) void action.onAction(fullParams)
          else void actionProcessor?.(fullParams)
        }
        return (
          <Tooltip key={idx} label={label}>
            <ActionIcon variant="subtle" color={color} onClick={onAction} disabled={!enabled}>
              {icon ? <Icon icon={icon} size={iconSize} /> : null}
            </ActionIcon>
          </Tooltip>
        )
      })}

      {/* Overflow menu when actions exceed maxIcons */}
      {actions.length > maxIcons && (
        <Menu position="bottom-end">
          <Menu.Target>
            <ActionIcon variant="subtle">
              <IconDots size={16} />
            </ActionIcon>
          </Menu.Target>
          <Menu.Dropdown>
            {actions.slice(maxIcons).map((action, idx) => {
              const fullParams = { action, ...params } as unknown as UIActionParams<T, TScope>
              const { label, icon, color, enabled } = getattributs(fullParams, texts)
              const onAction = () => {
                if (action.onAction) void action.onAction(fullParams)
                else void actionProcessor?.(fullParams)
              }
              return (
                <Menu.Item
                  key={idx}
                  leftSection={icon ? <Icon icon={icon} size={16} /> : null}
                  onClick={onAction}
                  disabled={!enabled}
                  color={color}
                >
                  {label}
                </Menu.Item>
              )
            })}
          </Menu.Dropdown>
        </Menu>
      )}
    </>
  )

  return horizontal ? (
    <Group gap="xs" justify="center" wrap="nowrap">
      {renderIcons()}
    </Group>
  ) : (
    <Stack gap="xs">{renderIcons()}</Stack>
  )
}

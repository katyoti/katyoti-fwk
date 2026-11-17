// SPDX-License-Identifier: Apache-2.0
// ActionMenu — dropdown menu triggered by a button or icon. Useful for
// "more actions" surfaces: end-of-row toolbox, in-form actions, etc.
//
// Ported from _ktw_workspace/.../cpm-core/actions/menus/ActionMenu.tsx
// during gui-action Subtask 1 (2026-05-04). Adaptations:
//   - `@core/cpm-core/icons` → `'../../components/icons/index.ts'`
//   - `@core/cpm-core/texts` → drop; UITexts is a prop
//   - `'../utils'` → `'../utils.tsx'` (our utils file)

import { ActionIcon, Button, Menu } from '@mantine/core'
import { IconDotsVertical } from '@tabler/icons-react'
import type { ComponentType, MouseEvent, ReactNode } from 'react'
import { useCallback } from 'react'

import { Icon } from '../../components/icons/index.ts'
import type { UITexts } from '../../i18n/index.ts'
import type { UIActionMenuItem, UIActionParams, UIActionProcessor, UIActionScope } from '../types.ts'
import { getattributs, isUISeparator, resolveUIActionValue } from '../utils.tsx'

export interface ActionMenuProps<T = any, TScope extends UIActionScope = any> {
  /** Menu items (actions + separators). */
  actions: UIActionMenuItem<T, TScope>[]

  params: Omit<UIActionParams<T, TScope>, 'action'>
  actionProcessor: UIActionProcessor<T>
  texts: UITexts

  /** Trigger variant. */
  trigger?: 'icon' | 'button'

  /** Button label (button variant). */
  buttonLabel?: string
  /** Button icon (button variant). */
  buttonIcon?: ComponentType<{ size?: number }>
  /** Icon size for icon trigger. */
  iconSize?: number
  /** Button/icon variant. */
  variant?: 'filled' | 'light' | 'outline' | 'subtle' | 'transparent' | 'default'
  /** Button/icon size. */
  size?: 'xs' | 'sm' | 'md' | 'lg' | 'xl'
  /** Whether to use portal. */
  withinPortal?: boolean
  /** Menu position. */
  position?:
    | 'top'
    | 'left'
    | 'right'
    | 'bottom'
    | 'top-start'
    | 'top-end'
    | 'right-start'
    | 'right-end'
    | 'bottom-start'
    | 'bottom-end'
    | 'left-start'
    | 'left-end'

  /** Custom trigger element. */
  customTrigger?: ReactNode
  /** Callback when trigger is clicked. */
  onTriggerClick?: (event: MouseEvent) => void
  onOpen?: () => void
  onClose?: () => void
}

export function ActionMenu<T = any, TScope extends UIActionScope = any>({
  actions,
  params,
  onOpen,
  onClose,
  trigger = 'icon',
  buttonLabel,
  buttonIcon: ButtonIcon,
  iconSize = 16,
  variant = 'subtle',
  size = 'sm',
  withinPortal = true,
  position = 'bottom-end',
  customTrigger,
  onTriggerClick,
  actionProcessor,
  texts,
}: ActionMenuProps<T, TScope>) {
  const handleActionClick = useCallback(
    async (action: UIActionMenuItem<T, TScope>, event: MouseEvent) => {
      event.preventDefault()
      event.stopPropagation()

      if (isUISeparator(action)) return

      const fullParams = { action, ...params } as unknown as UIActionParams<T, TScope>
      const enabled = resolveUIActionValue<boolean, T, TScope>(action.enabled, fullParams, true)
      if (!enabled) return

      await actionProcessor(fullParams)
    },
    [params, actionProcessor],
  )

  const visibleItems = actions.filter((menuItem) => {
    if (isUISeparator(menuItem)) return true
    const fullParams = { action: menuItem, ...params } as unknown as UIActionParams<T, TScope>
    return resolveUIActionValue<boolean, T, TScope>(menuItem.visible, fullParams, true)
  })

  const renderTrigger = () => {
    if (customTrigger) return customTrigger

    if (trigger === 'button') {
      return (
        <Button variant={variant} size={size} leftSection={ButtonIcon ? <ButtonIcon size={iconSize} /> : null} onClick={onTriggerClick}>
          {buttonLabel || 'Actions'}
        </Button>
      )
    }

    return (
      <ActionIcon
        variant={variant}
        size={size}
        onClick={(e) => {
          e.stopPropagation()
          onTriggerClick?.(e)
        }}
      >
        <IconDotsVertical size={iconSize} />
      </ActionIcon>
    )
  }

  return (
    <Menu position={position} withinPortal={withinPortal} closeOnItemClick closeOnClickOutside onOpen={onOpen} onClose={onClose}>
      <Menu.Target>{renderTrigger()}</Menu.Target>
      <Menu.Dropdown onClick={(e) => e.stopPropagation()}>
        {visibleItems.map((menuItem) => {
          if (isUISeparator(menuItem)) {
            return <Menu.Divider key={menuItem.id} />
          }

          const fullParams = { action: menuItem, ...params } as unknown as UIActionParams<T, TScope>
          const { label, icon, color, enabled } = getattributs(fullParams, texts)

          if (menuItem.render) {
            return (
              <Menu.Item key={menuItem.id} disabled={!enabled} onClick={(e) => handleActionClick(menuItem, e)}>
                {menuItem.render(fullParams)}
              </Menu.Item>
            )
          }

          return (
            <Menu.Item
              key={menuItem.id}
              leftSection={icon ? <Icon icon={icon} size={16} /> : null}
              color={color}
              disabled={!enabled}
              onClick={(e) => handleActionClick(menuItem, e)}
            >
              {label}
            </Menu.Item>
          )
        })}
      </Menu.Dropdown>
    </Menu>
  )
}

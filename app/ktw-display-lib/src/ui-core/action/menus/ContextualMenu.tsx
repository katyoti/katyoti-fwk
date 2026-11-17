// SPDX-License-Identifier: Apache-2.0
// ContextualMenu — right-click context menu rendered via Mantine Menu
// with Portal-based positioning. Wraps `children` and listens for
// `contextmenu` to open at the cursor.
//
// Ported from _ktw_workspace/.../cpm-core/actions/menus/ContextualMenu.tsx
// during gui-action Subtask 1 (2026-05-04). Adaptations:
//   - `@core/cpm-core/icons` → `'../../components/icons/index.ts'`
//   - `@core/cpm-core/texts` → drop; UITexts is a prop
//   - Dropped dangling import `'./types'` (`ContextualMenuConfig` was never used)
//   - Loose params shape (`{item, selectedItems, action}` regardless of scope)
//     kept as in the source — discriminated UIActionParams cast at the boundary

import { type FloatingPosition, Menu } from '@mantine/core'
import type { MouseEvent, ReactNode } from 'react'
import { useCallback, useRef, useState } from 'react'

import { Icon } from '../../components/icons/index.ts'
import type { UITexts } from '../../i18n/index.ts'
import type { UIAction, UIActionMenuItem, UIActionParams } from '../types.ts'
import { getattributs, isUISeparator, resolveUIActionValue } from '../utils.tsx'

export interface ContextualMenuProps<T = any> {
  /** Menu items (actions + separators). */
  items: UIActionMenuItem<T>[]

  /** Current item (single-item actions). */
  item?: T
  /** Selected items (multi-item actions). */
  selectedItems?: T[]

  onOpen?: () => void
  onClose?: () => void
  /** Action click callback (host-resolved). */
  onActionClick: (params: UIActionParams<T>) => Promise<void>

  /** Children that trigger the menu (right-click target). */
  children: ReactNode

  withinPortal?: boolean
  position?: FloatingPosition
  className?: string
  texts: UITexts
}

export function ContextualMenu<T = any>({
  items,
  item,
  selectedItems = [],
  onOpen,
  onClose,
  onActionClick,
  children,
  withinPortal = true,
  position = 'right-start',
  className,
  texts,
}: ContextualMenuProps<T>) {
  const [opened, setOpened] = useState(false)
  const [menuPosition, setMenuPosition] = useState<{ x: number; y: number } | null>(null)
  const targetRef = useRef<HTMLDivElement>(null)

  const handleContextMenu = useCallback(
    (event: MouseEvent) => {
      event.preventDefault()
      event.stopPropagation()
      setMenuPosition({ x: event.clientX, y: event.clientY })
      setOpened(true)
      onOpen?.()
    },
    [onOpen],
  )

  const handleClose = useCallback(() => {
    setOpened(false)
    setMenuPosition(null)
    onClose?.()
  }, [onClose])

  const handleActionClick = useCallback(
    async (action: UIActionMenuItem<T>, event: MouseEvent) => {
      event.preventDefault()
      event.stopPropagation()
      if (isUISeparator(action)) return
      const params = { item, selectedItems, action } as unknown as UIActionParams<T>
      const enabled = resolveUIActionValue<boolean, T>(
        (action as UIAction<T>).enabled,
        params,
        true,
      )
      if (!enabled) return
      handleClose()
      await onActionClick(params)
    },
    [item, selectedItems, handleClose, onActionClick],
  )

  const visibleItems = items.filter((menuItem) => {
    if (isUISeparator(menuItem)) return true
    const params = { item, selectedItems, action: menuItem } as unknown as UIActionParams<T>
    return resolveUIActionValue<boolean, T>(menuItem.visible, params, true)
  })

  return (
    <div ref={targetRef} onContextMenu={handleContextMenu} className={className}>
      {children}

      <Menu
        opened={opened}
        onChange={setOpened}
        onClose={handleClose}
        withinPortal={withinPortal}
        position={position}
        closeOnItemClick
        closeOnClickOutside
      >
        <Menu.Target>
          <div style={{ position: 'absolute', left: menuPosition?.x, top: menuPosition?.y }} />
        </Menu.Target>

        <Menu.Dropdown>
          {visibleItems.map((menuItem, index) => {
            if (isUISeparator(menuItem)) {
              return <Menu.Divider key={menuItem.id ?? index.toString()} />
            }

            const action = menuItem as UIAction<T>
            const params = { item, selectedItems, action: menuItem } as unknown as UIActionParams<T>
            const { label, icon, color, enabled } = getattributs(params, texts)

            if (action.render) {
              return (
                <Menu.Item key={action.id ?? index.toString()} disabled={!enabled} onClick={(e) => handleActionClick(action, e)}>
                  {action.render(params)}
                </Menu.Item>
              )
            }

            return (
              <Menu.Item
                key={action.id ?? index.toString()}
                leftSection={icon ? <Icon icon={icon} size={16} /> : null}
                color={color}
                disabled={!enabled}
                onClick={(e) => handleActionClick(action, e)}
              >
                {label}
              </Menu.Item>
            )
          })}
        </Menu.Dropdown>
      </Menu>
    </div>
  )
}

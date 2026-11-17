// SPDX-License-Identifier: Apache-2.0
/**
 * ToastStack — transient, auto-dismissing feedback banners.
 *
 * sdd-ux-p0: when an agent task creates/updates a workspace node or item,
 * useAgentEvents pushes a toast so the user SEES the artifact appear
 * ("watch the dossier fill") instead of the tree silently changing.
 *
 * No notifications dependency — built on Mantine core's standalone
 * <Notification> rendered into a fixed bottom-right portal. Each item
 * owns its own dismiss timer.
 */

import { useEffect } from 'react'
import { Notification, Portal, Stack } from '@mantine/core'
import { useAppStore } from '../../store'

const DISMISS_MS = 4000

function ToastItem({
  id, message, color, onClose,
}: {
  id: string
  message: string
  color?: string
  onClose: (id: string) => void
}) {
  useEffect(() => {
    const h = window.setTimeout(() => onClose(id), DISMISS_MS)
    return () => window.clearTimeout(h)
  }, [id, onClose])

  return (
    <Notification
      color={color ?? 'teal'}
      onClose={() => onClose(id)}
      withBorder
      style={{ boxShadow: 'var(--mantine-shadow-md)' }}
    >
      {message}
    </Notification>
  )
}

export function ToastStack() {
  const toasts       = useAppStore((s) => s.toasts)
  const dismissToast = useAppStore((s) => s.dismissToast)

  if (toasts.length === 0) return null

  return (
    <Portal>
      <Stack
        gap="xs"
        style={{
          position: 'fixed',
          bottom: 16,
          right: 16,
          zIndex: 1000,
          maxWidth: 380,
        }}
      >
        {toasts.map((t) => (
          <ToastItem
            key={t.id}
            id={t.id}
            message={t.message}
            color={t.color}
            onClose={dismissToast}
          />
        ))}
      </Stack>
    </Portal>
  )
}

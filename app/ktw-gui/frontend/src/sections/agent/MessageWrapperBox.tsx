// SPDX-License-Identifier: Apache-2.0
/**
 * MessageWrapperBox — boxed message display
 * ─────────────────────────────────────────────────────────────────────────
 * Paper with colored left border, collapsible title bar, content area.
 * Used for tool calls, system messages — anything that needs a box.
 */
import { useState } from 'react'
import {
  ActionIcon,
  Box,
  Group,
  Menu,
  Paper,
  Text,
  ThemeIcon,
} from '@mantine/core'
import {
  IconCopy,
  IconDots,
  IconTrash,
} from '@tabler/icons-react'
import type { ReactNode } from 'react'
import type { MessageRoute } from './messageRegistry'
import { useAppStore } from '../../store'
import { useT } from '../../i18n'

interface Props {
  route:    MessageRoute
  children: ReactNode
}

export function MessageWrapperBox({ route, children }: Props) {
  const fullWidth = useAppStore((s) => s.appSettings.general.fullWidthMessages)
  const [open, setOpen] = useState(route.defaultOpen)
  const toggle = () => setOpen((o) => !o)

  return (
    <Paper
      withBorder
      radius="md"
      p={0}
      style={{
        overflow: 'hidden',
        borderLeft: `3px solid var(--mantine-color-${route.color}-6)`,
        ...(fullWidth ? {} : { width: 'fit-content', maxWidth: '100%' }),
      }}
    >
      {/* Title bar — colored bg, click anywhere to toggle */}
      <Group
        justify="space-between"
        gap={4}
        px={8}
        py={6}
        wrap="nowrap"
        onClick={toggle}
        style={{
          background: `var(--mantine-color-${route.color}-light)`,
          borderBottom: open
            ? `1px solid var(--mantine-color-${route.color}-light-hover)`
            : 'none',
          cursor: 'pointer',
          userSelect: 'none',
          minHeight: 32,
        }}
      >
        {/* Left: icon + summary */}
        <Group gap={6} wrap="nowrap" style={{ flex: 1, overflow: 'hidden' }}>
          <ThemeIcon
            size={18}
            radius="xl"
            color={route.color}
            variant="light"
            style={{ flexShrink: 0 }}
          >
            {route.icon}
          </ThemeIcon>
          <Text
            size="xs"
            fw={500}
            style={{
              color: `var(--mantine-color-${route.color}-light-color)`,
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
              flex: 1,
            }}
          >
            {route.summary}
          </Text>
        </Group>

        {/* Right: context menu */}
        <Box onClick={(e) => e.stopPropagation()}>
          <ContextMenu />
        </Box>
      </Group>

      {/* Content — only rendered when expanded */}
      {open && children}
    </Paper>
  )
}

// ── Shared context menu ───────────────────────────────────────────────────

function ContextMenu() {
  const t = useT('chat')
  return (
    <Menu shadow="md" width={150} position="bottom-end">
      <Menu.Target>
        <ActionIcon variant="transparent" color="gray" size="xs" title={t('message.more')}>
          <IconDots size={12} />
        </ActionIcon>
      </Menu.Target>
      <Menu.Dropdown>
        <Menu.Item leftSection={<IconCopy  size={13} />} fz={12}>{t('message.copy')}</Menu.Item>
        <Menu.Divider />
        <Menu.Item leftSection={<IconTrash size={13} />} fz={12} color="red">{t('message.delete')}</Menu.Item>
      </Menu.Dropdown>
    </Menu>
  )
}

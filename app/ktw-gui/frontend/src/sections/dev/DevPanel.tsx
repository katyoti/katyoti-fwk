// SPDX-License-Identifier: Apache-2.0
/**
 * DevPanel — in-app developer console (bottom drawer).
 *
 * Renders the captured console / error / IPC log from useDevStore. Toggled
 * from the IconRibbon's terminal button. A lightweight stand-in for browser
 * devtools that works in any build; for full network/DOM inspection a
 * `-devtools` build still offers Ctrl+Shift+I.
 */

import { useMemo, useRef, useState, useEffect } from 'react'
import { ActionIcon, Badge, Box, Group, ScrollArea, SegmentedControl, Text, Tooltip } from '@mantine/core'
import { IconTrash, IconX, IconArrowDown } from '@tabler/icons-react'
import { useDevStore, type DevLevel } from '../../store/devStore'

const LEVEL_COLOR: Record<DevLevel, string> = {
  log: 'gray', info: 'blue', warn: 'yellow', error: 'red', ipc: 'teal', event: 'violet',
}

export function DevPanel() {
  const open = useDevStore((s) => s.open)
  const setOpen = useDevStore((s) => s.setOpen)
  const logs = useDevStore((s) => s.logs)
  const clear = useDevStore((s) => s.clear)
  const [filter, setFilter] = useState<'all' | 'ipc' | 'error' | 'console'>('all')
  const [follow, setFollow] = useState(true)
  const viewportRef = useRef<HTMLDivElement>(null)

  const shown = useMemo(() => {
    return logs.filter((l) => {
      if (filter === 'all') return true
      if (filter === 'ipc') return l.level === 'ipc'
      if (filter === 'error') return l.level === 'error' || l.level === 'warn'
      return l.level === 'log' || l.level === 'info' || l.level === 'warn' || l.level === 'error'
    })
  }, [logs, filter])

  useEffect(() => {
    if (follow && viewportRef.current) {
      viewportRef.current.scrollTo({ top: viewportRef.current.scrollHeight })
    }
  }, [shown, follow])

  if (!open) return null

  return (
    <Box
      style={{
        position: 'fixed',
        left: 44, right: 0, bottom: 0,
        height: 280,
        zIndex: 9998,
        background: 'var(--mantine-color-body)',
        borderTop: '2px solid var(--mantine-color-default-border)',
        display: 'flex', flexDirection: 'column',
        boxShadow: '0 -4px 16px rgba(0,0,0,0.3)',
      }}
    >
      <Group justify="space-between" px="sm" py={4} style={{ borderBottom: '1px solid var(--mantine-color-default-border)' }}>
        <Group gap="xs">
          <Text size="sm" fw={600}>Dev console</Text>
          <Badge size="xs" variant="light" color="gray">{shown.length}</Badge>
          <SegmentedControl
            size="xs"
            value={filter}
            onChange={(v) => setFilter(v as typeof filter)}
            data={[
              { label: 'All', value: 'all' },
              { label: 'IPC', value: 'ipc' },
              { label: 'Console', value: 'console' },
              { label: 'Errors', value: 'error' },
            ]}
          />
        </Group>
        <Group gap={4}>
          <Tooltip label={follow ? 'Auto-scroll on' : 'Auto-scroll off'} withArrow>
            <ActionIcon size="sm" variant={follow ? 'light' : 'subtle'} color="gray" onClick={() => setFollow((f) => !f)}>
              <IconArrowDown size={14} />
            </ActionIcon>
          </Tooltip>
          <Tooltip label="Clear" withArrow>
            <ActionIcon size="sm" variant="subtle" color="gray" onClick={clear}><IconTrash size={14} /></ActionIcon>
          </Tooltip>
          <Tooltip label="Close" withArrow>
            <ActionIcon size="sm" variant="subtle" color="gray" onClick={() => setOpen(false)}><IconX size={14} /></ActionIcon>
          </Tooltip>
        </Group>
      </Group>

      <ScrollArea style={{ flex: 1 }} viewportRef={viewportRef}>
        <Box p={6} style={{ fontFamily: 'monospace', fontSize: 11, lineHeight: 1.5 }}>
          {shown.length === 0 ? (
            <Text size="xs" c="dimmed" p="sm">No entries. Interact with the app to capture console / IPC / errors.</Text>
          ) : (
            shown.map((l) => (
              <Group key={l.id} gap={6} wrap="nowrap" align="flex-start">
                <Text span size="xs" c="dimmed" style={{ minWidth: 52, textAlign: 'right' }}>{(l.t / 1000).toFixed(2)}s</Text>
                <Badge size="xs" variant="light" color={LEVEL_COLOR[l.level]} style={{ minWidth: 46 }}>{l.level}</Badge>
                <Text span size="xs" style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-word', flex: 1 }}>{l.text}</Text>
              </Group>
            ))
          )}
        </Box>
      </ScrollArea>
    </Box>
  )
}

// SPDX-License-Identifier: Apache-2.0
/**
 * TerminalView — main pane viewer for a running terminal session.
 *
 * Mounted by BaseLayout when `mainPaneRoute.kind === 'terminal'`. Reads
 * the session from the terminalSessions store and renders:
 *   - log kind: scrollback in a monospace ScrollArea, auto-scrolled to
 *     bottom when new content arrives.
 *   - shell kind: a placeholder informing the user the actual session
 *     lives in an external window (ConPTY embedding is a follow-up).
 *
 * Per the v1 of the TERM tab redesign (2026-05-07).
 */

import { useEffect, useRef } from 'react'
import { ActionIcon, Box, Code, Group, Stack, Text, Tooltip } from '@mantine/core'
import { StateBadge } from '@ktw/display-lib/ui-core/components/other/index.ts'
import { IconEraser, IconX } from '@tabler/icons-react'
import { useTerminalSessionsStore } from '../../store/terminalSessionsSlice'
import { XtermView } from './XtermView'
import { useT } from '../../i18n'

interface Props {
  id: string
}

export function TerminalView({ id }: Props) {
  const t = useT('screens')
  const session = useTerminalSessionsStore((s) => s.sessions.get(id))
  const close = useTerminalSessionsStore((s) => s.close)
  const clearScrollback = useTerminalSessionsStore((s) => s.clearScrollback)
  const viewportRef = useRef<HTMLDivElement | null>(null)

  // Auto-scroll on every scrollback change. Cheap; the pre is rendered
  // as a single string so React only diffs the textContent.
  useEffect(() => {
    const v = viewportRef.current
    if (!v) return
    v.scrollTop = v.scrollHeight
  }, [session?.scrollback])

  if (!session) {
    return (
      <Stack align="center" justify="center" h="100%">
        <Text size="sm" c="dimmed">{t('terminals.notFound')}</Text>
      </Stack>
    )
  }

  return (
    <Stack gap={0} style={{ height: '100%', overflow: 'hidden' }}>
      {/* Header strip */}
      <Group
        gap="xs"
        justify="space-between"
        px="md"
        py={6}
        style={{ borderBottom: '1px solid var(--mantine-color-default-border)', flexShrink: 0 }}
      >
        <Group gap="xs">
          <Text size="sm" fw={600}>{session.label}</Text>
          <StateBadge value={session.status} tones={TERMINAL_TONES} />
          <Text size="xs" c="dimmed">
            <Code style={{ fontSize: 10 }}>{session.source}</Code>
          </Text>
        </Group>
        <Group gap={4}>
          {session.kind === 'log' && (
            <Tooltip label={t('terminals.clear')}>
              <ActionIcon variant="default" size="sm" onClick={() => clearScrollback(session.id)}>
                <IconEraser size={14} />
              </ActionIcon>
            </Tooltip>
          )}
          <Tooltip label={t('terminals.close')}>
            <ActionIcon variant="default" size="sm" color="red" onClick={() => close(session.id)}>
              <IconX size={14} />
            </ActionIcon>
          </Tooltip>
        </Group>
      </Group>

      {/* Body — scrollback or external-shell placeholder */}
      {session.kind === 'log' ? (
        <Box
          ref={viewportRef}
          style={{
            flex: 1,
            minHeight: 0,
            overflow: 'auto',
            background: 'var(--mantine-color-dark-9)',
            padding: 8,
          }}
        >
          <pre
            style={{
              margin: 0,
              fontFamily: 'ui-monospace, SFMono-Regular, monospace',
              fontSize: 12,
              lineHeight: 1.45,
              color: 'var(--mantine-color-gray-3)',
              whiteSpace: 'pre-wrap',
              wordBreak: 'break-word',
            }}
          >
            {session.scrollback || '(empty — waiting for output…)'}
          </pre>
        </Box>
      ) : (
        // Embedded interactive shell — xterm.js bridged to a Go-side
        // ConPTY/PTY via Wails events (shell.<id>.data / WriteShellPTY).
        <XtermView id={session.id} />
      )}
    </Stack>
  )
}

/** A terminal session's status, as a state badge's tone. */
const TERMINAL_TONES: Record<string, string> = { running: 'green', ended: 'gray', external: 'yellow' }

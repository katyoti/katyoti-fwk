// SPDX-License-Identifier: Apache-2.0
import {
  NavLink,
  ScrollArea,
  Stack,
  Text,
} from '@mantine/core'
import { IconCircleCheck, IconCircleX, IconLoader2, IconMessageCircle } from '@tabler/icons-react'
import type { SessionEntry } from '../../types/project'
import { useT, type TFn } from '../../i18n'

interface SessionListProps {
  /** Sessions to display. When empty, shows a "no sessions yet" message. */
  sessions: SessionEntry[]
}

function formatAge(startedAt: number, t: TFn): string {
  const ageMs = Date.now() - startedAt
  if (ageMs < 60_000)              return t('sessionList.justNow')
  if (ageMs < 3_600_000)            return t('sessionList.minutesAgo', { n: Math.floor(ageMs / 60_000) })
  if (ageMs < 24 * 3_600_000)        return t('sessionList.hoursAgo', { n: Math.floor(ageMs / 3_600_000) })
  return t('sessionList.daysAgo', { n: Math.floor(ageMs / (24 * 3_600_000)) })
}

function statusIcon(status: SessionEntry['status']) {
  switch (status) {
    case 'completed': return <IconCircleCheck size={14} color="var(--mantine-color-green-6)" />
    case 'failed':    return <IconCircleX     size={14} color="var(--mantine-color-red-6)" />
    case 'running':   return <IconLoader2     size={14} color="var(--mantine-color-yellow-6)" />
  }
}

/**
 * Sidebar sessions list. Pure session-history view — the run-mode
 * selector lives in ProjectPanel (above the tabs), not here.
 *
 * `sessions` prop comes from the active project's `Project.sessions` field.
 * Real backend wiring is deferred to a future ms10/CWM follow-up.
 */
export function SessionList({ sessions }: SessionListProps) {
  const t = useT('shell')
  return (
    <Stack
      gap={0}
      style={{
        height: '100%',
        background: 'var(--mantine-color-body)',
      }}
    >
      <ScrollArea style={{ flex: 1 }} scrollbarSize={4}>
        {sessions.length === 0 ? (
          <Stack align="center" gap="xs" py="xl">
            <IconMessageCircle size={32} stroke={1.2} color="var(--mantine-color-dimmed)" />
            <Text size="xs" c="dimmed" ta="center">
              {t('sessionList.empty')}
              <br />
              {t('sessionList.emptyHint')}
            </Text>
          </Stack>
        ) : (
          <Stack gap={0} py={4}>
            {sessions.map((s) => (
              <NavLink
                key={s.id}
                label={s.taskName}
                description={`${formatAge(s.startedAt, t)}${s.durationMs ? ` · ${Math.round(s.durationMs / 1000)}s` : ''}`}
                leftSection={statusIcon(s.status)}
                styles={{
                  root: { padding: '6px 12px', borderRadius: 4, margin: '0 4px' },
                  label: { fontSize: 13 },
                  description: { fontSize: 11 },
                }}
              />
            ))}
          </Stack>
        )}
      </ScrollArea>
    </Stack>
  )
}

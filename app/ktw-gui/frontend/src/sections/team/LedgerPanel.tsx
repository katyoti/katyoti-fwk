// SPDX-License-Identifier: Apache-2.0
/**
 * LedgerPanel — the LEDGER lens: renders the displayed run's shared-state
 * ledger timeline (the Magentic pattern). Each entry is a snapshot of the full
 * shared state at a monotonic version; the terminal entry is marked complete.
 *
 * Fed live by team.ledger events (teamSlice.onLedger) and cold-reconstructed
 * from the journal (ledger_snapshot / ledger_complete → scope.ledger). Same
 * shape either way.
 */
import { Badge, Code, Group, Stack, Text, Timeline } from '@mantine/core'
import { IconDatabase, IconCircleCheck } from '@tabler/icons-react'
import { useTeamStore, selectViewScope } from '../../store/teamSlice'
import { useT } from '../../i18n'

export function LedgerPanel() {
  const t = useT('team')
  const scope = useTeamStore(selectViewScope)
  const ledger = scope?.ledger ?? []

  if (ledger.length === 0) {
    return (
      <Stack p="md" gap="xs">
        <Text size="sm" c="dimmed">
          {t('ledger.empty')}
        </Text>
      </Stack>
    )
  }

  const latest = ledger[ledger.length - 1]

  return (
    <Stack gap={0} style={{ height: '100%', overflow: 'hidden' }}>
      <Group
        gap="xs"
        px="sm"
        py={8}
        style={{ borderBottom: '1px solid var(--mantine-color-dark-4)', flexShrink: 0 }}
      >
        <IconDatabase size={16} color="var(--mantine-color-grape-4)" />
        <Text size="sm" fw={600}>{t('ledger.title')}</Text>
        <Badge size="xs" color="grape" variant="light">v{latest.version}</Badge>
        {latest.complete && (
          <Badge size="xs" color="green" variant="light" leftSection={<IconCircleCheck size={10} />}>
            {t('ledger.complete')}
          </Badge>
        )}
        <Text size="xs" c="dimmed">{t('ledger.updates', { count: ledger.length })}</Text>
      </Group>

      <div style={{ flex: 1, overflowY: 'auto', padding: 16 }}>
        <Timeline bulletSize={20} lineWidth={1} active={ledger.length - 1} color="grape">
          {ledger.map((l, i) => (
            <Timeline.Item
              key={i}
              bullet={l.complete ? <IconCircleCheck size={11} /> : <IconDatabase size={11} />}
              title={<Text size="xs" fw={600}>v{l.version}{l.complete ? ` · ${t('ledger.complete')}` : ''}</Text>}
            >
              <Group gap={6} mb={4}>
                {l.ts > 0 && <Text size="xs" c="dimmed">{new Date(l.ts).toLocaleTimeString()}</Text>}
                {l.reason && <Text size="xs" c="gray.4">{l.reason}</Text>}
              </Group>
              <Code block style={{ whiteSpace: 'pre-wrap', fontSize: 11, maxHeight: 240, overflow: 'auto' }}>
                {safeJSON(l.state)}
              </Code>
            </Timeline.Item>
          ))}
        </Timeline>
      </div>
    </Stack>
  )
}

function safeJSON(v: unknown): string {
  try {
    return JSON.stringify(v, null, 2)
  } catch {
    return String(v)
  }
}

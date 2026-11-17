// SPDX-License-Identifier: Apache-2.0
/**
 * WorkflowStatusBar — top-of-tab strip showing the current workflow's runID,
 * state, elapsed time, and (when applicable) a dry-run badge.
 *
 * Only rendered when in live mode. Reads from useTeamStore.
 */
import { Badge, Group, Text } from '@mantine/core'
import { useEffect, useState } from 'react'
import { useTeamStore, selectViewScope, selectViewWorkflow, selectSubstitutedCount } from '../../store/teamSlice'
import { useT } from '../../i18n'

const STATE_COLOR: Record<string, string> = {
  running: 'blue',
  done: 'green',
  failed: 'red',
}

export function WorkflowStatusBar() {
  const t = useT('team')
  const scope = useTeamStore(selectViewScope)
  const wf = useTeamStore(selectViewWorkflow)
  const subCount = useTeamStore(selectSubstitutedCount)

  // Tick once per second so the elapsed counter advances while the workflow is running.
  const [, setTick] = useState(0)
  useEffect(() => {
    if (!wf || wf.state !== 'running') return
    const id = window.setInterval(() => setTick((n) => n + 1), 1000)
    return () => window.clearInterval(id)
  }, [wf])

  // The status bar is a LIVE affordance. For a cold/opened session there is no
  // running workflow, so suppress it entirely — the TeamPanel toolbar already
  // carries the 'journal' badge + task counts. (Without this, a done session
  // showed the misleading "No workflow running. Launch a task from the CLI…".)
  // NOTE: must come AFTER all hooks — an early return before them trips React
  // error #310 (fewer hooks rendered than the previous render).
  if (scope?.cold) return null

  if (!wf) {
    return (
      <Group
        gap="xs"
        px="sm"
        py={6}
        style={{ borderBottom: '1px solid var(--mantine-color-dark-4)', flexShrink: 0 }}
      >
        <Text size="xs" c="dimmed">{t('workflowStatus.none')}</Text>
      </Group>
    )
  }

  const elapsedMs = (wf.endedAt ?? Date.now()) - wf.startedAt
  const elapsed = formatElapsed(elapsedMs)

  return (
    <Group
      gap="xs"
      px="sm"
      py={6}
      style={{ borderBottom: '1px solid var(--mantine-color-dark-4)', flexShrink: 0 }}
    >
      <Text size="xs" fw={600}>{t('workflowStatus.workflow')}</Text>
      <Text size="xs" c="dimmed" ff="monospace">{wf.runId}</Text>
      {wf.name && <Text size="xs" c="dimmed">· {wf.name}</Text>}
      <Badge size="xs" color={STATE_COLOR[wf.state] ?? 'gray'} variant="light">{t(`common.state.${wf.state}`, { defaultValue: wf.state })}</Badge>
      <Text size="xs" c="dimmed">{elapsed}</Text>
      {subCount > 0 && (
        <Badge size="xs" color="violet" variant="light" title={t('workflowStatus.dryRunTip', { count: subCount })}>
          {t('workflowStatus.dryRun', { count: subCount })}
        </Badge>
      )}
    </Group>
  )
}

function formatElapsed(ms: number): string {
  if (ms < 1000) return `${ms}ms`
  const s = Math.floor(ms / 1000)
  if (s < 60) return `${s}s`
  const m = Math.floor(s / 60)
  return `${m}m ${s % 60}s`
}

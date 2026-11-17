// SPDX-License-Identifier: Apache-2.0
/**
 * RunDrawer — one run, from any tile, chip or history row: what it is, its
 * actions (open its session, stop it — killed 10 s later if it does not end —,
 * repeat it once ended), its parent and children (each opens in turn), and its
 * journal followed live while it runs.
 */

import { Fragment, useEffect, useState } from 'react'
import { Badge, Button, Drawer, Group, Modal, Stack, Text } from '@mantine/core'
import { IconExternalLink, IconPlayerStop, IconRepeat } from '@tabler/icons-react'
import { useT } from '../../i18n'
import { SessionJournalPanel } from '../shell/SessionJournalPanel'
import { STATUS_TONE, childrenOf, fmtDuration, isActive, originKind, type RunInfo } from './model'
import { useNow } from './LiveView'
import { panelClient } from '../../ktsp'
import { DisplayPanel } from '../panels/DisplayPanel'

export interface RunDrawerProps {
  universeName: string
  run: RunInfo | null
  runs: RunInfo[]
  onClose: () => void
  onPick: (r: RunInfo) => void
  onOpenSession: (r: RunInfo) => void
  onStop: (r: RunInfo) => Promise<void>
  onRepeat: (r: RunInfo) => Promise<void>
}

const LIVE_MS = 2000

export function RunDrawer({ universeName, run, runs, onClose, onPick, onOpenSession, onStop, onRepeat }: RunDrawerProps) {
  const t = useT('runs')
  const now = useNow()
  const [confirm, setConfirm] = useState(false)
  const [busy, setBusy] = useState(false)
  const [refreshKey, setRefreshKey] = useState(0)
  const live = !!run && isActive(run)
  // The run's display panels: the panels the panel server keeps
  // for this run, each drawn in place.
  const [panels, setPanels] = useState<Array<{ panel: string; title?: string }>>([])
  useEffect(() => {
    setPanels([])
    if (!run?.runId) return
    let gone = false
    panelClient(universeName).list({ owner: run.runId }).then(
      (out) => { if (!gone) setPanels((out.panels ?? []).map((p) => ({ panel: p.panel, title: p.title }))) },
      () => { if (!gone) setPanels([]) },
    )
    return () => { gone = true }
  }, [universeName, run?.runId, refreshKey]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    setRefreshKey(0)
    if (!live) return
    const timer = window.setInterval(() => setRefreshKey((k) => k + 1), LIVE_MS)
    return () => window.clearInterval(timer)
  }, [run?.runId, live]) // eslint-disable-line react-hooks/exhaustive-deps

  if (!run) return <Drawer opened={false} onClose={onClose} />
  const parent = run.parentRunId ? runs.find((r) => r.runId === run.parentRunId) : undefined
  const children = childrenOf(runs, run.runId)
  const ended = run.endedAt ? Date.parse(run.endedAt) : now

  const act = async (fn: () => Promise<void>) => { setBusy(true); try { await fn() } finally { setBusy(false) } }

  return (
    <Drawer opened onClose={onClose} position="right" size="lg" padding="md"
      title={<Group gap="sm"><span className="sc-dot" data-live={live} style={live ? undefined : { background: `var(--mantine-color-${STATUS_TONE[run.status] ?? 'gray'}-6)` }} /><Text fw={700}>{run.taskName}</Text></Group>}>
      <Stack gap="md">
        <Group gap={6}>
          <Badge variant="light" color={STATUS_TONE[run.status] ?? 'gray'}>{t(`status.${run.status}`, { defaultValue: run.status })}</Badge>
          <Badge variant="light" color="gray">{t(`console.origin.${originKind(run)}`, { defaultValue: originKind(run) })}</Badge>
          {run.group && <Badge variant="outline" color="gray">{run.group}</Badge>}
          {run.target && <Text size="xs" c="dimmed" ff="monospace">{run.target}</Text>}
        </Group>

        <Group gap="xs">
          {run.sessionId && <Button size="xs" leftSection={<IconExternalLink size={14} />} onClick={() => onOpenSession(run)}>{t('console.drawer.openSession')}</Button>}
          {live && <Button size="xs" color="red" variant="light" leftSection={<IconPlayerStop size={14} />} loading={busy} onClick={() => setConfirm(true)}>
            {run.status === 'queued' ? t('console.drawer.unqueue') : t('console.drawer.stop')}
          </Button>}
          {!live && <Button size="xs" variant="light" leftSection={<IconRepeat size={14} />} loading={busy} onClick={() => act(() => onRepeat(run))}>{t('console.drawer.repeat')}</Button>}
        </Group>
        {!live && <Text size="xs" c="dimmed">{t('console.drawer.repeatHint')}</Text>}

        <dl style={{ display: 'grid', gridTemplateColumns: 'auto minmax(0,1fr)', gap: '4px 14px', margin: 0, fontSize: 13 }}>
          <Text component="dt" size="sm" c="dimmed">{t('console.drawer.run')}</Text><Text component="dd" size="sm" ff="monospace" m={0}>{run.runId}</Text>
          {run.sessionId && <><Text component="dt" size="sm" c="dimmed">{t('console.drawer.session')}</Text><Text component="dd" size="sm" ff="monospace" m={0}>{run.sessionId}</Text></>}
          <Text component="dt" size="sm" c="dimmed">{t('console.drawer.started')}</Text><Text component="dd" size="sm" m={0}>{new Date(run.startedAt).toLocaleString()}</Text>
          <Text component="dt" size="sm" c="dimmed">{t('console.drawer.duration')}</Text><Text component="dd" size="sm" className="sc-mono" m={0}>{fmtDuration(ended - Date.parse(run.startedAt))}</Text>
          {run.state && <><Text component="dt" size="sm" c="dimmed">{t('console.drawer.state')}</Text><Text component="dd" size="sm" m={0}>{run.state}</Text></>}
          {run.error && <><Text component="dt" size="sm" c="dimmed">{t('console.drawer.why')}</Text><Text component="dd" size="sm" c="red" m={0}>{run.error}</Text></>}
          {(run.warnings ?? []).map((w, i) => <Fragment key={i}><Text component="dt" size="sm" c="dimmed">{t('console.drawer.warning')}</Text><Text component="dd" size="sm" c="yellow" m={0}>{w}</Text></Fragment>)}
        </dl>

        {(parent || children.length > 0 || run.parentRunId) && (
          <Stack gap={4}>
            <Text size="xs" tt="uppercase" fw={600} c="dimmed">{t('console.drawer.family')}</Text>
            <div className="sc-tree">
              {parent
                ? <button type="button" className="sc-node" onClick={() => onPick(parent)}><Badge size="xs" variant="light" color="violet">{t('console.drawer.parent')}</Badge>{parent.taskName}<Text span size="xs" c="dimmed" ml="auto">{t(`status.${parent.status}`, { defaultValue: parent.status })}</Text></button>
                : run.parentRunId && <Text size="xs" c="dimmed">{t('console.drawer.parentGone', { id: run.parentRunId })}</Text>}
              <div className="sc-node" data-me="true" style={{ marginLeft: 18, cursor: 'default' }}>{run.taskName}</div>
              {children.map((c) => (
                <button key={c.runId} type="button" className="sc-node" style={{ marginLeft: 36 }} onClick={() => onPick(c)}>
                  <span className="sc-dot" data-live={isActive(c)} style={isActive(c) ? undefined : { background: `var(--mantine-color-${STATUS_TONE[c.status] ?? 'gray'}-6)` }} />
                  {c.taskName}<Text span size="xs" c="dimmed" ml="auto">{t(`status.${c.status}`, { defaultValue: c.status })}</Text>
                </button>
              ))}
            </div>
          </Stack>
        )}

        {panels.length > 0 && (
          <Stack gap="xs" data-guide="run.panels">
            <Text size="xs" tt="uppercase" fw={600} c="dimmed">{t('console.drawer.panels')}</Text>
            {panels.map((p) => <DisplayPanel key={p.panel} universeName={universeName} panelId={p.panel} title={p.title} variant="inline" />)}
          </Stack>
        )}

        {run.sessionId && (
          <Stack gap={4}>
            <Text size="xs" tt="uppercase" fw={600} c="dimmed">{live ? t('console.drawer.journalLive') : t('console.drawer.journal')}</Text>
            <div style={{ border: '1px solid var(--mantine-color-default-border)', borderRadius: 8, maxHeight: 420, overflow: 'auto' }}>
              <SessionJournalPanel universeName={universeName} sessionId={run.sessionId} refreshKey={refreshKey} />
            </div>
          </Stack>
        )}
      </Stack>

      <Modal opened={confirm} onClose={() => setConfirm(false)} title={run.status === 'queued' ? t('console.drawer.unqueueTitle') : t('stop.title')} centered>
        <Stack gap="sm">
          <Text size="sm">{t('console.drawer.stopText', { task: run.taskName })}</Text>
          <Group justify="flex-end">
            <Button variant="default" onClick={() => setConfirm(false)}>{t('stop.cancel')}</Button>
            <Button color="red" loading={busy} onClick={() => act(async () => { await onStop(run); setConfirm(false) })}>{t('stop.confirm')}</Button>
          </Group>
        </Stack>
      </Modal>
    </Drawer>
  )
}

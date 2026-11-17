// SPDX-License-Identifier: Apache-2.0
/**
 * SchedulerConsole — Global → Scheduler: everything the universe runs, waits
 * on and will run, and every move a person makes on it.
 *
 *   Live    — the band, one lane per task group with its runs inside (its
 *             slots, the runs waiting without one, its queue), the ticker of
 *             the run events as the task service tells them;
 *   Queues  — the scheduler's to-dos by module, with each module's mode and
 *             the internal scheduler's state;
 *   History — the ended runs, filtered; the session files on disk.
 *
 * A run opens in a drawer from anywhere: its facts, its session (followed live
 * while it runs), its parent and children, stop or repeat.
 */

import { useCallback, useMemo, useState } from 'react'
import { Alert, Badge, Group, Tabs, Text } from '@mantine/core'
import { IconActivity, IconAlertTriangle, IconHistory, IconListCheck } from '@tabler/icons-react'
import { useAppStore } from '../../store'
import { usePersonId } from '../../store/profileStore'
import { useT } from '../../i18n'
import { HistoryView } from './HistoryView'
import { LiveView } from './LiveView'
import { QueuesView } from './QueuesView'
import { RunDrawer } from './RunDrawer'
import { isActive, type RunInfo, type Todo } from './model'
import { TASKS, TODOS, useScheduler } from './useScheduler'
import './scheduler.css'

type Tab = 'live' | 'queues' | 'history'

export function SchedulerConsole({ universeName }: { universeName: string }) {
  const t = useT('runs')
  const d = useScheduler(universeName)
  const personId = usePersonId()
  const setMainPaneRoute = useAppStore((s) => s.setMainPaneRoute)
  const [tab, setTab] = useState<Tab>('live')
  const [openId, setOpenId] = useState<string | null>(null)
  const [notice, setNotice] = useState<{ ok: boolean; text: string } | null>(null)

  const runs = d.runs ?? []
  const open = useMemo(() => runs.find((r) => r.runId === openId) ?? null, [runs, openId])
  const activeCount = runs.filter(isActive).length
  const openTodos = d.todos.filter((x) => x.state !== 'reported' || x.result?.status === 'fail' || x.result?.status === 'blocked').length
  const by = { id: personId || 'person', kind: 'person' }

  /** Every move: the service call, what it did said once, everything read again. */
  const move = useCallback(async (slot: string, input: unknown, done: string) => {
    try {
      await d.call(slot, input)
      setNotice({ ok: true, text: done })
    } catch (e) {
      setNotice({ ok: false, text: e instanceof Error ? e.message : String(e) })
    }
    d.refresh()
  }, [d])

  const openSession = useCallback((r: RunInfo) => {
    if (r.sessionId) setMainPaneRoute({ kind: 'session', universeName, sessionId: r.sessionId })
  }, [setMainPaneRoute, universeName])

  return (
    <div className="sc-root">
      <Tabs value={tab} onChange={(v) => setTab((v as Tab) ?? 'live')}>
        <Tabs.List px="md">
          <Tabs.Tab value="live" leftSection={<IconActivity size={14} />} rightSection={activeCount ? <Badge size="xs" variant="light" circle>{activeCount}</Badge> : null}>{t('console.tabs.live')}</Tabs.Tab>
          <Tabs.Tab value="queues" leftSection={<IconListCheck size={14} />} rightSection={openTodos ? <Badge size="xs" variant="light" color="gray" circle>{openTodos}</Badge> : null}>{t('console.tabs.queues')}</Tabs.Tab>
          <Tabs.Tab value="history" leftSection={<IconHistory size={14} />}>{t('console.tabs.history')}</Tabs.Tab>
          <Group ml="auto" gap={8} pr="xs" style={{ alignSelf: 'center' }}>
            <span className="sc-dot" data-live={!d.error} style={d.error ? { background: 'var(--mantine-color-red-6)' } : undefined} />
            <Text size="xs" c="dimmed">{d.error ? t('console.unreachable') : d.lastRead ? t('console.readAt', { time: new Date(d.lastRead).toLocaleTimeString() }) : t('panel.reading')}</Text>
          </Group>
        </Tabs.List>
      </Tabs>
      <div className="sc-scroll">
        {notice && (
          <Alert mb="sm" color={notice.ok ? 'green' : 'red'} variant="light" withCloseButton onClose={() => setNotice(null)} icon={notice.ok ? undefined : <IconAlertTriangle size={16} />}>
            {notice.text}
          </Alert>
        )}
        {d.error && <Alert mb="sm" color="red" variant="light" icon={<IconAlertTriangle size={16} />}>{t('panel.noAnswer', { error: d.error })}</Alert>}
        {d.warnings.map((w) => <Alert key={w} mb="sm" color="yellow" variant="light">{t('panel.serviceWarning', { warning: w, defaultValue: w })}</Alert>)}

        {tab === 'live' && (
          <LiveView runs={runs} groups={d.groups} events={d.events}
            onOpen={(r) => setOpenId(r.runId)}
            onGroup={(group, change) => move(TASKS + 'SetGroup', { group, ...change }, t('console.done.group', { group }))}
            onClear={(group) => move(TASKS + 'ClearGroup', { group }, t('console.done.cleared', { group }))} />
        )}
        {tab === 'queues' && (
          <QueuesView todos={d.todos} modes={d.modes} schedulerOn={d.schedulerOn}
            onRun={(x: Todo) => move(TODOS + 'Run', { id: x.id, by }, t('console.done.ran', { step: x.work?.step ?? x.title }))}
            onRetry={(x: Todo) => move('ktw-agents:work/retry', { id: x.id, by, note: 'retried from the scheduler console' }, t('console.done.retried', { step: x.work?.step ?? x.title }))}
            onMode={(module, mode) => move(TODOS + 'Mode', { module, mode, by }, t('console.done.mode', { module, mode: t(`console.queues.mode.${mode}`) }))}
            onTick={() => move(TODOS + 'Tick', {}, t('console.done.tick'))} />
        )}
        {tab === 'history' && (
          <HistoryView universeName={universeName} runs={runs} onOpen={(r) => setOpenId(r.runId)} onOpenSession={openSession}
            onRepeat={(r) => move(TASKS + 'Repeat', { runId: r.runId, origin: { kind: 'person', id: by.id } }, t('console.done.repeated', { task: r.taskName }))} />
        )}
      </div>
      <RunDrawer universeName={universeName} run={open} runs={runs} onClose={() => setOpenId(null)} onPick={(r) => setOpenId(r.runId)}
        onOpenSession={openSession}
        onStop={(r) => move(TASKS + 'Stop', { runId: r.runId }, t('console.done.stopped', { task: r.taskName }))}
        onRepeat={(r) => move(TASKS + 'Repeat', { runId: r.runId, origin: { kind: 'person', id: by.id } }, t('console.done.repeated', { task: r.taskName }))} />
    </div>
  )
}

// SPDX-License-Identifier: Apache-2.0
/**
 * QueuesView — the scheduler's side: each module's open to-dos (held by whom
 * until when, failed with why, a person's step, ready, retried after a
 * backoff, waiting and on what) with its mode, and the internal scheduler's
 * state. A ready agent to-do runs now; a failed one is retried; a module's
 * mode is changed — every move by the person at the screen.
 */

import { useMemo } from 'react'
import { Badge, Button, Group, SegmentedControl, Stack, Text, Tooltip } from '@mantine/core'
import { SectionCard } from '@ktw/display-lib/ui-core/components/other/index.ts'
import { IconPlayerPlay, IconRefresh } from '@tabler/icons-react'
import { useT } from '../../i18n'
import { fmtDuration, queues, todoStanding, type ModuleMode, type Todo } from './model'
import { useNow } from './LiveView'

const TONE: Record<string, string> = { held: 'blue', failed: 'red', person: 'violet', ready: 'green', retrying: 'orange', waiting: 'gray', reported: 'teal' }

export interface QueuesProps {
  todos: Todo[]
  modes: ModuleMode[]
  schedulerOn: boolean
  onRun: (t: Todo) => Promise<void>
  onRetry: (t: Todo) => Promise<void>
  onMode: (module: string, mode: string) => Promise<void>
  onTick: () => Promise<void>
}

export function QueuesView({ todos, modes, schedulerOn, onRun, onRetry, onMode, onTick }: QueuesProps) {
  const t = useT('runs')
  const now = useNow(5000)
  const qs = useMemo(() => queues(todos, modes, now), [todos, modes, now])
  return (
    <Stack gap="md">
      <Group justify="space-between">
        <Group gap="sm">
          <span className="sc-dot" data-live={schedulerOn} style={schedulerOn ? undefined : { background: 'var(--mantine-color-gray-6)' }} />
          <Text size="sm">{schedulerOn ? t('console.queues.schedulerOn') : t('console.queues.schedulerOff')}</Text>
        </Group>
        <Tooltip label={t('console.queues.tickHint')} withArrow>
          <Button size="xs" variant="light" leftSection={<IconRefresh size={14} />} onClick={() => onTick()}>{t('console.queues.tick')}</Button>
        </Tooltip>
      </Group>
      {qs.length === 0 && <Text size="sm" c="dimmed">{t('console.queues.empty')}</Text>}
      <div className="sc-mods">
        {qs.map((q) => (
          <SectionCard key={q.module} title={q.module} icon="list-check" color="teal"
            description={t('console.queues.count', { count: q.todos.length })}
            actions={<SegmentedControl size="xs" value={q.mode} onChange={(m) => onMode(q.module, m)}
              data={['manual', 'auto', 'off'].map((m) => ({ value: m, label: t(`console.queues.mode.${m}`) }))} />}>
            {q.todos.map((td) => <TodoRow key={td.id} todo={td} now={now} onRun={onRun} onRetry={onRetry} />)}
          </SectionCard>
        ))}
      </div>
    </Stack>
  )
}

function TodoRow({ todo, now, onRun, onRetry }: { todo: Todo; now: number; onRun: QueuesProps['onRun']; onRetry: QueuesProps['onRetry'] }) {
  const t = useT('runs')
  const st = todoStanding(todo, now)
  const step = todo.work?.step ?? todo.title.split(' — ')[0]
  let why = ''
  switch (st) {
    case 'held': why = t('console.queues.held', { by: todo.claim?.by?.id ?? '?', until: todo.claim?.until ? new Date(todo.claim.until).toLocaleTimeString() : '?' }); break
    case 'failed': why = todo.result?.summary ?? ''; break
    case 'person': why = t('console.queues.person', { what: todo.work?.interaction ?? '' }); break
    case 'retrying': why = t('console.queues.retrying', { n: (todo.attempts ?? 0) + 1, of: todo.max_attempts ?? 0, in: fmtDuration(Date.parse(todo.not_before!) - now) }); break
    case 'waiting': why = todo.waits || (todo.after?.length ? t('console.queues.after', { what: todo.after.join(', ') }) : ''); break
    case 'ready': why = t('console.queues.ready', { priority: todo.priority }); break
    case 'reported': why = todo.result?.summary ?? ''; break
  }
  return (
    <div className="sc-todo">
      <Badge size="xs" variant="light" color={TONE[st]}>{t(`console.queues.standing.${st}`)}</Badge>
      <div style={{ minWidth: 0 }}>
        <Text size="sm" className="sc-ellipsis"><b>{step}</b> <Text span size="xs" c="dimmed" ff="monospace">{todo.request ?? todo.target}</Text></Text>
        {why && <Text size="xs" c="dimmed" className="sc-ellipsis" title={why}>{why}</Text>}
      </div>
      {st === 'ready' && todo.for === 'agent'
        ? <Button size="compact-xs" variant="light" leftSection={<IconPlayerPlay size={12} />} onClick={() => onRun(todo)}>{t('console.queues.run')}</Button>
        : st === 'failed'
          ? <Button size="compact-xs" variant="light" color="orange" leftSection={<IconRefresh size={12} />} onClick={() => onRetry(todo)}>{t('console.queues.retry')}</Button>
          : <span />}
    </div>
  )
}

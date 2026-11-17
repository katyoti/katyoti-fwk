// SPDX-License-Identifier: Apache-2.0
/**
 * LiveView — the console's live tab: the band (what runs, waits, queues; the
 * runs ended in the last hour, a sparkline; the failures of the day), one lane
 * per task group with its runs inside — its slots, the runs waiting without
 * one, its queue in order — and the ticker of the events heard.
 */

import { useEffect, useMemo, useState } from 'react'
import { ActionIcon, Badge, Button, Group, NumberInput, Popover, SimpleGrid, Stack, Switch, Text, Tooltip } from '@mantine/core'
import { IconAdjustments, IconClearAll, IconHourglass } from '@tabler/icons-react'
import { KpiTile, SectionCard } from '@ktw/display-lib/ui-core/components/other/index.ts'
import { useT } from '../../i18n'
import {
  eventTone, fmtDuration, kpiTiles, kpis, lanes, originKind, throughput, waitsOf,
  type GroupLoad, type Lane, type LifecycleEvent, type RunInfo,
} from './model'

export interface LiveProps {
  runs: RunInfo[]
  groups: GroupLoad[]
  events: LifecycleEvent[]
  onOpen: (r: RunInfo) => void
  onGroup: (group: string, change: { on?: boolean; maxRunning?: number; queueLimit?: number }) => void
  onClear: (group: string) => void
}

/** A clock ticking every second: the durations move. */
export function useNow(ms = 1000): number {
  const [now, setNow] = useState(Date.now())
  useEffect(() => { const t = window.setInterval(() => setNow(Date.now()), ms); return () => window.clearInterval(t) }, [ms])
  return now
}

export function LiveView({ runs, groups, events, onOpen, onGroup, onClear }: LiveProps) {
  const t = useT('runs')
  const now = useNow()
  const k = useMemo(() => kpis(groups, runs, now), [groups, runs, now])
  const ls = useMemo(() => lanes(groups, runs), [groups, runs])
  const spark = useMemo(() => throughput(runs, now), [runs, Math.floor(now / 60000)]) // eslint-disable-line react-hooks/exhaustive-deps
  const waits = useMemo(() => waitsOf(events), [events])

  return (
    <>
      <SimpleGrid cols={{ base: 2, sm: 3, lg: 5 }} spacing="md">
        {kpiTiles(k).map((x) => (
          <KpiTile key={x.key} label={t(x.label)} value={x.value} description={t(x.sub, x.subArgs)}
            color={x.color} icon={x.icon} spark={x.spark ? spark : undefined} />
        ))}
      </SimpleGrid>
      <div className="sc-live">
        <div className="sc-lanes">
          {ls.length === 0 && <Text size="sm" c="dimmed">{t('console.noGroups')}</Text>}
          {ls.map((l) => (
            <LaneCard key={l.group.name} lane={l} now={now} waits={waits} onOpen={onOpen} onGroup={onGroup} onClear={onClear} />
          ))}
        </div>
        <aside className="sc-side">
          <SectionCard title={t('console.ticker.title')} icon="activity" color="teal" actions={<span className="sc-dot" data-live="true" />}>
            {events.length === 0
              ? <Text size="xs" c="dimmed">{t('console.ticker.empty')}</Text>
              : (
                <ol className="sc-ticker" aria-live="polite">
                  {events.map((e, i) => (
                    <li className="sc-ev" key={`${e.runId}:${e.transition}:${e.at}:${i}`}>
                      <time>{e.at ? new Date(e.at).toLocaleTimeString() : ''}</time>
                      <span className="sc-tr" style={{ color: `var(--mantine-color-${eventTone(e.transition)}-6)` }}>{t(`console.transition.${e.transition}`, { defaultValue: e.transition })}</span>
                      <span className="sc-ellipsis" title={e.reason}><b>{e.taskName || e.runId}</b>{e.reason ? ` — ${e.reason}` : ''}</span>
                    </li>
                  ))}
                </ol>
              )}
          </SectionCard>
        </aside>
      </div>
    </>
  )
}

function LaneCard({ lane, now, waits, onOpen, onGroup, onClear }: {
  lane: Lane; now: number; waits: Map<string, LifecycleEvent>
  onOpen: (r: RunInfo) => void
  onGroup: LiveProps['onGroup']; onClear: LiveProps['onClear']
}) {
  const t = useT('runs')
  const g = lane.group
  const pct = g.maxRunning ? Math.round((lane.running.length / g.maxRunning) * 100) : 0
  return (
    <SectionCard
      aria-label={g.name}
      dimmed={!g.on}
      title={g.name}
      icon="gauge"
      color="violet"
      description={
        t('console.lane.load', { running: lane.running.length, max: g.maxRunning, waiting: lane.waiting.length, queued: lane.queued.length })
        + (g.queueLimit ? ` · ${t('console.lane.queueLimit', { count: g.queueLimit })}` : '')
      }
      actions={<>
        <LimitsPopover group={g} onGroup={onGroup} />
        <Tooltip label={t('group.clear')} withArrow>
          <ActionIcon variant="subtle" disabled={lane.queued.length === 0} aria-label={t('group.clear')} onClick={() => onClear(g.name)}><IconClearAll size={16} /></ActionIcon>
        </Tooltip>
        <Tooltip label={g.on ? t('group.pause') : t('group.resume')} withArrow>
          <Switch size="sm" checked={g.on} aria-label={t('console.lane.on', { group: g.name })} onChange={(e) => onGroup(g.name, { on: e.currentTarget.checked })} />
        </Tooltip>
      </>}
    >
      <div className="sc-slotbar"><i style={{ width: `${pct}%` }} /></div>
      <Zone label={t('console.lane.slots')}>
        <div className="sc-slots">
          {lane.running.map((r) => <Tile key={r.runId} run={r} now={now} onOpen={onOpen} />)}
          {Array.from({ length: lane.free }, (_, i) => <div key={i} className="sc-free">{g.on ? t('console.lane.free') : t('console.lane.paused')}</div>)}
        </div>
      </Zone>
      {lane.waiting.length > 0 && (
        <Zone label={t('console.lane.waiting')}>
          <div className="sc-slots">{lane.waiting.map((r) => <Tile key={r.runId} run={r} now={now} wait={waits.get(r.runId)} onOpen={onOpen} />)}</div>
        </Zone>
      )}
      <Zone label={t('console.lane.queue')}>
        {lane.queued.length === 0
          ? <Text size="xs" c="dimmed">{t('console.lane.queueEmpty')}</Text>
          : (
            <div className="sc-queue">
              {lane.queued.map((r, i) => (
                <button key={r.runId} type="button" className="sc-chip" onClick={() => onOpen(r)}>
                  <span className="sc-pos">{i + 1}</span>
                  <span>{r.taskName}</span>
                  {r.target && <Text span size="xs" c="dimmed" ff="monospace">{r.target}</Text>}
                  <Text span size="xs" c="dimmed" className="sc-mono">{fmtDuration(now - Date.parse(r.startedAt))}</Text>
                </button>
              ))}
            </div>
          )}
      </Zone>
    </SectionCard>
  )
}

function Zone({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <Stack gap={6}>
      <Text size="xs" tt="uppercase" fw={600} c="dimmed" style={{ letterSpacing: '.06em' }}>{label}</Text>
      {children}
    </Stack>
  )
}

/** A run tile: its task, who launched it, its target, how long, what it does
 *  or waits on. */
export function Tile({ run, now, wait, onOpen }: { run: RunInfo; now: number; wait?: LifecycleEvent; onOpen: (r: RunInfo) => void }) {
  const t = useT('runs')
  const who = originKind(run)
  const waitingOn = wait?.waiting?.join(', ')
  return (
    <button type="button" className="sc-tile" data-state={run.status} data-stuck={!!run.stuckSince} onClick={() => onOpen(run)}>
      <Group gap={8} wrap="nowrap">
        <Text fw={600} size="sm" className="sc-ellipsis">{run.taskName}</Text>
        <Text size="xs" c="dimmed" className="sc-mono" ml="auto">{fmtDuration(now - Date.parse(run.startedAt))}</Text>
      </Group>
      {run.status === 'waiting' ? (
        <Text size="xs" c="orange" className="sc-ellipsis">
          <IconHourglass size={12} className="sc-breathe" style={{ verticalAlign: -2, marginRight: 4 }} />
          {waitingOn ? t('console.tile.waitsOn', { what: waitingOn }) : t('console.tile.waits')}
          {wait?.deadline ? ` · ${t('console.tile.until', { time: new Date(wait.deadline).toLocaleTimeString() })}` : ''}
        </Text>
      ) : (
        <Text size="xs" c="dimmed" className="sc-ellipsis">
          {run.stuckSince ? t('console.tile.stuck', { since: fmtDuration(now - Date.parse(run.stuckSince)) })
            : run.status === 'stopping' ? t('status.stopping')
            : run.state ? t('console.tile.state', { state: run.state }) : t('status.running')}
        </Text>
      )}
      <Group gap={6} wrap="nowrap">
        <Badge size="xs" variant="light" color={who === 'planner' ? 'violet' : who === 'agent' ? 'green' : who === 'system' ? 'gray' : 'blue'}>
          {t(`console.origin.${who}`, { defaultValue: who })}
        </Badge>
        {run.target && <Text size="xs" c="dimmed" ff="monospace" className="sc-ellipsis">{run.target}</Text>}
        {(run.warnings?.length ?? 0) > 0 && <Badge size="xs" color="yellow" variant="light">{run.warnings!.length}</Badge>}
      </Group>
    </button>
  )
}

/** The group's limits, changed live: its slots and its queue limit. */
function LimitsPopover({ group, onGroup }: { group: GroupLoad; onGroup: LiveProps['onGroup'] }) {
  const t = useT('runs')
  const [max, setMax] = useState<number>(group.maxRunning)
  const [limit, setLimit] = useState<number>(group.queueLimit ?? 0)
  useEffect(() => { setMax(group.maxRunning); setLimit(group.queueLimit ?? 0) }, [group.maxRunning, group.queueLimit])
  return (
    <Popover width={240} position="bottom-end" withArrow shadow="md">
      <Popover.Target>
        <Tooltip label={t('console.lane.limits')} withArrow>
          <ActionIcon variant="subtle" aria-label={t('console.lane.limits')}><IconAdjustments size={16} /></ActionIcon>
        </Tooltip>
      </Popover.Target>
      <Popover.Dropdown>
        <Stack gap="xs">
          <NumberInput label={t('console.lane.maxRunning')} min={1} value={max} onChange={(v) => setMax(Number(v) || 1)} />
          <NumberInput label={t('console.lane.queueLimitLabel')} description={t('console.lane.queueLimitHint')} min={0} value={limit} onChange={(v) => setLimit(Number(v) || 0)} />
          <Button size="xs" onClick={() => onGroup(group.name, { maxRunning: max, queueLimit: limit })}>{t('console.lane.apply')}</Button>
        </Stack>
      </Popover.Dropdown>
    </Popover>
  )
}

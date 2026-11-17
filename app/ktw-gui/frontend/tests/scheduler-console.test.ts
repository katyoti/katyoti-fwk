// SPDX-License-Identifier: Apache-2.0
// The scheduler console's model: each active run in its group's lane (slots,
// waiting beside them, the queue in order), the band's counts and throughput,
// the history of ended runs and its filters, the to-dos by module.

import { describe, expect, it } from 'bun:test'
import { readFileSync, readdirSync, statSync } from 'node:fs'
import { join } from 'node:path'

import {
  type GroupLoad, type RunInfo, type Todo,
  childrenOf, fmtDuration, history, kpiTiles, kpis, lanes, queues, throughput, todoStanding,
} from '../src/sections/scheduler/model'

const now = Date.parse('2026-10-07T15:00:00Z')
const at = (minAgo: number) => new Date(now - minAgo * 60_000).toISOString()
const run = (o: Partial<RunInfo>): RunInfo => ({ runId: 'r', taskName: 't', status: 'running', startedAt: at(1), ...o })

const groups: GroupLoad[] = [
  { name: 'user', running: 1, waiting: 1, queued: 0, maxRunning: 4, on: true },
  { name: 'planning', running: 1, waiting: 0, queued: 2, maxRunning: 1, on: true },
]
const runs: RunInfo[] = [
  run({ runId: 'a', group: 'user', status: 'running', startedAt: at(5) }),
  run({ runId: 'b', group: 'user', status: 'waiting' }),
  run({ runId: 'c', group: 'planning', status: 'running' }),
  run({ runId: 'q1', group: 'planning', status: 'queued', startedAt: at(3) }),
  run({ runId: 'q2', group: 'planning', status: 'queued', startedAt: at(2), priority: 5 }),
  run({ runId: 'x', group: 'build', status: 'running' }),
  run({ runId: 'done1', group: 'user', status: 'completed', endedAt: at(10) }),
  run({ runId: 'fail1', group: 'planning', status: 'failed', endedAt: at(120), error: 'it ran out of time', origin: { kind: 'planner' } }),
  run({ runId: 'child', parentRunId: 'a', status: 'running', group: 'user', startedAt: at(0) }),
]

describe('lanes', () => {
  const ls = lanes(groups, runs)
  const lane = (n: string) => ls.find((l) => l.group.name === n)!
  it('puts each active run in its group: running in the slots, waiting beside, the queue', () => {
    expect(lane('user').running.map((r) => r.runId)).toEqual(['a', 'child'])
    expect(lane('user').waiting.map((r) => r.runId)).toEqual(['b'])
    expect(lane('user').free).toBe(2)
    expect(lane('planning').free).toBe(0)
  })
  it('orders the queue by priority, then age', () => {
    expect(lane('planning').queued.map((r) => r.runId)).toEqual(['q2', 'q1'])
  })
  it('gives a run of an unlisted group a lane of its own; ended runs are in none', () => {
    expect(lane('build').running.map((r) => r.runId)).toEqual(['x'])
    expect(ls.flatMap((l) => [...l.running, ...l.waiting, ...l.queued]).some((r) => r.runId === 'done1')).toBe(false)
  })
})

describe('the band', () => {
  it('counts what runs, waits and queues, the slots of the groups that are on', () => {
    const k = kpis(groups, runs, now)
    expect([k.running, k.waiting, k.queued, k.slots]).toEqual([4, 1, 2, 5])
    expect(k.endedLastHour).toBe(1)
    expect(k.failedLastDay).toBe(1)
  })
  it('throughput puts each ended run in its slice of the last hour', () => {
    const t = throughput(runs, now, 6)
    expect(t.reduce((a, b) => a + b, 0)).toBe(1)
    expect(t[5]).toBe(1) // ended 10 min ago: the last slice
  })
  it('is five shared tiles in the planification colours, each with its icon; the ended one carries the sparkline', () => {
    const tiles = kpiTiles(kpis(groups, runs, now))
    expect(tiles.map((x) => [x.key, x.color])).toEqual([
      ['running', 'blue'], ['waiting', 'orange'], ['queued', 'indigo'], ['ended', 'green'], ['failed', 'red'],
    ])
    expect(tiles.every((x) => x.icon)).toBe(true)
    expect(tiles.filter((x) => x.spark).map((x) => x.key)).toEqual(['ended'])
    expect(tiles[0].subArgs).toEqual({ count: 5 })
  })
})

describe('one set of display components', () => {
  const sections = join(import.meta.dir, '../src/sections')
  const files = (dir: string): string[] => readdirSync(dir).flatMap((f) => {
    const p = join(dir, f)
    return statSync(p).isDirectory() ? files(p) : /\.(tsx|css)$/.test(f) ? [p] : []
  })
  it('no view declares its own KPI tile: they draw KpiTile from ktw-display-lib', () => {
    const own = files(sections).filter((p) => /function Kpi\b|function KpiCard\b|\.[a-z]+-kpi\b|className="[^"]*-kpi\b/.test(readFileSync(p, 'utf8')))
    expect(own.map((p) => p.slice(sections.length + 1))).toEqual([])
  })
  it('the scheduler console draws its band with KpiTile and its panels with SectionCard', () => {
    const live = readFileSync(join(sections, 'scheduler/LiveView.tsx'), 'utf8')
    const queues = readFileSync(join(sections, 'scheduler/QueuesView.tsx'), 'utf8')
    expect(live).toContain('<KpiTile')
    expect(live).toContain('<SectionCard')
    expect(queues).toContain('<SectionCard')
  })
})

describe('history', () => {
  it('keeps the ended runs, newest end first, and filters them', () => {
    expect(history(runs).map((r) => r.runId)).toEqual(['done1', 'fail1'])
    expect(history(runs, { status: 'failed' }).map((r) => r.runId)).toEqual(['fail1'])
    expect(history(runs, { origin: 'planner' }).map((r) => r.runId)).toEqual(['fail1'])
    expect(history(runs, { text: 'out of time' }).map((r) => r.runId)).toEqual(['fail1'])
  })
  it('finds a run\'s children', () => {
    expect(childrenOf(runs, 'a').map((r) => r.runId)).toEqual(['child'])
  })
})

describe('the to-dos', () => {
  const td = (o: Partial<Todo>): Todo => ({ id: 'm.TD-1', module: 'm', target: 'm.RQ-1', title: 'x', for: 'agent', priority: 'P2', state: 'ready', ...o })
  it('says where each stands', () => {
    expect(todoStanding(td({ state: 'ready', not_before: new Date(now + 60_000).toISOString() }), now)).toBe('retrying')
    expect(todoStanding(td({ state: 'claimed' }), now)).toBe('held')
    expect(todoStanding(td({ state: 'reported', result: { status: 'fail' } }), now)).toBe('failed')
    expect(todoStanding(td({ for: 'person' }), now)).toBe('person')
    expect(todoStanding(td({ state: 'waiting' }), now)).toBe('waiting')
  })
  it('groups them by module with its mode, what needs attention first', () => {
    const q = queues([td({ id: 'a', state: 'waiting' }), td({ id: 'b', state: 'claimed' }), td({ id: 'c', module: 'n' })], [{ module: 'm', mode: 'auto' }], now)
    expect(q.map((m) => [m.module, m.mode])).toEqual([['m', 'auto'], ['n', 'manual']])
    expect(q[0].todos.map((t) => t.id)).toEqual(['b', 'a'])
  })
})

describe('durations', () => {
  it('reads as a person would', () => {
    expect([fmtDuration(42_000), fmtDuration(365_000), fmtDuration(5_160_000)]).toEqual(['42s', '6m 05s', '1h 26m'])
  })
})

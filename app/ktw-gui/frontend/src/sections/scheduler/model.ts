// SPDX-License-Identifier: Apache-2.0
/**
 * The scheduler console's model — what the services answer, shaped for the
 * views. Pure functions: tested in tests/scheduler-console.test.ts.
 *
 * Services read (the universe's ktw-agents, through the generic KTSP binding):
 *   tasks/Runs, tasks/Groups — the task runtime's runs and groups;
 *   todos/List, todos/Modes  — the scheduler's to-dos and the modules' modes.
 */

// ── The wire shapes (internal/extensions/task/api, scheduler/api) ─────────

export interface RunInfo {
  runId: string
  taskName: string
  machineType?: string
  status: string
  state?: string
  parentRunId?: string
  sessionId?: string
  startedAt: string
  group?: string
  priority?: number
  origin?: { kind?: string; id?: string }
  target?: string
  endedAt?: string
  error?: string
  stuckSince?: string
  warnings?: string[]
}

export interface GroupLoad {
  name: string
  running: number
  waiting: number
  queued: number
  maxRunning: number
  queueLimit?: number
  priority?: number
  on: boolean
}

export interface KeepAliveInfo {
  task: string
  state: string
  runId?: string
  reason?: string
  nextAt?: string
  restarts: number
}

/** A run's lifecycle event as the task service tells it (engine.RunLifecycleEvent). */
export interface LifecycleEvent {
  runId?: string
  taskName?: string
  transition?: string
  reason?: string
  group?: string
  waiting?: string[]
  deadline?: string
  status?: string
  parentRunId?: string
  wokenBy?: string
  queuedForMs?: number
  at?: string
}

export interface Todo {
  id: string
  module: string
  target: string
  request?: string
  title: string
  for: string
  group?: string
  start?: string
  priority: string
  state: string
  since?: string
  after?: string[]
  waits?: string
  attempts?: number
  max_attempts?: number
  not_before?: string
  claim?: { by?: { id?: string; kind?: string }; at?: string; until?: string; run?: string; session?: string }
  result?: { status?: string; summary?: string }
  work?: { step?: string; interaction?: string; doer?: string }
}

export interface ModuleMode { module: string; mode: string }

// ── Runs ───────────────────────────────────────────────────────────────

export const ACTIVE = new Set(['queued', 'running', 'waiting', 'stopping', 'pending'])
export const isActive = (r: RunInfo) => ACTIVE.has(r.status)

/** A status's tone (its label is `runs:status.<status>`). */
export const STATUS_TONE: Record<string, string> = {
  queued: 'indigo', running: 'blue', waiting: 'orange', stopping: 'orange', pending: 'gray',
  completed: 'green', failed: 'red', cancelled: 'orange', killed: 'red',
}

/** A lane: one task group, the runs it holds, its free slots. */
export interface Lane {
  group: GroupLoad
  running: RunInfo[]
  waiting: RunInfo[]
  queued: RunInfo[]
  free: number
}

/** lanes puts each active run in its group's lane — running ones in its slots
 *  (oldest first), waiting ones beside them (they hold no slot), queued ones
 *  in their order (priority, then oldest). A run of a group the service does
 *  not list gets a lane of its own, so nothing active is hidden. */
export function lanes(groups: GroupLoad[], runs: RunInfo[]): Lane[] {
  const byName = new Map<string, Lane>()
  for (const g of groups) byName.set(g.name, { group: g, running: [], waiting: [], queued: [], free: 0 })
  for (const r of runs) {
    if (!isActive(r)) continue
    const name = r.group || 'user'
    let lane = byName.get(name)
    if (!lane) {
      lane = { group: { name, running: 0, waiting: 0, queued: 0, maxRunning: 0, on: true }, running: [], waiting: [], queued: [], free: 0 }
      byName.set(name, lane)
    }
    if (r.status === 'waiting') lane.waiting.push(r)
    else if (r.status === 'queued' || r.status === 'pending') lane.queued.push(r)
    else lane.running.push(r)
  }
  const out = [...byName.values()]
  for (const l of out) {
    const since = (r: RunInfo) => Date.parse(r.startedAt) || 0
    l.running.sort((a, b) => since(a) - since(b))
    l.waiting.sort((a, b) => since(a) - since(b))
    l.queued.sort((a, b) => (b.priority ?? 0) - (a.priority ?? 0) || since(a) - since(b))
    l.free = Math.max(0, (l.group.maxRunning || 0) - l.running.length)
  }
  // The busiest first, then by name: what moves is on top.
  return out.sort((a, b) => (b.running.length + b.waiting.length + b.queued.length) - (a.running.length + a.waiting.length + a.queued.length) || a.group.name.localeCompare(b.group.name))
}

export interface Kpis { running: number; waiting: number; queued: number; slots: number; endedLastHour: number; failedLastDay: number }

export function kpis(groups: GroupLoad[], runs: RunInfo[], now = Date.now()): Kpis {
  const hour = now - 3600_000, day = now - 86_400_000
  let running = 0, waiting = 0, queued = 0, endedLastHour = 0, failedLastDay = 0
  for (const r of runs) {
    if (r.status === 'running' || r.status === 'stopping') running++
    else if (r.status === 'waiting') waiting++
    else if (r.status === 'queued' || r.status === 'pending') queued++
    const end = r.endedAt ? Date.parse(r.endedAt) : NaN
    if (!isActive(r) && end >= hour) endedLastHour++
    if ((r.status === 'failed' || r.status === 'killed') && end >= day) failedLastDay++
  }
  const slots = groups.reduce((n, g) => n + (g.on ? g.maxRunning : 0), 0)
  return { running, waiting, queued, slots, endedLastHour, failedLastDay }
}

/** throughput counts the runs ended in each of `buckets` slices of `spanMs`
 *  up to now — the sparkline of the band, oldest first. */
export function throughput(runs: RunInfo[], now = Date.now(), buckets = 12, spanMs = 3600_000): number[] {
  const out = new Array<number>(buckets).fill(0)
  const width = spanMs / buckets, start = now - spanMs
  for (const r of runs) {
    if (isActive(r) || !r.endedAt) continue
    const t = Date.parse(r.endedAt)
    if (!(t >= start && t <= now)) continue
    out[Math.min(buckets - 1, Math.floor((t - start) / width))]++
  }
  return out
}

/** The run's children (its parentRunId is it), newest last. */
export function childrenOf(runs: RunInfo[], runId: string): RunInfo[] {
  return runs.filter((r) => r.parentRunId === runId).sort((a, b) => Date.parse(a.startedAt) - Date.parse(b.startedAt))
}

/** One tile of the band: what it counts, its colour and icon — the same
 *  colours as the planification deck's states (running blue, waiting orange,
 *  done green, failed red). Labels are keys of the runs namespace. */
export interface KpiTileSpec { key: string; label: string; sub: string; subArgs?: Record<string, unknown>; value: number; color: string; icon: string; spark?: boolean }

export function kpiTiles(k: Kpis): KpiTileSpec[] {
  return [
    { key: 'running', label: 'console.kpi.running', sub: 'console.kpi.ofSlots', subArgs: { count: k.slots }, value: k.running, color: 'blue', icon: 'loader-2' },
    { key: 'waiting', label: 'console.kpi.waiting', sub: 'console.kpi.noSlot', value: k.waiting, color: 'orange', icon: 'hourglass' },
    { key: 'queued', label: 'console.kpi.queued', sub: 'console.kpi.forSlot', value: k.queued, color: 'indigo', icon: 'list-numbers' },
    { key: 'ended', label: 'console.kpi.endedHour', sub: 'console.kpi.throughput', value: k.endedLastHour, color: 'green', icon: 'circle-check', spark: true },
    { key: 'failed', label: 'console.kpi.failedDay', sub: 'console.kpi.failedHint', value: k.failedLastDay, color: 'red', icon: 'alert-octagon' },
  ]
}

// ── History ────────────────────────────────────────────────────────────

export interface HistoryFilter { status?: string; group?: string; origin?: string; text?: string }

/** history: the ended runs, newest end first, kept by the filter. */
export function history(runs: RunInfo[], f: HistoryFilter = {}): RunInfo[] {
  const text = (f.text ?? '').trim().toLowerCase()
  return runs
    .filter((r) => !isActive(r))
    .filter((r) => !f.status || r.status === f.status)
    .filter((r) => !f.group || (r.group || '') === f.group)
    .filter((r) => !f.origin || originKind(r) === f.origin)
    .filter((r) => !text || [r.taskName, r.target, r.error, r.runId].some((s) => (s ?? '').toLowerCase().includes(text)))
    .sort((a, b) => (Date.parse(b.endedAt ?? b.startedAt) || 0) - (Date.parse(a.endedAt ?? a.startedAt) || 0))
}

/** Who launched it: you · planner · agent · system (a run with no origin is a person's). */
export function originKind(r: RunInfo): string {
  return r.origin?.kind || 'person'
}

// ── Events ─────────────────────────────────────────────────────────────

/** A transition's tone in the ticker. */
export function eventTone(transition = ''): string {
  switch (transition) {
    case 'started': case 'restarted': case 'resumed': return 'blue'
    case 'completed': return 'green'
    case 'failed': case 'killed': case 'interrupted': return 'red'
    case 'waiting': case 'woken': return 'orange'
    case 'queued': return 'indigo'
    case 'paused': case 'stopping': case 'cancelled': case 'stuck': case 'timed-out': return 'orange'
    default: return 'gray'
  }
}

/** What each run waits on, from the last waiting event heard for it. */
export function waitsOf(events: LifecycleEvent[]): Map<string, LifecycleEvent> {
  const out = new Map<string, LifecycleEvent>()
  for (const e of events) { // newest first
    if (!e.runId || out.has(e.runId)) continue
    if (e.transition === 'waiting') out.set(e.runId, e)
    else out.set(e.runId, {}) // a later move: what it waited on no longer holds
  }
  for (const [k, v] of out) if (!v.transition) out.delete(k)
  return out
}

/** keepEvents adds an event on top of the ticker, keeping the last `max`. */
export function keepEvents(list: LifecycleEvent[], e: LifecycleEvent, max = 40): LifecycleEvent[] {
  return [e, ...list].slice(0, max)
}

// ── To-dos ─────────────────────────────────────────────────────────────

export interface ModuleQueue { module: string; mode: string; todos: Todo[] }

/** The to-do's standing, as the console shows it. */
export function todoStanding(t: Todo, now = Date.now()): 'ready' | 'retrying' | 'held' | 'waiting' | 'person' | 'failed' | 'reported' {
  if (t.state === 'ready' && t.not_before && Date.parse(t.not_before) > now) return 'retrying'
  if (t.state === 'claimed') return 'held'
  if (t.state === 'reported') return t.result?.status === 'fail' || t.result?.status === 'blocked' ? 'failed' : 'reported'
  if (t.for === 'person' && t.state === 'ready') return 'person'
  if (t.state === 'ready') return 'ready'
  return 'waiting'
}

const STANDING_ORDER = ['held', 'failed', 'person', 'ready', 'retrying', 'waiting', 'reported']

/** queues groups the open to-dos by module (each with its mode), what needs
 *  attention first; modules with no to-do are left out. */
export function queues(todos: Todo[], modes: ModuleMode[], now = Date.now()): ModuleQueue[] {
  const mode = new Map(modes.map((m) => [m.module, m.mode]))
  const by = new Map<string, Todo[]>()
  for (const t of todos) by.set(t.module, [...(by.get(t.module) ?? []), t])
  const rank = (t: Todo) => STANDING_ORDER.indexOf(todoStanding(t, now))
  return [...by.entries()]
    .map(([module, list]) => ({ module, mode: mode.get(module) ?? 'manual', todos: list.sort((a, b) => rank(a) - rank(b) || (a.priority < b.priority ? -1 : a.priority > b.priority ? 1 : 0)) }))
    .sort((a, b) => a.module.localeCompare(b.module))
}

// ── Time ───────────────────────────────────────────────────────────────

/** A duration for a person: 42s · 6m 05s · 1h 26m. */
export function fmtDuration(ms: number): string {
  const s = Math.max(0, Math.floor(ms / 1000))
  const h = Math.floor(s / 3600), m = Math.floor((s % 3600) / 60), x = s % 60
  if (h) return `${h}h ${String(m).padStart(2, '0')}m`
  if (m) return `${m}m ${String(x).padStart(2, '0')}s`
  return `${x}s`
}

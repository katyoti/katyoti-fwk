// SPDX-License-Identifier: Apache-2.0
/**
 * useScheduler — what the scheduler console shows, kept current: the runs and
 * groups (re-read on each run event — `runs.changed`, which now carries the
 * event — and every 15 s besides), the to-dos and the modules' modes, and the
 * ticker of the events heard. Every action goes through the same services,
 * then re-reads.
 */

import { useCallback, useEffect, useRef, useState } from 'react'
import { dispatcherFor } from '../../ktsp'
import { EventsOn } from '../../../wailsjs/runtime/runtime'
import { keepEvents, type GroupLoad, type KeepAliveInfo, type LifecycleEvent, type ModuleMode, type RunInfo, type Todo } from './model'

const TASKS = 'ktw-agents:tasks/'
const TODOS = 'ktw-agents:todos/'
const POLL_MS = 15000
const CHANGE_DEBOUNCE_MS = 250

export interface SchedulerData {
  runs: RunInfo[] | null
  groups: GroupLoad[]
  keepAlive: KeepAliveInfo[]
  warnings: string[]
  todos: Todo[]
  modes: ModuleMode[]
  schedulerOn: boolean
  events: LifecycleEvent[]
  error: string | null
  lastRead: number
  refresh: () => void
  call: (slot: string, input: unknown) => Promise<unknown>
}

export function useScheduler(universeName: string): SchedulerData {
  const [runs, setRuns] = useState<RunInfo[] | null>(null)
  const [groups, setGroups] = useState<GroupLoad[]>([])
  const [keepAlive, setKeepAlive] = useState<KeepAliveInfo[]>([])
  const [warnings, setWarnings] = useState<string[]>([])
  const [todos, setTodos] = useState<Todo[]>([])
  const [modes, setModes] = useState<ModuleMode[]>([])
  const [schedulerOn, setSchedulerOn] = useState(true)
  const [events, setEvents] = useState<LifecycleEvent[]>([])
  const [error, setError] = useState<string | null>(null)
  const [lastRead, setLastRead] = useState(0)

  const call = useCallback((slot: string, input: unknown) => dispatcherFor(universeName)(slot, input), [universeName])

  const readRuns = useCallback(async () => {
    try {
      const [r, g] = await Promise.all([
        call(TASKS + 'Runs', {}) as Promise<{ runs?: RunInfo[] }>,
        call(TASKS + 'Groups', {}) as Promise<{ groups?: GroupLoad[]; warnings?: string[]; keepAlive?: KeepAliveInfo[] }>,
      ])
      setRuns(r?.runs ?? [])
      setGroups(g?.groups ?? [])
      setWarnings(g?.warnings ?? [])
      setKeepAlive(g?.keepAlive ?? [])
      setError(null)
      setLastRead(Date.now())
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
      setRuns((prev) => prev ?? [])
    }
  }, [call])

  const readTodos = useCallback(async () => {
    try {
      const [l, m] = await Promise.all([
        call(TODOS + 'List', {}) as Promise<{ todos?: Todo[] }>,
        call(TODOS + 'Modes', {}) as Promise<{ scheduler?: boolean; modules?: ModuleMode[] }>,
      ])
      setTodos(l?.todos ?? [])
      setModes(m?.modules ?? [])
      setSchedulerOn(m?.scheduler ?? true)
    } catch {
      // No scheduler in this universe's ktw-agents: the console shows the runs only.
      setTodos([])
    }
  }, [call])

  const refresh = useCallback(() => { void readRuns(); void readTodos() }, [readRuns, readTodos])

  useEffect(() => {
    setRuns(null)
    setEvents([])
    refresh()
    const timer = window.setInterval(() => { if (!document.hidden) refresh() }, POLL_MS)
    return () => window.clearInterval(timer)
  }, [refresh])

  // A run changed: the event goes on the ticker at once; the runs are read
  // again once the burst settles.
  const pending = useRef<number | null>(null)
  useEffect(() => {
    const off = EventsOn('runs.changed', (payload: { universe?: string; event?: LifecycleEvent | null }) => {
      if (payload?.universe && payload.universe !== universeName) return
      if (payload?.event) {
        const e = { ...payload.event, at: payload.event.at ?? new Date().toISOString() }
        setEvents((list) => keepEvents(list, e))
      }
      if (pending.current) window.clearTimeout(pending.current)
      pending.current = window.setTimeout(() => { pending.current = null; void readRuns() }, CHANGE_DEBOUNCE_MS)
    })
    return () => { off(); if (pending.current) window.clearTimeout(pending.current) }
  }, [universeName, readRuns])

  return { runs, groups, keepAlive, warnings, todos, modes, schedulerOn, events, error, lastRead, refresh, call }
}

export { TASKS, TODOS }

/** How many runs are active (queued, running, waiting, stopping) — the strip
 *  entry's badge: read on each run event and every 15 s. */
export function useActiveRunCount(universeName: string | null): number {
  const [count, setCount] = useState(0)
  const read = useCallback(async () => {
    if (!universeName) return
    try {
      const r = await dispatcherFor(universeName)(TASKS + 'Runs', { status: 'active' }) as { runs?: RunInfo[] }
      setCount(r?.runs?.length ?? 0)
    } catch { /* the console says why */ }
  }, [universeName])
  useEffect(() => {
    void read()
    const timer = window.setInterval(() => { if (!document.hidden) void read() }, POLL_MS)
    const off = EventsOn('runs.changed', (p: { universe?: string }) => { if (!p?.universe || p.universe === universeName) void read() })
    return () => { window.clearInterval(timer); off() }
  }, [read, universeName])
  return count
}

// SPDX-License-Identifier: Apache-2.0
/**
 * usePanelCounts — the count a declared panel's `count` action answers
 * (`{count: n; color?: tone}`), shown as its badge: on the Global strip beside
 * the panel's name (« À faire » with what waits on the person), on an object's
 * tab as the tab's badge (an increment's « Open requests »). Each panel is
 * read with its own arguments (an object's tab: `{id}` laid over its input);
 * equal inputs never re-run, an older answer is dropped once a newer read of
 * the same panel starts. Re-read whenever `revision` changes (an object's
 * detail reload, a write to the project) and, given where the panels are
 * shown, when a `workspace.changed` event matches a panel's declared refresh -
 * that panel only, through ONE subscription for them all. 0, an error or an
 * answer that is not a count lays nothing.
 */

import { useEffect, useRef, useState } from 'react'
import { WorkspaceRunAction } from '../../wailsjs/go/main/App'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import type { StripEntry } from '../sections/workspace/VerticalTabBody'
import {
  PANEL_REFRESH_DEBOUNCE_MS, panelRefreshMatches,
  type PanelPlace, type PanelRefresh, type WorkspaceChangedEvent,
} from './usePanelRefresh'

/** A count action's answer: the number and, when the action gives one, its tone. */
export interface PanelCount {
  count: number
  color?: string
}

/** What a count needs of a panel: its id (strip or tab), its count action,
 *  the arguments it is called with and its declared refresh. */
export interface CountedPanel {
  id: string
  count?: string
  args?: Record<string, unknown>
  refresh?: PanelRefresh | null
}

/** countAnswerOf reads a count action's answer ({count: n; color?}, as JSON
 *  text or a value): the number (0 when it is not a positive one) and the
 *  tone when it is a string. */
export function countAnswerOf(value: unknown): PanelCount {
  let v = value
  if (typeof v === 'string') {
    if (v.trim() === '') return { count: 0 }
    try { v = JSON.parse(v) } catch { return { count: 0 } }
  }
  if (!v || typeof v !== 'object' || Array.isArray(v)) return { count: 0 }
  const rec = v as Record<string, unknown>
  const n = rec.count
  const count = typeof n === 'number' && Number.isFinite(n) && n > 0 ? Math.floor(n) : 0
  return typeof rec.color === 'string' && rec.color !== '' ? { count, color: rec.color } : { count }
}

/** countOf is a count action's number alone. */
export function countOf(value: unknown): number {
  return countAnswerOf(value).count
}

/** withCounts lays each panel's count on its strip entry as its badge (its
 *  tone too when the answer gives one); a panel with nothing to count, or a
 *  count of 0, keeps its entry as it is. */
export function withCounts(entries: StripEntry[], counts: Record<string, number | PanelCount | undefined>): StripEntry[] {
  return entries.map((e) => {
    const c = counts[e.id]
    const n = typeof c === 'number' ? c : c?.count ?? 0
    if (!(n > 0)) return e
    const color = typeof c === 'object' ? c?.color : undefined
    return { ...e, badge: String(n), ...(color ? { badgeColor: color } : {}) }
  })
}

type Latest = { universe: string; panels: CountedPanel[] }

/** readCounts calls each panel's count action; an answer lands only while it
 *  is the panel's latest read. */
function readCounts(
  universe: string, panels: CountedPanel[], seq: Record<string, number>,
  set: (f: (prev: Record<string, PanelCount>) => Record<string, PanelCount>) => void,
): void {
  for (const p of panels) {
    if (!p.count) continue
    const mine = (seq[p.id] = (seq[p.id] ?? 0) + 1)
    WorkspaceRunAction(universe, p.count, JSON.stringify(p.args ?? {}))
      .then((r): PanelCount => (r.error ? { count: 0 } : countAnswerOf(r.value)))
      .catch((): PanelCount => ({ count: 0 }))
      .then((c) => { if (seq[p.id] === mine) set((prev) => ({ ...prev, [p.id]: c })) })
  }
}

/** usePanelCounts reads the count of every panel declaring one, by the
 *  panel's id: again when `revision` changes and, with `place`, when an event
 *  matches a panel's refresh (that panel only). */
export function usePanelCounts(
  universeName: string | null | undefined, panels: CountedPanel[], revision: unknown, place?: PanelPlace,
): Record<string, PanelCount> {
  const [counts, setCounts] = useState<Record<string, PanelCount>>({})
  const [nonces, setNonces] = useState<Record<string, number>>({})
  const seq = useRef<Record<string, number>>({})
  const counted = panels.filter((p) => p.count)
  // The cache key: a panel's id, its action and its arguments.
  const key = JSON.stringify(counted.map((p) => [p.id, p.count, p.args ?? {}]))
  const latest = useRef<Latest>({ universe: '', panels: [] })
  latest.current = { universe: universeName ?? '', panels: counted }

  useEffect(() => {
    const { universe, panels: now } = latest.current
    if (!universe || now.length === 0) {
      for (const id of Object.keys(seq.current)) seq.current[id]++
      setCounts({})
      return
    }
    const ids = new Set(now.map((p) => p.id))
    setCounts((prev) => Object.fromEntries(Object.entries(prev).filter(([id]) => ids.has(id))))
    readCounts(universe, now, seq.current, setCounts)
    const mine = seq.current
    return () => { for (const id of Object.keys(mine)) mine[id]++ }
  }, [universeName, key, revision])

  // A refresh's re-read: only the panels whose nonce moved.
  const seen = useRef<Record<string, number>>({})
  useEffect(() => {
    const moved = Object.keys(nonces).filter((id) => nonces[id] !== seen.current[id])
    seen.current = { ...nonces }
    if (moved.length === 0) return
    const { universe, panels: now } = latest.current
    if (!universe) return
    readCounts(universe, now.filter((p) => moved.includes(p.id)), seq.current, setCounts)
  }, [nonces])

  // ONE subscription for every panel declaring a refresh.
  const refreshKey = JSON.stringify(counted
    .filter((p) => p.refresh?.scope && p.refresh.scope !== 'none')
    .map((p) => [p.id, { scope: p.refresh?.scope, types: p.refresh?.types ?? [] }]))
  const { universe = '', object = '', module = '' } = place ?? {}
  const placed = !!place
  useEffect(() => {
    // The Wails runtime is injected by the WebView — absent in a plain browser.
    if (!placed || refreshKey === '[]' || !(window as unknown as { runtime?: unknown }).runtime) return
    const declared = JSON.parse(refreshKey) as [string, PanelRefresh][]
    const here: PanelPlace = { universe, object, module }
    const timers = new Map<string, ReturnType<typeof setTimeout>>()
    const off = EventsOn('workspace.changed', (ev: WorkspaceChangedEvent) => {
      for (const [id, refresh] of declared) {
        if (!panelRefreshMatches(refresh, ev, here)) continue
        const t = timers.get(id)
        if (t) clearTimeout(t)
        timers.set(id, setTimeout(() => {
          timers.delete(id)
          setNonces((n) => ({ ...n, [id]: (n[id] ?? 0) + 1 }))
        }, PANEL_REFRESH_DEBOUNCE_MS))
      }
    })
    return () => {
      off()
      timers.forEach((t) => clearTimeout(t))
    }
  }, [placed, refreshKey, universe, object, module])

  return counts
}

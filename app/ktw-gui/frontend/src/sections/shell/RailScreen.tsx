// SPDX-License-Identifier: Apache-2.0
/**
 * The screen the rail's Inbox opens, as the backend names it:
 * the universe answers a page source to draw - « À faire » - a method whose
 * `{count}` is the rail's badge and the Global strip's declared panels it
 * stands in for. The GUI names no pack's screen: the backend decides.
 * Its screen is `screen:todo`, its badge
 * `rail:todo-count`.
 */

import { useCallback, useEffect, useState } from 'react'
import { Box } from '@mantine/core'
import { WorkspaceRunAction } from '../../../wailsjs/go/main/App'
import { useAppStore } from '../../store'
import { SourcePanel } from '../workspace/RenderDefView'
import { countOf } from '../../hooks/usePanelCounts'
import type { PanelRefresh } from '../../hooks/usePanelRefresh'

/** The method naming what the rail's Inbox opens. */
export const RAIL_SCREEN = 'ktw-agents:waiting/inbox'

/** How often the badge is read again, besides every change of the universe:
 *  what it counts may change with no event. */
export const RAIL_COUNT_EVERY_MS = 30_000

/** How « À faire » follows the universe's changes:
 *  every object event may move its queue - it reads requests, decisions,
 *  goals, features, criteria and verdicts, inbox entries, holding lists,
 *  increments and releases, the very objects the workers write - so its
 *  re-read is paced rather than narrowed: once a burst has been quiet 1.5 s,
 *  and at most every 6 s under a steady stream; a re-read asked while one
 *  runs waits for it (useHostLoad), never restarts it. */
export const RAIL_REFRESH: PanelRefresh = { scope: 'universe', debounceMs: 1_500, maxWaitMs: 6_000 }

/** What the rail's Inbox opens: a page source, its badge's method, the
 *  Global strip's panels it stands in for. Empty: nothing named yet. */
export interface RailScreenSpec {
  page?: string
  count?: string
  replaces?: string[]
}

/** railScreenOf reads the backend's answer; anything else names nothing. */
export function railScreenOf(value: string | undefined | null): RailScreenSpec {
  try {
    const v = JSON.parse(value || '{}') as RailScreenSpec
    if (!v || typeof v !== 'object') return {}
    return {
      ...(typeof v.page === 'string' && v.page ? { page: v.page } : {}),
      ...(typeof v.count === 'string' && v.count ? { count: v.count } : {}),
      ...(Array.isArray(v.replaces) ? { replaces: v.replaces.filter((r) => typeof r === 'string') } : {}),
    }
  } catch {
    return {}
  }
}

/** The rail's badge: the count, « 99+ » past 99, none at 0. */
export function badgeOf(count: number): string {
  if (!Number.isFinite(count) || count <= 0) return ''
  return count > 99 ? '99+' : String(Math.floor(count))
}

/** The rail's width and its buttons' (ActionIcon size md), in pixels. */
export const RAIL_WIDTH = 44
export const RAIL_BUTTON = 28

/** The rail's count bubble: it wraps the button, never sits
 *  inside it - the button clips what overflows it - and its offset pulls it
 *  in so that « 99+ » stays whole inside the rail. */
export const RAIL_BADGE = { size: 16, fontSize: 10, padding: 3, glyph: 6, offset: 6 }

/** badgeWidth is the bubble's width for a label: its glyphs and padding,
 *  never less than a circle. */
export function badgeWidth(label: string): number {
  return Math.max(RAIL_BADGE.size, label.length * RAIL_BADGE.glyph + 2 * RAIL_BADGE.padding)
}

/** badgeRightEdge is where the bubble ends, from the rail's left side: the
 *  button's right corner less the offset, plus half the bubble (Mantine
 *  centres a top-end indicator on that point). Inside the rail when it is
 *  below RAIL_WIDTH less its 1px border. */
export function badgeRightEdge(label: string): number {
  return (RAIL_WIDTH + RAIL_BUTTON) / 2 - RAIL_BADGE.offset + badgeWidth(label) / 2
}

/** Whether a global panel shows on the Global strip: not one the rail's
 *  screen stands in for. */
export function shownOnStrip(panelId: string, spec: RailScreenSpec): boolean {
  const stem = panelId.replace(/^(home|panel):/, '')
  return !(spec.replaces ?? []).includes(stem)
}

/** What the rail's Inbox opens, read once per universe and revision:
 * the rail, the Home tab and the screen share one
 *  call. An answer that failed is not kept - the next to ask reads again. */
const railScreens = new Map<string, Promise<RailScreenSpec>>()
export function railScreenFor(universeName: string, revision: number): Promise<RailScreenSpec> {
  const key = JSON.stringify([universeName, revision])
  let spec = railScreens.get(key)
  if (!spec) {
    // One revision kept per universe: an older one is never asked again.
    for (const k of [...railScreens.keys()]) if ((JSON.parse(k) as [string])[0] === universeName) railScreens.delete(k)
    const forget = () => { if (railScreens.get(key) === spec) railScreens.delete(key) }
    spec = WorkspaceRunAction(universeName, RAIL_SCREEN, '{}').then(
      (r) => { if (r.error) { forget(); return {} } return railScreenOf(r.value) },
      () => { forget(); return {} },
    )
    railScreens.set(key, spec)
  }
  return spec
}

/** useRailScreen reads what the rail's Inbox opens in the universe, again on
 *  every change of it; nothing while it cannot be read. */
export function useRailScreen(universeName: string | null | undefined): RailScreenSpec {
  const revision = useAppStore((s) => s.universeRevision)
  const [spec, setSpec] = useState<RailScreenSpec>({})
  useEffect(() => {
    if (!universeName) { setSpec({}); return }
    let cancelled = false
    railScreenFor(universeName, revision).then((s) => { if (!cancelled) setSpec(s) })
    return () => { cancelled = true }
  }, [universeName, revision])
  return spec
}

/** The count « À faire »'s page answered, by universe, and when:
 * while the page is read, the badge is that count -
 *  the queue is not computed again just for the number. */
const pageCounts = new Map<string, { count: number; at: number }>()
const countListeners = new Set<(universeName: string, count: number) => void>()

/** pageCountOf reads the count a page's answer carries (`data.count`). */
export function pageCountOf(answer: unknown): number | undefined {
  const data = (answer as { data?: unknown } | null)?.data
  const count = (data as { count?: unknown } | null | undefined)?.count
  return typeof count === 'number' && Number.isFinite(count) ? count : undefined
}

/** publishRailCount tells the badge the count the page just answered. */
export function publishRailCount(universeName: string, count: number, now = Date.now()): void {
  pageCounts.set(universeName, { count, at: now })
  countListeners.forEach((l) => l(universeName, count))
}

/** freshRailCount is the page's count when it answered within
 *  RAIL_COUNT_EVERY_MS: the badge need not read its own. */
export function freshRailCount(universeName: string, now = Date.now()): number | undefined {
  const got = pageCounts.get(universeName)
  return got && now - got.at < RAIL_COUNT_EVERY_MS ? got.count : undefined
}

/** useRailCount reads the badge's count from the method named: again on
 *  every change of the universe and every RAIL_COUNT_EVERY_MS - unless « À
 *  faire »'s page answered it meanwhile; 0 with none. */
export function useRailCount(universeName: string | null | undefined, method: string | undefined): number {
  const revision = useAppStore((s) => s.universeRevision)
  const [count, setCount] = useState(0)
  const [tick, setTick] = useState(0)
  useEffect(() => {
    if (!method || !universeName) return
    const timer = setInterval(() => setTick((n) => n + 1), RAIL_COUNT_EVERY_MS)
    const told = (u: string, n: number) => { if (u === universeName) setCount(n) }
    countListeners.add(told)
    return () => { clearInterval(timer); countListeners.delete(told) }
  }, [method, universeName])
  useEffect(() => {
    if (!method || !universeName) { setCount(0); return }
    const fresh = freshRailCount(universeName)
    if (fresh !== undefined) { setCount(fresh); return }
    let cancelled = false
    WorkspaceRunAction(universeName, method, '{}')
      .then((r) => { if (!cancelled) setCount(r.error ? 0 : countOf(r.value)) })
      .catch(() => { if (!cancelled) setCount(0) })
    return () => { cancelled = true }
  }, [method, universeName, revision, tick])
  return count
}

/** RailScreen is what the rail's Inbox tab draws: the page the backend names;
 *  nothing while it is not named. Its page's count is the badge's. */
export function RailScreen() {
  const universeName = useAppStore((s) => s.displayedUniverseName)
  const spec = useRailScreen(universeName)
  const onAnswer = useCallback((answer: unknown) => {
    const n = pageCountOf(answer)
    if (universeName && n !== undefined) publishRailCount(universeName, n)
  }, [universeName])
  if (!spec.page || !universeName) return null
  return (
    <Box data-element="screen:todo" data-guide="todo.screen" p="md" style={{ height: '100%', overflow: 'auto' }}>
      <SourcePanel source={spec.page} args={{}} refresh={RAIL_REFRESH} onAnswer={onAnswer}
        universeName={universeName} objectId="" objectName="" />
    </Box>
  )
}

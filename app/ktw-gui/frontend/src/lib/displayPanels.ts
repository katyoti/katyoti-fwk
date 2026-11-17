// SPDX-License-Identifier: Apache-2.0
// Following a display panel from the GUI: its snapshot read from
// the hub's panel server, then its events as they go out, in
// order and once each - the same rule as the events package's PanelFollower.
// The snapshot is read once the live follow is up, and the
// events arriving meanwhile are held, so none published between the two is
// lost. The snapshot is read again when an event is missing (a gap held a
// second, or PANEL_REORDER_WINDOW events waiting on it), when the panel stays
// quiet before its end (the last events may be the lost ones), and when the
// follow fails; a follow the snapshot shows to have missed events, or one
// that failed, is started again. A follow or a snapshot that
// does not answer within PANEL_CALL_TIMEOUT_MS has failed: the panel is
// lost, its held events are released, and the call is tried again; its late
// answer is dropped.

import type { PanelEvent } from '@ktw/display-lib/ui-builders/page/panel.ts'

import { PanelFollow, PanelPage, PanelSendInput, PanelUnfollow } from '../../wailsjs/go/main/App'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import { panelClient } from '../ktsp'

/** What a panel's snapshot gives a screen: the events to fold, after which
 *  it follows from seq. */
export interface PanelSnapshot {
  seq: number
  events?: PanelEvent[]
}

/** How a screen reaches a panel: its snapshot (undefined when the server
 *  keeps nothing of it yet) and its live events. */
export interface PanelTransport {
  snapshot(id: string): Promise<PanelSnapshot | undefined>
  follow(id: string, onEvent: (e: PanelEvent) => void): Promise<() => void>
}

/** Where a follow stands: live, or lost (its follow or its snapshot failed:
 *  the panel shows its last known state while the follow is tried again). */
export type FollowState = 'live' | 'lost'

/** The timers a follow waits with - the window's by default, a fake in a test. */
export interface PanelClock {
  set(fn: () => void, ms: number): unknown
  clear(handle: unknown): void
}

const windowClock: PanelClock = {
  set: (fn, ms) => setTimeout(fn, ms),
  clear: (h) => clearTimeout(h as ReturnType<typeof setTimeout>),
}

/** How many events may wait on a missing one before the snapshot is read
 *  again at once (the events package's PanelReorderWindow). */
export const PANEL_REORDER_WINDOW = 16

/** How long a missing event is waited for before the snapshot is read again. */
export const PANEL_GAP_WAIT_MS = 1000

/** How long a panel not yet ended may stay quiet before its snapshot is read
 *  again; each quiet read doubles the wait, up to PANEL_QUIET_MAX_MS, and an
 *  event brings it back to the start. */
export const PANEL_QUIET_MS = 3000
export const PANEL_QUIET_MAX_MS = 30000

/** How long a failed follow waits before it is tried again, doubling up to
 *  PANEL_QUIET_MAX_MS. */
export const PANEL_RETRY_MS = 1000

/** How long a follow or a snapshot may go unanswered before it counts as
 *  failed (a hub that cannot be reached). */
export const PANEL_CALL_TIMEOUT_MS = 10000

/** followPanelWith follows panel id over t: onEvent gets the snapshot's
 *  events, then each live event after them, in order, once each, until the
 *  panel completes or closes. Answers the function that stops following. */
export function followPanelWith(
  t: PanelTransport, id: string, onEvent: (e: PanelEvent) => void, onState?: (s: FollowState) => void,
  clock: PanelClock = windowClock,
): () => void {
  let stopped = false
  let ended = false
  let last = 0
  let synced = false
  let resyncing = false
  let again = false
  let following = false
  const held = new Map<number, PanelEvent>()
  let unfollow: (() => void) | null = null
  let gapTimer: unknown = null
  let quietTimer: unknown = null
  let quietWait = PANEL_QUIET_MS
  let retryTimer: unknown = null
  let retryWait = PANEL_RETRY_MS
  const callTimers = new Set<unknown>()

  // within bounds a call by PANEL_CALL_TIMEOUT_MS: past it the call fails, and
  // its late answer goes to late (or is dropped), never to the caller.
  function within<T>(p: Promise<T>, late?: (v: T) => void): Promise<T> {
    return new Promise<T>((resolve, reject) => {
      let done = false
      const h = clock.set(() => {
        callTimers.delete(h)
        if (done) return
        done = true
        reject(new Error('panel call timed out'))
      }, PANEL_CALL_TIMEOUT_MS)
      callTimers.add(h)
      const settle = (fn: () => void) => { callTimers.delete(h); clock.clear(h); fn() }
      p.then(
        (v) => { if (done) { late?.(v); return } done = true; settle(() => resolve(v)) },
        (err) => { if (done) return; done = true; settle(() => reject(err)) },
      )
    })
  }

  const clearTimer = (h: unknown) => { if (h !== null) clock.clear(h) }
  const deliver = (e: PanelEvent) => {
    if (stopped || e.panel !== id || e.seq <= last) return
    last = e.seq
    if (e.kind === 'complete' || e.kind === 'close') ended = true
    onEvent(e)
  }
  // After each delivery: wait a second on a missing event, and re-read a
  // quiet panel that has not ended.
  const watch = () => {
    if (stopped) return
    if (held.size === 0) { clearTimer(gapTimer); gapTimer = null }
    else if (gapTimer === null) gapTimer = clock.set(() => { gapTimer = null; if (held.size > 0) void sync() }, PANEL_GAP_WAIT_MS)
    clearTimer(quietTimer); quietTimer = null
    if (ended) { clearTimer(gapTimer); gapTimer = null; return }
    quietTimer = clock.set(() => {
      quietTimer = null
      quietWait = Math.min(quietWait * 2, PANEL_QUIET_MAX_MS)
      void sync(true)
    }, quietWait)
  }
  const drain = () => {
    for (;;) {
      const next = held.get(last + 1)
      if (!next) break
      held.delete(last + 1)
      deliver(next)
    }
    for (const s of [...held.keys()]) if (s <= last) held.delete(s)
  }
  const sync = async (quiet = false): Promise<void> => {
    if (stopped || ended) return
    if (resyncing) { again = true; return }
    resyncing = true
    const before = last
    let missed = false
    try {
      const snap = await within(t.snapshot(id))
      if (stopped) return
      for (const e of snap?.events ?? []) deliver(e)
      if (snap && snap.seq > last) last = snap.seq
      // A quiet panel whose snapshot is ahead of what came live: its follow
      // missed them - it may be gone (a universe restarted): follow again.
      missed = quiet && last > before && following
      onState?.(following ? 'live' : 'lost')
    } catch {
      if (!stopped) onState?.('lost')
    } finally {
      synced = true
      resyncing = false
    }
    if (stopped) return
    drain()
    watch()
    if (missed) refollow()
    if (again) { again = false; void sync() }
  }
  const take = (e: PanelEvent) => {
    if (stopped || e.panel !== id || e.seq <= last) return
    held.set(e.seq, e)
    if (!synced || resyncing) return
    quietWait = PANEL_QUIET_MS
    drain()
    watch()
    if (held.size > PANEL_REORDER_WINDOW) void sync()
  }
  const follow = () => {
    within(t.follow(id, take), (stop) => stop()).then(
      (stop) => {
        if (stopped) { stop(); return }
        unfollow = stop
        following = true
        retryWait = PANEL_RETRY_MS
        void sync()
      },
      () => {
        if (stopped) return
        following = false
        onState?.('lost')
        void sync()
        retryTimer = clock.set(() => { retryTimer = null; if (!stopped && !ended) follow() }, retryWait)
        retryWait = Math.min(retryWait * 2, PANEL_QUIET_MAX_MS)
      },
    )
  }
  const refollow = () => {
    if (stopped || ended) return
    unfollow?.()
    unfollow = null
    following = false
    follow()
  }

  follow()
  return () => {
    stopped = true
    held.clear()
    clearTimer(gapTimer); clearTimer(quietTimer); clearTimer(retryTimer)
    gapTimer = quietTimer = retryTimer = null
    for (const h of callTimers) clock.clear(h)
    callTimers.clear()
    unfollow?.()
    unfollow = null
  }
}

/** This load of the frontend, named once: the GUI ends the follows a page
 *  before it started (a webview reload) when it names itself. */
const PAGE = `page-${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`
let pageNamed: Promise<void> | null = null

/** namePanelPage names this page to the GUI - at the frontend's start
 *  (main.tsx), so a reload ends the earlier page's follows at once, not at
 *  its first panel follow. Once: a later call answers the same naming; a
 *  failed one is tried again by the next call (a follow's). */
export function namePanelPage(name: (page: string) => Promise<void> = PanelPage): Promise<void> {
  if (!pageNamed) {
    let p: Promise<void>
    try { p = name(PAGE) } catch (err) { p = Promise.reject(err) }
    pageNamed = p.catch(() => { pageNamed = null })
  }
  return pageNamed
}

/** The message key the panel server refuses a panel it keeps nothing of
 *  with (panelapi.MsgUnknownPanel), carried by the KTSP error (KtspError.msg). */
export const MSG_UNKNOWN_PANEL = 'panel.unknownPanel'

/** isUnknownPanel tells the panel server's refusal of a panel it keeps
 *  nothing of yet - by its message key, never by the error's text. */
export function isUnknownPanel(err: unknown): boolean {
  return (err as { msg?: { key?: string } } | null)?.msg?.key === MSG_UNKNOWN_PANEL
}

/** The GUI's transport to a universe's panels: the snapshot from its hub's
 *  panel-service, the live events through the Wails follow (PanelFollow,
 *  "panel.event"). */
export function wailsPanelTransport(universe: string): PanelTransport {
  const client = panelClient(universe)
  return {
    snapshot: async (id) => {
      try {
        const s = await client.snapshot({ panel: id })
        return { seq: s.seq, events: (s.events ?? []) as unknown as PanelEvent[] }
      } catch (err) {
        // A panel the server keeps nothing of yet: follow it live from its start.
        if (isUnknownPanel(err)) return undefined
        throw err
      }
    },
    follow: async (id, onEvent) => {
      // Listen before the follow starts; its follow id says which events are ours.
      let mine: string | null = null
      const early: Array<{ follow: string; event: PanelEvent }> = []
      const off = EventsOn('panel.event', (p: { universe?: string; follow?: string; panel?: string; event?: PanelEvent }) => {
        if (p?.universe !== universe || p?.panel !== id || !p.event || !p.follow) return
        if (mine === null) early.push({ follow: p.follow, event: p.event })
        else if (p.follow === mine) onEvent(p.event)
      })
      try {
        await namePanelPage()
        mine = await PanelFollow(universe, id, PAGE)
      } catch (err) {
        off()
        throw err
      }
      for (const x of early) if (x.follow === mine) onEvent(x.event)
      early.length = 0
      const followID = mine
      return () => { off(); void PanelUnfollow(followID) }
    },
  }
}

/** One input the person sends a panel (internal/core/panel Input). */
export interface PanelInput {
  panel: string
  kind: 'submit' | 'action' | 'message' | 'cancel'
  name?: string
  value?: unknown
  text?: string
  by?: string
}

/** sendPanelInput sends the person's input to the run owning the panel. */
export function sendPanelInput(universe: string, input: PanelInput): Promise<void> {
  return PanelSendInput(universe, JSON.stringify(input))
}

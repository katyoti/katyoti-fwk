// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import type { PanelEvent } from '@ktw/display-lib/ui-builders/page/panel.ts'
import { panelTemplateFetch, panelView, reducePanel } from '@ktw/display-lib/ui-builders/page/panel.ts'

import { followPanelWith, isUnknownPanel, MSG_UNKNOWN_PANEL, namePanelPage, PANEL_CALL_TIMEOUT_MS, PANEL_GAP_WAIT_MS, PANEL_QUIET_MS, PANEL_RETRY_MS, PANEL_REORDER_WINDOW, wailsPanelTransport, type PanelClock, type PanelSnapshot, type PanelTransport } from '../src/lib/displayPanels'
import { KtspError } from '../src/ktsp'
import { answerInput, askFormNode, cancelInput, messageInput, panelControls, panelTabId, progressShare } from '../src/sections/panels/panelInput'
import { openPanelTab } from '../src/sections/panels/openPanelTab'
import { guiHostServices } from '../src/components/compositions/hostServices'
import { useTabsStore } from '../src/store/tabsSlice'

// The GUI draws any display panel by its id - its snapshot read,
// then its events followed live -, sends the person's input back, and never
// disables the rest of the screen.

const ID = 'dp-01a16c25-7963-7ab3-b99f-8c1310f81bae'
const ev = (seq: number, kind: PanelEvent['kind'] = 'progress', more: Partial<PanelEvent> = {}): PanelEvent =>
  ({ panel: ID, seq, kind, ...(kind === 'progress' ? { progress: { current: seq } } : {}), ...more })
const tick = () => new Promise((r) => setTimeout(r, 0))

/** A transport the test drives: the snapshot it answers, the live events it pushes. */
function fakeTransport(snapshots: Array<PanelSnapshot | undefined>) {
  let push: ((e: PanelEvent) => void) | null = null
  let reads = 0
  let stopped = false
  let release: (() => void) | null = null
  const gate = new Promise<void>((r) => { release = r })
  const t: PanelTransport = {
    snapshot: async () => { const s = snapshots[Math.min(reads, snapshots.length - 1)]; reads++; await gate; return s },
    follow: async (_id, onEvent) => { push = onEvent; return () => { stopped = true } },
  }
  return { t, push: (e: PanelEvent) => push?.(e), reads: () => reads, stopped: () => stopped, open: () => release?.() }
}

/** Timers the test moves by hand: advance(ms) fires what falls due, in order. */
function fakeClock() {
  let now = 0
  let next = 0
  const due = new Map<number, { at: number; fn: () => void }>()
  const clock: PanelClock = {
    set: (fn, ms) => { const h = ++next; due.set(h, { at: now + ms, fn }); return h },
    clear: (h) => { due.delete(h as number) },
  }
  const advance = async (ms: number) => {
    const end = now + ms
    for (;;) {
      const first = [...due.entries()].filter(([, d]) => d.at <= end).sort((a, b) => a[1].at - b[1].at)[0]
      if (!first) break
      due.delete(first[0])
      now = first[1].at
      first[1].fn()
      for (let i = 0; i < 5; i++) await tick()
    }
    now = end
    for (let i = 0; i < 5; i++) await tick()
  }
  return { clock, advance, pending: () => due.size }
}

/** A hub the test drives: it stores every event it publishes (the panel
 *  server's sequence) and pushes it live to the follows subscribed - unless
 *  told to lose it on the way. The follow subscribes when the test lets it. */
function fakeHub() {
  const stored: PanelEvent[] = []
  const subs = new Set<(e: PanelEvent) => void>()
  let reads = 0
  let follows = 0
  let letFollow: (() => void) | null = null
  let gated = false
  const t: PanelTransport = {
    snapshot: async () => { reads++; return stored.length ? { seq: stored[stored.length - 1].seq, events: [...stored] } : undefined },
    follow: async (_id, onEvent) => {
      follows++
      if (gated) await new Promise<void>((r) => { letFollow = r })
      subs.add(onEvent)
      return () => { subs.delete(onEvent) }
    },
  }
  const publish = (e: PanelEvent, live = true) => { stored.push(e); if (live) for (const s of subs) s(e) }
  return { t, publish, reads: () => reads, follows: () => follows, gate: () => { gated = true }, resolveFollow: () => letFollow?.() }
}

describe('a display panel followed by its id', () => {
  test('folds its snapshot first, then the live events after it, once each and in order', async () => {
    const f = fakeTransport([{ seq: 2, events: [ev(1, 'open', { open: { title: 'Publication', answer: {} } }), ev(2)] }])
    const got: number[] = []
    const stop = followPanelWith(f.t, ID, (e) => got.push(e.seq), undefined, fakeClock().clock)
    await tick()
    // Live events arrive while the snapshot is read: held, none lost.
    f.push(ev(2)); f.push(ev(4)); f.push(ev(3))
    expect(got).toEqual([])
    f.open()
    await tick(); await tick()
    expect(got).toEqual([1, 2, 3, 4])
    f.push(ev(4)); f.push(ev(5))
    expect(got).toEqual([1, 2, 3, 4, 5])
    stop()
    f.push(ev(6))
    expect(got).toEqual([1, 2, 3, 4, 5])
    expect(f.stopped()).toBe(true)
  })

  test('reads the snapshot again after a gap no event fills', async () => {
    const f = fakeTransport([{ seq: 1, events: [ev(1, 'open', { open: { answer: {} } })] }, { seq: 30, events: [ev(1, 'open', { open: { answer: {} } }), ev(30)] }])
    const got: number[] = []
    followPanelWith(f.t, ID, (e) => got.push(e.seq), undefined, fakeClock().clock)
    f.open()
    await tick(); await tick()
    expect(got).toEqual([1])
    // Event 2 is lost on the way: the next ones wait, then the snapshot is read again.
    for (let s = 3; s <= 3 + PANEL_REORDER_WINDOW; s++) f.push(ev(s))
    await tick(); await tick()
    expect(f.reads()).toBe(2)
    expect(got[got.length - 1]).toBe(30)
  })

  test('a panel the server keeps nothing of yet is followed live from its start', async () => {
    const f = fakeTransport([undefined])
    const got: number[] = []
    followPanelWith(f.t, ID, (e) => got.push(e.seq), undefined, fakeClock().clock)
    f.open()
    await tick(); await tick()
    f.push(ev(1, 'open', { open: { answer: {} } })); f.push(ev(2))
    expect(got).toEqual([1, 2])
  })
})

// A panel that missed live events - its last ones, one in the
// middle, or one published before its follow was up - still reaches the
// hub's stored sequence, and stops reading it once complete.
describe('a panel that missed live events catches up from the stored sequence', () => {
  const open = ev(1, 'open', { open: { answer: {} } })

  test('its last events lost on the way: the quiet panel reads its snapshot again and reaches complete', async () => {
    const h = fakeHub()
    const c = fakeClock()
    const got: number[] = []
    const stop = followPanelWith(h.t, ID, (e) => got.push(e.seq), undefined, c.clock)
    await c.advance(0)
    h.publish(open)
    h.publish(ev(2), false)
    h.publish(ev(3, 'complete'), false)
    expect(got).toEqual([1])
    await c.advance(PANEL_QUIET_MS)
    expect(got).toEqual([1, 2, 3])
    stop()
  })

  test('an event published between the snapshot read and the follow being up is reached', async () => {
    const h = fakeHub()
    const c = fakeClock()
    h.gate()
    h.publish(open)
    const got: number[] = []
    const stop = followPanelWith(h.t, ID, (e) => got.push(e.seq), undefined, c.clock)
    await c.advance(0)
    h.publish(ev(2))
    h.resolveFollow()
    await c.advance(0)
    expect(got).toEqual([1, 2])
    h.publish(ev(3))
    expect(got).toEqual([1, 2, 3])
    stop()
  })

  test('a single event lost in the middle: the held ones wait a second, not the whole window', async () => {
    const h = fakeHub()
    const c = fakeClock()
    const got: number[] = []
    const stop = followPanelWith(h.t, ID, (e) => got.push(e.seq), undefined, c.clock)
    await c.advance(0)
    h.publish(open)
    h.publish(ev(2), false)
    h.publish(ev(3))
    expect(got).toEqual([1])
    await c.advance(PANEL_GAP_WAIT_MS)
    expect(got).toEqual([1, 2, 3])
    stop()
  })

  test('once complete, the snapshot is never read again', async () => {
    const h = fakeHub()
    const c = fakeClock()
    const got: number[] = []
    const stop = followPanelWith(h.t, ID, (e) => got.push(e.seq), undefined, c.clock)
    await c.advance(0)
    h.publish(open)
    h.publish(ev(2, 'complete'))
    const reads = h.reads()
    await c.advance(10 * PANEL_QUIET_MS)
    expect(got).toEqual([1, 2])
    expect(h.reads()).toBe(reads)
    expect(c.pending()).toBe(0)
    stop()
  })

  test('a quiet panel not yet ended is read again, less and less often, until it stops', async () => {
    const h = fakeHub()
    const c = fakeClock()
    const stop = followPanelWith(h.t, ID, () => {}, undefined, c.clock)
    await c.advance(0)
    h.publish(open)
    const r0 = h.reads()
    await c.advance(PANEL_QUIET_MS)
    expect(h.reads()).toBe(r0 + 1)
    await c.advance(PANEL_QUIET_MS)
    expect(h.reads()).toBe(r0 + 1)
    await c.advance(PANEL_QUIET_MS)
    expect(h.reads()).toBe(r0 + 2)
    stop()
    expect(c.pending()).toBe(0)
    await c.advance(10 * PANEL_QUIET_MS)
    expect(h.reads()).toBe(r0 + 2)
  })

  test('a failed follow is lost, then tried again with its snapshot', async () => {
    const h = fakeHub()
    const c = fakeClock()
    let fail = true
    const t: PanelTransport = { snapshot: h.t.snapshot, follow: (id, on) => (fail ? Promise.reject(new Error('no bus')) : h.t.follow(id, on)) }
    const states: string[] = []
    const got: number[] = []
    const stop = followPanelWith(t, ID, (e) => got.push(e.seq), (s) => states.push(s), c.clock)
    h.publish(open)
    await c.advance(0)
    expect(states[0]).toBe('lost')
    fail = false
    h.publish(ev(2), false)
    await c.advance(PANEL_GAP_WAIT_MS)
    expect(states[states.length - 1]).toBe('live')
    expect(got).toEqual([1, 2])
    h.publish(ev(3))
    expect(got).toEqual([1, 2, 3])
    stop()
  })

  test('a quiet read that finds missed events starts the follow again (one the GUI ended)', async () => {
    const h = fakeHub()
    const c = fakeClock()
    const stop = followPanelWith(h.t, ID, () => {}, undefined, c.clock)
    await c.advance(0)
    h.publish(open)
    h.publish(ev(2), false)
    expect(h.follows()).toBe(1)
    await c.advance(PANEL_QUIET_MS)
    expect(h.follows()).toBe(2)
    stop()
  })
})

// A follow or a snapshot that never answers (a hub that cannot be
// reached) counts as failed after PANEL_CALL_TIMEOUT_MS - the panel neither
// stays empty nor freezes, and a late answer is never applied twice.
describe('a panel whose follow or snapshot never answers', () => {
  const open = ev(1, 'open', { open: { answer: {} } })

  test('a follow that never answers: the snapshot is read after the timeout, and the follow tried again', async () => {
    const h = fakeHub()
    const c = fakeClock()
    h.publish(open)
    h.publish(ev(2))
    let hang = true
    let lateFollow: ((stop: () => void) => void) | null = null
    let lateStopped = false
    const t: PanelTransport = {
      snapshot: h.t.snapshot,
      follow: (id, on) => hang
        ? new Promise((r) => { lateFollow = r })
        : h.t.follow(id, on),
    }
    const states: string[] = []
    const got: number[] = []
    const stop = followPanelWith(t, ID, (e) => got.push(e.seq), (s) => states.push(s), c.clock)
    await c.advance(0)
    expect(got).toEqual([])
    hang = false
    await c.advance(PANEL_CALL_TIMEOUT_MS)
    expect(states[0]).toBe('lost')
    expect(got).toEqual([1, 2])
    await c.advance(PANEL_RETRY_MS)
    expect(h.follows()).toBe(1)
    expect(states[states.length - 1]).toBe('live')
    // The first follow answers late: it is ended at once, never used.
    lateFollow!(() => { lateStopped = true })
    await c.advance(0)
    expect(lateStopped).toBe(true)
    h.publish(ev(3))
    expect(got).toEqual([1, 2, 3])
    stop()
    expect(c.pending()).toBe(0)
  })

  test('a snapshot that never answers: the held live events are released after the timeout, and the snapshot read again', async () => {
    const h = fakeHub()
    const c = fakeClock()
    let hang = true
    let lateSnap: ((s: PanelSnapshot | undefined) => void) | null = null
    const t: PanelTransport = {
      snapshot: (id) => hang ? new Promise((r) => { lateSnap = r }) : h.t.snapshot(id),
      follow: h.t.follow,
    }
    const states: string[] = []
    const got: number[] = []
    const stop = followPanelWith(t, ID, (e) => got.push(e.seq), (s) => states.push(s), c.clock)
    await c.advance(0)
    h.publish(open)
    h.publish(ev(2))
    expect(got).toEqual([])
    hang = false
    await c.advance(PANEL_CALL_TIMEOUT_MS)
    expect(states[0]).toBe('lost')
    expect(got).toEqual([1, 2])
    h.publish(ev(3))
    expect(got).toEqual([1, 2, 3])
    // The late snapshot is dropped: nothing applied twice.
    lateSnap!({ seq: 3, events: [open, ev(2), ev(3)] })
    await c.advance(0)
    expect(got).toEqual([1, 2, 3])
    // The snapshot is read again (the quiet read), and the follow is live.
    const reads = h.reads()
    await c.advance(PANEL_QUIET_MS)
    expect(h.reads()).toBe(reads + 1)
    expect(states[states.length - 1]).toBe('live')
    stop()
    expect(c.pending()).toBe(0)
  })
})

describe("the person's input goes back to the run", () => {
  const asking = reducePanel(reducePanel(panelView(ID), ev(1, 'open', { open: { answer: {} } })),
    ev(2, 'ask', { ask: { question: 'Publish?', actions: [{ name: 'publish', label: 'Publier' }, { name: 'skip' }] } }))
  test('an answer is the action chosen among those offered, or the text written', () => {
    expect(answerInput(asking, { action: 'skip' }, 'jdoe')).toEqual({ panel: ID, kind: 'action', name: 'skip', by: 'jdoe' })
    expect(answerInput(asking, {})).toEqual({ panel: ID, kind: 'action', name: 'publish' })
    expect(answerInput(asking, { action: 'nope' })).toBeNull()
    const free = { id: ID, ask: { question: 'Why?' } }
    expect(answerInput(free, { text: '  because  ' })).toEqual({ panel: ID, kind: 'submit', value: 'because' })
    expect(answerInput(free, { text: '   ' })).toBeNull()
  })
  test('a chat turn and a cancel', () => {
    expect(messageInput(ID, ' hello ')).toEqual({ panel: ID, kind: 'message', text: 'hello' })
    expect(messageInput(ID, '')).toBeNull()
    expect(cancelInput(ID, 'jdoe')).toEqual({ panel: ID, kind: 'cancel', by: 'jdoe' })
  })
})

describe('only the panel waits, never the rest of the screen', () => {
  test("the panel's own controls take input by its state, and wait while their input goes out", () => {
    expect(panelControls({ state: 'asking' }, true, false)).toEqual({ answer: true, cancel: true, message: false })
    expect(panelControls({ state: 'asking' }, true, true)).toEqual({ answer: false, cancel: false, message: false })
    expect(panelControls({ state: 'open', type: 'ktw-agents:/panel-type/chat' }, true, false)).toEqual({ answer: false, cancel: true, message: true })
    expect(panelControls({ state: 'complete' }, true, false)).toEqual({ answer: false, cancel: false, message: false })
    expect(panelControls({ state: 'asking' }, false, false).answer).toBe(false)
  })
  test('DisplayPanel lays nothing over the screen', async () => {
    const src = await Bun.file(new URL('../src/sections/panels/DisplayPanel.tsx', import.meta.url)).text()
    expect(src).not.toMatch(/LoadingOverlay|<Overlay|pointerEvents|Modal/)
    expect(src).toMatch(/data-element=\{`panel:\$\{panelId\}`\}/)
  })
  test('the bar says how far the work is', () => {
    expect(progressShare({ current: 2, total: 8 })).toBe(25)
    expect(progressShare({ current: 3 })).toBe(0)
  })
})

describe('where a panel shows', () => {
  test('in a tab of its own, one per panel, by its element address', () => {
    openPanelTab('u-panels', ID, 'Publication')
    openPanelTab('u-panels', ID, 'Publication')
    const tabs = useTabsStore.getState().getForUniverse('u-panels')
    const mine = tabs.openTabs.filter((t) => t.id === panelTabId(ID))
    expect(mine.length).toBe(1)
    expect(mine[0].kind).toBe('panel')
    expect(mine[0].payload).toEqual({ panelId: ID, title: 'Publication' })
    expect(tabs.activeTabId).toBe(`panel:${ID}`)
  })
  test('inline where its action ran: the host follows and opens panels', () => {
    const host = guiHostServices('u', 'standard', 'fr', 'jdoe')
    expect(typeof host.followPanel).toBe('function')
    expect(typeof host.openPanel).toBe('function')
  })
  test('in the run drawer', async () => {
    const src = await Bun.file(new URL('../src/sections/scheduler/RunDrawer.tsx', import.meta.url)).text()
    expect(src).not.toMatch(/useDisplaySwitch/)
    expect(src).toMatch(/list\(\{ owner: run\.runId \}\)/)
    expect(src).toMatch(/<DisplayPanel [^>]*variant="inline"/)
  })
})

// A question carrying a form draws that form, prefilled with the
// question's value, and sends the form's object back as the answer.
describe('a panel question carrying a form', () => {
  const asked = (ask: NonNullable<PanelEvent['ask']>) => reducePanel(reducePanel(panelView(ID), ev(1, 'open', { open: { type: 'x' } })), ev(2, 'ask', { ask }))

  test('draws the form over its data type, prefilled with its value, its submit the answer', () => {
    const v = asked({ question: 'Which release?', form: { uri: 'ktw-agents:/data-type/release-pick' }, value: { name: '2026.11' } })
    const f = askFormNode(v.ask) as unknown as Record<string, unknown>
    expect(f).toMatchObject({ kind: 'form', schema: 'ktw-agents:/data-type/release-pick', data: { name: '2026.11' }, mode: 'edit', submit: { uri: 'panel:answer' } })
    const inline = askFormNode(asked({ form: { root: 'R', types: {} } }).ask) as unknown as Record<string, unknown>
    expect(inline.schema).toEqual({ root: 'R', types: {} })
  })

  test('sends the object of the form as the input value', () => {
    const v = asked({ form: { uri: 'ktw-agents:/data-type/release-pick' } })
    expect(answerInput(v, { value: { name: '2026.12' } }, 'jdoe')).toEqual({ panel: ID, kind: 'submit', value: { name: '2026.12' }, by: 'jdoe' })
  })

  test('no form when the question offers choices or carries none: choices or a text, as before', () => {
    expect(askFormNode(asked({ form: { uri: 'u' }, actions: [{ name: 'keep' }] }).ask)).toBeUndefined()
    expect(askFormNode(asked({ question: 'Why?' }).ask)).toBeUndefined()
    expect(askFormNode(undefined)).toBeUndefined()
  })
})

// The page names itself when the frontend loads, not at its first
// panel follow - a reload ends the follows the page before it started at once.
describe('the page named at the frontend start', () => {
  test('main.tsx names the page before the first render', async () => {
    const main = await Bun.file(new URL('../src/main.tsx', import.meta.url)).text()
    const at = main.indexOf('namePanelPage()')
    expect(at).toBeGreaterThan(-1)
    expect(at).toBeLessThan(main.indexOf('createRoot('))
  })

  test('names it once; a failed naming is tried again by the next call', async () => {
    const named: string[] = []
    let fail = true
    const name = async (page: string) => { named.push(page); if (fail) throw new Error('no GUI') }
    await namePanelPage(name)
    expect(named.length).toBe(1)
    expect(named[0]).toMatch(/^page-/)
    fail = false
    await namePanelPage(name)
    await namePanelPage(name)
    expect(named).toEqual([named[0], named[0]])
  })
})

// The panel server refuses a panel it keeps nothing of by its
// message key; the GUI reads that key - never the error's text - to follow
// the panel live from its start.
describe('a panel the server keeps nothing of yet', () => {
  const answering = (res: unknown) => {
    ;(globalThis as unknown as { window: unknown }).window = {
      go: { main: { App: { CallKTSPMethod: async () => res } } },
    }
  }
  const SLOT = 'ktw-hub:panel-service/snapshot'

  test('its snapshot answers nothing when the refusal carries the unknown-panel key', async () => {
    answering({ error: 'panel server: unknown panel: ' + ID, errorMsg: { key: MSG_UNKNOWN_PANEL, params: { panel: ID } } })
    expect(await wailsPanelTransport('u').snapshot(ID)).toBeUndefined()
  })

  test('a refusal without the key fails, whatever its text says', async () => {
    answering({ error: 'panel server: unknown panel: ' + ID })
    await expect(wailsPanelTransport('u').snapshot(ID)).rejects.toThrow('unknown panel')
    expect(isUnknownPanel(new Error('panel server: unknown panel'))).toBe(false)
    expect(isUnknownPanel(new KtspError(SLOT, 'x', { key: MSG_UNKNOWN_PANEL }))).toBe(true)
    expect(isUnknownPanel(new KtspError(SLOT, 'x', { key: 'cwm.objectNotFound' }))).toBe(false)
    expect(isUnknownPanel(null)).toBe(false)
  })

  test('the key is the one the panel server sends (panelapi.MsgUnknownPanel)', async () => {
    const api = await Bun.file(new URL('../../../../internal/extensions/panelserver/api/api.go', import.meta.url)).text()
    expect(api).toContain(`const MsgUnknownPanel = "${MSG_UNKNOWN_PANEL}"`)
    const src = await Bun.file(new URL('../src/lib/displayPanels.ts', import.meta.url)).text()
    expect(src).not.toContain("'unknown panel'")
  })
})

// The panel draws the template its latest open
// names; the ops taken meanwhile are kept by the view, behind its seq guard.
describe('a panel whose open names a page template', () => {
  const named = (seq: number, uri: string): PanelEvent => ({ panel: ID, seq, kind: 'open', open: { answer: { tree: { uri, hash: 'h' } as never } } })
  test('a second open draws its new template, not the page of the first', async () => {
    const fetch = async (s: { uri: string }) => ({ kind: 'text', id: s.uri })
    let v = reducePanel(panelView(ID), named(1, 'ktw-agents:/page-template/one'))
    v = (await panelTemplateFetch(v, fetch))!(v)
    expect((v.page?.tree as { id: string }).id).toBe('ktw-agents:/page-template/one')
    v = reducePanel(v, named(2, 'ktw-agents:/page-template/two'))
    const step = panelTemplateFetch(v, fetch)
    expect(step).toBeDefined()
    v = (await step!)(v)
    expect((v.page?.tree as { id: string }).id).toBe('ktw-agents:/page-template/two')
  })
  test('DisplayPanel keeps no buffer of its own: the view holds the ops', async () => {
    const src = await Bun.file(new URL('../src/sections/panels/DisplayPanel.tsx', import.meta.url)).text()
    expect(src).not.toContain('waiting.current')
    expect(src).toContain('panelTemplateFetch(view,')
  })
})

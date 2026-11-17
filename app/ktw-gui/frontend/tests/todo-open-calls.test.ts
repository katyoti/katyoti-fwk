// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'

import { refreshPacer } from '../src/hooks/usePanelRefresh'
import { leftLoads, rereadQueue } from '../src/hooks/useHostLoad'
import { loadPart } from '../src/components/compositions/hostServices'
import { needsResolve } from '../src/lib/specCache'
import { askedInput } from '../src/sections/workspace/RenderDefView'
import {
  freshRailCount, pageCountOf, publishRailCount, RAIL_COUNT_EVERY_MS, RAIL_REFRESH, RAIL_SCREEN, railScreenFor,
} from '../src/sections/shell/RailScreen'

// « À faire » opened slowly and re-read endlessly. One
// open now costs one read of what the rail opens (shared by the rail, the Home
// tab and the screen), one read of the page, no resolve of its tree (its forms
// come resolved) and no count of its own (the badge takes the page's); a burst
// of changes re-reads it once, and a re-read never restarts one in flight.

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms))

// The page's answer as the server gives it: forms with their schema inline,
// their ui block already laid (variant), and its count in data.
const pageAnswer = {
  kind: 'page', title: 'À faire', data: { count: 7 },
  dslTree: { kind: 'vbox', children: [{ kind: 'card', children: [
    { kind: 'form', mode: 'edit', variant: 'default', schema: { name: 'hold', type: 'object', extensions: [{ type: 'ui-form', name: 'default' }] }, submit: { uri: 'ktw-agents:requests/hold' } },
  ] }] },
}

function mockApp(calls: Record<string, number>, fail = false) {
  ;(globalThis as unknown as { window: unknown }).window = {
    go: { main: { App: {
      WorkspaceRunAction: async (_u: string, uri: string) => {
        calls[uri] = (calls[uri] ?? 0) + 1
        if (fail) return { error: 'core starting' }
        if (uri === RAIL_SCREEN) return { value: JSON.stringify({ page: 'ktw-agents:views/person-queue-page', count: 'ktw-agents:views/person-queue-count' }) }
        return { value: JSON.stringify(pageAnswer) }
      },
    } } },
  }
}

describe('one open of « À faire »', () => {
  test('what the rail opens is read once per universe and revision, shared by every reader', async () => {
    const calls: Record<string, number> = {}
    mockApp(calls)
    const [a, b, c] = await Promise.all([railScreenFor('open-u', 0), railScreenFor('open-u', 0), railScreenFor('open-u', 0)])
    expect(calls[RAIL_SCREEN]).toBe(1)
    expect(a.page).toBe('ktw-agents:views/person-queue-page')
    expect(b).toEqual(a)
    expect(c).toEqual(a)
    await railScreenFor('open-u', 1)
    expect(calls[RAIL_SCREEN]).toBe(2)
    await railScreenFor('other-u', 1)
    expect(calls[RAIL_SCREEN]).toBe(3)
  })

  test('a failed read is not kept: the next reader asks again', async () => {
    const calls: Record<string, number> = {}
    mockApp(calls, true)
    expect(await railScreenFor('fail-u', 0)).toEqual({})
    mockApp(calls)
    expect((await railScreenFor('fail-u', 0)).page).toBe('ktw-agents:views/person-queue-page')
    expect(calls[RAIL_SCREEN]).toBe(2)
  })

  test("the page's tree needs no second round trip; a tree with a ui block still does", () => {
    expect(needsResolve(pageAnswer.dslTree)).toBe(false)
    const unresolved = { kind: 'vbox', children: [{ kind: 'form', schema: { name: 'hold' }, ui: { actions: [] } }] }
    expect(needsResolve(unresolved)).toBe(true)
  })

  test("the badge takes the page's count while it is fresh, and reads its own after", () => {
    expect(pageCountOf(pageAnswer)).toBe(7)
    expect(pageCountOf({ kind: 'page' })).toBeUndefined()
    const now = 1_000_000
    publishRailCount('count-u', 7, now)
    expect(freshRailCount('count-u', now + 1_000)).toBe(7)
    expect(freshRailCount('count-u', now + RAIL_COUNT_EVERY_MS + 1)).toBeUndefined()
    expect(freshRailCount('never-u', now)).toBeUndefined()
  })

  test('counted end to end: 1 inbox read, 1 page read, 0 resolve, 0 count', async () => {
    const calls: Record<string, number> = {}
    mockApp(calls)
    // The rail, the Home tab and the screen ask what the rail opens.
    const specs = await Promise.all([railScreenFor('e2e-u', 0), railScreenFor('e2e-u', 0), railScreenFor('e2e-u', 0)])
    const w = (globalThis as unknown as { window: { go: { main: { App: { WorkspaceRunAction: (u: string, uri: string, i: string) => Promise<{ value: string }> } } } } }).window
    const answer = JSON.parse((await w.go.main.App.WorkspaceRunAction('e2e-u', specs[2].page!, '{}')).value)
    if (needsResolve(answer.dslTree)) await w.go.main.App.WorkspaceRunAction('e2e-u', 'ktw-agents:object-pane/resolve-panel', '{}')
    publishRailCount('e2e-u', pageCountOf(answer)!)
    if (freshRailCount('e2e-u') === undefined) await w.go.main.App.WorkspaceRunAction('e2e-u', specs[0].count!, '{}')
    expect(calls).toEqual({ [RAIL_SCREEN]: 1, 'ktw-agents:views/person-queue-page': 1 })
  })
})

describe('a refresh burst re-reads « À faire » once', () => {
  test('the screen paces its re-read: seconds of quiet, a longest wait', () => {
    expect(RAIL_REFRESH.scope).toBe('universe')
    expect(RAIL_REFRESH.debounceMs).toBeGreaterThanOrEqual(1_000)
    expect(RAIL_REFRESH.maxWaitMs).toBeGreaterThan(RAIL_REFRESH.debounceMs!)
  })

  test('ten events in a burst: one re-read', async () => {
    let fired = 0
    const pacer = refreshPacer(() => fired++, 30, 200)
    for (let i = 0; i < 10; i++) { pacer.event(); await sleep(3) }
    await sleep(60)
    expect(fired).toBe(1)
    pacer.stop()
  })

  test('a steady stream re-reads at most once per longest wait', async () => {
    let fired = 0
    const pacer = refreshPacer(() => fired++, 40, 100)
    const end = Date.now() + 350
    while (Date.now() < end) { pacer.event(); await sleep(10) }
    pacer.stop()
    expect(fired).toBeGreaterThanOrEqual(2)
    expect(fired).toBeLessThanOrEqual(4)
  })

  test('a re-read asked while one runs is held, run once after it, never restarting it', () => {
    let started = 0
    const q = rereadQueue(() => started++)
    q.ask()
    expect(started).toBe(1)
    q.began()
    for (let i = 0; i < 5; i++) q.ask()
    expect(started).toBe(1)
    expect(q.holding()).toBe(true)
    q.ended()
    expect(started).toBe(2)
    q.began()
    q.ended()
    expect(started).toBe(2)
    // A read given up holds nothing.
    q.began(); q.ask(); q.dropped()
    expect(q.holding()).toBe(false)
  })
})

// A click in « À faire » (a tile, a kind chip) is one
// read of the page with the query the click carries: no resolve of its tree,
// no new ask of what the rail opens, no count of its own. The server answers
// a click from the queue the open just read (queryReuse), so the click costs
// the page's drawing, not the queue computed again.
describe('one click in « À faire »', () => {
  test('counted end to end: 1 page read with the query, 0 resolve, 0 rail screen, 0 count', async () => {
    const calls: Record<string, number> = {}
    const inputs: string[] = []
    mockApp(calls)
    const w = (globalThis as unknown as { window: { go: { main: { App: { WorkspaceRunAction: (u: string, uri: string, i: string) => Promise<{ value: string }> } } } } }).window
    const real = w.go.main.App.WorkspaceRunAction
    w.go.main.App.WorkspaceRunAction = (u, uri, i) => { if (uri.endsWith('views/person-queue-page')) inputs.push(i); return real(u, uri, i) }
    // The open.
    const [spec] = await Promise.all([railScreenFor('click-u', 0), railScreenFor('click-u', 0)])
    const open = JSON.stringify({ language: 'fr', level: 'essential' })
    const first = JSON.parse((await w.go.main.App.WorkspaceRunAction('click-u', spec.page!, open)).value)
    publishRailCount('click-u', pageCountOf(first)!)
    for (const k of Object.keys(calls)) delete calls[k]
    // A click: the tile's query over the same input.
    const spec2 = await railScreenFor('click-u', 0)
    const click = JSON.stringify({ action: 'answer', language: 'fr', level: 'essential' })
    const answer = JSON.parse((await w.go.main.App.WorkspaceRunAction('click-u', spec2.page!, click)).value)
    if (needsResolve(answer.dslTree)) await w.go.main.App.WorkspaceRunAction('click-u', 'ktw-agents:object-pane/resolve-panel', '{}')
    publishRailCount('click-u', pageCountOf(answer)!)
    if (freshRailCount('click-u') === undefined) await w.go.main.App.WorkspaceRunAction('click-u', spec2.count!, '{}')
    expect(calls).toEqual({ 'ktw-agents:views/person-queue-page': 1 })
    // The click keeps the open's language: the server's queue is reused for
    // the same universe and language only.
    expect(JSON.parse(inputs[1]).language).toBe(JSON.parse(inputs[0]).language)
    expect(JSON.parse(inputs[1]).action).toBe('answer')
  })
})

// « À faire » answers a click from its kept read; a
// re-read (a change, an action) must never be answered from it.
describe('a re-read of « À faire » asks it computed again', () => {
  test('the same input again goes fresh; a click, the first read go as they are', () => {
    const all = JSON.stringify({ language: 'fr', level: 'essential' })
    const tile = JSON.stringify({ action: 'answer', language: 'fr', level: 'essential' })
    expect(askedInput(all, undefined)).toBe(all)
    expect(JSON.parse(askedInput(all, all))).toEqual({ language: 'fr', level: 'essential', fresh: true })
    expect(askedInput(tile, all)).toBe(tile)
    expect(JSON.parse(askedInput(tile, tile)).fresh).toBe(true)
    expect(askedInput('not json', 'not json')).toBe('not json')
  })
})

// The open read the page twice at once (the walk's
// reads 1 ms apart, on the GUI the walks run - React's development double
// effect): the screen's effect ended and ran again with the same ask, gave its
// read up - cancelled on the server - and asked it again, `fresh`, so the
// server computed the queue twice for one open. A view's effect run again in
// the same turn with the same ask now joins the read in flight.
describe('« À faire » mounted twice at once reads its page once', () => {
  const rig = () => {
    const started: string[] = []
    const cancelled: string[] = []
    let n = 0
    const start = (signal: AbortSignal) => {
      const id = `call-${++n}`
      started.push(id)
      signal.addEventListener('abort', () => cancelled.push(id), { once: true })
      return new Promise<string>((go) => setTimeout(() => go(id), 20))
    }
    return { started, cancelled, start }
  }

  test('the same ask again within the turn: one call, joined, never cancelled', async () => {
    const { started, cancelled, start } = rig()
    const left = leftLoads()
    const ask = JSON.stringify(['twice-u|page', 'page|{}', 0, 0])
    left.begin(ask)
    const first = new AbortController()
    const a = loadPart('twice-u', 'page', start, first.signal).catch((e: Error) => e.name)
    left.leave(ask, first)
    left.begin(ask)
    const second = new AbortController()
    const b = loadPart('twice-u', 'page', start, second.signal)
    await sleep(5)
    expect(started).toEqual(['call-1'])
    expect(await a).toBe('AbortError')
    expect(await b).toBe('call-1')
    expect(cancelled).toEqual([])
  })

  test('another ask (a re-read, another part) gives the left read up at once', async () => {
    const { started, cancelled, start } = rig()
    const left = leftLoads()
    const first = new AbortController()
    const a = loadPart('reread-u', 'page', start, first.signal).catch((e: Error) => e.name)
    await sleep(1)
    left.leave('round 0', first)
    left.begin('round 1')
    const b = loadPart('reread-u', 'page', start, new AbortController().signal)
    expect(await a).toBe('AbortError')
    expect(await b).toBe('call-2')
    expect(started).toEqual(['call-1', 'call-2'])
    expect(cancelled).toEqual(['call-1'])
  })

  test('a read left and never asked again is given up once the turn ends', async () => {
    const { cancelled, start } = rig()
    const left = leftLoads()
    const first = new AbortController()
    const a = loadPart('gone-u', 'page', start, first.signal).catch((e: Error) => e.name)
    await sleep(1)
    left.leave('only', first)
    expect(cancelled).toEqual([])
    expect(await a).toBe('AbortError')
    await sleep(5)
    expect(cancelled).toEqual(['call-1'])
  })
})

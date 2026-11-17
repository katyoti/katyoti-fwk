// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'

import { guiHostServices, isReadTimeout, sectionGate, sharedLoads } from '../src/components/compositions/hostServices'
import { runPart } from '../src/hooks/useHostLoad'

// The GUI gives up a section's load made obsolete - the section
// scrolled away, a newer refresh, the page closed - and cancels it on the
// server through its call id; a load already in flight is never started
// twice: a second section asking for it shares it.
describe('a section load made obsolete', () => {
  const rig = () => {
    const calls: Array<{ uri: string; id: string }> = []
    const cancelled: string[] = []
    const pending = new Map<string, () => void>()
    ;(globalThis as unknown as { window: unknown }).window = {
      go: { main: { App: {
        WorkspaceRunActionAs: (_u: string, uri: string, _in: string, id: string) => {
          calls.push({ uri, id })
          return new Promise((resolve) => pending.set(id, () => resolve({ value: JSON.stringify({ kind: 'text', content: uri }) })))
        },
        WorkspaceCancelCall: async (id: string) => { cancelled.push(id) },
        // A schema-free answer is not resolved through the object pane.
        WorkspaceRunAction: async () => ({ value: 'null' }),
      } } },
    }
    return { calls, cancelled, finish: (id: string) => pending.get(id)?.() }
  }
  const tick = () => new Promise((r) => setTimeout(r, 5))

  test('is cancelled on the server by its call id', async () => {
    const { calls, cancelled } = rig()
    const host = guiHostServices('u')
    const quit = new AbortController()
    const load = host.loadSection!({ method: 'ktw-x:views/lane', args: { s: 1 } }, quit.signal)
    await tick()
    expect(calls.length).toBe(1)
    quit.abort()
    await expect(load).rejects.toThrow()
    expect(cancelled).toEqual([calls[0].id])
  })

  test('a load in flight is shared, not started twice; it is cancelled only once no section waits on it', async () => {
    const { calls, cancelled, finish } = rig()
    const host = guiHostServices('u')
    const a = new AbortController()
    const first = host.loadSection!({ method: 'ktw-x:views/lane', args: { s: 1 } }, a.signal)
    const second = host.loadSection!({ method: 'ktw-x:views/lane', args: { s: 1 } })
    const other = host.loadSection!({ method: 'ktw-x:views/lane', args: { s: 2 } })
    await tick()
    expect(calls.map((c) => c.uri)).toEqual(['ktw-x:views/lane', 'ktw-x:views/lane'])
    a.abort()
    await expect(first).rejects.toThrow()
    expect(cancelled).toEqual([])
    finish(calls[0].id)
    finish(calls[1].id)
    expect(await second).toEqual({ kind: 'text', content: 'ktw-x:views/lane' })
    expect(await other).toEqual({ kind: 'text', content: 'ktw-x:views/lane' })
  })

  test('the shared loads forget a load once it ends', async () => {
    const loads = sharedLoads()
    await loads.load('k', undefined, async () => 1)
    expect(loads.size()).toBe(0)
    let started = 0
    await Promise.all([loads.load('k', undefined, async () => { started++; return 2 }), loads.load('k', undefined, async () => { started++; return 3 })])
    expect(started).toBe(1)
  })

  // A read the hub never answers ends with a readable error -
  // never placeholder rows for minutes - its call given up, every section
  // waiting on it told, and a retry starts it anew.
  test('a load unanswered past its deadline fails every waiter with a readable error, gives its call up, and is started anew when asked again', async () => {
    const loads = sharedLoads(20)
    let gaveUp = false
    const hung = (s: AbortSignal) => new Promise<number>(() => { s.addEventListener('abort', () => { gaveUp = true }) })
    const first = loads.load('k', undefined, hung)
    const second = loads.load('k', undefined, hung)
    for (const p of [first, second]) {
      const e = await p.then(() => null, (err: unknown) => err)
      expect(isReadTimeout(e)).toBe(true)
      expect((e as Error).message).toContain('did not answer')
    }
    expect(gaveUp).toBe(true)
    expect(loads.size()).toBe(0)
    expect(await loads.load('k', undefined, async () => 7)).toBe(7)
  })

  // The deadline counts from the read's turn at the gate, so a
  // read queued behind slow but answering reads is not failed for its wait.
  test('a read queued at the gate past the deadline still answers: its clock starts with its turn', async () => {
    const loads = sharedLoads(100, sectionGate(1))
    const slow = (v: number) => () => new Promise<number>((r) => setTimeout(() => r(v), 70))
    const first = loads.load('a', undefined, slow(1))
    const queued = loads.load('b', undefined, slow(2))
    expect(await first).toBe(1)
    expect(await queued.then((v) => v, (e: unknown) => e)).toBe(2)
  })

  // A read given up leaves its turn at once, even while a step
  // after its call never answers - the next read runs.
  test('a section read given up while a step after its call hangs frees its turn', async () => {
    const calls: string[] = []
    ;(globalThis as unknown as { window: unknown }).window = {
      go: { main: { App: {
        WorkspaceRunActionAs: async (_u: string, uri: string) => {
          calls.push(uri)
          // A tree named by {uri, hash}: its template is asked, never answered.
          return { value: JSON.stringify(uri.endsWith('/hang') ? { tree: { uri: 'ktw-x:t', hash: 'h' } } : { kind: 'text', content: uri }) }
        },
        WorkspaceCancelCall: async () => {},
        CallKTSPMethod: () => new Promise(() => {}),
        WorkspaceRunAction: async () => ({ value: 'null' }),
      } } },
    }
    const host = guiHostServices('u-hang-step')
    const quits = [1, 2, 3, 4].map(() => new AbortController())
    const hung = quits.map((q, i) => host.loadSection!({ method: 'ktw-x:views/hang', args: { i } }, q.signal))
    await tick()
    for (const q of quits) q.abort()
    for (const h of hung) await expect(h).rejects.toThrow()
    const next = host.loadSection!({ method: 'ktw-x:views/next' })
    const got = await Promise.race([next, new Promise((r) => setTimeout(() => r('still queued'), 200))])
    expect(got).toEqual({ kind: 'text', content: 'ktw-x:views/next' })
  })

  test('a part whose read timed out is not asked again by the cold-start backoff', async () => {
    let tries = 0
    const err = await runPart({
      universe: 'u-timeout', key: 'k', retryErrors: true,
      load: async () => { tries++; throw Object.assign(new Error('late'), { name: 'ReadTimeout' }) },
    }, new AbortController().signal).then(() => null, (e: unknown) => e)
    expect(isReadTimeout(err)).toBe(true)
    expect(tries).toBe(1)
  })
})

// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'

import { guiHostServices, sectionGate, SECTION_LOADS_AT_ONCE } from '../src/components/compositions/hostServices'

// The GUI loads a page's sections through one host service - each
// load run as a region's source is (its args, the person's language and
// level), a few at a time, a schema-bound answer resolved through the object
// pane, a load its section gave up before its turn never sent.
describe('the loadSection host service', () => {
  const answering = (answers: Record<string, unknown>, gate?: Promise<void>) => {
    const calls: Array<{ uri: string; input: unknown }> = []
    const run = async (_u: string, uri: string, input: string) => {
      calls.push({ uri, input: JSON.parse(input) })
      if (gate) await gate
      return { value: JSON.stringify(answers[uri] ?? null) }
    }
    // A section's load runs under a call id it may give up.
    ;(globalThis as unknown as { window: unknown }).window = {
      go: { main: { App: { WorkspaceRunAction: run, WorkspaceRunActionAs: run, WorkspaceCancelCall: async () => {} } } },
    }
    return calls
  }

  test('runs the load with its args, the language and the level, and answers the one answer', async () => {
    const calls = answering({ 'ktw-x:views/lane': { tree: { kind: 'text', content: 'lane' }, data: { n: 1 } } })
    const got = await guiHostServices('u', 'standard', 'fr').loadSection!({ method: 'ktw-x:views/lane', args: { stream: 's1' } })
    expect(got).toEqual({ tree: { kind: 'text', content: 'lane' }, data: { n: 1 } })
    expect(calls).toEqual([{ uri: 'ktw-x:views/lane', input: { stream: 's1', language: 'fr', level: 'standard' } }])
  })

  test('resolves a schema-bound tree through the object pane', async () => {
    const calls = answering({
      'a:b/c': { dslTree: { kind: 'table', schema: 'ktw-x:/data-type/row' }, data: [] },
      'ktw-agents:object-pane/resolve-panel': { def: { dslTree: { kind: 'table', schema: { root: 'Row' } } } },
    })
    const host = guiHostServices('u')
    expect(await host.loadSection!({ method: 'a:b/c' })).toEqual({ dslTree: { kind: 'table', schema: { root: 'Row' } }, data: [] })
    expect(calls.map((c) => c.uri)).toEqual(['a:b/c', 'ktw-agents:object-pane/resolve-panel'])
  })

  test(`runs at most ${SECTION_LOADS_AT_ONCE} loads at once, the others in turn; one given up while it waits is never sent`, async () => {
    let open!: () => void
    const gate = new Promise<void>((go) => { open = go })
    const calls = answering({}, gate)
    const host = guiHostServices('u')
    const quit = new AbortController()
    const loads = Array.from({ length: 6 }, (_, i) => host.loadSection!({ method: `s:x/${i}` }, i === 5 ? quit.signal : undefined))
    await new Promise((r) => setTimeout(r, 10))
    expect(calls.length).toBe(SECTION_LOADS_AT_ONCE)
    quit.abort()
    open()
    const settled = await Promise.allSettled(loads)
    expect(calls.map((c) => c.uri)).toEqual(['s:x/0', 's:x/1', 's:x/2', 's:x/3', 's:x/4'])
    expect(settled[5].status).toBe('rejected')
  })

  test('the gate hands each ended turn to the next one waiting', async () => {
    const run = sectionGate(1)
    const order: number[] = []
    let inFlight = 0
    let most = 0
    const job = (n: number) => async () => { inFlight++; most = Math.max(most, inFlight); await new Promise((r) => setTimeout(r, 2)); order.push(n); inFlight-- }
    await Promise.all([run(job(1)), run(job(2)), run(job(3))])
    expect(order).toEqual([1, 2, 3])
    expect(most).toBe(1)
  })
})

// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'

import { guiHostServices } from '../src/components/compositions/hostServices'

// The GUI provides the display library's callMethod host service -
// a form field or a picker calls a contributed method by its URI, as the
// person, through the one KTSP binding, and gets its result.
describe('the callMethod host service', () => {
  test('calls the method by its URI with its input and answers its result', async () => {
    const calls: Array<{ universe: string; slot: string; input: string }> = []
    ;(globalThis as unknown as { window: unknown }).window = {
      go: { main: { App: { CallKTSPMethod: async (universe: string, slot: string, input: string) => {
        calls.push({ universe, slot, input })
        return { response: { items: [{ id: 'page-1' }] } }
      } } } },
    }
    const host = guiHostServices('archifactory')
    expect(typeof host.callMethod).toBe('function')
    const out = await host.callMethod!('ktw-acme:confluence/search', { query: 'acme' })
    expect(out).toEqual({ items: [{ id: 'page-1' }] })
    expect(calls).toEqual([{ universe: 'archifactory', slot: 'ktw-acme:confluence/search', input: '{"query":"acme"}' }])
  })
})

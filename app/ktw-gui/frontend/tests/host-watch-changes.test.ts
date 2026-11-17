// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'

import { guiHostServices } from '../src/components/compositions/hostServices'

// A section's own refresh - the GUI's object changes
// (workspace.changed) matched as a panel's refresh is, against the object or
// module the section names; a burst calls the section once; stopping ends it.
describe('the watchChanges host service', () => {
  const bus = () => {
    const handlers: Array<(ev: unknown) => void> = []
    ;(globalThis as unknown as { window: unknown }).window = {
      runtime: { EventsOnMultiple: (_name: string, cb: (ev: unknown) => void) => { handlers.push(cb); return () => { handlers.splice(handlers.indexOf(cb), 1) } } },
    }
    return { emit: (ev: unknown) => handlers.forEach((h) => h(ev)), count: () => handlers.length }
  }
  const wait = (ms: number) => new Promise((r) => setTimeout(r, ms))

  test('a burst of matching changes calls the section once; another module or type never', async () => {
    const b = bus()
    let calls = 0
    const stop = guiHostServices('u').watchChanges!({ scope: 'module', types: ['release'], module: 'billing' }, () => { calls++ })
    b.emit({ universe: 'u', object: 'billing.RL-1', module: 'billing', type: 'release' })
    b.emit({ universe: 'u', object: 'billing.RL-2', module: 'billing', type: 'release' })
    b.emit({ universe: 'u', object: 'shipping.F-1', module: 'shipping', type: 'release' })
    b.emit({ universe: 'u', object: 'billing.F-1', module: 'billing', type: 'feature' })
    b.emit({ universe: 'other', object: 'billing.RL-3', module: 'billing', type: 'release' })
    await wait(400)
    expect(calls).toBe(1)
    stop()
    expect(b.count()).toBe(0)
  })

  test('self reads the object the section names', async () => {
    const b = bus()
    let calls = 0
    const stop = guiHostServices('u').watchChanges!({ scope: 'self', object: 'm.IN-1' }, () => { calls++ })
    b.emit({ universe: 'u', object: 'm.IN-2', module: 'm' })
    await wait(400)
    expect(calls).toBe(0)
    b.emit({ universe: 'u', object: 'm.IN-1', module: 'm' })
    await wait(400)
    expect(calls).toBe(1)
    stop()
  })
})

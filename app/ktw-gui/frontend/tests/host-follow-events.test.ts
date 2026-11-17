// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'

import { guiHostServices } from '../src/components/compositions/hostServices'

// A progress list follows the source.sync events
// raised on sources through the host's followEvents - the raised events the
// GUI is told of (workspace.changed, action event) of the named type in its
// universe, each payload read as JSON with the object it was raised on;
// stopping ends it. Offered behind its switch only.
describe('the followEvents host service', () => {
  const bus = () => {
    const handlers: Array<(ev: unknown) => void> = []
    ;(globalThis as unknown as { window: unknown }).window = {
      runtime: { EventsOnMultiple: (_name: string, cb: (ev: unknown) => void) => { handlers.push(cb); return () => { handlers.splice(handlers.indexOf(cb), 1) } } },
    }
    return { emit: (ev: unknown) => handlers.forEach((h) => h(ev)), count: () => handlers.length }
  }
  const raised = (over: Record<string, unknown>) => ({
    universe: 'u', subject: 's', object: 'm.SRC-1', module: 'm', action: 'event', event: 'source.sync',
    payload: '{"source":"m.SRC-1","phase":"items","read":2,"total":5}', ...over,
  })

  test('calls onEvent for the named event only and unsubscribes on cleanup', () => {
    const b = bus()
    const got: Array<[unknown, string | undefined]> = []
    const stop = guiHostServices('u', 'expert', undefined, undefined, true).followEvents!('source.sync', (p, o) => { got.push([p, o]) })
    expect(b.count()).toBe(1)
    b.emit(raised({}))
    b.emit(raised({ event: 'code.changed' }))
    b.emit(raised({ action: 'updated', event: undefined }))
    b.emit(raised({ universe: 'other' }))
    b.emit(raised({ payload: '{"source":"m.SRC-1","pha…' }))
    expect(got).toEqual([[{ source: 'm.SRC-1', phase: 'items', read: 2, total: 5 }, 'm.SRC-1']])
    stop()
    expect(b.count()).toBe(0)
    b.emit(raised({}))
    expect(got.length).toBe(1)
  })

  test('is not offered while its switch is off', () => {
    bus()
    expect(guiHostServices('u').followEvents).toBeUndefined()
  })
})

// SPDX-License-Identifier: Apache-2.0
import { beforeEach, describe, expect, test } from 'bun:test'

import { dispatcherFor } from '../src/ktsp'
import { busyCount, countBusyCalls } from '../src/ktsp/busy'

// The GUI counts its own request/response calls in flight in
// window.__ktwBusy, so browse/wait-idle can tell a screen still loading
// through the Wails IPC (invisible as network) from a settled one.

/** A call the test answers when it chooses. */
function pending() {
  let resolve!: (v: unknown) => void
  let reject!: (e: unknown) => void
  const promise = new Promise((res, rej) => { resolve = res; reject = rej })
  return { promise, resolve, reject }
}

describe('window.__ktwBusy', () => {
  beforeEach(() => {
    delete (globalThis as { __ktwBusy?: number }).__ktwBusy
  })

  test('counts a call while it is in flight, through the KTSP client too, and drops it once answered or failed', async () => {
    const a = pending()
    const b = pending()
    const queue = [a, b]
    const app: Record<string, unknown> = {
      CallKTSPMethod: () => queue.shift()!.promise,
    }
    countBusyCalls(app)
    expect(busyCount()).toBe(0)
    ;(globalThis as unknown as { window: unknown }).window = { go: { main: { App: app } } }
    const call = dispatcherFor('u')

    const first = call('svc:x/y', {})
    const second = call('svc:x/z', {}).catch(() => 'failed')
    expect(busyCount()).toBe(2)
    a.resolve({ response: 1 })
    expect(await first).toBe(1)
    expect(busyCount()).toBe(1)
    b.reject(new Error('down'))
    expect(await second).toBe('failed')
    expect(busyCount()).toBe(0)
  })

  test('a call that throws at once is not left counted', () => {
    const app: Record<string, unknown> = {
      WorkspaceRunAction: () => { throw new Error('no bridge') },
    }
    countBusyCalls(app)
    expect(() => (app.WorkspaceRunAction as () => unknown)()).toThrow('no bridge')
    expect(busyCount()).toBe(0)
  })

  test('a dialog waiting on the person and the frontend log are not counted', () => {
    const open = pending()
    const app: Record<string, unknown> = {
      PickProjectsFolder: () => open.promise,
      FeedbackPickFolder: () => open.promise,
      LogFromFrontend: () => open.promise,
      // SaveDocument waits on the native save dialog.
      SaveDocument: () => open.promise,
    }
    countBusyCalls(app)
    ;(app.PickProjectsFolder as () => unknown)()
    ;(app.FeedbackPickFolder as () => unknown)()
    ;(app.LogFromFrontend as () => unknown)()
    ;(app.SaveDocument as () => unknown)()
    expect(busyCount()).toBe(0)
  })

  test('counting the bindings twice counts each call once', async () => {
    const a = pending()
    const app: Record<string, unknown> = { WorkspaceObjectDetailFetch: () => a.promise }
    countBusyCalls(app)
    countBusyCalls(app)
    const p = (app.WorkspaceObjectDetailFetch as () => Promise<unknown>)()
    expect(busyCount()).toBe(1)
    a.resolve('ok')
    await p
    expect(busyCount()).toBe(0)
  })
})

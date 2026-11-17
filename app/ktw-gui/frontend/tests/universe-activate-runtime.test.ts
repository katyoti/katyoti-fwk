// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'

import { useAppStore } from '../src/store'
import { saysHubKeepsRunning } from '../src/sections/shell/universeRuntime'

// The GUI lists the universes before it activates its launch
// universe, so the list carries hubKeepsRunning:false then. Activating it
// re-reads the runtime facts: when the hub was found running (started outside
// the GUI), the notice universe.hub-keeps-running shows.
function mockApp(hubKeepsRunning: boolean) {
  ;(globalThis as unknown as { window: unknown }).window = {
    go: { main: { App: {
      UniverseActivate: async () => ({ running: true }),
      UniverseListActive: async () => ({
        active: [{ name: 'scn-smoke', homePath: '/x', startedAtUnix: 1, hubKeepsRunning }],
      }),
    } } },
    localStorage: { setItem: () => {} },
  }
}

const smoke = () => useAppStore.getState().universes.find((u) => u.name === 'scn-smoke')

describe('activating a universe refreshes its runtime facts', () => {
  test('a hub found running shows the hub-keeps-running notice', async () => {
    mockApp(true)
    useAppStore.getState().setUniverses([{ name: 'scn-smoke', busRunning: false, hubKeepsRunning: false } as never])
    await useAppStore.getState().activateUniverse('scn-smoke')
    expect(smoke()?.busRunning).toBe(true)
    expect(saysHubKeepsRunning(smoke())).toBe(true)
  })
  test('a hub the GUI started shows no notice', async () => {
    mockApp(false)
    useAppStore.getState().setUniverses([{ name: 'scn-smoke', busRunning: false, hubKeepsRunning: false } as never])
    await useAppStore.getState().activateUniverse('scn-smoke')
    expect(smoke()?.busRunning).toBe(true)
    expect(saysHubKeepsRunning(smoke())).toBe(false)
  })
})

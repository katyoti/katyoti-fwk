// SPDX-License-Identifier: Apache-2.0
import { afterEach, describe, expect, test } from 'bun:test'

import { cancelCall, installClient, type CallPanel, type CallPanelBridge } from '../src/ktsp/inPanel'

// The welcome screen's and the Marketplace's installs
// are control-plane calls of the admin universe. Each call is lent a panel the
// GUI opens first - the screen hears it before the call starts and draws it -
// then the call answers.

type Sent = { binding: string; args: unknown[] }

function fakeBridge(answer: { response?: unknown; error?: string }, sent: Sent[]): CallPanelBridge {
  return {
    OpenCallPanel: async (...args) => { sent.push({ binding: 'OpenCallPanel', args }); return 'dp-1' },
    CallKTSPMethodInPanel: async (...args) => { sent.push({ binding: 'CallKTSPMethodInPanel', args }); return answer },
    WorkspaceCancelCall: async (...args) => { sent.push({ binding: 'WorkspaceCancelCall', args }) },
  }
}

const g = globalThis as unknown as { window?: unknown }
afterEach(() => { delete g.window })

describe('an install', () => {
  test('opens its panel, tells the screen, then calls lent that panel', async () => {
    const sent: Sent[] = []
    const panels: CallPanel[] = []
    const mkt = installClient((p) => panels.push(p), (_slot, input) => `Install ${(input as { name: string }).name}`,
      fakeBridge({ response: { name: 'ktw-acme', version: '1.2.0' } }, sent))

    const out = await mkt.install({ name: 'ktw-acme' })

    expect(out).toEqual({ name: 'ktw-acme', version: '1.2.0' })
    expect(sent.map((s) => s.binding)).toEqual(['OpenCallPanel', 'CallKTSPMethodInPanel'])
    const [universe, title, callId] = sent[0].args as string[]
    expect(universe).toBe('admin')
    expect(title).toBe('Install ktw-acme')
    expect(sent[1].args).toEqual(['admin', 'ktw-univers:marketplace/install', '{"name":"ktw-acme"}', callId])
    expect(panels).toEqual([{ universe: 'admin', panelId: 'dp-1', callId, title: 'Install ktw-acme' }])
  })

  test('a failed install rejects', async () => {
    const mkt = installClient(() => {}, undefined, fakeBridge({ error: 'download refused' }, []))
    await expect(mkt.installPrereqs({ universes: ['/u'], programs: ['git'] })).rejects.toThrow('download refused')
  })

  test('the panel is named by its method when the screen gives no title', async () => {
    const sent: Sent[] = []
    await installClient(() => {}, undefined, fakeBridge({ response: [] }, sent)).doctor({ universeHome: '/u' })
    expect(sent[0].args[1]).toBe('doctor')
  })

  test('Cancel gives the call up by its id', () => {
    const sent: Sent[] = []
    cancelCall({ universe: 'admin', panelId: 'dp-1', callId: 'mkt-1', title: 'x' }, fakeBridge({}, sent))
    expect(sent).toEqual([{ binding: 'WorkspaceCancelCall', args: ['mkt-1'] }])
  })
})

describe('an install with no direct call left', () => {
  test('a GUI without the bindings rejects it: no direct call is made', async () => {
    const direct: unknown[] = []
    g.window = { go: { main: { App: { CallKTSPMethod: async (...args: unknown[]) => { direct.push(args); return { response: {} } } } } } }
    await expect(installClient(() => {}, undefined, undefined).install({ name: 'x' })).rejects.toThrow('Wails bridge unavailable')
    expect(direct).toEqual([])
  })
})

// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'

import { guiHostServices } from '../src/components/compositions/hostServices'
import { answerInput, cancelInput, inputBy } from '../src/sections/panels/panelInput'
import { panelAnswer, panelCancel } from '@ktw/display-lib/ui-builders/page/panel.ts'

// A display panel embedded inline (the page DSL's panel node,
// where an action ran) answers its question and cancels its work through the
// host - the GUI's host services send the input on the panel's input subject
// (PanelSendInput) as the person at the screen.
const ID = 'dp-00000000-0000-4000-8000-000000000001'

describe('the sendPanelInput host service', () => {
  test('is offered', () => {
    expect(typeof guiHostServices('u', 'standard', 'fr', 'jdoe').sendPanelInput).toBe('function')
  })
  test('sends the input to the run owning the panel, as the person at the screen', async () => {
    const sent: Array<{ universe: string; input: unknown }> = []
    ;(globalThis as unknown as { window: unknown }).window = {
      go: { main: { App: { PanelSendInput: async (universe: string, input: string) => { sent.push({ universe, input: JSON.parse(input) }) } } } },
    }
    const host = guiHostServices('archifactory', 'standard', 'fr', 'jdoe')
    await host.sendPanelInput!({ panel: ID, kind: 'action', name: 'publish' })
    await host.sendPanelInput!({ panel: ID, kind: 'cancel', by: 'other' })
    expect(sent).toEqual([
      { universe: 'archifactory', input: { panel: ID, kind: 'action', name: 'publish', by: 'jdoe' } },
      { universe: 'archifactory', input: { panel: ID, kind: 'cancel', by: 'other' } },
    ])
  })
  test('names no one when no person is known', () => {
    expect(inputBy({ panel: ID, kind: 'cancel' })).toEqual({ panel: ID, kind: 'cancel' })
  })
})

describe('DisplayPanel builds its input as the inline panel node does', () => {
  test("its answer and cancel are the display library's", () => {
    const asking = { id: ID, ask: { question: 'Publier ?', actions: [{ name: 'publish' }, { name: 'skip' }] } }
    expect(answerInput(asking, { action: 'skip' }, 'jdoe')).toEqual(panelAnswer(asking, { action: 'skip' }, 'jdoe'))
    const free = { id: ID, ask: { question: 'Pourquoi ?' } }
    expect(answerInput(free, { text: ' parce que ' })).toEqual(panelAnswer(free, { text: ' parce que ' }))
    expect(cancelInput(ID, 'jdoe')).toEqual(panelCancel(ID, 'jdoe'))
  })
})

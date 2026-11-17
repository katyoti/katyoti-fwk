// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { landsOnOutput, runFailureReason } from '../src/sections/agent/runFailure'
import type { MsgItem } from '../src/types/messages'

// A task that fails before it starts (agent.modelKeyMissing) said
// its reason only in a Système message the person did not see: the run
// landed on its Result tab (« Aucune sortie structurée ») under a banner
// saying only « Tâche en échec ». The banner says the reason, and a run that
// failed with a reason stays on its chat, where the reason is.
const keyMissing = 'La tâche n’a pas pu appeler le modèle de langage foundry-sonnet : sa clé n’est pas installée.'
const chat: MsgItem[] = [
  { kind: 'text', id: 'u1', sender: 'user', text: 'go', done: true },
  { kind: 'system', id: 's1', sender: 'system', level: 'info', text: 'starting' },
  { kind: 'system', id: 's2', sender: 'system', level: 'error', text: keyMissing },
  { kind: 'system', id: 's3', sender: 'system', level: 'warn', text: 'late warning' },
]

describe('a failed run says why at every display level', () => {
  test('its reason is its last error message', () => {
    expect(runFailureReason(chat)).toBe(keyMissing)
    expect(runFailureReason(chat.filter((m) => m.id !== 's2'))).toBeUndefined()
    expect(runFailureReason([])).toBeUndefined()
  })

  test('a run that failed with a reason stays on its chat; others land on their result', () => {
    expect(landsOnOutput('error', keyMissing)).toBe(false)
    expect(landsOnOutput('error', undefined)).toBe(true)
    expect(landsOnOutput('done', undefined)).toBe(true)
  })

  test('the failure banner is given the reason, not a debug tab', () => {
    const src = readFileSync(new URL('../src/sections/agent/ChatPanel.tsx', import.meta.url), 'utf8')
    expect(src).toContain("sessionStatus === 'error' ? runFailureReason(messages)")
    expect(src).toContain('<SessionDoneCard status={sessionStatus} reason={failureReason}')
    expect(src).toContain('data-testid="run-failure-reason"')
    expect(src).toContain('if (landsOnOutput(sessionStatus, failureReason))')
  })
})

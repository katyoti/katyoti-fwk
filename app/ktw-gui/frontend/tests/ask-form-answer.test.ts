// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'

import { askFormAnswer, askFormRetry } from '../src/sections/agent/messages/askFormAnswer'

// An ask-user-data form is the question of a display panel,
// its artifact id the panel's id - the chat card answers that panel's input.
describe('the chat form card answers its question panel', () => {
  const panel = 'dp-01928c6e-4f1a-7b2c-9d3e-0a1b2c3d4e5f'

  test('a submit is the panel input carrying the form value', () => {
    expect(askFormAnswer(panel, 'submit', { name: 'Atlas' }, 'jdoe')).toEqual({ panel, kind: 'submit', value: { name: 'Atlas' }, by: 'jdoe' })
  })

  test('a cancel is the panel cancel', () => {
    expect(askFormAnswer(panel, 'cancel')).toEqual({ panel, kind: 'cancel' })
  })

  test('an artifact that is no panel gets no panel input (the ui_event path)', () => {
    expect(askFormAnswer('ask-user-data:run-1', 'submit', {})).toBeNull()
    expect(askFormAnswer(undefined, 'cancel')).toBeNull()
  })

  test('a refused input shows the form again, holding what was submitted', () => {
    expect(askFormRetry('submit', { name: 'Atlas' }, { name: '' })).toEqual({ submitted: undefined, cancelled: false, value: { name: 'Atlas' } })
    expect(askFormRetry('cancel', undefined, { name: 'x' })).toEqual({ submitted: undefined, cancelled: false, value: { name: 'x' } })
  })
})

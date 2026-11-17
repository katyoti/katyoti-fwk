// SPDX-License-Identifier: Apache-2.0
// An element's action with `disabledWhen` shows disabled, with the reason the
// element gives, while the element sets that field (the display library's
// page DSL): « À faire »'s Accept stays disabled until the hand tests before
// it passed, then runs.

import { describe, expect, test } from 'bun:test'

import { actionDisabled, actionShown } from '../../../ktw-display-lib/src/ui-builders/page/nodes/interactive'

describe('an action with disabledWhen', () => {
  const accept = { uri: 'ktw-agents:inbox/answer', disabledWhen: 'waits_on' }

  test('is disabled with the element reason while the field is set', () => {
    expect(actionDisabled(accept, { id: 'v:a.RQ-1', waits_on: 'Test by hand first: “It opens”' })).toBe('Test by hand first: “It opens”')
    expect(actionShown(accept, { id: 'v:a.RQ-1', waits_on: 'x' })).toBe(true)
  })

  test('runs once the field is gone', () => {
    expect(actionDisabled(accept, { id: 'v:a.RQ-1' })).toBeUndefined()
    expect(actionDisabled(accept, { id: 'v:a.RQ-1', waits_on: '' })).toBeUndefined()
    expect(actionDisabled({ uri: 'ktw-agents:inbox/answer' }, { waits_on: 'x' })).toBeUndefined()
  })

  test('a reason that is not text still disables it', () => {
    expect(actionDisabled(accept, { waits_on: true })).toBe('Not available yet')
  })
})

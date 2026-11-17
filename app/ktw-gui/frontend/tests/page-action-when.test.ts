// SPDX-License-Identifier: Apache-2.0
// An element's action with `when` shows only on the elements that set that
// field (the display library's page DSL): the planning deck's retry shows on a
// failed step, not on an end refused or a question awaited.

import { describe, expect, test } from 'bun:test'

import { actionShown } from '../../../ktw-display-lib/src/ui-builders/page/nodes/interactive'

describe('an action with when', () => {
  test('shows only where its field is set', () => {
    const retry = { uri: 'ktw-agents:work/retry', when: 'todo' }
    expect(actionShown(retry, { id: 'a.RQ-1', todo: 'a.TD-1' })).toBe(true)
    expect(actionShown(retry, { id: 'a.RQ-2' })).toBe(false)
    expect(actionShown(retry, { id: 'a.RQ-3', todo: '' })).toBe(false)
    expect(actionShown({ uri: 'ktw-agents:plans/finish' }, { id: 'a.RQ-2' })).toBe(true)
  })
})

// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { actionOutcome, outcomeAfterRead } from '../src/components/compositions/actionOutcome'

// The alert an action leaves above a panel stays across the action's own
// re-read and goes at the next one: it used to stay until closed by hand.
describe("an action's alert", () => {
  test("survives the action's own re-read, goes at the next", () => {
    const o = actionOutcome(true, 'Close: done', 1)
    const afterOwn = outcomeAfterRead(o)
    expect(afterOwn?.text).toBe('Close: done')
    expect(outcomeAfterRead(afterOwn)).toBeNull()
  })
  test('a panel that does not re-read itself drops it at the first read', () => {
    expect(outcomeAfterRead(actionOutcome(false, 'refused', 0))).toBeNull()
  })
  test('no alert, nothing to clear', () => {
    expect(outcomeAfterRead(null)).toBeNull()
  })
})

// A result carrying its inverse keeps it on its alert, which
// offers « Undo », for as long as the alert stays.
describe("an action's alert with an undo", () => {
  const undo = { label: 'Undo', uri: 'ktw-agents:requests/assess', args: { id: 'IN-1', priority: 'P2' } }
  test('carries the undo across its own re-read', () => {
    const o = actionOutcome(true, 'IN-1 moved to P0', 1, undo)
    expect(o.undo).toEqual(undo)
    expect(outcomeAfterRead(o)?.undo).toEqual(undo)
  })
  test('an alert without one has none', () => {
    expect(actionOutcome(true, 'Close: done', 1).undo).toBeUndefined()
  })
})

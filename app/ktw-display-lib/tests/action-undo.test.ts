// SPDX-License-Identifier: Apache-2.0
// An action's result may carry its inverse: `undo`, an action
// naming a method with its args written out, which the host offers with the
// result's message as « Undo ». Only a method takes a change back, and only a
// success has one.

import { describe, expect, test } from 'bun:test'

import { parseActionResult } from '../src/ui-builders/page/ui-page.interactive.ts'

const undo = { label: 'Undo', uri: 'ktw-agents:requests/assess', args: { id: 'IN-1', priority: 'P2' } }

describe('a result carrying its inverse', () => {
  test('keeps its undo, from the JSON text or the value', () => {
    expect(parseActionResult({ ok: true, message: 'IN-1 moved to P0', undo })?.undo).toEqual(undo)
    expect(parseActionResult(JSON.stringify({ message: 'moved', undo }))?.undo).toEqual(undo)
    // An undo alone follows the convention: a success.
    expect(parseActionResult({ undo })).toEqual({ ok: true, message: undefined, open: undefined, undo })
  })

  test('an undo naming no method, or on a refusal, is left out', () => {
    expect(parseActionResult({ ok: true, undo: { label: 'Undo', select: { key: 'k', value: 'v' } } })?.undo).toBeUndefined()
    expect(parseActionResult({ ok: true, undo: 'no' })?.undo).toBeUndefined()
    expect(parseActionResult({ ok: false, message: 'refused', undo })?.undo).toBeUndefined()
  })

  test('a result without one is as before', () => {
    expect(parseActionResult({ ok: true, message: 'Filed' })).toEqual({ ok: true, message: 'Filed', open: undefined })
  })
})

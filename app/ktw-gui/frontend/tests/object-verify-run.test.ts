// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'

import { checkText, verifyRunOutcome } from '../src/sections/workspace/objectVerify'

// The object pane's Verify runs through the action surface: its
// run's panel drawn under the check, each check reported there.

describe('what a verify run shows', () => {
  test('a long verification: its run\'s panel', () => {
    expect(verifyRunOutcome({ taskId: 'run-1', panel: 'dp-1' })).toEqual({ panel: 'dp-1' })
  })

  test('an outcome answered at once, or a run that could not start: the text the old path shows', () => {
    expect(verifyRunOutcome({ check: { status: 'fail', desc: '2 run, 1 passed, 1 failed' } })).toEqual({ text: 'fail — 2 run, 1 passed, 1 failed' })
    expect(verifyRunOutcome({ check: { status: 'harness-error', error: 'verify failed: no hub' } })).toEqual({ text: 'harness-error (verify failed: no hub)' })
  })

  test('neither a run nor an outcome is no verdict', () => {
    expect(checkText({ status: 'pass' })).toBe('pass')
    const o = verifyRunOutcome({})
    expect('text' in o && o.text.startsWith('harness-error')).toBe(true)
  })
})

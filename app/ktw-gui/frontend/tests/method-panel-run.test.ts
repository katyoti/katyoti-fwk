// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'

import { refreshAfterRun, runOutcome, type ActionSpec } from '../src/sections/workspace/methodAction'

// The capability method-panel runs an action through the action
// surface and draws, in place, the display panel a long method's run reports
// into - generic: whatever the method, its sidecar or its panel's type.

const status: ActionSpec = { label: 'Status', method: 'ktw-acme:confluence/status', args: { folder: '${node.module_dir}/confluence' } }

describe('a method-panel action', () => {
  test('always runs through the action surface: no direct call path is left', () => {
    const src = readFileSync(join(import.meta.dir, '../src/sections/workspace/MethodPanel.tsx'), 'utf8')
    expect(src).not.toContain('useDisplaySwitch')
    expect(src).not.toContain('actionRoute')
    expect(src).not.toMatch(/\bcall\(act\)/)
    expect(src).toContain('runMethod(act)')
  })
})

describe('what a run through the action surface shows', () => {
  test('a long method: its run\'s display panel, drawn in place', () => {
    expect(runOutcome({ taskId: 'run-1', panel: 'dp-1' })).toEqual({ panel: 'dp-1' })
  })

  test('a method that is not long: its value, as the text report', () => {
    expect(runOutcome({ result: '{"report":"3 pages"}' })).toEqual({ result: { result: '{"report":"3 pages"}' } })
  })

  test('an error: the error, never a panel', () => {
    expect(runOutcome({ error: 'refused', panel: 'dp-1' })).toEqual({ result: { error: 'refused' } })
  })
})

describe('once the run panel of a long method ends', () => {
  test('the state is read again when it completed ok and the action asks it, as a direct call does', () => {
    expect(refreshAfterRun({ refreshStateOnSuccess: true }, 'ok')).toBe(true)
  })

  test('never when it failed or was cancelled, nor when the action does not ask it', () => {
    expect(refreshAfterRun({ refreshStateOnSuccess: true }, 'failed')).toBe(false)
    expect(refreshAfterRun({ refreshStateOnSuccess: true }, 'cancelled')).toBe(false)
    expect(refreshAfterRun({}, 'ok')).toBe(false)
  })
})

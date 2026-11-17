// SPDX-License-Identifier: Apache-2.0
// A stacked gauge's segment lists what it counts on hover, and a stepper draws
// as a row of dots: a dot's state is the step's own, else
// done before the current step, running at it, todo after; a list bound to a
// stepper gives its steps, to a gauge its segments.

import { describe, expect, test } from 'bun:test'

import { stepDotLook, stepDotStatus } from '../src/ui-builders/page/nodes/stepper.tsx'
import { statusChip } from '../src/ui-builders/fields/_planning/palette.ts'
import { applyDataBinding } from '../src/ui-builders/page/nodes/data-binding.ts'
import type { UIPageNode } from '../src/ui-builders/page/index.ts'

const steps = ['intake', 'implement', 'auto-review', 'verify'].map((id) => ({ id, label: id }))

describe('step dots', () => {
  test('done before the current step, running at it, todo after', () => {
    const node = { steps, current: 'auto-review' }
    expect(steps.map((_, i) => stepDotStatus(node, i))).toEqual(['done', 'done', 'running', 'todo'])
  })
  test("a step's own state wins; a completed workflow is all done; no current is all todo", () => {
    const own = { steps: steps.map((s, i) => (i === 1 ? { ...s, status: 'blocked' } : s)), current: 'implement' }
    expect(stepDotStatus(own, 1)).toBe('blocked')
    expect(steps.map((_, i) => stepDotStatus({ steps, completed: true }, i))).toEqual(['done', 'done', 'done', 'done'])
    expect(stepDotStatus({ steps }, 0)).toBe('todo')
  })
})

describe("a dot's look", () => {
  test('running pulses orange; on-you is violet; failed and blocked red; todo empty', () => {
    expect(stepDotLook('running')).toEqual({ background: statusChip('running').fg, border: statusChip('running').fg, pulse: true })
    expect(stepDotLook('on-you').background).toBe(statusChip('planned').fg)
    expect(stepDotLook('on-you').pulse).toBe(false)
    expect(stepDotLook('failed').background).toBe(statusChip('blocked').fg)
    expect(stepDotLook('blocked').background).toBe(statusChip('blocked').fg)
    expect(stepDotLook('done').background).toBe(statusChip('done').fg)
    expect(stepDotLook('todo').background).toBe('transparent')
  })
})

describe('a list bound to a stepper or a gauge', () => {
  const data = { row: { steps: [{ id: 'intake', label: 'intake', status: 'done' }], step: 'intake', bars: [{ value: 2, label: 'running', items: [{ id: 'RQ-1' }, { id: 'RQ-2' }] }] } }
  test('gives the stepper its steps, a value its current step', () => {
    const bound = applyDataBinding({ kind: 'stepper', variant: 'dots', steps: [], dataField: '$.row.steps' } as UIPageNode, data) as { steps: unknown[] }
    expect(bound.steps).toEqual(data.row.steps)
    const current = applyDataBinding({ kind: 'stepper', steps, dataField: '$.row.step' } as UIPageNode, data) as { current: string }
    expect(current.current).toBe('intake')
  })
  test('gives the gauge its segments with their items', () => {
    const bound = applyDataBinding({ kind: 'gauge', label: 'goal', dataField: '$.row.bars' } as UIPageNode, data) as { segments: { items: unknown[] }[] }
    expect(bound.segments[0].items).toEqual([{ id: 'RQ-1' }, { id: 'RQ-2' }])
  })
})

// SPDX-License-Identifier: Apache-2.0
// The timeline layout's steps (ui-builders/deck/timeline.ts): `**Step** — line`
// shows the step, then the line — the separator is the layout's to drop.

import { describe, expect, it } from 'bun:test'
import { timelineSteps } from '@ktw/display-lib/ui-builders/deck/timeline.ts'

const SEPARATORS = ['—', '–', ':', '-']

describe('timelineSteps', () => {
  for (let n = 2; n <= 8; n++) {
    it(`drops the separator of each of ${n} steps, whatever it is`, () => {
      const md = Array.from({ length: n }, (_, i) => `${i + 1}. **Step ${i + 1}** ${SEPARATORS[i % 4]} line ${i + 1}`).join('\n')
      const got = timelineSteps(md).split('\n')
      expect(got).toHaveLength(n)
      got.forEach((l, i) => expect(l).toBe(`${i + 1}. **Step ${i + 1}** line ${i + 1}`))
    })
  }
  it('keeps a dash inside the line and a step with no separator', () => {
    expect(timelineSteps('1. **Go-live** — run — and support\n2. **Build** the work'))
      .toBe('1. **Go-live** run — and support\n2. **Build** the work')
  })
  it('takes bullets and __bold__ too', () => {
    expect(timelineSteps('- __Idea__ — a need')).toBe('- __Idea__ a need')
  })
})

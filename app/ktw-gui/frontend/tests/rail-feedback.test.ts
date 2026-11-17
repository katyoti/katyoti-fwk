// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'fs'

// The Feedback entry is on the left rail, beside Debug, Inbox and
// Marketplace, so it is reachable whatever panel is open; the tab bar has none;
// no universe can hide it.
const src = (f: string) => readFileSync(new URL(`../src/sections/shell/${f}`, import.meta.url), 'utf8')

describe('the Feedback entry', () => {
  test('is on the rail and opens the Feedback form', () => {
    const rail = src('IconRibbon.tsx')
    const block = rail.slice(rail.indexOf('data-guide="ribbon.feedback"') - 400, rail.indexOf('data-guide="ribbon.feedback"') + 50)
    expect(block).toContain('onClick={() => void openFeedback()}')
    expect(block).toContain("guideT('feedback.button')")
  })
  test('is not in the tab bar any more', () => {
    expect(src('MainPaneTabs.tsx')).not.toContain('openFeedback')
  })
  test('cannot be hidden by a universe', () => {
    const rail = src('IconRibbon.tsx')
    expect(rail).not.toContain("hidden('rail:feedback')")
    const at = rail.indexOf('data-guide="ribbon.feedback"')
    const before = rail.slice(Math.max(0, at - 700), at)
    expect(before).not.toContain("hidden('feedback')")
  })
})

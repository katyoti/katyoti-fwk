// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { isLeftOut } from '../src/sections/shell/universeGui'
import { ACTIONS_TAB_ELEMENT, ESSENTIAL_HIDDEN } from '../src/elements'

// An object's « Actions » tab shows at every display level,
// Essentiel included, for every object - never left out by a universe's tag
// view.

// A solution's tag view, written here: the ids its tag lists, at every
// level (its view, at Expert) - the rail, Home's strip and an object's
// Information, never the Actions tab itself.
const solutionView = {
  all: false,
  allowed: ['rail:chat', 'rail:inbox', 'home:actions', 'tab:information', 'tab:contents'],
}
const levels = ['essential', 'standard', 'expert'] as const

describe('« Actions »', () => {
  test('is the element tab:actions, not one Essentiel hides', () => {
    expect(ACTIONS_TAB_ELEMENT).toBe('tab:actions')
    expect(ESSENTIAL_HIDDEN.has(ACTIONS_TAB_ELEMENT)).toBe(false)
    for (const level of levels) expect(isLeftOut(ACTIONS_TAB_ELEMENT, level, null)).toBe(false)
  })

  test("shows at the three levels, untagged or under a solution's view", () => {
    expect(solutionView.allowed).not.toContain(ACTIONS_TAB_ELEMENT)
    const untagged = { all: true, allowed: [] }
    for (const level of levels) {
      for (const view of [null, untagged, solutionView]) {
        expect(isLeftOut(ACTIONS_TAB_ELEMENT, level, view)).toBe(false)
      }
    }
  })

  test('the other technical tabs keep the Essentiel rule', () => {
    for (const id of ['tab:links', 'tab:agent-context', 'tab:lifecycle']) {
      expect(isLeftOut(id, 'essential', null)).toBe(true)
      expect(isLeftOut(id, 'standard', null)).toBe(false)
    }
  })
})

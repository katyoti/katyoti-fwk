// SPDX-License-Identifier: Apache-2.0
// A navigation to a panel of an object can name an element of it,
// which the pane brings into view and highlights once drawn; a declared
// panel's tab shows its badge read from the object, in its tone.

import { describe, expect, test } from 'bun:test'

import { paneFocusOf, tabBadge } from '../src/sections/workspace/paneFocus'

describe('a navigation with an element and a tab badge', () => {
  test('a navigation naming an element carries it to the object tab, each ask told apart', () => {
    expect(paneFocusOf({ element: 'section-03' }, 7)).toEqual({ focus: { element: 'section-03', n: 7 } })
    expect(paneFocusOf({ element: '  picker:sources ' }, 8)).toEqual({ focus: { element: 'picker:sources', n: 8 } })
    expect(paneFocusOf({}, 9)).toEqual({})
    expect(paneFocusOf({ element: '  ' }, 9)).toEqual({})
  })

  test('a declared panel’s badge is its tab’s, in its tone; none without one', () => {
    expect(tabBadge({ label: '2', color: 'red' })).toEqual({ badge: '2', badgeColor: 'red' })
    expect(tabBadge({ label: '5' })).toEqual({ badge: '5' })
    expect(tabBadge(undefined)).toEqual({})
    expect(tabBadge({ label: '' })).toEqual({})
  })
})

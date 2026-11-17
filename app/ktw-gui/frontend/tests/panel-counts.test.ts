// SPDX-License-Identifier: Apache-2.0
// A declared panel's count action (object-panel `count`, answering
// {count: n; color?: tone}) is its tab's badge on an object:
// the number in the answer's tone, nothing at 0 or on an error, and a badge
// stored on the object wins over the count.

import { describe, expect, test } from 'bun:test'

import { countAnswerOf, withCounts } from '../src/hooks/usePanelCounts'
import { countBadge, panelTabBadge } from '../src/sections/workspace/paneFocus'

describe('a panel count', () => {
  test('reads the count action answer and its tone', () => {
    expect(countAnswerOf({ count: 3, color: 'orange' })).toEqual({ count: 3, color: 'orange' })
    expect(countAnswerOf('{"count": 10, "color": "orange"}')).toEqual({ count: 10, color: 'orange' })
    expect(countAnswerOf('{"count": 2}')).toEqual({ count: 2 })
    expect(countAnswerOf({ count: 2, color: 7 })).toEqual({ count: 2 })
    expect(countAnswerOf('garbage')).toEqual({ count: 0 })
    expect(countAnswerOf({ count: -4 })).toEqual({ count: 0 })
    expect(countAnswerOf(null)).toEqual({ count: 0 })
  })

  test('is a tab badge in its tone, none at 0 or with no answer', () => {
    expect(countBadge({ count: 3, color: 'orange' })).toEqual({ label: '3', color: 'orange' })
    expect(countBadge({ count: 3 })).toEqual({ label: '3' })
    expect(countBadge({ count: 0, color: 'orange' })).toBeUndefined()
    expect(countBadge(undefined)).toBeUndefined()
  })

  test('gives way to a badge stored on the object', () => {
    expect(panelTabBadge({ label: '2', color: 'red' }, { count: 9, color: 'orange' })).toEqual({ badge: '2', badgeColor: 'red' })
    expect(panelTabBadge(undefined, { count: 9, color: 'orange' })).toEqual({ badge: '9', badgeColor: 'orange' })
    expect(panelTabBadge({ label: '' }, { count: 9 })).toEqual({ badge: '9' })
    expect(panelTabBadge(undefined, { count: 0 })).toEqual({})
  })

  test('lays its tone on a strip entry', () => {
    const got = withCounts([{ id: 'a', label: 'A' }, { id: 'b', label: 'B' }], { a: { count: 4, color: 'orange' }, b: { count: 0 } })
    expect(got[0]).toMatchObject({ badge: '4', badgeColor: 'orange' })
    expect(got[1].badge).toBeUndefined()
  })
})

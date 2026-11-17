// SPDX-License-Identifier: Apache-2.0
// A global panel's declared count (object-panel `count`, a method answering
// {count: n}) is its badge on the Global strip: « À faire » with the number of
// items waiting on the person. Nothing to count, or 0, leaves the entry as is.

import { describe, expect, test } from 'bun:test'

import { countOf, withCounts } from '../src/hooks/usePanelCounts'

describe('a global panel count', () => {
  test('reads the count method answer', () => {
    expect(countOf('{"count": 7}')).toBe(7)
    expect(countOf({ count: 3 })).toBe(3)
    expect(countOf('{"count": 0}')).toBe(0)
    expect(countOf('not json')).toBe(0)
    expect(countOf('{"items": []}')).toBe(0)
    expect(countOf('')).toBe(0)
  })

  test('is the badge of its strip entry', () => {
    const entries = [
      { id: 'panel:person-queue', label: 'À faire' },
      { id: 'panel:planning-deck', label: 'Planification' },
    ]
    const got = withCounts(entries, { 'panel:person-queue': 4, 'panel:planning-deck': 0 })
    expect(got[0].badge).toBe('4')
    expect(got[1].badge).toBeUndefined()
    expect(entries[0]).not.toHaveProperty('badge')
  })
})

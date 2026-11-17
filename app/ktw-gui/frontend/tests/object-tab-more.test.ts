// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { splitStrip } from '../src/sections/workspace/objectTabMore'

// An object whose aspect lists its main tabs opens on them; the others wait
// behind « More… ».
describe('the object strip splits main and More tabs', () => {
  const tabs = [
    { id: 'panel:information' },
    { id: 'panel:cadrage' },
    { id: 'panel:links', more: true },
    { id: 'panel:actions', more: true },
  ]
  const ids = (ts: { id: string }[]) => ts.map((t) => t.id)

  test('the main tabs alone, and a toggle counting the others', () => {
    const s = splitStrip(tabs, { expert: false, showMore: false, activeId: 'panel:cadrage' })
    expect(ids(s.shown)).toEqual(['panel:information', 'panel:cadrage'])
    expect(s.moreCount).toBe(2)
    expect(s.revealed).toBe(false)
    expect(s.toggle).toBe(true)
  })
  test('opened by the person: every tab, in order, and the toggle to close', () => {
    const s = splitStrip(tabs, { expert: false, showMore: true })
    expect(ids(s.shown)).toEqual(ids(tabs))
    expect(s.revealed).toBe(true)
    expect(s.toggle).toBe(true)
  })
  test('Mode expert shows every tab and no toggle', () => {
    const s = splitStrip(tabs, { expert: true, showMore: false })
    expect(ids(s.shown)).toEqual(ids(tabs))
    expect(s.toggle).toBe(false)
  })
  test('an active More tab is shown', () => {
    const s = splitStrip(tabs, { expert: false, showMore: false, activeId: 'panel:links' })
    expect(ids(s.shown)).toEqual(ids(tabs))
    expect(s.revealed).toBe(true)
  })
  test('no More tab: the strip as it comes, no toggle', () => {
    const plain = [{ id: 'a' }, { id: 'b' }]
    const s = splitStrip(plain, { expert: false, showMore: false })
    expect(ids(s.shown)).toEqual(['a', 'b'])
    expect(s.toggle).toBe(false)
    expect(s.revealed).toBe(false)
  })
})

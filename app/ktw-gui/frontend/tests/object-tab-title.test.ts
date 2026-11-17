// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { objectTabLabel, retitledObjectPayload } from '../src/sections/workspace/objectTabTitle'

// A page navigating opens the Object tab by id alone; the tab must take the
// object's name once its detail is read, not stay "Object".
describe('the Object tab names itself', () => {
  test('opened by id alone, it takes the title the detail read', () => {
    const next = retitledObjectPayload({ objectId: 'shop.api' }, 'shop.api', 'Shop API')
    expect(next).toEqual({ objectId: 'shop.api', objectTitle: 'Shop API' })
    expect(objectTabLabel(next?.objectTitle)).toBe('Object: Shop API')
  })
  test('a tab that moved on to another object keeps its own', () => {
    expect(retitledObjectPayload({ objectId: 'other' }, 'shop.api', 'Shop API')).toBeNull()
  })
  test('nothing to change: no title read, or the same one', () => {
    expect(retitledObjectPayload({ objectId: 'a' }, 'a', '')).toBeNull()
    expect(retitledObjectPayload({ objectId: 'a', objectTitle: 'A' }, 'a', 'A')).toBeNull()
  })
  test('the bare label without a title', () => {
    expect(objectTabLabel()).toBe('Object')
  })
})

// SPDX-License-Identifier: Apache-2.0
// A section's load args read the page's selection:
// `${selection.<key>}` resolved, the section waiting while a
// key it reads is unset, and loading again only when what it reads changes.

import { describe, expect, test } from 'bun:test'

import { sectionLoad } from '../src/ui-builders/page/nodes/section-load.tsx'

const load = { method: 'ktw-x:delivery/release', args: { id: '${selection.release}', view: 'rows' } }

describe("a section's load and the page's selection", () => {
  test('waits while a key it reads is unset', () => {
    expect(sectionLoad(load, {})).toBeUndefined()
    expect(sectionLoad(load, { release: '' })).toBeUndefined()
  })

  test('loads with the resolved value', () => {
    expect(sectionLoad(load, { release: 'RL-1' })?.load).toEqual({ method: 'ktw-x:delivery/release', args: { id: 'RL-1', view: 'rows' } })
  })

  test('loads again when what it reads changes, not when another key does', () => {
    const a = sectionLoad(load, { release: 'RL-1', increment: 'IN-1' })!.at
    expect(sectionLoad(load, { release: 'RL-1', increment: 'IN-2' })!.at).toBe(a)
    expect(sectionLoad(load, { release: 'RL-2', increment: 'IN-1' })!.at).not.toBe(a)
  })

  test('a load reading no selection loads at once, the same each time', () => {
    const plain = { method: 'ktw-x:a/b', args: { id: 'm.X' } }
    expect(sectionLoad(plain, {})?.load).toEqual(plain)
    expect(sectionLoad(plain, { release: 'RL-1' })!.at).toBe(sectionLoad(plain, {})!.at)
  })
})

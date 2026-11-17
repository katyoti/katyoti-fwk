// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'fs'
import { sortItems } from '../src/sections/task/listPrefs'

// The task and skill lists sort by the order the person picks.
const items = [
  { title: 'Beta', category: 'write', source: 'ktw-docs' },
  { title: 'alpha', category: 'plan', source: 'ktw-methods' },
  { title: 'Gamma', source: 'ktw-agents' },
]

describe('a list sorted', () => {
  test('by name', () => {
    expect(sortItems(items, 'name').map((x) => x.title)).toEqual(['alpha', 'Beta', 'Gamma'])
  })
  test('by category, the uncategorised last', () => {
    expect(sortItems(items, 'category').map((x) => x.title)).toEqual(['alpha', 'Beta', 'Gamma'])
    expect(sortItems([items[2], items[0]], 'category').map((x) => x.title)).toEqual(['Beta', 'Gamma'])
  })
  test('by source', () => {
    expect(sortItems(items, 'source').map((x) => x.source)).toEqual(['ktw-agents', 'ktw-docs', 'ktw-methods'])
  })
  test('the task and skill lists offer the choice and keep it', () => {
    for (const f of ['task/TaskGalleryPanel.tsx', 'task/SkillGalleryPanel.tsx']) {
      const src = readFileSync(new URL(`../src/sections/${f}`, import.meta.url), 'utf8')
      expect(src).toContain('<SortSelect')
      expect(src).toContain('listPrefs.setSort(')
    }
  })
})

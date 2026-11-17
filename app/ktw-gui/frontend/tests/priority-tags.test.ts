// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'fs'
import { firstByHashtags, galleryFirstLevel, objectDedicated, priorityOnly } from '../src/sections/shell/universeGui'

// Every task list shows the universe's priority tags first, in
// their order; at Essentiel a tagged universe shows only its end-user
// actions - the tasks declaring audience end-user not dedicated to an object
// type - with « Voir toutes les tâches » for
// the rest.
const tasks = [
  { id: 'admin', hashtags: ['dev'], audience: 'technical' },
  { id: 'study', hashtags: ['etude'], audience: 'end-user' },
  { id: 'dossier', hashtags: ['archifactory'] },
  { id: 'plain', audience: 'end-user' },
  { id: 'both', hashtags: ['etude', 'archifactory'], audience: 'end-user', kind: 'object', types: ['*'] },
  { id: 'on-a-study', hashtags: ['archifactory'], audience: 'end-user', kind: 'object', types: ['study'] },
  { id: 'demo', hashtags: ['archifactory'], audience: 'demo' },
]
const first = ['archifactory', 'etude']
const ids = (xs: { id: string }[]) => xs.map((x) => x.id)

describe('the priority tags', () => {
  test('come first in their order, each item under the earliest it carries', () => {
    expect(ids(firstByHashtags(tasks, first))).toEqual(['dossier', 'both', 'on-a-study', 'demo', 'study', 'admin', 'plain'])
    expect(ids(firstByHashtags(tasks, ['etude', 'archifactory']))).toEqual(['study', 'both', 'dossier', 'on-a-study', 'demo', 'admin', 'plain'])
  })
  test('leave the order as it is without any', () => {
    expect(ids(firstByHashtags(tasks, []))).toEqual(ids(tasks))
  })
})

describe('at Essentiel', () => {
  test('only the end-user tasks not dedicated to an object type show, the others counted', () => {
    const r = priorityOnly(tasks, first, 'essential', false)
    expect(ids(r.items)).toEqual(['study', 'plain', 'both'])
    expect(r.more).toBe(4)
    // ordered by the priority tags, the end-user task of another pack last
    expect(ids(firstByHashtags(r.items, first))).toEqual(['both', 'study', 'plain'])
  })
  test('a task on any object is not dedicated; one on a type is', () => {
    expect(objectDedicated({ kind: 'object', types: ['*'] })).toBe(false)
    expect(objectDedicated({ kind: 'collection', types: ['study'] })).toBe(true)
    expect(objectDedicated({})).toBe(false)
  })
  test('« Voir toutes les tâches » shows every task', () => {
    expect(priorityOnly(tasks, first, 'essential', true)).toEqual({ items: tasks, more: 0 })
  })
  test('every task shows at Standard and Expert, or without priority tags', () => {
    expect(priorityOnly(tasks, first, 'standard', false).more).toBe(0)
    expect(priorityOnly(tasks, first, 'expert', false).more).toBe(0)
    expect(priorityOnly(tasks, [], 'essential', false).more).toBe(0)
  })
})

describe('the task gallery', () => {
  test("a node's own list is never narrowed: its tasks are the ones dedicated to its type", () => {
    const own = tasks.filter((x) => x.id === 'on-a-study')
    expect(galleryFirstLevel(own, true, first, 'essential', false)).toEqual({ items: own, more: 0 })
  })
  test('the global gallery narrows what it lists and counts only that', () => {
    const r = galleryFirstLevel(tasks, false, first, 'essential', false)
    expect(ids(r.items)).toEqual(['study', 'plain', 'both'])
    expect(r.more).toBe(4)
  })
})

describe('each task list', () => {
  const src = (p: string) => readFileSync(new URL(p, import.meta.url), 'utf8')
  test('Home and the task gallery order and narrow, with the link to the rest', () => {
    const grid = src('../src/sections/workspace/SearchableCardGrid.tsx')
    expect(grid).toContain("priorityOnly(items, firstTags, level ?? '', showAll)")
    expect(grid).toContain("t('showAllTasks', { count: narrowed.more })")
    expect(src('../src/sections/home/HomeTab.tsx')).toContain('level={level}')
    const gallery = src('../src/sections/task/TaskGalleryPanel.tsx')
    expect(gallery).toContain("filterTasks(tasks, '', [], [], taskNames)")
    expect(gallery).toContain('galleryFirstLevel(listable, taskNames !== undefined, first, level, showAll)')
    expect(gallery).toContain('firstByHashtags(sortItems(')
    expect(gallery).toContain("t('showAllTasks', { count: narrowed.more })")
  })
})

// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'fs'
import { isTagged, quickTasks, toggleStar } from '../src/sections/task/listPrefs'

// Home's quick actions are the built-in ones, the tasks tagged quick-action
// and the tasks the person starred in this universe; unstarring removes one.
const tasks = [
  { uri: 'p:tasks/a', name: 'a', tags: ['quick-action'] },
  { uri: 'p:tasks/b', name: 'b' },
  { uri: 'p:tasks/c', name: 'c' },
]

describe('the quick actions', () => {
  test('are the tagged tasks, then the starred ones, each once', () => {
    expect(quickTasks(tasks, ['p:tasks/c', 'p:tasks/a']).map((t) => t.name)).toEqual(['a', 'c'])
  })
  test('starring adds a task, starring again removes it', () => {
    const on = toggleStar([], 'p:tasks/b')
    expect(quickTasks(tasks, on).map((t) => t.name)).toEqual(['a', 'b'])
    expect(quickTasks(tasks, toggleStar(on, 'p:tasks/b')).map((t) => t.name)).toEqual(['a'])
  })
  test('a tagged task keeps its place whatever the stars', () => {
    expect(isTagged(tasks[0])).toBe(true)
    expect(quickTasks(tasks, []).map((t) => t.name)).toEqual(['a'])
  })
  test('Home takes its task quick actions from them', () => {
    const home = readFileSync(new URL('../src/sections/home/HomeTab.tsx', import.meta.url), 'utf8')
    expect(home).toContain('quickTasks(tasks, listPrefs.starred, listPrefs.removed, defaults)')
    expect(home).toContain('onToggleStar=')
  })
})

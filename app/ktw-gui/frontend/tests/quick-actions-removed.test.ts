// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'fs'
import { isRemoved, quickId, quickTasks, removeQuick, starTask, type QuickChoices } from '../src/sections/task/listPrefs'

// Home's quick actions are the universe's defaults at the person's
// level; a ✕ removes one in one click and it stays removed for that person
// (GuiSettings, per universe); a star adds a task, and brings back one removed.
const tasks = [
  { uri: 'af:tasks/new-dossier', name: 'new-dossier' },
  { uri: 'af:tasks/new-study', name: 'new-study' },
  { uri: 'af:tasks/admin', name: 'admin' },
  { uri: 'p:tasks/tagged', name: 'tagged', tags: ['quick-action'] },
]
const names = (choices: QuickChoices, defaults?: string[]) =>
  quickTasks(tasks, choices.starred, choices.removed, defaults).map((t) => t.name)
const none: QuickChoices = { starred: [], removed: [] }

describe('the universe\'s defaults', () => {
  test('are the tasks its view lists at the person\'s level', () => {
    const essentiel = ['af:tasks/new-dossier']
    const standard = ['af:tasks/new-dossier', 'task:af:tasks/new-study']
    expect(names(none, essentiel)).toEqual(['new-dossier'])
    expect(names(none, standard)).toEqual(['new-dossier', 'new-study'])
  })
  test('are the tagged tasks when its tags give none', () => {
    expect(names(none)).toEqual(['tagged'])
  })
})

describe('a ✕ on a quick action', () => {
  const defaults = ['af:tasks/new-dossier', 'af:tasks/new-study']
  test('removes a default, and it stays removed', () => {
    const after = removeQuick(none, 'task:af:tasks/new-study')
    expect(names(after, defaults)).toEqual(['new-dossier'])
    expect(removeQuick(after, 'task:af:tasks/new-study')).toEqual(after)
  })
  test('removes a starred task and unstars it', () => {
    const starred = starTask(none, 'af:tasks/admin')
    expect(names(starred, defaults)).toEqual(['new-dossier', 'new-study', 'admin'])
    const after = removeQuick(starred, 'task:af:tasks/admin')
    expect(after.starred).toEqual([])
    expect(names(after, defaults)).toEqual(['new-dossier', 'new-study'])
  })
  test('removes a built-in by its element id', () => {
    expect(removeQuick(none, 'quick:term')).toEqual({ starred: [], removed: ['quick:term'] })
  })
  test('removes a task by its element id, and unstars it', () => {
    const starred = starTask(none, 'af:tasks/admin')
    const after = removeQuick(starred, 'quick:task/af/admin')
    expect(after.starred).toEqual([])
    expect(names(after, ['af:tasks/new-dossier'])).toEqual(['new-dossier'])
  })
  // A task's URI in the GUI's list is `<provider>:<name>`
  // (ListTasksDetail); the ✕ passes its element id. Removing it must take its
  // star off whatever the URI's shape, so the bar and the stars agree at once.
  test('unstars a task whose URI is <provider>:<name>, as the GUI lists it', () => {
    for (const uri of ['ktw-acme:new-study', 'af:tasks/admin', 'p:sub/task']) {
      const starred = starTask(none, uri)
      const after = removeQuick(starred, quickId({ uri, name: 'x' }))
      expect(after.starred).toEqual([])
      expect(after.removed).toEqual([quickId({ uri, name: 'x' })])
      // and starring it again brings it back to the bar
      const back = starTask(after, uri)
      expect(back.starred).toEqual([uri])
      expect(isRemoved(back.removed, quickId({ uri, name: 'x' }))).toBe(false)
    }
  })
  test('what was removed under its old id stays removed', () => {
    expect(isRemoved(['term'], 'quick:term')).toBe(true)
    expect(isRemoved(['debug-and-tests'], 'quick:debug')).toBe(true)
    expect(isRemoved(['start-chat'], 'quick:start-chat')).toBe(true)
    expect(isRemoved(['task:af:tasks/new-study'], 'quick:task/af/new-study')).toBe(true)
    expect(isRemoved(['term'], 'quick:drawio')).toBe(false)
  })
})

describe('a star', () => {
  test('adds a task', () => {
    expect(names(starTask(none, 'af:tasks/admin'), ['af:tasks/new-dossier'])).toEqual(['new-dossier', 'admin'])
  })
  test('brings back a quick action the person removed', () => {
    const removed = removeQuick(none, 'task:af:tasks/new-dossier')
    const back = starTask(removed, 'af:tasks/new-dossier')
    expect(back.removed).toEqual([])
    expect(names(back, ['af:tasks/new-dossier'])).toEqual(['new-dossier'])
  })
})

describe('Home', () => {
  const src = (p: string) => readFileSync(new URL(p, import.meta.url), 'utf8')
  test('drops what the person removed, offers the ✕ and keeps the choice in the settings', () => {
    const home = src('../src/sections/home/HomeTab.tsx')
    expect(home).toContain('quickActions.filter((q) => !isRemoved(listPrefs.removed, q.id))')
    expect(home).toContain('onRemoveQuick={(q) => listPrefs.removeQuick(q.id)}')
    expect(src('../src/sections/workspace/SearchableCardGrid.tsx')).toContain("aria-label={t('removeQuick')}")
    expect(src('../src/hooks/useListPrefs.ts')).toContain('removed: { ...(prefs.removed ?? {}), [u]: next.removed }')
  })
})

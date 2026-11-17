// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { pickOf } from '../src/sections/workspace/NewMenu'

// « + New » is the pane's toolbar button opening a menu of
// exactly what « + » offered - the create dialog's own list (offeredChoices
// over WorkspaceObjectChildTypes) - and no box stands on the pane.
describe('« + New » button and menu', () => {
  test('a type opens its creation, one with a create task runs it', () => {
    expect(pickOf({ name: 'goal', label: 'Goal' } as never)).toEqual({ kind: 'create', type: 'goal' })
    const study = { name: 'etude-existant', createTask: 'ktw-acme:tasks/new-study' } as never
    expect(pickOf(study)).toEqual({ kind: 'task', choice: study })
  })
  test('the menu lists what the create dialog lists, and the pane holds no box', () => {
    const src = (f: string) => readFileSync(join(import.meta.dir, '..', 'src', 'sections', 'workspace', f), 'utf8')
    const menu = src('NewMenu.tsx'), modal = src('ObjectCreateModal.tsx'), pane = src('ObjectTab.tsx')
    for (const s of [menu, modal]) {
      expect(s).toContain('WorkspaceObjectChildTypes(universeName, parentId)')
      expect(s).toContain('offeredChoices(choices, ')
    }
    expect(pane).toContain('<NewMenu ')
    expect(pane).not.toContain('NewHereBox')
  })
})

// With nothing allowed, « + New » says why and where to add
// instead - object.new-empty - and lists nothing (no fallback of every type).
describe('« + New » with nothing allowed', () => {
  test('an explaining empty state, never a fallback list', () => {
    const menu = readFileSync(join(import.meta.dir, '..', 'src', 'sections', 'workspace', 'NewMenu.tsx'), 'utf8')
    expect(menu).toContain('data-guide="object.new-empty"')
    expect(menu).toContain("t('create.emptyTitle')")
    expect(menu).toContain("t('create.emptyBody')")
    expect(menu).toMatch(/offered === null \|\| offered\.length === 0 \? null :/)
    for (const lang of ['en', 'fr']) {
      const c = JSON.parse(readFileSync(join(import.meta.dir, '..', 'src', 'locales', lang, 'workspace.json'), 'utf8')).create
      expect(c.emptyTitle).toBeTruthy()
      expect(c.emptyBody).toBeTruthy()
    }
  })
})

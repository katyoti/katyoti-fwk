// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { formInitial, formItem } from '../src/hooks/formInitial'
import { formFirst } from '../src/sections/workspace/createChoices'

const src = (...p: string[]) => readFileSync(join(import.meta.dir, '..', 'src', ...p), 'utf8')

// « + New » on a type whose create task has a form, and a global
// action at the project's root, open that task's form with its parent
// filled - for every user (its display switch retired).
describe('a create task opens its form with its parent filled', () => {
  test('the form opens with the parent only when the display asks it', () => {
    expect(formInitial('dossier-1', true)).toEqual({ parent: 'dossier-1' })
    expect(formInitial('dossier-1', false)).toBeUndefined()
    expect(formInitial(undefined, true)).toBeUndefined()
  })

  test('a form keeps its defaults and takes only the fields it has', () => {
    expect(formItem({ name: '', lang: 'fr' }, { parent: 'd-1' }, ['name', 'lang', 'parent']))
      .toEqual({ name: '', lang: 'fr', parent: 'd-1' })
    // A form with no parent field gets nothing: no unknown field is sent.
    expect(formItem({ name: '' }, { parent: 'd-1' }, ['name'])).toBeUndefined()
    expect(formItem({ name: '' }, undefined, ['name', 'parent'])).toBeUndefined()
  })

  test('picking a type with a create task tries its form first, only with the display on', () => {
    expect(formFirst({ name: 'etude-existant', createTask: 'ktw-acme:tasks/new-study' }, true)).toBe(true)
    expect(formFirst({ name: 'etude-existant', createTask: 'new-study' }, true)).toBe(true)
    expect(formFirst({ name: 'etude-existant', createTask: 'ktw-acme:tasks/new-study' }, false)).toBe(false)
    expect(formFirst({ name: 'goal' }, true)).toBe(false)
    // A page method is drawn by the new-project dialog, not tried as a form.
    expect(formFirst({ name: 'dossier', createTask: 'ktw-acme:acme/dossier-new-page' }, true)).toBe(false)
  })

  test('every entry point reaches the form with its parent', () => {
    const modal = src('sections', 'workspace', 'ObjectCreateModal.tsx')
    expect(modal).not.toContain("useDisplaySwitch('in045')")
    expect(modal).toContain('const formFirst = true')
    expect(modal).toContain('openTaskForm(universeName, c.createTask, parentId || undefined, undefined, true, entryTrail(')
    expect(modal).toContain('onClick={() => pick(c)}')
    expect(src('sections', 'workspace', 'ObjectTab.tsx')).toMatch(/runCreateTask\([^\n]*, formFirst\)/)
    const actions = src('sections', 'workspace', 'ActionsPanel.tsx')
    expect(actions).toContain('initial: formInitial(nodeId, true)')
    expect(src('hooks', 'taskForm.ts')).toContain('initial: formInitial(targetId, fillParent)')
    expect(src('sections', 'agent', 'RunTaskPanel.tsx')).toContain('initial={pendingTask.initial}')
    expect(src('sections', 'agent', 'FormWidgetPanel.tsx')).toContain('item={item}')
  })
})

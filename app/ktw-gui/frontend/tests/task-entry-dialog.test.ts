// SPDX-License-Identifier: Apache-2.0
import { beforeEach, describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { declaredActions, entryTitle } from '../src/sections/agent/entryForm'
import { entryTrail } from '../src/sections/workspace/createChoices'
import { openTaskForm } from '../src/hooks/taskForm'
import { useAppStore } from '../src/store'

// « + Nouveau » on a type whose create task has a form shows that form in a
// dialog over the object (TaskEntryDialog), never in the main area: a header
// line and the form's own title, no task name nor URI, and the form's
// declared submit label on its button - not the launcher's « Lancer ».
const src = (...p: string[]) => readFileSync(new URL('../src/' + p.join('/'), import.meta.url), 'utf8')

const schema = (groups: { title?: string; fields: string[] }[], labels: Record<string, string> = {}) => ({
  root: 'Input',
  types: {
    Input: {
      type: 'object',
      fields: { name: { type: 'string' } },
      extensions: [{ type: 'ui-form', name: 'default', ...labels, groups }],
    },
  },
}) as never

const refs = [
  { type: 'ui-action', id: 'cancel', scope: 'item', label: 'Annuler' },
  { type: 'ui-action', id: 'submit', scope: 'item', label: 'Créer' },
] as never[]

describe('the entry form of « + Nouveau »', () => {
  test('its single group title is lifted into the header and drawn once', () => {
    const s = schema([{ title: 'Nouvel exemple', fields: ['name'] }])
    const { title, schema: lifted } = entryTitle(s)
    expect(title).toBe('Nouvel exemple')
    const groups = (lifted as { types: { Input: { extensions: { groups: { title?: string; fields: string[] }[] }[] } } }).types.Input.extensions[0].groups
    expect(groups).toEqual([{ fields: ['name'] }])
    // The fetched schema is kept as it came.
    expect((s as { types: { Input: { extensions: { groups: { title?: string }[] }[] } } }).types.Input.extensions[0].groups[0].title).toBe('Nouvel exemple')
  })

  test('a form of several groups, or untitled, keeps its schema and has no title', () => {
    const several = schema([{ title: 'A', fields: ['name'] }, { title: 'B', fields: [] }])
    expect(entryTitle(several)).toEqual({ schema: several })
    const untitled = schema([{ fields: ['name'] }])
    expect(entryTitle(untitled).title).toBeUndefined()
  })

  test('its buttons carry the form declared labels', () => {
    const s = schema([{ fields: ['name'] }], { submit_label: "Créer l'exemple" })
    const out = declaredActions(s, refs) as { id: string; label: string }[]
    expect(out.map((r) => r.label)).toEqual(['Annuler', "Créer l'exemple"])
    // A form declaring no label keeps the caller's.
    expect(declaredActions(schema([{ fields: ['name'] }]), refs)).toBe(refs)
    expect(declaredActions(null, refs)).toBe(refs)
  })

  test('its header line names what is created and under which object', () => {
    const t = (key: string, o?: Record<string, unknown>) => `${key}:${o?.what}:${o?.under ?? ''}`
    expect(entryTrail(t, 'Exemple', 'Dossier X')).toBe('create.entryTrailUnder:Exemple:Dossier X')
    expect(entryTrail(t, 'Exemple')).toBe('create.entryTrail:Exemple:')
    const fr = JSON.parse(src('locales', 'fr', 'workspace.json')).create
    expect(fr.entryTrailUnder).toBe('+ Nouveau › {{what}} · sous « {{under}} »')
  })
})

describe('« + Nouveau » opens the form in the dialog, the main area untouched', () => {
  beforeEach(() => {
    useAppStore.setState({ pendingTask: null, entryForm: null })
    ;(globalThis as unknown as { window: unknown }).window = {
      go: { main: { App: { WorkspaceActionTask: async () => ({ task: 'ktw-example:new-thing', title: 'New thing', inputSchemaURI: 'ktw-example:/data-type/new-thing-input' }) } } },
    }
  })

  test('with a header line the form goes to the dialog', async () => {
    expect(await openTaskForm('u', 'ktw-example:tasks/new-thing', 'dossier-1', undefined, true, '+ Nouveau › Thing')).toBe(true)
    const s = useAppStore.getState()
    expect(s.pendingTask).toBeNull()
    expect(s.entryForm?.trail).toBe('+ Nouveau › Thing')
    expect(s.entryForm?.initial).toEqual({ parent: 'dossier-1' })
    s.closeEntryForm()
    expect(useAppStore.getState().entryForm).toBeNull()
  })

  test('without one (a page chat button) it stays in the main area', async () => {
    expect(await openTaskForm('u', 'ktw-example:tasks/new-thing', 'dossier-1')).toBe(true)
    expect(useAppStore.getState().entryForm).toBeNull()
    expect(useAppStore.getState().pendingTask?.inputSchemaURI).toBe('ktw-example:/data-type/new-thing-input')
  })

  test('both « + Nouveau » paths pass the header line; the app mounts the dialog', () => {
    const modal = src('sections', 'workspace', 'ObjectCreateModal.tsx')
    expect(modal).toContain('openTaskForm(universeName, createTask, parentId || undefined, undefined, fillParent, trail)')
    expect(modal).toMatch(/openTaskForm\(universeName, c\.createTask, parentId \|\| undefined, undefined, true, entryTrail\(/)
    expect(src('App.tsx')).toContain('<TaskEntryDialog />')
  })

  test('the dialog shows no task name nor URI, and runs as the main area form does', () => {
    const dialog = src('sections', 'agent', 'TaskEntryDialog.tsx')
    expect(dialog).not.toContain('TaskIdentity')
    expect(dialog).not.toMatch(/\b(name|uri|provider):\s*entry\./)
    expect(dialog).toContain('entry={{ trail: entry.trail }}')
    expect(dialog).toContain('useFormRun(entry, universeName, close)')
    const panel = src('sections', 'agent', 'FormWidgetPanel.tsx')
    const entryHeader = panel.slice(panel.indexOf('const customTitleNode = entry ?'), panel.indexOf(') : header ? ('))
    expect(entryHeader).toContain('task.entry.trail')
    expect(entryHeader).not.toContain('TaskIdentity')
  })
})

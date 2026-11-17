// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { createTaskPage, newProjectPick } from '../src/sections/workspace/newProjectPick'
import { typeStylesOf } from '../src/hooks/useTypeStyles'

// The new-project dialog runs the chosen project type's create
// task with the name typed; a type without one is created by the hub and typed
// as before (the default type kept, another retyped).
describe('the new-project dialog runs a project type\'s create task', () => {
  test('a type declaring a create task runs it with the name', () => {
    const pick = newProjectPick('dossier-architecture', "Dossier d'architecture", ' ERP Acme ', 'ktw-acme:tasks/new-dossier')
    expect(pick.kind).toBe('task')
    if (pick.kind === 'task') {
      expect(pick.task).toBe('new-dossier')
      expect(pick.input).toContain('named "ERP Acme"')
    }
  })
  test('a type without one is created, typed only when not the default', () => {
    expect(newProjectPick('software-project', 'Software project', 'shop', undefined)).toEqual({ kind: 'create', retype: null })
    expect(newProjectPick('dossier-architecture', 'Dossier', 'demos', undefined)).toEqual({ kind: 'create', retype: 'dossier-architecture' })
  })
  test('the type styles carry a project type\'s create task', () => {
    const s = typeStylesOf({ object_types: [{ name: 'dossier-architecture', capabilities: ['project'], create_task: 'ktw-acme:tasks/new-dossier' }] })
    expect(s['dossier-architecture'].createTask).toBe('ktw-acme:tasks/new-dossier')
  })
})

// A create_task that is a method URI answers a page - the dialog
// opens that form (a dossier d'architecture, no AI run); a tasks/<name> one is
// run as a task.
describe('a project type created by its form', () => {
  test('a method-URI create_task gives a page pick, its source the method', () => {
    expect(newProjectPick('dossier-architecture', "Dossier d'architecture", '', 'ktw-acme:acme/dossier-project-new-page'))
      .toEqual({ kind: 'page', source: 'ktw-acme:acme/dossier-project-new-page' })
  })
  test('a tasks/<name> create_task, or a bare name, is a task pick', () => {
    expect(newProjectPick('dossier-architecture', 'D', 'x', 'ktw-acme:tasks/new-dossier').kind).toBe('task')
    expect(newProjectPick('dossier-architecture', 'D', 'x', 'new-dossier').kind).toBe('task')
    expect(createTaskPage('ktw-acme:tasks/new-dossier')).toBeUndefined()
    expect(createTaskPage('new-dossier')).toBeUndefined()
    expect(createTaskPage(undefined)).toBeUndefined()
  })
})

// « + New project »
// makes a project the universe's in one of two ways - created as its own
// repository through its type's wizard (above), or an existing repository
// linked (the project's attach). Never a project inside another: no
// project-group row, no folder of an opened repository.
import { linkExistingProject } from '../src/sections/univers/linkProject'

describe('« + New project » creates a project as its own repository or links one', () => {
  const linker = () => {
    const calls: unknown[] = []
    return { calls, client: { attachProject: async (input: { universe: string; projectDir: string }) => { calls.push(input); return { ...input, restarted: true } } } }
  }
  test('the folder picked is attached to the universe: one hub call', async () => {
    const { calls, client } = linker()
    const out = await linkExistingProject(client, 'acme', async () => ({ path: 'D:/git/ktw-acme-demo' }))
    expect(calls).toEqual([{ universe: 'acme', projectDir: 'D:/git/ktw-acme-demo' }])
    expect(out?.projectDir).toBe('D:/git/ktw-acme-demo')
  })
  test('a cancelled picker attaches nothing; a failing one says why', async () => {
    const { calls, client } = linker()
    expect(await linkExistingProject(client, 'acme', async () => ({}))).toBeNull()
    const err = await linkExistingProject(client, 'acme', async () => ({ error: 'no display' })).catch((e) => e)
    expect(String(err?.message)).toBe('no display')
    expect(calls).toEqual([])
  })
  test('the dialog offers create and link, waits until the link is served, and offers no group or folder-in-a-repository', () => {
    const src = (...p: string[]) => readFileSync(join(import.meta.dir, '..', 'src', ...p), 'utf8')
    const dialog = src('sections', 'workspace', 'NewAnywhere.tsx')
    expect(dialog).toContain('await createNewProject(orchestratorClient(), universeName, name)')
    expect(dialog).toContain('await linkExistingProject(orchestratorClient(), universeName, pick)')
    expect(dialog).toContain('await awaitServed(universeName, out, out.projectDir)')
    expect(dialog).toContain('data-guide="project.link-existing"')
    expect(dialog).not.toContain('project-group')
    expect(dialog).not.toMatch(/in: /)
    expect(src('sections', 'workspace', 'NewMenu.tsx')).not.toContain('onChild')
    for (const lang of ['en', 'fr']) {
      const project = JSON.parse(src('locales', lang, 'shell.json')).project
      expect(typeof project.linkExisting).toBe('string')
      expect(typeof project.orLink).toBe('string')
    }
  })
})

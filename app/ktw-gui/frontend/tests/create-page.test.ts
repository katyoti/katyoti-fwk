// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { createPage, createTaskPage } from '../src/sections/workspace/createChoices'
import { pickOf } from '../src/sections/workspace/NewMenu'
import type { main } from '../wailsjs/go/models'

// « + Nouveau » under an object, for a type whose create_task
// answers a page (an information source's: its connector asked, a connector
// the holder already has greyed with « déjà dans l'étude »), opens the create
// dialog at that type, which draws the page under the parent, for every user
// (its switch retired). A task's create_task is run as before.
const source = { name: 'information-source', label: 'Information source', createTask: 'ktw-agents:sources/new-source-page' } as main.WorkspaceNodeTypeChoice
const study = { name: 'etude-existant-v2', label: 'Étude', createTask: 'ktw-acme:tasks/new-study-v2' } as main.WorkspaceNodeTypeChoice
const plain = { name: 'document', label: 'Document' } as main.WorkspaceNodeTypeChoice

describe('a type created by its page under an object', () => {
  test('picking it opens the create dialog at the type; a task runs', () => {
    expect(pickOf(source)).toEqual({ kind: 'create', type: 'information-source' })
    expect(pickOf(plain)).toEqual({ kind: 'create', type: 'document' })
    expect(pickOf(study).kind).toBe('task')
  })
  test('the dialog draws the page of a type created by its page', () => {
    expect(createPage(source)).toBe('ktw-agents:sources/new-source-page')
    expect(createPage(study)).toBeUndefined()
    expect(createPage(plain)).toBeUndefined()
    expect(createPage(undefined)).toBeUndefined()
    expect(createTaskPage('ktw-acme:tasks/new-dossier')).toBeUndefined()
  })
  test('the dialog loads the page with the parent, behind no switch', () => {
    const src = readFileSync(new URL('../src/sections/workspace/ObjectCreateModal.tsx', import.meta.url), 'utf8')
    expect(src).not.toContain('useDisplaySwitch')
    expect(src).toContain("id: 'object-create-form'")
    expect(src).toContain('load: { method: page, args: { parent: parentId } }')
  })
})

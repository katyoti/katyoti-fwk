// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { viewAllows, type ViewLists } from '../src/sections/shell/universeGui'
import { createTaskName, offeredChoices } from '../src/sections/workspace/createChoices'

// « + New » offers a module carrying an aspect - the étude de
// l'existant - beside the object types, matched by the aspect's name in the
// view's « + New » types, and opens its short creation (its create task).
const choices = [
  { name: 'document', label: 'Document' },
  { name: 'module', label: 'Module' },
  { name: 'dossier-architecture', label: "Dossier d'architecture", createTask: 'ktw-acme:tasks/new-dossier' },
  { name: 'etude-existant', label: "Étude de l'existant", createTask: 'ktw-acme:tasks/new-study' },
]
const archifactory: ViewLists = { all: false, allowed: ['new:etude-existant'] }

describe('« + New » under a view whose types are [etude-existant]', () => {
  const offered = offeredChoices(choices, undefined, (id) => viewAllows(id, archifactory))
  test('lists « Étude de l\'existant » only', () => {
    expect(offered?.map((c) => c.label)).toEqual(["Étude de l'existant"])
  })
  test('opens new-study', () => {
    expect(createTaskName(offered![0].createTask!)).toBe('new-study')
  })
})

describe('« + New » in an untagged universe', () => {
  test('lists the types and every creatable aspect', () => {
    const all: ViewLists = { all: true, allowed: [] }
    expect(offeredChoices(choices, undefined, (id) => viewAllows(id, all))?.map((c) => c.name)).toEqual(['document', 'module', 'dossier-architecture', 'etude-existant'])
  })
  test('keeps a type the caller preset, whatever the view', () => {
    expect(offeredChoices(choices, 'module', (id) => viewAllows(id, archifactory))?.map((c) => c.name)).toEqual(['module', 'etude-existant'])
  })
  test('is still loading while the choices are unknown', () => {
    expect(offeredChoices(null, undefined, () => true)).toBeNull()
  })
})

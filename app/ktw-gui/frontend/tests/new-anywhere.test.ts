// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { newTargetFor, projectTypes } from '../src/sections/workspace/newTarget'
import { viewAllows, type ViewLists } from '../src/sections/shell/universeGui'
import { isElementId } from '../src/elements'

// « + New » from the top bar adds under the place selected; at the
// universe's level it offers the project types, the default one first.
describe('« + New » anywhere', () => {
  test('under the object selected, else a new project', () => {
    expect(newTargetFor('agent')).toEqual({ kind: 'object', parent: 'agent' })
    expect(newTargetFor(undefined)).toEqual({ kind: 'project' })
  })
  test('the project types, the default first, then by label', () => {
    const styles = {
      'software-project': { level: 'project' as const, label: 'Software project' },
      'dossier-architecture': { level: 'project' as const, label: "Dossier d'architecture" },
      'software-module': { level: 'module' as const },
    }
    expect(projectTypes(styles)).toEqual(['software-project', 'dossier-architecture'])
  })
  // A tagged universe offers the project types its view names by
  // project:<type> - ArchiFactory the dossier d'architecture alone; untagged,
  // every one, the default first.
  test('the project types the view allows', () => {
    const styles = {
      'software-project': { level: 'project' as const, label: 'Software project' },
      'dossier-architecture': { level: 'project' as const, label: "Dossier d'architecture" },
    }
    const archifactory: ViewLists = { all: false, allowed: ['new:etude-existant', 'project:dossier-architecture'] }
    expect(projectTypes(styles, (id) => viewAllows(id, archifactory))).toEqual(['dossier-architecture'])
    const all: ViewLists = { all: true, allowed: [] }
    expect(projectTypes(styles, (id) => viewAllows(id, all))).toEqual(['software-project', 'dossier-architecture'])
    expect(isElementId('project:dossier-architecture')).toBe(true)
  })
})

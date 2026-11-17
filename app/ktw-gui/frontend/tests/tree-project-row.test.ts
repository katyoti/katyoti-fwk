// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'

// The tree draws a project-level root with its type's icon and badge, and no
// « its own repository » mark: one repository is one project, so every project
// is its own repository and the mark said nothing.
const read = (rel: string) => readFileSync(new URL(rel, import.meta.url), 'utf8')

describe('a project row in the tree', () => {
  test('carries no own-repository mark', () => {
    const tree = read('../src/sections/workspace/WorkspaceObjectsTree.tsx')
    expect(tree).not.toContain('tree.own-repository')
    expect(tree).not.toContain('tree.ownRepository')
    expect(read('../src/guide/anchors.ts')).not.toContain('tree.own-repository')
  })
  test('keeps its type icon and type badge', () => {
    const tree = read('../src/sections/workspace/WorkspaceObjectsTree.tsx')
    expect(tree).toContain('<Icon size={13}')
    expect(tree).toContain('data-guide="tree.type-badge"')
  })
  test('the live tree walk finds project rows without the removed mark', () => {
    const walk = read('./live/linked-projects-tree.sh')
    expect(walk).not.toContain('own-repository')
    expect(walk).toContain('const projectNodes=()=>nodes().filter(isProject)')
  })
  test('the locales hold no own-repository label', () => {
    for (const lang of ['en', 'fr']) {
      const ws = JSON.parse(read(`../src/locales/${lang}/workspace.json`))
      expect(ws.tree?.ownRepository).toBeUndefined()
    }
  })
})

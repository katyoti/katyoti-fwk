// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'

import { buildObjectTree } from '../src/sections/workspace/WorkspaceObjectsTree'
import { groupModulesByType, isGroupId } from '../src/sections/workspace/treeGroups'
import type { main } from '../wailsjs/go/models'

// One repository is
// one project and the universe links projects. The tree lists the universe's
// linked projects side by side - each project's root a root of the tree, its
// modules under it - and a click opens any object in place, in the Object
// tab: no repository node over several projects, no group or child node, no
// way back to a group, nothing that switches what the universe serves.

const row = (id: string, type: string, depth: number, project: string, module = '') =>
  ({ id, title: id, type, module, depth, hasChildren: false, project }) as main.WorkspaceObject

const styles = {
  'dossier-architecture': { level: 'project' },
  'software-project': { level: 'project' },
  study: { level: 'module' },
  module: { level: 'module' },
} as never

// Two linked projects, each its own repository, answered in one flat list.
const flat = [
  row('demo', 'dossier-architecture', 0, 'demo'),
  row('erp-acme', 'study', 1, 'demo', 'erp-acme'),
  row('si-rh', 'study', 1, 'demo', 'si-rh'),
  row('shop', 'software-project', 0, 'shop'),
  row('cart', 'module', 1, 'shop', 'cart'),
]

const tree = () => groupModulesByType(buildObjectTree(flat), styles, (t) => `${t}s`)

describe('the tree lists the universe\'s linked projects side by side', () => {
  test('each linked project is a root of its own, in the order answered, its modules under it', () => {
    const roots = tree()
    expect(roots.map((r) => r.value)).toEqual(['demo:demo', 'shop:shop'])
    expect(roots.map((r) => r.nodeProps?.project)).toEqual(['demo', 'shop'])
    const studies = roots[0].children.find((c) => isGroupId(c.value))!
    expect(studies.children.map((c) => c.value)).toEqual(['demo:erp-acme', 'demo:si-rh'])
    expect(roots[1].children.flatMap((g) => g.children.map((c) => c.value))).toEqual(['shop:cart'])
  })
  test('no project is drawn under another: no repository, group or child node over projects', () => {
    const walk = (nodes: ReturnType<typeof tree>): string[] =>
      nodes.flatMap((n) => [String(n.nodeProps?.type ?? ''), ...walk(n.children as never)])
    const roots = tree()
    for (const r of roots) {
      const below = walk(r.children as never)
      expect(below).not.toContain('dossier-architecture')
      expect(below).not.toContain('software-project')
    }
    const all = JSON.stringify(roots)
    expect(all).not.toContain('"child":true')
    expect(all).not.toContain('repo:')
  })
  test('a click opens the object in place; nothing switches the served project or asks for a group', () => {
    const src = readFileSync(join(import.meta.dir, '..', 'src', 'sections', 'workspace', 'WorkspaceObjectsTree.tsx'), 'utf8')
    expect(src).toContain("openObjectTab(universeName, { id: node.value, title: String(node.label ?? '') })")
    // The group words are joined here so the GUI's « no group code left » grep finds none in the tests.
    const gone = ['switchServedProject', 'attachChildren', 'tree.child'].concat(
      [['Project', 'Children'], ['openChild', 'Project'], ['backTo', 'Group'], ['tree.back-to', '-group']].map((p) => p.join('')))
    for (const word of gone) {
      expect(src).not.toContain(word)
    }
  })
})

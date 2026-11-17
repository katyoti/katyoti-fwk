// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { groupModulesByType, groupsToOpen, isGroupId, type GroupedNode } from '../src/sections/workspace/treeGroups'

// The tree groups a project's modules by type under their plural
// label, in the order the project type gives.
const node = (value: string, type: string, depth: number, children: GroupedNode[] = []): GroupedNode =>
  ({ value, label: value, nodeProps: { type, depth }, children })

const styles = {
  'software-project': { level: 'project' as const, childOrder: ['software-module', 'documentation', 'study', 'system-module'] },
  'software-module': { level: 'module' as const },
  documentation: { level: 'module' as const },
  'system-module': { level: 'module' as const },
}
const plural = (t: string) => ({ 'software-module': 'Software modules', documentation: 'Documentation', 'system-module': 'System modules' } as Record<string, string>)[t] ?? t


describe('the tree groups modules by type', () => {
  test('one header per type, in the project type\'s order, each module under its header', () => {
    const tree = [node('ktw-agents', 'software-project', 0, [
      node('rules', 'documentation', 1),
      node('sys-inbox', 'system-module', 1),
      node('billing', 'software-module', 1, [node('billing.G-001', 'goal', 2)]),
      node('cwm', 'software-module', 1),
    ])]
    const [p] = groupModulesByType(tree, styles, plural)
    expect(p.children.map((g) => g.label)).toEqual(['Software modules', 'Documentation', 'System modules'])
    expect(p.children.every((g) => isGroupId(g.value))).toBe(true)
    expect(p.children[0].children.map((m) => m.value)).toEqual(['billing', 'cwm'])
    expect(p.children[0].children[0].nodeProps?.depth).toBe(2)
    expect(p.children[0].children[0].children[0].nodeProps?.depth).toBe(3)
  })
  test('a root that is no project, or a project of no module, is left as it is', () => {
    const loose = [node('x', 'note', 0), node('p', 'software-project', 0, [node('p.d', 'document', 1)])]
    expect(groupModulesByType(loose, styles, plural)).toEqual(loose)
  })
})

describe('a group header is open when it first shows', () => {
  test('the headers of a tree loaded after the first render open once; one closed stays closed', () => {
    const tree = groupModulesByType([node('walk-b', 'software-project', 0, [node('walk-b:sales', 'documentation', 1)])], styles, plural)
    const opened = new Set<string>()
    expect(groupsToOpen(tree, opened)).toEqual(['group:walk-b:documentation'])
    expect(groupsToOpen(tree, opened)).toEqual([])
  })
})

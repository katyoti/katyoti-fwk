// SPDX-License-Identifier: Apache-2.0
// The tree groups a project's modules by type (the approved
// object-levels mock-up): under the project, one group per module type, its
// plural label as the header, in the order the project type gives
// (child_order); a type it does not name comes after, by name. A module's own
// subtree stays under it. The groups are display-only: a header row, never an
// object.

import type { TreeNodeData } from '@mantine/core'
import type { DeclaredTypeStyle } from './type-style'

export interface GroupedNode extends TreeNodeData {
  children: GroupedNode[]
}

/** groupId is a group header's value: never an object id. */
export function groupId(project: string, type: string): string {
  return `group:${project}:${type}`
}

/** isGroupId says whether a node value is a group header's. */
export function isGroupId(value: string): boolean {
  return value.startsWith('group:')
}

/** groupModulesByType puts each project's module children under one header
 *  per type, ordered by the project type's child order. `plural` gives a
 *  type's plural label (translated). */
export function groupModulesByType(
  roots: GroupedNode[],
  styles: Record<string, DeclaredTypeStyle>,
  plural: (type: string) => string,
): GroupedNode[] {
  return roots.map((root) => {
    const rootType = String(root.nodeProps?.type ?? '')
    if (styles[rootType]?.level !== 'project') return root
    const order = styles[rootType]?.childOrder ?? []
    const byType = new Map<string, GroupedNode[]>()
    const rest: GroupedNode[] = []
    for (const child of root.children) {
      const type = String(child.nodeProps?.type ?? '')
      if (styles[type]?.level !== 'module') {
        rest.push(child)
        continue
      }
      byType.set(type, [...(byType.get(type) ?? []), child])
    }
    if (byType.size === 0) return root
    const rank = (t: string) => {
      const i = order.indexOf(t)
      return i < 0 ? order.length : i
    }
    const types = [...byType.keys()].sort((a, b) => rank(a) - rank(b) || a.localeCompare(b))
    const groups: GroupedNode[] = types.map((type) => ({
      value: groupId(root.value, type),
      label: plural(type),
      nodeProps: { group: true, type, depth: Number(root.nodeProps?.depth ?? 0) + 1, count: byType.get(type)!.length },
      children: byType.get(type)!.map((m) => shift(m)),
    }))
    return { ...root, children: [...groups, ...rest] }
  })
}

// shift moves a module's subtree one level deeper: it now sits under its
// group's header.
function shift(n: GroupedNode): GroupedNode {
  return { ...n, nodeProps: { ...n.nodeProps, depth: Number(n.nodeProps?.depth ?? 0) + 1 }, children: n.children.map(shift) }
}

/** groupsToOpen gives the group headers of `roots` to open now: each header
 *  not opened before (`opened`, which it fills). A header is open when it
 *  first shows - the tree's expanded state is kept from its first render,
 *  which is often before the objects arrive, so the headers of the tree
 *  that loads later are opened here; one the person closed stays closed. */
export function groupsToOpen(roots: GroupedNode[], opened: Set<string>): string[] {
  const out: string[] = []
  for (const root of roots) {
    for (const child of root.children) {
      if (isGroupId(child.value) && !opened.has(child.value)) {
        opened.add(child.value)
        out.push(child.value)
      }
    }
  }
  return out
}

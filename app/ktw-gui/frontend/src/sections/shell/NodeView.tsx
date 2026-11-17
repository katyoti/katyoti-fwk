// SPDX-License-Identifier: Apache-2.0
/**
 * NodeView — main panel content when a node is selected and no task is
 * running. Header (breadcrumb + node description) + scoped task gallery.
 *
 * Mounted by MainArea when activeNodeId !== null && runningTaskId === null
 * && pendingTask === null.
 */
import { useMemo } from 'react'
import { Divider, Stack, Text } from '@mantine/core'
import { useAppStore } from '../../store'
import type { ContextNode } from '../../types/project'
import { collectAllTaskNames } from '../../types/project'
import { NodeBreadcrumb } from '../context/NodeBreadcrumb'
import { GalleryTabs } from '../task/GalleryTabs'

function findPath(root: ContextNode, targetId: string): ContextNode[] {
  if (root.id === targetId) return [root]
  for (const child of root.children ?? []) {
    const sub = findPath(child, targetId)
    if (sub.length > 0) return [root, ...sub]
  }
  return []
}

export function NodeView() {
  const activeProjectId = useAppStore((s) => s.activeProjectId)
  const activeNodeId    = useAppStore((s) => s.activeNodeId)
  const projects        = useAppStore((s) => s.projects)
  const selectNode      = useAppStore((s) => s.selectNode)
  const clearNode       = useAppStore((s) => s.clearNode)

  const project = projects.find((p) => p.id === activeProjectId)

  const path = useMemo(() => {
    if (!project || !activeNodeId) return []
    return findPath(project.contextTree, activeNodeId)
  }, [project, activeNodeId])

  const node = path[path.length - 1]
  // Aggregate taskNames from this node + ALL descendants — clicking a
  // parent (workspace / area) shows the union of children's tasks.
  const aggregatedTaskNames = useMemo(
    () => (node ? collectAllTaskNames(node) : []),
    [node]
  )

  if (!node) return null

  return (
    <Stack gap={0} style={{ height: '100%', minHeight: 0 }}>
      {/* Header: breadcrumb + description */}
      <Stack gap="xs" px="md" pt="sm" pb="xs" style={{ flexShrink: 0 }}>
        <NodeBreadcrumb
          path={path}
          onSegmentClick={(id) => {
            if (id === project?.contextTree.id) clearNode()
            else                                 selectNode(id)
          }}
        />
        {node.description && (
          <Text size="xs" c="dimmed">{node.description}</Text>
        )}
      </Stack>

      <Divider />

      {/* Body: tabbed gallery (Tasks scoped to descendants, Skills universe-wide) */}
      <div style={{ flex: 1, minHeight: 0, overflow: 'hidden' }}>
        <GalleryTabs taskNames={aggregatedTaskNames} />
      </div>
    </Stack>
  )
}

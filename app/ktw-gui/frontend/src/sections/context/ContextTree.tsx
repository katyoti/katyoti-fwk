// SPDX-License-Identifier: Apache-2.0
/**
 * ContextTree — renders a project's node hierarchy.
 *
 * Two modes:
 *   - Full tree (default when no node selected, or user toggled): Mantine
 *     `<Tree>` component with expand/collapse + selection.
 *   - Compacted (when activeNodeId is set): just a `<NodeBreadcrumb>` showing
 *     the path to the active node + a button to flip back to full-tree view.
 *
 * The compaction state is local to this component. Selecting a node from
 * the full tree auto-flips to compacted; clicking the maximize button
 * flips back.
 */
import { useEffect, useMemo, useState, type MouseEvent } from 'react'
import {
  ActionIcon,
  Badge,
  Group,
  Stack,
  Text,
  Tree,
  useTree,
  type RenderTreeNodePayload,
  type TreeNodeData,
} from '@mantine/core'
import {
  IconArrowsMaximize,
  IconBookmark,
  IconChevronDown,
  IconChevronRight,
  IconCircleDot,
  IconFolderOpen,
  IconHexagon,
  IconNotes,
  IconStack,
  IconTarget,
} from '@tabler/icons-react'
import { useAppStore } from '../../store'
import type { ContextNode, ContextNodeType } from '../../types/project'
import { NodeBreadcrumb } from './NodeBreadcrumb'
import { useT } from '../../i18n'

// ── Helpers ───────────────────────────────────────────────────────────────

function sortChildren(children: ContextNode[] | undefined): ContextNode[] {
  if (!children) return []
  return [...children].sort((a, b) => (a.order - b.order) || a.name.localeCompare(b.name))
}

/** Walk the tree to find the path (root → target). Returns [] if not found. */
function findPath(root: ContextNode, targetId: string): ContextNode[] {
  if (root.id === targetId) return [root]
  for (const child of sortChildren(root.children)) {
    const sub = findPath(child, targetId)
    if (sub.length > 0) return [root, ...sub]
  }
  return []
}

/** Convert a ContextNode tree → Mantine TreeNodeData (sorted by order). */
function toTreeData(node: ContextNode): TreeNodeData {
  return {
    value: node.id,
    label: node.name,
    nodeProps: { type: node.type, taskCount: node.taskNames?.length ?? 0 },
    children: sortChildren(node.children).map(toTreeData),
  }
}

/** Pick a Tabler icon for the node type. */
function nodeIcon(type: ContextNodeType, size = 14) {
  switch (type) {
    case 'workspace': return <IconStack       size={size} />
    case 'area':      return <IconFolderOpen  size={size} />
    case 'project':   return <IconHexagon     size={size} />
    case 'milestone': return <IconTarget      size={size} />
    case 'task':      return <IconCircleDot   size={size} />
    case 'subtask':   return <IconBookmark    size={size} />
    case 'note':      return <IconNotes       size={size} />
  }
}

// ── Main component ────────────────────────────────────────────────────────

interface ContextTreeProps {
  tree: ContextNode
}

export function ContextTree({ tree }: ContextTreeProps) {
  const t = useT('screens')
  const activeNodeId = useAppStore((s) => s.activeNodeId)
  const selectNode   = useAppStore((s) => s.selectNode)
  const clearNode    = useAppStore((s) => s.clearNode)

  // Auto-compact when activeNodeId becomes non-null. Local state — user can
  // override by clicking the maximize button.
  const [compacted, setCompacted] = useState(activeNodeId !== null)

  // Re-compact whenever the project's tree changes (project switch) and
  // there's an active node — keeps the UX consistent across project hops.
  useEffect(() => {
    setCompacted(activeNodeId !== null)
  }, [tree.id, activeNodeId])

  const path = useMemo(
    () => (activeNodeId ? findPath(tree, activeNodeId) : []),
    [tree, activeNodeId]
  )

  // Hooks for the full-tree branch must run unconditionally (rules of hooks);
  // the conditional return below decides which branch's UI to render.
  const data = useMemo(() => [toTreeData(tree)], [tree])
  const initialExpandedState = useMemo(
    () => Object.fromEntries(path.map((n) => [n.id, true])),
    [path]
  )
  const treeState = useTree({ initialExpandedState })

  if (compacted && path.length > 0) {
    return (
      <Stack gap="xs" px="sm" py="sm">
        <Group justify="space-between" align="center" wrap="nowrap">
          <div style={{ flex: 1, minWidth: 0, overflow: 'hidden' }}>
            <NodeBreadcrumb
              path={path}
              onSegmentClick={(id) => {
                if (id === tree.id) {
                  clearNode()
                } else {
                  selectNode(id)
                }
              }}
            />
          </div>
          <ActionIcon
            size="sm"
            variant="subtle"
            color="gray"
            onClick={() => setCompacted(false)}
            aria-label={t('contextTree.showFull')}
            title={t('contextTree.showFull')}
          >
            <IconArrowsMaximize size={14} />
          </ActionIcon>
        </Group>
      </Stack>
    )
  }

  // Full tree view
  return (
    <Tree
      tree={treeState}
      data={data}
      levelOffset={20}
      expandOnClick={false}
      renderNode={(payload) => (
        <TreeNodeRow
          payload={payload}
          activeNodeId={activeNodeId}
          onSelect={(id) => {
            if (id !== activeNodeId) {
              selectNode(id)
              setCompacted(true)
            }
          }}
        />
      )}
    />
  )
}

// ── Custom row renderer ───────────────────────────────────────────────────

interface TreeNodeRowProps {
  payload:      RenderTreeNodePayload
  activeNodeId: string | null
  onSelect:     (id: string) => void
}

function TreeNodeRow({ payload, activeNodeId, onSelect }: TreeNodeRowProps) {
  const { node, expanded, hasChildren, elementProps, tree: t } = payload
  const tr = useT('screens')
  const nodeProps = (node.nodeProps ?? {}) as { type: ContextNodeType; taskCount: number }
  const isActive = node.value === activeNodeId

  // Chevron toggles expand WITHOUT selecting. Row body selects WITHOUT
  // toggling expand. Splitting the targets is essential — otherwise
  // clicking a parent to open it auto-compacts to the session list view
  // (the auto-compact-on-select behavior in ContextTree) and the user
  // never sees the children.
  const handleChevronClick = (e: MouseEvent) => {
    e.stopPropagation()
    if (expanded) t.collapse(node.value)
    else          t.expand(node.value)
  }

  const handleRowClick = (e: MouseEvent) => {
    e.stopPropagation()
    onSelect(node.value)
  }

  return (
    <Group
      {...elementProps}
      gap={4}
      wrap="nowrap"
      style={{
        ...elementProps.style,
        padding: '4px 6px',
        borderRadius: 4,
        background: isActive ? 'var(--mantine-color-blue-light)' : undefined,
        cursor: 'pointer',
      }}
      onClick={handleRowClick}
    >
      {hasChildren ? (
        <span
          onClick={handleChevronClick}
          style={{
            display: 'inline-flex',
            alignItems: 'center',
            justifyContent: 'center',
            width: 16,
            height: 16,
            cursor: 'pointer',
            borderRadius: 2,
          }}
          role="button"
          aria-label={expanded ? tr('contextTree.collapse') : tr('contextTree.expand')}
          title={expanded ? tr('contextTree.collapse') : tr('contextTree.expand')}
        >
          {expanded
            ? <IconChevronDown  size={12} color="var(--mantine-color-dimmed)" />
            : <IconChevronRight size={12} color="var(--mantine-color-dimmed)" />}
        </span>
      ) : (
        <span style={{ width: 16 }} />
      )}
      {nodeIcon(nodeProps.type, 13)}
      <Text size="sm" fw={isActive ? 600 : 400} truncate style={{ flex: 1 }}>
        {node.label}
      </Text>
      {nodeProps.taskCount > 0 && (
        <Badge size="xs" variant="light" color="gray">
          {nodeProps.taskCount}
        </Badge>
      )}
    </Group>
  )
}

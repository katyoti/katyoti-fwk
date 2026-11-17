// SPDX-License-Identifier: Apache-2.0
/**
 * NodeBreadcrumb — clickable path of ancestors leading to the active node.
 * The last segment (active node) is bold and not clickable.
 *
 * Used by:
 *   - ContextTree (compacted view header)
 *   - NodeView (header)
 */
import { Anchor, Breadcrumbs, Text } from '@mantine/core'
import type { ContextNode } from '../../types/project'

interface NodeBreadcrumbProps {
  /** Ancestors from root → active node (inclusive). */
  path: ContextNode[]
  /** Click handler for any non-last segment. */
  onSegmentClick?: (nodeId: string) => void
}

export function NodeBreadcrumb({ path, onSegmentClick }: NodeBreadcrumbProps) {
  if (path.length === 0) return null

  return (
    <Breadcrumbs separator="›" separatorMargin={6} styles={{ separator: { color: 'var(--mantine-color-dimmed)' } }}>
      {path.map((node, i) => {
        const isLast = i === path.length - 1
        if (isLast) {
          return (
            <Text key={node.id} size="sm" fw={600}>
              {node.name}
            </Text>
          )
        }
        return (
          <Anchor
            key={node.id}
            size="sm"
            c="dimmed"
            onClick={(e) => {
              e.preventDefault()
              onSegmentClick?.(node.id)
            }}
            style={{ cursor: 'pointer' }}
          >
            {node.name}
          </Anchor>
        )
      })}
    </Breadcrumbs>
  )
}

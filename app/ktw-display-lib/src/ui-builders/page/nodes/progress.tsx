// SPDX-License-Identifier: Apache-2.0
// Renderer of the page DSL's `progress` node: a labelled bar
// and « 2 / 5 pages » under it, as the display-panels mock-up draws a
// panel's progress. Pushed: a panel's merge op sets current and total.

import { dataAttrs } from './data-attrs.ts'
import type { ReactNode } from 'react'
import { Group, Progress, Stack, Text } from '@mantine/core'

import { progressCount, progressPercent, type UIProgressNode } from '../progress-log.ts'

export function ProgressRenderer({ node }: { node: UIProgressNode }): ReactNode {
  const state = node.state ?? 'running'
  const count = progressCount(node)
  return (
    <Stack {...dataAttrs(node)} gap={4} data-element={node.id ? `progress:${node.id}` : undefined} data-state={state}>
      {node.label && <Text size="sm" fw={500}>{node.label}</Text>}
      <Progress value={progressPercent(node)} size={8} radius="xl" color={state === 'failed' ? 'red' : state === 'done' ? 'teal' : 'blue'}
        animated={state === 'running' && (node.total ?? 0) <= 0} aria-label={node.label ?? 'progress'} />
      {count && (
        <Group gap="xs"><Text size="sm" c="dimmed" data-guide="progress.count">{count}</Text></Group>
      )}
    </Stack>
  )
}

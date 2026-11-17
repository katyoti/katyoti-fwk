// SPDX-License-Identifier: Apache-2.0
// Renderer of the page DSL's `log` node: the lines a piece
// of work writes as it goes, in a monospace box that follows the newest -
// the last ones kept, a line repeated in a row shown once with « ×3 ».
// Pushed: a panel's append op adds lines to its data.

import { dataAttrs } from './data-attrs.ts'
import { useEffect, useRef, type ReactNode } from 'react'
import { Box, Stack, Text } from '@mantine/core'

import { logRows, type UILogNode } from '../progress-log.ts'

const TONE: Record<string, string | undefined> = { info: undefined, warn: 'orange', error: 'red' }

export function LogRenderer({ node }: { node: UILogNode }): ReactNode {
  const { rows, hidden } = logRows(node.data, node.max ?? 200)
  const box = useRef<HTMLDivElement>(null)
  const last = rows.length + hidden
  // Follow the work: the newest line stays in view as lines come.
  useEffect(() => {
    const el = box.current
    if (el) el.scrollTop = el.scrollHeight
  }, [last])
  return (
    <Stack {...dataAttrs(node)} gap={4} data-element={node.id ? `log:${node.id}` : undefined}>
      {node.title && <Text fw={600} size="sm">{node.title}</Text>}
      <Box ref={box} data-guide="log.lines"
        style={{ fontFamily: 'var(--mantine-font-family-monospace)', fontSize: 12, maxHeight: 300, overflow: 'auto',
          border: '1px solid var(--mantine-color-default-border)', borderRadius: 8, padding: '8px 10px',
          background: 'var(--mantine-color-body)', display: 'flex', flexDirection: 'column', gap: 2 }}>
        {hidden > 0 && <Text size="xs" c="dimmed" ff="monospace">… {hidden} lignes plus anciennes</Text>}
        {rows.length === 0 && <Text size="xs" c="dimmed" ff="monospace">{node.empty ?? '—'}</Text>}
        {rows.map((r, i) => (
          <Text key={i} size="xs" ff="monospace" c={TONE[r.level]} data-level={r.level} style={{ whiteSpace: 'pre-wrap' }}>
            {r.at && <Text span c="dimmed" ff="monospace" size="xs">{r.at} </Text>}
            {r.text}
            {r.count > 1 && <Text span c="dimmed" ff="monospace" size="xs" data-guide="log.count"> ×{r.count}</Text>}
          </Text>
        ))}
      </Box>
    </Stack>
  )
}

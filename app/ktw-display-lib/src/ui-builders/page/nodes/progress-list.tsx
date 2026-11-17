// SPDX-License-Identifier: Apache-2.0
// Renderer of the page DSL's `progress-list` node: one row
// per stage of a run - its label, a bar, « 3/7 », its tone (running blue,
// success teal, failure red) and what it says -, updated as the run reports:
// with `poll`, the run's progress method is called through the host's
// callMethod until the run is over, the rows following each answer, the
// panel never read again. Pushed instead, a panel's merge
// op sets its stages; `layout: rows` draws them as the generic steps panel
// type does - one bordered list, each stage its label and its state in
// words, under one bar for the stages ended (`bar`) - the display-panels
// mock-up's publication and install panels. With `follow`, one more line
// per source its reading syncs, from the `source.sync` events raised on
// each source: its count n/m, then done or failed with why.

import { dataAttrs } from './data-attrs.ts'
import { useEffect, useState, type ReactNode } from 'react'
import { Box, Group, Paper, Progress, Stack, Text } from '@mantine/core'

import { useHostServices } from '../../../ui-core/host/index.tsx'
import { applySourceSync, pollProgress, progressEnded, progressOver, progressRow, sourceLines, type UIProgressListNode, type UIProgressStage } from '../ui-page.interactive.ts'

const seconds = (ms?: number) => (ms === undefined ? '' : ms < 1000 ? `${ms} ms` : `${Math.round(ms / 100) / 10} s`)

/** A stage's state in words and its colour, in the rows layout. */
export const STAGE_WORDS: Record<string, { label: string; color: string }> = {
  done: { label: 'terminé', color: 'teal' },
  running: { label: 'en cours…', color: 'orange' },
  waiting: { label: 'en attente', color: 'gray' },
  skipped: { label: 'ignoré', color: 'dimmed' },
  failed: { label: 'échec', color: 'red' },
}

export function ProgressListRenderer({ node }: { node: UIProgressListNode }): ReactNode {
  const { callMethod, followEvents } = useHostServices()
  const [polled, setStages] = useState<UIProgressStage[]>(node.stages ?? [])
  const [error, setError] = useState('')
  const key = JSON.stringify(node.stages ?? [])
  const [seen, setSeen] = useState(key)
  // What the producer sends again replaces the rows (a panel re-read).
  if (seen !== key) { setSeen(key); setStages(node.stages ?? []) }

  const pollKey = JSON.stringify(node.poll ?? null)
  useEffect(() => {
    const poll = node.poll
    if (!poll || !callMethod) return
    let stop = false
    let current = node.stages ?? []
    if (progressOver(undefined, current)) return
    let timer: ReturnType<typeof setTimeout> | undefined
    const tick = async () => {
      const got = await pollProgress(callMethod, poll, current)
      if (stop) return
      current = got.stages
      setStages(got.stages)
      setError(got.error ?? '')
      if (!got.over) timer = setTimeout(() => { void tick() }, poll.every ?? 1000)
    }
    void tick()
    return () => { stop = true; if (timer) clearTimeout(timer) }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pollKey, callMethod])

  // The run's per-source lines: each source.sync event
  // folded into its source's line, as it comes.
  const followKey = JSON.stringify(node.follow ?? null)
  const [lines, setLines] = useState<UIProgressStage[]>(() => sourceLines(node.follow))
  useEffect(() => {
    const follow = node.follow
    setLines(sourceLines(follow))
    if (!follow || !followEvents) return
    return followEvents(follow.event, (payload) => setLines((ls) => applySourceSync(ls, payload, follow)))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [followKey, followEvents])
  const stages = lines.length > 0 ? [...polled, ...lines] : polled

  const { ended, total } = progressEnded(stages)
  const bar = node.bar && total > 0 && (
    <Stack gap={4} data-guide="progress.bar">
      <Progress value={Math.round((ended / total) * 100)} size={8} radius="xl" aria-label="progress" />
      <Text size="sm" c="dimmed" data-guide="progress.count">{ended} / {total}{node.unit ? ` ${node.unit}` : ''}</Text>
    </Stack>
  )
  if (node.layout === 'rows') {
    return (
      <Stack {...dataAttrs(node)} gap="sm" data-element={node.id ? `progress-list:${node.id}` : undefined} data-layout="rows">
        {node.title && <Text fw={600} size="sm">{node.title}</Text>}
        {bar}
        {stages.length === 0 && <Text size="sm" c="dimmed">{node.empty ?? '—'}</Text>}
        {stages.length > 0 && (
          <Paper withBorder radius="md" style={{ overflow: 'hidden' }}>
            {stages.map((s, i) => {
              const row = progressRow(s)
              const words = STAGE_WORDS[row.state] ?? STAGE_WORDS.waiting
              return (
                <Box key={s.id} px="sm" py={7} data-guide="progress.stage" data-guide-name={s.id} data-state={row.state}
                  style={i < stages.length - 1 ? { borderBottom: '1px solid var(--mantine-color-default-border)' } : undefined}>
                  <Group justify="space-between" gap="sm" wrap="nowrap">
                    <Text size="sm">{s.label}</Text>
                    <Group gap="xs" wrap="nowrap">
                      {row.count && <Text size="xs" c="dimmed" ff="monospace">{row.count}</Text>}
                      <Text size="xs" ff="monospace" c={words.color}>{words.label}</Text>
                    </Group>
                  </Group>
                  {s.message && <Text size="xs" c={row.state === 'failed' ? 'red' : 'dimmed'} mt={2}>{s.message}</Text>}
                </Box>
              )
            })}
          </Paper>
        )}
        {error && <Text size="xs" c="red">{error}</Text>}
      </Stack>
    )
  }
  return (
    <Stack {...dataAttrs(node)} gap="xs" data-element={node.id ? `progress-list:${node.id}` : undefined}>
      {node.title && <Text fw={600} size="sm">{node.title}</Text>}
      {bar}
      {stages.length === 0 && <Text size="sm" c="dimmed">{node.empty ?? '—'}</Text>}
      {stages.map((s) => {
        const row = progressRow(s)
        return (
          <Paper key={s.id} withBorder p="xs" radius="sm" data-guide="progress.stage" data-guide-name={s.id} data-state={row.state}>
            <Group justify="space-between" gap="xs" wrap="nowrap">
              <Text size="sm" fw={500}>{s.label}</Text>
              <Group gap="xs" wrap="nowrap">
                {row.count && <Text size="xs" c="dimmed">{row.count}</Text>}
                {s.ms !== undefined && <Text size="xs" c="dimmed">{seconds(s.ms)}</Text>}
              </Group>
            </Group>
            <Progress value={row.percent} color={row.tone} size="sm" mt={4} animated={s.state === 'running'} />
            {s.message && <Text size="xs" c={row.tone === 'red' ? 'red' : 'dimmed'} mt={2}>{s.message}</Text>}
          </Paper>
        )
      })}
      {error && <Text size="xs" c="red">{error}</Text>}
    </Stack>
  )
}

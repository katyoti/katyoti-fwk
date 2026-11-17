// SPDX-License-Identifier: Apache-2.0
// A grouped list: a list whose node names `groupBy`. Its
// header counts each group - and the overdue elements when it names an
// overdue field -, a click on a count shows only that group and a second
// click shows all; the elements are drawn under their group's heading, or
// under the headings of `sectionBy` (counted by kind, drawn by holder - each
// heading kept under a filter with « Nothing of this kind. », a heading for
// some groups alone drawn only with them), or in their own order with
// `sections: false`; the header's words and the headings' counts are the
// list's to set; with `bulk` actions, a tick on each element
// selects several of one group and each action runs once per element ticked
// it applies to - one answer each. With `filterStyle: tiles` the counts are
// KPI tiles (each group's icon, colour and hint; `allLabel` a first tile
// counting everything), with `sectionStyle: card` each heading a bordered
// card « <label> — <n> », and `placeField` names each element's place in a
// dimmed line when it changes. What it computes is in
// grouped-list.ts.

import { dataAttrs } from './data-attrs.ts'
import { useMemo, useState } from 'react'
import { Badge, Box, Button, Checkbox, Group, Paper, SimpleGrid, Stack, Text, UnstyledButton } from '@mantine/core'
import i18next from 'i18next'
import { IconPointFilled } from '@tabler/icons-react'

import { tablerIcon } from '../../../ui-core/components/icons/index.ts'
import { KpiTile } from '../../../ui-core/components/other/index.ts'
import {
  bulkEvents, bulkLabel, bulkSkipNote, countLook, emptySectionNote, filterNote, groupCounts, groupItems, groupTiles, NO_SELECTION,
  sectionLook, sectionTick, selectedItems, selectGroup, toggleFilter, toggleSelected, visibleSections, withPlaces,
  type BulkSelection, type GroupedItem,
} from '../grouped-list.ts'
import { actionVerb, listItemNode, type UIListNode } from '../ui-page.interactive.ts'
import { useRunAction } from '../page-context.tsx'
import type { RenderNode } from './layout.tsx'
import { actionLabel, ListCell, useRun } from './interactive.tsx'
import { ShowMore, useWindow } from './window.tsx'

export function GroupedListRenderer({ node, render }: { node: UIListNode; render: RenderNode }) {
  const items = Array.isArray(node.data) ? node.data : []
  const groups = useMemo(() => groupItems(items, node.groupBy ?? '', node.groups), [items, node.groupBy, node.groups])
  const [filter, setFilter] = useState<string | null>(null)
  const [sel, setSel] = useState<BulkSelection>(NO_SELECTION)
  const run = useRun()
  const open = useRunAction()(node.open)
  const lang = i18next.language
  const shown = visibleSections(groups, filter, node)
  const total = shown.reduce((n, g) => n + g.items.length, 0)
  // On a progressive page only a page of the elements shown is drawn.
  const win = useWindow(total, node.pageSize)
  const item = listItemNode(node)
  const id = node.id ? `list:${node.id}` : undefined
  if (items.length === 0 || !item) {
    return <Text {...dataAttrs(node)} size="sm" c="dimmed" data-element={id}>{node.empty ?? 'Nothing to show.'}</Text>
  }
  const counts = groupCounts(groups, node, lang)
  const filterLabel = filter === null ? null : counts.find((c) => c.key === filter)?.label ?? filter
  const bulk = node.bulk ?? []
  const picked = selectedItems(groups, sel)

  // A tick selects in the element's own group: a bulk answer is for one kind.
  const tick = (it: GroupedItem) => bulk.length === 0 ? undefined : (
    <Checkbox size="xs" aria-label={`select ${it.key}`} checked={sel.group === it.group && sel.keys.includes(it.key)}
      data-select={it.key} onClick={(e) => e.stopPropagation()}
      onChange={() => setSel((s) => toggleSelected(s, it.group, it.key))} />
  )
  let left = win.shown
  const cellsOf = (its: GroupedItem[]) => {
    const take = its.slice(0, Math.max(0, left))
    left -= take.length
    // An element's place, named in a dimmed line when it changes (placeField).
    return withPlaces(take, node.placeField, node.places).flatMap(({ place, item: it }) => [
      ...(place !== undefined ? [<Text key={`place:${it.key}`} size="xs" fw={600} c="dimmed" data-place={place}>{place}</Text>] : []),
      <ListCell key={it.key} node={node} render={render} item={item} el={it.el} index={it.index} open={open} lead={tick(it)} />,
    ])
  }
  const gap = node.gap ?? 'sm'
  const body = node.sections === false
    ? <Stack gap={gap}>{shown.flatMap((g) => cellsOf(g.items))}</Stack>
    : shown.map((g) => {
      if (left <= 0 && g.items.length > 0) return null
      const in_ = bulk.length > 0 ? sectionTick(g, filter, node) : null
      const mine = in_ === null ? [] : g.items.filter((it) => it.group === in_)
      const all = in_ !== null && sel.group === in_ && mine.length > 0 && mine.every((it) => sel.keys.includes(it.key))
      const ticker = in_ !== null && mine.length > 0 && (
        <Checkbox size="xs" aria-label={`select ${g.label}`} checked={all} data-select-group={g.key}
          onChange={() => setSel((s) => selectGroup(s, in_, mine))} />
      )
      if (node.sectionStyle === 'card') {
        // A bordered card titled « <label> — <n> », its group's icon, colour and hint.
        const look = sectionLook(g.key, node.groups)
        return (
          <Paper key={g.key} withBorder p="lg" radius="md" data-group={g.key}>
            <Stack gap="sm">
              <Group gap="xs" align="center" wrap="nowrap" data-group-heading={g.key}>
                {ticker}
                {render({ kind: 'heading', text: `${g.label} — ${g.items.length}`, level: 5, icon: look.icon, color: look.color } as never)}
              </Group>
              {look.hint && <Text size="xs" c="dimmed">{look.hint}</Text>}
              <Stack gap={gap}>
                {g.items.length === 0
                  ? <Text size="sm" c="dimmed" data-empty-section={g.key}>{emptySectionNote(lang)}</Text>
                  : cellsOf(g.items)}
              </Stack>
            </Stack>
          </Paper>
        )
      }
      return (
        <Stack key={g.key} gap={gap} data-group={g.key}>
          <Group gap="xs" align="center" data-group-heading={g.key}>
            {ticker}
            <Text size="xs" fw={700} c="dimmed" tt="uppercase">{g.label}</Text>
            {node.sectionCounts !== false && <Badge size="xs" variant="light" color="gray">{g.items.length}</Badge>}
          </Group>
          {g.items.length === 0
            ? <Text size="sm" c="dimmed" data-empty-section={g.key}>{emptySectionNote(lang)}</Text>
            : cellsOf(g.items)}
        </Stack>
      )
    })

  return (
    <Stack {...dataAttrs(node)} gap="sm" data-element={id}>
      <Stack gap={4}>
        {node.filterStyle === 'tiles' ? (
          // The counts as KPI tiles: a click filters as a count does; the
          // first, allLabel's, counts everything and shows all.
          <SimpleGrid cols={{ base: 2, sm: 3, md: Math.min(6, counts.length + (node.allLabel ? 1 : 0)) }} spacing="md"
            role="group" aria-label={node.filterLabel || 'Filter by group'}>
            {node.allLabel && (
              <UnstyledButton aria-pressed={filter === null} data-group-all={items.length}
                onClick={() => { setFilter(null); setSel(NO_SELECTION) }} style={{ display: 'block', width: '100%' }}>
                <KpiTile label={node.allLabel} value={items.length} color="indigo" icon="list-check" selected={filter === null} />
              </UnstyledButton>
            )}
            {groupTiles(counts, node.groups).map((c) => (
              <UnstyledButton key={c.key} aria-pressed={filter === c.key} data-group-filter={c.key} data-count={c.count}
                onClick={() => { setFilter((f) => toggleFilter(f, c.key)); setSel(NO_SELECTION) }} style={{ display: 'block', width: '100%' }}>
                <KpiTile label={c.label} value={c.count} description={c.hint} color={c.color} icon={c.icon} selected={filter === c.key} />
              </UnstyledButton>
            ))}
          </SimpleGrid>
        ) : (
        <Group gap={6} wrap="wrap" role="group" aria-label={node.filterLabel || 'Filter by group'}>
          {counts.map((c) => (
            // Every count alike - the overdue one too -, the active one filled.
            <Button key={c.key} size="compact-sm" {...countLook(filter, c.key)}
              aria-pressed={filter === c.key} data-group-filter={c.key} data-count={c.count}
              onClick={() => { setFilter((f) => toggleFilter(f, c.key)); setSel(NO_SELECTION) }}
              rightSection={<Text span size="sm" fw={700}>{c.count}</Text>}>
              {c.label}
            </Button>
          ))}
        </Group>
        )}
        <Text size="xs" c="dimmed">{filterNote(lang, filterLabel, node.filterHint)}</Text>
      </Stack>
      {picked.length > 0 && (
        <Paper withBorder p="xs" radius="md" data-bulk={sel.group ?? ''}>
          <Group gap="sm" justify="space-between" wrap="wrap">
            <Text size="sm" fw={600}>{bulkLabel(lang, picked.length)}</Text>
            <Group gap={6}>
              {bulk.map((a, i) => {
                const Icon = a.icon ? tablerIcon(a.icon, IconPointFilled) : undefined
                // The elements where the action is hidden or disabled are skipped, and said.
                const plan = bulkEvents(a, picked)
                const skip = plan.skipped.length
                return (
                  <Group key={a.id ?? i} gap={4} wrap="nowrap">
                    <Button size="compact-sm" variant="light" color={a.dangerous ? 'red' : a.color}
                      leftSection={Icon ? <Icon size={12} /> : undefined}
                      disabled={!run || !actionVerb(a) || plan.events.length === 0}
                      data-bulk-action={a.id ?? a.uri ?? String(i)} data-bulk-runs={plan.events.length}
                      onClick={() => {
                        // One run per element ticked it applies to: each answer is its own.
                        for (const ev of plan.events) run?.(ev.action, ev.item, ev.extra)
                        setSel(NO_SELECTION)
                      }}>
                      {actionLabel(a.label, undefined) || a.id || 'Run'}
                    </Button>
                    {skip > 0 && (
                      <Text size="xs" c="dimmed" data-bulk-skipped={skip}>{bulkSkipNote(lang, skip)}</Text>
                    )}
                  </Group>
                )
              })}
              <Button size="compact-sm" variant="subtle" color="gray" onClick={() => setSel(NO_SELECTION)} data-bulk-clear>
                ×
              </Button>
            </Group>
          </Group>
        </Paper>
      )}
      {node.sections === false ? body : <Stack gap="md">{body}</Stack>}
      <Box><ShowMore rest={total - win.shown} onMore={win.more} name={node.id} /></Box>
    </Stack>
  )
}

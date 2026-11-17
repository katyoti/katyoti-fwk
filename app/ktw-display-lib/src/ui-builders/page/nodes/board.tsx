// SPDX-License-Identifier: Apache-2.0
// Renderer of the page DSL's `board` node: a list of items
// beside the selected item's panel. Each row shows its icon, label, count,
// chips and - when the board declares `toggle` - a switch; the head shows the
// title and how many rows. A click on a row sets the page selection's key
// (the first row is selected while none is), and the panel - usually a section
// loading with `${selection.<key>}` - is drawn beside. Generic: the rows are data.
// With `layout: cards` and a host showing the source model, the
// rows are a grid of cards - icon, label, state, meta line, the first items it
// picks, a remark, « Open… », its actions and the switch -, and an opened card
// draws the panel in place of the grid with a way back.

import { dataAttrs } from './data-attrs.ts'
import { useEffect, useState } from 'react'
import { Alert, Badge, Box, Button, Group, Paper, Stack, Switch, Text, ThemeIcon, UnstyledButton } from '@mantine/core'
import { IconPointFilled } from '@tabler/icons-react'

import { tablerIcon } from '../../../ui-core/components/icons/index.ts'
import { useSourceModel } from '../../../ui-core/host/index.tsx'
import { usePageContext, useRunAction, useSelection } from '../page-context.tsx'
import { boardCardItems, boardMoreText, boardRows, boardSelected, boardToggleEvent, type UIBoardNode, type UIBoardRow } from '../ui-page.interactive.ts'
import { ActionButtons } from './interactive.tsx'
import type { RenderNode } from './layout.tsx'

function chipOf(c: UIBoardRow['chips'] extends Array<infer T> | undefined ? T : never): { text: string; color?: string } {
  return typeof c === 'string' ? { text: c } : c
}

export function BoardRenderer({ node, render }: { node: UIBoardNode; render: RenderNode }) {
  const sourceModel = useSourceModel()
  if (node.layout === 'cards' && sourceModel) return <BoardCards node={node} render={render} />
  return <BoardList node={node} render={render} />
}

function BoardSwitch({ node, row }: { node: UIBoardNode; row: UIBoardRow }) {
  const { onAction } = usePageContext()
  const toggle = useRunAction()(node.toggle)
  if (!node.toggle) return null
  return (
    <Switch size="xs" checked={!!row.on} disabled={!toggle || !onAction} aria-label={row.label ?? row.key}
      onClick={(e) => e.stopPropagation()}
      onChange={() => { const ev = boardToggleEvent(node.toggle!, row); onAction?.(ev) }} />
  )
}

/** BoardCards keeps which card is open: none, the grid; one, its panel. */
function BoardCards({ node, render }: { node: UIBoardNode; render: RenderNode }) {
  const { select } = useSelection()
  const [opened, setOpened] = useState<string>()
  return <BoardCardsView node={node} render={render} opened={opened}
    onOpen={(key) => { select(node.select, key); setOpened(key) }} onBack={() => setOpened(undefined)} />
}

/** BoardCardsView draws a cards board: the grid of cards, or - `opened` the
 *  key of a row it holds - that card's panel in place of the grid, under a
 *  way back. */
export function BoardCardsView({ node, render, opened, onOpen, onBack }: {
  node: UIBoardNode; render: RenderNode; opened?: string; onOpen: (key: string) => void; onBack: () => void
}) {
  const rows = boardRows(node.data)
  const open = opened !== undefined ? rows.find((r) => r.key === opened) : undefined
  if (open) {
    return (
      <Stack {...dataAttrs(node)} gap="sm" data-element={node.id ? `board:${node.id}` : undefined} data-opened={open.key}>
        <Group gap="sm">
          <Button size="compact-sm" variant="subtle" onClick={onBack} data-guide="board.back">{node.backLabel ?? '← All'}</Button>
          <Text size="sm" fw={600}>{open.label ?? open.key}</Text>
        </Group>
        <Box>{render(node.panel, 'panel')}</Box>
      </Stack>
    )
  }
  return (
    <Stack {...dataAttrs(node)} gap="xs" data-element={node.id ? `board:${node.id}` : undefined}>
      {node.title ? (
        <Group gap={6}>
          <Text size="xs" fw={700} tt="uppercase" c="dimmed">{node.title}</Text>
          <Badge size="xs" variant="light" color="gray">{rows.length}</Badge>
        </Group>
      ) : null}
      {rows.length === 0 ? (
        <Text size="sm" c="dimmed">{node.empty ?? 'Nothing to show.'}</Text>
      ) : (
        <Box style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(280px, 1fr))', gap: 12 }}>
          {rows.map((r) => <BoardCard key={r.key} node={node} row={r} onOpen={onOpen} />)}
        </Box>
      )}
    </Stack>
  )
}

function BoardCard({ node, row, onOpen }: { node: UIBoardNode; row: UIBoardRow; onOpen: (key: string) => void }) {
  const Icon = row.icon ? tablerIcon(row.icon, IconPointFilled) : undefined
  const state = row.state === undefined || row.state === '' ? undefined : chipOf(row.state)
  const { shown, more } = boardCardItems(row.items, node.itemsMax)
  return (
    <Paper withBorder radius="md" p="sm" data-guide="board.card" data-guide-name={row.key}
      style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
      <Group justify="space-between" wrap="nowrap" gap={8}>
        <UnstyledButton onClick={() => onOpen(row.key)} style={{ minWidth: 0 }} data-guide="board.card-title">
          <Group gap={8} wrap="nowrap">
            {Icon ? <ThemeIcon size={24} radius="sm" color={row.color ?? 'gray'}><Icon size={14} /></ThemeIcon> : null}
            <Text size="sm" fw={700} truncate>{row.label ?? row.key}</Text>
          </Group>
        </UnstyledButton>
        <Group gap={6} wrap="nowrap">
          {state ? <Badge size="sm" variant="light" color={state.color ?? 'gray'} data-guide="board.card-state">{state.text}</Badge> : null}
          <BoardSwitch node={node} row={row} />
        </Group>
      </Group>
      {row.meta ? <Text size="xs" c="dimmed">{row.meta}</Text> : null}
      {(row.chips ?? []).length > 0 ? (
        <Group gap={4}>
          {(row.chips ?? []).map((c, i) => {
            const chip = chipOf(c)
            return <Badge key={i} size="xs" variant="light" color={chip.color ?? 'gray'}>{chip.text}</Badge>
          })}
        </Group>
      ) : null}
      {shown.length > 0 ? (
        <Box component="ul" m={0} pl={18} style={{ display: 'flex', flexDirection: 'column', gap: 3 }} data-guide="board.card-items">
          {shown.map((t, i) => <Text key={i} component="li" size="sm">{t}</Text>)}
          {more > 0 ? <Text component="li" size="sm" c="dimmed" style={{ listStyle: 'none' }}>{boardMoreText(more, node.moreLabel)}</Text> : null}
        </Box>
      ) : null}
      {row.note ? <Alert color={row.noteColor ?? 'yellow'} variant="light" p="xs" data-guide="board.card-note"><Text size="xs">{row.note}</Text></Alert> : null}
      <Group gap={4} wrap="wrap">
        <Button size="compact-xs" variant="light" onClick={() => onOpen(row.key)} data-guide="board.open">{row.openLabel ?? node.openLabel ?? 'Open…'}</Button>
        <ActionButtons actions={node.actions} item={row} />
      </Group>
    </Paper>
  )
}

function BoardList({ node, render }: { node: UIBoardNode; render: RenderNode }) {
  const rows = boardRows(node.data)
  const { selection, select } = useSelection()
  const current = boardSelected(rows, selection, node.select)

  // The first row is selected while none is: the panel beside shows it.
  const unset = selection[node.select] === undefined || selection[node.select] === null || selection[node.select] === ''
  useEffect(() => {
    if (unset && current !== undefined) select(node.select, current)
  }, [unset, current, node.select, select])

  const list = (
    <Paper withBorder radius="md" p="xs" style={{ width: node.listWidth ?? 280, maxWidth: '100%', flex: '0 0 auto' }}
      data-element={node.id ? `board:${node.id}` : undefined}>
      <Group justify="space-between" mb={6} px={4}>
        <Text size="xs" fw={700} tt="uppercase" c="dimmed">{node.title ?? ''}</Text>
        <Badge size="xs" variant="light" color="gray">{rows.length}</Badge>
      </Group>
      {rows.length === 0 ? (
        <Text size="sm" c="dimmed" px={4}>{node.empty ?? 'Nothing to show.'}</Text>
      ) : (
        <Stack gap={2}>
          {rows.map((r) => {
            const Icon = r.icon ? tablerIcon(r.icon, IconPointFilled) : undefined
            const selected = r.key === current
            return (
              <UnstyledButton key={r.key} onClick={() => select(node.select, r.key)}
                aria-current={selected ? 'true' : undefined} data-guide="board.row" data-guide-name={r.key}
                style={{ display: 'block', width: '100%', borderRadius: 6, padding: '6px 8px',
                  background: selected ? 'var(--mantine-color-indigo-light)' : undefined }}>
                <Group gap={8} wrap="nowrap">
                  {Icon ? <Icon size={16} /> : null}
                  <Box style={{ flex: 1, minWidth: 0 }}>
                    <Group gap={6} wrap="wrap">
                      <Text size="sm" fw={selected ? 600 : 500} truncate>{r.label ?? r.key}</Text>
                      {(r.chips ?? []).map((c, i) => {
                        const chip = chipOf(c)
                        return <Badge key={i} size="xs" variant="light" color={chip.color ?? 'gray'}>{chip.text}</Badge>
                      })}
                    </Group>
                  </Box>
                  {r.count !== undefined && r.count !== '' ? <Text size="xs" c="dimmed">{String(r.count)}</Text> : null}
                  <BoardSwitch node={node} row={r} />
                </Group>
              </UnstyledButton>
            )
          })}
        </Stack>
      )}
    </Paper>
  )
  return (
    <Box {...dataAttrs(node)} style={{ display: 'flex', flexWrap: 'wrap', gap: 'var(--mantine-spacing-md)', alignItems: 'flex-start' }}>
      {list}
      <Box style={{ flex: '1 1 360px', minWidth: 0 }}>{current !== undefined ? render(node.panel, 'panel') : null}</Box>
    </Box>
  )
}

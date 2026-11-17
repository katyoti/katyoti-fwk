// SPDX-License-Identifier: Apache-2.0
// The one object card: an object drawn as a
// card is filled from its CWM row - its title, id and type, its lifecycle
// state, the badges its type and capabilities declare, one next action and a
// « ⋯ » menu. Producers pass the row, never nodes of their own (a compose rule); every action of the card runs on the row.

import { Badge, Button, Card, Group, Stack, Text, Tooltip } from '@mantine/core'
import { IconPointFilled } from '@tabler/icons-react'

import { tablerIcon } from '../../../ui-core/components/icons/index.ts'
import { ItemContext, useRunAction } from '../page-context.tsx'
import {
  menuItemAction, objectCardRow,
  type UIObjectCardBadge, type UIObjectCardNode,
} from '../ui-page.interactive.ts'
import { dataAttrs } from './data-attrs.ts'
import { MenuRenderer } from './choice.tsx'
import { actionLabel, actionShown } from './interactive.tsx'

function CardBadge({ badge, element }: { badge: UIObjectCardBadge; element: string }) {
  const Icon = badge.icon ? tablerIcon(badge.icon, IconPointFilled) : undefined
  const b = (
    <Badge size="sm" variant="light" color={badge.color || 'gray'} leftSection={Icon ? <Icon size={10} /> : undefined}
      data-element={element}>
      {badge.label}
    </Badge>
  )
  return badge.tooltip ? <Tooltip label={badge.tooltip} withArrow>{b}</Tooltip> : b
}

export function ObjectCardRenderer({ node }: { node: UIObjectCardNode }) {
  const runOf = useRunAction()
  const row = objectCardRow(node.row)
  if (!row) return null
  const open = runOf(node.open)
  const next = row.next && actionShown({ when: row.next.when }, row) ? row.next : undefined
  const runNext = next ? runOf(menuItemAction(next)) : undefined
  const menu = (row.menu ?? []).filter((e) => actionShown({ when: e.when }, row))
  const TypeIcon = row.icon ? tablerIcon(row.icon, IconPointFilled) : undefined
  const NextIcon = next?.icon ? tablerIcon(next.icon, IconPointFilled) : undefined
  return (
    <ItemContext.Provider value={row}>
      <Card {...dataAttrs(node)} withBorder radius="md" p="sm" data-element={`card:${row.id}`} data-object-type={row.type}
        style={open ? { cursor: 'pointer' } : undefined}
        onClick={open ? () => open(row) : undefined}>
        <Stack gap={6}>
          <Group justify="space-between" wrap="nowrap" gap="xs" align="flex-start">
            <Group gap={6} wrap="nowrap" style={{ minWidth: 0 }}>
              {TypeIcon && <TypeIcon size={16} />}
              <Text fw={600} size="sm" lineClamp={2} data-element={`card-title:${row.id}`}>{row.title}</Text>
            </Group>
            {menu.length > 0 && (
              <span data-element={`card-menu:${row.id}`}>
                <MenuRenderer node={{ kind: 'menu', id: `card-menu:${row.id}`, items: menu }} />
              </span>
            )}
          </Group>
          <Group gap={6} wrap="wrap">
            <Text size="xs" c="dimmed" ff="monospace">{row.id}</Text>
            {row.state && <CardBadge badge={row.state} element={`card-state:${row.id}`} />}
            {(row.badges ?? []).map((b, i) => <CardBadge key={b.id ?? `${b.label}-${i}`} badge={b} element={`card-badge:${row.id}:${b.id ?? i}`} />)}
          </Group>
          {next && (
            <Group justify="flex-end">
              <Button size="compact-xs" variant="light" disabled={!runNext}
                leftSection={NextIcon ? <NextIcon size={12} /> : undefined}
                onClick={(e) => { e.stopPropagation(); runNext?.(row) }}
                data-element={`card-next:${row.id}`}>
                {actionLabel(next.label, row)}
              </Button>
            </Group>
          )}
        </Stack>
      </Card>
    </ItemContext.Provider>
  )
}

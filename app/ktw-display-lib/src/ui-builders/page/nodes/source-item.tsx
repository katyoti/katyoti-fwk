// SPDX-License-Identifier: Apache-2.0
// One item of a source drawn from its roles: the same item
// card wherever a source's item shows - a search hit, a browsed level's row,
// a held selection's row, a summary - read from the item's roles (key,
// title, kind, path, detail, why, badges, attrs, modified),
// never a connector's name. Each size draws the roles it needs; an item kind
// the source declares adds its slots (one attribute each, with its label) as
// data. What each size draws is itemCardParts', tested apart from rendering.

import type { ReactNode } from 'react'
import { Badge, Group, Stack, Text, Tooltip } from '@mantine/core'

import type { UIItemCardSize, UIItemKinds, UIPickerBadge, UIPickerItem } from '../ui-page.interactive.ts'

/** The roles an item card draws beside its title. */
export type ItemCardRole = 'kind' | 'path' | 'detail' | 'why' | 'badges' | 'slots' | 'modified'

/** The roles each size draws: a search hit says what and where it is and why
 *  it was found; a browsed row only what it is (the path is the one walked);
 *  a selection row where it is and why it was held; a summary every role. */
export const ITEM_CARD_ROLES: Record<UIItemCardSize, ItemCardRole[]> = {
  search: ['kind', 'path', 'why', 'badges', 'slots'],
  browse: ['kind', 'badges', 'slots'],
  selection: ['path', 'why', 'badges', 'slots'],
  summary: ['kind', 'path', 'detail', 'why', 'badges', 'slots', 'modified'],
}

/** The words an item card shows. */
export const ITEM_CARD_LABELS = { modified: 'modifié le' }

/** What an item card draws at a size. */
export interface ItemCardParts {
  title: string
  kind?: string
  path?: string[]
  detail?: string
  why?: string
  badges: UIPickerBadge[]
  slots: { attr: string; label: string; value: string }[]
  modified?: string
}

/** modifiedDay is the day an RFC 3339 instant falls on, as dd/mm/yyyy -
 *  the text itself when it is no date. */
export function modifiedDay(at: string): string {
  const m = /^(\d{4})-(\d{2})-(\d{2})/.exec(at)
  return m ? `${m[3]}/${m[2]}/${m[1]}` : at
}

/** itemCardParts is what an item's card draws at a size: the roles the size
 *  draws that the item carries; its kind's label and slots as the source
 *  declares them (a slot drawn when the item has its attribute and the slot
 *  names the size, or names none). A why line is drawn only with no state - a
 *  state's why is that state's tooltip. `nested`: the row already sits under
 *  its path, which is not repeated. */
export function itemCardParts(item: UIPickerItem, size: UIItemCardSize, opts: {
  kinds?: UIItemKinds
  badges?: Record<string, UIPickerBadge>
  nested?: boolean
} = {}): ItemCardParts {
  const roles = new Set(ITEM_CARD_ROLES[size])
  const decl = item.kind ? opts.kinds?.[item.kind] : undefined
  const parts: ItemCardParts = { title: item.title, badges: [], slots: [] }
  if (roles.has('kind') && item.kind) parts.kind = decl?.label || item.kind
  if (roles.has('path') && !opts.nested && item.path?.length) parts.path = item.path
  if (roles.has('detail') && item.detail) parts.detail = item.detail
  if (roles.has('why') && item.why && !item.state) parts.why = item.why
  if (roles.has('badges')) parts.badges = (item.badges ?? []).map((b) => opts.badges?.[b] ?? { label: b })
  if (roles.has('slots')) {
    for (const s of decl?.slots ?? []) {
      const value = item.attrs?.[s.attr]
      if (!value || (s.sizes && !s.sizes.includes(size))) continue
      parts.slots.push({ attr: s.attr, label: s.label || s.attr, value })
    }
  }
  if (roles.has('modified') && item.modified) parts.modified = item.modified
  return parts
}

/** SourceItem draws an item of a source at a size: its title (struck through
 *  when `off`, dimmed when unreachable or `dim`), then on its line its kind,
 *  its slots, its badges and the caller's marks (`extra`: a state, « déjà
 *  retenue »), then where it is, one line on it, why it was found and when it
 *  last changed. `guide` names its lines' data-guide (`picker.why`). */
export function SourceItem({ item, size, kinds, badges, nested, off, dim, extra, guide = 'item-card' }: {
  item: UIPickerItem
  size: UIItemCardSize
  kinds?: UIItemKinds
  badges?: Record<string, UIPickerBadge>
  nested?: boolean
  off?: boolean
  dim?: boolean
  extra?: ReactNode
  guide?: string
}): ReactNode {
  const p = itemCardParts(item, size, { kinds, badges, nested })
  const head = (
    <Group gap={6} wrap="nowrap">
      <Text size="sm" c={off || dim || item.unreachable ? 'dimmed' : undefined} td={off ? 'line-through' : undefined}>{p.title}</Text>
      {p.kind && <Badge size="xs" variant="outline" color="gray" tt="none" data-guide={`${guide}.kind`}>{p.kind}</Badge>}
      {p.slots.map((s) => (
        <Tooltip key={s.attr} label={s.label}>
          <Badge size="xs" variant="light" color="indigo" tt="none" data-guide={`${guide}.slot`} data-guide-name={s.attr}>{s.value}</Badge>
        </Tooltip>
      ))}
      {p.badges.map((b) => {
        const chip = <Badge key={b.label} size="xs" variant="light" color={b.color ?? 'gray'}>{b.label}</Badge>
        return b.description ? <Tooltip key={b.label} label={b.description}>{chip}</Tooltip> : chip
      })}
      {extra}
    </Group>
  )
  const lines = p.path || p.detail || p.why || p.modified
  return (
    <Stack gap={0} data-guide={guide === 'item-card' ? 'item-card' : `${guide}.item-card`} data-size={size} data-kind={item.kind}>
      {head}
      {lines && (
        <>
          {p.path && <Text size="xs" c="dimmed" data-guide={`${guide}.path`}>{p.path.join(' › ')}</Text>}
          {p.detail && <Text size="xs" data-guide={`${guide}.detail`}>{p.detail}</Text>}
          {p.why && <Text size="xs" c="dimmed" data-guide={`${guide}.why`}>{p.why}</Text>}
          {p.modified && <Text size="xs" c="dimmed" data-guide={`${guide}.modified`}>{`${ITEM_CARD_LABELS.modified} ${modifiedDay(p.modified)}`}</Text>}
        </>
      )}
    </Stack>
  )
}

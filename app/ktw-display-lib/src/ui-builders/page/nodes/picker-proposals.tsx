// SPDX-License-Identifier: Apache-2.0
// The picker's proposal region (mock-up v6's take tab): a
// tickable picker declaring `proposals` draws in ONE column the items
// proposed (its `selected`, but those flagged `proposed: false`: chosen
// earlier, not proposed now, they show only as chips) ticked in place - the check box at the left, the
// name, the kind's slots, the detail line, « · » why, the declared badges at
// the right -, « Tout cocher » while one is unticked, then a search for
// another whose hits are ticked the same way, every chosen item as a chip
// (« × » unticks a proposal, removes another) and, with `savebar`, the save
// bar: « N choisi(e)s », « Annuler » and « Enregistrer et lire » while the
// choice differs from the one held, else the node's idle line. With groups
// (`proposals.groups`, or proposals carrying `group`) the proposals are drawn
// one block per group under a head (mock-up v6's dtGroup): its check box on,
// mixed or off, badge, title, code, tag chip, badges and « k sur n », or the
// group's empty line. Generic: no
// connector named, every word from the node.

import { useEffect, useState, type ReactNode } from 'react'
import { ActionIcon, Badge, Box, Button, Checkbox, Group, Paper, ScrollArea, Stack, Text, TextInput, ThemeIcon } from '@mantine/core'
import { IconCheck, IconSearch, IconSparkles, IconTag, IconX } from '@tabler/icons-react'

import { useHostServices, useSourceModel } from '../../../ui-core/host/index.tsx'
import type { UIPickerGroup, UIPickerItem, UIPickerNode, UIPickerStatus } from '../ui-page.interactive.ts'
import type { RenderNode } from './layout.tsx'
import { itemCardParts } from './source-item.tsx'
import {
  applyInput, badgeOf, liveQuery, pickerGroups, pickInto, PICKER_LABELS, PickerBanner, removeFromSelection,
  searchesWhileTyping, searchInput, statusBanner, tickedItems, tickGroup, toggleItem,
} from './picker.tsx'

// =================================================================================================
// Pure - tested apart from rendering
// =================================================================================================

/** tickHit ticks or unticks a search hit (or a chip's item): ticked, it
 *  joins the choice ticked (ticked back when it is there); unticked, a
 *  proposal stays unticked and any other item leaves the choice. */
export function tickHit(selection: UIPickerItem[], hit: UIPickerItem, on: boolean, proposedKeys: ReadonlySet<string>): UIPickerItem[] {
  const there = selection.some((s) => s.key === hit.key)
  if (on) return there ? toggleItem(selection, hit.key, true) : pickInto(selection, hit)
  if (!there) return selection
  return proposedKeys.has(hit.key) ? toggleItem(selection, hit.key, false) : removeFromSelection(selection, hit.key)
}

/** proposedKeys is the keys of the proposals: the node's selected items
 *  but those flagged `proposed: false` (chosen earlier, not proposed now -
 *  drawn only as chips). */
export function proposedKeys(selected: UIPickerItem[] | undefined): Set<string> {
  return new Set((selected ?? []).filter((s) => s.proposed !== false).map((s) => s.key))
}

/** chosenChips is every chosen item, drawn as a chip (mock-up v6's lxChips). */
export function chosenChips(selection: UIPickerItem[]): UIPickerItem[] {
  return tickedItems(selection)
}

const keysOf = (items: UIPickerItem[]): string => tickedItems(items).map((s) => s.key).sort().join('\n')

/** saveBarState is the save bar's count of the items ticked and whether
 *  they differ from those held at the last draw or save. */
export function saveBarState(selection: UIPickerItem[], held: UIPickerItem[]): { count: number; dirty: boolean } {
  return { count: tickedItems(selection).length, dirty: keysOf(selection) !== keysOf(held) }
}

/** undoTo is « Annuler »: the choice held, as it was. */
export function undoTo(held: UIPickerItem[]): UIPickerItem[] {
  return held.map((s) => ({ ...s }))
}

/** countLine is « N <one|many> »: singular up to one, as French counts. */
export function countLine(n: number, one: string, many: string): string {
  return `${n} ${n > 1 ? many : one}`
}

/** tickAllProposals ticks every proposal. */
export function tickAllProposals(selection: UIPickerItem[], proposedKeys: ReadonlySet<string>): UIPickerItem[] {
  return [...proposedKeys].reduce((acc, k) => toggleItem(acc, k, true), selection)
}

/** A block of the grouped proposal region: its key ("" the items in no
 *  group), its declaration when it has one, its proposals, how many are
 *  ticked and its box - on when all are, mixed when some, off when none;
 *  none for a group holding nothing. */
export interface ProposalGroup {
  key: string
  decl?: UIPickerGroup
  items: UIPickerItem[]
  ticked: number
  box?: 'on' | 'mixed' | 'off'
}

/** groupedProposals says whether the region draws by group: groups are
 *  declared or a proposal carries one. */
export function groupedProposals(props: UIPickerItem[], declared: UIPickerGroup[] | undefined): boolean {
  return (declared?.length ?? 0) > 0 || props.some((p) => !!p.group)
}

/** proposalGroups splits the proposals by group: the declared groups in
 *  their order (an empty one kept), then those found on items but not
 *  declared (first seen), then the items in no group. The ticked counts are
 *  those of `sent` (the keys handed back) when given, else of the proposals
 *  alone. */
export function proposalGroups(props: UIPickerItem[], declared: UIPickerGroup[] | undefined, sent?: ReadonlySet<string>): ProposalGroup[] {
  const found = pickerGroups(props)
  const of = (key: string) => found.find((g) => g.group === key)
  const block = (key: string, decl?: UIPickerGroup): ProposalGroup => {
    const items = of(key)?.items ?? []
    const ticked = sent ? items.filter((s) => sent.has(s.key)).length : (of(key)?.ticked ?? 0)
    const box = items.length === 0 ? undefined : ticked === 0 ? 'off' : ticked === items.length ? 'on' : 'mixed'
    return { key, ...(decl ? { decl } : {}), items, ticked, ...(box ? { box } : {}) }
  }
  const named = new Set((declared ?? []).map((d) => d.key))
  const out = (declared ?? []).map((d) => block(d.key, d))
  for (const g of found) if (g.group !== '' && !named.has(g.group)) out.push(block(g.group))
  if (of('') && !named.has('')) out.push(block(''))
  return out
}

/** tickProposalGroup is a group's box: tickGroup limited to the group's
 *  proposals - an item chosen earlier but not proposed (a chip only) is not
 *  moved. */
export function tickProposalGroup(selection: UIPickerItem[], group: string, on: boolean, proposedKeys: ReadonlySet<string>): UIPickerItem[] {
  const inGroup = (s: UIPickerItem) => (s.group ?? '') === group
  if (!on) return selection.map((s) => (inGroup(s) && proposedKeys.has(s.key) ? { ...s, off: true } : s))
  // The other items of the group are set aside, the proposals ticked as
  // tickGroup ticks them, then the others put back as they were.
  const aside = new Map(selection.filter((s) => inGroup(s) && !proposedKeys.has(s.key)).map((s) => [s.key, s]))
  if (aside.size === 0) return tickGroup(selection, group, true)
  const masked = selection.map((s) => (aside.has(s.key) ? { ...s, group: '\u0000' } : s))
  return tickGroup(masked, group, true).map((s) => aside.get(s.key) ?? s)
}

// =================================================================================================
// Rendering
// =================================================================================================

/** ProposalRow is one line of the region: the check box at the left, the
 *  name and its slots, the detail line with « · » why, the badges at the
 *  right - a badge declared `unticked` only while the row is unticked. */
function ProposalRow({ item, on, guide, node, disabled, onTick }: {
  item: UIPickerItem
  on: boolean
  guide: string
  node: Omit<UIPickerNode, 'kind'>
  disabled?: boolean
  onTick: (on: boolean) => void
}): ReactNode {
  const p = itemCardParts(item, 'summary', { kinds: node.kinds, badges: node.badges })
  const line = [p.detail, p.why].filter(Boolean)
  return (
    <Group gap="xs" wrap="nowrap" align="flex-start" px={8} py={6} data-guide={guide} data-guide-name={item.key}
      data-off={on ? undefined : 'true'} style={{ cursor: disabled || item.unreachable ? undefined : 'pointer' }}
      onClick={() => { if (!disabled && !item.unreachable) onTick(!on) }}>
      <Checkbox size="xs" mt={3} checked={on} disabled={disabled || !!item.unreachable} aria-label={item.title}
        onClick={(e) => e.stopPropagation()} onChange={() => onTick(!on)} />
      <Box style={{ flex: 1, minWidth: 0 }}>
        <Group gap={6} wrap="nowrap">
          <Text size="sm" fw={500}>{p.title}</Text>
          {p.slots.map((s) => (
            <Text key={s.attr} size="xs" c="dimmed" ff="monospace" title={s.label} data-guide="picker.slot" data-guide-name={s.attr}>{s.value}</Text>
          ))}
        </Group>
        {line.length > 0 && (
          <Text size="xs" c="dimmed">
            {p.detail}{p.detail && p.why ? ' · ' : ''}{p.why && <Text span size="xs" c="blue" data-guide="picker.why">{p.why}</Text>}
          </Text>
        )}
      </Box>
      {(item.badges ?? []).map((b) => {
        const d = badgeOf(node.badges, b)
        if (d.unticked && on) return null
        return <Badge key={b} size="xs" variant="light" color={d.color ?? 'gray'} tt="none">{d.label}</Badge>
      })}
    </Group>
  )
}

/** ProposalGroupHead is a group's head (mock-up v6's dtGroup): its box at
 *  the left - a spacer when it holds nothing -, the badge, the bold title,
 *  the code in monospace, the tag chip, the group's badges, then « k sur n »
 *  at the right. */
function ProposalGroupHead({ group, node, outOf, disabled, onTick }: {
  group: ProposalGroup
  node: Omit<UIPickerNode, 'kind'>
  outOf: string
  disabled?: boolean
  onTick: (on: boolean) => void
}): ReactNode {
  const d = group.decl
  const title = d?.title ?? group.key
  return (
    <Group gap={8} wrap="nowrap" px={8} py={6} data-guide="picker.prop-group-head">
      {group.box ? (
        <Checkbox size="xs" checked={group.box === 'on'} indeterminate={group.box === 'mixed'} disabled={disabled}
          aria-checked={group.box === 'on' ? 'true' : group.box === 'mixed' ? 'mixed' : 'false'} aria-label={title}
          onChange={() => onTick(group.box !== 'on')} data-guide="picker.prop-group-box" data-guide-name={group.key} />
      ) : <Box w={18} style={{ flexShrink: 0 }} />}
      {d?.badge && <Badge size="xs" radius="sm" variant="filled" color={d.badge.color ?? 'blue'} tt="none" data-guide="picker.prop-group-badge">{d.badge.label}</Badge>}
      <Text size="sm" fw={700}>{title}</Text>
      {d?.code && <Text size="xs" c="dimmed" ff="monospace">{d.code}</Text>}
      {d?.chip && (
        <Badge size="xs" variant="outline" color="gray" tt="none" leftSection={<IconTag size={12} />} data-guide="picker.prop-group-chip">{d.chip}</Badge>
      )}
      {(d?.badges ?? []).map((b) => {
        const m = badgeOf(node.badges, b)
        return <Badge key={b} size="xs" variant="light" color={m.color ?? 'gray'} tt="none">{m.label}</Badge>
      })}
      <Box style={{ flex: 1 }} />
      {group.items.length > 0 && <Text size="xs" c="dimmed" data-guide="picker.prop-group-count">{`${group.ticked} ${outOf} ${group.items.length}`}</Text>}
    </Group>
  )
}

/** PickerProposalsRenderer is the page node: its selection its own. */
export function PickerProposalsRenderer({ node, render }: { node: UIPickerNode; render?: RenderNode }) {
  const [selection, setSelection] = useState<UIPickerItem[]>(node.selected ?? [])
  return <PickerProposals node={node} selection={selection} onSelection={setSelection} render={render} />
}

/** PickerProposals is the proposal region, its selection held by its
 *  caller. */
export function PickerProposals({ node, selection, onSelection, render, disabled }: {
  node: Omit<UIPickerNode, 'kind'> & { kind?: string }
  selection: UIPickerItem[]
  onSelection: (next: UIPickerItem[]) => void
  render?: RenderNode
  disabled?: boolean
}) {
  const host = useHostServices()
  const call = host.callMethod
  const sourceModel = useSourceModel()
  const labels = { ...PICKER_LABELS, ...(node.labels ?? {}) }
  const decl = node.proposals!
  const bar = node.savebar
  const proposed = proposedKeys(node.selected)
  // The choice held: the one drawn first, then each one saved.
  const [held, setHeld] = useState<UIPickerItem[]>(node.selected ?? [])
  const [query, setQuery] = useState('')
  const [hits, setHits] = useState<UIPickerItem[] | null>(null)
  const [busy, setBusy] = useState(false)
  const [saving, setSaving] = useState(false)
  const [message, setMessage] = useState('')
  const [status, setStatus] = useState<UIPickerStatus | undefined>()
  const [asked, setAsked] = useState(0)
  const argsKey = JSON.stringify(node.args ?? {})
  useEffect(() => {
    if (!call || !node.status) return
    let live = true
    call(node.status, { ...(node.args ?? {}) })
      .then((out) => { if (live) setStatus(out as UIPickerStatus) })
      .catch((e) => { if (live) setStatus({ reachable: false, message: String(e) }) })
    return () => { live = false }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [call, node.status, argsKey, asked])
  const banner = statusBanner(status, labels)

  const search = async () => {
    if (!call || !node.search || query.trim() === '') return
    setBusy(true)
    setMessage('')
    try {
      const out = (await call(node.search, searchInput(node, query))) as { items?: UIPickerItem[] } | undefined
      setHits(out?.items ?? [])
    } catch (e) {
      setMessage(String(e))
    } finally {
      setBusy(false)
    }
  }
  const typing = !!node.search && searchesWhileTyping(node, sourceModel)
  useEffect(() => {
    if (!typing || !call || !liveQuery(query)) return
    const timer = setTimeout(() => { void search() }, 300)
    return () => clearTimeout(timer)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [query, typing, call])

  const save = async () => {
    if (!call || !node.apply) return
    setSaving(true)
    setMessage('')
    try {
      const out = (await call(node.apply, applyInput({ ...node, tickable: true }, selection, host.person))) as { ok?: boolean; message?: string } | undefined
      if (out?.ok === false) setMessage(out.message ?? 'refused')
      else { setHeld(selection); setMessage(out?.message ?? '') }
    } catch (e) {
      setMessage(String(e))
    } finally {
      setSaving(false)
    }
  }

  const sent = new Set(tickedItems(selection).map((s) => s.key))
  const props = selection.filter((s) => proposed.has(s.key))
  const unticked = props.some((s) => !sent.has(s.key))
  const chips = chosenChips(selection)
  const state = saveBarState(selection, held)
  const tick = (item: UIPickerItem, on: boolean) => onSelection(tickHit(selection, item, on, proposed))
  const groups = groupedProposals(props, decl.groups) ? proposalGroups(props, decl.groups, sent) : undefined

  return (
    <Stack gap="sm" data-element={node.id ? `picker:${node.id}` : undefined} data-guide="picker.proposals">
      {banner && status && (
        <PickerBanner banner={banner} status={status} labels={labels} retrying={false}
          onRetry={call && node.status ? () => setAsked((n) => n + 1) : undefined} />
      )}
      <Paper withBorder data-guide="picker.proposals-card">
        <Group gap="sm" wrap="nowrap" align="flex-start" p="sm" style={{ borderBottom: '1px solid var(--mantine-color-default-border)' }}>
          <ThemeIcon size={30} variant="light" radius="md"><IconSparkles size={16} /></ThemeIcon>
          <Box style={{ flex: 1, minWidth: 0 }}>
            <Text size="sm" fw={600}>{decl.title}</Text>
            {decl.sub && render && <Group gap={4} mt={4}>{render(decl.sub, 'sub')}</Group>}
          </Box>
          {unticked && (
            <Button size="compact-xs" variant="subtle" disabled={disabled} data-guide="picker.tick-all"
              onClick={() => onSelection(tickAllProposals(selection, proposed))}>{labels.tickAll}</Button>
          )}
        </Group>
        {groups
          ? groups.map((g) => (
            <Box key={g.key} pb={4} data-guide="picker.prop-group" data-guide-name={g.key}>
              {(g.key !== '' || g.decl) && (
                <ProposalGroupHead group={g} node={node} outOf={labels.outOf} disabled={disabled}
                  onTick={(on) => onSelection(tickProposalGroup(selection, g.key, on, proposed))} />
              )}
              {g.items.length === 0
                ? <Text size="sm" c="dimmed" pl={34} pr={8} pb={6} data-guide="picker.prop-group-empty">{g.decl?.empty ?? labels.none}</Text>
                : <Box pl={g.key !== '' || g.decl ? 26 : 0}>{g.items.map((item) => (
                  <ProposalRow key={item.key} item={item} on={sent.has(item.key)} guide="picker.proposal" node={node}
                    disabled={disabled} onTick={(on) => tick(item, on)} />
                ))}</Box>}
            </Box>
          ))
          : props.length === 0
          ? <Text size="sm" c="dimmed" p="xs" data-guide="picker.proposals-none">{decl.none ?? labels.none}</Text>
          : props.map((item) => (
            <ProposalRow key={item.key} item={item} on={sent.has(item.key)} guide="picker.proposal" node={node}
              disabled={disabled} onTick={(on) => tick(item, on)} />
          ))}
      </Paper>
      {node.search && call && (
        <TextInput maw={560} placeholder={labels.search} value={query} disabled={disabled} leftSection={<IconSearch size={16} />}
          onChange={(e) => setQuery(e.currentTarget.value)}
          onKeyDown={(e) => { if (e.key === 'Enter') void search() }}
          rightSection={busy ? <ActionIcon size="sm" variant="subtle" loading aria-label={labels.find} /> : undefined}
          data-guide="picker.search" />
      )}
      {node.search && call && !hits && node.hint && <Text size="sm" c="dimmed" data-guide="picker.hint">{node.hint}</Text>}
      {hits && (
        <Paper withBorder p={6} data-guide="picker.hits">
          {hits.length === 0 ? <Text size="sm" c="dimmed" p="xs">{labels.none}</Text> : (
            <ScrollArea.Autosize mah={node.height ?? 260}>
              <Text size="xs" fw={600} c="dimmed" px={8} py={6}>{labels.results}</Text>
              {hits.map((m) => (
                <ProposalRow key={m.key} item={proposed.has(m.key) ? { ...m, why: labels.alreadyProposed } : m} on={sent.has(m.key)}
                  guide="picker.hit" node={node} disabled={disabled} onTick={(on) => tick(m, on)} />
              ))}
            </ScrollArea.Autosize>
          )}
        </Paper>
      )}
      <Group gap={6} data-guide="picker.chips">
        {chips.length === 0 ? <Text size="sm" c="dimmed">{labels.noneChosen}</Text> : chips.map((c) => {
          const p = itemCardParts(c, 'selection', { kinds: node.kinds })
          return (
            <Badge key={c.key} size="lg" variant="light" color="gray" tt="none" data-guide="picker.chip" data-guide-name={c.key}
              rightSection={(
                <ActionIcon size="xs" variant="transparent" color="gray" aria-label={labels.remove} disabled={disabled}
                  onClick={() => tick(c, false)} data-guide="picker.chip-remove"><IconX size={12} /></ActionIcon>
              )}>
              {p.title}{p.slots.map((s) => <Text key={s.attr} span size="xs" c="dimmed" ff="monospace" ml={6}>{s.value}</Text>)}
            </Badge>
          )
        })}
      </Group>
      {bar && call && node.apply ? (
        <Paper withBorder p="sm" style={{ position: 'sticky', bottom: 0 }} data-guide="picker.savebar" data-dirty={state.dirty ? 'true' : undefined}>
          <Group gap="sm" wrap="nowrap">
            <Text size="sm" fw={700} style={{ flex: 1 }} data-guide="picker.count">{countLine(state.count, bar.one, bar.many)}</Text>
            {message && <Text size="xs" c="red" data-guide="picker.message">{message}</Text>}
            {state.dirty ? (
              <>
                <Button size="xs" variant="default" disabled={saving} onClick={() => { setMessage(''); onSelection(undoTo(held)) }}
                  data-guide="picker.cancel">{bar.cancel ?? labels.cancel}</Button>
                <Button size="xs" loading={saving} leftSection={<IconCheck size={15} />} onClick={() => void save()}
                  data-guide="picker.save">{bar.save ?? labels.saveRead}</Button>
              </>
            ) : (
              <Text size="sm" c="dimmed" data-guide="picker.idle">{state.count ? bar.idle : bar.empty}</Text>
            )}
          </Group>
        </Paper>
      ) : message ? <Text size="xs" c="red">{message}</Text> : null}
    </Stack>
  )
}


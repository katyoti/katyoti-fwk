// SPDX-License-Identifier: Apache-2.0
// Renderers of the interactive nodes: button, list, tree, lanes, gauge, gantt,
// graph. Generic: each draws its data contract; an action goes to the host
// through the page context (page-context.tsx), with the element it is on.

import { useEffect, useMemo, useRef, useState, type KeyboardEvent, type ReactNode } from 'react'
import {
  ActionIcon, Badge, Box, Button, Group, HoverCard, Menu, Paper, Progress, ScrollArea, SimpleGrid, Stack, Text,
  TextInput, Tooltip, UnstyledButton, getThemeColor, useMantineTheme,
} from '@mantine/core'
import i18next from 'i18next'
import { IconArrowsMove, IconChevronDown, IconChevronRight, IconPointFilled } from '@tabler/icons-react'

import { tablerIcon } from '../../../ui-core/components/icons/index.ts'
import { ItemCard, PULSE_CSS, softLine, toneBorder } from '../../../ui-core/components/other/index.ts'
import { DataContext, ItemContext, usePageContext, useItem, useRunAction, useSelection, withItem } from '../page-context.tsx'
import { missingKeys } from './choice-cards.tsx'
import {
  actionVerb, elementNode, FILE_MAX_SIZE, gaugeBar, type GaugePart, listItemNode, ganttCalendar, ganttSpan, gridCell, gridCounts, gridKeyTarget, gridMove, layerGraph, nestRows,
  type TreeRow, type UIButtonNode, type UIGanttNode, type UIGaugeNode, type UIGraphBox, type UIGraphNode, type UILanesNode,
  type UIGridBoardNode, type UIListNode, type UIPageAction, type UITreeNode, withFile, pressOnly,
} from '../ui-page.interactive.ts'
import type { RenderNode } from './layout.tsx'
import type { UIPageNode } from '../ui-page.types.ts'
import { statusChip } from '../../fields/_planning/palette.ts'
import { ListEditor } from './list-editor.tsx'
import { dataAttrs } from './data-attrs.ts'
import { ShowMore, useWindow, useWindows } from './window.tsx'
import { useFillHeight } from './fill-height.ts'

// =================================================================================================
// Actions
// =================================================================================================

/** useRun answers a function running an action on an element, or undefined
 *  when the page has no host to run it. An action naming no verb (or
 *  several) does nothing. */
export function useRun(): ((action: UIPageAction, item?: unknown, extra?: Record<string, unknown>) => void) | undefined {
  const { onAction, hasHost } = usePageContext()
  if (!onAction || !hasHost) return undefined
  return (action, item, extra) => { if (actionVerb(action)) onAction({ action, item, extra }) }
}

const NO_HOST = 'Runs in a panel'

/** actionShown: an element's action shows unless its `when` names a field the
 *  element does not set - or, written `field=value`, a field not holding
 *  that value. */
export function actionShown(a: UIPageAction, item: unknown): boolean {
  if (!a.when) return true
  const eq = a.when.indexOf('=')
  const field = (eq < 0 ? a.when : a.when.slice(0, eq)).trim()
  const v = item && typeof item === 'object' ? (item as Record<string, unknown>)[field] : undefined
  if (eq >= 0) return v !== undefined && v !== null && String(v) === a.when.slice(eq + 1).trim()
  return v !== undefined && v !== null && v !== '' && v !== false
}

const ITEM_REF = /\$\{\s*item\.([^}\s]+)\s*\}/g

/** actionLabel is an action's label with each `${item.<path>}` read from the
 *  element (`${item.next_label}`: a label its row words); an unset path
 *  reads empty. */
export function actionLabel(label: string | undefined, item: unknown): string {
  if (!label || !label.includes('${')) return label ?? ''
  return label.replace(ITEM_REF, (_, path: string) => {
    let v: unknown = item
    for (const k of path.split('.')) v = v && typeof v === 'object' ? (v as Record<string, unknown>)[k] : undefined
    return v === undefined || v === null ? '' : String(v)
  })
}

/** actionDisabled: why an element's action is not available yet — the text
 *  of the field its `disabledWhen` names, when the element sets it; else
 *  undefined (the action runs). */
export function actionDisabled(a: UIPageAction, item: unknown): string | undefined {
  if (!a.disabledWhen) return undefined
  const v = item && typeof item === 'object' ? (item as Record<string, unknown>)[a.disabledWhen] : undefined
  if (v === undefined || v === null || v === '' || v === false) return undefined
  return typeof v === 'string' ? v : 'Not available yet'
}

/** ActionButtons draws an element's actions (a list item, a tree row, a card,
 *  a table row), each run on that element. */
export function ActionButtons({ actions, item }: { actions?: UIPageAction[]; item: unknown }) {
  const run = useRun()
  const shown = (actions ?? []).filter((a) => actionShown(a, item))
  if (!shown.length) return null
  return (
    <Group gap={4} wrap="nowrap">
      {shown.map((a, i) => {
        const Icon = a.icon ? tablerIcon(a.icon, IconPointFilled) : undefined
        const why = actionDisabled(a, item)
        const btn = (
          <Button key={a.id ?? i} size="compact-xs" variant="light" color={a.dangerous ? 'red' : a.color}
            leftSection={Icon ? <Icon size={12} /> : undefined} disabled={!run || !actionVerb(a) || why !== undefined}
            onClick={(e) => { e.stopPropagation(); run?.(a, item) }}
            data-guide="page.action" data-guide-name={a.uri ?? a.id} data-guide-label={a.label}>
            {actionLabel(a.label, item) || a.id || 'Run'}
          </Button>
        )
        if (why !== undefined) return <Tooltip key={a.id ?? i} label={why} withArrow multiline w={260}><span>{btn}</span></Tooltip>
        return run ? btn : <Tooltip key={a.id ?? i} label={NO_HOST} withArrow><span>{btn}</span></Tooltip>
      })}
    </Group>
  )
}

/** ItemScope renders a template for one element: `$.item` the element, and
 *  actions inside it on that element. */
function ItemScope({ item, index, children }: { item: unknown; index: number; children: ReactNode }) {
  const page = usePageContext()
  const value = useMemo(() => ({
    ...page,
    draft: withItem(page.draft, item, index),
    seed: withItem(page.seed, item, index),
  }), [page, item, index])
  return (
    <DataContext.Provider value={value}>
      <ItemContext.Provider value={item}>{children}</ItemContext.Provider>
    </DataContext.Provider>
  )
}

// =================================================================================================
// button
// =================================================================================================

export function ButtonRenderer({ node }: { node: UIButtonNode }) {
  const run = useRun()
  const item = useItem()
  const { selection } = useSelection()
  const input = useRef<HTMLInputElement>(null)
  const [refused, setRefused] = useState<string>()
  const Icon = tablerIcon(node.icon ?? node.action.icon, IconPointFilled)
  const file = node.action.file
  // A file-choosing action: the press opens the system's
  // choice; the chosen file runs the action, a cancel runs nothing.
  const chosen = async (f: File | undefined) => {
    if (!f || !run) return
    const r = withFile(node.action, { name: f.name, size: f.size, content: f.size > FILE_MAX_SIZE ? '' : await base64Of(f) },
      // The person's language when the file is chosen: the shared i18n's,
      // which the host sets.
      i18next.language || 'en')
    if ('error' in r) { setRefused(r.error); return }
    setRefused(undefined)
    run(r.action, item, r.extra)
  }
  const own = dataAttrs(node)
  const color = node.action.dangerous ? 'red' : (node.color ?? node.action.color)
  // Disabled until the selection sets every key it requires.
  const missing = missingKeys(node.requires, selection)
  // The icon before the label, or after it with iconPosition right.
  const icon = node.icon || node.action.icon ? <Icon size={14} /> : undefined
  const after = node.iconPosition === 'right'
  const btn = (
    <Button {...(run && !file ? own : {})} size={node.size ?? 'sm'} variant={node.variant ?? 'filled'}
      color={color} style={node.variant === 'outline' ? outlineButtonStyle(color) : undefined}
      leftSection={after ? undefined : icon} rightSection={after ? icon : undefined}
      data-element={node.id ? `button:${node.id}` : undefined} data-missing={missing.length ? missing.join(' ') : undefined}
      disabled={!run || !actionVerb(node.action) || missing.length > 0} onClick={(e) => pressOnly(e, () => file ? input.current?.click() : run?.(node.action, item))}>
      {node.label}
    </Button>
  )
  const shown = !run ? <Tooltip label={NO_HOST} withArrow><span {...(file ? {} : own)}>{btn}</span></Tooltip>
    : missing.length && node.requiresHint ? <Tooltip label={node.requiresHint} withArrow><span>{btn}</span></Tooltip>
    : btn
  if (!file) return shown
  return (
    <Stack {...own} gap={4} align="flex-start">
      {shown}
      <input ref={input} type="file" hidden accept={file.accept}
        onChange={(e) => { const f = e.currentTarget.files?.[0]; e.currentTarget.value = ''; void chosen(f) }} />
      {refused && <Text size="xs" c="red">{refused}</Text>}
    </Stack>
  )
}

/** An outlined button's look: a soft line in its colour (softLine, the
 *  inline KPI tile's ring) and its label in a light shade of it on the dark
 *  theme, a deep one on the light; a 6px corner, a 10px side padding. With
 *  no colour, Mantine's own outline. */
export function outlineButtonStyle(color?: string): Record<string, string> | undefined {
  if (!color) return undefined
  return {
    '--button-bd': `1px solid ${softLine(color)}`,
    '--button-color': `light-dark(var(--mantine-color-${color}-7), var(--mantine-color-${color}-3))`,
    '--button-radius': '6px',
    '--button-padding-x': '10px',
  }
}

/** base64Of reads a file as base64, without its data-URL prefix. */
function base64Of(f: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const r = new FileReader()
    r.onload = () => { const s = String(r.result ?? ''); resolve(s.slice(s.indexOf(',') + 1)) }
    r.onerror = () => reject(r.error)
    r.readAsDataURL(f)
  })
}

// =================================================================================================
// list — one template per element
// =================================================================================================

export function ListRenderer({ node, render }: { node: UIListNode; render: RenderNode }) {
  const open = useRunAction()(node.open)
  const items = Array.isArray(node.data) ? node.data : []
  // On a progressive page only a page of items is drawn.
  const win = useWindow(items.length, node.pageSize)
  // Its template, or its elements' type drawn as its card.
  const item = listItemNode(node)
  if (items.length === 0 || !item) {
    return <Text {...dataAttrs(node)} size="sm" c="dimmed">{node.empty ?? 'Nothing to show.'}</Text>
  }
  const more = <ShowMore rest={items.length - win.shown} onMore={win.more} name={node.id} />
  const cells = items.slice(0, win.shown).map((el, i) => (
    <ListCell key={keyOf(el, i)} node={node} render={render} item={item} el={el} index={i} open={open} />
  ))
  const gap = node.gap ?? 'sm'
  switch (node.layout) {
    case 'grid':
      return <><SimpleGrid {...dataAttrs(node)} cols={{ base: 1, sm: Math.min(2, node.columns ?? 3), md: node.columns ?? 3 }} spacing={gap}>{cells}</SimpleGrid>{more}</>
    case 'horizontal':
      return <ScrollArea {...dataAttrs(node)} type="auto"><Group gap={gap} wrap="nowrap" align="stretch">{cells}{more}</Group></ScrollArea>
    default:
      return <Stack {...dataAttrs(node)} gap={gap}>{cells}{more}</Stack>
  }
}

/** ListCell draws one element of a list - its template (or its type's card),
 *  its actions, its inline editor, its card and tone, a click opening it;
 *  `lead` drawn before it (a grouped list's tick). */
export function ListCell({ node, render, item, el, index, open, lead }: {
  node: UIListNode
  render: RenderNode
  item: UIPageNode
  el: unknown
  index: number
  open?: (el: unknown) => void
  lead?: ReactNode
}) {
  // An element holding its own node draws it (`nodeField`), else the template.
  const drawn = render(elementNode(node, el, item), `item-${index}`)
  const shown = node.actions?.length || lead ? (
    <Group justify="space-between" wrap="nowrap" align="center" gap="sm">
      {lead}
      <Box style={{ flex: 1, minWidth: 0 }}>{drawn}</Box>
      <ActionButtons actions={node.actions} item={el} />
    </Group>
  ) : drawn
  // An inline editor in the card, on the elements it is for.
  const body = node.editor && actionShown({ when: node.editor.when }, el)
    ? <Stack gap={6}>{shown}<ListEditor editor={node.editor} item={el} /></Stack>
    : shown
  const tone = toneOf(el, node.toneField, node.toneColors)
  return (
    <ItemScope item={el} index={index}>
      {node.variant === 'cards' || tone ? (
        <ItemCard tone={tone} pulse={isPulsing(el, node.toneField, node.pulse)} padding={node.cardPadding as never} onClick={open ? () => open(el) : undefined}>{body}</ItemCard>
      ) : open ? <UnstyledButton onClick={() => open(el)} style={{ display: 'block', width: '100%' }}>{body}</UnstyledButton> : body}
    </ItemScope>
  )
}

/** toneOf: the accent color of an element — its tone field's value through
 *  the color map, or the value itself when it is a color. */
function toneOf(el: unknown, field?: string, colors?: Record<string, string>): string | undefined {
  if (!field || !el || typeof el !== 'object') return undefined
  const v = (el as Record<string, unknown>)[field]
  if (v === undefined || v === null || v === '') return undefined
  return colors?.[String(v)] ?? (colors ? undefined : String(v))
}

/** isPulsing: the element's tone value is one of the node's `pulse` values. */
function isPulsing(el: unknown, field?: string, pulse?: string[]): boolean {
  if (!field || !pulse?.length || !el || typeof el !== 'object') return false
  return pulse.includes(String((el as Record<string, unknown>)[field] ?? ''))
}

/** The pulse keyframes for the tree's rows (an ItemCard writes its own). */
function PulseStyle({ on }: { on: boolean }) {
  return on ? <style>{PULSE_CSS}</style> : null
}

function keyOf(el: unknown, i: number): string {
  if (el && typeof el === 'object' && 'id' in el) return String((el as { id: unknown }).id)
  return String(i)
}

// =================================================================================================
// tree — rows linked by their parent
// =================================================================================================

export function TreeRenderer({ node, render }: { node: UITreeNode; render: RenderNode }) {
  const rows = Array.isArray(node.data) ? node.data : []
  const forest = useMemo(
    () => nestRows(rows, node.idField ?? 'id', node.parentField ?? 'parent'),
    [rows, node.idField, node.parentField],
  )
  const [open, setOpen] = useState<Record<string, boolean>>({})
  const openRow = useRunAction()(node.open)
  // On a progressive page only a page of the rows shown is drawn.
  const win = useWindow(Number.MAX_SAFE_INTEGER, node.pageSize)
  if (forest.length === 0) return <Text {...dataAttrs(node)} size="sm" c="dimmed">{node.empty ?? 'Nothing to show.'}</Text>
  const isOpen = (r: TreeRow<Record<string, unknown>>) => open[r.id] ?? (node.expanded ?? r.depth === 0)
  const toggle = (id: string, now: boolean) => setOpen((o) => ({ ...o, [id]: !now }))
  let index = 0
  const walk = (rs: TreeRow<Record<string, unknown>>[]): ReactNode[] => rs.flatMap((r) => {
    const i = index++
    const expanded = isOpen(r)
    const title = String(r.row[node.titleField ?? 'title'] ?? r.id)
    const badge = node.badgeField ? r.row[node.badgeField] : undefined
    const row = (
      <ItemScope key={r.id} item={r.row} index={i}>
        <Group gap="xs" wrap="nowrap" py={8} pl={r.depth * 24 + 4} pr={4}
          style={{ borderBottom: '1px solid var(--mantine-color-default-border)', ...toneBorder(toneOf(r.row, node.toneField, node.toneColors), isPulsing(r.row, node.toneField, node.pulse)) }}>
          {r.children.length > 0 ? (
            <ActionIcon size="sm" variant="subtle" color="gray" aria-label={expanded ? `Collapse ${title}` : `Expand ${title}`}
              onClick={() => toggle(r.id, expanded)}>
              {expanded ? <IconChevronDown size={14} /> : <IconChevronRight size={14} />}
            </ActionIcon>
          ) : <Box w={28} />}
          <Box style={{ flex: 1, minWidth: 0, cursor: openRow ? 'pointer' : undefined }} onClick={openRow ? () => openRow(r.row) : undefined}>
            {node.item ? render(node.item, `row-${r.id}`)
              : <Text size="sm" fw={r.depth === 0 ? 600 : 400} truncate>{title}</Text>}
          </Box>
          {badge !== undefined && badge !== '' && (
            <Badge size="sm" variant="light" color={node.badgeColors?.[String(badge)] ?? 'gray'}>{String(badge)}</Badge>
          )}
          <ActionButtons actions={node.actions} item={r.row} />
        </Group>
      </ItemScope>
    )
    return expanded ? [row, ...walk(r.children)] : [row]
  })
  const all = walk(forest)
  const shown = Math.min(all.length, win.shown)
  return (
    <Stack {...dataAttrs(node)} gap={0}>
      <PulseStyle on={rows.some((r) => isPulsing(r, node.toneField, node.pulse))} />
      {all.slice(0, shown)}
      <ShowMore rest={all.length - shown} onMore={win.more} name={node.id} />
    </Stack>
  )
}

// =================================================================================================
// lanes — cards in columns; a drop runs `move`
// =================================================================================================

/** LaneScroll scrolls a lane's cards: inside the lane's full height (`fill`),
 *  or past a max height. */
function LaneScroll({ fill, height, children }: { fill: boolean; height?: number; children: ReactNode }) {
  if (fill) return <ScrollArea type="auto" offsetScrollbars style={{ flex: 1, minHeight: 0 }}>{children}</ScrollArea>
  return <ScrollArea.Autosize mah={height} type="auto" offsetScrollbars={height !== undefined}>{children}</ScrollArea.Autosize>
}

export function LanesRenderer({ node, render }: { node: UILanesNode; render: RenderNode }) {
  const run = useRun()
  const field = node.laneField ?? 'lane'
  const cards = Array.isArray(node.data) ? node.data : []
  const [over, setOver] = useState<string | null>(null)
  const moveInto = (to: string) => node.lanes.find((l) => l.id === to)?.move ?? node.move
  const canMove = !!(run && (node.move || node.lanes.some((l) => l.move)))
  const accepts = (from: string, to: string) => {
    if (from === to) return false
    const lane = node.lanes.find((l) => l.id === from)
    return !lane?.accepts || lane.accepts.includes(to)
  }
  const drop = (card: Record<string, unknown>, to: string) => {
    const action = moveInto(to)
    if (action && accepts(String(card[field]), to)) run?.(action, card, { lane: to })
  }
  const openCard = useRunAction()(node.open)
  const fill = node.laneHeight === 'fill'
  const [box, height] = useFillHeight(fill)
  // On a progressive page each lane draws a page of its cards.
  const wins = useWindows(node.pageSize)
  return (
    <ScrollArea {...dataAttrs(node)} type="auto" ref={box}>
      <Group gap="sm" wrap="nowrap" align="stretch">
        {node.lanes.map((lane) => {
          const mine = cards.filter((c) => String(c[field]) === lane.id)
          return (
            <Paper key={lane.id} withBorder p="sm" radius="md" miw={node.laneWidth ?? 220} style={{
              flex: 1, background: over === lane.id ? 'var(--mantine-color-default-hover)' : undefined,
              ...(fill ? { height, display: 'flex', flexDirection: 'column' as const } : {}),
              borderTop: lane.color ? `3px solid var(--mantine-color-${lane.color}-6)` : undefined,
            }}
              onDragOver={canMove ? (e) => { e.preventDefault(); setOver(lane.id) } : undefined}
              onDragLeave={canMove ? () => setOver(null) : undefined}
              onDrop={canMove ? (e) => {
                e.preventDefault(); setOver(null)
                const id = e.dataTransfer.getData('text/plain')
                const card = cards.find((c, i) => keyOf(c, i) === id)
                if (card) drop(card, lane.id)
              } : undefined}>
              <Group justify="space-between" mb="sm" wrap="nowrap">
                <Box style={{ minWidth: 0 }}>
                  <Text size="sm" fw={700} truncate c={lane.color ? `${lane.color}.4` : undefined}>{lane.title}</Text>
                  {lane.meta && <Text size="xs" c="dimmed" truncate>{lane.meta}</Text>}
                </Box>
                <Badge size="md" variant="light" color={lane.color ?? 'gray'} circle={mine.length < 10}>{mine.length}</Badge>
              </Group>
              <LaneScroll fill={fill} height={typeof node.laneHeight === 'number' ? node.laneHeight : undefined}>
              <Stack gap="xs">
                {mine.slice(0, wins.shown(lane.id, mine.length)).map((card) => {
                  const i = cards.indexOf(card)
                  const id = keyOf(card, i)
                  const others = node.lanes.filter((l) => accepts(lane.id, l.id) && moveInto(l.id))
                  return (
                    <ItemScope key={id} item={card} index={i}>
                      <ItemCard draggable={canMove} shadow="xs"
                        onDragStart={canMove ? (e) => e.dataTransfer.setData('text/plain', id) : undefined}
                        onClick={openCard ? () => openCard(card) : undefined}
                        tone={toneOf(card, node.toneField, node.toneColors)} pulse={isPulsing(card, node.toneField, node.pulse)}
                        style={canMove ? { cursor: 'grab' } : undefined}>
                        <Group justify="space-between" wrap="nowrap" align="flex-start" gap={4}>
                          <Box style={{ flex: 1, minWidth: 0 }}>
                            {node.item ? render(node.item, `card-${id}`)
                              : <Text size="sm">{String(card[node.titleField ?? 'title'] ?? id)}</Text>}
                          </Box>
                          {canMove && others.length > 0 && (
                            <Menu position="bottom-end" withinPortal>
                              <Menu.Target>
                                <ActionIcon size="sm" variant="subtle" color="gray" aria-label="Move to another lane" onClick={(e) => e.stopPropagation()}>
                                  <IconArrowsMove size={14} />
                                </ActionIcon>
                              </Menu.Target>
                              <Menu.Dropdown>
                                <Menu.Label>Move to</Menu.Label>
                                {others.map((l) => <Menu.Item key={l.id} onClick={(e) => { e.stopPropagation(); drop(card, l.id) }}>{l.title}</Menu.Item>)}
                              </Menu.Dropdown>
                            </Menu>
                          )}
                        </Group>
                        <Box mt={node.actions?.length ? 6 : 0}><ActionButtons actions={node.actions} item={card} /></Box>
                      </ItemCard>
                    </ItemScope>
                  )
                })}
                <ShowMore rest={mine.length - wins.shown(lane.id, mine.length)} onMore={() => wins.more(lane.id)} name={lane.id} />
              </Stack>
              </LaneScroll>
            </Paper>
          )
        })}
      </Group>
    </ScrollArea>
  )
}

// =================================================================================================
// gauge
// =================================================================================================

export function GaugeRenderer({ node }: { node: UIGaugeNode }) {
  const run = useRunAction()(node.action)
  const item = useItem()
  const gauge = <Gauge node={node} attrs={run ? undefined : dataAttrs(node)} />
  if (!run) return gauge
  return (
    <UnstyledButton {...dataAttrs(node)} onClick={() => run(item)} aria-label={node.action?.label ?? node.label} style={{ display: 'block', width: '100%' }}>
      {gauge}
    </UnstyledButton>
  )
}

function Gauge({ node, attrs }: { node: UIGaugeNode; attrs?: Record<string, string> }) {
  const theme = useMantineTheme()
  const { onAction } = usePageContext()
  const { value, max, parts } = gaugeBar(node)
  const pct = max > 0 ? Math.round(Math.min(100, (value / max) * 100)) : 0
  const fill = (t: string) => t.replaceAll('{value}', String(value)).replaceAll('{max}', String(max)).replaceAll('{percent}', String(pct))
  const hint = node.hint ? fill(node.hint) : undefined
  const valueText = fill(node.valueText ?? '{value} / {max}')
  const stacked = !!node.segments?.length
  const size = node.size ?? 'md'
  const shown = parts.filter((p) => p.value > 0)
  // A segment listing what it counts shows it on hover, each opening its
  // object.
  const listed = shown.some((p) => p.items?.length)
  const section = (p: GaugePart, i: number) => {
    const sec = (
      <Progress.Section key={i} value={p.percent} color={p.color ?? 'blue'} data-segment={p.label}>
        {(size === 'lg' || size === 'xl') && p.percent >= 12 && <Progress.Label>{p.value}</Progress.Label>}
      </Progress.Section>
    )
    if (!p.items?.length) return sec
    return (
      <HoverCard key={i} withArrow openDelay={150} shadow="md" withinPortal>
        <HoverCard.Target>{sec}</HoverCard.Target>
        <HoverCard.Dropdown p="xs" data-segment-items={p.label}>
          <Text size="xs" fw={600} mb={4}>{`${p.value} ${p.label ?? ''}`.trim()}</Text>
          <Stack gap={2}>
            {p.items.map((it) => (
              <UnstyledButton key={it.id} disabled={!onAction} onClick={() => onAction?.({ action: { navigate: { object: it.id } }, item: it })}>
                <Text size="xs"><Text span ff="monospace" c="dimmed">{it.id}</Text>{it.title ? ` ${it.title}` : ''}</Text>
              </UnstyledButton>
            ))}
          </Stack>
        </HoverCard.Dropdown>
      </HoverCard>
    )
  }
  const bar = stacked ? (
    <Progress.Root size={size} radius="sm" aria-label={`${node.label ?? ''}: ${shown.map((p) => `${p.value} ${p.label ?? ''}`).join(', ')}`}>
      {shown.map(section)}
    </Progress.Root>
  ) : (
    <Progress value={parts[0]?.percent ?? 0} color={node.color ?? (pct >= 100 ? 'orange' : 'blue')} size={size} radius="sm"
      aria-label={`${node.label ?? ''}: ${value} of ${max}`} />
  )
  return (
    <Stack {...attrs} gap={4}>
      {(node.label || valueText) && (
        <Group justify="space-between" wrap="nowrap">
          <Text size="sm" fw={600}>{node.label}</Text>
          {valueText && <Text size="xs" c="dimmed">{valueText}</Text>}
        </Group>
      )}
      {stacked && shown.length > 0 && !listed ? (
        <Tooltip label={shown.map((p) => `${p.value} ${p.label ?? ''}`.trim()).join(' · ')} withArrow>
          <Box>{bar}</Box>
        </Tooltip>
      ) : bar}
      {stacked && node.legend === 'swatch' && shown.length > 0 && (
        // The swatch legend (request-rows-mockup.html
        // .legend): per shown part an 8px square in the part's own colour
        // beside its dimmed count and label, wrapping, gap 10px, 11px text.
        <div data-gauge-legend="swatch" style={{ display: 'flex', flexWrap: 'wrap', gap: 10, fontSize: 11, color: 'var(--mantine-color-dimmed)' }}>
          {shown.map((p, i) => (
            <span key={i} data-swatch={p.label}>
              <span style={{ display: 'inline-block', width: 8, height: 8, borderRadius: 2, marginRight: 4, verticalAlign: -1, background: getThemeColor(p.color ?? 'blue', theme) }} />
              {`${p.value} ${p.label ?? ''}`.trim()}
            </span>
          ))}
        </div>
      )}
      {stacked && node.legend === true && shown.length > 0 && (
        <Group gap="xs" wrap="wrap">
          {shown.map((p, i) => (
            <Badge key={i} size="sm" variant="light" color={p.color ?? 'blue'}>{`${p.value} ${p.label ?? ''}`.trim()}</Badge>
          ))}
        </Group>
      )}
      {hint && <Text size="xs" c="dimmed">{hint}</Text>}
    </Stack>
  )
}

// =================================================================================================
// gantt
// =================================================================================================

export function GanttRenderer({ node }: { node: UIGanttNode }) {
  const run = useRun()
  const given = Array.isArray(node.data) ? node.data : []
  const cal = node.scale === 'calendar' ? ganttCalendar(given, node.today) : undefined
  const tasks = cal ? cal.tasks : given
  if (tasks.length === 0) return <Text {...dataAttrs(node)} size="sm" c="dimmed">Nothing scheduled.</Text>
  const span = cal ? cal.span : ganttSpan(tasks)
  const pct = (v: number) => `${(v / span) * 100}%`
  const n = Math.min(10, Math.ceil(span))
  const ticks = cal
    ? cal.ticks
    : Array.from({ length: n + 1 }, (_, i) => ({ at: (i * span) / n, label: `${Math.round(((i * span) / n) * 10) / 10}${i === n && node.unit ? ` ${node.unit}` : ''}` }))
  const todayAt = cal?.today
  const todayLine = todayAt === undefined ? null : (
    <Box data-gantt-today style={{ position: 'absolute', top: 0, bottom: 0, left: pct(todayAt), width: 2, background: 'var(--mantine-color-red-6)', pointerEvents: 'none' }} />
  )
  return (
    <Stack {...dataAttrs(node)} gap={6}>
      <Box style={{ display: 'grid', gridTemplateColumns: 'minmax(120px, 220px) 1fr', gap: 12 }}>
        <span />
        <Box style={{ position: 'relative', height: 16 }}>
          {ticks.map((k, i) => (
            <Text key={i} size="xs" c="dimmed" style={{ position: 'absolute', left: pct(k.at), whiteSpace: 'nowrap', transform: !cal && i === ticks.length - 1 ? 'translateX(-100%)' : undefined }}>
              {k.label}
            </Text>
          ))}
        </Box>
      </Box>
      {tasks.map((t) => {
        const color = t.color ?? (t.critical ? 'blue' : 'blue')
        const bar = (
          <Box style={{
            position: 'absolute', top: 3, height: 16, borderRadius: 3,
            left: pct(Number(t.start) || 0), width: pct(Math.max(Number(t.length) || 0, span / 200)),
            background: `var(--mantine-color-${color}-${t.critical ? 7 : 3})`,
          }} />
        )
        return (
          <Box key={t.id} style={{ display: 'grid', gridTemplateColumns: 'minmax(120px, 220px) 1fr', gap: 12, alignItems: 'center' }}>
            <Text size="sm" truncate title={t.title}>{t.title}</Text>
            <UnstyledButton disabled={!(run && node.open)} onClick={() => node.open && run?.(node.open, t)}
              aria-label={cal ? `${t.title}: ${t.from} to ${t.to ?? t.from}` : `${t.title}: from ${t.start} for ${t.length}`}
              style={{ position: 'relative', height: 22, background: 'var(--mantine-color-default-hover)', borderRadius: 4, cursor: run && node.open ? 'pointer' : 'default' }}>
              {bar}
              {todayLine}
            </UnstyledButton>
          </Box>
        )
      })}
    </Stack>
  )
}

// =================================================================================================
// graph — boxes and edges in layers
// =================================================================================================

const BOX_W = 160
const GAP_X = 56
const GAP_Y = 18
const FRAME_PAD = 10

export function GraphRenderer({ node }: { node: UIGraphNode }) {
  const run = useRun()
  const nodes = node.data?.nodes ?? []
  const edges = node.data?.edges ?? []
  const { boxes, layers, frames } = useMemo(() => layerGraph(nodes, edges), [nodes, edges])
  if (boxes.length === 0) return <Text {...dataAttrs(node)} size="sm" c="dimmed">Nothing to show.</Text>
  // A box with badges takes a third line.
  const BOX_H = boxes.some((b) => b.badges?.length) ? 62 : 48
  const rows = Math.max(1, ...boxes.map((b) => b.slot + 1))
  const pad = frames.length ? FRAME_PAD : 0
  const width = layers * BOX_W + (layers - 1) * GAP_X + 2 + 2 * pad
  const height = node.height ?? rows * BOX_H + (rows - 1) * GAP_Y + 2 + 2 * pad
  const xOf = (layer: number) => 1 + pad + layer * (BOX_W + GAP_X)
  const yOf = (slot: number) => 1 + pad + slot * (BOX_H + GAP_Y)
  const at = new Map<string, { x: number; y: number; w: number; h: number }>(
    boxes.map((b) => [b.id, { x: xOf(b.layer), y: yOf(b.slot), w: BOX_W, h: BOX_H }]))
  for (const f of frames) {
    const x = xOf(f.layerFrom) - pad, y = yOf(f.slotFrom) - pad
    at.set(f.id, { x, y, w: xOf(f.layerTo) + BOX_W + pad - x, h: yOf(f.slotTo - 1) + BOX_H + pad - y })
  }
  const clickable = !!(run && node.open)
  const open = (b: UIGraphBox) => node.open && run?.(node.open, b)
  return (
    <ScrollArea {...dataAttrs(node)} type="auto">
      <svg width={width} height={height} viewBox={`0 0 ${width} ${height}`} role="img" aria-label="Boxes and edges">
        <defs>
          <marker id="ktw-graph-arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
            <path d="M0 0L10 5L0 10z" fill="var(--mantine-color-gray-6)" />
          </marker>
        </defs>
        {frames.map((f) => {
          const r = at.get(f.id)!
          const look = f.status ? statusChip(f.status) : undefined
          return (
            <g key={f.id} data-graph-frame={f.id} data-frame={f.frame ?? 'solid'} style={{ cursor: clickable ? 'pointer' : 'default' }}
              onClick={() => open(f)} role={clickable ? 'button' : undefined} aria-label={clickable ? `Open ${f.title}` : undefined}>
              <rect x={r.x} y={r.y} width={r.w} height={r.h} rx={10} fill="none"
                stroke={look?.border ?? 'var(--mantine-color-gray-5)'} strokeWidth={1.2}
                strokeDasharray={f.frame === 'dotted' ? '2 4' : undefined} />
              <text x={r.x + 10} y={r.y + 18} fontSize={12} fontWeight={700} fill="var(--mantine-color-text)">{clip(f.title, 40)}</text>
              {(f.meta || f.badges?.length) && (
                <text x={r.x + 10} y={r.y + 34} fontSize={11} fill={look?.fg ?? 'var(--mantine-color-dimmed)'}>
                  {clip([f.meta, ...(f.badges ?? [])].filter(Boolean).join(' · '), 60)}
                </text>
              )}
            </g>
          )
        })}
        {edges.map((e, i) => {
          const a = at.get(e.from), b = at.get(e.to)
          if (!a || !b) return null
          const x1 = a.x + a.w, y1 = a.y + a.h / 2, x2 = b.x, y2 = b.y + b.h / 2
          const mid = (x1 + x2) / 2
          return (
            <path key={i} d={`M${x1} ${y1} C${mid} ${y1} ${mid} ${y2} ${x2 - 2} ${y2}`} fill="none"
              stroke="var(--mantine-color-gray-6)" strokeWidth={1.5} strokeDasharray={e.style === 'dashed' ? '4 3' : undefined}
              markerEnd="url(#ktw-graph-arrow)" />
          )
        })}
        {boxes.map((b) => {
          const p = at.get(b.id)!
          const tone = b.tone ?? (b.critical ? 'blue' : 'gray')
          // A status draws the box in the shared status colours.
          const look = b.status ? statusChip(b.status) : undefined
          return (
            <g key={b.id} transform={`translate(${p.x} ${p.y})`} style={{ cursor: clickable ? 'pointer' : 'default' }}
              data-graph-box={b.id} data-status={b.status} data-frame={b.frame}
              onClick={() => open(b)} role={clickable ? 'button' : undefined}
              aria-label={clickable ? `Open ${b.title}` : undefined}>
              <rect width={BOX_W} height={BOX_H} rx={8}
                fill={look ? look.bg : b.critical ? `var(--mantine-color-${tone}-light)` : 'var(--mantine-color-body)'}
                stroke={look ? look.border : `var(--mantine-color-${tone}-${b.critical ? 6 : 4})`} strokeWidth={b.critical ? 2 : 1}
                strokeDasharray={b.frame === 'dotted' ? '2 4' : undefined} />
              <text x={10} y={20} fontSize={12} fontWeight={600} fill="var(--mantine-color-text)">{clip(b.title, 22)}</text>
              {b.meta && <text x={10} y={36} fontSize={11} fill="var(--mantine-color-dimmed)">{clip(b.meta, 24)}</text>}
              {b.badges?.length ? <text x={10} y={52} fontSize={10} fill={look?.fg ?? 'var(--mantine-color-dimmed)'}>{clip(b.badges.join(' · '), 26)}</text> : null}
            </g>
          )
        })}
      </svg>
    </ScrollArea>
  )
}

function clip(s: string, n: number): string {
  return s.length > n ? s.slice(0, n - 1) + '…' : s
}

// =================================================================================================
// grid-board - cards in cells of rows x columns
// =================================================================================================

/** GridBoardRenderer draws rows by one field and columns by another, counts
 *  on both; a card dragged to another column of its row - or moved there by
 *  Alt+Left / Alt+Right - runs the move (a prompted column asks its text in
 *  the card first); a drop in another row is refused with the board's words. */
export function GridBoardRenderer({ node, render }: { node: UIGridBoardNode; render: RenderNode }) {
  const run = useRun()
  const { onAction } = usePageContext()
  const openCard = useRunAction()(node.open)
  const cards = Array.isArray(node.data) ? node.data : []
  const counts = gridCounts(node)
  const [over, setOver] = useState<string | null>(null)
  const [message, setMessage] = useState<string | null>(null)
  const [asking, setAsking] = useState<{ id: string; column: string; prompt: string; text: string } | null>(null)
  const canMove = !!(run && (node.move || node.columns.some((c) => c.move)))
  const rf = node.rowField ?? 'row'
  const width = node.columnWidth ?? 180
  // On a progressive page each cell draws a page of its cards.
  const wins = useWindows(node.pageSize)

  const tryMove = (card: Record<string, unknown>, id: string, row: string, column: string, text?: string) => {
    const m = gridMove(node, card, row, column, text)
    if ('refused' in m) { setMessage(m.refused); return }
    setMessage(null)
    if ('prompt' in m) { setAsking({ id, column: m.column, prompt: m.prompt, text: '' }); return }
    if ('run' in m) { setAsking(null); onAction?.(m.run) }
  }

  return (
    <Stack {...dataAttrs(node)} gap={6} data-element={node.id ? `grid-board:${node.id}` : undefined}>
      {message ? <Text size="xs" c="red" role="alert">{message}</Text> : null}
      <ScrollArea type="auto">
        <Box style={{ display: 'grid', gridTemplateColumns: `200px repeat(${node.columns.length}, minmax(${width}px, 1fr))`, gap: 6 }}>
          <Box />
          {node.columns.map((c) => (
            <Group key={c.id} justify="space-between" wrap="nowrap" px={4}
              style={{ borderTop: c.color ? `3px solid var(--mantine-color-${c.color}-6)` : undefined }}>
              <Text size="xs" fw={700} tt="uppercase">{c.title}</Text>
              <Badge size="xs" variant="light" color={c.color ?? 'gray'} data-guide="grid.column-count" data-guide-name={c.id}>{counts.columns[c.id] ?? 0}</Badge>
            </Group>
          ))}
          {node.rows.map((r) => [
            <Paper key={`h-${r.id}`} withBorder p={6} radius="sm">
              <Group justify="space-between" wrap="nowrap">
                <Box style={{ minWidth: 0 }}>
                  <Text size="sm" fw={600} truncate>{r.title}</Text>
                  {r.meta ? <Text size="xs" c="dimmed" truncate>{r.meta}</Text> : null}
                </Box>
                <Badge size="xs" variant="light" color="gray" data-guide="grid.row-count" data-guide-name={r.id}>{counts.rows[r.id] ?? 0}</Badge>
              </Group>
            </Paper>,
            ...node.columns.map((c) => {
              const cell = `${r.id}|${c.id}`
              return (
                <Paper key={cell} withBorder p={6} radius="sm" data-guide="grid.cell" data-guide-name={cell}
                  style={{ minHeight: 56, background: over === cell ? 'var(--mantine-color-default-hover)' : undefined }}
                  onDragOver={canMove ? (e) => { e.preventDefault(); setOver(cell) } : undefined}
                  onDragLeave={canMove ? () => setOver(null) : undefined}
                  onDrop={canMove ? (e) => {
                    e.preventDefault(); setOver(null)
                    const id = e.dataTransfer.getData('text/plain')
                    const i = cards.findIndex((cd, k) => keyOf(cd, k) === id)
                    if (i >= 0) tryMove(cards[i], id, r.id, c.id)
                  } : undefined}>
                  <Stack gap={6}>
                    {gridCell(node, r.id, c.id).slice(0, wins.shown(cell, gridCell(node, r.id, c.id).length)).map((card) => {
                      const i = cards.indexOf(card)
                      const id = keyOf(card, i)
                      const ask = asking && asking.id === id ? asking : null
                      return (
                        <ItemScope key={id} item={card} index={i}>
                          <Box tabIndex={canMove ? 0 : undefined} data-guide="grid.card" data-guide-name={id}
                            onKeyDown={canMove ? (e: KeyboardEvent<HTMLDivElement>) => {
                              if (!e.altKey || (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight')) return
                              e.preventDefault()
                              const to = gridKeyTarget(node, card, e.key === 'ArrowLeft' ? -1 : 1)
                              if (to) tryMove(card, id, String(card[rf]), to)
                            } : undefined}>
                          <ItemCard draggable={canMove} shadow="xs"
                            onDragStart={canMove ? (e) => e.dataTransfer.setData('text/plain', id) : undefined}
                            onClick={openCard ? () => openCard(card) : undefined}
                            style={canMove ? { cursor: 'grab' } : undefined}>
                            {node.item ? render(node.item, `card-${id}`)
                              : <Text size="sm">{String(card[node.titleField ?? 'title'] ?? id)}</Text>}
                            {ask ? (
                              <Stack gap={4} mt={6} onClick={(e) => e.stopPropagation()}>
                                <TextInput size="xs" label={ask.prompt} value={ask.text} autoFocus
                                  onChange={(e) => setAsking({ ...ask, text: e.currentTarget.value })} />
                                <Group gap={4}>
                                  <Button size="compact-xs" disabled={!ask.text.trim()}
                                    onClick={() => tryMove(card, id, String(card[rf]), ask.column, ask.text)}>OK</Button>
                                  <Button size="compact-xs" variant="subtle" onClick={() => setAsking(null)}>Cancel</Button>
                                </Group>
                              </Stack>
                            ) : null}
                            <Box mt={node.actions?.length ? 6 : 0}><ActionButtons actions={node.actions} item={card} /></Box>
                          </ItemCard>
                          </Box>
                        </ItemScope>
                      )
                    })}
                    <ShowMore rest={gridCell(node, r.id, c.id).length - wins.shown(cell, gridCell(node, r.id, c.id).length)} onMore={() => wins.more(cell)} name={cell} />
                  </Stack>
                </Paper>
              )
            }),
          ])}
        </Box>
      </ScrollArea>
    </Stack>
  )
}

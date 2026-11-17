// SPDX-License-Identifier: Apache-2.0
// Layout node renderers: vbox, hbox, grid, box, card, section, tabs,
// accordion, split. Thin wrappers around Mantine v7 primitives that
// recurse into children via the lazy <UIPageView> reference passed in.
//
// The lazy reference (rather than a direct import) breaks the import
// cycle: UIPageView imports these node renderers; each renderer needs
// to recurse via UIPageView. We pass it down as a prop.

import { Badge,
  Accordion,
  Box,
  Card,
  Group,
  ScrollArea,
  SimpleGrid,
  Stack,
  Tabs,
  Text,
  ThemeIcon,
  Title,
} from '@mantine/core'
import { tablerIcon } from '../../../ui-core/components/icons/index.ts'
import { IconPointFilled } from '@tabler/icons-react'
import { useContext, useEffect, useState, type CSSProperties, type ReactNode } from 'react'
import { TabRequestContext, tabShown, usePageContext, useRunAction } from '../page-context.tsx'
import { boundValue } from './marks.ts'
import { dataAttrs } from './data-attrs.ts'
import { panelMounted } from './window.tsx'
import { useProgressive } from '../../../ui-core/host/index.tsx'
import { surfaceBg } from './surface.ts'
import { useFillHeight } from './fill-height.ts'

import type {
  UIAccordionNode,
  UIBoxNode,
  UICardNode,
  UIGridNode,
  UIHBoxNode,
  UIPageNode,
  UIPanelSpec,
  UISectionNode,
  UISplitNode,
  UITabsNode,
  UIVBoxNode,
} from '../ui-page.types.ts'

/** Render-children helper passed down from `<UIPageView>` so layout
 *  components can recurse without an import cycle. */
export type RenderNode = (node: UIPageNode, key?: string | number) => ReactNode

// =================================================================================================
// split — a main column and a side panel
// =================================================================================================

/** The side panel's width when a split names none, in px. */
export const SPLIT_SIDE_WIDTH = 380

// One column when the split's own box is narrower than 1000px (a container
// query: a page in a narrow pane collapses as on a narrow screen).
const SPLIT_CSS = `.ktw-split{container-type:inline-size}
.ktw-split-grid{display:grid;grid-template-columns:minmax(0,1fr) var(--ktw-split-side,${SPLIT_SIDE_WIDTH}px);gap:16px;align-items:start}
.ktw-split-side[data-sticky]{position:sticky;top:0;align-self:start}
@container (max-width: 999px){.ktw-split-grid{grid-template-columns:minmax(0,1fr)}.ktw-split-side[data-sticky]{position:static}}`

export function SplitRenderer({ node, render }: { node: UISplitNode; render: RenderNode }) {
  const width = node.sideWidth && node.sideWidth > 0 ? node.sideWidth : SPLIT_SIDE_WIDTH
  const sticky = node.sticky !== false
  return (
    <div {...dataAttrs(node)} className="ktw-split">
      <style>{SPLIT_CSS}</style>
      <div className="ktw-split-grid" style={{ ['--ktw-split-side' as string]: `${width}px` }}>
        <div className="ktw-split-main" style={{ minWidth: 0 }}>{render(node.main, 'main')}</div>
        <div className="ktw-split-side" data-sticky={sticky ? '' : undefined} style={{ minWidth: 0 }}>{render(node.side, 'side')}</div>
      </div>
    </div>
  )
}

// =================================================================================================
// vbox / hbox / grid / box / card / section
// =================================================================================================

export function VBoxRenderer({ node, render }: { node: UIVBoxNode; render: RenderNode }) {
  return (
    <Stack {...dataAttrs(node)} gap={node.gap as never} p={node.p as never} align={node.align} justify={node.justify}>
      {(node.children ?? []).map((c, i) => render(c, c.id ?? i))}
    </Stack>
  )
}

export function HBoxRenderer({ node, render }: { node: UIHBoxNode; render: RenderNode }) {
  return (
    <Group
      {...dataAttrs(node)}
      gap={node.gap as never}
      p={node.p as never}
      align={node.align}
      justify={node.justify}
      wrap={node.wrap === false ? 'nowrap' : 'wrap'}
    >
      {(node.children ?? []).map((c, i) => render(c, c.id ?? i))}
    </Group>
  )
}

/** ratioColumns are a grid's columns by ratio: each a share
 *  of the width (`minmax(0, 2fr)`), never narrower than its content allows;
 *  a ratio not positive counts 1. */
export function ratioColumns(ratio: number[]): string {
  return ratio.map((r) => `minmax(0, ${Number.isFinite(r) && r > 0 ? r : 1}fr)`).join(' ')
}

/** widthColumns are a grid's columns by fixed widths: a
 *  width in pixels, or 0 (or none valid) for a share of what is left - by
 *  the ratio at its place when it is positive, else 1. */
export function widthColumns(widths: number[], ratio?: number[]): string {
  return widths.map((w, i) => {
    if (Number.isFinite(w) && w > 0) return `${w}px`
    const r = ratio?.[i]
    return `minmax(0, ${Number.isFinite(r) && (r as number) > 0 ? r : 1}fr)`
  }).join(' ')
}

/** gridGap is a grid's gap: pixels for a number (0: the cells touch), a
 *  spacing token's variable otherwise (default md). */
export function gridGap(gap: string | number | undefined): string {
  if (typeof gap === 'number') return `${gap}px`
  return `var(--mantine-spacing-${gap ?? 'md'})`
}

export function GridRenderer({ node, render }: { node: UIGridNode; render: RenderNode }) {
  if (node.widths?.length) {
    // Each cell in a box of its own, stretched to its row's height and never
    // wider than its column; a divider column's box draws a line on its left.
    const cols = node.widths.length
    const lined = new Set(node.dividers ?? [])
    return (
      <Box {...dataAttrs(node)} data-guide="grid.widths" style={{ display: 'grid', gridTemplateColumns: widthColumns(node.widths, node.ratio), gap: gridGap(node.gap), alignItems: 'stretch' }}>
        {(node.children ?? []).map((c, i) => {
          const divided = lined.has(i % cols)
          return (
            <div key={c.id ?? i} data-guide={divided ? 'grid.divider' : undefined}
              style={{ display: 'grid', minWidth: 0, borderLeft: divided ? '1px solid var(--mantine-color-default-border)' : undefined }}>
              {render(c, c.id ?? i)}
            </div>
          )
        })}
      </Box>
    )
  }
  if (node.ratio?.length) {
    return (
      <Box {...dataAttrs(node)} data-guide="grid.ratio" style={{ display: 'grid', gridTemplateColumns: ratioColumns(node.ratio), gap: gridGap(node.gap) }}>
        {(node.children ?? []).map((c, i) => render(c, c.id ?? i))}
      </Box>
    )
  }
  return (
    <SimpleGrid {...dataAttrs(node)} cols={node.cols} spacing={node.gap as never}>
      {(node.children ?? []).map((c, i) => render(c, c.id ?? i))}
    </SimpleGrid>
  )
}

export function BoxRenderer({ node, render }: { node: UIBoxNode; render: RenderNode }) {
  // A callout: the tone's light background and a border of
  // it, dashed when asked. Its background, and a light `bg`, follow the
  // colour scheme (surfaceBg): never a light box on the dark
  // theme.
  const style = node.callout
    ? { border: `1px ${node.dashed ? 'dashed' : 'solid'} var(--mantine-color-${node.callout}-5)`, borderRadius: (node.radius ?? 8) as never }
    : { borderRadius: node.radius as never }
  return (
    <Box {...dataAttrs(node)} bg={surfaceBg(node.bg ?? (node.callout ? `${node.callout}.0` : undefined))} p={(node.p ?? (node.callout ? 'xs' : undefined)) as never} style={style}
      data-guide={node.callout ? 'callout' : undefined} data-guide-name={node.callout ? `${node.callout}${node.dashed ? '-dashed' : ''}` : undefined}>
      <Stack gap={node.gap as never}>
        {(node.children ?? []).map((c, i) => render(c, c.id ?? i))}
      </Stack>
    </Box>
  )
}

export function CardRenderer({ node, render }: { node: UICardNode; render: RenderNode }) {
  // `height: 100%` + flex-column lets cards in a stretched grid (e.g. 2-col
  // SimpleGrid where the sibling has more content) carry that height down
  // to their children. The Image/Video renderers inside use `flex: 1`
  // wrappers to claim the extra space — filling instead of leaving a strip
  // of whitespace below the media.
  // height: fill - the card takes the window's height below it and its last
  // child scrolls inside it (a ScrollArea taking what is left), so a row of
  // such cards scrolls each box and not the page.
  const fill = node.height === 'fill'
  const [box, height] = useFillHeight(fill)
  const children = node.children ?? []
  const last = children.length - 1
  return (
    <Card
      {...dataAttrs(node)}
      ref={fill ? box : undefined}
      withBorder={node.withBorder !== false}
      shadow={node.shadow as never}
      radius={node.radius as never}
      padding={node.p as never}
      // dashed: its normal border drawn dashed - the
      // mock-up's .constraint goal card.
      // stripe: a band of its tone down its left edge - a row's kind (the
      // increment's Blockers tab).
      style={{ height: fill ? height : '100%', display: 'flex', flexDirection: 'column', ...(node.dashed ? { borderStyle: 'dashed' } : {}),
        ...(node.stripe ? { borderLeft: `6px solid var(--mantine-color-${node.stripe}-6)` } : {}) }}
      data-element={node.id ? `card:${node.id}` : undefined}
      data-dashed={node.dashed ? '' : undefined}
      data-stripe={node.stripe || undefined}
      data-fill={fill ? '' : undefined}
    >
      <Stack gap={node.gap as never} style={{ flex: 1, minHeight: 0 }}>
        {children.map((c, i) => fill && i === last
          ? <ScrollArea key={c.id ?? i} type="auto" offsetScrollbars data-fill-scroll="" style={{ flex: 1, minHeight: 0 }}>{render(c, c.id ?? i)}</ScrollArea>
          : render(c, c.id ?? i))}
      </Stack>
    </Card>
  )
}

export function SectionRenderer({ node, render }: { node: UISectionNode; render: RenderNode }) {
  const size = node.size ?? 'lg'
  const compact = size === 'sm'
  const children = node.children ?? []
  const aside = node.aside ?? []
  const Icon = node.icon ? tablerIcon(node.icon, IconPointFilled) : undefined
  const hasBadge = node.badge !== undefined && node.badge !== ''
  const head = !!(node.title || node.description || Icon || hasBadge || aside.length)
  const title = node.title && (compact
    ? <Text size="sm" fw={500}>{node.title}</Text>
    : <Title order={size === 'md' ? 5 : 3} fw={600}>{node.title}</Title>)
  const icon = Icon && (compact
    ? <Icon size={14} color={node.color ? `var(--mantine-color-${node.color}-6)` : undefined} />
    : <ThemeIcon variant="light" color={node.color ?? 'blue'} radius="md" size={size === 'md' ? 'md' : 'lg'}><Icon size={16} /></ThemeIcon>)
  return (
    <Stack {...dataAttrs(node)} gap={node.gap ?? (compact ? 4 : 'sm')} p={node.p as never}>
      {head && (
        <Group justify="space-between" align="flex-start" wrap="nowrap" gap="sm">
          <Stack gap={2} style={{ minWidth: 0 }}>
            <Group gap={6} wrap="nowrap">
              {icon}
              {title}
              {hasBadge && <Badge size="xs" variant="light" color={node.badgeColor ?? 'gray'}>{node.badge}</Badge>}
            </Group>
            {node.description && (
              <Text size={compact ? 'xs' : 'sm'} c="dimmed">
                {node.description}
              </Text>
            )}
          </Stack>
          {aside.length > 0 && (
            <Group gap={6} wrap="wrap" justify="flex-end">
              {aside.map((c, i) => render(c, c.id ?? `aside-${i}`))}
            </Group>
          )}
        </Group>
      )}
      {children.length > 0
        ? children.map((c, i) => render(c, c.id ?? i))
        : node.empty && <Text size="xs" c={node.emptyColor ?? 'dimmed'}>{node.empty}</Text>}
    </Stack>
  )
}

// =================================================================================================
// tabs / accordion — special child shape (panel array)
// =================================================================================================

/** tabChoice is what choosing a tab does: show it, and run the tabs node's
 *  select when it is another tab than the one shown; nothing for none. */
export function tabChoice(current: string, next: string | null): { value: string; select: boolean } | undefined {
  if (!next) return undefined
  return { value: next, select: next !== current }
}

export function TabsRenderer({ node, render }: { node: UITabsNode; render: RenderNode }) {
  // Resolve initial active tab. Mantine Tabs requires every Tab to have a
  // unique `value` prop; we use panel.id, falling back to index.
  const tabValue = (panelId: string | undefined, idx: number) =>
    panelId ?? `tab-${idx}`
  const defaultValue =
    node.defaultTab ?? tabValue(node.tabs[0]?.id, 0)
  const [value, setValue] = useState<string>(defaultValue)
  // A page building only the tab shown re-reads its source for the one chosen.
  const select = useRunAction()(node.select)
  const choose = (v: string | null) => {
    const c = tabChoice(value, v)
    if (!c) return
    setValue(c.value)
    if (c.select) select?.({ id: c.value })
  }
  const { draft } = usePageContext()
  // On a progressive page only the open tab is mounted.
  const progressive = useProgressive()
  // A `tab` action asking for one of its panels shows it.
  const asked = useContext(TabRequestContext)
  const [since] = useState(asked?.n ?? 0)
  useEffect(() => {
    const shown = tabShown(asked, (node.tabs ?? []).map((p, i) => tabValue(p.id, i)), since)
    if (shown) setValue(shown)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [asked])

  return (
    <Tabs {...dataAttrs(node)} value={value} onChange={choose} variant={node.variant ?? 'default'} keepMounted={!progressive}>
      <Tabs.List>
        {(node.tabs ?? []).map((panel, i) => (
          <Tabs.Tab key={tabValue(panel.id, i)} value={tabValue(panel.id, i)}
            leftSection={panel.icon ? (() => { const I = tablerIcon(panel.icon, IconPointFilled); return <I size={15} /> })() : undefined}
            rightSection={(() => {
              // A badge bound to the page's data follows it.
              const b = boundValue(draft, panel.badgeField, panel.badge)
              return b !== undefined && b !== '' ? (
                <Badge size="sm" variant="light" color={panel.badgeColor ?? 'gray'} circle={String(b).length <= 1}
                  data-guide="tab.badge" data-guide-name={panel.id}>{b}</Badge>
              ) : undefined
            })()}>
            {panel.label}
          </Tabs.Tab>
        ))}
      </Tabs.List>
      {(node.tabs ?? []).map((panel, i) => (
        <Tabs.Panel
          key={tabValue(panel.id, i)}
          value={tabValue(panel.id, i)}
          pt="sm"
        >
          {render(panel.content, panel.id ?? i)}
        </Tabs.Panel>
      ))}
    </Tabs>
  )
}

export function AccordionRenderer({ node, render }: { node: UIAccordionNode; render: RenderNode }) {
  // Same id-or-index strategy as tabs.
  const itemValue = (panelId: string | undefined, idx: number) =>
    panelId ?? `panel-${idx}`
  const defaultValue = node.defaultOpen?.length
    ? node.multiple
      ? node.defaultOpen
      : node.defaultOpen[0]
    : undefined

  // On a progressive page an item's content is mounted only once open:
  // the accordion is controlled to know which are.
  const progressive = useProgressive()
  const [opened, setOpened] = useState<string[]>(
    Array.isArray(defaultValue) ? defaultValue : defaultValue ? [defaultValue] : [])
  const content = (panel: { id?: string; content: UIPageNode }, i: number) =>
    panelMounted(progressive, opened.includes(itemValue(panel.id, i))) ? render(panel.content, panel.id ?? i) : null

  // A fold's header: its label, then its badge as a dimmed count beside it
  // (mock-up delivery-views-v2 view 5) - plain text, not the tabs' pill. A
  // badge bound to the page's data follows it, as a tab's does;
  // nothing is drawn without one, and 0 is drawn as the tabs draw it.
  const { draft } = usePageContext()
  const foldControl = (panel: UIPanelSpec) => {
    const b = boundValue(draft, panel.badgeField, panel.badge)
    if (b === undefined || b === '') return panel.label
    return (
      <Group gap={10} wrap="nowrap">
        <span>{panel.label}</span>
        <Text span fw={400} c={panel.badgeColor ?? 'dimmed'}
          data-guide="fold.badge" data-guide-name={panel.id}>{String(b)}</Text>
      </Group>
    )
  }

  if (node.look === 'rule') {
    const toggle = (v: string) => setOpened((o) => o.includes(v) ? o.filter((x) => x !== v) : node.multiple ? [...o, v] : [v])
    return (
      <div {...dataAttrs(node)} data-guide="accordion.rule">
        {(node.panels ?? []).map((panel, i) => {
          const v = itemValue(panel.id, i)
          const isOpen = opened.includes(v)
          return (
            <div key={v} {...dataAttrs(panel)} data-fold={v} data-open={isOpen ? '' : undefined} style={{ margin: '0 0 12px' }}>
              <RuleHead panel={panel} open={isOpen} badge={boundValue(draft, panel.badgeField, panel.badge)} onToggle={() => toggle(v)} />
              {panelMounted(progressive, isOpen) && <div hidden={!isOpen}>{render(panel.content, panel.id ?? i)}</div>}
            </div>
          )
        })}
      </div>
    )
  }

  // Mantine Accordion's `multiple` prop is a discriminated union — we
  // branch the JSX so the type stays tight per-mode.
  if (node.multiple) {
    return (
      <Accordion {...dataAttrs(node)} multiple value={opened} onChange={(v) => setOpened(v)}>
        {(node.panels ?? []).map((panel, i) => (
          <Accordion.Item key={itemValue(panel.id, i)} value={itemValue(panel.id, i)}>
            <Accordion.Control>{foldControl(panel)}</Accordion.Control>
            <Accordion.Panel>{content(panel, i)}</Accordion.Panel>
          </Accordion.Item>
        ))}
      </Accordion>
    )
  }
  return (
    <Accordion {...dataAttrs(node)} value={opened[0] ?? null} onChange={(v) => setOpened(v ? [v] : [])}>
      {(node.panels ?? []).map((panel, i) => (
        <Accordion.Item key={itemValue(panel.id, i)} value={itemValue(panel.id, i)}>
          <Accordion.Control>{foldControl(panel)}</Accordion.Control>
          <Accordion.Panel>{content(panel, i)}</Accordion.Panel>
        </Accordion.Item>
      ))}
    </Accordion>
  )
}

/** RuleHead is the head of a fold drawn as a rule line
 *  (request-rows-mockup.html section 3): with a colour, a caret, the label as
 *  an uppercase tag of it, its count and a rule to the end of the line; with
 *  none, a quiet dimmed line - a caret, the label and its badge (« ▸ 59
 *  done »). A click opens or folds it. */
function RuleHead({ panel, open, badge, onToggle }: { panel: UIPanelSpec; open: boolean; badge: unknown; onToggle: () => void }) {
  const caret = <span data-guide="fold.caret" style={{ width: 12, color: 'var(--mantine-color-dimmed)', opacity: 0.75 }}>{open ? '▾' : '▸'}</span>
  const count = badge === undefined || badge === '' ? null : String(badge)
  const head = (style: CSSProperties, body: ReactNode) => (
    <div role="button" tabIndex={0} aria-expanded={open} data-guide="fold.head" data-guide-name={panel.id}
      onClick={onToggle} onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); onToggle() } }}
      style={{ display: 'flex', alignItems: 'center', padding: '6px 2px', fontSize: 12, color: 'var(--mantine-color-dimmed)', cursor: 'pointer', ...style }}>
      {caret}{body}
    </div>
  )
  if (!panel.color) {
    return head({ gap: 6, flexWrap: 'wrap' }, <>
      <span>{panel.label}</span>
      {count !== null && <span data-guide="fold.badge" data-guide-name={panel.id}>{count}</span>}
    </>)
  }
  const c = panel.color
  return head({ gap: 8, fontWeight: 700, textTransform: 'uppercase', letterSpacing: '.06em' }, <>
    <span data-guide="fold.tag" style={{ display: 'inline-block', borderRadius: 4, padding: '0 6px', fontSize: 10, lineHeight: '17px', fontWeight: 700, letterSpacing: '.04em',
      background: `var(--mantine-color-${c}-light)`, color: `var(--mantine-color-${c}-light-color)` }}>{panel.label}</span>
    {count !== null && <span data-guide="fold.badge" data-guide-name={panel.id} style={{ color: 'var(--mantine-color-text)' }}>{count}</span>}
    <span data-guide="fold.rule" style={{ flex: 1, height: 1, background: 'var(--mantine-color-default-border)' }} />
  </>)
}

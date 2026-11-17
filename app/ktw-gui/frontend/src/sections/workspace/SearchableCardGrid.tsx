// SPDX-License-Identifier: Apache-2.0
/**
 * SearchableCardGrid — the ONE component for every "tag-rich list of
 * things you can click" surface. Replaces the per-page custom
 * search-bar + tag-filter + card-grid duplication across:
 *
 *   - Node tab "Actions" entry (per-node actions)
 *   - Global tab "Actions" entry (global tasks + quick actions)
 *   - Debug tab Tasks (operator inventory)
 *   - Debug tab Skills (skill gallery)
 *   - TaskGalleryPanel + SkillGalleryPanel (the original canonical
 *     surfaces; now thin wrappers around this component)
 *
 * Layout:
 *
 *   ┌──────────────────────────────────────────────────────┐
 *   │ Quick actions (optional cards above the bar)         │
 *   ├──────────────────────────────────────────────────────┤
 *   │ [🔍 Search…              ] [Filter by tags ▼ ]       │
 *   ├──────────────────────────────────────────────────────┤
 *   │ ┌─ Card ─┐ ┌─ Card ─┐ ┌─ Card ─┐                    │
 *   │ │ Title  │ │ Title  │ │ Title  │                    │
 *   │ │ tags   │ │ tags   │ │ tags   │                    │
 *   │ └────────┘ └────────┘ └────────┘                    │
 *   └──────────────────────────────────────────────────────┘
 *
 * The compact MultiSelect for tags lives on the same row as the
 * search bar (cwm-v3 UX restructure 2026-06-06) — the prior wide
 * `<Chip.Group>` chip wall is gone.
 */

import { isElementId } from '../../elements'
import { useMemo, useState } from 'react'
import {
  ActionIcon,
  Anchor,
  Badge,
  Box,
  Card,
  Chip,
  Code,
  Group,
  MultiSelect,
  Popover,
  ScrollArea,
  Select,
  SimpleGrid,
  Stack,
  Text,
  TextInput,
  Title,
  Tooltip,
} from '@mantine/core'
import { tablerIcon } from '@ktw/display-lib/ui-core/components/icons/index.ts'
import { IconArrowsSort, IconBolt, IconInfoCircle, IconPlayerPlay, IconSearch, IconStar, IconStarFilled, IconX } from '@tabler/icons-react'
import { SORT_KEYS, sortItems, type SortKey } from '../task/listPrefs'
import { firstByHashtags, priorityOnly } from '../shell/universeGui'
import type { ReactNode } from 'react'
import { categoryToColor } from '../../utils/categoryColor'
import { Markdown } from '../../components/Markdown'
import { activateOnKey } from '../../guide/anchors'
import { useT } from '../../i18n'

/** One row in either the quick-actions strip or the main item grid. */
export interface CardListItem {
  /** Stable key + click target id. */
  id: string
  /** Card title (typically the entry's name / label). */
  title: string
  /** Optional secondary line below the title. */
  description?: string
  /** Optional rich body (markdown source) revealed behind a "?" affordance
   *  at the top-right of the card. When set, the icon button appears. */
  infoBody?: string
  /** When the grid is given an `infoFetch`, a card with no `infoBody` still
   *  shows the affordance and loads its body when opened. Lets a list that
   *  must NOT read every body up front (an action list) offer the same
   *  popover a list that already holds them (the task gallery) offers. */
  infoLazy?: boolean
  /** Optional URI shown at the top of the "?" popover (the runnable id). */
  infoUri?: string
  /** Tag list — searched + filtered. */
  hashtags?: string[]
  /** A task's audience, kind and types: at Essentiel a tagged universe keeps
   *  the end-user tasks not dedicated to an object type (priorityOnly). */
  audience?: string
  kind?: string
  types?: string[]
  /** Optional small category line below title. */
  category?: string
  /** Tabler icon name (kebab-case). Resolved at render by tablerIcon.
   *  Falls back to the default icon when unresolved. */
  iconName?: string
  /** Optional small footer content (URI in Code, source badge, etc.). */
  meta?: ReactNode
  /** Optional Mantine color for icon accent. */
  accent?: string
  /** Optional status badge shown top-right of the card title (e.g. an
   *  "À venir" marker for preview/future actions). Distinct from
   *  `hashtags`, which are searchable filter chips. */
  badge?: { label: string; color?: string }
  /** When set + truthy, the card renders muted + non-clickable. */
  disabled?: boolean
  /** Free-form extra slug appended to the searchable haystack. */
  searchSlug?: string
  /** Where it comes from (a task's provider): the « source » sort order. */
  source?: string
}

interface SearchableCardGridProps {
  /** The order the person picked (name, category, source); with onSortChange,
   *  a select beside the search changes it. */
  sort?: SortKey
  onSortChange?: (key: SortKey) => void
  /** The ids starred; with onToggleStar, each item card carries a star. */
  starred?: string[]
  onToggleStar?: (item: CardListItem) => void
  /** Ids whose star is set by the item itself (a task tagged quick-action):
   *  shown on, not clickable. */
  lockedStars?: string[]
  /** Hashtags whose items come first (the universe's `gui.first`), each part
   *  in the order picked. */
  firstTags?: string[]
  /** The person's display level: at Essentiel, with firstTags, only the items
   *  carrying one show until « Voir toutes les tâches ». */
  level?: string
  /** Optional cards rendered above the search bar — "Quick actions"
   *  block. When empty, the section is hidden entirely. */
  quickActions?: CardListItem[]
  /** With it, each quick action carries a ✕ that removes it. */
  onRemoveQuick?: (item: CardListItem) => void
  /** Main item list — driven by search + tag filter. */
  items: CardListItem[]
  /** Triggered when the user clicks a card. Receives the full item. */
  onItemClick?: (item: CardListItem) => void
  /** Default placeholder when items is non-empty but filters match nothing. */
  emptyFilteredMessage?: string
  /** Default placeholder when items is empty (nothing to show). */
  emptyDataMessage?: string
  /** Optional title rendered above the items grid. */
  itemsTitle?: string
  /** Optional title rendered above the quick-actions grid. */
  quickActionsTitle?: string
  /** Optional override for the default-icon. Tabler icons are
   *  ForwardRefExoticComponent — type loose to accept them. */
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  defaultIcon?: any
  /** Loads one card's long form when its info popover first opens.
   *
   *  A list that ALREADY holds the bodies passes `infoBody` per item and no
   *  fetcher. A list that must not read them up front — an action list, where
   *  forty bodies would be forty round trips to draw one panel — passes this
   *  instead and pays for one body when a reader asks for it. */
  infoFetch?: (item: CardListItem) => Promise<string>
}

/** Resolve a Tabler icon name to its component (Icon + PascalCase).
 *  Tabler icons are ForwardRefExoticComponent — the loose `any`-typed
 *  return tolerates that without disabling the rest of the type-check. */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
function resolveTablerIcon(iconName: string | undefined, fallback: any): any {
  return tablerIcon(iconName, fallback)
}

export function SearchableCardGrid({
  quickActions = [],
  items,
  onItemClick,
  emptyFilteredMessage,
  emptyDataMessage,
  itemsTitle,
  quickActionsTitle,
  defaultIcon, infoFetch,
  sort, onSortChange, starred = [], onToggleStar, lockedStars = [], firstTags, level, onRemoveQuick,
}: SearchableCardGridProps) {
  const t = useT('cards')
  const [query, setQuery] = useState('')
  const [selectedTags, setSelectedTags] = useState<string[]>([])
  const [selectedCategories, setSelectedCategories] = useState<string[]>([])

  // Aggregate available tags from the items only (quick actions are
  // always rendered, not filterable).
  const allTags = useMemo(() => {
    const set = new Set<string>()
    for (const it of items) {
      for (const tag of it.hashtags ?? []) set.add(tag)
    }
    return Array.from(set).sort()
  }, [items])

  // Aggregate categories for the one-click toggle row.
  const allCategories = useMemo(() => {
    const set = new Set<string>()
    for (const it of items) {
      if (it.category) set.add(it.category)
    }
    return Array.from(set).sort()
  }, [items])

  const [showAll, setShowAll] = useState(false)
  const narrowed = useMemo(() => priorityOnly(items, firstTags, level ?? '', showAll), [items, firstTags, level, showAll])

  const visible = useMemo(() => {
    const q = query.trim().toLowerCase()
    return firstByHashtags(sortItems(narrowed.items, sort), firstTags).filter((it) => {
      // Category toggles OR together: a card matches if its category is any
      // of the selected ones. No selection = no category filter.
      if (selectedCategories.length > 0) {
        if (!it.category || !selectedCategories.includes(it.category)) return false
      }
      // Tag filter is AND-combined: every selected tag must appear.
      if (selectedTags.length > 0) {
        const itTags = new Set(it.hashtags ?? [])
        for (const t of selectedTags) if (!itTags.has(t)) return false
      }
      if (!q) return true
      const haystack = [
        it.title,
        it.description ?? '',
        it.category ?? '',
        it.searchSlug ?? '',
        ...(it.hashtags ?? []),
      ]
        .join(' ')
        .toLowerCase()
      return haystack.includes(q)
    })
  }, [narrowed, sort, firstTags, query, selectedTags, selectedCategories])

  const fallbackIcon = defaultIcon ?? IconPlayerPlay

  return (
    // Root is a bounded flex column that fills the remaining height of
    // its (flex-column) parent: the quick-actions block + the
    // search/filter row stay fixed, and ONLY the items grid scrolls.
    // `flex: 1` (not `height: 100%`) so the grid sizes correctly even
    // when siblings sit above it (e.g. ActionsPanel's result banners).
    // The grid lives in its own `flex: 1, minHeight: 0` ScrollArea —
    // the robust pattern that does not depend on every ancestor
    // resolving a percentage height (the prior single `height: 100%`
    // ScrollArea silently failed to scroll inside VerticalTabBody).
    <Stack gap="xl" p="lg" style={{ flex: 1, minHeight: 0 }}>
      {/* Quick actions (optional) — also exported standalone as
          QuickActionsCardGrid so the Node tab can render the same
          cards block ABOVE the vertical strip without the rest of
          the SearchableCardGrid scaffolding (search bar, items
          grid, empty states). cwm-v3 UX restructure 2026-06-07. */}
      {quickActions.length > 0 && (
        <Box style={{ flexShrink: 0 }}>
          {quickActionsTitle && (
            <Title order={5} mb="xs">{quickActionsTitle}</Title>
          )}
          <QuickActionsCardGrid items={quickActions} onItemClick={onItemClick} onRemove={onRemoveQuick} />
        </Box>
      )}

      {/* Search + compact tag filter row (fixed) + scrollable items grid */}
      <Box style={{ flex: 1, minHeight: 0, display: 'flex', flexDirection: 'column' }}>
        <Group justify="space-between" align="center" mb="xs" style={{ flexShrink: 0 }}>
          {itemsTitle && <Title order={5}>{itemsTitle}</Title>}
          <Text size="xs" c="dimmed">
            {t('count', { visible: visible.length, total: items.length })}
          </Text>
        </Group>
        <Group gap="xs" mb="sm" wrap="nowrap" align="center" style={{ flexShrink: 0 }}>
          <TextInput
            size="sm"
            placeholder={t('searchItems')}
            leftSection={<IconSearch size={14} />}
            value={query}
            onChange={(e) => setQuery(e.currentTarget.value)}
            style={{ flex: 1, minWidth: 0 }}
          />
          <MultiSelect
            size="sm"
            placeholder={t('filterByTags')}
            data={allTags}
            value={selectedTags}
            onChange={setSelectedTags}
            searchable
            clearable
            hidePickedOptions
            w={240}
            comboboxProps={{ withinPortal: true }}
            disabled={allTags.length === 0}
          />
          {onSortChange && <SortSelect value={sort} onChange={onSortChange} />}
        </Group>

        {/* One-click category filter — toggle a category on/off. */}
        {allCategories.length > 0 && (
          <Chip.Group multiple value={selectedCategories} onChange={setSelectedCategories}>
            <Group gap={6} wrap="wrap" mb="sm" style={{ flexShrink: 0 }}>
              {allCategories.map((cat) => (
                <Chip key={cat} value={cat} size="xs" variant="light" color={categoryToColor(cat)}>
                  {cat}
                </Chip>
              ))}
            </Group>
          </Chip.Group>
        )}

        {items.length === 0 ? (
          <Text size="xs" c="dimmed" ta="center" py="md">{emptyDataMessage ?? t('nothingToShow')}</Text>
        ) : visible.length === 0 ? (
          <Text size="xs" c="dimmed" ta="center" py="md">{emptyFilteredMessage ?? t('noMatches')}</Text>
        ) : (
          <ScrollArea style={{ flex: 1, minHeight: 0 }} offsetScrollbars>
            <SimpleGrid cols={{ base: 1, sm: 2, lg: 3, xl: 4 }} spacing="sm">
              {visible.map((it) => {
                const Icon = resolveTablerIcon(it.iconName, fallbackIcon)
                return (
                  <Card
                    key={it.id}
                    withBorder
                    radius="md"
                    padding="md"
                    style={{
                      cursor: it.disabled ? 'default' : 'pointer',
                      opacity: it.disabled ? 0.5 : 1,
                    }}
                    onClick={() => { if (!it.disabled) onItemClick?.(it) }}
                    role="button"
                    tabIndex={it.disabled ? -1 : 0}
                    aria-disabled={it.disabled || undefined}
                    aria-label={it.title}
                    onKeyDown={activateOnKey(() => { if (!it.disabled) onItemClick?.(it) })}
                    data-guide="card"
                    data-guide-name={it.id}
                  >
                    <Stack gap={6}>
                      <Group gap="xs" wrap="nowrap" align="flex-start">
                        <Icon
                          size={20}
                          color={it.accent ? `var(--mantine-color-${it.accent}-6)` : undefined}
                        />
                        <Stack gap={0} style={{ flex: 1, minWidth: 0 }}>
                          <Text fw={600} size="sm" truncate>{it.title}</Text>
                          {it.category && (
                            <Text size="xs" c="dimmed">{it.category}</Text>
                          )}
                        </Stack>
                        {onToggleStar && (
                          <StarToggle
                            on={starred.includes(it.id) || lockedStars.includes(it.id)}
                            locked={lockedStars.includes(it.id)}
                            onToggle={() => onToggleStar(it)}
                          />
                        )}
                        {it.badge && (
                          <Badge
                            size="xs"
                            variant="light"
                            color={it.badge.color ?? 'grape'}
                            style={{ flexShrink: 0, textTransform: 'none' }}
                          >
                            {it.badge.label}
                          </Badge>
                        )}
                        {(it.infoBody || infoFetch) && (
                          <CardBodyInfo
                            body={it.infoBody}
                            title={it.title}
                            uri={it.infoUri || it.id}
                            fetch={it.infoBody ? undefined : () => infoFetch!(it)}
                          />
                        )}
                      </Group>
                      {it.description && (
                        <Text size="xs" c="dimmed" lineClamp={3}>
                          {it.description}
                        </Text>
                      )}
                      {it.hashtags && it.hashtags.length > 0 && (
                        <Group gap={4} wrap="wrap">
                          {it.hashtags.slice(0, 4).map((tag) => (
                            <Badge key={tag} size="xs" variant="default" color="gray">
                              {tag}
                            </Badge>
                          ))}
                          {it.hashtags.length > 4 && (
                            <Badge size="xs" variant="default" color="gray">
                              +{it.hashtags.length - 4}
                            </Badge>
                          )}
                        </Group>
                      )}
                      {it.meta}
                    </Stack>
                  </Card>
                )
              })}
            </SimpleGrid>
          </ScrollArea>
        )}
        {narrowed.more > 0 && (
          <Anchor
            component="button"
            size="sm"
            mt="xs"
            style={{ flexShrink: 0, alignSelf: 'flex-start' }}
            onClick={() => setShowAll(true)}
            data-guide="cards.show-all"
          >
            {t('showAllTasks', { count: narrowed.more })}
          </Anchor>
        )}
      </Box>
    </Stack>
  )
}

/**
 * A small "?" button that opens a popover with the item's rich body (the
 * markdown description). Click toggles it; clicks are stopped from bubbling
 * so the enclosing card doesn't fire its onClick.
 */
function CardBodyInfo({ body, title, uri, fetch }: {
  body?: string
  title: string
  uri?: string
  /** Loads the body the first time the popover opens. Absent when the caller
   *  already has it. */
  fetch?: () => Promise<string>
}) {
  const [opened, setOpened] = useState(false)
  const [loaded, setLoaded] = useState<string | undefined>(body)
  const [loading, setLoading] = useState(false)
  const tw = useT('workspace')
  const openAndLoad = () => {
    const next = !opened
    setOpened(next)
    // ONE FETCH, on first open. A list of forty actions must not read forty
    // bodies to draw itself — that cost is exactly what made the task gallery
    // take four seconds.
    if (next && loaded === undefined && fetch && !loading) {
      setLoading(true)
      fetch()
        .then((b) => setLoaded(b || `_${tw('cardInfo.noDescription')}_`))
        .catch((e) => setLoaded(tw('cardInfo.loadFailed', { error: String(e) })))
        .finally(() => setLoading(false))
    }
  }
  return (
    <Popover
      opened={opened}
      onChange={setOpened}
      position="bottom-end"
      withArrow
      shadow="md"
      width={420}
      withinPortal
    >
      <Popover.Target>
        <ActionIcon
          variant="subtle"
          color="gray"
          size="sm"
          aria-label={tw('cardInfo.show')}
          style={{ flexShrink: 0 }}
          onClick={(e) => {
            e.stopPropagation()
            openAndLoad()
          }}
        >
          <IconInfoCircle size={16} />
        </ActionIcon>
      </Popover.Target>
      <Popover.Dropdown onClick={(e) => e.stopPropagation()}>
        <Stack gap="xs">
          <Text size="sm" fw={600}>{title}</Text>
          {uri && <Code style={{ fontSize: 11 }}>{uri}</Code>}
          <ScrollArea.Autosize mah={320}>
            {loading && loaded === undefined
              ? <Text size="xs" c="dimmed">{tw('cardInfo.loading')}</Text>
              : <Markdown text={loaded ?? ''} />}
          </ScrollArea.Autosize>
        </Stack>
      </Popover.Dropdown>
    </Popover>
  )
}

interface QuickActionsCardGridProps {
  items: CardListItem[]
  onItemClick?: (item: CardListItem) => void
  /** With it, each card carries a ✕ that removes it, in one click. */
  onRemove?: (item: CardListItem) => void
}

/**
 * QuickActionsCardGrid — the standalone cards row used by the Node
 * tab (and the Global tab) when quick actions need to live OUTSIDE
 * the vertical-strip body. Same styling as the SearchableCardGrid's
 * embedded quick-actions block (single source of truth for the look
 * + feel: icon + title + description, Tabler icon name resolved
 * dynamically, accent color via Mantine palette).
 *
 * No title, no search, no filter — those are the SearchableCardGrid's
 * responsibilities. Pass an empty `items` and the component renders
 * nothing (the caller decides when to skip the wrapper).
 */
export function QuickActionsCardGrid({ items, onItemClick, onRemove }: QuickActionsCardGridProps) {
  const t = useT('cards')
  if (items.length === 0) return null
  return (
    <SimpleGrid cols={{ base: 2, sm: 3, md: 4 }} spacing="sm">
      {items.map((q) => {
        const Icon = resolveTablerIcon(q.iconName, IconBolt)
        return (
          <Card
            key={q.id}
            withBorder
            radius="md"
            padding="md"
            style={{
              cursor: q.disabled ? 'default' : 'pointer',
              opacity: q.disabled ? 0.5 : 1,
            }}
            onClick={() => { if (!q.disabled) onItemClick?.(q) }}
            role="button"
            tabIndex={q.disabled ? -1 : 0}
            aria-disabled={q.disabled || undefined}
            aria-label={q.title}
            onKeyDown={activateOnKey(() => { if (!q.disabled) onItemClick?.(q) })}
            data-guide="quick-action"
            data-guide-name={q.id}
            data-element={isElementId(q.id) ? q.id : undefined}
          >
            <Group gap="xs" mb={4} wrap="nowrap">
              <Icon
                size={18}
                color={q.accent ? `var(--mantine-color-${q.accent}-6)` : undefined}
              />
              <Text fw={600} size="sm" truncate style={{ flex: 1, minWidth: 0 }}>
                {q.title}
              </Text>
              {onRemove && (
                <Tooltip label={t('removeQuick')} withArrow>
                  <ActionIcon
                    variant="subtle"
                    color="gray"
                    size="sm"
                    aria-label={t('removeQuick')}
                    data-guide="quick-action.remove"
                    onClick={(e) => { e.stopPropagation(); onRemove(q) }}
                    onKeyDown={(e) => e.stopPropagation()}
                    style={{ flexShrink: 0 }}
                  >
                    <IconX size={14} />
                  </ActionIcon>
                </Tooltip>
              )}
            </Group>
            {q.description && (
              <Text size="xs" c="dimmed" lineClamp={2}>
                {q.description}
              </Text>
            )}
          </Card>
        )
      })}
    </SimpleGrid>
  )
}

/** A card's star: on puts the item in the quick actions; a locked star is set
 *  by the item itself (a task tagged quick-action) and says so. */
/** The order a list is shown in: name, category, source. */
export function SortSelect({ value, onChange }: { value?: SortKey; onChange: (key: SortKey) => void }) {
  const t = useT('cards')
  return (
    <Select
      size="sm"
      aria-label={t('sortBy')}
      leftSection={<IconArrowsSort size={14} />}
      data={SORT_KEYS.map((k) => ({ value: k, label: t(`sort.${k}`) }))}
      value={value ?? 'name'}
      onChange={(v) => { if (v) onChange(v as SortKey) }}
      allowDeselect={false}
      w={150}
      comboboxProps={{ withinPortal: true }}
    />
  )
}

export function StarToggle({ on, locked, onToggle }: { on: boolean; locked: boolean; onToggle: () => void }) {
  const t = useT('cards')
  const label = locked ? t('starLocked') : on ? t('unstar') : t('star')
  return (
    <Tooltip label={label} withArrow>
      <ActionIcon
        variant="subtle"
        color={on ? 'yellow' : 'gray'}
        size="sm"
        aria-label={label}
        aria-pressed={on}
        data-guide="card.star"
        disabled={locked}
        onClick={(e) => { e.stopPropagation(); onToggle() }}
        onKeyDown={(e) => e.stopPropagation()}
        style={{ flexShrink: 0 }}
      >
        {on ? <IconStarFilled size={14} /> : <IconStar size={14} />}
      </ActionIcon>
    </Tooltip>
  )
}

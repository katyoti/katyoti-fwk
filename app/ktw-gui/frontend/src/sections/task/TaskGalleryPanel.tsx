// SPDX-License-Identifier: Apache-2.0
/**
 * TaskGalleryPanel — default-when-idle home screen for the central panel.
 *
 * Card grid showing all available tasks with rich metadata (icon, name,
 * description, capability, category badge, hashtags). Top strip provides
 * search + tag-chip filtering (AND across selected tags). Click a card →
 * opens RunTaskPanel pre-filled with the task and the run mode currently
 * selected in the sidebar.
 *
 * Mounted by NodeView (scoped to a context-tree node) — the node's
 * `taskNames` field is passed as a prop to filter the card grid.
 */
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
  Loader,
  MultiSelect,
  Popover,
  ScrollArea,
  SimpleGrid,
  Stack,
  Text,
  TextInput,
  Title,
} from '@mantine/core'
import { EmptyState } from '@ktw/display-lib/ui-core/components/other/index.ts'
import { tablerIcon } from '@ktw/display-lib/ui-core/components/icons/index.ts'
import { IconPlayerPlay, IconSearch, IconForms, IconInfoCircle } from '@tabler/icons-react'
import { useAppStore } from '../../store'
import { categoryToColor } from '../../utils/categoryColor'
import { Markdown } from '../../components/Markdown'
import { activateOnKey } from '../../guide/anchors'
import { ListTasksDetail } from '../../../wailsjs/go/main/App'
import type { main } from '../../../wailsjs/go/models'
import { useHostLoad } from '../../hooks/useHostLoad'
import { SectionFailed } from '@ktw/display-lib/ui-builders/page/nodes/section-load.tsx'
import { useT, type TFn } from '../../i18n'
import { useListPrefs } from '../../hooks/useListPrefs'
import { useDisplayLevel, useUniverseGui } from '../../hooks/useUniverseGui'
import { firstByHashtags, galleryFirstLevel } from '../shell/universeGui'
import { isTagged, quickId, sortItems } from './listPrefs'
import { SortSelect, StarToggle } from '../workspace/SearchableCardGrid'

// ── Helpers ───────────────────────────────────────────────────────────────

/**
 * Resolve a Tabler icon name (e.g., "hammer") to its React component.
 * Falls back to IconPlayerPlay if the name is unknown.
 */
function resolveTablerIcon(iconName?: string) {
  return tablerIcon(iconName, IconPlayerPlay)
}

/**
 * Aggregate all unique hashtags across the task list, sorted by frequency
 * (most-used first), so the chip strip surfaces the most signal-dense tags.
 */
function collectHashtags(tasks: main.TaskInfo[]): string[] {
  const counts = new Map<string, number>()
  for (const t of tasks) {
    for (const tag of t.hashtags ?? []) {
      counts.set(tag, (counts.get(tag) ?? 0) + 1)
    }
  }
  return [...counts.entries()]
    .sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]))
    .map(([tag]) => tag)
}

/**
 * Aggregate all unique categories across the task list, sorted by frequency
 * (most-used first), for the one-click category toggle row.
 */
function collectCategories(tasks: main.TaskInfo[]): string[] {
  const counts = new Map<string, number>()
  for (const t of tasks) {
    if (t.category) counts.set(t.category, (counts.get(t.category) ?? 0) + 1)
  }
  return [...counts.entries()]
    .sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]))
    .map(([cat]) => cat)
}

/**
 * Apply the optional taskNames filter (when provided, only tasks whose
 * `name` is in the set survive — used by NodeView to scope to a tree node).
 * Then apply search + tag-chip filters. Matches are case-insensitive
 * substrings against name + description + hashtags + category. Tag chips
 * are AND-combined: every selected tag must appear in the task's hashtags.
 */
function filterTasks(
  tasks: main.TaskInfo[],
  query: string,
  selectedTags: string[],
  selectedCategories: string[],
  taskNames: string[] | undefined
): main.TaskInfo[] {
  const q = query.trim().toLowerCase()
  const allowed = taskNames ? new Set(taskNames) : null
  return tasks.filter((t) => {
    // Global gallery (no node scope): a TARGETED task (kind node/item/
    // item-collection) needs a context to run and belongs on that node/item —
    // it must NOT flood the global launcher (ms-action-system). kind:global /
    // unset stay.
    if (allowed === null) {
      const k = t.kind ?? ''
      if (k === 'node' || k === 'item' || k === 'item-collection') return false
    }
    if (allowed !== null && !allowed.has(t.name)) return false
    // Category toggles are OR-combined: a task matches if its category is
    // any of the selected ones. No selection = no category filter.
    if (selectedCategories.length > 0) {
      if (!t.category || !selectedCategories.includes(t.category)) return false
    }
    if (selectedTags.length > 0) {
      const tags = t.hashtags ?? []
      for (const sel of selectedTags) {
        if (!tags.includes(sel)) return false
      }
    }
    if (!q) return true
    // name stays in the haystack so a task is findable by its runnable id
    // even when the visible label is the title.
    const haystack = [
      t.name,
      t.title ?? '',
      t.description ?? '',
      t.category ?? '',
      ...(t.hashtags ?? []),
    ]
      .join(' ')
      .toLowerCase()
    return haystack.includes(q)
  })
}

// ── Main component ────────────────────────────────────────────────────────

interface TaskGalleryPanelProps {
  /** When provided, only tasks whose name is in this set are shown.
   *  Used by NodeView to scope the gallery to a context-tree node.
   *  When undefined, all backend tasks are shown. When [], the gallery
   *  is empty and shows a "no tasks for this node" empty state. */
  taskNames?: string[]
}

const NO_TASKS: main.TaskInfo[] = []

export function TaskGalleryPanel({ taskNames }: TaskGalleryPanelProps = {}) {
  const openRunTask  = useAppStore((s) => s.openRunTask)
  const universeName = useAppStore((s) => s.displayedUniverseName)
  // Re-fetch on every (re-)activation, even if the universe name is
  // unchanged — closes the hub-catalog hydration race.
  const universeRev  = useAppStore((s) => s.universeRevision)

  const t = useT('cards')
  const [query, setQuery] = useState('')
  const [selectedTags, setSelectedTags] = useState<string[]>([])
  const [selectedCategories, setSelectedCategories] = useState<string[]>([])

  // The task list through the host's one load, shared with the
  // Home tab's; an empty list is asked again while the core starts.
  const listed = useHostLoad<main.TaskInfo[]>({
    universe: universeName, key: 'tasks', revision: universeRev,
    load: async () => (await ListTasksDetail(universeName!)) ?? [],
    isEmpty: (arr) => arr.length === 0,
  })
  const tasks = listed.data ?? NO_TASKS
  const loading = listed.loading

  // Scope to taskNames (if NodeView provided one) BEFORE collecting tag
  // chips, so the chip strip only shows tags relevant to this node.
  const scopedTasks = useMemo(() => {
    if (taskNames === undefined) return tasks
    const allowed = new Set(taskNames)
    return tasks.filter((t) => allowed.has(t.name))
  }, [tasks, taskNames])

  const allTags = useMemo(() => collectHashtags(scopedTasks), [scopedTasks])
  const allCategories = useMemo(() => collectCategories(scopedTasks), [scopedTasks])
  const listPrefs = useListPrefs(universeName)
  const sort = listPrefs.sortOf('tasks')
  // The universe's priority tags first, in their order; at Essentiel only the
  // tasks carrying one until « Voir toutes les tâches ».
  const first = useUniverseGui(universeName).first
  const level = useDisplayLevel()
  const [showAll, setShowAll] = useState(false)
  // Narrowed over what the gallery lists (its node scope and kind exclusion
  // first): a node's own Tasks tab is never narrowed, and the count is what
  // « Voir toutes les tâches » adds.
  const listable = useMemo(() => filterTasks(tasks, '', [], [], taskNames), [tasks, taskNames])
  const narrowed = useMemo(
    () => galleryFirstLevel(listable, taskNames !== undefined, first, level, showAll),
    [listable, taskNames, first, level, showAll],
  )
  const visible = useMemo(
    () => firstByHashtags(sortItems(
      filterTasks(narrowed.items, query, selectedTags, selectedCategories, taskNames).map((x) => ({ ...x, source: x.provider })),
      sort,
    ), first),
    [narrowed, query, selectedTags, selectedCategories, taskNames, sort, first]
  )

  function handleClick(task: main.TaskInfo) {
    // Prefer the full URI when present so RunTaskInUniverse hits the
    // exact-match stage of resolveTask. Bare names still work via the
    // bare-name-unique disambiguation, but URI is unambiguous.
    openRunTask({
      name:           task.uri || task.name,
      description:    task.description ?? '',
      title:          task.title,
      provider:       task.provider,
      taskName:       task.name,
      uri:            task.uri || task.name,
      inputSchemaURI: task.inputSchemaURI,
      iconName:       task.iconName,
      tags:           task.tags ?? undefined,
    })
  }

  if (listed.error) return <SectionFailed id="tasks" message={listed.error} onRetry={listed.retry} />
  if (loading) {
    return (
      <Group justify="center" py="xl">
        <Loader size="sm" />
      </Group>
    )
  }

  return (
    <Stack gap="sm" p="md" style={{ height: '100%', minHeight: 0 }}>
      {/* Header + search + tag chips. Title intentionally omitted — every
         host context (HomeTab section header, GalleryTabs `Tasks` tab,
         NodeView scoped gallery) already labels this surface, so the
         inner Title was producing a "Tasks / Tasks" duplication.
         Surfaced 2026-05-20. */}
      {/* Search + compact tag filter on the same row — replaces the
          prior wide always-visible Chip.Group that took 50px+ of
          vertical space on tag-rich workspaces. Tag MultiSelect shows
          as a single line with selected pills inside; opens
          searchable autocomplete on click. cwm-v3 UX restructure
          2026-06-06. */}
      <Stack gap="xs" style={{ flexShrink: 0 }}>
        <Group justify="space-between" align="center">
          <Text size="xs" c="dimmed">
            {t('count', { visible: visible.length, total: tasks.length })}
          </Text>
        </Group>

        <Group gap="xs" wrap="nowrap" align="center">
          <TextInput
            size="sm"
            placeholder={t('searchTasks')}
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
          <SortSelect value={sort} onChange={(k) => listPrefs.setSort('tasks', k)} />
        </Group>

        {/* One-click category filter — toggle a category on/off. Multiple
            selected categories OR together. */}
        {allCategories.length > 0 && (
          <Chip.Group multiple value={selectedCategories} onChange={setSelectedCategories}>
            <Group gap={6} wrap="wrap">
              {allCategories.map((cat) => (
                <Chip key={cat} value={cat} size="xs" variant="light" color={categoryToColor(cat)}>
                  {cat}
                </Chip>
              ))}
            </Group>
          </Chip.Group>
        )}
      </Stack>

      {/* Card grid */}
      <ScrollArea style={{ flex: 1, minHeight: 0 }}>
        {visible.length === 0 ? (
          <NoTasks
            hasTasks={tasks.length > 0}
            scoped={taskNames !== undefined}
            scopedEmpty={taskNames !== undefined && scopedTasks.length === 0}
          />
        ) : (
          <SimpleGrid cols={{ base: 1, sm: 2, lg: 3, xl: 4 }} spacing="sm">
            {visible.map((task) => (
              <TaskCard
                key={task.name}
                task={task}
                onClick={handleClick}
                starred={(isTagged(task) && !listPrefs.removed.includes(quickId(task))) || listPrefs.starred.includes(task.uri || task.name)}
                starLocked={isTagged(task) && !listPrefs.removed.includes(quickId(task))}
                onToggleStar={() => listPrefs.toggleStar(task.uri || task.name)}
              />
            ))}
          </SimpleGrid>
        )}
      </ScrollArea>
      {narrowed.more > 0 && (
        <Anchor
          component="button"
          size="sm"
          style={{ flexShrink: 0, alignSelf: 'flex-start' }}
          onClick={() => setShowAll(true)}
          data-guide="cards.show-all"
        >
          {t('showAllTasks', { count: narrowed.more })}
        </Anchor>
      )}
    </Stack>
  )
}

// ── Card ──────────────────────────────────────────────────────────────────

interface TaskCardProps {
  task: main.TaskInfo
  onClick: (task: main.TaskInfo) => void
  /** In Home's quick actions: starred by the person, or tagged (locked). */
  starred: boolean
  starLocked: boolean
  onToggleStar: () => void
}

function TaskCard({ task, onClick, starred, starLocked, onToggleStar }: TaskCardProps) {
  const t = useT('cards')
  const Icon = resolveTablerIcon(task.iconName)
  const categoryColor = categoryToColor(task.category)

  return (
    <Card
      withBorder
      shadow="sm"
      padding="sm"
      radius="md"
      style={{ cursor: 'pointer', height: '100%' }}
      onClick={() => onClick(task)}
      role="button"
      tabIndex={0}
      aria-label={task.title || task.name}
      onKeyDown={activateOnKey(() => onClick(task))}
      data-guide="task.card"
      data-guide-name={task.name}
      data-guide-label={task.title || task.name}
    >
      <Stack gap="xs" style={{ height: '100%' }}>
        {/* Header */}
        <Group gap="xs" align="flex-start" wrap="nowrap">
          <Box style={{ flexShrink: 0, paddingTop: 2 }}>
            <Icon size={20} />
          </Box>
          <Stack gap={2} style={{ flex: 1, minWidth: 0 }}>
            <Text size="sm" fw={600} truncate>
              {task.title || task.name}
            </Text>
            {task.provider && (
              <Text size="xs" c="dimmed" truncate>
                {task.provider}
              </Text>
            )}
            <Group gap={4}>
              {task.capability && task.capability !== '(default)' && (
                <Badge size="xs" variant="light" color="gray">
                  {task.capability}
                </Badge>
              )}
              <Badge size="xs" variant="outline" color="gray">
                {task.type}
              </Badge>
              {/* Form-runnable: the task declares an input-schema, so
                  clicking it opens the auto-generated form instead of
                  the free-text prompt. */}
              {task.inputSchemaURI && (
                <Badge
                  size="xs"
                  variant="light"
                  color="teal"
                  leftSection={<IconForms size={10} />}
                >
                  {t('formBadge')}
                </Badge>
              )}
            </Group>
          </Stack>
          <StarToggle on={starred} locked={starLocked} onToggle={onToggleStar} />
          {/* "?" affordance — reveals the task's full markdown body. The
              card's onClick launches the task, so stop propagation here. */}
          {task.body && (
            <TaskBodyInfo
              body={task.body}
              title={task.title || task.name}
              uri={task.uri || task.name}
            />
          )}
        </Group>

        {/* Description */}
        {task.description && (
          <Text size="xs" c="dimmed" lineClamp={3} style={{ flex: 1 }}>
            {task.description}
          </Text>
        )}

        {/* Footer: category + hashtags */}
        <Group gap={4} wrap="wrap">
          {task.category && (
            <Badge size="xs" variant="filled" color={categoryColor}>
              {task.category}
            </Badge>
          )}
          {(task.hashtags ?? []).slice(0, 4).map((tag) => (
            <Badge key={tag} size="xs" variant="dot" color="gray">
              {tag}
            </Badge>
          ))}
          {(task.hashtags ?? []).length > 4 && (
            <Text size="xs" c="dimmed">
              +{(task.hashtags ?? []).length - 4}
            </Text>
          )}
        </Group>
      </Stack>
    </Card>
  )
}

// ── Body info "?" affordance ────────────────────────────────────────────────

/**
 * A small "?" button that opens a popover with the task's full markdown body
 * (the rich description). Click toggles the popover; clicks are stopped from
 * bubbling so the enclosing card doesn't launch the task.
 */
function TaskBodyInfo({ body, title, uri }: { body: string; title: string; uri: string }) {
  const t = useT('cards')
  const [opened, setOpened] = useState(false)
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
          aria-label={t('showDescription')}
          style={{ flexShrink: 0 }}
          onClick={(e) => {
            e.stopPropagation()
            setOpened((o) => !o)
          }}
        >
          <IconInfoCircle size={16} />
        </ActionIcon>
      </Popover.Target>
      <Popover.Dropdown onClick={(e) => e.stopPropagation()}>
        <Stack gap="xs">
          <Text size="sm" fw={600}>{title}</Text>
          <Code style={{ fontSize: 11 }}>{uri}</Code>
          <ScrollArea.Autosize mah={320}>
            <Markdown text={body} />
          </ScrollArea.Autosize>
        </Stack>
      </Popover.Dropdown>
    </Popover>
  )
}

// ── Empty state ───────────────────────────────────────────────────────────

interface NoTasksProps {
  hasTasks:    boolean
  scoped:      boolean
  scopedEmpty: boolean
}

/** The task builder, named in bold inside a translated sentence. */
const TASK_BUILDER = 'task-builder-master'
function withTaskBuilder(t: TFn, key: string) {
  const mark = '\u0001'
  const [before, after = ''] = t(key, { task: mark }).split(mark)
  return <>{before}<Text component="span" fw={600}>{TASK_BUILDER}</Text>{after}</>
}

/** No task to show: none in the universe, none on the node, or none matching. */
function NoTasks({ hasTasks, scoped, scopedEmpty }: NoTasksProps) {
  const t = useT('cards')
  if (!hasTasks) return <EmptyState icon="hammer" iconSize={48} message={<>{t('noTasks')}<br />{withTaskBuilder(t, 'tryBuilder')}</>} />
  if (scoped && scopedEmpty) return <EmptyState icon="hammer" message={<>{t('noNodeTasks')}<br />{withTaskBuilder(t, 'pickNode')}</>} />
  return <EmptyState message={t('noMatch')} />
}

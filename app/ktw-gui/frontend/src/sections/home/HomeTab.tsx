// SPDX-License-Identifier: Apache-2.0
/**
 * HomeTab — the Global tab (user-facing name). Vertical-strip layout
 * symmetric with the Node tab so users learn one navigation idiom.
 *
 * Strip entries (cwm-v3 UX restructure 2026-06-07 — single "Actions"
 * entry merges what were previously separate "Quick Actions" +
 * "Tasks" panels):
 *   - Actions (default): SearchableCardGrid with quick action cards
 *     on top (Terminal / Drawio / Debug & tests / Start chat /
 *     tagged quick-action tasks) + the searchable global task list
 *     below with compact tag filter.
 *   - Scheduler: the scheduler console (../scheduler/SchedulerConsole) —
 *     each task group with its runs inside, the run events live, the
 *     to-dos by module, the history of ended runs; a run opens in a drawer.
 *     The count of active runs is its badge.
 *   - Errors: every object CWM found in error and every file it could
 *     not read, with a red count when there are any.
 *   - Chat: only after "Start chat" clicked in Quick Actions.
 *
 * A universe's Univers.yaml `gui` section hides entries (`hide`, by id) unless
 * the person turned Mode expert on, and lists the tasks with a `first`
 * hashtag before the others (../shell/universeGui.ts).
 *
 * Skills moved out — accessible via Debug & tests → Skills tab.
 */

import { useEffect, useMemo, useState } from 'react'
import { Box, Text } from '@mantine/core'
import {
  IconAlertTriangle,
  IconBolt,
  IconBug,
  IconMessage,
  IconActivity,
  IconTerminal2,
  IconVectorTriangle,
} from '@tabler/icons-react'
import { useAppStore } from '../../store'
import { useTabsStore, type GlobalTabPayload } from '../../store/tabsSlice'
import { ListTasksDetail, WorkspaceObjectErrorList } from '../../../wailsjs/go/main/App'
import type { main } from '../../../wailsjs/go/models'
import { useHostLoad } from '../../hooks/useHostLoad'
import { useDebugLauncher } from '../../hooks/useDebugLauncher'
import { VerticalTabBody, type StripEntry } from '../workspace/VerticalTabBody'
import { SearchableCardGrid, type CardListItem } from '../workspace/SearchableCardGrid'
import { ObjectErrorsPanel } from './ObjectErrorsPanel'
import { SchedulerConsole } from '../scheduler/SchedulerConsole'
import { useActiveRunCount } from '../scheduler/useScheduler'
import { GlobalPanelsFailed, GlobalPanelView, globalPanelEntries, useGlobalPanels } from './GlobalPanels'
import { usePanelCounts, withCounts } from '../../hooks/usePanelCounts'
import { useT } from '../../i18n'
import { FirstRunChoice } from '../../guide/FirstRunChoice'
import { useListPrefs } from '../../hooks/useListPrefs'
import { defaultOf, isRemoved, quickId, quickTasks } from '../task/listPrefs'
import { useDisplayLevel, useUniverseGui } from '../../hooks/useUniverseGui'
import { homeElementId } from '../../elements'
import { shownOnStrip, useRailScreen } from '../shell/RailScreen'

/**
 * Built-in quick-action descriptors. `tabKind` resolves to which tab
 * gets opened. Skills moved to the Debug surface; left here as a
 * comment so the relocation is discoverable.
 */
interface BuiltInAction {
  id: string
  /** its title and description: `home:builtIn.<id>.title|description` */
  iconName: string
  tabKind: 'run-shell' | 'run-drawio'
}

const BUILT_IN_ACTIONS: BuiltInAction[] = [
  {
    id: 'term',
    iconName: 'terminal-2',
    tabKind: 'run-shell',
  },
  {
    id: 'drawio',
    iconName: 'vector-triangle',
    tabKind: 'run-drawio',
  },
]

const NO_TASKS: main.TaskInfo[] = []

export function HomeTab() {
  const t = useT('home')
  const universeName    = useAppStore((s) => s.displayedUniverseName)
  const listPrefs       = useListPrefs(universeName)
  // What the universe's Univers.yaml `gui` hides from the strip and which task
  // hashtags it lists first, unless the person turned Mode expert on.
  const universeGui     = useUniverseGui(universeName)
  const level           = useDisplayLevel()
  const universeRev     = useAppStore((s) => s.universeRevision)
  const openRunTask     = useAppStore((s) => s.openRunTask)
  const openTab         = useTabsStore((s) => s.openTab)
  const updateTabPayload = useTabsStore((s) => s.updateTabPayload)
  const { launchDebug }  = useDebugLauncher()
  // A GUI that opens straight on a universe (the demo a first install sets up)
  // never shows the picker: the welcome tour is offered here too, until done.
  const userUniverses   = useAppStore((s) => s.universes.filter((u) => !u.protected).length)

  // Read the Home tab's payload to drive activeKind + chatActive.
  const homeTab = useTabsStore((s) =>
    universeName
      ? s.getForUniverse(universeName).openTabs.find((t) => t.id === 'home')
      : undefined,
  )
  const payload = (homeTab?.payload ?? {}) as GlobalTabPayload & { chatActive?: boolean }
  // An entry the universe hides is not opened: Home falls back on its actions.
  const activeKind = payload.activeKind && !universeGui.hidden(homeElementId(payload.activeKind)) ? payload.activeKind : 'actions'

  const setActiveKind = (id: string) => {
    if (!universeName) return
    updateTabPayload(universeName, 'home', { ...payload, activeKind: id })
  }

  const startChat = () => {
    if (!universeName) return
    updateTabPayload(
      universeName, 'home',
      { ...payload, chatActive: true, activeKind: 'chat' },
    )
  }

  // The task list through the host's one load, shared with the
  // task gallery's; an empty list is asked again while the core starts.
  const tasks = useHostLoad<main.TaskInfo[]>({
    universe: universeName, key: 'tasks', revision: universeRev,
    load: async () => (await ListTasksDetail(universeName!)) ?? [],
    isEmpty: (arr) => arr.length === 0,
  }).data ?? NO_TASKS

  // The Global tab shows every task the catalog serves. Node-relative
  // scoping belongs in the per-node tab, not here — and the previous
  // scoping read a hardcoded placeholder context tree, which silently
  // dropped any catalog task not in that fixed list.
  const scopedTasks = tasks

  // The project's errors, re-read with every write (the tree revision).
  const treeRev = useAppStore((s) => s.workspaceTreeRevision)
  const [objectErrors, setObjectErrors] = useState<main.WorkspaceObjectErrors | null>(null)
  useEffect(() => {
    if (!universeName) return
    let cancelled = false
    WorkspaceObjectErrorList(universeName)
      .then((r) => { if (!cancelled) setObjectErrors(r) })
      .catch(() => { if (!cancelled) setObjectErrors(null) })
    return () => { cancelled = true }
  }, [universeName, universeRev, treeRev])
  const errorCount = objectErrors?.total ?? 0
  const activeRuns = useActiveRunCount(universeName)

  // ── Strip entries ──────────────────────────────────────────────
  const allEntries: StripEntry[] = [
    {
      id:     'actions',
      label:  t('strip.actions'),
      accent: 'blue',
      badge:  scopedTasks.length > 0 ? String(scopedTasks.length) : '',
      icon:   <IconBolt size={14} />,
    },
    {
      id:     'runs',
      label:  t('strip.runs'),
      accent: activeRuns > 0 ? 'blue' : 'gray',
      badge:  activeRuns > 0 ? String(activeRuns) : '',
      icon:   <IconActivity size={14} />,
    },
    {
      id:     'errors',
      label:  t('strip.errors'),
      accent: errorCount > 0 ? 'red' : 'gray',
      alert:  errorCount > 0 ? String(errorCount) : undefined,
      icon:   <IconAlertTriangle size={14} />,
    },
  ]
  // The declared panels marked global (object-pane GlobalPanels), after the
  // built-in entries.
  // The panels the rail's Inbox screen stands in for, as the backend names
  // them, leave the strip.
  const railScreen = useRailScreen(universeName)
  // A failed load is the error card with « Réessayer » below the strip's
  // entries, never a strip silently left without its panels.
  const { panels: declaredPanels, part: panelsPart } = useGlobalPanels(universeName, universeRev)
  const globalPanels = useMemo(() => declaredPanels.filter((p) => shownOnStrip(p.id, railScreen)), [declaredPanels, railScreen])
  // A panel's declared count (« À faire »: what waits on the person) is its badge.
  const panelCounts = usePanelCounts(universeName, globalPanels, `${universeRev}|${treeRev}`)
  allEntries.push(...withCounts(globalPanelEntries(globalPanels), panelCounts))
  if (payload.chatActive) {
    allEntries.push({
      id:     'chat',
      label:  t('strip.chat'),
      accent: 'violet',
      icon:   <IconMessage size={14} />,
    })
  }
  // The universe's Univers.yaml `gui.hide` leaves entries out (universeGui.ts),
  // and so does its view when its tags list the panels: each
  // entry by its element id, `home:<entry>`.
  const entries = allEntries.filter((e) => !universeGui.hidden(homeElementId(e.id)))

  // ── Map data → CardListItem for the generic grid ───────────────
  const chatActive = !!payload.chatActive

  const quickActions: CardListItem[] = []
  // Built-ins (Terminal / Drawio), each an element (`quick:<name>`) the
  // universe's view can hide; a card's id is its element id.
  for (const a of BUILT_IN_ACTIONS) {
    if (universeGui.hidden(`quick:${a.id}`)) continue
    quickActions.push({
      id:          `quick:${a.id}`,
      title:       t(`builtIn.${a.id}.title`),
      description: t(`builtIn.${a.id}.description`),
      iconName:    a.iconName,
      accent:      'blue',
    })
  }
  // Debug & tests routes to the existing DebugRoot panel.
  if (!universeGui.hidden('quick:debug')) quickActions.push({
    id:          'quick:debug',
    title:       t('builtIn.debug.title'),
    description: t('builtIn.debug.description'),
    iconName:    'bug',
    accent:      'gray',
  })
  // System Start-chat — parallels the Node tab Quick Action.
  if (!universeGui.hidden('quick:start-chat')) quickActions.push({
    id:          'quick:start-chat',
    title:       t('builtIn.chat.title'),
    description: chatActive ? t('builtIn.chat.active') : t('builtIn.chat.description'),
    iconName:    'message',
    accent:      'violet',
    disabled:    chatActive,
  })
  // Task quick actions: the universe's defaults - the tasks its
  // view lists at the person's level when its tags give some, else those
  // tagged quick-action in their TASK.md - then those the person starred in
  // this universe, less those the person removed (listPrefs.quickTasks).
  const view = universeGui.view
  const defaults = view && !view.all ? view.allowed.filter((id) => id.startsWith('quick:task/')) : undefined
  const isDefault = defaultOf(defaults)
  // A task's quick action is `quick:task/<provider>/<task>`, its task found
  // back by that id.
  const quickTask = new Map<string, main.TaskInfo>()
  for (const task of quickTasks(tasks, listPrefs.starred, listPrefs.removed, defaults)) {
    quickTask.set(quickId(task), task)
    quickActions.push({
      id:          quickId(task),
      title:       task.title || task.name,
      description: task.description,
      infoBody:    task.body,
      iconName:    task.iconName,
      accent:      'blue',
    })
  }

  // Items grid: every task as a clickable card (scoped to the active
  // node when one is selected — same behavior the prior
  // TaskGalleryPanel had).
  const taskItems: CardListItem[] = scopedTasks.map((task) => ({
    id:          task.uri || task.name,
    title:       task.title || task.name,
    description: task.description,
    infoBody:    task.body,
    infoUri:     task.uri || task.name,
    hashtags:    task.hashtags,
    audience:    task.audience,
    kind:        task.kind,
    types:       task.types,
    category:    task.category,
    iconName:    task.iconName,
    // name kept in the search haystack so a task stays findable by its
    // runnable id even though the card shows the title.
    searchSlug:  [task.name, task.provider, task.uri].filter(Boolean).join(' '),
    source:      task.provider,
  }))

  const dispatchClick = (item: CardListItem) => {
    // Quick-action: built-ins by id; chat / debug routed inline.
    if (item.id === 'quick:debug') {
      launchDebug()
      return
    }
    if (item.id === 'quick:start-chat') {
      if (!chatActive) startChat()
      return
    }
    if (item.id === 'quick:term' && universeName) {
      openTab(universeName, { id: 'term',   kind: 'run-shell',  label: t('builtIn.term.title'), closeable: true })
      return
    }
    if (item.id === 'quick:drawio' && universeName) {
      openTab(universeName, { id: 'drawio', kind: 'run-drawio', label: t('builtIn.drawio.title'), closeable: true })
      return
    }
    if (quickTask.has(item.id)) {
      const task = quickTask.get(item.id)
      if (task) openRunTask({
        name: task.uri || task.name,
        description: task.description ?? '',
        title: task.title,
        provider: task.provider,
        taskName: task.name,
        uri: task.uri || task.name,
        inputSchemaURI: task.inputSchemaURI,
        iconName: task.iconName,
        tags: task.tags ?? undefined,
      })
      return
    }
    // Items grid: cards that match a task — open the run-task UI.
    const task = tasks.find((x) => (x.uri || x.name) === item.id)
    if (task) {
      openRunTask({
        name: task.uri || task.name,
        description: task.description ?? '',
        title: task.title,
        provider: task.provider,
        taskName: task.name,
        uri: task.uri || task.name,
        inputSchemaURI: task.inputSchemaURI,
        iconName: task.iconName,
        tags: task.tags ?? undefined,
      })
    }
  }

  // ── Content panel ──────────────────────────────────────────────
  let content: React.ReactNode
  if (activeKind === 'actions') {
    content = (
      <SearchableCardGrid
        quickActionsTitle={t('actions.quickActions')}
        itemsTitle={t('actions.tasks')}
        quickActions={quickActions.filter((q) => !isRemoved(listPrefs.removed, q.id))}
        onRemoveQuick={(q) => listPrefs.removeQuick(q.id)}
        items={taskItems}
        onItemClick={dispatchClick}
        sort={listPrefs.sortOf('tasks')}
        onSortChange={(k) => listPrefs.setSort('tasks', k)}
        starred={listPrefs.starred}
        lockedStars={tasks.filter((x) => isDefault(x) && !isRemoved(listPrefs.removed, quickId(x))).map((x) => x.uri || x.name)}
        onToggleStar={(it) => listPrefs.toggleStar(it.id)}
        firstTags={universeGui.first}
        level={level}
        emptyDataMessage={t('actions.emptyData')}
        emptyFilteredMessage={t('actions.emptyFiltered')}
      />
    )
  } else if (activeKind === 'runs') {
    content = <SchedulerConsole universeName={universeName ?? ''} />
  } else if (activeKind === 'errors') {
    content = <ObjectErrorsPanel errors={objectErrors} universeName={universeName ?? ''} />
  } else if (globalPanels.some((p) => p.id === activeKind)) {
    const panel = globalPanels.find((p) => p.id === activeKind)!
    content = <GlobalPanelView key={panel.id} panel={panel} universeName={universeName ?? ''} />
  } else if (activeKind === 'chat') {
    content = (
      <Box p="md">
        <Text size="sm" c="dimmed" ta="center" py="md">
          {t('chat.placeholder')}
        </Text>
      </Box>
    )
  } else {
    content = (
      <Box p="md">
        <Text size="sm" c="dimmed">{t('unknownSection', { section: activeKind })}</Text>
      </Box>
    )
  }

  // Suppress unused-icon warnings (used elsewhere or kept for future).
  void IconTerminal2; void IconVectorTriangle; void IconBug

  return (
    <VerticalTabBody
      header={<FirstRunChoice userUniverses={userUniverses} />}
      entries={entries}
      activeId={activeKind}
      onChange={setActiveKind}
      footer={panelsPart.error ? <GlobalPanelsFailed part={panelsPart} /> : undefined}
    >
      {content}
    </VerticalTabBody>
  )
}

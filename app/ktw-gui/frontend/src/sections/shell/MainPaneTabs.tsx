// SPDX-License-Identifier: Apache-2.0
/**
 * MainPaneTabs — right-pane orchestrator. Renders the tab strip + the
 * body of the active tab. Anchor: `Home`. Run tabs are dynamic
 * (Term, Skills, Drawio, plus task tabs).
 *
 * Historical sessions live in the LEFT pane (ProjectPanel's Sessions
 * tab → LiveSessionList → SessionViewer); the right-pane Sessions anchor
 * was removed because it was a duplicate showing mock data.
 *
 * Body resolution:
 *   1. If a task is pending or running globally, render the task UI
 *      (RunTaskPanel form / ChatPanel) — single-task-at-a-time legacy
 *      lifecycle. Stage 2 will move this state per-tab.
 *   2. Otherwise dispatch by active tab kind.
 *
 * Per gui-mainpane-restructure (todo/ms06, 2026-05-08).
 */

import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { ActionIcon, Box, Group, Menu, Text, Tooltip } from '@mantine/core'
import {
  IconBug,
  IconBuildingStore,
  IconChevronDown,
  IconHelp,
  IconHome,
  IconInbox,
  IconBox,
  IconList,
  IconPencil,
  IconRocket,
  IconLayoutDashboard,
  IconTerminal2,
  IconVectorTriangle,
  IconWand,
  IconX,
} from '@tabler/icons-react'
import { activateOnKey } from '../../guide/anchors'
import { useAppStore } from '../../store'
import { useTabsStore, type OpenTab, type ChatTabPayload, type DebugTabPayload, type MarketplaceTabPayload } from '../../store/tabsSlice'
import { useTerminalSessionsStore } from '../../store/terminalSessionsSlice'
import { HomeTab } from '../home/HomeTab'
import { TerminalsBody } from '../terminal/TerminalsBody'
import { SkillGalleryPanel } from '../task/SkillGalleryPanel'
import { DrawioTab } from '../home/DrawioTab'
import { RunTaskPanel } from '../agent/RunTaskPanel'
import { ChatPanel } from '../agent/ChatPanel'
import { ObjectTab } from '../workspace/ObjectTab'
import { closeObjectTab } from '../workspace/openObjectTab'
import { RailScreen } from './RailScreen'
import { MarketplaceRoot } from '../marketplace/MarketplaceRoot'
import { ReadyScreen } from '../ready/ReadyScreen'
import { ReadyBadge, useReadyAtStart } from '../ready/ReadyBadge'
import { DebugRoot } from '../dev/DebugRoot'
import { GuideBrowser, topicsForTabKind, type GuideTabPayload } from '../../guide'
import { useGuideLauncher } from '../../hooks/useGuideLauncher'
import { tFor, useT, type TFn } from '../../i18n'
import { DisplayPanel } from '../panels/DisplayPanel'
import { PanelsScreen } from '../panels/PanelsScreen'
import type { PanelTabPayload } from '../panels/openPanelTab'

function tabIcon(kind: OpenTab['kind']) {
  switch (kind) {
    case 'home':        return <IconHome           size={13} />
    case 'run-shell':   return <IconTerminal2      size={13} />
    case 'run-skills':  return <IconWand           size={13} />
    case 'run-drawio':  return <IconVectorTriangle size={13} />
    case 'run-task':    return <IconBug            size={13} />
    case 'object':              return <IconBox            size={13} />
    case 'inbox':               return <IconInbox          size={13} />
    case 'marketplace':         return <IconBuildingStore  size={13} />
    case 'debug':               return <IconBug            size={13} />
    case 'guide':               return <IconHelp           size={13} />
    case 'ready':               return <IconRocket         size={13} />
    case 'panel':               return <IconLayoutDashboard size={13} />
    case 'panels':              return <IconLayoutDashboard size={13} />
    default:           return null
  }
}

/** A tab's label in the GUI language: the Global anchor's is the shell's word;
 *  every other tab is named by what it shows (a task, an object, a session). */
function tabLabel(tab: OpenTab, t: TFn): string {
  if (tab.kind === 'ready') return tFor('ready')('tab')
  return tab.kind === 'home' ? t('tabs.global') : tab.label
}

export function MainPaneTabs() {
  const displayedUniverseName = useAppStore((s) => s.displayedUniverseName)
  const universeKey = displayedUniverseName ?? '__none__'

  const { openTabs, activeTabId } = useTabsStore((s) => s.getForUniverse(universeKey))
  const setActive = useTabsStore((s) => s.setActive)
  const closeTab  = useTabsStore((s) => s.closeTab)
  // The Object tab's close goes back to where the person was (the previous
  // object, or the panel it was opened from) before it closes for good.
  const closeAnyTab = (tab: OpenTab) =>
    tab.kind === 'object' ? closeObjectTab(universeKey, tab.id) : closeTab(universeKey, tab.id)
  const { launchGuide } = useGuideLauncher()
  const t = useT('shell')
  // The Ready to start screen: checked whenever the universe changes, opened
  // by itself at a universe's first start.
  useReadyAtStart()

  // pendingTask: the pre-launch RunTaskPanel form takeover (still
  // global until we move form-tabs per-launch). runningTaskId is NOT
  // read here anymore — chat lives in its own 'chat' tab now.
  const pendingTask   = useAppStore((s) => s.pendingTask)

  // Terminal-tab guard: while any terminal session is open, the Term tab
  // can't be closed (would orphan the running shells/log tails).
  const terminalCount = useTerminalSessionsStore((s) => s.sessions.size)

  const active = openTabs.find((t) => t.id === activeTabId) ?? openTabs[0]

  // Overflow detection — the tab strip can hold more tabs than fit. We
  // measure the inner scrollable list vs its visible width and, when
  // overflowed, surface a ▾ menu listing every tab so users can reach
  // tabs that scrolled off-screen.
  const stripRef = useRef<HTMLDivElement>(null)
  const [hasOverflow, setHasOverflow] = useState(false)
  useLayoutEffect(() => {
    const el = stripRef.current
    if (!el) return
    const measure = () => setHasOverflow(el.scrollWidth > el.clientWidth + 1)
    measure()
    // Re-measure when the container resizes (window resize, split-pane
    // drag, parent-layout change).
    if (typeof ResizeObserver === 'undefined') return
    const ro = new ResizeObserver(measure)
    ro.observe(el)
    return () => ro.disconnect()
  }, [openTabs.length])

  // Whenever the active tab changes, scroll it into view so the user
  // always sees the active tab even after overflowing.
  useEffect(() => {
    const el = stripRef.current
    if (!el || !active?.id) return
    const target = el.querySelector(`[data-tab-id="${active.id}"]`) as HTMLElement | null
    target?.scrollIntoView({ block: 'nearest', inline: 'nearest', behavior: 'smooth' })
  }, [active?.id])

  return (
    <Box style={{ height: '100%', display: 'flex', flexDirection: 'column', minHeight: 0 }}>
      {/* Tab strip — scrollable list + optional overflow ▾ menu */}
      <Group
        gap={0}
        wrap="nowrap"
        align="stretch"
        style={{
          flexShrink: 0,
          borderBottom: '1px solid var(--mantine-color-default-border)',
        }}
      >
        <Group
          ref={stripRef}
          role="tablist"
          gap={0}
          wrap="nowrap"
          style={{
            flex: 1,
            minWidth: 0,
            overflowX: 'auto',
            overflowY: 'hidden',
            // Hide the horizontal scrollbar but keep the wheel/drag
            // behavior; the ▾ menu is the primary navigation aid.
            scrollbarWidth: 'none',
          }}
        >
          {openTabs.map((tab) => {
            const isActive = tab.id === active?.id
            const showClose =
              tab.closeable && !(tab.kind === 'run-shell' && terminalCount > 0)
            return (
              <Group
                key={tab.id}
                data-tab-id={tab.id}
                data-guide={tab.kind === 'home' ? 'tabs.global' : undefined}
                gap={4}
                wrap="nowrap"
                px="sm"
                py={6}
                onClick={() => setActive(universeKey, tab.id)}
                role="tab"
                aria-selected={isActive}
                aria-label={tabLabel(tab, t)}
                tabIndex={0}
                onKeyDown={activateOnKey(() => setActive(universeKey, tab.id))}
                style={{
                  cursor: 'pointer',
                  borderRight: '1px solid var(--mantine-color-default-border)',
                  background: isActive ? 'var(--mantine-color-default)' : 'transparent',
                  color: isActive
                    ? 'var(--mantine-color-default-color)'
                    : 'var(--mantine-color-dimmed)',
                  fontSize: 12,
                  fontWeight: isActive ? 600 : 400,
                  whiteSpace: 'nowrap',
                  userSelect: 'none',
                  flexShrink: 0,
                }}
              >
                {tabIcon(tab.kind)}
                <span>{tabLabel(tab, t)}</span>
                {showClose && (
                  <Tooltip label={t('tabs.close')} withArrow>
                    <ActionIcon
                      variant="subtle"
                      color="gray"
                      size={16}
                      onClick={(e) => {
                        e.stopPropagation()
                        closeAnyTab(tab)
                      }}
                      aria-label={t('tabs.closeAria', { label: tabLabel(tab, t) })}
                    >
                      <IconX size={12} />
                    </ActionIcon>
                  </Tooltip>
                )}
              </Group>
            )
          })}
        </Group>

        {hasOverflow && (
          <Menu shadow="md" position="bottom-end" width={240} withinPortal>
            <Menu.Target>
              <Tooltip label={t('tabs.all')} withArrow position="bottom">
                <ActionIcon
                  variant="subtle"
                  size="md"
                  aria-label={t('tabs.all')}
                  style={{
                    flexShrink: 0,
                    borderLeft: '1px solid var(--mantine-color-default-border)',
                    borderRadius: 0,
                  }}
                >
                  <IconChevronDown size={14} />
                </ActionIcon>
              </Tooltip>
            </Menu.Target>
            <Menu.Dropdown>
              <Menu.Label>{t('tabs.open')}</Menu.Label>
              {openTabs.map((tab) => (
                <Menu.Item
                  key={tab.id}
                  leftSection={tabIcon(tab.kind)}
                  onClick={() => setActive(universeKey, tab.id)}
                  rightSection={
                    tab.closeable && !(tab.kind === 'run-shell' && terminalCount > 0) ? (
                      <ActionIcon
                        variant="subtle"
                        size="xs"
                        onClick={(e) => {
                          e.stopPropagation()
                          closeAnyTab(tab)
                        }}
                        aria-label={t('tabs.closeAria', { label: tabLabel(tab, t) })}
                      >
                        <IconX size={12} />
                      </ActionIcon>
                    ) : null
                  }
                  style={{
                    fontWeight: tab.id === active?.id ? 600 : 400,
                  }}
                >
                  <Text size="xs">{tabLabel(tab, t)}</Text>
                </Menu.Item>
              ))}
            </Menu.Dropdown>
          </Menu>
        )}

        <ReadyBadge />

        <Tooltip label={t('tabs.guide')} withArrow position="bottom">
          <ActionIcon
            variant="subtle"
            size="md"
            aria-label={t('tabs.guide')}
            onClick={() => launchGuide(topicsForTabKind(active?.kind))}
            style={{
              flexShrink: 0,
              borderLeft: '1px solid var(--mantine-color-default-border)',
              borderRadius: 0,
            }}
          >
            <IconHelp size={14} />
          </ActionIcon>
        </Tooltip>
      </Group>

      {/* Active tab body */}
      <Box style={{ flex: 1, minHeight: 0, overflow: 'hidden' }}>
        <TabBody
          active={active}
          pendingTask={pendingTask}
          displayedUniverseName={displayedUniverseName}
        />
      </Box>
    </Box>
  )
}

interface TabBodyProps {
  active: OpenTab | undefined
  pendingTask: { name: string; description: string } | null
  displayedUniverseName: string | null
}

function TabBody({ active, pendingTask, displayedUniverseName }: TabBodyProps) {
  // cwm-v3 US-3 Phase 1 (2026-06-07) — the global
  // `runningTaskId !== null → ChatPanel` takeover was REMOVED. Chat
  // now lives in its own `chat` tab (see TabKind in tabsSlice). The
  // tab strip stays visible during chat, so the user can navigate to
  // Global / Node / Debug and come back without ending the session.
  //
  // RunTaskPanel (the pre-launch form for tasks with input-schema)
  // STILL takes over the pane while the user is filling it out — the
  // form is short-lived and the takeover keeps the surface
  // single-purpose during that moment. Once the user clicks Run,
  // session.created fires, the chat tab opens (see
  // useAgentEvents.session.created handler), and the right pane
  // returns to normal tab navigation.
  if (pendingTask !== null) return <RunTaskPanel />
  return <ActiveTabBody active={active} displayedUniverseName={displayedUniverseName} />
}

/** The Debug tab keeps its section + selected demo page in its payload, so
 *  switching away and back restores the view. */
function DebugTabBody({ tab, universeName }: { tab: OpenTab; universeName: string | null }) {
  const updateTabPayload = useTabsStore((s) => s.updateTabPayload)
  const payload = (tab.payload as DebugTabPayload | undefined) ?? {}
  return (
    <DebugRoot
      payload={payload}
      onChange={(next) => {
        if (universeName) updateTabPayload(universeName, tab.id, next)
      }}
    />
  )
}

/** The Marketplace tab keeps its section in its payload. */
function MarketplaceTabBody({ tab, universeName }: { tab: OpenTab; universeName: string | null }) {
  const updateTabPayload = useTabsStore((s) => s.updateTabPayload)
  const payload = (tab.payload as MarketplaceTabPayload | undefined) ?? {}
  return (
    <MarketplaceRoot
      section={payload.section ?? 'sidecars'}
      onSection={(section) => {
        if (universeName) updateTabPayload(universeName, tab.id, { section })
      }}
    />
  )
}

/** The Guide tab keeps the topics of the `?` that opened it in its payload. */
function GuideTabBody({ tab, universeName }: { tab: OpenTab; universeName: string | null }) {
  const updateTabPayload = useTabsStore((s) => s.updateTabPayload)
  const payload = (tab.payload as GuideTabPayload | undefined) ?? {}
  return (
    <GuideBrowser
      topics={payload.topics ?? []}
      onTopicsChange={(topics) => {
        if (universeName) updateTabPayload(universeName, tab.id, { ...payload, topics })
      }}
    />
  )
}

function ActiveTabBody({
  active,
  displayedUniverseName,
}: {
  active: OpenTab | undefined
  displayedUniverseName: string | null
}) {
  const t = useT('shell')
  switch (active?.kind) {
    case 'home':       return <HomeTab />
    case 'chat': {
      // NO remount-by-key: ChatPanel + its children read THIS session's
      // bucket explicitly via the sessionId prop (per-run display), so
      // switching tabs just changes the prop — the panel's own UI state
      // (debug toggle, active sub-tab) persists.
      const chatPayload = active.payload as ChatTabPayload | undefined
      return <ChatPanel sessionId={chatPayload?.sessionId} scopedNodeName={chatPayload?.scopedNodeName} />
    }
    case 'inbox':      return <RailScreen />
    case 'marketplace': return <MarketplaceTabBody tab={active} universeName={displayedUniverseName} />
    case 'debug':      return <DebugTabBody tab={active} universeName={displayedUniverseName} />
    case 'guide':      return <GuideTabBody tab={active} universeName={displayedUniverseName} />
    case 'ready':      return displayedUniverseName ? <ReadyScreen universe={displayedUniverseName} section={(active.payload as { section?: string } | undefined)?.section} /> : null
    case 'panels':     return displayedUniverseName ? <PanelsScreen universeName={displayedUniverseName} /> : null
    case 'panel': {
      const p = active.payload as PanelTabPayload | undefined
      return displayedUniverseName && p?.panelId
        ? <Box p="md"><DisplayPanel universeName={displayedUniverseName} panelId={p.panelId} title={p.title} /></Box>
        : null
    }
    case 'run-shell':  return <TerminalsBody />
    case 'run-skills': return <SkillGalleryPanel />
    case 'run-drawio': return <DrawioTab />
    case 'run-task':
      // No pending/running task but a run-task tab is selected — likely a
      // stale tab that survived the run. Fall back to a placeholder.
      return (
        <Box p="md">
          <em>{t('mainPane.taskNotRunning')}</em>
        </Box>
      )
    case 'object':
      // The single Object tab: its payload swaps on the next selection, and
      // it reads the object model through the object pane (object-pane/Detail).
      if (!displayedUniverseName) {
        return (
          <Box p="md">
            <em>{t('mainPane.objectNeedsUniverse')}</em>
          </Box>
        )
      }
      return <ObjectTab tab={active} universeName={displayedUniverseName} />
    default:
      return null
  }
}

// SPDX-License-Identifier: Apache-2.0
/**
 * The way into the Ready to start screen:
 *
 *   - `useReadyLauncher` opens (or focuses) the single `ready` tab under the
 *     universe on screen;
 *   - `useReadyAtStart` checks the universe on screen whenever it changes, and
 *     opens the screen by itself at the first start of each installed version
 *     in that universe (the installed KaTyoti's and its solution's versions):
 *     the first start after an installer or an update opened the GUI. It is
 *     the first of the start screens (sections/shell/startScreens): it opens
 *     alone, and the others wait until it is closed;
 *   - `ReadyBadge` sits in the tab strip whenever the universe is not fully
 *     operational (a sidecar's check not green, a required program missing, a
 *     key missing, or a part that could not be checked), and opens the screen.
 */

import { useCallback, useEffect, useRef } from 'react'
import { Badge, Tooltip } from '@mantine/core'
import { IconAlertTriangle, IconHelpCircle } from '@tabler/icons-react'
import { useAppStore } from '../../store'
import { useTabsStore } from '../../store/tabsSlice'
import { useT } from '../../i18n'
import { badgeCount, badgeShown, opensAtStart, summarize } from './readiness'
import { installedVersion, markSeen, seenUniverses, useReadyInputs, useReadyStore } from './readyStore'
import { passStart, useStartScreens } from '../shell/startScreens'

export const READY_TAB_ID = 'ready'

export function useReadyLauncher() {
  const universe = useAppStore((s) => s.displayedUniverseName)
  const openTab = useTabsStore((s) => s.openTab)
  const launchReady = useCallback((name: string | null = universe) => {
    if (!name) return
    openTab(name, { id: READY_TAB_ID, kind: 'ready', label: READY_TAB_ID, closeable: true })
  }, [universe, openTab])
  return { launchReady }
}

export function useReadyAtStart() {
  const universe = useAppStore((s) => s.displayedUniverseName)
  const home = useAppStore((s) => s.universes.find((u) => u.name === s.displayedUniverseName)?.homePath ?? '')
  const settled = useStartScreens((s) => s.settled)
  const check = useReadyStore((s) => s.check)
  const { launchReady } = useReadyLauncher()
  // The universes whose screen this start opened by itself: false once
  // launched, true once seen open - its closing lets the next screen come.
  const opened = useRef(new Map<string, boolean>())
  const readyOpen = useTabsStore((s) => !!universe && !!s.byUniverse.get(universe)?.openTabs.some((t) => t.id === READY_TAB_ID))
  useEffect(() => {
    if (universe && home) void check(universe)
  }, [universe, home, check])
  // The universes this start already decided for (the version is read first).
  const decided = useRef(new Set<string>())
  useEffect(() => {
    if (!universe || !home || !settled || decided.current.has(universe)) return
    decided.current.add(universe)
    void installedVersion(universe).then((version) => {
      if (opensAtStart(universe, seenUniverses(), version)) {
        markSeen(universe, version)
        opened.current.set(universe, false)
        launchReady(universe)
      } else {
        // Nothing to show at this start: the next start screen may come.
        passStart(universe, 'ready')
      }
    })
  }, [universe, home, settled, launchReady])
  // Opened by itself, then closed: the next start screen may come.
  useEffect(() => {
    if (!universe || !opened.current.has(universe)) return
    if (readyOpen) opened.current.set(universe, true)
    else if (opened.current.get(universe)) passStart(universe, 'ready')
  }, [universe, readyOpen])
}

export function ReadyBadge() {
  const t = useT('ready')
  const universe = useAppStore((s) => s.displayedUniverseName)
  const inputs = useReadyInputs(universe)
  const { launchReady } = useReadyLauncher()
  const summary = summarize(inputs)
  if (!universe || !badgeShown(summary)) return null
  const unknown = summary.state === 'unknown'
  return (
    <Tooltip label={t('badge.tooltip', { universe })} withArrow position="bottom">
      <Badge
        component="button"
        type="button"
        onClick={() => launchReady()}
        color={unknown ? 'gray' : 'orange'}
        variant="light"
        size="sm"
        radius="sm"
        leftSection={unknown ? <IconHelpCircle size={12} /> : <IconAlertTriangle size={12} />}
        data-guide="ready.badge"
        style={{ alignSelf: 'center', margin: '0 6px', cursor: 'pointer', flexShrink: 0, border: 'none' }}
      >
        {unknown ? t('badge.unknown') : t('badge.label', { count: badgeCount(summary) })}
      </Badge>
    </Tooltip>
  )
}

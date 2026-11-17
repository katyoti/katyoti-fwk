// SPDX-License-Identifier: Apache-2.0
/**
 * useInboxLauncher — single source of truth for "open the inbox tab".
 *
 * The inbox tab is « À faire »: the page the backend names for
 * the rail's Inbox (RailScreen), everything waiting on the person.
 *
 * Three callers:
 *   1. The inbox icon in IconRibbon.tsx
 *   2. Clicking the Inbox node in the workspace tree
 *   3. A page's `screen: todo` action (PageHost: the planification deck's
 *      « Ouvrir À faire ») - through openInboxTab
 *
 * Both converge on a single 'inbox' tab — openTab is idempotent on id
 * collision, so a second open focuses the existing tab rather than
 * duplicating it. The gating (no active universe = no-op) lives here.
 */

import { useCallback } from 'react'
import { useAppStore } from '../store'
import { useTabsStore } from '../store/tabsSlice'

const INBOX_TAB_ID = 'inbox'

/** openInboxTab opens « À faire » in universeName's tabs (focused when open). */
export function openInboxTab(universeName: string): void {
  if (!universeName) return
  useTabsStore.getState().openTab(universeName, {
    id:        INBOX_TAB_ID,
    kind:      'inbox',
    label:     '« À faire »',
    closeable: true,
    payload:   {},
  })
}

export function useInboxLauncher() {
  const displayedUniverseName = useAppStore((s) => s.displayedUniverseName)

  const launchInbox = useCallback(() => {
    if (!displayedUniverseName) return
    openInboxTab(displayedUniverseName)
  }, [displayedUniverseName])

  return {
    launchInbox,
    canLaunch: !!displayedUniverseName,
  }
}

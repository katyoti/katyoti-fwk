// SPDX-License-Identifier: Apache-2.0
/**
 * useGuideLauncher — single source of truth for "open the guide".
 *
 * Inside a universe (main-pane route `universe`) the guide is ONE `guide` tab
 * (openTab is idempotent on id, so a second open focuses it; its topics are
 * retargeted). Everywhere else — the universe picker has no tabs — it opens
 * as a drawer with the same content.
 *
 * Callers: "Help & Feedback" and « KaTyoti concepts » (the concepts topic) in
 * the profile menu, the `?` on the tab strip, the `?` on the project panel.
 */

import { useCallback } from 'react'
import { useAppStore } from '../store'
import { useTabsStore } from '../store/tabsSlice'
import { GUIDE_TAB_ID, useGuideDrawer, type GuideTabPayload } from '../guide/surface'
import { guideT } from '../guide/i18n'

export function useGuideLauncher() {
  const openTab          = useTabsStore((s) => s.openTab)
  const updateTabPayload = useTabsStore((s) => s.updateTabPayload)
  const openDrawer       = useGuideDrawer((s) => s.openDrawer)

  const launchGuide = useCallback((topics?: string[]) => {
    const { displayedUniverseName, mainPaneRoute } = useAppStore.getState()
    if (displayedUniverseName && mainPaneRoute.kind === 'universe') {
      const payload: GuideTabPayload = { topics: topics ?? [] }
      openTab(displayedUniverseName, {
        id:        GUIDE_TAB_ID,
        kind:      'guide',
        label:     guideT()('drawer.title'),
        closeable: true,
        payload,
      })
      // Already open: openTab only focuses it — retarget its topics.
      updateTabPayload(displayedUniverseName, GUIDE_TAB_ID, payload)
      return
    }
    openDrawer(topics)
  }, [openTab, updateTabPayload, openDrawer])

  return { launchGuide }
}

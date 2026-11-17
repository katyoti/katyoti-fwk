// SPDX-License-Identifier: Apache-2.0
/**
 * useMarketplaceLauncher — single source of truth for "open the Marketplace tab".
 *
 * The Marketplace tab hosts MarketplaceRoot: the five sections (Infra, Sidecars,
 * Config-sets, Seeds, Solutions) under the shared subscriptions. `launchMarketplace(section)`
 * opens it on that section (default: the one it shows, else Sidecars).
 *
 * The marketplace engine itself is global (installs land in the shared store),
 * but the tab — like every other — lives under the active universe, and the
 * universe actions target that universe. openTab is idempotent on id, so a
 * re-launch focuses the existing tab; a section asked for is then set on it.
 */

import { useCallback } from 'react'
import { useAppStore } from '../store'
import { useTabsStore, type MarketplaceSection } from '../store/tabsSlice'

const MARKETPLACE_TAB_ID = 'marketplace'

export function useMarketplaceLauncher() {
  const displayedUniverseName = useAppStore((s) => s.displayedUniverseName)
  const openTab               = useTabsStore((s) => s.openTab)
  const updateTabPayload      = useTabsStore((s) => s.updateTabPayload)

  const launchMarketplace = useCallback((section?: MarketplaceSection) => {
    if (!displayedUniverseName) return
    openTab(displayedUniverseName, {
      id:        MARKETPLACE_TAB_ID,
      kind:      'marketplace',
      label:     'Marketplace',
      closeable: true,
      payload:   { section: section ?? 'sidecars' },
    })
    if (section) updateTabPayload(displayedUniverseName, MARKETPLACE_TAB_ID, { section })
  }, [displayedUniverseName, openTab, updateTabPayload])

  return {
    launchMarketplace,
    canLaunch: !!displayedUniverseName,
  }
}

// SPDX-License-Identifier: Apache-2.0
/**
 * useDebugLauncher — single source of truth for "open the Debug tab".
 *
 * The Debug tab hosts DebugRoot: UI component demos, the KTSP browser, the
 * resource / type / task / skill browsers, the bus trace and the workspace
 * state. Like every other tab it lives under the active universe (the KTSP,
 * resource and workspace panels read that universe). openTab is idempotent on
 * id, so re-launch focuses the existing tab.
 */

import { useCallback } from 'react'
import { useAppStore } from '../store'
import { useTabsStore } from '../store/tabsSlice'

const DEBUG_TAB_ID = 'debug'

export function useDebugLauncher() {
  const displayedUniverseName = useAppStore((s) => s.displayedUniverseName)
  const openTab               = useTabsStore((s) => s.openTab)

  const launchDebug = useCallback(() => {
    if (!displayedUniverseName) return
    openTab(displayedUniverseName, {
      id:        DEBUG_TAB_ID,
      kind:      'debug',
      label:     'Debug',
      closeable: true,
      payload:   {},
    })
  }, [displayedUniverseName, openTab])

  return {
    launchDebug,
    canLaunch: !!displayedUniverseName,
  }
}

// SPDX-License-Identifier: Apache-2.0
// useDisplayStatus — lightweight global health hook for the ktw-display
// sidecar reachability. Probes `App.GetWidgetURL(universe, 'hello', ...)`
// once per mount + on universe change. Surfaces a coarse status flag for
// debug panels / status badges.
//
// Usage:
//
//   const { status, lastError } = useDisplayStatus()
//
// The probe is best-effort — if a real <IframeWidget> already mounted in
// this universe, that one's status is more accurate. This hook is for the
// "is the bridge reachable at all?" question (e.g. an IconRibbon dot).

import { useEffect, useState } from 'react'
import { GetWidgetURL } from '../../../wailsjs/go/main/App'
import { useAppStore } from '../../store'

export type DisplayStatus =
  | 'unknown'   // not probed yet
  | 'ok'        // bridge reachable + returned a URL
  | 'down'      // bridge unreachable (Wails error / connection failure)

export interface UseDisplayStatusResult {
  status: DisplayStatus
  lastError?: string
}

export function useDisplayStatus(): UseDisplayStatusResult {
  const universe = useAppStore((s) => s.displayedUniverseName)
  const [status, setStatus] = useState<DisplayStatus>('unknown')
  const [lastError, setLastError] = useState<string | undefined>()

  useEffect(() => {
    let cancelled = false
    if (!universe) {
      setStatus('unknown')
      setLastError('no displayed universe')
      return
    }
    ;(async () => {
      try {
        const url = await GetWidgetURL(universe, 'hello', '', '', '')
        if (cancelled) return
        if (url) {
          setStatus('ok')
          setLastError(undefined)
        } else {
          setStatus('down')
          setLastError('bridge returned empty URL')
        }
      } catch (e) {
        if (cancelled) return
        setStatus('down')
        setLastError(e instanceof Error ? e.message : String(e))
      }
    })()
    return () => {
      cancelled = true
    }
  }, [universe])

  return { status, lastError }
}

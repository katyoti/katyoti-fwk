// SPDX-License-Identifier: Apache-2.0
/**
 * FirstGuide — a universe's first guide: the guide the universe
 * names (Univers.yaml `first_guide`) plays at the universe's
 * first open, and on closing it the guide tab opens - on the essentials at the
 * Essentiel level. Seen once per universe, kept with the
 * person's completions (`<guide>@first:<universe>`). It is the second of the
 * start screens (sections/shell/startScreens): it waits until « Prêt à
 * démarrer » is closed, and « Quoi de neuf » waits until it is - or until it
 * has nothing to show. It also waits while another guide plays or is read.
 */

import { useEffect, useRef } from 'react'
import { ADMIN_UNIVERSE } from '../ktsp'
import { useAppStore } from '../store'
import { useGuideLauncher } from '../hooks/useGuideLauncher'
import { openGuide, useGuidePlayer } from './player'
import { fetchRecipe, firstGuideDue, firstGuideKey } from './recipes'
import { useWelcomeOff } from './welcome'
import { passStart, useStartTurn } from '../sections/shell/startScreens'

export function FirstGuide() {
  const displayed = useAppStore((s) => s.displayedUniverseName)
  const universe = useAppStore((s) => s.universes.find((u) => u.name === displayed))
  const loaded = useGuidePlayer((s) => s.completionsLoaded)
  const completed = useGuidePlayer((s) => s.completed)
  const busy = useGuidePlayer((s) => s.status !== 'idle' || !!s.reading || !!s.onceCurrent)
  const { launchGuide } = useGuideLauncher()
  // No first guide by itself when the welcome screens are off.
  const off = useWelcomeOff(displayed)
  // Its turn among the start screens: once « Prêt à démarrer » is closed.
  const turn = useStartTurn(displayed, 'first-guide')
  // The first guide this window started, until it closes.
  const running = useRef<{ uri: string; universe: string; opened: boolean } | null>(null)

  useEffect(() => {
    if (!loaded || off === undefined || !turn || !displayed || displayed === ADMIN_UNIVERSE) return
    // Off, or none due: nothing to show - the next start screen may come.
    if (off) { passStart(displayed, 'first-guide'); return }
    if (!loaded || off !== false || busy || running.current || !turn) return
    const uri = firstGuideDue(universe, completed)
    if (!uri) { passStart(displayed, 'first-guide'); return }
    running.current = { uri, universe: displayed, opened: false }
    void fetchRecipe(uri, displayed)
      .then((entry) => openGuide(entry))
      .catch(() => {
        // A first guide that cannot be read is not asked again at every start.
        void useGuidePlayer.getState().complete(firstGuideKey(uri, displayed), 'skipped')
        running.current = null
        passStart(displayed, 'first-guide')
      })
  }, [loaded, off, busy, turn, displayed, universe, completed])

  // Opened, then closed: seen for this universe, and the guide tab opens.
  useEffect(() => {
    const r = running.current
    if (!r) return
    if (busy) { r.opened = true; return }
    if (!r.opened) return
    running.current = null
    void useGuidePlayer.getState().complete(firstGuideKey(r.uri, r.universe), 'seen')
    launchGuide()
    passStart(r.universe, 'first-guide')
  }, [busy, launchGuide])

  return null
}

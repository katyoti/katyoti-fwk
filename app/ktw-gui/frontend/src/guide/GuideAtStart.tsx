// SPDX-License-Identifier: Apache-2.0
/**
 * GuideAtStart — opens the guides marked `shown_once` (a Welcome, a What's
 * new) by themselves at the person's start, one after the other in their
 * declared order, each never again once seen. It reads the same
 * catalogs as the guide browser and the person's completions; it runs once per
 * universe shown in this start of the GUI - the admin one at the picker, then
 * each universe opened - so a universe's own What's new (its installed letter)
 * is asked once it is on screen, whatever loads first. What was
 * seen stays seen: the completions decide. They are the last of the start
 * screens (sections/shell/startScreens): in a universe they wait until
 * « Prêt à démarrer » and the universe's first guide are closed.
 */

import { useEffect } from 'react'
import { ADMIN_UNIVERSE } from '../ktsp'
import { useAppStore } from '../store'
import { useGuidePlayer } from './player'
import { fetchSourced, fillSourced, loadRecipes, startQueue } from './recipes'
import { useWelcomeOff } from './welcome'
import { useStartTurn } from '../sections/shell/startScreens'

/** The universes this start of the GUI already ran the shown-once guides for. */
const ranFor = new Set<string>()

export function GuideAtStart() {
  const displayed = useAppStore((s) => s.displayedUniverseName)
  const loaded = useGuidePlayer((s) => s.completionsLoaded)
  const refreshCompletions = useGuidePlayer((s) => s.refreshCompletions)
  // Started without welcome screens, or in a universe that turned them off:
  // no shown-once guide opens by itself.
  const off = useWelcomeOff(displayed)
  // Their turn among the start screens: the earlier ones have passed.
  const turn = useStartTurn(displayed, 'shown-once')

  useEffect(() => { void refreshCompletions() }, [refreshCompletions])

  useEffect(() => {
    const here = displayed || ADMIN_UNIVERSE
    if (!loaded || off !== false || !turn || ranFor.has(here)) return
    ranFor.add(here)
    const universes = displayed && displayed !== ADMIN_UNIVERSE ? [ADMIN_UNIVERSE, displayed] : [ADMIN_UNIVERSE]
    // A guide whose text is data (What's new) is asked what is
    // new in the universe on screen; nothing new, it does not open.
    const source = fetchSourced(displayed && displayed !== ADMIN_UNIVERSE ? displayed : ADMIN_UNIVERSE)
    void loadRecipes(universes).then(async ({ entries }) => {
      const s = useGuidePlayer.getState()
      s.runOnce(startQueue(await fillSourced(entries, s.completed, source), s.completed))
    })
  }, [loaded, displayed, off, turn])

  return null
}

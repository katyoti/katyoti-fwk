// SPDX-License-Identifier: Apache-2.0
/**
 * startScreens — the ONE owner of the screens that open by themselves when
 * KaTyoti starts: they come one at a time, in one decided
 * order, never together.
 *
 *   1. `ready` — « Prêt à démarrer », what the installation still needs:
 * alone, until the person closes it;
 *   2. `first-guide` — the universe's first guide;
 *   3. `shown-once` — the guides shown once at start, « Quoi de neuf ».
 *
 * Each opener asks `useStartTurn(universe, screen)` before opening and says
 * `passStart(universe, screen)` once its screen is closed, or when it has
 * nothing to show; a later screen waits until every earlier one has passed.
 * Nothing opens before the start has settled on the universe it opens
 * (`settleStart`, once the launch target is activated or the picker shown):
 * a screen of the picker never jumps ahead of the installed universe's.
 * At the picker (no universe, or the admin one) only the shown-once guides
 * apply: the earlier screens are passed there by definition.
 */

import { create } from 'zustand'
import { ADMIN_UNIVERSE } from '../../ktsp'

/** The order the start screens come in. */
export const START_ORDER = ['ready', 'first-guide', 'shown-once'] as const
export type StartScreen = (typeof START_ORDER)[number]

/** The screens that apply only inside a universe, never at the picker. */
const UNIVERSE_ONLY: ReadonlySet<StartScreen> = new Set<StartScreen>(['ready', 'first-guide'])

/** The key a universe's start is kept under: the picker is the admin one. */
export const startKey = (universe: string | null | undefined): string => universe || ADMIN_UNIVERSE

/** startTurn decides whether `screen` may open now in a universe: the start
 *  has settled and every screen before it has passed (closed, or nothing to
 *  show). Pure: the decision the openers and the tests share. */
export function startTurn(settled: boolean, key: string, passed: ReadonlySet<StartScreen>, screen: StartScreen): boolean {
  if (!settled) return false
  for (const s of START_ORDER) {
    if (s === screen) return true
    const byDefinition = key === ADMIN_UNIVERSE && UNIVERSE_ONLY.has(s)
    if (!byDefinition && !passed.has(s)) return false
  }
  return false
}

interface StartScreensState {
  /** The start has settled on the universe it opens (or on the picker). */
  settled: boolean
  /** Per universe key, the screens that have passed in this start. */
  passed: Record<string, StartScreen[]>
}

export const useStartScreens = create<StartScreensState>(() => ({ settled: false, passed: {} }))

/** The start has settled: the launch target is open, or the picker shown. */
export function settleStart() {
  if (!useStartScreens.getState().settled) useStartScreens.setState({ settled: true })
}

/** A screen has passed in a universe: closed, or nothing to show. */
export function passStart(universe: string | null | undefined, screen: StartScreen) {
  const key = startKey(universe)
  const { passed } = useStartScreens.getState()
  if (passed[key]?.includes(screen)) return
  useStartScreens.setState({ passed: { ...passed, [key]: [...(passed[key] ?? []), screen] } })
}

/** Whether `screen` may open now in the universe on screen. */
export function useStartTurn(universe: string | null | undefined, screen: StartScreen): boolean {
  const key = startKey(universe)
  const settled = useStartScreens((s) => s.settled)
  const passed = useStartScreens((s) => s.passed[key])
  return startTurn(settled, key, new Set(passed ?? []), screen)
}

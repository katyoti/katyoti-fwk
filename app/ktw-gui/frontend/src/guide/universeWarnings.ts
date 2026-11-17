// SPDX-License-Identifier: Apache-2.0
/**
 * Warnings shown on a universe's card: the guide's read-only universe checks
 * (modules/guide/tools.md, « Checks ») that warn about the universe itself
 * rather than prove a recipe's step. A failed check is a warning, said by its
 * `detailMsg` in the `guide` namespace; a check that cannot be reached (no
 * hub, an older ktw-univers) shows nothing — it is not a warning.
 */

import { useEffect, useState } from 'react'
import { runStepAction, type CheckResult } from './recipes'

/** The checks run for every user universe's card, as action URIs. */
export const UNIVERSE_CARD_CHECKS = ['ktw-univers:universe-checks/repositories-have-remote']

type Run = (actionURI: string, universe: string) => Promise<unknown>

const dispatch: Run = (uri, universe) => runStepAction(uri, universe)

/** The failed checks of the universe, in the order of `checks`. */
export async function universeWarnings(universe: string, checks: string[] = UNIVERSE_CARD_CHECKS, run: Run = dispatch): Promise<CheckResult[]> {
  const results = await Promise.all(checks.map(async (uri) => {
    try {
      const out = (await run(uri, universe)) as Partial<CheckResult> | null
      if (!out || out.ok !== false) return null
      return { ok: false, detail: out.detail ?? '', detailMsg: out.detailMsg } as CheckResult
    } catch {
      return null
    }
  }))
  return results.filter((r): r is CheckResult => r !== null)
}

/** The warnings of a universe's card, read once per universe; none for a
 *  universe that is skipped (the admin universe holds no work). */
export function useUniverseWarnings(universe: string, skip = false): CheckResult[] {
  const [warnings, setWarnings] = useState<CheckResult[]>([])
  useEffect(() => {
    if (skip) { setWarnings([]); return }
    let live = true
    void universeWarnings(universe).then((w) => { if (live) setWarnings(w) })
    return () => { live = false }
  }, [universe, skip])
  return warnings
}

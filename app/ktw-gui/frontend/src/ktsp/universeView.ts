// SPDX-License-Identifier: Apache-2.0
/**
 * The universe's resolved view: what its GUI shows at the
 * person's display level - the element ids its tags allow (rail, Home, quick
 * actions, « + New », object tabs) - resolved by the hub from the universe's
 * tags (ktw-agents' `universe-view` slot). A universe with no
 * tag, or none served, shows everything.
 */

import { dispatcherFor } from './index'
import type { DisplayLevel } from '../types/settings'

export interface UniverseView {
  level: string
  /** No tag: everything is shown, and nothing is listed. */
  all: boolean
  /** The element ids shown at the level. */
  allowed: string[]
  first: string[]
}

/** The view's word for a level: Essentiel is `essentiel` there. */
export function viewLevel(level: DisplayLevel): string {
  return level === 'essential' ? 'essentiel' : level
}

/** The universe's view at a level; null when no member serves it. */
export async function resolveView(universe: string, level: DisplayLevel): Promise<UniverseView | null> {
  try {
    const out = (await dispatcherFor(universe)('ktw-agents:universe-view/resolve-view', { level: viewLevel(level) })) as UniverseView | undefined
    return out ?? null
  } catch {
    return null
  }
}

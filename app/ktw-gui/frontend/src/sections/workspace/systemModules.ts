// SPDX-License-Identifier: Apache-2.0
/**
 * System modules (sys-inbox, sys-journal, sys-planification…) are the
 * platform's own operational spaces: a GUI shows them, and what lies in them,
 * at the Expert display level only. Whether an object is one is
 * data - its `system` flag, which CWM sets from the system-module capability -
 * never a test on its name.
 */

import type { DisplayLevel } from '../../types/settings'

/** visibleObjects leaves the system modules out below Expert. In a
 *  depth-first list, a row left out takes the deeper rows after it along. */
export function visibleObjects<T extends { system?: boolean; depth?: number }>(rows: T[], level: DisplayLevel): T[] {
  if (level === 'expert') return rows
  const out: T[] = []
  let skipBelow = -1
  for (const r of rows) {
    const depth = r.depth ?? 0
    if (skipBelow >= 0 && depth > skipBelow) continue
    skipBelow = -1
    if (r.system) {
      skipBelow = depth
      continue
    }
    out.push(r)
  }
  return out
}

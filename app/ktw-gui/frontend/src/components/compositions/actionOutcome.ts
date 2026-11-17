// SPDX-License-Identifier: Apache-2.0
/**
 * The alert an action leaves above a panel's page, and when it goes.
 *
 * The action's own re-read (PageHost calls onRefresh when it is through) shows
 * its effect, so the alert stays across it; the NEXT re-read of the panel's
 * data — a write elsewhere, a later refresh — is news the alert no longer
 * describes, and it goes. Kept free of React so it is tested on its own.
 */
import type { UIPageAction } from '@ktw/display-lib/ui-builders/page/index.ts'

export interface ActionOutcome {
  ok: boolean
  text: string
  /** Re-reads still owed to the action itself before the alert is stale. */
  ownReads: number
  /** The action taking the change back, offered as « Undo ». */
  undo?: UIPageAction
}

/** The outcome of an action, owing `ownReads` re-reads (1 when the action
 *  re-reads the panel itself, 0 when nothing does); `undo` when its result
 *  carried its inverse. */
export function actionOutcome(ok: boolean, text: string, ownReads: number, undo?: UIPageAction): ActionOutcome {
  return undo ? { ok, text, ownReads, undo } : { ok, text, ownReads }
}

/** The outcome after the panel's data was read again. */
export function outcomeAfterRead(o: ActionOutcome | null): ActionOutcome | null {
  if (!o) return null
  return o.ownReads > 0 ? { ...o, ownReads: o.ownReads - 1 } : null
}

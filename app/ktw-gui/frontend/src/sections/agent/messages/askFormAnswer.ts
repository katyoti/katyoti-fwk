// SPDX-License-Identifier: Apache-2.0
// The chat form card's answer as a display panel input: an
// ask-user-data form is the question of a display panel, its artifact id the
// panel's id, so the card answers that panel's input like any screen
// following it. Pure.

import { isPanelId } from '@ktw/display-lib/ui-builders/page/panel.ts'

import type { PanelInput } from '../../../lib/displayPanels'

/** askFormAnswer is the panel input of the card's submit (the form's value)
 *  or cancel; null when the form's artifact is no panel (an older run). */
export function askFormAnswer(artifactId: string | undefined, event: 'submit' | 'cancel', value?: unknown, by?: string): PanelInput | null {
  if (!isPanelId(artifactId)) return null
  const who = by ? { by } : {}
  return event === 'submit'
    ? { panel: artifactId, kind: 'submit', value, ...who }
    : { panel: artifactId, kind: 'cancel', ...who }
}

/** askFormRetry is the card's patch when its panel input is refused: the
 *  card leaves its answered state and shows the form again, holding what was
 *  submitted, so the person can answer again. */
export function askFormRetry(event: 'submit' | 'cancel', value: unknown, prior?: Record<string, unknown>): { submitted: undefined; cancelled: false; value?: Record<string, unknown> } {
  const kept = event === 'submit' && value && typeof value === 'object' ? (value as Record<string, unknown>) : prior
  return { submitted: undefined, cancelled: false, value: kept }
}

// SPDX-License-Identifier: Apache-2.0
// A page source's answer that is a display panel:
// render.Answer{panel: <id>} - no tree, the panel's open event carrying its
// skeleton and its events the ops that patch it. The GUI's object and global
// panels (SourcePanel) draw such an answer as DisplayPanel, in place.

/** The keys a panel answer may hold beside `panel` (render.Answer). */
const ANSWER_KEYS = new Set(['panel', 'parts', 'data'])

/** panelOfAnswer is the display panel a source's answer names: an object holding a non-empty `panel` id and, beside
 *  it, only what a render.Answer holds (no tree, no kind). Anything else -
 *  data that happens to carry a `panel` field, a tree, a Def - is no panel. */
export function panelOfAnswer(answer: unknown): string | undefined {
  if (!answer || typeof answer !== 'object' || Array.isArray(answer)) return undefined
  const a = answer as Record<string, unknown>
  if (typeof a.panel !== 'string' || a.panel === '') return undefined
  if (!Object.keys(a).every((k) => ANSWER_KEYS.has(k))) return undefined
  return a.panel
}

// SPDX-License-Identifier: Apache-2.0
import type { MsgItem } from '../../types/messages'

/** The reason a failed run gives: the text of the last system message of
 *  level error in its chat (a run that fails before it starts says it there,
 *  agent.modelKeyMissing, …). The failure banner says it, so the person
 *  reads why at every display level, never only in the debug tabs. */
export function runFailureReason(messages: readonly MsgItem[]): string | undefined {
  for (let i = messages.length - 1; i >= 0; i--) {
    const m = messages[i]
    if (m.kind === 'system' && m.level === 'error' && m.text.trim()) return m.text.trim()
  }
  return undefined
}

/** Whether a run that ended lands on its Result tab. A run that failed with a
 *  reason stays on its chat, where the reason is: its Result tab would only
 *  say « Aucune sortie structurée ». */
export function landsOnOutput(status: string, reason: string | undefined): boolean {
  return !(status === 'error' && !!reason)
}

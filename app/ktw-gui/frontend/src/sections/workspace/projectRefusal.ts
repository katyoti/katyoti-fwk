// SPDX-License-Identifier: Apache-2.0
/**
 * A project the universe refuses to serve.
 *
 * CWM refuses a project whose type needs a solution the universe lacks - a
 * Dossier d'architecture in a universe with no archifactory. Its refusal
 * reaches the GUI as the answer of every read of the project, its text led by
 * PROJECT_REFUSED (cwm's ErrProjectRefused) and followed by the reason, which
 * names the solution and what to do (`katyoti universe set-solution …`, or open
 * the project in a universe of that solution).
 *
 * Such an answer is not a core still starting: it is not asked again, and the
 * project area shows the reason with the way to the universe's page, where its
 * Solution section gives the universe one.
 */

/** The lead of CWM's refusal of a project: `project refused: <reason>`. */
export const PROJECT_REFUSED = 'project refused: '

function textOf(err: unknown): string {
  if (err == null) return ''
  if (typeof err === 'string') return err
  const e = err as { message?: string; error?: string }
  return e.message ?? e.error ?? String(err)
}

/** projectRefusal is the reason of a refused project, from a read's failure
 *  (an error or its text, possibly wrapped by the calls it went through);
 *  null when the failure is not a refusal of the project. */
export function projectRefusal(err: unknown): string | null {
  const text = textOf(err)
  const at = text.indexOf(PROJECT_REFUSED)
  if (at < 0) return null
  const reason = text.slice(at + PROJECT_REFUSED.length).trim()
  return reason || null
}

/** retryUnlessRefused: a failed read is asked again while the core starts,
 *  unless it is a refusal of the project - that answer will not change. */
export function retryUnlessRefused(err: unknown): boolean {
  return projectRefusal(err) === null
}

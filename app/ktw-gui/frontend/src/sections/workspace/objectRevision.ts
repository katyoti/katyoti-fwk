// SPDX-License-Identifier: Apache-2.0
/**
 * The version of each object this GUI last read or wrote. Every save sends it
 * back, so CWM refuses a write over a change made elsewhere since the pane
 * loaded (by hand, an agent, git) with « reload » (cwm.objectConflict) instead
 * of overwriting it. The pane records it when it loads the object, and every
 * save records the version it just wrote.
 */
const known = new Map<string, string>()

/** Records the version read or written for an object (nothing when absent). */
export function noteRevision(id: string | undefined, revision: string | undefined): void {
  if (id && revision) known.set(id, revision)
}

/** The version to send with a save; empty when none was read (an unconditional write). */
export function revisionOf(id: string | undefined): string {
  return (id && known.get(id)) || ''
}

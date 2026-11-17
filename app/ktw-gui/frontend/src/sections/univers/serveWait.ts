// SPDX-License-Identifier: Apache-2.0
// THE WAIT AFTER A CHANGE OF WHAT A UNIVERSE SERVES:
// an attach, a detach, a creation or a switch is recorded by the hub, which
// restarts the member serving the universe's objects; the GUI waits until
// that member serves the change. A change recorded but not served is shown as
// it is, with the reason - never as a silent success.

/** NotServedError: the hub switched the project, but the universe does not
 *  serve it - its serving member did not restart onto it, or not in time. */
export class NotServedError extends Error {
  constructor(readonly reason: string) {
    super(reason)
    this.name = 'NotServedError'
  }
}

/** samePath compares two folders as the system does: separators and, on
 *  Windows drives, case aside. */
export function samePath(a: string, b: string): boolean {
  const norm = (p: string) => {
    const s = p.replace(/\\/g, '/').replace(/\/+$/, '')
    return /^[A-Za-z]:/.test(s) ? s.toLowerCase() : s
  }
  return !!a && !!b && norm(a) === norm(b)
}

/** What a wait expects the universe to serve: a folder (an attach, a
 *  creation, a switch), '' for no project at all, or { gone } - a detached
 *  folder served no more, the universe serving its other members still
 *  (several repositories in one universe). */
export type ServeWant = string | { gone: string }

/** servesWant: the folders served now meet what the wait expects. */
function servesWant(dirs: string[], want: ServeWant): boolean {
  if (typeof want !== 'string') return !dirs.some((d) => samePath(d, want.gone))
  return want === '' ? dirs.length === 0 : dirs.some((d) => samePath(d, want))
}

/** waitUntilServed resolves once served() - every folder the universe serves
 *  now - meets want (see ServeWant), read every everyMs; past timeoutMs it
 *  throws a NotServedError naming what is served. A read that fails (the
 *  member restarting) is read again: only a read that answers can end the
 *  wait. */
export async function waitUntilServed(
  served: () => Promise<string[]>, want: ServeWant, opts: { timeoutMs?: number; everyMs?: number } = {},
): Promise<void> {
  const timeoutMs = opts.timeoutMs ?? 60_000
  const everyMs = opts.everyMs ?? 1_000
  const end = Date.now() + timeoutMs
  let last: string[] = []
  for (;;) {
    const read = await served().then((d) => ({ ok: true, dirs: d }), () => ({ ok: false, dirs: last }))
    last = read.dirs
    if (read.ok && servesWant(last, want)) return
    if (Date.now() >= end) {
      const secs = Math.round(timeoutMs / 1000)
      throw new NotServedError(last.length
        ? `after ${secs} s the universe still serves ${last.join(', ')}`
        : want === ''
          ? `after ${secs} s the universe does not answer what it serves`
          : `after ${secs} s the universe serves no project yet`)
    }
    await new Promise((r) => setTimeout(r, everyMs))
  }
}

/** What the hub answers about serving a change it recorded (an attach, a
 *  detach, a creation or a switch): the member serving the universe's objects
 *  restarted onto it, or why it could not be. Neither: the universe, or that
 *  member, does not run and starts on the change. */
export interface ServeOutcome {
  restarted?: boolean
  restartError?: string
}

/** serveChange waits until the universe serves expected (see ServeWant; a
 *  detach waits for its folder gone) once the hub restarted its serving
 *  member; a restart that failed rejects with a NotServedError saying why. */
export async function serveChange(
  out: ServeOutcome, expected: ServeWant, served: () => Promise<string[]>, opts?: { timeoutMs?: number; everyMs?: number },
): Promise<void> {
  if (out.restartError) throw new NotServedError(out.restartError)
  if (out.restarted) await waitUntilServed(served, expected, opts)
}

/** notServedText is what a project's attach, detach or creation shows when it
 *  failed: recorded but not served, with the reason, for a NotServedError;
 *  the hub's refusal as it is otherwise. */
export function notServedText(err: unknown, t: (key: string, vars?: Record<string, unknown>) => string): string {
  if (err instanceof NotServedError) return t('projects.notServed', { reason: err.reason })
  return err instanceof Error ? err.message : String(err)
}

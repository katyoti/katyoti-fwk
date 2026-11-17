// SPDX-License-Identifier: Apache-2.0
/**
 * window.__ktwBusy — how many of the GUI's request/response calls to its Go
 * side are in flight.
 *
 * Every backend call goes through a Wails binding (window.go.main.App.<name>:
 * CallKTSPMethod for the typed KTSP clients, WorkspaceRunAction and the other
 * Workspace* calls for actions and objects). Those calls cross the Wails IPC,
 * which a browser does not show as network, so a script driving the page
 * (browse/wait-idle) cannot tell the screen is still loading. Each binding is
 * wrapped once: the counter goes up when the call starts and down when it
 * ends, answered or failed. Wails events and the panel stream are not
 * bindings: they are never counted. A call that waits on a person (a file or
 * folder dialog, the save dialog of SaveDocument) is not counted either, nor
 * the frontend's own log lines.
 */

type Bindings = Record<string, unknown>

/** The bindings that never count: they wait on a person, or only log. */
const NOT_COUNTED = /^(Pick[A-Z]|FeedbackPickFolder$|SaveDocument$|LogFromFrontend$)/

const MARK = '__ktwBusyCounted'

interface BusyWindow {
  __ktwBusy?: number
  go?: { main?: { App?: Bindings } }
}

function win(): BusyWindow {
  return globalThis as unknown as BusyWindow
}

/** The calls in flight now. */
export function busyCount(): number {
  return win().__ktwBusy ?? 0
}

function add(n: number): void {
  const w = win()
  w.__ktwBusy = Math.max(0, (w.__ktwBusy ?? 0) + n)
}

/** Wraps each binding of `app` (once) so its calls count in window.__ktwBusy. */
export function countBusyCalls(app: Bindings): void {
  if (win().__ktwBusy === undefined) win().__ktwBusy = 0
  for (const name of Object.keys(app)) {
    const fn = app[name]
    if (typeof fn !== 'function' || NOT_COUNTED.test(name)) continue
    if ((fn as unknown as Record<string, unknown>)[MARK]) continue
    const orig = fn as (...a: unknown[]) => unknown
    const counted = (...args: unknown[]): unknown => {
      add(1)
      let out: unknown
      try {
        out = orig(...args)
      } catch (err) {
        add(-1)
        throw err
      }
      if (out && typeof (out as Promise<unknown>).then === 'function') {
        ;(out as Promise<unknown>).then(() => add(-1), () => add(-1))
      } else {
        add(-1)
      }
      return out
    }
    ;(counted as unknown as Record<string, unknown>)[MARK] = true
    app[name] = counted
  }
}

/** Sets window.__ktwBusy to 0 at once and counts the bindings as soon as the
 *  Wails runtime has injected them (after the bundle loads). */
export function installBusyCount(): void {
  if (win().__ktwBusy === undefined) win().__ktwBusy = 0
  const tryWrap = (): boolean => {
    const app = win().go?.main?.App
    if (!app) return false
    countBusyCalls(app)
    return true
  }
  if (tryWrap()) return
  let tries = 0
  const iv = setInterval(() => {
    tries++
    if (tryWrap() || tries > 50) clearInterval(iv)
  }, 100)
}

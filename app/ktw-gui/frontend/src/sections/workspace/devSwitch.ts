// SPDX-License-Identifier: Apache-2.0
// A DISPLAY SWITCH ON FOR A SCENARIO CASE'S DEV GUI ONLY: a live
// check needs the display an increment keeps behind its switch, while the
// person sees nothing of it until the delivery is approved.
// VITE_KTW_DISPLAY_SWITCHES lists the increments whose display is on
// (in-036,in-037); it is read in a dev build (wails dev) only - the installed
// build never turns a switch on this way.
export function devSwitchOn(increment: string): boolean {
  const env = (import.meta as { env?: Record<string, unknown> }).env
  if (!env?.DEV) return false
  return String(env.VITE_KTW_DISPLAY_SWITCHES ?? '')
    .split(',')
    .map((s) => s.trim().toLowerCase())
    .includes(increment)
}

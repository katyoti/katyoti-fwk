// SPDX-License-Identifier: Apache-2.0
import { useEffect } from 'react'
import { create } from 'zustand'
import { DisplaySwitches } from '../../wailsjs/go/main/App'

/**
 * An increment's display switch: what the increment
 * changes on screen shows only once the person approves its delivery, the
 * former display showing until then. This start of the GUI's switches are
 * read once from the backend (KTW_IN<n>_DISPLAY - in a scenario case's
 * environment); a switch is off while they are read, and when
 * they cannot be.
 */
const useSwitches = create<{ on: Record<string, boolean>; asked: boolean }>(() => ({ on: {}, asked: false }))

/** Whether a switch is on in what this start read: off unless it said so. */
export function switchOn(on: Record<string, boolean> | null | undefined, name: string): boolean {
  return on?.[name] === true
}

/** Whether the display switch `name` (an increment: `in<n>`) is on. */
export function useDisplaySwitch(name: string): boolean {
  const on = useSwitches((s) => s.on)
  useEffect(() => {
    if (useSwitches.getState().asked) return
    useSwitches.setState({ asked: true })
    DisplaySwitches().then(
      (v) => useSwitches.setState({ on: v ?? {} }),
      () => useSwitches.setState({ on: {} }),
    )
  }, [])
  return switchOn(on, name)
}

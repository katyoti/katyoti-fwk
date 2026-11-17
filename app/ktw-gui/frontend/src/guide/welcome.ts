// SPDX-License-Identifier: Apache-2.0
// The welcome screens - a universe's first guide and « Quoi de neuf » - stay
// closed when this start of the GUI says so (--no-welcome, KTW_NO_WELCOME) or
// when the universe on screen turned them off (Univers.yaml no_welcome),
// Tests and screen captures start clean. Without either, nothing
// changes for a person.

import { useEffect } from 'react'
import { create } from 'zustand'
import { NoWelcome } from '../../wailsjs/go/main/App'
import { useAppStore } from '../store'

/** welcomeOff decides whether the welcome screens stay closed: the start's
 *  option, or the universe's own setting. */
export function welcomeOff(startOption: boolean, universe?: { noWelcome?: boolean }): boolean {
  return startOption || !!universe?.noWelcome
}

const useStart = create<{ option: boolean | undefined; asked: boolean }>(() => ({ option: undefined, asked: false }))

/** Whether the welcome screens stay closed for the universe on screen;
 *  undefined while the start's option is read (open nothing until known). */
export function useWelcomeOff(universe: string | null | undefined): boolean | undefined {
  const option = useStart((s) => s.option)
  const summary = useAppStore((s) => s.universes.find((u) => u.name === universe))
  useEffect(() => {
    if (useStart.getState().asked) return
    useStart.setState({ asked: true })
    NoWelcome().then((v) => useStart.setState({ option: !!v }), () => useStart.setState({ option: false }))
  }, [])
  if (option === undefined) return undefined
  return welcomeOff(option, summary as { noWelcome?: boolean } | undefined)
}

// SPDX-License-Identifier: Apache-2.0
import { useEffect, useRef } from 'react'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import { useAppStore } from '../store'
import { listUniversesWithStatus } from '../ktsp/universes'
import { openableTarget } from '../sections/shell/workUniverses'
import { openAddress } from '../sections/shell/openAddress'
import { settleStart } from '../sections/shell/startScreens'

/**
 * Loads the registered universes from disk via Wails (App.UniverseList).
 *
 * Boot lands on the welcome state (the universe-picker banner) UNLESS the app
 * was launched with a target universe — `katyoti gui --universe <name>` (or the
 * startup default_universe), surfaced by the Go side as App.InitialUniverse().
 * When that name is set and present in the registry, the hook ACTIVATES it (the
 * same path the IconRibbon uses — spawn the core, then display), so the
 * universe-scoped panels have a running backend to read. Otherwise the user
 * picks explicitly (no auto-resume from history).
 * `localStorage ktw.lastDisplayed` is still written when the user picks (see
 * store.displayUniverse) but is not read on boot.
 *
 * Universe changes made IN this GUI refresh through the store actions. Changes
 * made ANYWHERE ELSE — a terminal running `katyoti universe create`, a second
 * GUI window — arrive as the `universes.changed` Wails event, forwarded from the
 * admin universe's bus. Without that subscription a universe created from a
 * terminal stayed invisible here until something else happened to trigger a
 * re-fetch.
 *
 * The event carries no payload on purpose: we re-read the registry rather than
 * trust a snapshot travelling in an event, so the list cannot go stale.
 */
export function useUniverses() {
  const setUniverses        = useAppStore((s) => s.setUniverses)
  const setUniversesError   = useAppStore((s) => s.setUniversesError)
  const activateUniverse    = useAppStore((s) => s.activateUniverse)
  const universesLoaded     = useAppStore((s) => s.universesLoaded)

  const booted = useRef(false)

  useEffect(() => {
    if (booted.current) return
    booted.current = true

    const win = window as unknown as {
      go?: { main?: { App?: { InitialUniverse?: () => Promise<string>; InitialOpen?: () => Promise<string> } } }
    }

    // The registry comes from the typed KTSP client; busRunning is joined in from
    // this process's own state (see ktsp/universes.ts).
    listUniversesWithStatus()
      .then(async (list) => {
        setUniverses(list)

        // Honor the launch target: open directly into the universe the launcher
        // pointed us at, when it exists. Absent/unknown → stay on the welcome
        // banner and let the user pick.
        const initFn = win.go?.main?.App?.InitialUniverse
        if (initFn) {
          try {
            // The admin universe is no place to work: a
            // target naming it stays on the welcome — the universes' management.
            const target = openableTarget(list, await initFn())
            if (target) {
              // ACTIVATE, don't merely display. activateUniverse spawns the
              // universe's ktw-agents core AND bumps universeRevision so the
              // universe-scoped panels (workspace, agents, …) re-fetch against
              // it. Calling displayUniverse alone paints the header but leaves
              // no core running, so those panels render empty — or keep showing
              // whichever universe was resolved before.
              await activateUniverse(target)
            }
          } catch (err) {
            // eslint-disable-next-line no-console
            console.warn('[universes] InitialUniverse failed:', err)
          }
        }
        // An address to start on: `--open <address>` or a
        // katyoti:// link, else the development page's `#/open/<address>`.
        const open = (await win.go?.main?.App?.InitialOpen?.().catch(() => '')) || (window.location.hash.startsWith('#/open/') ? window.location.hash : '')
        if (open) await openAddress(open)
      })
      .catch((err) => {
        // eslint-disable-next-line no-console
        console.warn('[universes] UniverseList failed:', err)
        setUniversesError(String(err))
      })
      // The start has settled on its universe (or the picker): the screens
      // that open by themselves may come, in their order.
      .finally(settleStart)
  }, [setUniverses, setUniversesError, activateUniverse])

  // A link opened while the GUI runs: the launch it came with
  // hands its address to this window (single instance per universe).
  useEffect(() => EventsOn('open.address', (addr: string) => { void openAddress(addr) }), [])

  // Live registry refresh. Separate effect from boot: this one re-subscribes
  // independently and must NOT be gated by the `booted` ref.
  useEffect(() => {
    const off = EventsOn('universes.changed', () => {
      listUniversesWithStatus()
        .then(setUniverses)
        .catch((err) => {
          // A failed refresh leaves the previous list on screen, which is the
          // right call: a transient bus hiccup should not blank the ribbon.
          // eslint-disable-next-line no-console
          console.warn('[universes] refresh after universes.changed failed:', err)
        })
    })
    return () => { off() }
  }, [setUniverses])

  return { universesLoaded }
}

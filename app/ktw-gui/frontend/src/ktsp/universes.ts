// SPDX-License-Identifier: Apache-2.0
/**
 * Universe reads that need BOTH the hub-served registry and this GUI process's
 * own state.
 *
 * `busRunning` is not a service fact — it means "this ktw-gui process has a core
 * running for that universe", which only the process knows. So it cannot come
 * from the typed KTSP client, and the service must not be asked to carry it.
 *
 * The join lives HERE, in one place, rather than in a Go binding that would
 * re-model the whole `list-universes` result just to add one boolean (that is the
 * per-method-binding pattern rule `management-plane-hub-authority` invariant 5
 * removes). What stays in Go is only the part KTSP genuinely cannot express:
 * `UniverseListActive`, which returns process-local state and nothing else.
 */

import { orchestratorClient } from './index'
import type { ListUniversesOutput } from './orchestrator'

/** One registered universe as the service reports it. */
export type UniverseSummary = ListUniversesOutput['universes'][number]

/** A universe plus whether THIS process is running a core for it. */
export type UniverseWithStatus = UniverseSummary & {
  busRunning: boolean
  /** The GUI found the universe's hub running: it keeps running once the GUI closes. */
  hubKeepsRunning?: boolean
}

interface ActiveUniverseInfo {
  name: string
  homePath?: string
  startedAtUnix?: number
  hubKeepsRunning?: boolean
}

/** Reads the process-local active-universes map. Kept as a Wails binding because
 *  it is GUI state, not service state. Absent outside Wails → nothing is active. */
async function listActive(): Promise<ActiveUniverseInfo[]> {
  const win = window as unknown as {
    go?: {
      main?: {
        App?: {
          UniverseListActive?: () => Promise<{ active?: ActiveUniverseInfo[]; error?: string }>
        }
      }
    }
  }
  const fn = win.go?.main?.App?.UniverseListActive
  if (!fn) return []
  const res = await fn()
  // A failure here must not sink the universe list: the registry is the payload,
  // running-state is an annotation. Report nothing and show everything as stopped.
  if (res?.error) return []
  return res?.active ?? []
}

/** The registered universes, each annotated with this process's running state. */
export async function listUniversesWithStatus(): Promise<UniverseWithStatus[]> {
  const [out, active] = await Promise.all([
    orchestratorClient().listUniverses({}),
    listActive(),
  ])
  const running = new Map(active.map((a) => [a.name, a]))
  return (out.universes ?? []).map((u) => ({
    ...u,
    busRunning: running.has(u.name),
    hubKeepsRunning: running.get(u.name)?.hubKeepsRunning === true,
  }))
}

// SPDX-License-Identifier: Apache-2.0
/**
 * readyStore — what the Ready to start screen and its badge read about each
 * universe, shared by both.
 *
 * `check(universe)` asks the hub-owned marketplace service, over KTSP through
 * the generated client, in this order: the keys (KeySets — it also names the
 * solution's universes found on this machine), then, together, each of those
 * universes' config check (Doctor) and the programs their sidecars need
 * (PlanPrereqs) — and beside them the person's profile, the installation's
 * projects folder and the project folders the universe attaches (where its
 * first study goes). A part that cannot be read keeps its error; the others still
 * show. The GUI writes nothing here: every change goes through a service call
 * the screen makes, then `check` again.
 *
 * Two per-person conveniences live in localStorage (wrapped: a blocked storage
 * only means the screen opens again, or the password is offered again): the
 * universes the screen already opened by itself in, and the universes whose key
 * sets were imported. The first is kept per universe and installed version,
 * so the screen opens again at the first start after an update.
 */

import { create } from 'zustand'
import { marketplaceClient, orchestratorClient, profileClient } from '../../ktsp'
import type { KeySetsInput } from '../../ktsp/marketplace'
import { useAppStore } from '../../store'
import { installationVersion, readyLogLine, seenKey, splitRef, type ReadyInputs, type SidecarCheck } from './readiness'

const SEEN_KEY = 'ktw.ready.seen'
const IMPORTED_KEY = 'ktw.ready.keySetsImported'

function readSet(key: string): Set<string> {
  try {
    const raw = globalThis.localStorage?.getItem(key)
    return new Set(raw ? (JSON.parse(raw) as string[]) : [])
  } catch {
    return new Set()
  }
}

function addToSet(key: string, value: string) {
  try {
    const s = readSet(key)
    s.add(value)
    globalThis.localStorage?.setItem(key, JSON.stringify([...s]))
  } catch { /* storage blocked: asked again next time */ }
}

/** The universes (at an installed version, seenKey) the screen already opened by itself in. */
export const seenUniverses = () => readSet(SEEN_KEY)
export const markSeen = (universe: string, version = '') => addToSet(SEEN_KEY, seenKey(universe, version))

/** The installed version this start runs in a universe (installationVersion):
 *  the installed katyoti, from the hub's list of installed items, and the
 *  universe's solution ref. A list that cannot be read leaves the solution's. */
export async function installedVersion(universe: string): Promise<string> {
  let katyoti: string | undefined
  try {
    katyoti = ((await marketplaceClient().listInstalled()) ?? []).find((i) => i.name === 'katyoti')?.version
  } catch { /* no list: the solution's version alone */ }
  const ref = useAppStore.getState().universes.find((x) => x.name === universe)?.solution
  return installationVersion(katyoti, ref)
}
/** Whether the key sets of this universe's solution were imported. */
export const keySetsImported = (universe: string) => readSet(IMPORTED_KEY).has(universe)
export const markKeySetsImported = (universe: string) => addToSet(IMPORTED_KEY, universe)

/** A universe's home and solution, from the registry the GUI already holds. */
export function universeTarget(universe: string): { home: string; solution: KeySetsInput['solution'] } {
  const u = useAppStore.getState().universes.find((x) => x.name === universe)
  const ref = u?.solution ?? ''
  return { home: u?.homePath ?? '', solution: ref ? splitRef(ref) : undefined }
}

const errText = (e: unknown) => (e instanceof Error ? e.message : String(e))

interface ReadyStoreState {
  byUniverse: Record<string, ReadyInputs>
  /** Read the universe's keys, config check and programs plan again. */
  check: (universe: string) => Promise<void>
}

const EMPTY: ReadyInputs = { loading: false, errors: {} }

export const useReadyStore = create<ReadyStoreState>((set, get) => ({
  byUniverse: {},
  check: async (universe) => {
    const { home, solution } = universeTarget(universe)
    if (!home) return
    const prev = get().byUniverse[universe] ?? EMPTY
    const put = (patch: Partial<ReadyInputs>) =>
      set((s) => ({ byUniverse: { ...s.byUniverse, [universe]: { ...(s.byUniverse[universe] ?? prev), ...patch } } }))
    put({ loading: true, errors: {} })
    const mkt = marketplaceClient()
    const errors: ReadyInputs['errors'] = {}

    let homes = [home]
    try {
      const keys = await mkt.keySets({ universeHome: home, ...(solution ? { solution } : {}) })
      put({ keys })
      if (keys.universes?.length) homes = keys.universes
    } catch (e) {
      errors.keys = errText(e)
    }

    await Promise.all([
      (async () => {
        try {
          const per = await Promise.all(homes.map(async (h) =>
            ((await mkt.doctor({ universeHome: h })) ?? []).map((d): SidecarCheck => ({ ...d, universeHome: h }))))
          put({ doctor: per.flat() })
        } catch (e) {
          errors.doctor = errText(e)
        }
      })(),
      (async () => {
        try {
          put({ plan: await mkt.planPrereqs({ universes: homes }) })
        } catch (e) {
          errors.plan = errText(e)
        }
      })(),
      (async () => {
        try {
          put({ profile: await profileClient().get() })
        } catch (e) {
          errors.profile = errText(e)
        }
      })(),
      (async () => {
        try {
          put({ projectsFolder: await mkt.projectsFolder() })
        } catch (e) {
          errors.projectsFolder = errText(e)
        }
      })(),
      (async () => {
        try {
          put({ projects: (await orchestratorClient().listProjects({ universe })).projects ?? [] })
        } catch (e) {
          errors.projects = errText(e)
        }
      })(),
    ])
    put({ loading: false, errors })
    logLine('ready', readyLogLine(universe, get().byUniverse[universe] ?? EMPTY))
  },
}))

type GoBindings = {
  go?: { main?: { App?: { LogFromFrontend?: (tag: string, msg: string) => Promise<void> } } }
}

/** A line in the GUI's log file (production builds have no DevTools). */
function logLine(tag: string, msg: string) {
  try {
    const fn = (globalThis as unknown as GoBindings).go?.main?.App?.LogFromFrontend
    if (fn) void fn(tag, msg)
  } catch { /* no binding: a browser run, nothing to log to */ }
}

/** The inputs read for a universe (empty until the first check). */
export function useReadyInputs(universe: string | null): ReadyInputs {
  return useReadyStore((s) => (universe ? s.byUniverse[universe] : undefined) ?? EMPTY)
}

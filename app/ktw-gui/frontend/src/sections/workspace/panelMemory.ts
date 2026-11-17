// SPDX-License-Identifier: Apache-2.0
/**
 * panelMemory — what a panel shows that is not on a tab's payload (a Global
 * panel's target and query, the Inbox's open item and its draft answer),
 * kept across the panel's unmount so the person finds it as they left it when
 * they come back from an object they opened there. Only the active tab is
 * rendered: without it, switching tabs reset these to their defaults.
 *
 * Keyed by the panel and its universe; mirrored to the browser's
 * localStorage (one key, JSON) so a reload of the GUI finds it too — « À
 * faire » on the action the person chose. A per-viewer convenience: absent,
 * full or corrupt storage leaves the memory in memory, never an error.
 */

/** The localStorage key holding every panel's memory. */
export const PANEL_MEMORY_KEY = 'ktw.panel-memory'

type Memory = Map<string, Record<string, unknown>>

function storage(): Storage | undefined {
  try {
    return (globalThis as { localStorage?: Storage }).localStorage ?? undefined
  } catch {
    return undefined
  }
}

/** The memory the storage holds: empty when it holds none or junk. */
function load(): Memory {
  try {
    const raw = storage()?.getItem(PANEL_MEMORY_KEY)
    if (!raw) return new Map()
    const all: unknown = JSON.parse(raw)
    if (!all || typeof all !== 'object' || Array.isArray(all)) return new Map()
    return new Map(Object.entries(all as Record<string, unknown>)
      .filter((e): e is [string, Record<string, unknown>] => !!e[1] && typeof e[1] === 'object' && !Array.isArray(e[1])))
  } catch {
    return new Map()
  }
}

function save(): void {
  try {
    storage()?.setItem(PANEL_MEMORY_KEY, JSON.stringify(Object.fromEntries(memory)))
  } catch { /* no storage, or full: kept in memory only */ }
}

let memory: Memory = load()

/** The value `field` of the panel `key` was left with, else `fallback`. */
export function recall<T>(key: string, field: string, fallback: T): T {
  const m = memory.get(key)
  return m && field in m ? (m[field] as T) : fallback
}

/** Keeps `value` as what `field` of the panel `key` shows. */
export function remember(key: string, field: string, value: unknown): void {
  memory.set(key, { ...(memory.get(key) ?? {}), [field]: value })
  save()
}

/** Reads the memory back from the storage, as a reload of the GUI does (tests). */
export function reloadPanels(): void {
  memory = load()
}

/** Forgets every panel, in memory and in the storage (tests). */
export function forgetPanels(): void {
  memory.clear()
  try {
    storage()?.removeItem(PANEL_MEMORY_KEY)
  } catch { /* no storage */ }
}

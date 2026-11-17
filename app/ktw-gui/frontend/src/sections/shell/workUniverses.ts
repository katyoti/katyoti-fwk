// SPDX-License-Identifier: Apache-2.0
/**
 * Where the admin universe goes (not listed). The
 * admin universe manages the others and holds no work, so it is never shown
 * as a place to work: the rail lists only work universes, a launch target
 * naming it lands on the universes' management (the picker), and its card
 * there opens its settings. It is the one registry entry marked `protected`,
 * the mark the picker already reads.
 */
export interface RegistryUniverse {
  name: string
  protected?: boolean
}

export function isAdminUniverse(u: RegistryUniverse): boolean {
  return u.protected === true
}

/** The universes the rail lists: every one but the admin universe. */
export function railUniverses<U extends RegistryUniverse>(list: U[]): U[] {
  return list.filter((u) => !isAdminUniverse(u))
}

/** The launch target to open at boot, or null: unknown, or the admin universe. */
export function openableTarget(list: RegistryUniverse[], target: string | undefined): string | null {
  const name = target?.trim()
  if (!name) return null
  const u = list.find((x) => x.name === name)
  return u && !isAdminUniverse(u) ? u.name : null
}

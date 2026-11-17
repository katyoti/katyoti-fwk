// SPDX-License-Identifier: Apache-2.0
/**
 * What the left panel's header names: the universe on display, as the rail
 * shows it (its icon, colour and display name). The project is the tree's
 * root and is not repeated here; it names the header only while the universe
 * list has not loaded.
 */
export interface HeaderUniverse {
  name: string
  displayName?: string
  icon: string
  color: string
}

export interface HeaderProject {
  name: string
  icon: string
  color: string
}

export interface HeaderIdentity {
  label: string
  icon: string
  color: string
  universe: boolean
}

export function headerIdentity(
  universes: HeaderUniverse[],
  displayed: string | null | undefined,
  project: HeaderProject,
): HeaderIdentity {
  const u = displayed ? universes.find((x) => x.name === displayed) : undefined
  if (u) {
    return { label: u.displayName || u.name, icon: u.icon, color: u.color, universe: true }
  }
  return { label: project.name, icon: project.icon, color: project.color, universe: false }
}

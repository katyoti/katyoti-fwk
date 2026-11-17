// SPDX-License-Identifier: Apache-2.0
/**
 * contributionsModel — the Contributions panel's data, as
 * project-contributions/Panel answers it (the entries the universe publishes
 * under the project's provider token, a group per kind), and what the view
 * derives from it: the groups a filter keeps, the count.
 */

/** One contribution: its URI opens it. */
export interface ContributionEntry {
  uri: string
  id: string
  name?: string
  description?: string
}

/** The entries of one kind. */
export interface ContributionGroup {
  kind: string
  entries: ContributionEntry[]
}

/** A member's entry the project overrides, and the layers it serves of it. */
export interface Overridden {
  uri: string
  layers: string[]
}

/** The panel's input. */
export interface ProjectContributions {
  provider: string
  folder: string
  groups: ContributionGroup[]
  overrides: Overridden[]
}

/** The groups whose entries match the filter (the kind, or an entry's id,
 *  name or description, case-folded), each with only its matching entries;
 *  empty groups dropped. An empty filter keeps every group. */
export function filterGroups(groups: ContributionGroup[], filter: string): ContributionGroup[] {
  const q = filter.trim().toLowerCase()
  if (!q) return groups
  return groups
    .map((g) => ({
      kind: g.kind,
      entries: g.entries.filter((e) =>
        g.kind.toLowerCase().includes(q) ||
        [e.id, e.name ?? '', e.description ?? ''].some((s) => s.toLowerCase().includes(q))),
    }))
    .filter((g) => g.entries.length > 0)
}

/** The overridden entries a filter keeps (the URI or a layer, case-folded). */
export function filterOverrides(list: Overridden[], filter: string): Overridden[] {
  const q = filter.trim().toLowerCase()
  if (!q) return list
  return list.filter((o) => o.uri.toLowerCase().includes(q) || o.layers.some((l) => l.toLowerCase().includes(q)))
}

/** The provider, kind and id a contribution URI names (`<provider>:/<kind>/<id>`). */
export function partsOf(uri: string): { provider: string; kind: string; id: string } {
  const at = uri.indexOf(':/')
  const provider = at < 0 ? '' : uri.slice(0, at)
  const rest = at < 0 ? uri : uri.slice(at + 2)
  const slash = rest.indexOf('/')
  return { provider, kind: slash < 0 ? rest : rest.slice(0, slash), id: slash < 0 ? '' : rest.slice(slash + 1) }
}

/** How many entries the groups hold. */
export function countOf(groups: ContributionGroup[]): number {
  return groups.reduce((n, g) => n + g.entries.length, 0)
}

/** The input, read defensively: a source that answered nothing is no group. */
export function asContributions(input: unknown): ProjectContributions {
  const o = (input ?? {}) as Partial<ProjectContributions>
  return {
    provider: o.provider ?? '',
    folder: o.folder ?? '',
    groups: Array.isArray(o.groups) ? o.groups : [],
    overrides: Array.isArray(o.overrides) ? o.overrides : [],
  }
}

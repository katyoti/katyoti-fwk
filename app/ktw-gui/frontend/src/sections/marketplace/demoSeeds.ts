// SPDX-License-Identifier: Apache-2.0
/**
 * demoSeeds — whether a project template (a seed, by its internal name) is
 * demo data, from the marketplace's seed list (list-seeds carries `demo`).
 * The solution detail's « Modèle de projet » line draws the
 * « Démo » badge from it, as the Project templates tab does.
 */

/** A listed seed, as far as the demo badge reads it. */
export interface ListedSeed { name: string; marketplace?: string; demo?: boolean }

/** Whether the seed `name` a solution of `marketplace` pins is demo data: the
 *  seed of that name in the solution's marketplace, else any seed of that name. */
export function isDemoSeed(seeds: readonly ListedSeed[], name: string, marketplace?: string): boolean {
  const named = seeds.filter((s) => s.name === name)
  const own = named.find((s) => marketplace && s.marketplace === marketplace)
  return !!(own ?? named[0])?.demo
}

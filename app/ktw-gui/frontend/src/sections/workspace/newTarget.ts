// SPDX-License-Identifier: Apache-2.0
// « + New » from the top bar and by a keyboard shortcut: for the
// place selected in the tree - the object open in the Object tab - the types
// one may add under it; at the universe's level, with nothing selected, the
// project types (a new dossier d'architecture is a project).

import type { DeclaredTypeStyle } from './type-style'

/** Where « + New » adds: under the selected object, or a new project. */
export type NewTarget = { kind: 'object'; parent: string } | { kind: 'project' }

/** newTargetFor is « + New »'s target for the place selected. */
export function newTargetFor(selected: string | undefined): NewTarget {
  return selected ? { kind: 'object', parent: selected } : { kind: 'project' }
}

/** The default project type, offered first. */
export const DEFAULT_PROJECT_TYPE = 'software-project'

/** projectTypes are the types carrying the project level that the
 *  universe's view allows - by `project:<type>`, a group's child projects and
 *  the universe's new project alike - the default one first, then
 *  by label. */
export function projectTypes(styles: Record<string, DeclaredTypeStyle>, allows: (id: string) => boolean = () => true): string[] {
  return Object.keys(styles)
    .filter((t) => styles[t]?.level === 'project' && allows(`project:${t}`))
    .sort((a, b) => (a === DEFAULT_PROJECT_TYPE ? -1 : b === DEFAULT_PROJECT_TYPE ? 1 : (styles[a].label ?? a).localeCompare(styles[b].label ?? b)))
}

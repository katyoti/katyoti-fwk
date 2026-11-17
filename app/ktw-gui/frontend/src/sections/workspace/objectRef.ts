// SPDX-License-Identifier: Apache-2.0
// THE ID THE GUI SENDS FOR AN OBJECT. A universe
// serving several projects answers each object bare (`<module>.<local>`) with
// the key of the project holding it (cwm's store set); two projects
// may hold one bare id. Opening, saving and listing the actions of such an
// object goes by its qualified id `<key>:<module>.<local>` (cwmapi.QualifiedID)
// - the one grammar, never another join. A row naming no project (a
// universe serving one, whose answers carry no key) keeps the bare id: today's
// calls, unchanged.

/** objectRef is the id to send for object `id` of the row or answer naming
 *  `project`: qualified when a project is named, bare otherwise. An id written
 *  qualified already (a reference leaving its project) is kept as it is. */
export function objectRef(id: string, project?: string | null): string {
  const key = (project ?? '').trim()
  if (!id || !key || id.includes(':')) return id
  return `${key}:${id}`
}

/** projectOfRef is the project key an id sent names: the key of a qualified
 *  id, "" for a bare one. A link or child of that object written bare is of
 *  the same project (a bare id is relative to its container). */
export function projectOfRef(id: string | undefined): string {
  const i = (id ?? '').indexOf(':')
  return i > 0 ? (id as string).slice(0, i) : ''
}

/** bareOfRef is the bare id `<module>.<local>` an id sent names: a qualified
 *  id without its key, a bare one as it is (an object event, which names no
 *  project, is matched by it). */
export function bareOfRef(id: string | undefined): string {
  const s = id ?? ''
  const i = s.indexOf(':')
  return i > 0 ? s.slice(i + 1) : s
}

/** rowProject is the project a page's row names (an object row's `project`,
 *  object-pane ObjectRow), "" when it names none: a bare id an action of that
 *  row carries (navigate, drawer, create under, chat on) is of that project. */
export function rowProject(item: unknown): string {
  const p = (item as { project?: unknown } | null | undefined)?.project
  return typeof p === 'string' ? p.trim() : ''
}

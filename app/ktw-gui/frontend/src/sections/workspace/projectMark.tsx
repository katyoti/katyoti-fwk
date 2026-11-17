// SPDX-License-Identifier: Apache-2.0
// THE PROJECT OF EACH OBJECT, shown for everyone (no display
// switch: the one it had was retired at the person's word). A universe serving several
// projects - two project folders of one repository, or two repositories -
// answers each object with the key of the project holding it (cwm's store
// set); the tree's nodes, the object pane's header and the search
// results then show it as a mark. A universe serving one project (no key in
// its answers) shows no mark.
import { Badge } from '@mantine/core'
import { useT } from '../../i18n'

/** The project mark to draw for an object: its project's key when the answer
 *  names one; null otherwise. */
export function projectMarkOf(project: string | null | undefined): string | null {
  const key = (project ?? '').trim()
  return key ? key : null
}

/** unlocate reads one object of a listed or searched answer in either form:
 *  a bare object (one store) or a located one (`{project, object}`, the
 *  store set) - the object, with the project holding it when named. */
export function unlocate<T extends object>(item: T | { project?: string; object?: T }): T & { project?: string } {
  const l = item as { project?: string; object?: T }
  if (l && typeof l === 'object' && l.object && typeof l.object === 'object') {
    return { ...l.object, ...(l.project ? { project: l.project } : {}) }
  }
  return item as T & { project?: string }
}

/** ProjectMark draws an object's project mark where `guide` places it
 *  (tree.project-mark, object.project-mark, search.project-mark). */
export function ProjectMark({ project, guide }: { project?: string; guide: string }) {
  const t = useT('workspace')
  const mark = projectMarkOf(project)
  return mark ? <ProjectMarkView mark={mark} guide={guide} title={t('tree.projectMark', { name: mark })} /> : null
}

/** ProjectMarkView is the mark itself: the project's key, by its element id. */
export function ProjectMarkView({ mark, guide, title }: { mark: string; guide: string; title?: string }) {
  return (
    <Badge size="xs" variant="outline" color="grape" data-guide={guide} data-project={mark}
      title={title} style={{ textTransform: 'none', flexShrink: 0 }}>
      {mark}
    </Badge>
  )
}

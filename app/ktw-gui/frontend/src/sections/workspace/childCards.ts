// SPDX-License-Identifier: Apache-2.0
/**
 * A holder's children as object cards:
 * the children tab draws each child with the display
 * library's one object card, filled from the row the pane
 * resolved - its title, id, lifecycle state and declared badges -, a click
 * opening the child in the pane, and a « ⋯ » menu to open it or retire it
 * (« Retirer » archives). Kept free of React so it is tested on
 * its own (tests/child-cards.test.ts).
 */
import type { UIPageNode } from '@ktw/display-lib/ui-builders/page/index.ts'
import { objectRef } from './objectRef'

/** A badge as the pane resolves it (a WorkspaceBadge). */
export interface ChildBadge { id?: string; label: string; icon?: string; color?: string; tooltip?: string }

/** The children group a holder's tab lists (a WorkspaceObjectChildGroup). */
export interface ChildGroup {
  type: string
  icon?: string
  objects?: Array<{ id: string; title?: string; type?: string }>
  rows?: Array<{ id: string; state?: ChildBadge; badges?: ChildBadge[] }>
}

/** The words the cards speak, in the GUI language. */
export interface ChildCardTexts { open: string; retire: string; retireConfirm: string; empty: string }

function badge(b: ChildBadge): ChildBadge {
  return { ...(b.id ? { id: b.id } : {}), label: b.label, ...(b.icon ? { icon: b.icon } : {}), ...(b.color ? { color: b.color } : {}), ...(b.tooltip ? { tooltip: b.tooltip } : {}) }
}

/** childCardsPage is the page of a holder's children: one object card per
 *  child, in the order the pane lists them, in a grid of three; `project`
 *  rides on each row so an action names the child in its own project. */
export function childCardsPage(group: ChildGroup, project: string | undefined, texts: ChildCardTexts): UIPageNode {
  const rows = new Map((group.rows ?? []).map((r) => [r.id, r]))
  const data = (group.objects ?? []).map((o) => {
    const r = rows.get(o.id)
    return {
      id: o.id, title: o.title || o.id, type: o.type || group.type,
      ...(group.icon ? { icon: group.icon } : {}),
      ...(r?.state?.label ? { state: badge(r.state) } : {}),
      ...((r?.badges ?? []).length ? { badges: (r?.badges ?? []).filter((b) => b.label).map(badge) } : {}),
      ...(project ? { project } : {}),
      // The id an action sends: qualified in a universe serving several projects.
      ref: objectRef(o.id, project),
      menu: [
        { id: 'open', label: texts.open, icon: 'external-link', action: { navigate: { object: '${item.id}' } } },
        { id: 'retire', label: texts.retire, icon: 'archive', confirm: texts.retireConfirm, action: { uri: 'ktw-agents:cwm/object-archive', args: { id: '${item.ref}' } } },
      ],
    }
  })
  return {
    kind: 'list', id: `children:${group.type}`, layout: 'grid', columns: 3, data, empty: texts.empty,
    item: { kind: 'object-card', dataField: '$.item', open: { navigate: { object: '${item.id}' } } },
  } as unknown as UIPageNode
}

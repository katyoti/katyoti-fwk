// SPDX-License-Identifier: Apache-2.0
import { ACTIONS_TAB_ELEMENT, CHECK_INSTALL_ELEMENT, ESSENTIAL_HIDDEN, objectTabId, placeOf } from '../../elements'

/**
 * What a universe shows: its resolved view (its tags' allow-lists at the
 * person's level) and the level's own Essentiel rule. The
 * `gui` section of its Univers.yaml, carried by the hub's universe summary
 * (the GUI reads it, never writes it - Univers.yaml is hub-owned), gives the
 * task hashtags listed first:
 *
 *   gui:
 *     first: [archifactory]
 *
 * Its `hide` is retired: the universe's tags alone decide,
 * and the hub names what a universe still declares there (the view's
 * retired_hide). Elements are named by their element id (elements.ts).
 *
 * `first` names task hashtags: Home lists the tasks carrying one before the
 * others, each part in the order the person picked.
 *
 * Pure: HomeTab, IconRibbon and the card grid read it through these functions.
 */

/** A universe's `gui` section, as the summary carries it. */
export interface UniverseGui {
  hide?: string[]
  first?: string[]
}

/** The element id an object tab is left out by: its tab id (the object pane
 *  answers `tab:<stem>`; a former `panel:<stem>` reads as it). */
export function tabSurface(tabId: string): string {
  return objectTabId(tabId)
}

/**
 * Whether an element is left out at a display level: in a
 * universe with tags the view alone decides the places it lists (its tags say
 * at which level each element shows), at every level, Expert
 * included (the view is resolved at the person's level); otherwise the
 * level's Essentiel rule leaves out what it hides - a universe without tags
 * shows everything at Standard and Expert.
 */
export function isLeftOut(id: string, level: 'essential' | 'standard' | 'expert', view: ViewLists | null | undefined): boolean {
  if (view && !view.all && VIEW_PLACES.has(placeOf(id) ?? '')) return !viewAllows(id, view)
  return level === 'essential' && ESSENTIAL_HIDDEN.has(id)
}

/** A view, as the GUI reads it (ktsp/universeView.ts): the element ids the
 *  universe's tags allow at the person's level. */
export interface ViewLists {
  all: boolean
  allowed: string[]
}

/** The places a tag lists - an object's sections and actions included;
 * an element of another place (a badge, a screen) is not the
 *  view's to hide. */
const VIEW_PLACES = new Set(['rail', 'home', 'quick', 'new', 'project', 'tab', 'section', 'action'])

/** Whether the universe's resolved view allows an element: no
 *  view or a universe without tags allows everything; otherwise the element
 *  is one of the ids it allows. The elements never hidden always show. */
export function viewAllows(id: string, view: ViewLists | null | undefined): boolean {
  if (!view || view.all) return true
  if (!VIEW_PLACES.has(placeOf(id) ?? '') || NEVER_HIDDEN.has(id)) return true
  return view.allowed.includes(id)
}

/** The elements never left out, at any level, by a tag's view: Home's own
 *  actions, « Vérifier l'installation » and an object's
 *  « Actions » tab. */
export const NEVER_HIDDEN: ReadonlySet<string> = new Set(['home:actions', CHECK_INSTALL_ELEMENT, ACTIONS_TAB_ELEMENT])

/** An item's place among the priority tags: the earliest it carries, else none. */
function priorityRank(hashtags: string[] | undefined, order: Map<string, number>): number {
  let rank = Infinity
  for (const h of hashtags ?? []) rank = Math.min(rank, order.get(h) ?? Infinity)
  return rank
}

/** The items carrying a `first` hashtag, in the tags' order (an item under the
 *  earliest it carries), then the others; each part keeps its order. */
export function firstByHashtags<T extends { hashtags?: string[] }>(items: T[], first: string[] | null | undefined): T[] {
  const order = new Map((first ?? []).map((h, i) => [h, i] as const))
  if (order.size === 0) return items
  return items
    .map((it, i) => ({ it, i, rank: priorityRank(it.hashtags, order) }))
    .sort((a, b) => (a.rank === b.rank ? a.i - b.i : a.rank < b.rank ? -1 : 1))
    .map((x) => x.it)
}

/** What a task list needs to know of a task to place it at Essentiel. */
export interface FirstLevelTask {
  hashtags?: string[]
  /** Whom the task is for: end-user | technical | demo; absent: undeclared. */
  audience?: string
  kind?: string
  types?: string[]
}

/**
 * A task dedicated to an object type: kind object or collection on types
 * other than any (`*`). It runs from that object's pane, not a first list.
 */
export function objectDedicated(it: FirstLevelTask): boolean {
  if (it.kind !== 'object' && it.kind !== 'collection') return false
  const types = it.types ?? []
  return !(types.length === 1 && types[0] === '*')
}

/**
 * At Essentiel, when the universe has priority tags (a tagged universe), a
 * task list shows only its end-user actions - the tasks declaring audience
 * end-user that are not dedicated to an object type -
 * until the person asks for every task; `more` counts the tasks
 * left out, and firstByHashtags orders what shows by the priority tags. At
 * Standard and Expert, or without priority tags, every task shows.
 */
export function priorityOnly<T extends FirstLevelTask>(
  items: T[], first: string[] | null | undefined, level: string, all: boolean,
): { items: T[]; more: number } {
  const tags = new Set(first ?? [])
  if (all || level !== 'essential' || tags.size === 0) return { items, more: 0 }
  const shown = items.filter((it) => it.audience === 'end-user' && !objectDedicated(it))
  return { items: shown, more: items.length - shown.length }
}

/**
 * The task gallery's first level: `listable` is what the gallery would list
 * (its node scope and its own exclusions applied), so `more` counts what
 * « Voir toutes les tâches » would add. A node's own list (`scoped`) holds the
 * tasks of that object's type - the very ones a first level leaves out - and
 * is never narrowed.
 */
export function galleryFirstLevel<T extends FirstLevelTask>(
  listable: T[], scoped: boolean, first: string[] | null | undefined, level: string, all: boolean,
): { items: T[]; more: number } {
  if (scoped) return { items: listable, more: 0 }
  return priorityOnly(listable, first, level, all)
}


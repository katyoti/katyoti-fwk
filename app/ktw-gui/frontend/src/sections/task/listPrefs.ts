// SPDX-License-Identifier: Apache-2.0
/**
 * What the person chose for the task and skill lists, kept per universe in the
 * GUI settings: the tasks starred into Home's quick actions, and the order of
 * each list. Pure: the lists and Home read it through these functions.
 */

import { taskQuickId, taskURIOf } from '../../elements'

/** The tag a task declares in its TASK.md to be a quick action. */
export const QUICK_ACTION_TAG = 'quick-action'

export type SortKey = 'name' | 'category' | 'source'
export const SORT_KEYS: SortKey[] = ['name', 'category', 'source']

export interface QuickTask {
  uri?: string
  name: string
  tags?: string[]
}

const keyOf = (t: QuickTask) => t.uri || t.name

/** Whether the task declares itself a quick action (its star cannot be taken off). */
export function isTagged(t: QuickTask): boolean {
  return (t.tags ?? []).includes(QUICK_ACTION_TAG)
}

/** A task's id among Home's quick actions: its element id
 *  `quick:task/<provider>/<task>` (a built-in's is `quick:<name>`). */
export function quickId(t: QuickTask): string {
  return taskQuickId(keyOf(t))
}

/** The ids a quick action was removed under before it had its element id:
 * a task's `task:<uri>`, a built-in's card id. */
const OLD_BUILT_IN: Record<string, string> = {
  'quick:term': 'term', 'quick:drawio': 'drawio', 'quick:debug': 'debug-and-tests', 'quick:start-chat': 'start-chat',
}

/** Whether the person removed a quick action - by its element id, or by the
 *  id it had when they removed it (a task's `task:<uri>`, a built-in's card id). */
export function isRemoved(removed: string[], id: string): boolean {
  if (removed.includes(id)) return true
  if (id.startsWith('quick:task/')) return removed.includes(`task:${taskURIOf(id)}`)
  return OLD_BUILT_IN[id] !== undefined && removed.includes(OLD_BUILT_IN[id])
}

/**
 * Whether a task is one of the universe's default quick actions: one its view
 * lists (`defaults`, when the universe's tags give some, by URI or quick id),
 * else one tagged quick-action.
 */
export function defaultOf(defaults?: string[]): (t: QuickTask) => boolean {
  if (!defaults) return isTagged
  const d = new Set(defaults)
  return (t) => d.has(keyOf(t)) || d.has(quickId(t)) || d.has(`task:${keyOf(t)}`)
}

/**
 * The tasks Home shows as quick actions: the universe's defaults, then those
 * the person starred in this universe, each once, in the list's order, less
 * those the person removed (by quick id).
 */
export function quickTasks<T extends QuickTask>(tasks: T[], starred: string[] = [], removed: string[] = [], defaults?: string[]): T[] {
  const star = new Set(starred)
  const isDefault = defaultOf(defaults)
  return [...tasks.filter(isDefault), ...tasks.filter((t) => !isDefault(t) && star.has(keyOf(t)))]
    .filter((t) => !isRemoved(removed, quickId(t)))
}

/** The starred set after a click on a task's star. */
export function toggleStar(starred: string[] = [], uri: string): string[] {
  return starred.includes(uri) ? starred.filter((u) => u !== uri) : [...starred, uri]
}

/** What the person keeps per universe for Home's quick actions. */
export interface QuickChoices {
  starred: string[]
  removed: string[]
}

/** After a click on a task's star: starring adds it, and brings it back if
 *  the person had removed it; unstarring takes it off. */
export function starTask({ starred, removed }: QuickChoices, uri: string): QuickChoices {
  const on = !starred.includes(uri)
  const back = [taskQuickId(uri), `task:${uri}`]
  return { starred: toggleStar(starred, uri), removed: on ? removed.filter((r) => !back.includes(r)) : removed }
}

/** After a ✕ on a quick action (its element id): it
 *  stays removed for the person; a starred task is unstarred. A starred task
 *  is matched by its element id, so whatever shape its URI has (the GUI lists
 *  `<provider>:<name>`) the bar and its star agree at once. */
export function removeQuick({ starred, removed }: QuickChoices, id: string): QuickChoices {
  const qid = id.startsWith('task:') ? taskQuickId(id.slice('task:'.length)) : id
  return {
    starred: qid.startsWith('quick:task/') ? starred.filter((u) => taskQuickId(u) !== qid) : starred,
    removed: removed.includes(id) ? removed : [...removed, id],
  }
}

export interface Sortable {
  title?: string
  name?: string
  category?: string
  source?: string
}

/** The list in the order picked; ties and missing values fall back on the name. */
export function sortItems<T extends Sortable>(items: T[], key: SortKey | string | undefined): T[] {
  const name = (x: T) => (x.title || x.name || '').toLowerCase()
  const field = (x: T) => (key === 'category' ? x.category : key === 'source' ? x.source : undefined) ?? '\uffff'
  return [...items].sort((a, b) =>
    (key === 'category' || key === 'source' ? field(a).toLowerCase().localeCompare(field(b).toLowerCase()) : 0) ||
    name(a).localeCompare(name(b)))
}

// SPDX-License-Identifier: Apache-2.0
// A GROUPED LIST - any page's list grouped by a field of its
// elements: a header with a count per group (and how many are overdue) whose
// click shows only that group and a second click shows all, the elements
// under headings - their group's, or those of a second field (`sectionBy`:
// counted by kind, drawn by holder) -, and a bulk selection - within one
// group - that runs an action on each element ticked where it applies, one
// run each. The pure part: what the renderer (nodes/grouped-list.tsx) draws
// and sends.

import type { UIListGroup, UIListNode, UIPageAction, UIPageActionEvent } from './ui-page.interactive.ts'
import { actionDisabled, actionShown } from './nodes/interactive.tsx'

/** The header's filter showing only the overdue elements. */
export const OVERDUE_FILTER = ':overdue'

/** One element of a grouped list: the element, its position in the list's
 *  data, its key (itemKey) and its group - the value of its `groupBy`
 *  field, what the header counts and a bulk selection keeps to. */
export interface GroupedItem {
  el: unknown
  index: number
  key: string
  group: string
}

/** One group: its key, label and elements, in the list's order. */
export interface ItemGroup {
  key: string
  label: string
  items: GroupedItem[]
}

/** One count of the header: a group's, or the overdue one (OVERDUE_FILTER). */
export interface GroupCount {
  key: string
  label: string
  count: number
}

function field(el: unknown, name: string): unknown {
  return el && typeof el === 'object' ? (el as Record<string, unknown>)[name] : undefined
}

/** itemKey is an element's key: « id: » and its `id`, else « at: » and its
 *  position - an element without an id never shares a key with one whose id
 *  reads like a position. */
export function itemKey(el: unknown, index: number): string {
  const id = field(el, 'id')
  return id === undefined || id === null || id === '' ? `at:${index}` : `id:${String(id)}`
}

function valueOf(el: unknown, name: string): string {
  const v = field(el, name)
  return v === undefined || v === null ? '' : String(v)
}

/** groupItems groups the elements by their `groupBy` field: the groups
 *  named first, in their order and with their labels, then each other value
 *  in the order first seen, labelled by itself. An element without the field
 *  is in the group « » (labelled « - »). A named group with no element is
 *  kept: its count is 0. */
export function groupItems(items: unknown[], groupBy: string, named?: UIListGroup[]): ItemGroup[] {
  const groups: ItemGroup[] = []
  const at = new Map<string, ItemGroup>()
  const add = (key: string, label?: string): ItemGroup => {
    let g = at.get(key)
    if (!g) {
      g = { key, label: label || key || '-', items: [] }
      at.set(key, g)
      groups.push(g)
    }
    return g
  }
  for (const g of named ?? []) add(String(g.key), g.label)
  items.forEach((el, index) => {
    const group = valueOf(el, groupBy)
    add(group).items.push({ el, index, key: itemKey(el, index), group })
  })
  return groups
}

/** isOverdue: the element's overdue field is set (true, or any value but
 *  false, 0 and empty). */
export function isOverdue(el: unknown, overdueField?: string): boolean {
  if (!overdueField) return false
  const v = field(el, overdueField)
  return v !== undefined && v !== null && v !== '' && v !== false && v !== 0
}

/** overdueLabel is the overdue count's label in the person's language. */
export function overdueLabel(language: string | undefined, own?: string): string {
  if (own) return own
  return (language ?? '').startsWith('fr') ? 'en retard' : 'overdue'
}

/** groupCounts is the header: each group's count, then - when the list names
 *  an overdue field and some element is overdue - the overdue count. */
export function groupCounts(groups: ItemGroup[], node: Pick<UIListNode, 'overdueField' | 'overdueLabel'>, language?: string): GroupCount[] {
  const out: GroupCount[] = groups.map((g) => ({ key: g.key, label: g.label, count: g.items.length }))
  const late = groups.reduce((n, g) => n + g.items.filter((it) => isOverdue(it.el, node.overdueField)).length, 0)
  if (late > 0) out.push({ key: OVERDUE_FILTER, label: overdueLabel(language, node.overdueLabel), count: late })
  return out
}

/** countLook is how a count of the header is drawn: every count alike -
 *  the overdue one too, never a red button -, the one filtering filled. */
export function countLook(filter: string | null, key: string): { variant: 'filled' | 'default' } {
  return { variant: filter === key ? 'filled' : 'default' }
}

/** toggleFilter is the filter after a click on a count: that count's group,
 *  or - clicked again - none (everything shows). */
export function toggleFilter(current: string | null, clicked: string): string | null {
  return current === clicked ? null : clicked
}

/** keep: the element shows under the filter - every element with none, a
 *  group's elements, or the overdue ones. */
function keep(it: GroupedItem, filter: string | null, overdueField?: string): boolean {
  if (filter === null) return true
  if (filter === OVERDUE_FILTER) return isOverdue(it.el, overdueField)
  return it.group === filter
}

/** How a list's sections are drawn: by its groups (no `sectionBy`), or by a
 *  second field with its own order and labels. */
export type SectionSpec = Pick<UIListNode, 'overdueField' | 'sectionBy' | 'sectionGroups'>

/** visibleSections are the headings drawn and the elements under each, under
 *  the header's filter.
 *
 *  With `sectionBy`, the headings are that field's values - the named ones
 *  first, then each other value of the list as first seen - and every one is
 *  kept, under a filter too: a section with nothing left shows « Nothing of
 *  this kind. » (emptySectionNote). A heading naming `only` some groups is
 *  drawn with no filter or under one of theirs; under one of theirs, the
 *  headings naming none of them are not drawn.
 *
 *  Without, the headings are the groups: with no filter the groups holding an
 *  element; a group's filter that group alone; the overdue filter every
 *  group holding an element, with its overdue ones - an empty one kept for
 *  its note. */
export function visibleSections(groups: ItemGroup[], filter: string | null, spec: SectionSpec): ItemGroup[] {
  const by = spec.sectionBy
  if (by) {
    const all = groups.flatMap((g) => g.items).sort((a, b) => a.index - b.index)
    const sections = groupItems([], by, spec.sectionGroups)
    // A heading for some groups alone (`only`): drawn with no filter or under
    // one of theirs - and then the only kind of heading drawn.
    const only = new Map((spec.sectionGroups ?? []).filter((g) => g.only?.length).map((g) => [String(g.key), g.only as string[]]))
    const apart = filter !== null && [...only.values()].some((o) => o.includes(filter))
    const drawn = (key: string): boolean => {
      const o = only.get(key)
      if (!o) return !apart
      return filter === null || o.includes(filter)
    }
    const at = new Map(sections.map((s) => [s.key, s]))
    for (const it of all) {
      const key = valueOf(it.el, by)
      let s = at.get(key)
      if (!s) {
        s = { key, label: key || '-', items: [] }
        at.set(key, s)
        sections.push(s)
      }
      if (keep(it, filter, spec.overdueField)) s.items.push(it)
    }
    return sections.filter((s) => drawn(s.key))
  }
  if (filter === null) return groups.filter((g) => g.items.length > 0)
  if (filter === OVERDUE_FILTER) {
    return groups
      .filter((g) => g.items.length > 0)
      .map((g) => ({ ...g, items: g.items.filter((it) => keep(it, filter, spec.overdueField)) }))
  }
  return groups.filter((g) => g.key === filter)
}

/** emptySectionNote is what a heading with nothing under it shows, in the
 *  person's language. */
export function emptySectionNote(language: string | undefined): string {
  return (language ?? '').startsWith('fr') ? 'Rien de ce genre.' : 'Nothing of this kind.'
}

/** sectionTick: the group a heading's tick selects in - its own group when
 *  the headings are the groups, else, with `sectionBy`, the group the
 *  header's filter shows (a heading mixing kinds has no tick: a bulk answer
 *  is for one kind). */
export function sectionTick(section: ItemGroup, filter: string | null, spec: SectionSpec): string | null {
  if (!spec.sectionBy) return section.key
  return filter !== null && filter !== OVERDUE_FILTER ? filter : null
}

/** A bulk selection: the elements ticked, all in one group. */
export interface BulkSelection {
  group: string | null
  keys: string[]
}

export const NO_SELECTION: BulkSelection = { group: null, keys: [] }

/** toggleSelected ticks or unticks an element of a group: in the group
 *  already selected it is added or removed; in another group it starts a new
 *  selection there - a bulk answer is for the elements of one group. */
export function toggleSelected(sel: BulkSelection, group: string, key: string): BulkSelection {
  if (sel.group !== group) return { group, keys: [key] }
  const keys = sel.keys.includes(key) ? sel.keys.filter((k) => k !== key) : [...sel.keys, key]
  return keys.length === 0 ? NO_SELECTION : { group, keys }
}

/** selectGroup ticks every element shown of a group (`items`, those under a
 *  heading - only the group's are taken), or unticks them all when they all
 *  are. */
export function selectGroup(sel: BulkSelection, group: string, items: GroupedItem[]): BulkSelection {
  const keys = items.filter((it) => it.group === group).map((it) => it.key)
  const all = sel.group === group && keys.length > 0 && keys.every((k) => sel.keys.includes(k))
  return all || keys.length === 0 ? NO_SELECTION : { group, keys }
}

/** selectedItems are the elements of a selection, in the list's order -
 *  only those still in their group (a refresh may have moved one). */
export function selectedItems(groups: ItemGroup[], sel: BulkSelection): unknown[] {
  const g = groups.find((x) => x.key === sel.group)
  if (!g) return []
  return g.items.filter((it) => sel.keys.includes(it.key)).map((it) => it.el)
}

/** bulkApplies: the action applies to the element as its own button would -
 *  shown (its `when` holds) and not disabled (its `disabledWhen` unset). */
export function bulkApplies(action: UIPageAction, item: unknown): boolean {
  return actionShown(action, item) && actionDisabled(action, item) === undefined
}

/** bulkEvents is what a bulk action sends the host: one run of the action
 *  per element it applies to, on that element - each answer separate -, and
 *  the elements skipped, where the action is hidden or disabled. */
export function bulkEvents(action: UIPageAction, items: unknown[], extra?: Record<string, unknown>): { events: UIPageActionEvent[]; skipped: unknown[] } {
  const events: UIPageActionEvent[] = []
  const skipped: unknown[] = []
  for (const item of items) {
    if (bulkApplies(action, item)) events.push({ action, item, ...(extra ? { extra } : {}) })
    else skipped.push(item)
  }
  return { events, skipped }
}

/** bulkSkipNote says how many elements ticked an action skips, in the
 *  person's language. */
export function bulkSkipNote(language: string | undefined, n: number): string {
  if ((language ?? '').startsWith('fr')) return n > 1 ? `${n} non concernés` : `${n} non concerné`
  return `${n} not applicable`
}

/** bulkLabel is the bulk bar's « n sélectionnés » in the person's language. */
export function bulkLabel(language: string | undefined, n: number): string {
  if ((language ?? '').startsWith('fr')) return n > 1 ? `${n} sélectionnés` : `${n} sélectionné`
  return `${n} selected`
}

/** filterNote says what the header's filter shows, in the person's language;
 *  hint is the list's own note while no count filters (`filterHint`). */
export function filterNote(language: string | undefined, label: string | null, hint?: string): string {
  const fr = (language ?? '').startsWith('fr')
  if (label === null) return hint || (fr ? 'Cliquez un compte pour ne voir que ce groupe.' : 'Click a count to show only that group.')
  return fr ? `Seulement « ${label} ». Cliquez-le encore pour tout voir.` : `Showing only « ${label} ». Click it again to see everything.`
}

/** One tile of a header drawn as tiles (filterStyle: tiles): a count with its
 *  group's icon, colour and hint - the overdue count red, with an alarm. */
export interface GroupTile extends GroupCount {
  icon?: string
  color?: string
  hint?: string
}

/** groupTiles gives each count of the header its group's look. */
export function groupTiles(counts: GroupCount[], named?: UIListGroup[]): GroupTile[] {
  const look = new Map((named ?? []).map((g) => [String(g.key), g]))
  return counts.map((c) => {
    if (c.key === OVERDUE_FILTER) return { ...c, icon: 'alarm', color: 'red' }
    const g = look.get(c.key)
    return { ...c, ...(g?.icon ? { icon: g.icon } : {}), ...(g?.color ? { color: g.color } : {}), ...(g?.hint ? { hint: g.hint } : {}) }
  })
}

/** sectionLook is a section's icon, colour and hint, as its group names them. */
export function sectionLook(key: string, named?: UIListGroup[]): Pick<UIListGroup, 'icon' | 'color' | 'hint'> {
  const g = (named ?? []).find((x) => String(x.key) === key)
  return { icon: g?.icon, color: g?.color, hint: g?.hint }
}

/** withPlaces puts before an element the label of its place (placeField)
 *  each time it differs from the element before - none without a field. */
export function withPlaces(items: GroupedItem[], field: string | undefined, places?: UIListGroup[]): Array<{ place?: string; item: GroupedItem }> {
  if (!field) return items.map((item) => ({ item }))
  const label = new Map((places ?? []).map((p) => [String(p.key), p.label ?? String(p.key)]))
  let last: string | undefined
  return items.map((item) => {
    const raw = item.el && typeof item.el === 'object' ? (item.el as Record<string, unknown>)[field] : undefined
    const v = raw === undefined || raw === null ? '' : String(raw)
    if (v === last) return { item }
    last = v
    return v === '' ? { item } : { place: label.get(v) ?? v, item }
  })
}

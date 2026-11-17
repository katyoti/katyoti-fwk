// SPDX-License-Identifier: Apache-2.0
// What a navigation to an object's panel brings into view: the
// element it names, highlighted once the pane draws it - the names of a page's
// own focus (an element id, a row's name, a region's id) - and
// a declared panel's tab badge read from the object, in its tone.

import type { StripEntry } from './VerticalTabBody'

/** PaneFocus is the element a navigation asked for; `n` tells two asks of the
 *  same element apart, so asking again highlights it again. */
export interface PaneFocus { element: string; n: number }

/** paneFocusOf is the focus a navigation target carries, if it names one. */
export function paneFocusOf(target: { element?: string }, n: number): { focus?: PaneFocus } {
  const element = target.element?.trim()
  return element ? { focus: { element, n } } : {}
}

/** tabBadge is a strip entry's badge from a declared panel's: its label and
 *  its tone; nothing when the panel has none. */
export function tabBadge(badge: { label?: string; color?: string } | undefined): Pick<StripEntry, 'badge' | 'badgeColor'> {
  if (!badge?.label) return {}
  return { badge: badge.label, ...(badge.color ? { badgeColor: badge.color } : {}) }
}

/** countBadge is the badge a panel's count action lays on its tab:
 * the number in the answer's tone; nothing at 0, on an
 *  error or with no answer yet. */
export function countBadge(c: { count: number; color?: string } | undefined): { label: string; color?: string } | undefined {
  if (!c || !(c.count > 0)) return undefined
  return c.color ? { label: String(c.count), color: c.color } : { label: String(c.count) }
}

/** panelTabBadge is a declared panel's tab badge: the one read from the
 *  object when it has one (a stored field wins), else its count's. */
export function panelTabBadge(
  stored: { label?: string; color?: string } | undefined, counted: { count: number; color?: string } | undefined,
): Pick<StripEntry, 'badge' | 'badgeColor'> {
  return tabBadge(stored?.label ? stored : countBadge(counted))
}

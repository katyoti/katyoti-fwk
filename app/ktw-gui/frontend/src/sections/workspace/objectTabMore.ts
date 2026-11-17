// SPDX-License-Identifier: Apache-2.0
/**
 * The object strip's main and « More… » tabs.
 *
 * An aspect the object carries may list its main tabs (`pane.tabs`); the pane
 * then flags every other tab `more`. The strip shows the main tabs and ends
 * with « More… (n) »; the More tabs show too when Mode expert is on, when the
 * person opened them (`showMore`, kept on the tab), or when the active tab is
 * one of them — a remembered More tab is never hidden under the person.
 */

/** The strip entry that opens or closes the More tabs. */
export const MORE_TOGGLE_ID = '__more'

export interface StripSplit<T> {
  /** The tabs the strip shows, in the pane's order. */
  shown: T[]
  /** How many tabs are behind « More… ». */
  moreCount: number
  /** Whether the More tabs are shown. */
  revealed: boolean
  /** Whether the strip ends with the « More… » / « Fewer » entry: there are
   *  More tabs and Mode expert is off. */
  toggle: boolean
}

export function splitStrip<T extends { id: string; more?: boolean }>(
  tabs: T[],
  opts: { expert: boolean; showMore: boolean; activeId?: string },
): StripSplit<T> {
  const moreCount = tabs.filter((t) => t.more).length
  const activeIsMore = tabs.some((t) => t.more && t.id === opts.activeId)
  const revealed = moreCount > 0 && (opts.expert || opts.showMore || activeIsMore)
  return {
    shown: revealed ? tabs : tabs.filter((t) => !t.more),
    moreCount,
    revealed,
    toggle: moreCount > 0 && !opts.expert,
  }
}

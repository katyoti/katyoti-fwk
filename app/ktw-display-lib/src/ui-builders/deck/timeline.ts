// SPDX-License-Identifier: Apache-2.0
// The timeline layout's steps (modules/slidedeck/): each item is written
// `**Step** — line`, as the deck-author skill and the layout's example say.
// The step's name is shown on its own line above the text, so the separator
// between them is the layout's to drop, not text to show.

/** A list item that opens with a bold step name, then a separator: em or en
 *  dash, hyphen or colon. */
const STEP = /^(\s*(?:\d+[.)]|[-*+])\s+(?:\*\*[^*\n]+\*\*|__[^_\n]+__))[ \t]*[—–:-][ \t]*/gm

/** The steps' markdown with each item's separator after its bold name
 *  dropped; every other byte kept. */
export function timelineSteps(md: string): string {
  return md.replace(STEP, '$1 ')
}

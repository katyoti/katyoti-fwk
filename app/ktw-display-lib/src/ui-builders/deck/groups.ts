// SPDX-License-Identifier: Apache-2.0
// Slide groups (modules/slidedeck/, th22): a slide whose only setting is
// `include=<group>` shows the group's slides in its place. The host hands the
// view the groups the deck includes (deckapi.GroupsFor), as it hands the theme;
// expandGroups is the one place they are put in. The Go lint checks includes
// (lintInclude); the parser stays the grammar's (parse.ts = deckapi.Parse).
import { parseDeck, type Slide } from './parse.ts'
import type { Deck } from './generated/deck-model.ts'

/** The group a slide includes, or ''. */
export const includeOf = (s: Slide): string => (s.settings?.include ?? '').trim()

/** The group names a deck includes, once each, in order. */
export function includesOf(deck: Deck): string[] {
  const out: string[] = []
  for (const s of deck.slides) {
    const n = includeOf(s)
    if (n && !out.includes(n)) out.push(n)
  }
  return out
}

/** A placeholder for a group the host does not have: the deck shows where it
 *  goes rather than dropping it. */
function missing(name: string, at: Slide): Slide {
  return {
    h: at.h, v: at.v, line: at.line, group: name,
    slots: [{ name: 'default', line: at.line, markdown: `## Slide group « ${name} »\n\nNot found in reach — the deck includes it here.` }],
  }
}

/** The deck with each include slide replaced by its group's slides, each
 *  marked `group` (they belong to the group, not to the deck: read-only here).
 *  A group's own includes are not followed. Slides are renumbered: a group's
 *  horizontal slides become the deck's, its vertical ones stay under them; a
 *  group included on a vertical slide stays vertical. */
export function expandGroups(deck: Deck, groups?: Record<string, string>): Deck {
  if (!deck.slides.some((s) => includeOf(s))) return deck
  const seq: { slide: Slide; vertical: boolean }[] = []
  for (const s of deck.slides) {
    const name = includeOf(s)
    const vertical = s.v > 0
    if (!name) { seq.push({ slide: s, vertical }); continue }
    const md = groups?.[name]
    const inner = md === undefined ? [] : parseDeck(md).slides.filter((g) => !includeOf(g))
    if (inner.length === 0) { seq.push({ slide: missing(name, s), vertical }); continue }
    inner.forEach((g, i) => seq.push({
      slide: { ...g, group: name },
      // The group's first slide takes the include's place; its others keep
      // their own shape, or all stay under a vertical include.
      vertical: i === 0 ? vertical : vertical || g.v > 0,
    }))
  }
  let h = -1
  let v = 0
  const slides = seq.map(({ slide, vertical }) => {
    if (vertical && h >= 0) v++
    else { h++; v = 0 }
    return { ...slide, h, v }
  })
  return { ...deck, slides }
}

/** The deck's markdown with each include slide's settings line replaced by
 *  the group's slides as written — what a host sends where the deck leaves
 *  the editor (the presenting window, the exports): they need no groups.
 *  A group it does not have stays an include line. deckapi.InlineGroups is
 *  the same rule. */
export function inlineGroups(markdown: string, groups?: Record<string, string>): string {
  if (!groups || Object.keys(groups).length === 0) return markdown
  const lines = markdown.replace(/\r\n/g, '\n').split('\n')
  const deck = parseDeck(markdown)
  // From the last slide up, so earlier line numbers stay right.
  for (const s of [...deck.slides].reverse()) {
    const name = includeOf(s)
    const md = name ? groups[name] : undefined
    if (md === undefined || !s.settingsLine) continue
    lines.splice(s.settingsLine - 1, 1, md.replace(/\r\n/g, '\n').trim())
  }
  return lines.join('\n')
}

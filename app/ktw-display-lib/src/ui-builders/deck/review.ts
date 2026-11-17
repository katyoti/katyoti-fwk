// SPDX-License-Identifier: Apache-2.0
// A deck's review (modules/slidedeck/, OD-017): comment threads on its slides
// and the reviews marked done, kept by the host beside the deck (a deck
// object's `review` named file). This file holds the rules — which slide a
// thread is about, what changed since the last review; review-panel.tsx shows
// them. Pure functions over the parsed deck, so a test drives them.
import type { DeckReview } from './generated/deck-review.ts'
import type { Slide } from './parse.ts'

export type { DeckReview }
export type ReviewThread = DeckReview['threads'][number]
export type ReviewComment = ReviewThread['comments'][number]

/** A review file's text, read leniently: anything that is not one is an
 *  empty review (a review never stops a deck from showing). */
export function parseReview(text?: string | null): DeckReview {
  if (!text) return { threads: [] }
  try {
    const v = JSON.parse(text) as Partial<DeckReview>
    return {
      threads: Array.isArray(v.threads) ? v.threads.filter((t) => t && Array.isArray(t.comments) && t.slide) : [],
      ...(Array.isArray(v.reviews) && v.reviews.length ? { reviews: v.reviews } : {}),
    }
  } catch {
    return { threads: [] }
  }
}

/** The slide's title: the text of its first heading, in any slot. */
export function slideTitle(s: Slide): string {
  for (const slot of s.slots) {
    const m = /^#{1,6}\s+(.+?)\s*#*\s*$/m.exec(slot.markdown)
    if (m) return m[1]!.trim()
  }
  return ''
}

/** A fingerprint of what the slide shows: its settings, slots and notes. */
export function slideHash(s: Slide): string {
  const text = JSON.stringify([s.settings ?? {}, s.slots.map((x) => [x.name, x.markdown.trim()]), (s.notes ?? '').trim()])
  // FNV-1a, 32 bits: enough to tell a changed slide from an unchanged one.
  let h = 0x811c9dc5
  for (let i = 0; i < text.length; i++) {
    h ^= text.charCodeAt(i)
    h = Math.imul(h, 0x01000193) >>> 0
  }
  return h.toString(16).padStart(8, '0')
}

/** The slide (its index in reading order) a thread is about: the slide with
 *  its title nearest where it was, else — for a slide that had no title —
 *  the same position; -1 when its slide is gone. */
export function threadSlide(t: ReviewThread, slides: Slide[]): number {
  const { index, title } = t.slide
  if (title) {
    let best = -1
    slides.forEach((s, i) => {
      if (slideTitle(s) === title && (best < 0 || Math.abs(i - index) < Math.abs(best - index))) best = i
    })
    return best
  }
  return index >= 0 && index < slides.length && !slideTitle(slides[index]!) ? index : -1
}

/** A new thread on slide `at`, its first comment `text`. */
export function addThread(r: DeckReview, slides: Slide[], at: number, author: string, text: string, now: string, id: string): DeckReview {
  const s = slides[at]
  const title = s ? slideTitle(s) : ''
  const thread: ReviewThread = { id, slide: { index: at, ...(title ? { title } : {}) }, comments: [{ author, text, at: now }] }
  return { ...r, threads: [...r.threads, thread] }
}

/** A reply on thread `id`; re-anchors it on the slide where it is now. */
export function reply(r: DeckReview, slides: Slide[], id: string, author: string, text: string, now: string): DeckReview {
  return {
    ...r,
    threads: r.threads.map((t) => {
      if (t.id !== id) return t
      const at = threadSlide(t, slides)
      return { ...t, ...(at >= 0 ? { slide: { ...t.slide, index: at } } : {}), comments: [...t.comments, { author, text, at: now }] }
    }),
  }
}

export function setResolved(r: DeckReview, id: string, resolved: boolean): DeckReview {
  return { ...r, threads: r.threads.map((t) => (t.id === id ? { ...t, resolved } : t)) }
}

/** The review marked done now: every slide's title and fingerprint. */
export function markReviewed(r: DeckReview, slides: Slide[], by: string, now: string): DeckReview {
  const mark = { at: now, ...(by ? { by } : {}), slides: slides.map((s) => {
    const title = slideTitle(s)
    return { ...(title ? { title } : {}), hash: slideHash(s) }
  }) }
  return { ...r, reviews: [...(r.reviews ?? []), mark] }
}

export interface SlideChange {
  kind: 'added' | 'changed' | 'removed'
  /** The slide's index now (added, changed) or then (removed). */
  index: number
  title: string
}

/** What changed since the last review marked done: slides matched by title
 *  (the nearest one when a title repeats), untitled slides by their order
 *  among the untitled. Null when no review was ever marked. */
export function changesSince(r: DeckReview, slides: Slide[]): SlideChange[] | null {
  const last = r.reviews?.[r.reviews.length - 1]
  if (!last) return null
  const used = new Set<number>()
  const out: SlideChange[] = []
  const untitledThen = last.slides.map((m, i) => (m.title ? -1 : i)).filter((i) => i >= 0)
  let untitledSeen = 0
  slides.forEach((s, i) => {
    const title = slideTitle(s)
    let match = -1
    if (title) {
      last.slides.forEach((m, j) => {
        if (!used.has(j) && m.title === title && (match < 0 || Math.abs(j - i) < Math.abs(match - i))) match = j
      })
    } else {
      const j = untitledThen[untitledSeen++]
      if (j !== undefined && !used.has(j)) match = j
    }
    if (match < 0) {
      out.push({ kind: 'added', index: i, title })
      return
    }
    used.add(match)
    if (last.slides[match]!.hash !== slideHash(s)) out.push({ kind: 'changed', index: i, title })
  })
  last.slides.forEach((m, j) => {
    if (!used.has(j)) out.push({ kind: 'removed', index: j, title: m.title ?? '' })
  })
  return out
}

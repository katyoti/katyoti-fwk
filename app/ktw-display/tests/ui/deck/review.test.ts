// SPDX-License-Identifier: Apache-2.0
// A deck's review (ktw-display-lib deck/review.ts, OD-017): a thread stays on
// its slide when slides move, and « changed since the last review » names what
// was added, changed and removed.

import { describe, expect, it } from 'bun:test'
import { parseDeck } from '../../../../ktw-display-lib/src/ui-builders/deck/parse.ts'
import {
  addThread, changesSince, markReviewed, parseReview, reply, setResolved, threadSlide,
} from '../../../../ktw-display-lib/src/ui-builders/deck/review.ts'

const deck = (body: string) => parseDeck(`---\ntitle: T\n---\n\n${body}`).slides
const NOW = '2026-10-04T10:00:00Z'

const v1 = deck('# Cover\n\n---\n\n## Context\n- a\n\n---\n\n## Risks\n- r1\n\n---\n\n![x](a.png)\n')

describe('a thread on a slide', () => {
  it('follows its slide by title when slides move', () => {
    let r = addThread(parseReview(''), v1, 2, 'Ann', 'Add a risk', NOW, 't1')
    expect(r.threads[0]!.slide).toEqual({ index: 2, title: 'Risks' })
    const moved = deck('# Cover\n\n---\n\n## Risks\n- r1\n\n---\n\n## Context\n- a\n')
    expect(threadSlide(r.threads[0]!, moved)).toBe(1)
    r = reply(r, moved, 't1', 'Bob', 'Done', NOW)
    expect(r.threads[0]!.comments.length).toBe(2)
    expect(r.threads[0]!.slide.index).toBe(1)
  })
  it('keeps an untitled slide by position, and loses a slide that is gone', () => {
    const r = addThread(parseReview(''), v1, 3, 'Ann', 'Picture too small', NOW, 't2')
    expect(threadSlide(r.threads[0]!, v1)).toBe(3)
    const gone = deck('# Cover\n\n---\n\n## Context\n- a\n')
    expect(threadSlide(addThread(parseReview(''), v1, 2, 'A', 'x', NOW, 't').threads[0]!, gone)).toBe(-1)
  })
  it('is resolved and reopened', () => {
    const r = setResolved(addThread(parseReview(''), v1, 1, 'A', 'x', NOW, 't'), 't', true)
    expect(r.threads[0]!.resolved).toBe(true)
    expect(setResolved(r, 't', false).threads[0]!.resolved).toBe(false)
  })
})

describe('changes since the last review', () => {
  it('is null before any review', () => {
    expect(changesSince(parseReview(''), v1)).toBeNull()
  })
  it('names added, changed and removed slides', () => {
    const r = markReviewed(parseReview(''), v1, 'Ann', NOW)
    expect(changesSince(r, v1)).toEqual([])
    const v2 = deck('# Cover\n\n---\n\n## Risks\n- r1\n- r2\n\n---\n\n## Costs\n- c\n\n---\n\n![x](a.png)\n')
    expect(changesSince(r, v2)).toEqual([
      { kind: 'changed', index: 1, title: 'Risks' },
      { kind: 'added', index: 2, title: 'Costs' },
      { kind: 'removed', index: 1, title: 'Context' },
    ])
  })
  it('counts from the last mark, and a notes change is a change', () => {
    let r = markReviewed(parseReview(''), v1, 'Ann', NOW)
    const v2 = deck('# Cover\n\n---\n\n## Context\n- a\n\nNote:\nsay it slowly\n\n---\n\n## Risks\n- r1\n\n---\n\n![x](a.png)\n')
    expect(changesSince(r, v2)).toEqual([{ kind: 'changed', index: 1, title: 'Context' }])
    r = markReviewed(r, v2, 'Bob', NOW)
    expect(changesSince(r, v2)).toEqual([])
  })
})

describe('the review file', () => {
  it('reads anything that is not a review as an empty one', () => {
    expect(parseReview('not json')).toEqual({ threads: [] })
    expect(parseReview('{"threads":[{"id":"x"}]}')).toEqual({ threads: [] })
  })
})

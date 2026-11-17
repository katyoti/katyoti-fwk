// SPDX-License-Identifier: Apache-2.0
// editDeck — one slide edited without resending the deck
// (app/ktw-display-lib/src/ui-builders/deck/edit.ts).
import { describe, expect, it } from 'bun:test'

import { editDeck } from '../../../../ktw-display-lib/src/ui-builders/deck/edit.ts'
import { parseDeck } from '../../../../ktw-display-lib/src/ui-builders/deck/parse.ts'

const DECK = `---
title: T
---

<!-- slide: layout=cover -->
# One

---

## Two

- a

Note:
two notes

--

## Two below

---

## Three
`

const titles = (md: string) =>
  parseDeck(md).slides.map((s) => (s.slots[0]?.markdown ?? '').split('\n')[0])

describe('editDeck', () => {
  it('replaces one slide and keeps every other byte', () => {
    const { markdown, errors } = editDeck(DECK, [{ op: 'replace', target: 'slide-2', value: '<!-- slide: steps=list -->\n## Two, edited\n\n- b' }])
    expect(errors).toEqual([])
    expect(titles(markdown)).toEqual(['# One', '## Two, edited', '## Two below', '## Three'])
    expect(parseDeck(markdown).slides[1].settings).toEqual({ steps: 'list' })
    expect(markdown.startsWith('---\ntitle: T\n---\n')).toBe(true)
    expect(parseDeck(markdown).slides[2].v).toBe(1) // the vertical slide stays vertical
  })
  it('counts vertical slides in document order', () => {
    const { markdown } = editDeck(DECK, [{ op: 'replace', target: 'slide-3', value: '## Below, edited' }])
    expect(titles(markdown)).toEqual(['# One', '## Two', '## Below, edited', '## Three'])
    expect(parseDeck(markdown).slides[2].v).toBe(1)
  })
  it('adds a slide before another, and appends one', () => {
    const { markdown, errors } = editDeck(DECK, [
      { op: 'add', target: 'slide-4', value: '## Before three' },
      { op: 'append', value: '## Questions?' },
    ])
    expect(errors).toEqual([])
    expect(titles(markdown)).toEqual(['# One', '## Two', '## Two below', '## Before three', '## Three', '## Questions?'])
  })
  it('adds before the first slide', () => {
    expect(titles(editDeck(DECK, [{ op: 'add', target: 'slide-1', value: '# Zero' }]).markdown)[0]).toBe('# Zero')
  })
  it('removes a slide, the first one included', () => {
    expect(titles(editDeck(DECK, [{ op: 'remove', target: 'slide-4' }]).markdown)).toEqual(['# One', '## Two', '## Two below'])
    expect(titles(editDeck(DECK, [{ op: 'remove', target: 'slide-1' }]).markdown)).toEqual(['## Two', '## Two below', '## Three'])
  })
  it('applies ops in order', () => {
    const { markdown } = editDeck(DECK, [
      { op: 'remove', target: 'slide-2' },
      { op: 'replace', target: 'slide-2', value: '## Was below' },
    ])
    expect(titles(markdown)).toEqual(['# One', '## Was below', '## Three'])
  })
  it('reports what it cannot apply and applies the rest', () => {
    const { markdown, errors } = editDeck(DECK, [
      { op: 'replace', target: 'slide-9', value: 'x' },
      { op: 'replace', target: 'node-1', value: 'x' },
      { op: 'merge', target: 'slide-1', value: 'x' },
      { op: 'append', value: '## Last' },
    ])
    expect(errors.length).toBe(3)
    expect(titles(markdown).at(-1)).toBe('## Last')
  })
})

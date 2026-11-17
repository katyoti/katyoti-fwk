// SPDX-License-Identifier: Apache-2.0
// Changing one setting of a deck in its markdown (ktw-display-lib
// deck/settings.ts): the key edited changes, every other byte stays.

import { describe, expect, it } from 'bun:test'
import { insertSlideAfter, setDeckSetting, setSlideSetting, slideIndexAt } from '../../../../ktw-display-lib/src/ui-builders/deck/settings.ts'
import { parseDeck } from '../../../../ktw-display-lib/src/ui-builders/deck/parse.ts'

const deck = [
  '---',
  'title: Les présentations',
  'theme: katyoti',
  '---',
  '',
  '<!-- slide: layout=cover -->',
  '# Les présentations',
  '',
  '---',
  '',
  '## Sans réglage',
  '- un',
  '',
  '---',
  '',
  '<!-- slide: layout=icons variant=dense -->',
  '## Trois',
  'Note: à dire',
].join('\n')

describe('setDeckSetting', () => {
  it('replaces a key and keeps every other line', () => {
    const out = setDeckSetting(deck, 'theme', 'acme')
    expect(out).toBe(deck.replace('theme: katyoti', 'theme: acme'))
  })
  it('adds a key before the closing fence, quoting what YAML would misread', () => {
    const out = setDeckSetting(deck, 'aspect', '16:9')
    expect(out.split('\n').slice(0, 5)).toEqual(['---', 'title: Les présentations', 'theme: katyoti', 'aspect: "16:9"', '---'])
    expect(parseDeck(out).settings.aspect).toBe('16:9')
    expect(out.split('\n').slice(5).join('\n')).toBe(deck.split('\n').slice(4).join('\n'))
  })
  it('removes a key on an empty value', () => {
    expect(setDeckSetting(deck, 'theme', '')).toBe(deck.replace('theme: katyoti\n', ''))
  })
  it('removes a key with its indented lines', () => {
    const md = '---\ntitle: T\nauthor:\n  - a\n  - b\ntheme: x\n---\n\n# S'
    expect(setDeckSetting(md, 'author', 'Me')).toBe('---\ntitle: T\nauthor: Me\ntheme: x\n---\n\n# S')
  })
  it('creates a frontmatter when the deck has none', () => {
    expect(setDeckSetting('# S', 'theme', 'acme')).toBe('---\ntheme: acme\n---\n\n# S')
    expect(setDeckSetting('# S', 'theme', '')).toBe('# S')
  })
  it('leaves an open frontmatter (still being written) alone', () => {
    expect(setDeckSetting('---\ntitle: T', 'theme', 'acme')).toBe('---\ntitle: T')
  })
  it('keeps values that read back the same', () => {
    for (const v of ['true', '12', '#fff', 'a: b', 'Christophe S']) {
      expect(parseDeck(setDeckSetting(deck, 'author', v)).settings.author).toBe(v)
    }
  })
})

describe('setSlideSetting', () => {
  it('changes a key of a settings line in place', () => {
    const out = setSlideSetting(deck, 2, 'variant', 'wide')
    expect(out).toBe(deck.replace('variant=dense', 'variant=wide'))
  })
  it('adds a key at the end of the line, quoting a value with spaces', () => {
    const out = setSlideSetting(deck, 0, 'background', 'bg/a b.jpg')
    expect(out).toBe(deck.replace('<!-- slide: layout=cover -->', '<!-- slide: layout=cover background="bg/a b.jpg" -->'))
    expect(parseDeck(out).slides[0].settings).toEqual({ layout: 'cover', background: 'bg/a b.jpg' })
  })
  it('creates a settings line before the slide\'s first line', () => {
    const out = setSlideSetting(deck, 1, 'layout', 'statement')
    expect(out).toBe(deck.replace('\n## Sans réglage', '\n<!-- slide: layout=statement -->\n## Sans réglage'))
    expect(parseDeck(out).slides[1].settings).toEqual({ layout: 'statement' })
  })
  it('removes a key, and the line when no key is left', () => {
    const one = setSlideSetting(deck, 2, 'variant', '')
    expect(one).toBe(deck.replace(' variant=dense', ''))
    const none = setSlideSetting(deck, 0, 'layout', '')
    expect(none).toBe(deck.replace('<!-- slide: layout=cover -->\n', ''))
    expect(parseDeck(none).slides).toHaveLength(3)
  })
  it('keeps the notes and the other slides', () => {
    const out = setSlideSetting(deck, 2, 'transition', 'fade')
    const d = parseDeck(out)
    expect(d.slides[2].notes).toBe('à dire')
    expect(d.slides[2].settings).toEqual({ layout: 'icons', variant: 'dense', transition: 'fade' })
    expect(d.slides.map((s) => s.slots)).toEqual(parseDeck(deck).slides.map((s) => s.slots))
  })
  it('ignores a slide that does not exist', () => {
    expect(setSlideSetting(deck, 9, 'layout', 'blank')).toBe(deck)
  })
})

describe('slideIndexAt', () => {
  it('maps reveal.js indices to the slide', () => {
    expect(slideIndexAt(deck, 0, 0)).toBe(0)
    expect(slideIndexAt(deck, 2, 0)).toBe(2)
    expect(slideIndexAt(deck, 5, 0)).toBe(-1)
  })
})

describe('insertSlideAfter', () => {
  const inc = '<!-- slide: include=team -->'
  const md = '---\ntitle: T\n---\n\n## One\n\n--\n\n## One down\n\n---\n\n## Two\n'

  it('goes after the slide shown and its vertical slides, every other byte kept', () => {
    const out = insertSlideAfter(md, 0, inc)
    expect(out).toBe('---\ntitle: T\n---\n\n## One\n\n--\n\n## One down\n\n---\n\n' + inc + '\n\n---\n\n## Two\n')
    const d = parseDeck(out)
    expect(d.slides.map((s) => [s.h, s.v, s.settings?.include ?? s.slots[0]?.markdown])).toEqual([
      [0, 0, '## One'], [0, 1, '## One down'], [1, 0, 'team'], [2, 0, '## Two'],
    ])
    // From the vertical slide: the same place.
    expect(insertSlideAfter(md, 1, inc)).toBe(out)
  })

  it('goes at the end after the last slide, or when no slide is shown', () => {
    const end = '---\ntitle: T\n---\n\n## One\n\n--\n\n## One down\n\n---\n\n## Two\n\n---\n\n' + inc + '\n'
    expect(insertSlideAfter(md, 2, inc)).toBe(end)
    expect(insertSlideAfter(md, -1, inc)).toBe(end)
  })

  it('is the first slide of a deck that has none', () => {
    const out = insertSlideAfter('---\ntitle: T\n---\n', -1, inc)
    expect(out).toBe('---\ntitle: T\n---\n\n' + inc + '\n')
    expect(parseDeck(out).slides[0].settings?.include).toBe('team')
  })
})

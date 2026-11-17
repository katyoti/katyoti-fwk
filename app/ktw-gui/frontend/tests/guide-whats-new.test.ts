// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'fs'
import { fillSourced, lastKey, nature, onceId, startQueue, type RecipeEntry } from '../src/guide/recipes'

// What's new is a shown-once guide whose text is the change-log
// entries recorded since the person last looked - asked of its source with
// the key last seen, opened only when something is new, seen once per key.
const whatsNew: RecipeEntry = {
  uri: 'ktw-hub:/guide-recipe/whats-new', provider: 'ktw-hub', id: 'whats-new',
  recipe: { name: "What's new", source: 'ktw-agents:changes/whats-new', shown_once: 2 },
} as RecipeEntry
const welcome: RecipeEntry = {
  uri: 'ktw-hub:/guide-recipe/welcome', provider: 'ktw-hub', id: 'welcome',
  recipe: { name: 'Welcome', body: 'Hello.', shown_once: 1 },
} as RecipeEntry

describe('What\'s new', () => {
  test('is an Explain me whose text comes from its source', () => {
    expect(nature(whatsNew.recipe)).toBe('explain')
  })
  test('is asked what is new after the key last seen', async () => {
    const completed = new Set([`${whatsNew.uri}@ops.CH-9`, `${whatsNew.uri}@ops.CH-12`, 'other@x'])
    expect(lastKey(whatsNew.uri, completed)).toBe('ops.CH-12')
    const asked: string[] = []
    await fillSourced([whatsNew], completed, async (_s, after) => { asked.push(after); return null })
    expect(asked).toEqual(['ops.CH-12'])
  })
  test('opens with the new versions\' text, seen once for its key', async () => {
    const md = '## 2026.11 — Le dossier\n\n### En un coup d\'œil\n\n- …\n\n### Les changements\n\n- **A**\n\n## 2026.10 — …'
    const filled = await fillSourced([welcome, whatsNew], new Set(), async () => ({ markdown: md, key: 'ops.CH-20' }))
    const queue = startQueue(filled, new Set())
    expect(queue.map((e) => e.id)).toEqual(['welcome', 'whats-new'])
    expect(queue[1].recipe.body).toBe(md)
    expect(onceId(queue[1])).toBe(`${whatsNew.uri}@ops.CH-20`)
    // Seen for that key: not again, until a newer entry gives a new key.
    expect(startQueue(filled, new Set([onceId(queue[1]), welcome.uri]))).toEqual([])
  })
  test('does not open when nothing is new', async () => {
    const filled = await fillSourced([whatsNew], new Set(), async () => ({ markdown: '', key: '' }))
    expect(filled).toEqual([])
  })
  test('the GUI fills it at start, and the reader asks its source when opened from the list', () => {
    const src = (p: string) => readFileSync(new URL(p, import.meta.url), 'utf8')
    expect(src('../src/guide/GuideAtStart.tsx')).toContain('startQueue(await fillSourced(entries, s.completed, source), s.completed)')
    expect(src('../src/guide/GuideReader.tsx')).toContain("fetchSourced(universe)(source, '')")
  })
})

// On an installation the text is the installed version's letter.
// Its key ends in a hash, not a number, so what was
// seen is told by the completions themselves; the reader offers the whole
// document - the full change log below the letter.
describe("What's new on an installation", () => {
  const rc1 = 'letter:billing.quoi-de-neuf-fr@0a1b2c3d4e5f'
  const rc2 = 'letter:billing.quoi-de-neuf-fr@ff0099aa1122'
  const letter = (key: string) => async () => ({ markdown: '# Quoi de neuf\n\n- **Les études**', key, document: 'billing.quoi-de-neuf-fr' })
  test('opens on the letter, carrying the document to open whole', async () => {
    const filled = await fillSourced([whatsNew], new Set(), letter(rc1))
    expect(filled).toHaveLength(1)
    expect(filled[0].document).toBe('billing.quoi-de-neuf-fr')
    expect(onceId(filled[0])).toBe(`${whatsNew.uri}@${rc1}`)
  })
  test('a letter seen does not open again, whatever the order of its keys', async () => {
    // rc2 seen after rc1: lastKey may hand back rc1 (it compares trailing digits);
    // the source then answers rc2 again - seen, so it stays closed.
    const seen = new Set([`${whatsNew.uri}@${rc1}`, `${whatsNew.uri}@${rc2}`])
    expect(await fillSourced([whatsNew], seen, letter(rc2))).toEqual([])
    // The next version's letter, never seen, opens.
    expect(await fillSourced([whatsNew], new Set([`${whatsNew.uri}@${rc1}`]), letter(rc2))).toHaveLength(1)
  })
  test('the reader offers the full change log by its anchor', () => {
    const reader = readFileSync(new URL('../src/guide/GuideReader.tsx', import.meta.url), 'utf8')
    expect(reader).toContain('data-guide="reader.full-log"')
    expect(reader).toContain("openAddress(stepAddress(universe, `object/${document}`))")
  })
})

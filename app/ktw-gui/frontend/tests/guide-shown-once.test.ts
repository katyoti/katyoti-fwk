// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { onceId, startQueue, type RecipeEntry } from '../src/guide/recipes'

// A guide marked shown_once opens by itself at the person's next start and
// never again once seen; several queue in their declared order;
// a What's new keyed by its version opens once per version.
const entry = (id: string, recipe: Partial<RecipeEntry['recipe']>): RecipeEntry => ({
  uri: `ktw-hub:/guide-recipe/${id}`, provider: 'ktw-hub', id, universe: 'admin',
  recipe: { name: id, steps: [], ...recipe },
})
const welcome = entry('welcome', { body: 'Welcome to ArchiFactory.', shown_once: 1 })
const news = entry('whats-new', { body: 'What changed.', shown_once: 2, once_key: '1.006' })
const tour = entry('tour', { steps: [{ id: 'a', say: 'Look here.' }] })

describe('guides shown once at start', () => {
  test('at the first start, every shown-once guide opens, in its declared order', () => {
    expect(startQueue([news, tour, welcome], new Set()).map((e) => e.id)).toEqual(['welcome', 'whats-new'])
  })
  test('at the second start, what was seen does not open again', () => {
    const seen = new Set([onceId(welcome), onceId(news)])
    expect(startQueue([news, tour, welcome], seen)).toEqual([])
  })
  test("a new version's What's new opens once", () => {
    const seen = new Set([onceId(welcome), onceId(news)])
    const next = entry('whats-new', { body: 'What changed since.', shown_once: 2, once_key: '1.007' })
    expect(startQueue([next, welcome], seen).map((e) => e.recipe.once_key)).toEqual(['1.007'])
  })
})

describe('the shown-once guides open one after the other', () => {
  test('closing one opens the next; the last closed leaves the reader empty', async () => {
    const { useGuidePlayer } = await import('../src/guide/player')
    const a = entry('a', { body: 'First.', shown_once: 1 })
    const b = entry('b', { body: 'Second.', shown_once: 2 })
    useGuidePlayer.setState({ reading: null, onceQueue: [], onceCurrent: null })
    useGuidePlayer.getState().runOnce([a, b])
    expect(useGuidePlayer.getState().reading?.id).toBe('a')
    useGuidePlayer.getState().closeReading()
    expect(useGuidePlayer.getState().reading?.id).toBe('b')
    useGuidePlayer.getState().closeReading()
    expect(useGuidePlayer.getState().reading).toBeNull()
    expect(useGuidePlayer.getState().onceCurrent).toBeNull()
  })
})

// The start guides run once per universe shown in a start - the
// admin one first, then the universe opened, whose What's new is its own
// installed letter. Guides arriving while one plays follow it, each once.
describe("a universe opened while a start guide plays", () => {
  test('its guides follow the one playing, none twice', async () => {
    const { useGuidePlayer } = await import('../src/guide/player')
    const welcomeA = entry('welcome', { body: 'Welcome.', shown_once: 1 })
    const letter = entry('whats-new', { body: 'La lettre.', shown_once: 2 })
    useGuidePlayer.setState({ reading: null, onceQueue: [], onceCurrent: null })
    useGuidePlayer.getState().runOnce([welcomeA])
    useGuidePlayer.getState().runOnce([welcomeA, letter])
    expect(useGuidePlayer.getState().reading?.id).toBe('welcome')
    expect(useGuidePlayer.getState().onceQueue.map((e) => e.id)).toEqual(['whats-new'])
    useGuidePlayer.getState().closeReading()
    expect(useGuidePlayer.getState().reading?.id).toBe('whats-new')
  })
  test('the start runs once per universe shown, not once per start', async () => {
    const { readFileSync } = await import('fs')
    const src = readFileSync(new URL('../src/guide/GuideAtStart.tsx', import.meta.url), 'utf8')
    expect(src).toContain('ranFor.has(here)')
    expect(src).not.toContain('ranThisStart')
  })
})

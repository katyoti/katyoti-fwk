// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { isTour, nature, primaryAction, recipeText } from '../src/guide/recipes'
import type { Recipe } from '../src/generated/guide-recipe'

// A guide's nature follows from what it holds (modules/guide, OD-003): steps
// make a Help me, a video without steps a Show me, a markdown body alone an
// Explain me. The browser offers each its own first action.
const help: Recipe = { name: 'Create a universe', steps: [{ id: 'open', say: 'Open the picker.' }] }
const checked: Recipe = { name: 'Install', steps: [{ id: 'done', say: 'Done.', check: 'ktw-univers:checks/installed' }] }
const show: Recipe = { name: 'A tour on video', video: 'https://example.org/v.mp4', body: 'What the video shows.' }
const explain: Recipe = { name: 'What a software factory is', body: 'A software factory is the workshop that builds your applications.' }

describe('a guide is a Help me, a Show me or an Explain me', () => {
  test('its nature follows its content', () => {
    expect(nature(help)).toBe('help')
    expect(nature(checked)).toBe('help')
    expect(nature(show)).toBe('show')
    expect(nature(explain)).toBe('explain')
    expect(nature({ name: 'empty' })).toBe('')
  })
  test('each offers its own first action: start, watch or read', () => {
    expect(primaryAction(nature(help))).toBe('start')
    expect(primaryAction(nature(show))).toBe('watch')
    expect(primaryAction(nature(explain))).toBe('read')
  })
  test('a tour is a Help me with no check; an Explain me is never a tour', () => {
    expect(isTour(help)).toBe(true)
    expect(isTour(checked)).toBe(false)
    expect(isTour(explain)).toBe(false)
  })
  test('an Explain me renders its body, translated when the language has it', () => {
    const e = { recipe: { ...explain, steps: [] }, translations: { fr: { body: 'Une usine logicielle est l’atelier qui construit vos applications.' } } }
    expect(recipeText(e, 'fr').body).toContain('atelier')
    expect(recipeText(e, 'en').body).toContain('workshop')
  })
})

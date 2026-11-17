// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'fs'
import { FURTHER_STEP, furtherShown, furtherTarget, withFurther } from '../src/guide/further'
import type { LoadedRecipe } from '../src/guide/recipes'

// A recipe's `next` is shown as the player's last step
// « Pour aller plus loin » - one mechanism for every guide.
const recipe = (next?: string[]) => ({ name: 'Frame your study', steps: [{ id: 'open', say: 'x' }], ...(next ? { next } : {}) }) as unknown as LoadedRecipe

describe('« Pour aller plus loin »', () => {
  test('ends a guide that names what to read next, once', () => {
    const r = withFurther(recipe(['collect-study-sources']))
    expect(r.steps.map((s) => s.id)).toEqual(['open', FURTHER_STEP])
    expect(withFurther(r).steps.length).toBe(2)
  })
  test('adds nothing to a guide that names nothing', () => {
    expect(withFurther(recipe()).steps.map((s) => s.id)).toEqual(['open'])
    expect(withFurther(recipe([])).steps.length).toBe(1)
  })
  test('opens a guide by its id or URI, or a document by its address', () => {
    const cur = 'ktw-acme:/guide-recipe/frame-a-study'
    expect(furtherTarget('collect-study-sources', cur)).toEqual({ kind: 'guide', uri: 'ktw-acme:/guide-recipe/collect-study-sources' })
    expect(furtherTarget('ktw-hub:/guide-recipe/chat-about-an-object', cur)).toEqual({ kind: 'guide', uri: 'ktw-hub:/guide-recipe/chat-about-an-object' })
    expect(furtherTarget('object/acme.studio-interface', cur)).toEqual({ kind: 'address', address: 'object/acme.studio-interface' })
  })
  test('the player appends it and draws it as the list', () => {
    expect(readFileSync(new URL('../src/guide/player.ts', import.meta.url), 'utf8')).toContain('recipe: withFurther(entry.recipe)')
    const player = readFileSync(new URL('../src/guide/GuidePlayer.tsx', import.meta.url), 'utf8')
    expect(player).toContain('step.id === FURTHER_STEP')
    expect(player).toContain('<FurtherList uri={uri} recipe={recipe} onOpen={close} />')
  })
})

// An entry naming a guide this universe does not have (the
// new-study guide outside ArchiFactory) is left out.
describe('an entry the universe cannot load', () => {
  const uri = 'ktw-hub:/guide-recipe/use-katyoti'
  test('a guide shows once loaded, the others kept in order', () => {
    const next = ['answer-the-inbox', 'ktw-acme:/guide-recipe/create-a-study', 'whats-new']
    expect(furtherShown(next, { 'answer-the-inbox': 'Répondre', 'whats-new': 'Quoi de neuf' }, uri)).toEqual(['answer-the-inbox', 'whats-new'])
    expect(furtherShown(next, {}, uri)).toEqual([])
  })
  test('a document address shows at once', () => {
    expect(furtherShown(['object/guide.concepts', 'whats-new'], {}, uri)).toEqual(['object/guide.concepts'])
  })
  test('the player shows what it loaded', () => {
    const player = readFileSync(new URL('../src/guide/GuidePlayer.tsx', import.meta.url), 'utf8')
    expect(player).toContain('furtherShown(next, names, uri).map(')
  })
})

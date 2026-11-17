// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { recipeText, type RecipeEntry } from '../src/guide/recipes'

// A concept document is shown in the person's language - French
// and English first - and falls back to English when it has no translation.
const concept = (translations?: Record<string, unknown>): RecipeEntry =>
  ({ uri: 'ktw-hub:/guide-recipe/projects', provider: 'ktw-hub', id: 'projects',
     recipe: { name: 'Projects, repositories and universes', concept: true, body: '# Projects\nA project is a folder.' },
     translations }) as unknown as RecipeEntry

const fr = { fr: { name: 'Projets, dépôts et univers', description: 'x', body: '# Projets\nUn projet est un dossier.' } }

describe('a concept document in the person\'s language', () => {
  test('French for a French profile', () => {
    const tx = recipeText(concept(fr), 'fr')
    expect(tx.name).toBe('Projets, dépôts et univers')
    expect(tx.body).toStartWith('# Projets')
  })
  test('English for an English profile', () => {
    expect(recipeText(concept(fr), 'en').body).toStartWith('# Projects')
  })
  test('English when the person\'s language has no translation', () => {
    expect(recipeText(concept(fr), 'de').body).toStartWith('# Projects')
    expect(recipeText(concept(), 'fr').body).toStartWith('# Projects')
  })
  test('English for a text the translation leaves out', () => {
    const tx = recipeText(concept({ fr: { name: 'Projets, dépôts et univers' } }), 'fr')
    expect(tx.name).toBe('Projets, dépôts et univers')
    expect(tx.body).toStartWith('# Projects')
  })
})

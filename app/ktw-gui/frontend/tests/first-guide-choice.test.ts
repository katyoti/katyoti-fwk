// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'fs'
import { chosenFirstGuide, firstGuideChoices } from '../src/sections/univers/firstGuideChoices'

// The creation form offers the guides of the
// sidecars picked as the universe's first guide - none by default - and sends
// the one chosen to create-universe.
const guides = [
  { uri: 'ktw-acme:/guide-recipe/create-a-study', provider: 'ktw-acme', name: 'Create an as-is study' },
  { uri: 'ktw-acme:/guide-recipe/frame-a-study', provider: 'ktw-acme', name: 'Frame your study' },
  { uri: 'ktw-docs:/guide-recipe/make-a-deck', provider: 'ktw-docs', name: 'Make a deck' },
  { uri: 'ktw-acme:/guide-recipe/create-a-study', provider: 'ktw-acme', name: 'Create an as-is study' },
]

describe('the first guide of a new universe', () => {
  test('is chosen among the guides of the sidecars picked, each once, by name', () => {
    expect(firstGuideChoices(guides, ['ktw-acme']).map((c) => c.value)).toEqual([
      'ktw-acme:/guide-recipe/create-a-study', 'ktw-acme:/guide-recipe/frame-a-study',
    ])
    expect(firstGuideChoices(guides, [])).toEqual([])
  })
  test('none is sent unless one is chosen, and a sidecar unpicked takes its guide away', () => {
    const choices = firstGuideChoices(guides, ['ktw-acme'])
    expect(chosenFirstGuide(null, choices)).toBeUndefined()
    expect(chosenFirstGuide('ktw-acme:/guide-recipe/create-a-study', choices)).toBe('ktw-acme:/guide-recipe/create-a-study')
    expect(chosenFirstGuide('ktw-acme:/guide-recipe/create-a-study', firstGuideChoices(guides, ['ktw-docs']))).toBeUndefined()
  })
  test('the creation form shows the choice and sends it', () => {
    const form = readFileSync(new URL('../src/sections/univers/UniverseCreationModal.tsx', import.meta.url), 'utf8')
    expect(form).toContain("label={t('create.firstGuide')}")
    expect(form).toContain('firstGuide:  chosenFirstGuide(firstGuide, guideChoices),')
  })
})

// SPDX-License-Identifier: Apache-2.0
// A form's standard actions speak the GUI's language: « Enregistrer », « Annuler », « Réinitialiser » in fr
// through the library's `form` namespace (ui-core/action/i18n), English in en.

import { afterAll, describe, expect, test } from 'bun:test'
import i18next from 'i18next'

import { initI18n, makeUITexts } from '../src/ui-core/i18n/index.ts'
import { defaultFormActions } from '../src/ui-core/action/defaults.ts'

const labels = () => Object.fromEntries(defaultFormActions(makeUITexts(i18next, 'form')).map((a) => [a.id, typeof a.label === 'string' ? a.label : '']))

describe('a form standard actions', () => {
  afterAll(async () => { await i18next.changeLanguage('en') })

  test('read in French in fr and in English in en', async () => {
    await initI18n({ language: 'fr' })
    await i18next.changeLanguage('fr')
    const fr = labels()
    expect(fr.submit).toBe('Enregistrer')
    expect(fr.cancel).toBe('Annuler')
    expect(fr.reset).toBe('Réinitialiser')
    await i18next.changeLanguage('en')
    const en = labels()
    expect(en.submit).toBe('Save')
    expect(en.cancel).toBe('Cancel')
  })
})

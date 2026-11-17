// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { confirmTextsFor, type ConfirmText } from '../src/components/compositions/pageConfirm'
import en from '../src/locales/en/workspace.json'
import fr from '../src/locales/fr/workspace.json'

// A page action's confirmation dialog speaks the page's language on every
// button: what the action does not declare is an i18n key, translated in
// every language — no English text left for a French page.
const resolve = (texts: Record<string, unknown>, c: ConfirmText): string | undefined => {
  if ('text' in c) return c.text
  return c.key.split('.').reduce<unknown>((o, k) => (o as Record<string, unknown> | undefined)?.[k], texts) as string | undefined
}

describe("a page action's confirmation", () => {
  test('the action declares its label and its text: they are kept; cancel is translated', () => {
    const c = confirmTextsFor({ label: 'Publier', confirm: 'Publier la version ?' })
    expect(c.title).toEqual({ text: 'Publier' })
    expect(c.body).toEqual({ text: 'Publier la version ?' })
    expect(c.run).toEqual({ text: 'Publier' })
    expect(resolve(fr, c.cancel)).toBe('Annuler')
    expect(resolve(en, c.cancel)).toBe('Cancel')
  })
  test('what it does not declare is translated in every language', () => {
    const c = confirmTextsFor({})
    for (const part of [c.title, c.body, c.cancel, c.run]) {
      expect('key' in part).toBe(true)
      const inEn = resolve(en, part)
      const inFr = resolve(fr, part)
      expect(inEn).toBeTruthy()
      expect(inFr).toBeTruthy()
      expect(inFr).not.toBe(inEn)
    }
  })
})

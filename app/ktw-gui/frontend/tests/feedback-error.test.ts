// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { feedbackErrorText } from '../src/sections/feedback/feedbackError'

// The feedback bindings answer an error code; the form shows it in the GUI's
// language - the published build's `unavailable` included, never the code
// itself nor an English sentence from the backend.
const locale = (lang: string) =>
  JSON.parse(readFileSync(new URL(`../src/locales/${lang}/guide.json`, import.meta.url), 'utf8')).feedback as Record<string, string>

const tIn = (lang: string) => (key: string, opts?: Record<string, unknown>) => {
  const text = locale(lang)[key.replace(/^feedback\./, '')]
  if (text === undefined) return key
  return text.replace(/\{\{(\w+)\}\}/g, (_, k) => String(opts?.[k] ?? ''))
}

describe('feedback error codes', () => {
  for (const lang of ['fr', 'en']) {
    test(`the published build's « unavailable » is translated (${lang})`, () => {
      const t = tIn(lang)
      const text = feedbackErrorText(t, 'unavailable', 'unavailable')
      expect(text).toBe(locale(lang).unavailable)
      expect(text).not.toBe('unavailable')
      // a binding answering a bare string (FeedbackSaveConfig) passes it as the code
      expect(feedbackErrorText(t, undefined, 'unavailable')).toBe(text)
    })
  }

  test('the French text is not the English one', () => {
    expect(locale('fr').unavailable).not.toBe(locale('en').unavailable)
  })

  test('the folder codes are translated, other errors shown as written', () => {
    const t = tIn('fr')
    expect(feedbackErrorText(t, 'folder-unset', 'x')).toBe(locale('fr').folderUnset)
    expect(feedbackErrorText(t, 'folder-unreachable', 'disk gone')).toContain('disk gone')
    expect(feedbackErrorText(t, '', 'something broke')).toBe('something broke')
  })
})

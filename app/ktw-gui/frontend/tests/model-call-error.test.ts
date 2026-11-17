// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { errorTextWith } from '../src/i18n/errors'

// A run whose model call failed says why in its chat in the
// person's language with the next step: every message key the agent loop
// gives a failed call (agent.model*) is said in French and in English, and
// the French names « Vérifier l'installation › Clés » where a key is at stake.
const src = (rel: string) => readFileSync(new URL(rel, import.meta.url), 'utf8')
const goKeys = [...src('../../../../internal/extensions/agent/internal/runtime/model_error.go').matchAll(/"agent\.(model[A-Za-z]+)"/g)].map((m) => m[1])
const fr = JSON.parse(src('../src/locales/fr/errors.json')).agent as Record<string, string>
const en = JSON.parse(src('../src/locales/en/errors.json')).agent as Record<string, string>

describe('a failed model call, said in the chat', () => {
  test('every key the agent loop gives is said in French and in English, with the model and the reason', () => {
    expect(goKeys.length).toBeGreaterThanOrEqual(5)
    for (const k of goKeys) {
      for (const texts of [fr, en]) {
        expect(texts[k]).toContain('{{model}}')
        expect(texts[k]).toContain('{{reason}}')
      }
    }
    for (const k of ['modelKeyMissing', 'modelKeyRefused']) expect(fr[k]).toContain("Vérifier l'installation › Clés")
  })

  test('a system message with a key is said by it; without one, its text', () => {
    const t = ((key: string, o: Record<string, unknown>) => (key === 'agent.modelKeyMissing' ? `clé absente pour ${o.model}` : String(o.defaultValue))) as any
    expect(errorTextWith(t, 'key is not set', { key: 'agent.modelKeyMissing', params: { model: 'foundry-haiku' } })).toBe('clé absente pour foundry-haiku')
    expect(errorTextWith(t, 'plain error', null)).toBe('plain error')
  })
})

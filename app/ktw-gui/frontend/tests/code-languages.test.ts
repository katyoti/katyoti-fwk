// SPDX-License-Identifier: Apache-2.0
// Every language the text / code engine picks for a media type is registered
// in the trimmed highlight.js (src/lib/hljs.ts) — a missing one would silently
// show plain text.
import { expect, test } from 'bun:test'
import hljs from '../src/lib/hljs'
import { codeLanguage } from '../src/components/document/content'

test('code media types map to registered languages', () => {
  const types = [
    'application/json', 'application/yaml', 'application/xml', 'image/svg+xml', 'text/css', 'text/javascript',
    'text/x-go', 'text/x-python', 'text/x-typescript', 'text/x-tsx', 'text/x-jsx', 'text/x-sh', 'text/x-powershell',
    'text/x-rust', 'text/x-java', 'text/x-csharp', 'text/x-sql', 'text/x-toml', 'application/vnd.ktw.ui+json',
  ]
  for (const mt of types) {
    const lang = codeLanguage(mt)
    expect(lang, mt).not.toBe('')
    expect(hljs.getLanguage(lang), `${mt} → ${lang}`).toBeDefined()
  }
})

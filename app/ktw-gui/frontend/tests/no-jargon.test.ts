// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readdirSync, readFileSync } from 'node:fs'
import { join } from 'node:path'
import { ESSENTIAL_HIDDEN } from '../src/elements'

// No label a newcomer reads at Essentiel carries a technical word,
// in English or in French. The guard reads the locale files; a key belongs to
// a newcomer screen by its namespace, unless a surface hidden at Essentiel
// (ESSENTIAL_HIDDEN: the technical tabs and badges, the developer tools) owns
// it. Placeholders ({{type}}) are values, not words, and are not read. The
// plain words that replace these are in modules/gui/i18n.md.
const LOCALES = join(import.meta.dir, '..', 'src', 'locales')

/** The words a newcomer must not read, per language (plurals included). */
export const JARGON: Record<string, string[]> = {
  en: ['object', 'node', 'type', 'aspect', 'capability', 'capabilities', 'session', 'sidecar', 'KTSP', 'CWM', 'hub'],
  fr: ['objet', 'nœud', 'noeud', 'type', 'aspect', 'capacité', 'session', 'sidecar', 'KTSP', 'CWM', 'hub'],
}

/** The namespaces of the screens a newcomer sees at Essentiel. */
export const NEWCOMER_NAMESPACES = ['home', 'cards', 'chat', 'inbox', 'guide', 'profile', 'ready', 'errors', 'components', 'workspace']

/** Keys owned by a surface: a key under a prefix shows only where its surface does. */
export const KEY_SURFACES: [prefix: string, surface: string][] = [
  ['workspace:agentContext.', 'tab:agent-context'],
  ['workspace:lifecycle.', 'tab:lifecycle'],
  ['workspace:actionsPanel.', 'tab:actions'],
  ['workspace:launch.', 'tab:actions'],
  ['workspace:core.cap', 'badge:capability'],
  ['workspace:core.policiesHint', 'badge:capability'],
  ['workspace:objectTab.aspect', 'badge:aspect'],
  ['workspace:objectTab.addAspect', 'badge:aspect'],
  ['workspace:objectTab.chip.aspect', 'badge:aspect'],
  ['workspace:objectTab.capabilitiesBox', 'badge:capability'],
  ['workspace:core.kind', 'tab:links'],
  ['home:builtIn.debug.', 'quick:debug'],
  ['home:errors.', 'home:errors'],
  ['chat:panel.', 'rail:debug'],
  ['chat:transcript.', 'rail:debug'],
]

function flat(d: unknown, prefix: string, out: [string, string][]) {
  if (typeof d === 'string') out.push([prefix, d])
  else if (d && typeof d === 'object') for (const [k, v] of Object.entries(d)) flat(v, prefix.endsWith(':') ? prefix + k : `${prefix}.${k}`, out)
}

/** Every label of a language, as `namespace:key` and its text. */
function labels(lang: string): [string, string][] {
  const out: [string, string][] = []
  for (const f of readdirSync(join(LOCALES, lang)).filter((f) => f.endsWith('.json'))) {
    flat(JSON.parse(readFileSync(join(LOCALES, lang, f), 'utf8')), `${f.slice(0, -5)}:`, out)
  }
  return out
}

/** Whether a newcomer reads the key at Essentiel. */
export function seenAtEssentiel(key: string): boolean {
  if (!NEWCOMER_NAMESPACES.includes(key.split(':')[0])) return false
  const owner = KEY_SURFACES.find(([p]) => key.startsWith(p))
  return !owner || !ESSENTIAL_HIDDEN.has(owner[1])
}

/** The jargon words a text carries, placeholders left out. */
export function jargonIn(text: string, lang: string): string[] {
  const plain = text.replace(/\{\{[^}]*\}\}/g, ' ')
  return JARGON[lang].filter((w) => new RegExp(`(^|[^\\p{L}])${w}s?($|[^\\p{L}])`, 'iu').test(plain))
}

describe('no jargon at Essentiel', () => {
  for (const lang of Object.keys(JARGON)) {
    test(`in ${lang}`, () => {
      const found = labels(lang)
        .filter(([k]) => seenAtEssentiel(k))
        .flatMap(([k, v]) => jargonIn(v, lang).map((w) => `${k} (${w}): ${v}`))
      expect(found).toEqual([])
    })
  }
})

describe('the guard', () => {
  test('catches a planted word, plural and capitalised, and reads no placeholder', () => {
    expect(jargonIn('Open the Objects', 'en')).toEqual(['object'])
    expect(jargonIn('Choisissez un nœud', 'fr')).toEqual(['nœud'])
    expect(jargonIn('A {{type}} cannot go there', 'en')).toEqual([])
    expect(jargonIn('Typeface and hubris', 'en')).toEqual([])
  })
  test('reads the newcomer screens, not the surfaces hidden at Essentiel', () => {
    expect(seenAtEssentiel('home:actions.tasks')).toBe(true)
    expect(seenAtEssentiel('workspace:lifecycle.noWorkflow')).toBe(false)
    expect(seenAtEssentiel('runs:row.openSession')).toBe(false)
  })
})

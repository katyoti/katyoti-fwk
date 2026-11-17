// SPDX-License-Identifier: Apache-2.0
// The small marks of the page grammar: a monogram's letters
// and a value bound from the page's data by a path - a tab's badge, a
// stepper step's label.

import { resolveJSONPath } from '../../../utils/json-path.ts'

/** monogramLetters are the letters a monogram shows: the text itself when
 *  it is one or two characters, else the first letter of its first two
 *  words, or its first two letters for one word (« LeanIX » → « LE »,
 *  « Confluence Cloud » → « CC »). */
export function monogramLetters(text: string): string {
  const t = text.trim()
  if (t.length <= 2) return t.toUpperCase()
  const words = t.split(/[\s_-]+/).filter(Boolean)
  return (words.length > 1 ? words[0][0] + words[1][0] : t.slice(0, 2)).toUpperCase()
}

/** boundValue is the value at a path in the page's data as a text, or the
 *  fallback while it is unset. */
export function boundValue(data: unknown, path: string | undefined, fallback?: string | number): string | number | undefined {
  if (!path) return fallback
  const v = resolveJSONPath(data, path)
  if (v === undefined || v === null || v === '') return fallback
  return typeof v === 'number' ? v : String(v)
}

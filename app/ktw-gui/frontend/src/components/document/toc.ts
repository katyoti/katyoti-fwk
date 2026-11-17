// SPDX-License-Identifier: Apache-2.0
/**
 * toc — the table of contents of a markdown text: its
 * headings, outside fenced code, each with the line it is on and a unique id.
 * The renderer gives each heading the id of its line, so a click in the table
 * scrolls to it.
 */

export interface Heading {
  /** 1 to 4: `#` to `####`. */
  level: number
  text: string
  /** Unique within the text: the slug, then -2, -3 … on a repeat. */
  id: string
  /** 1-based line of the heading in the text. */
  line: number
}

/** The deepest heading listed and given an id. */
export const TOC_MAX_LEVEL = 4

/** A table of contents is shown from this many headings on. */
export const TOC_MIN_HEADINGS = 3

/** A heading's slug: lower case, accents kept as letters, words joined by -. */
export function slugify(text: string): string {
  const s = text
    .normalize('NFD').replace(/[̀-ͯ]/g, '')
    .toLowerCase()
    .replace(/[`*_~[\]()]/g, '')
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
  return s || 'section'
}

/** The headings of a markdown text, in order, fenced code skipped. */
export function headingsOf(md: string): Heading[] {
  const out: Heading[] = []
  const seen = new Map<string, number>()
  let fence = ''
  md.split('\n').forEach((raw, i) => {
    const line = raw.replace(/\r$/, '')
    const f = /^\s*(```|~~~)/.exec(line)
    if (f) {
      if (!fence) fence = f[1]
      else if (f[1] === fence) fence = ''
      return
    }
    if (fence) return
    const m = /^(#{1,6})\s+(.+?)\s*#*\s*$/.exec(line)
    if (!m || m[1].length > TOC_MAX_LEVEL) return
    const text = m[2].replace(/\[([^\]]*)\]\([^)]*\)/g, '$1').replace(/[`*_~]/g, '').trim()
    const base = slugify(text)
    const n = (seen.get(base) ?? 0) + 1
    seen.set(base, n)
    out.push({ level: m[1].length, text, id: n === 1 ? base : `${base}-${n}`, line: i + 1 })
  })
  return out
}

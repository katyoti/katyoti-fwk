// SPDX-License-Identifier: Apache-2.0
// Changing one setting of a deck in its markdown — what the deck settings
// panel does (modules/slidedeck/, the Deck component).
//
//   setDeckSetting(md, key, value)          a key of the frontmatter
//   setSlideSetting(md, slide, key, value)  a key of a slide's settings line
//   insertSlideAfter(md, slide, text)       a new slide after a slide's stack
//
// An empty value removes the key; a frontmatter or a settings line is created
// when the deck or the slide has none, and a settings line left without a key
// is removed. Every other byte of the markdown is kept: the agent and the
// author keep writing the same file.

import { expandGroups } from './groups.ts'
import { parseDeck, parseSettings } from './parse.ts'

/** The value as a YAML scalar: plain when it reads back as the same string,
 *  double-quoted otherwise (`16:9`, `true`, `12`, `#fff`…). */
function yamlScalar(v: string): string {
  const plain = /^[\p{L}\p{N}_][\p{L}\p{N}_ .\-/']*$/u.test(v) && !/[ ]$/.test(v) &&
    !/^(true|false|yes|no|on|off|null|~)$/i.test(v) && !/^[-+]?(\d|\.\d)/.test(v)
  return plain ? v : JSON.stringify(v)
}

/** Sets (or, with an empty value, removes) `key` in the deck's frontmatter. */
export function setDeckSetting(markdown: string, key: string, value: string | undefined): string {
  const md = markdown.replace(/\r\n/g, '\n')
  const lines = md.split('\n')
  const line = value ? `${key}: ${yamlScalar(value)}` : ''
  let end = -1
  if (lines[0]?.trimEnd() === '---') {
    for (let j = 1; j < lines.length; j++) if (lines[j].trimEnd() === '---') { end = j; break }
  }
  if (end < 0) {
    // No frontmatter (or an open one, still being written): add one.
    if (!value || lines[0]?.trimEnd() === '---') return md
    return ['---', line, '---', '', ...lines].join('\n')
  }
  const at = lines.findIndex((l, i) => i > 0 && i < end && new RegExp(`^${key}\\s*:`).test(l))
  if (at < 0) {
    if (!value) return md
    lines.splice(end, 0, line)
    return lines.join('\n')
  }
  // The key's own lines: its line and the indented ones under it.
  let to = at + 1
  while (to < end && /^\s+\S/.test(lines[to])) to++
  lines.splice(at, to - at, ...(value ? [line] : []))
  return lines.join('\n')
}

const settingsLineRe = /^(\s*)<!--\s*slide:(.*?)-->\s*$/

function pairText(key: string, value: string): string {
  return /^[^\s"]+$/.test(value) ? `${key}=${value}` : `${key}="${value.replace(/"/g, "'")}"`
}

/** Sets (or, with an empty value, removes) `key` on the settings line of the
 *  slide at `index` (from 0, in the order parseDeck gives). */
export function setSlideSetting(markdown: string, index: number, key: string, value: string | undefined): string {
  const md = markdown.replace(/\r\n/g, '\n')
  const slide = parseDeck(md).slides[index]
  if (!slide) return md
  const lines = md.split('\n')
  if (slide.settingsLine) {
    const i = slide.settingsLine - 1
    const m = settingsLineRe.exec(lines[i])
    if (!m) return md
    const { pairs, rest } = parseSettings(m[2])
    const keys = Object.keys(pairs)
    if (value) {
      pairs[key] = value
      if (!keys.includes(key)) keys.push(key)
    } else {
      delete pairs[key]
    }
    const kept = keys.filter((k) => k in pairs).map((k) => pairText(k, pairs[k]))
    if (kept.length === 0 && !rest) {
      // Nothing left: the line goes, and the blank line it opened.
      lines.splice(i, lines[i + 1]?.trim() === '' ? 2 : 1)
    } else {
      lines[i] = `${m[1]}<!-- slide: ${[...kept, ...(rest ? [rest] : [])].join(' ')} -->`
    }
    return lines.join('\n')
  }
  if (!value) return md
  // No settings line: one goes before the slide's first non-blank line.
  let i = slide.line - 1
  while (i < lines.length && lines[i].trim() === '') i++
  lines.splice(i, 0, `<!-- slide: ${pairText(key, value)} -->`)
  return lines.join('\n')
}

/** The index (from 0) of the slide shown at reveal.js's indices, or -1 —
 *  also for a slide of a slide group the deck shows with `groups` (it is
 *  changed in its group, not in this deck). */
export function slideIndexAt(markdown: string, h: number, v: number, groups?: Record<string, string>): number {
  const source = parseDeck(markdown)
  const shown = expandGroups(source, groups).slides.find((s) => s.h === h && s.v === v)
  if (!shown || shown.group) return -1
  return source.slides.findIndex((s) => s.line === shown.line)
}

/** Inserts a slide (its markdown, without separators) after the slide at
 *  `index` and the vertical slides under it — at the end when `index` is -1
 *  or past the last slide. Every other byte of the markdown is kept. */
export function insertSlideAfter(markdown: string, index: number, text: string): string {
  const md = markdown.replace(/\r\n/g, '\n')
  const slides = parseDeck(md).slides
  const body = text.trim()
  const at = slides[index]
  const next = at ? slides.find((s) => s.h > at.h) : undefined
  if (next) {
    // Before the separator that opens the next horizontal slide.
    const lines = md.split('\n')
    lines.splice(next.line - 2, 0, '---', '', body, '')
    return lines.join('\n')
  }
  const trimmed = md.replace(/\s*$/, '')
  return slides.length === 0 ? `${trimmed}\n\n${body}\n` : `${trimmed}\n\n---\n\n${body}\n`
}

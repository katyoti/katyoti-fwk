// SPDX-License-Identifier: Apache-2.0
// Editing one slide of a deck without resending the whole markdown — what
// `patch-ui-component` does to a deck artifact (modules/slidedeck/).
//
// A slide is addressed by its position in the document, from 1, the number the
// deck shows in its corner: `target: "slide-3"`. Vertical slides count in
// document order. The value of `replace`, `add` and `append` is the slide's own
// markdown — its settings line, slots and notes — without separators.
//
//   replace slide-N  the slide's markdown becomes `value`
//   add     slide-N  a new slide `value` is inserted before slide N
//   remove  slide-N  slide N is removed
//   append           a new slide `value` is added at the end (no target)
//
// Each op applies to the result of the previous one, so `remove slide-2` then
// `replace slide-2` edits what was slide 3. The markdown's frontmatter and
// every other slide are kept byte for byte.

import { parseDeck } from './parse.ts'

export interface DeckEditOp {
  op: string
  target?: string
  value?: unknown
}

export interface DeckEditResult {
  markdown: string
  /** One message per op that could not be applied; the others still are. */
  errors: string[]
}

/** The span of each slide in the source lines (0-based, end exclusive), and the
 *  index of the separator line that opens it (-1 for the first slide). */
function spans(lines: string[], md: string) {
  const slides = parseDeck(md).slides
  return slides.map((s, i) => {
    const start = s.line - 1
    const next = slides[i + 1]
    const end = next ? next.line - 2 : lines.length
    return { start, end, sep: i === 0 ? -1 : start - 1 }
  })
}

function slideLines(value: unknown): string[] | null {
  if (typeof value !== 'string') return null
  return ['', ...value.replace(/\r\n/g, '\n').replace(/^\n+|\n+$/g, '').split('\n'), '']
}

/** Applies the ops in order; see the file comment. */
export function editDeck(markdown: string, ops: DeckEditOp[]): DeckEditResult {
  let md = markdown.replace(/\r\n/g, '\n')
  const errors: string[] = []
  ops.forEach((o, k) => {
    const where = `op[${k}] ${o.op}${o.target ? ' ' + o.target : ''}`
    const lines = md.split('\n')
    const all = spans(lines, md)
    if (o.op === 'append') {
      const body = slideLines(o.value)
      if (!body) return errors.push(`${where}: value must be the new slide's markdown`)
      while (lines.length && lines[lines.length - 1].trim() === '') lines.pop()
      md = [...lines, '', '---', ...body].join('\n')
      return
    }
    const m = /^slide-(\d+)$/.exec(o.target ?? '')
    if (!m) return errors.push(`${where}: target must be slide-<n> (1 = the first slide)`)
    const n = Number(m[1])
    if (n < 1 || n > all.length) return errors.push(`${where}: the deck has ${all.length} slides`)
    const s = all[n - 1]
    switch (o.op) {
      case 'replace': {
        const body = slideLines(o.value)
        if (!body) return errors.push(`${where}: value must be the slide's markdown`)
        lines.splice(s.start, s.end - s.start, ...body)
        break
      }
      case 'add': {
        const body = slideLines(o.value)
        if (!body) return errors.push(`${where}: value must be the new slide's markdown`)
        if (s.sep >= 0) lines.splice(s.sep, 0, '---', ...body)
        else lines.splice(s.start, 0, ...body, '---')
        break
      }
      case 'remove': {
        if (all.length === 1) {
          lines.splice(s.start, s.end - s.start, '')
        } else if (s.sep >= 0) {
          lines.splice(s.sep, s.end - s.sep)
        } else {
          // The first slide goes with the separator that follows it.
          lines.splice(s.start, all[1].sep + 1 - s.start)
        }
        break
      }
      default:
        return errors.push(`${where}: a deck takes replace, add, remove or append`)
    }
    md = lines.join('\n')
  })
  return { markdown: md, errors }
}

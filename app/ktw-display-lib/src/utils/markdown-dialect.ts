// SPDX-License-Identifier: Apache-2.0
// markdown-dialect — the Markdown (Confluence) dialect a study page is
// written in, read before a GFM renderer draws it: admonitions
// (`!!! warning "Title"`, `??? note "Title"` collapsible closed, `???+` open)
// whose body is indented by four spaces, a `[[TOC]]` marker, and HTML
// comments (`<!-- acme-da:brouillon -->`) that are not prose, raw
// `<details><summary>…</summary>…</details>` blocks (a collapsible box) and
// Confluence status lozenges `![LABEL][status-green]` written with no
// reference definition (a coloured badge). Plain GFM reads
// an indented body as a code block and the markers as text; this splits the
// text into segments a renderer draws one by one - a box for an admonition,
// its body rendered as Markdown again (tables as tables). Fenced code is kept
// as written.

/** A run of plain Markdown. `line`: its first line in the text (1-based). */
export interface DialectText { type: 'text'; text: string; line: number }

/** An admonition: `!!!` a box, `???` a collapsible box (`???+` open at first).
 *  `title` empty when written `""`; the kind, capitalised, when absent.
 *  `body`: its lines dedented by four, starting on `line + 1`. */
export interface DialectAdmonition {
  type: 'admonition'
  kind: string
  title: string
  collapsible: boolean
  open: boolean
  body: string
  line: number
}

/** A `[[TOC]]` marker on a line of its own: the page's table of contents. */
export interface DialectToc { type: 'toc'; line: number }

export type DialectSegment = DialectText | DialectAdmonition | DialectToc

const LOZENGE = /!\[([^\]\n]*)\]\[status-(green|yellow|red|blue|purple|grey|gray)\]/g
const LOZENGE_SRC = /^#status-(green|yellow|red|blue|purple|grey|gray)$/
const DETAILS_OPEN = /^[ \t]*<details(\s+open(?:=["']?[^"'>]*["']?)?)?[ \t]*>(.*)$/i
const SUMMARY = /^[ \t]*<summary>(.*?)<\/summary>(.*)$/i
const ADMONITION = /^(!!!|\?\?\?\+?)[ \t]+([A-Za-z][\w-]*)(?:[ \t]+"([^"]*)")?[ \t]*$/
const TOC = /^[ \t]*\[\[_?TOC_?\]\][ \t]*$/i
const FENCE = /^[ \t]*(```|~~~)/

/** True when the text uses the dialect: anything for `splitDialect` to do. */
export function hasDialect(md: string): boolean {
  return /^(!!!|\?\?\?)/m.test(md) || /\[\[_?TOC_?\]\]/i.test(md) || md.includes('<!--') || /^[ \t]*<details/im.test(md)
}

/** The text with each status lozenge `![LABEL][status-<colour>]` (Confluence:
 *  no reference definition) written as an image of source `#status-<colour>`,
 *  outside fenced and inline code - a GFM renderer then hands it to its image
 *  drawer, which asks `lozengeColour`. Every line keeps its number. */
export function statusLozenges(md: string): string {
  if (!md.includes('][status-')) return md
  let fence = ''
  return md.split('\n').map((line) => {
    const f = FENCE.exec(line)
    if (f) {
      if (!fence) fence = f[1]
      else if (f[1] === fence) fence = ''
      return line
    }
    if (fence) return line
    return line.split(/(`+[^`]*`+)/).map((part, i) => (i % 2 ? part : part.replace(LOZENGE, '![$1](#status-$2)'))).join('')
  }).join('\n')
}

/** The colour of a status lozenge's image source (`#status-green` → `green`;
 *  `grey` and `gray` both `grey`); undefined for any other image. */
export function lozengeColour(src: string | undefined): string | undefined {
  const m = LOZENGE_SRC.exec(src ?? '')
  if (!m) return undefined
  return m[1] === 'gray' ? 'grey' : m[1]
}

/** The text of a `<summary>`: its tags dropped. */
function plainSummary(html: string): string {
  return html.replace(/<[^>]*>/g, '').trim()
}

/** The end of the `<details>` block opened on line `start`: the index of the
 *  line holding its `</details>` (nested blocks counted, fenced code skipped),
 *  -1 when it never closes. */
function detailsEnd(lines: string[], start: number): number {
  let depth = 0
  let fence = ''
  for (let i = start; i < lines.length; i++) {
    const f = FENCE.exec(lines[i])
    if (f) {
      if (!fence) fence = f[1]
      else if (f[1] === fence) fence = ''
      continue
    }
    if (fence) continue
    depth += (lines[i].match(/<details[\s>]/gi) ?? []).length
    depth -= (lines[i].match(/<\/details>/gi) ?? []).length
    if (depth <= 0) return i
  }
  return -1
}

/** The text with its HTML comments blanked - outside fenced code, the lines
 *  they covered kept (empty) so every line keeps its number. */
export function stripComments(md: string): string {
  const lines = md.split('\n')
  let fence = ''
  let inComment = false
  return lines.map((line) => {
    if (!inComment) {
      const f = FENCE.exec(line)
      if (f) {
        if (!fence) fence = f[1]
        else if (f[1] === fence) fence = ''
        return line
      }
      if (fence) return line
    }
    let out = ''
    let rest = line
    while (rest.length) {
      if (inComment) {
        const end = rest.indexOf('-->')
        if (end < 0) { rest = ''; break }
        rest = rest.slice(end + 3)
        inComment = false
      } else {
        const start = rest.indexOf('<!--')
        if (start < 0) { out += rest; break }
        out += rest.slice(0, start)
        rest = rest.slice(start + 4)
        inComment = true
      }
    }
    return out.trim() === '' && line.trim() !== '' ? '' : out
  }).join('\n')
}

function dedent(line: string): string {
  if (line.startsWith('\t')) return line.slice(1)
  return line.replace(/^ {1,4}/, '')
}

function isBodyLine(line: string): boolean {
  return line.trim() === '' || /^( {4}|\t)/.test(line)
}

function titleOf(kind: string, written: string | undefined): string {
  if (written !== undefined) return written
  return kind.charAt(0).toUpperCase() + kind.slice(1)
}

/** The segments of a text in the dialect, in order. A text without it is one
 *  text segment. An admonition's body may hold admonitions of its own: split
 *  it again to draw them. */
export function splitDialect(md: string): DialectSegment[] {
  const lines = stripComments(md.replace(/\r\n/g, '\n')).split('\n')
  const out: DialectSegment[] = []
  let run: string[] = []
  let runStart = 1
  let fence = ''
  const flush = (next: number) => {
    if (run.some((l) => l.trim() !== '')) out.push({ type: 'text', text: run.join('\n'), line: runStart })
    run = []
    runStart = next
  }
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]
    const f = FENCE.exec(line)
    if (f) {
      if (!fence) fence = f[1]
      else if (f[1] === fence) fence = ''
      run.push(line)
      continue
    }
    if (fence) { run.push(line); continue }
    if (TOC.test(line)) {
      flush(i + 2)
      out.push({ type: 'toc', line: i + 1 })
      continue
    }
    const d = DETAILS_OPEN.exec(line)
    const dEnd = d ? detailsEnd(lines, i) : -1
    if (d && dEnd >= i) {
      // A raw <details> block: a collapsible box, its <summary> the title;
      // the body keeps its lines (the tags blanked) so each keeps its number.
      flush(i + 1)
      const closing = /<\/details>\s*$/i
      const body = lines.slice(i + 1, dEnd + 1)
      if (body.length) body[body.length - 1] = body[body.length - 1].replace(closing, '')
      let title = ''
      let rest = dEnd === i ? d[2].replace(closing, '') : d[2]
      if (!SUMMARY.test(rest)) {
        const k = body.findIndex((l) => l.trim() !== '')
        if (k >= 0 && SUMMARY.test(body[k])) { rest = body[k]; body[k] = '' }
      }
      const sm = SUMMARY.exec(rest)
      if (sm) { title = plainSummary(sm[1]); rest = sm[2] }
      if (rest.trim()) body.unshift(rest)
      out.push({
        type: 'admonition',
        kind: 'details',
        title: title || 'Details',
        collapsible: true,
        open: !!d[1],
        body: body.join('\n').trimEnd(),
        line: i + 1,
      })
      i = dEnd
      runStart = dEnd + 2
      continue
    }
    const a = ADMONITION.exec(line)
    if (!a) { run.push(line); continue }
    flush(i + 1)
    let end = i + 1
    while (end < lines.length && isBodyLine(lines[end])) end++
    while (end > i + 1 && lines[end - 1].trim() === '') end--
    const body = lines.slice(i + 1, end).map(dedent).join('\n')
    out.push({
      type: 'admonition',
      kind: a[2].toLowerCase(),
      title: titleOf(a[2], a[3]),
      collapsible: a[1].startsWith('???'),
      open: a[1] === '???+' || a[1] === '!!!',
      body,
      line: i + 1,
    })
    i = end - 1
    runStart = end + 1
  }
  flush(lines.length + 1)
  return out
}

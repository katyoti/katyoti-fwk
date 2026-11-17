// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'

import { hasDialect, lozengeColour, splitDialect, statusLozenges, stripComments } from '../src/utils/markdown-dialect.ts'

// The Markdown (Confluence) dialect of a study page, read
// into segments before a GFM renderer draws them.
describe('the Markdown (Confluence) dialect', () => {
  test('an admonition takes its indented body, dedented; the text after it stays text', () => {
    const segs = splitDialect(['# T', '??? note "1. Gestion"', '    | a | b |', '    |---|---|', '', '    | 1 | 2 |', '', 'after'].join('\n'))
    expect(segs).toEqual([
      { type: 'text', text: '# T', line: 1 },
      { type: 'admonition', kind: 'note', title: '1. Gestion', collapsible: true, open: false, body: '| a | b |\n|---|---|\n\n| 1 | 2 |', line: 2 },
      { type: 'text', text: '\nafter', line: 7 },
    ])
  })
  test('!!! is an open box, ???+ a collapsible open one, a missing title is the kind', () => {
    const [w, t, e] = splitDialect('!!! warning "Brouillon"\n    x\n???+ tip\n    y\n!!! info ""\n    z') as any[]
    expect([w.kind, w.title, w.collapsible, w.open]).toEqual(['warning', 'Brouillon', false, true])
    expect([t.kind, t.title, t.collapsible, t.open]).toEqual(['tip', 'Tip', true, true])
    expect(e.title).toBe('')
  })
  test('[[TOC]] is a segment, comments are blanked keeping lines, fenced code is kept', () => {
    const md = '<!-- acme-da:brouillon -->\n[[TOC]]\n```\n!!! note\n    <!-- kept -->\n```'
    const segs = splitDialect(md)
    expect(segs[0]).toEqual({ type: 'toc', line: 2 })
    expect(segs[1]).toEqual({ type: 'text', text: '```\n!!! note\n    <!-- kept -->\n```', line: 3 })
    expect(stripComments('a <!-- x\ny --> b\nc')).toBe('a \n b\nc')
  })
  test('a text without the dialect is one text segment', () => {
    expect(hasDialect('# plain\n\n    code')).toBe(false)
    expect(splitDialect('# plain\n\n    code')).toEqual([{ type: 'text', text: '# plain\n\n    code', line: 1 }])
  })
})

// Confluence status lozenges and raw <details> blocks.
describe('status lozenges and <details> blocks', () => {
  test('a lozenge with no reference definition becomes an image of source #status-<colour>, code kept', () => {
    const md = '| s | ![DT-COMPLET][status-green] ![NON-TESTÉ][status-grey] |\n`![X][status-red]`\n```\n![Y][status-blue]\n```'
    expect(statusLozenges(md)).toBe('| s | ![DT-COMPLET](#status-green) ![NON-TESTÉ](#status-grey) |\n`![X][status-red]`\n```\n![Y][status-blue]\n```')
    expect(statusLozenges('![A][status-pink]')).toBe('![A][status-pink]')
    expect([lozengeColour('#status-yellow'), lozengeColour('#status-gray'), lozengeColour('img.png')]).toEqual(['yellow', 'grey', undefined])
  })
  test('a <details> block is a collapsible box titled by its <summary>, its body Markdown', () => {
    const md = 'before\n<details>\n<summary>Inventaire <b>PROD</b></summary>\n\n| a |\n|---|\n| 1 |\n</details>\nafter'
    expect(hasDialect(md)).toBe(true)
    expect(splitDialect(md)).toEqual([
      { type: 'text', text: 'before', line: 1 },
      { type: 'admonition', kind: 'details', title: 'Inventaire PROD', collapsible: true, open: false, body: '\n\n| a |\n|---|\n| 1 |', line: 2 },
      { type: 'text', text: 'after', line: 9 },
    ])
  })
  test('<details open> on one line, nested blocks and an unclosed tag', () => {
    const [d] = splitDialect('<details open><summary>S</summary>x</details>') as any[]
    expect([d.kind, d.title, d.open, d.body]).toEqual(['details', 'S', true, 'x'])
    const [outer] = splitDialect('<details><summary>A</summary>\n<details><summary>B</summary>\nb\n</details>\n</details>') as any[]
    expect(outer.body).toBe('<details><summary>B</summary>\nb\n</details>')
    expect(splitDialect(outer.body)[0]).toMatchObject({ kind: 'details', title: 'B', body: 'b' })
    expect(splitDialect('<details>\nnever closed')).toEqual([{ type: 'text', text: '<details>\nnever closed', line: 1 }])
  })
})

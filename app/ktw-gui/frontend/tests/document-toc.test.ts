// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'fs'
import { headingsOf, slugify, TOC_MIN_HEADINGS } from '../src/components/document/toc'

// A long markdown document reads full width beside its
// headings as a table of contents; its tables and diagrams render as before.
const doc = [
  '# Projects',
  'text',
  '```mermaid',
  'graph TD',
  '# not a heading',
  '```',
  '## Repositories',
  '| a | b |',
  '|---|---|',
  '## Repositories',
  '### Univers et dépôts',
  '##### too deep',
].join('\n')

describe('the table of contents', () => {
  test('lists the headings to level 4 with their lines, fenced code skipped', () => {
    expect(headingsOf(doc).map((h) => [h.level, h.text, h.line])).toEqual([
      [1, 'Projects', 1], [2, 'Repositories', 7], [2, 'Repositories', 10], [3, 'Univers et dépôts', 11],
    ])
  })
  test('gives each heading a unique id', () => {
    expect(headingsOf(doc).map((h) => h.id)).toEqual(['projects', 'repositories', 'repositories-2', 'univers-et-depots'])
    expect(slugify('Univers, solutions & `versions` !')).toBe('univers-solutions-versions')
  })
  test('reads a real long document: the architecture one', () => {
    const md = readFileSync(new URL('../../../../modules/architecture/objects/projects-repositories-and-universes.cwm.md', import.meta.url), 'utf8')
    const hs = headingsOf(md)
    expect(hs.length).toBeGreaterThanOrEqual(TOC_MIN_HEADINGS)
    expect(new Set(hs.map((h) => h.id)).size).toBe(hs.length)
  })
  const markdown = readFileSync(new URL('../src/components/Markdown.tsx', import.meta.url), 'utf8')
  test('each heading carries the id of its line, and the table sits beside the text when asked', () => {
    expect(markdown).toContain('<Tag id={headingId(node, ids, offset)}')
    expect(markdown).toContain('const showToc = !!toc && headings.length >= TOC_MIN_HEADINGS')
    expect(markdown).toContain('if (!showToc) return body')
    expect(markdown).toContain('<TableOfContents headings={headings} top={tocTop} />')
    // tables and diagrams render as before
    expect(markdown).toContain("className === 'language-mermaid'")
    expect(markdown).toContain('table: ({ children })')
  })
  test('the document view reads full width with the table of contents', () => {
    const engine = readFileSync(new URL('../src/components/document/engines/markdown.tsx', import.meta.url), 'utf8')
    expect(engine).toContain('<Markdown toc ')
    expect(engine).not.toContain('maxWidth: 860')
  })
  test('a long guide opens full screen beside its table of contents', () => {
    const reader = readFileSync(new URL('../src/guide/GuideReader.tsx', import.meta.url), 'utf8')
    expect(reader).toContain('fullScreen={long}')
    // its table sticks below the modal's sticky header
    expect(reader).toContain('<Markdown text={tx.body} toc={long} tocTop={READER_HEADER_PX} />')
  })
})

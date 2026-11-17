// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'fs'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { Markdown } from '../src/components/Markdown'

// A study page written in the Markdown (Confluence) dialect
// reads right in the file viewer - a ??? note is a collapsible box whose
// indented table is a table, a !!! warning a box, [[TOC]] the page's table of
// contents (nothing when the outline is beside), the brouillon comment hidden.
const page = [
  '<!-- acme:brouillon -->',
  '!!! warning "Brouillon"',
  '    Ce document est un brouillon.',
  '',
  '[[TOC]]',
  '',
  '# Architecture métier',
  '',
  '## Domaines',
  '',
  '??? note "AMONT — Gestion commerciale"',
  '    | Processus | Objectif |',
  '    |---|---|',
  '    | Vente | Vendre |',
  '',
  '## Suite',
  'Texte après.',
].join('\n')

const render = (toc: boolean) => renderToStaticMarkup(
  createElement(MantineProvider, null, createElement(Markdown, { text: page, toc })))

describe('the file viewer reads the Markdown (Confluence) dialect', () => {
  const html = render(false)
  test('??? note is a collapsible box holding a table, never a code block', () => {
    expect(html).toMatch(/<details[^>]*data-guide="markdown.admonition"[^>]*data-kind="note"/)
    expect(html).toContain('AMONT — Gestion commerciale')
    const box = html.slice(html.indexOf('data-kind="note"'))
    expect(box).toContain('<table')
    expect(box).toContain('Vente')
    expect(html).not.toContain('<pre')
    expect(html).not.toContain('| Processus |')
  })
  test('!!! warning is a box with its title and body; no literal markers left', () => {
    expect(html).toMatch(/<div[^>]*data-guide="markdown.admonition"[^>]*data-kind="warning"/)
    expect(html).toContain('Brouillon')
    expect(html).toContain('Ce document est un brouillon.')
    expect(html).not.toContain('!!!')
    expect(html).not.toContain('???')
    expect(html).not.toContain('[[TOC]]')
    expect(html).not.toContain('acme:brouillon')
  })
  test('[[TOC]] lists the headings inline, nothing when the outline is beside', () => {
    expect(html).toContain('data-guide="markdown.toc"')
    expect(html).toContain('href="#domaines"')
    expect(html).toContain('id="domaines"')
    expect(html).toContain('id="suite"')
    // beside the outline (TableOfContents needs the i18n provider: read in source)
    const src = readFileSync(new URL('../src/components/Markdown.tsx', import.meta.url), 'utf8')
    expect(src).toContain('if (showToc || headings.length === 0) return null')
  })
})

// Confluence status lozenges are coloured badges, a raw
// <details> block a collapsible box - never literal text.
describe('the file viewer draws status lozenges and <details> blocks', () => {
  const md = [
    '| Serveur | Monitoring |',
    '|---|---|',
    '| u203 | ![DT-COMPLET][status-green] |',
    '',
    'Statut ![NON-TESTÉ][status-grey] en ligne.',
    '',
    '<details>',
    '<summary>Inventaire PROD</summary>',
    '',
    '| a | b |',
    '|---|---|',
    '| 1 | ![KO][status-red] |',
    '</details>',
  ].join('\n')
  const html = renderToStaticMarkup(createElement(MantineProvider, null, createElement(Markdown, { text: md })))
  test('a lozenge is a badge of its colour, inline in its cell or sentence', () => {
    expect(html).toMatch(/data-guide="markdown.status"[^>]*data-status="green"[^>]*>[\s\S]*?DT-COMPLET/)
    expect(html).toMatch(/data-status="grey"/)
    expect(html).toMatch(/<p[^>]*>Statut <span[^>]*data-guide="markdown.status"/)
    expect(html).not.toContain('[status-')
    expect(html).not.toContain('<img')
  })
  test('<details> is a collapsible box titled by its summary, its table a table with its lozenge', () => {
    expect(html).toMatch(/<details[^>]*data-kind="details"/)
    expect(html).toContain('Inventaire PROD')
    const box = html.slice(html.indexOf('data-kind="details"'))
    expect(box).toContain('<table')
    expect(box).toMatch(/data-status="red"/)
    expect(html).not.toContain('&lt;summary')
  })
})

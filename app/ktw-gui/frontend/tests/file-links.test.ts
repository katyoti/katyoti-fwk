// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { folderOf, isPlainImage, isRelativeRef, joinRef, stripFrontmatter, wrapSpacedDestinations } from '../src/components/document/fileLinks'

describe('file links: a document read from a file resolves its neighbours', () => {
  test('a relative reference, not a URL, an anchor or a root path', () => {
    expect(isRelativeRef('media/a.drawio')).toBe(true)
    expect(isRelativeRef('../x.md')).toBe(true)
    for (const r of ['https://x.test/y.png', 'data:image/png;base64,AA', '#top', '/etc/x', 'mailto:a@b', '', undefined]) {
      expect(isRelativeRef(r)).toBe(false)
    }
  })

  test('joined on the file folder, decoded, never above the root', () => {
    expect(folderOf('confluence/as-is/01-a.md')).toBe('confluence/as-is')
    expect(folderOf('index.md')).toBe('')
    expect(joinRef('confluence/as-is', 'media/SP4321%20-%20Flux%20m%C3%A9tier.drawio')).toBe('confluence/as-is/media/SP4321 - Flux métier.drawio')
    expect(joinRef('confluence/as-is', '../solutions/index.md#top')).toBe('confluence/solutions/index.md')
    expect(joinRef('', './00-contexte.md')).toBe('00-contexte.md')
    expect(joinRef('a', '../../x.md')).toBeUndefined()
  })

  test('a destination with spaces is wrapped so markdown parses it, code and titles left alone', () => {
    const md = [
      '![SP - Flux](media/SP - Flux métier.drawio)',
      '[ok](plain.md) and [t](a b.md "title")',
      '```',
      '![x](in code.png)',
      '```',
      '![w](<already wrapped.png>)',
    ].join('\n')
    const out = wrapSpacedDestinations(md).split('\n')
    expect(out[0]).toBe('![SP - Flux](<media/SP - Flux métier.drawio>)')
    expect(out[1]).toBe('[ok](plain.md) and [t](a b.md "title")')
    expect(out[3]).toBe('![x](in code.png)')
    expect(out[5]).toBe('![w](<already wrapped.png>)')
  })

  test('a plain image is drawn by the browser, a diagram by its engine', () => {
    expect(isPlainImage('media/a.PNG')).toBe(true)
    expect(isPlainImage('media/a.drawio')).toBe(false)
  })
})

describe('navigate with a query: a panel opened on one file', async () => {
  const { navigateTarget, resolveAction } = await import('@ktw/display-lib/ui-builders/page/ui-page.interactive.ts')
  test('the query rides with the panel, templated like the rest', () => {
    const a = resolveAction({ navigate: { object: '${id}', panel: 'files', query: { file: '${item.path}' } } }, { path: 'confluence/as-is/01-a.md' }, { id: 'erp-acme' })
    expect(a.navigate).toEqual({ object: 'erp-acme', panel: 'files', query: { file: 'confluence/as-is/01-a.md' } })
    expect(navigateTarget(a.navigate)).toEqual({ object: 'erp-acme', panel: 'files', query: { file: 'confluence/as-is/01-a.md' } })
  })
  test('a query without a panel is dropped', () => {
    expect(navigateTarget({ object: 'm.x', query: { file: 'a.md' } })).toEqual({ object: 'm.x' })
  })
})

test('a frontmatter is metadata, not prose', () => {
  const fm = ['---', 'page_id: "1"', 'title: T', '---', '', '# T', ''].join('\n')
  expect(stripFrontmatter(fm)).toBe(['', '# T', ''].join('\n'))
  const rule = ['# T', '---', 'x'].join('\n')
  expect(stripFrontmatter(rule)).toBe(rule)
})

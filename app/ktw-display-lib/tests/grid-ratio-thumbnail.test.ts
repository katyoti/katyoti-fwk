// SPDX-License-Identifier: Apache-2.0
// A grid can size its columns by ratio, and a draw.io diagram
// can be shown as a small thumbnail that opens in full.

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { UIPageView } from '../src/ui-builders/page/ui-page.view.tsx'
import { ratioColumns } from '../src/ui-builders/page/nodes/layout.tsx'
import { thumbnailHeight } from '../src/ui-builders/page/nodes/drawio-thumbnail.tsx'
import { UIPageNodeJSONSchema } from '../src/ui-builders/page/ui-page.json-schema.ts'

const page = (node: unknown) => renderToStaticMarkup(createElement(MantineProvider, null, createElement(UIPageView, { node: node as never })))
const xml = '<mxfile><diagram id="d" name="Flux"><mxGraphModel><root><mxCell id="0"/></root></mxGraphModel></diagram></mxfile>'

describe('grid ratio and diagram thumbnail', () => {
  test('the grammar declares them', () => {
    const d = (UIPageNodeJSONSchema as any).definitions
    expect(d.Grid.properties.ratio).toBeDefined()
    expect(d.Drawio.properties.thumbnail).toBeDefined()
  })

  test('a grid sizes its columns by ratio, each a share never below its content', () => {
    expect(ratioColumns([2, 1])).toBe('minmax(0, 2fr) minmax(0, 1fr)')
    expect(ratioColumns([3, 0, -1])).toBe('minmax(0, 3fr) minmax(0, 1fr) minmax(0, 1fr)')
    const html = page({ kind: 'grid', cols: 2, ratio: [2, 1], children: [{ kind: 'text', content: 'main' }, { kind: 'text', content: 'side' }] })
    expect(html).toContain('data-guide="grid.ratio"')
    expect(html).toContain('grid-template-columns:minmax(0, 2fr) minmax(0, 1fr)')
    expect(page({ kind: 'grid', cols: 2, children: [] })).not.toContain('grid.ratio')
  })

  test('a diagram with thumbnail is a small view, landscape or of its height, that opens in full, closed at first', () => {
    expect(thumbnailHeight(true)).toBeUndefined()
    expect(thumbnailHeight(96)).toBe(96)
    expect(page({ kind: 'drawio', xml, thumbnail: true })).toMatch(/aspect-ratio:16 \/ 10/)
    const html = page({ kind: 'drawio', id: 'flux', xml, thumbnail: 96, title: 'Flux applicatifs' })
    expect(html).toContain('data-guide="drawio.thumbnail"')
    expect(html).toMatch(/height:96px/)
    expect(html).toContain('data-guide="drawio.open"')
    expect(html).toContain('Flux applicatifs')
    expect(html).not.toContain('data-guide="drawio.full"')
    expect(page({ kind: 'drawio', xml })).not.toContain('drawio.thumbnail')
  })
})

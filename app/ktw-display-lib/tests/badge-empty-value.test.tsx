// SPDX-License-Identifier: Apache-2.0
// Gate A of the as-is study v2 (the person, 2026-11-10): a badge bound to a
// value that is empty draws nothing - never an empty chip, nor its prefix
// alone (« écrit sans » with nothing after it) - while a static badge and a
// bound badge with a value draw as before.

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { UIPageView } from '../src/ui-builders/page/ui-page.view.tsx'

const page = (node: unknown, data: Record<string, unknown> = {}) =>
  renderToStaticMarkup(createElement(MantineProvider, null, createElement(UIPageView, { node: node as never, data, onAction: () => {} } as never)))

const list = {
  kind: 'list', dataField: '$.docs', item: {
    kind: 'vbox', children: [
      { kind: 'text', content: '', dataField: '$.item.name' },
      { kind: 'badge', text: '', dataField: '$.item.without', prefix: 'écrit sans', color: 'red' },
    ],
  },
}
const rows = (without: string) => page(list, { docs: [{ name: 'Index', without }] })

describe('a badge bound to an empty value', () => {
  test('draws nothing, its prefix neither', () => {
    const html = rows('')
    expect(html).toContain('Index')
    expect(html).not.toContain('écrit sans')
    expect(html).not.toContain('mantine-Badge-root')
  })

  test('a value draws the badge with its prefix', () => {
    const html = rows('LeanIX')
    expect(html).toContain('écrit sans')
    expect(html).toContain('LeanIX')
  })

  test('a static badge with no text still draws', () => {
    const html = page({ kind: 'badge', text: '', color: 'gray' })
    expect(html).toContain('mantine-Badge-root')
  })
})

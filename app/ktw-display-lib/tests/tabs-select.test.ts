// SPDX-License-Identifier: Apache-2.0
// A tabs node's select: choosing a tab runs it with ${item.id} the tab's id,
// so a page building only the tab shown re-reads its source for that tab;
// choosing the tab already shown runs nothing.

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { UIPageNodeJSONSchema } from '../src/ui-builders/page/ui-page.json-schema.ts'

describe('a tabs node select', () => {
  test('is declared on the tabs node, as an action', () => {
    const tabs = (UIPageNodeJSONSchema as any).definitions.Tabs
    expect(tabs.properties.select.$ref).toBe('#/definitions/Action')
  })

  test('choosing another tab runs select; the one shown, or none, does not', async () => {
    const { tabChoice } = await import('../src/ui-builders/page/nodes/layout.tsx')
    expect(tabChoice('a', 'b')).toEqual({ value: 'b', select: true })
    expect(tabChoice('a', 'a')).toEqual({ value: 'a', select: false })
    expect(tabChoice('a', null)).toBeUndefined()
  })

  test('renders the tabs as before', async () => {
    const { TabsRenderer } = await import('../src/ui-builders/page/nodes/layout.tsx')
    const html = renderToStaticMarkup(createElement(MantineProvider, null,
      createElement(TabsRenderer, {
        node: { kind: 'tabs', defaultTab: 'b', select: { label: 'Show', query: { args: { tab: '${item.id}' } } },
          tabs: [{ id: 'a', label: 'A', content: { kind: 'text', content: 'first' } }, { id: 'b', label: 'B', content: { kind: 'text', content: 'second' } }] } as any,
        render: (n: any) => createElement('span', null, n?.content ?? ''),
      })))
    expect(html).toContain('A')
    expect(html).toContain('second')
  })
})

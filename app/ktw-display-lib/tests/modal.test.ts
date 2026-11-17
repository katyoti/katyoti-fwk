// SPDX-License-Identifier: Apache-2.0
// An action can open a centred modal holding any page node.

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { UIPageView } from '../src/ui-builders/page/ui-page.view.tsx'
import { pageOwn, routePageAction } from '../src/ui-builders/page/page-context.tsx'
import { UIPageNodeJSONSchema } from '../src/ui-builders/page/ui-page.json-schema.ts'
import { ACTION_VERBS, actionVerb } from '../src/ui-builders/page/ui-page.interactive.ts'

const confirmNode = { kind: 'vbox', children: [
  { kind: 'text', content: "Rien n'est perdu : l'étude reste « À préparer »." },
  { kind: 'button', label: 'Abandonner', action: { uri: 'ktw-example:studies/abandon' } },
] }
const open = { label: 'Abandonner', modal: { title: "Abandonner l'étude ?", node: confirmNode, size: 'sm' } }

describe('a modal action', () => {
  test('is an action verb of the page grammar, its node required', () => {
    expect(ACTION_VERBS).toContain('modal')
    expect(actionVerb(open as never)).toBe('modal')
    expect((UIPageNodeJSONSchema as any).definitions.Action.properties.modal.required).toEqual(['node'])
  })

  test('the page opens it itself, with its node, title, size and the element it was run on - never the host', () => {
    const opened: unknown[] = []
    routePageAction({ action: open as never, item: { id: 'etude-erp' } }, {
      show: () => {}, select: () => {}, selection: {},
      host: () => { throw new Error('a modal is the page’s own') },
      modal: (m) => opened.push(m),
    })
    expect(opened).toEqual([{ title: "Abandonner l'étude ?", node: confirmNode, size: 'sm', item: { id: 'etude-erp' } }])
  })

  test('an action run from inside it that the host takes closes it; the page’s own leave it open', () => {
    expect(pageOwn({ uri: 'ktw-example:studies/abandon' })).toBe(false)
    expect(pageOwn({ tab: 'sources' })).toBe(true)
    expect(pageOwn({ select: { key: 'k' } })).toBe(true)
    expect(pageOwn(open as never)).toBe(true)
  })

  test('a page with a modal action draws its button and no modal until it runs', () => {
    const html = renderToStaticMarkup(createElement(MantineProvider, null,
      createElement(UIPageView, { node: { kind: 'button', label: 'Abandonner', action: open } as never, onAction: () => {} })))
    expect(html).toContain('Abandonner')
    expect(html).not.toContain('l&#x27;étude reste')
  })
})

// SPDX-License-Identifier: Apache-2.0
// A card and a button carrying an id draw it as their element id
// (`card:<id>`, `button:<id>`) - what a GUI test finds the human-task card and
// its answer buttons by; without an id, none.

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { UIPageView } from '../src/ui-builders/page/ui-page.view.tsx'

const draw = (node: unknown) => renderToStaticMarkup(createElement(MantineProvider, null,
  createElement(UIPageView, { node: node as never, onAction: () => {} })))

describe('element ids of a card and a button', () => {
  test('a card and its button draw their ids', () => {
    const html = draw({ kind: 'card', id: 'inbox:m.TD-1', children: [
      { kind: 'text', content: 'Approve the release notes' },
      { kind: 'button', id: 'approve:m.TD-1', label: 'Approuver', action: { uri: 'ktw-agents:todos/report' } },
    ] })
    expect(html).toContain('data-element="card:inbox:m.TD-1"')
    expect(html).toContain('data-element="button:approve:m.TD-1"')
  })

  test('without an id, no element id', () => {
    const html = draw({ kind: 'card', children: [{ kind: 'button', label: 'Go', action: { uri: 'x:y/z' } }] })
    expect(html).not.toContain('data-element="card:')
    expect(html).not.toContain('data-element="button:')
  })
})

// A form carrying an id draws it as its element id (`form:<id>`)
// - the human-task card's answer form (answer:<id>) and the « Pourquoi ? »
// modal's form (why:<id>) - whether it draws its fields or why it cannot (no
// field renderers are wired here); without an id, none.
const schema = { name: 'why', root: 'A', types: { A: { type: 'object', fields: { note: { type: 'string', label: 'Pourquoi ?' } } } } }

describe('element id of a form', () => {
  test('a form draws its id', () => {
    const html = draw({ kind: 'card', id: 'inbox:m.TD-1', children: [
      { kind: 'form', id: 'answer:m.TD-1', schema, mode: 'edit', hideTitle: true },
    ] })
    expect(html).toContain('data-element="form:answer:m.TD-1"')
  })

  test('without an id, no element id', () => {
    const html = draw({ kind: 'card', children: [{ kind: 'form', schema, mode: 'edit', hideTitle: true }] })
    expect(html).not.toContain('data-element="form:')
  })
})

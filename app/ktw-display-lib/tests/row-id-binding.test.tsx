// SPDX-License-Identifier: Apache-2.0
// A node in a row template takes a per-row element id from a
// bound field (`idField`), so every row of a list or a tree draws its own ids,
// its text toggle included.

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { UIPageView } from '../src/ui-builders/page/ui-page.view.tsx'
import { UIPageNodeJSONSchema } from '../src/ui-builders/page/ui-page.json-schema.ts'
import { applyDataBinding } from '../src/ui-builders/page/nodes/data-binding.ts'
import { TextToggle } from '../src/ui-builders/page/nodes/atomic.tsx'

const wrap = (el: unknown) => renderToStaticMarkup(createElement(MantineProvider, null, el as never))
const page = (node: unknown, data?: unknown) => wrap(createElement(UIPageView, { node: node as never, data, onAction: () => {} } as never))

describe('a per-row element id from a bound field', () => {
  test('idField appends the bound value to the id', () => {
    const n = applyDataBinding({ kind: 'text', id: 'note', idField: '$.item.id', content: 'x' } as never, { item: { id: 'RQ-1' } })
    expect(n.id).toBe('note:RQ-1')
  })

  test('a node with no id takes the value as its id', () => {
    const n = applyDataBinding({ kind: 'text', idField: '$.item.id', content: 'x' } as never, { item: { id: 'RQ-2' } })
    expect(n.id).toBe('RQ-2')
  })

  test('an unresolved or empty value leaves the id as it is', () => {
    const n = { kind: 'text', id: 'note', idField: '$.item.id', content: 'x' } as never
    expect(applyDataBinding(n, { item: {} }).id).toBe('note')
    expect(applyDataBinding(n, { item: { id: '' } }).id).toBe('note')
    expect(applyDataBinding(n, undefined).id).toBe('note')
  })

  test('the id and the main field bind together', () => {
    const n = applyDataBinding({ kind: 'text', id: 'note', idField: '$.item.id', dataField: '$.item.note', content: '' } as never, { item: { id: 'a', note: 'hello' } }) as { id?: string; content?: string }
    expect(n.id).toBe('note:a')
    expect(n.content).toBe('hello')
  })

  test('two rows of a list template draw two distinct element ids', () => {
    const html = page({
      kind: 'list', dataField: '$.rows',
      item: { kind: 'button', id: 'open', idField: '$.item.id', label: 'Open', action: { verb: 'open' } },
    }, { rows: [{ id: 'a' }, { id: 'b' }] })
    expect(html).toContain('data-element="button:open:a"')
    expect(html).toContain('data-element="button:open:b"')
    expect(html).not.toContain('data-element="button:open"')
  })

  test('a bound text toggle carries the row in its element id', () => {
    const a = applyDataBinding({ kind: 'text', id: 'note', idField: '$.item.id', content: 'x', lineClamp: 2, expandable: true } as never, { item: { id: 'a' } })
    const b = applyDataBinding({ kind: 'text', id: 'note', idField: '$.item.id', content: 'x', lineClamp: 2, expandable: true } as never, { item: { id: 'b' } })
    const ta = wrap(createElement(TextToggle, { id: a.id, open: false, onToggle: () => {} }))
    const tb = wrap(createElement(TextToggle, { id: b.id, open: false, onToggle: () => {} }))
    expect(ta).toContain('data-element="text:note:a:toggle"')
    expect(tb).toContain('data-element="text:note:b:toggle"')
  })

  test('the grammar declares idField on a text', () => {
    const t = (UIPageNodeJSONSchema as any).definitions.Text.properties
    expect(t.idField).toEqual({ $ref: '#/definitions/IdField' })
  })
})

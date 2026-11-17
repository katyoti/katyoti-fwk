// SPDX-License-Identifier: Apache-2.0
// A card is a variant of a data type - a list declares that
// each of its elements is of type X drawn as `card`, and each kind of card
// has one definition (the type's `ui-view` extension named `card`) drawn by
// the generic view, never a template written per list nor TSX.

import { describe, expect, test } from 'bun:test'
import { createElement, type ReactElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { wireFieldRenderers } from '../src/ui-builders/fields/factory.ts'
import { UIPageView } from '../src/ui-builders/page/ui-page.view.tsx'
import { UIPageNodeJSONSchema } from '../src/ui-builders/page/ui-page.json-schema.ts'
import { DEFAULT_ITEM_VARIANT, listItemNode } from '../src/ui-builders/page/ui-page.interactive.ts'
import { childrenOf, hasSchemaBound } from '../src/ui-builders/page/ui-page.types.ts'
import { dataType, releaseCard } from './fixtures/delivery-sample.ts'

wireFieldRenderers()

const html = (el: ReactElement) => renderToStaticMarkup(createElement(MantineProvider, null, el))
const page = (node: unknown) => html(createElement(UIPageView, { node: node as never, onAction: (() => {}) as never }))

const second = { ...releaseCard, id: 'RL-034', label: '2026.11', stage: 'planned', summary: '30/11 · 0/3 done', planned_text: '', candidate_text: '' }

describe('a list of type X drawn as its card', () => {
  test('draws each element by the card the data type defines, through the generic view', () => {
    const out = page({ kind: 'list', id: 'releases', data: [releaseCard, second], itemType: dataType('release-card'), itemVariant: 'card' })
    expect((out.match(/data-ktw-view="slots"/g) ?? []).length).toBe(2)
    expect(out).toContain('RL-033')
    expect(out).toContain('>1.006<')
    expect(out).toMatch(/data-ktw-mark="stage"[^>]*>candidate</)
    expect(out).toContain('RL-034')
    expect(out).toContain('>2026.11<')
    expect(out).toContain('31/10 · 5/8 done')
  })

  test('the card is the default variant; the list gives no template', () => {
    const typed = { id: 'releases', itemType: 'ktw-agents:/data-type/release-card' }
    expect(DEFAULT_ITEM_VARIANT).toBe('card')
    expect(listItemNode(typed)).toEqual({ kind: 'view', id: 'releases:item', schema: 'ktw-agents:/data-type/release-card', variant: 'card', dataField: '$.item' } as never)
    expect(listItemNode({ ...typed, itemVariant: 'row' })).toMatchObject({ variant: 'row' })
    // A template of its own wins; neither: nothing to draw.
    const item = { kind: 'text', content: '', dataField: '$.item.label' }
    expect(listItemNode({ ...typed, item: item as never })).toBe(item as never)
    expect(listItemNode({})).toBeUndefined()
    expect(childrenOf({ kind: 'list', ...typed } as never)).toHaveLength(1)
    expect(childrenOf({ kind: 'list' } as never)).toEqual([])
  })

  test('the same data type draws the same card in every list: one definition', () => {
    const a = page({ kind: 'list', data: [releaseCard], itemType: dataType('release-card') })
    const b = page({ kind: 'list', variant: 'cards', layout: 'grid', data: [releaseCard], itemType: dataType('release-card') })
    const card = (s: string) => s.match(/<div data-ktw-view="slots"[\s\S]*$/)?.[0].slice(0, 400)
    expect(card(a)).toBeDefined()
    expect(card(b)).toBe(card(a))
  })

  test('the grammar takes a list with an item template, an item type or the node each element carries (nodeField)', () => {
    const list = (UIPageNodeJSONSchema as any).definitions.List
    expect(list.required).toEqual(['kind'])
    expect(list.anyOf).toEqual([{ required: ['item'] }, { required: ['itemType'] }, { required: ['nodeField'] }])
    expect(list.properties.itemVariant.type).toBe('string')
  })
})

// A host resolves every schema a tree names before it draws
// it - a list's itemType named by id included, so a panel's release cards are
// drawn as cards, never as « named by id and was not resolved ».
describe('a list naming its item type by id is resolved before it is drawn', () => {
  const URI = 'ktw-agents:/data-type/release-card'
  /** A stub host resolution: every schema and itemType named by id inline. */
  const resolve = (v: unknown): unknown => {
    if (Array.isArray(v)) return v.map(resolve)
    if (!v || typeof v !== 'object') return v
    const out: Record<string, unknown> = {}
    for (const [k, c] of Object.entries(v as Record<string, unknown>)) {
      out[k] = (k === 'schema' || k === 'itemType') && typeof c === 'string' ? dataType(c.split('/').pop()!) : k === 'data' ? c : resolve(c)
    }
    return out
  }
  const tree = {
    kind: 'vbox', id: 'root', children: [
      { kind: 'list', id: 'releases', data: [releaseCard, second], itemType: URI, itemVariant: 'card' },
      { kind: 'section', id: 'more', title: 'More', children: [{ kind: 'view', id: 'one', schema: URI, variant: 'card', data: second }] },
    ],
  }

  test('the tree says it needs resolving: a list with an itemType counts', () => {
    expect(hasSchemaBound({ kind: 'page', children: [{ kind: 'list', data: [], itemType: URI }] })).toBe(true)
    expect(hasSchemaBound(tree)).toBe(true)
    expect(hasSchemaBound({ kind: 'page', children: [{ kind: 'list', data: [], item: { kind: 'text', content: '' } }] })).toBe(false)
  })

  test('resolved, the list draws its 2 release cards and the nested view its own', () => {
    const drawn = (t: unknown) => page(hasSchemaBound(t) ? resolve(t) : t)
    const alone = drawn(tree.children[0])
    expect(alone).not.toContain('was not resolved')
    expect((alone.match(/data-ktw-view="slots"/g) ?? []).length).toBe(2)
    const out = drawn(tree)
    expect(out).not.toContain('was not resolved')
    expect((out.match(/data-ktw-view="slots"/g) ?? []).length).toBe(3)
    expect(out).toContain('>1.006<')
    expect(out).toContain('31/10 · 5/8 done')
    expect(out).toContain('>2026.11<')
  })
})

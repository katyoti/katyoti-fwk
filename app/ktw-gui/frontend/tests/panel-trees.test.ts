// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'

import { askFormNode } from '../src/sections/panels/panelInput'
import { drawnTree, formShell, nextFormIdentity, sameValue, withFormData, type TreeResolve } from '../src/sections/panels/panelTrees'
import { shownResolution, type Resolution } from '../src/sections/workspace/resolvePanel'

// A display panel draws every tree resolved - the renderer refuses
// a schema named by id or URI on purpose, the panel resolves it first, one
// resolution for every schema-bound node kind.

const SCHEMA = { root: 'R', types: { R: { kind: 'object', fields: [{ name: 'name', type: 'string' }] } } }

/** A stub object pane: every schema-bound node naming its schema by a string
 *  gets it inline; the trees it was asked for are kept. */
function stubResolve(): { resolve: TreeResolve; asked: unknown[] } {
  const asked: unknown[] = []
  const inline = (v: unknown): unknown => {
    if (Array.isArray(v)) return v.map(inline)
    if (!v || typeof v !== 'object') return v
    const n = v as Record<string, unknown>
    const out: Record<string, unknown> = {}
    for (const [k, c] of Object.entries(n)) out[k] = (k === 'schema' || k === 'itemType') && typeof c === 'string' ? { ...SCHEMA, id: c } : inline(c)
    return out
  }
  return { asked, resolve: async (tree) => { asked.push(tree); return inline(tree) } }
}

const ask = (form: unknown) => ({ question: 'Which release?', form, value: { name: '2026.11' } })

describe('the trees a display panel draws are resolved first', () => {
  test('form: a question naming its data type by URI draws the type inline', async () => {
    const { resolve, asked } = stubResolve()
    const got = await drawnTree(askFormNode(ask({ uri: 'ktw-agents:/data-type/release-pick' })), resolve) as Record<string, unknown>
    expect(asked.length).toBe(1)
    expect(got).toMatchObject({ kind: 'form', id: 'panel:ask-form', data: { name: '2026.11' } })
    expect(got.schema).toMatchObject({ root: 'R', id: 'ktw-agents:/data-type/release-pick' })
  })

  test('form: a question carrying its form as a node {kind, schema} draws the node\'s schema, never the node as a schema', async () => {
    const form = askFormNode(ask({ kind: 'form', schema: 'ktw-agents:/data-type/release-pick' })) as unknown as Record<string, unknown>
    expect(form.schema).toBe('ktw-agents:/data-type/release-pick')
    const { resolve } = stubResolve()
    const got = await drawnTree(form, resolve) as Record<string, unknown>
    expect(got.schema).toMatchObject({ root: 'R', id: 'ktw-agents:/data-type/release-pick' })
    const named = askFormNode(ask({ kind: 'form', schema: { uri: 'ktw-agents:/data-type/release-pick', hash: 'h1' } })) as unknown as Record<string, unknown>
    expect(named.schema).toBe('ktw-agents:/data-type/release-pick')
  })

  test('view: a page tree whose view names its schema by id draws it resolved, ids kept for the ops', async () => {
    const tree = { kind: 'page', id: 'root', children: [{ kind: 'view', id: 'cards', schema: 'ktw-agents:/data-type/release-card', data: [] }] }
    const { resolve, asked } = stubResolve()
    const got = await drawnTree(tree, resolve) as { children: Array<Record<string, unknown>> }
    expect(asked).toEqual([tree])
    expect(got.children[0]).toMatchObject({ kind: 'view', id: 'cards' })
    expect(got.children[0].schema).toMatchObject({ root: 'R', id: 'ktw-agents:/data-type/release-card' })
  })

  test('list: a list of release cards naming its itemType by id, and a nested reference, are drawn resolved', async () => {
    const card = 'ktw-agents:/data-type/release-card'
    const cards = { kind: 'list', id: 'releases', itemType: card, itemVariant: 'card', data: [{ id: 'RL-033' }, { id: 'RL-034' }] }
    const tree = { kind: 'vbox', id: 'root', children: [cards, { kind: 'section', children: [{ kind: 'view', id: 'one', schema: card }] }] }
    const { resolve, asked } = stubResolve()
    // The list alone needs resolving: its item type is a reference.
    expect(await drawnTree(cards, resolve)).not.toBe(cards)
    const got = await drawnTree(tree, resolve) as { children: Array<Record<string, unknown>> }
    expect(asked).toEqual([cards, tree])
    expect(got.children[0]).toMatchObject({ kind: 'list', id: 'releases', itemVariant: 'card' })
    expect(got.children[0].itemType).toMatchObject({ root: 'R', id: card })
    expect((got.children[0].data as unknown[]).length).toBe(2)
    const nested = (got.children[1].children as Array<Record<string, unknown>>)[0]
    expect(nested.schema).toMatchObject({ root: 'R', id: card })
    // Resolved, nothing is left to ask.
    const inlineCards = { ...cards, itemType: SCHEMA }
    expect(await drawnTree(inlineCards, resolve)).toBe(inlineCards)
  })

  test('form: a question asked again with another value (a refusal) starts no new resolution and draws the new value at once', async () => {
    const type = { uri: 'ktw-agents:/data-type/document' }
    const first = formShell(askFormNode({ question: 'Which document?', form: type, value: { summary: 'untitled' } }))
    const again = formShell(askFormNode({ question: 'Which document?', form: type, value: { summary: 'refused' } }))
    // The resolution is keyed on the form without its value: the same key.
    expect(JSON.stringify(again.shell)).toBe(JSON.stringify(first.shell))
    const { resolve, asked } = stubResolve()
    const identity = 'u|dp-1|form'
    const key = JSON.stringify(first.shell)
    const got: Resolution = { identity, key, tree: await drawnTree(first.shell, resolve) }
    expect(asked.length).toBe(1)
    // The second ask is drawn from that resolution, with its own value.
    const shown = shownResolution(JSON.stringify(again.shell), got, identity)
    const drawn = withFormData(shown.tree, again.data) as Record<string, unknown>
    expect(drawn).toMatchObject({ kind: 'form', id: 'panel:ask-form', data: { summary: 'refused' } })
    expect(drawn.schema).toMatchObject({ root: 'R', id: 'ktw-agents:/data-type/document' })
    expect((withFormData(shown.tree, first.data) as Record<string, unknown>).data).toEqual({ summary: 'untitled' })
  })

  test("form: a refusal's re-ask keeps the form's key, a new question or another type gets a new one", () => {
    const type = { uri: 'ktw-agents:/data-type/document' }
    const formOf = (question: string, value: unknown, form: unknown = type) => askFormNode({ question, form, value })
    const first = nextFormIdentity(undefined, formOf('Which document?', { summary: 'untitled' }), undefined)
    // The person sends the form; Go re-marshals it with its keys sorted.
    const sent = { title: 'Mine', summary: 'untitled' }
    expect(sameValue(sent, { summary: 'untitled', title: 'Mine' })).toBe(true)
    // A refusal asks again over the same form with the value sent, its question prefixed by the reason.
    const reAsk = nextFormIdentity(first, formOf('Title too short\n\nWhich document?', { summary: 'untitled', title: 'Mine' }), sent)
    expect(reAsk.key).toBe(first.key)
    // A new question of the same type carries its own value: a new key, the form starts fresh.
    const next = nextFormIdentity(reAsk, formOf('Which document?', { summary: 'next' }), sent)
    expect(next.key).not.toBe(first.key)
    // Another data type: a new key.
    const other = nextFormIdentity(next, formOf('Which other?', { summary: 'next' }, { uri: 'ktw-agents:/data-type/other' }), { summary: 'next' })
    expect(other.key).not.toBe(next.key)
    // An ask with nothing sent through the form (an answer sent elsewhere): a new key.
    const fresh = nextFormIdentity(other, formOf('Which other?', { summary: 'next' }, { uri: 'ktw-agents:/data-type/other' }), undefined)
    expect(fresh.key).not.toBe(other.key)
  })

  test('a tree holding no reference is drawn as it is, without a call', async () => {
    const tree = { kind: 'page', children: [{ kind: 'view', id: 'cards', schema: SCHEMA }, { kind: 'markdown', text: 'hi' }] }
    const { resolve, asked } = stubResolve()
    expect(await drawnTree(tree, resolve)).toBe(tree)
    expect(asked.length).toBe(0)
  })

  test('a resolution that fails rejects with its error (the panel shows it)', async () => {
    const tree = { kind: 'view', id: 'cards', schema: 'ktw-agents:/data-type/missing' }
    await expect(drawnTree(tree, async () => { throw new Error('no data type missing') })).rejects.toThrow('no data type missing')
  })
})

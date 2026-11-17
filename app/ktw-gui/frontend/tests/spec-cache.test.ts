// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'

import { guiHostServices } from '../src/components/compositions/hostServices'
import { ctSchema } from '../src/i18n/contrib'
import { inlineParts, localizeResolved, namedTreeDef, namedTrees, needsResolve, partFor, partKey, RESOLVE_PART, TEMPLATE, templateFor, templateKey, templatePage, type PartFetch, type SpecCache, type TemplateFetch } from '../src/lib/specCache'

// The GUI keeps every specification part an answer names by
// {uri, hash} - once per universe, keyed by kind, URI and variant -, so a part
// already held at that hash costs no call and only data crosses the wire.
const row = { name: 'row', root: 'Row', types: { Row: { type: 'object', fields: { n: { type: 'number' } } } } }

describe('the spec cache', () => {
  const counting = (answers: Array<{ part?: unknown; hash: string; cacheable: boolean }>) => {
    const asked: unknown[] = []
    const fetch: PartFetch = async (input) => { asked.push(input); return answers.shift() as never }
    return { asked, fetch }
  }
  const ref = { kind: 'table', schema: 'ktw-x:/data-type/row' }

  test('a part held at the answer hash costs no call', async () => {
    const cache: SpecCache = new Map([[partKey(ref), { hash: 'h1', part: row }]])
    const { asked, fetch } = counting([])
    expect(await partFor(cache, ref, 'h1', fetch)).toEqual(row)
    expect(asked).toEqual([])
  })

  test('a miss asks once with the hash held; the hash alone keeps the part held; a new hash replaces it', async () => {
    const cache: SpecCache = new Map()
    const { asked, fetch } = counting([
      { part: row, hash: 'h1', cacheable: true },
      { hash: 'h1', cacheable: true },
      { part: { ...row, name: 'row2' }, hash: 'h2', cacheable: true },
    ])
    expect(await partFor(cache, ref, undefined, fetch)).toEqual(row)
    expect(await partFor(cache, ref, undefined, fetch)).toEqual(row)
    expect((await partFor(cache, ref, 'h2', fetch))?.name).toBe('row2')
    expect(asked).toEqual([{ kind: 'table', schema: 'ktw-x:/data-type/row' }, { kind: 'table', schema: 'ktw-x:/data-type/row', hash: 'h1' }, { kind: 'table', schema: 'ktw-x:/data-type/row', hash: 'h1' }])
    expect(cache.get(partKey(ref))?.hash).toBe('h2')
  })

  test('a part that computes its choices is never kept', async () => {
    const cache: SpecCache = new Map()
    const { fetch } = counting([{ part: row, hash: 'h1', cacheable: false }])
    await partFor(cache, ref, 'h1', fetch)
    expect(cache.size).toBe(0)
  })

  test('parts named in the tree are given inline; a node laying its own ui is left; the tree given is unchanged', async () => {
    const cache: SpecCache = new Map([[partKey(ref), { hash: 'h1', part: row }]])
    const tree = { kind: 'vbox', children: [{ kind: 'table', schema: 'row' }, { kind: 'table', schema: 'row', ui: { name: 'x' } }] }
    const out = await inlineParts(tree, { row: { uri: 'ktw-x:/data-type/row', hash: 'h1' } }, cache, async () => { throw new Error('no call') }) as typeof tree
    expect(out.children[0].schema).toEqual(row as never)
    expect(out.children[1].schema).toBe('row')
    expect(tree.children[0].schema).toBe('row')
    expect(needsResolve(out.children[0])).toBe(false)
    expect(needsResolve(out)).toBe(true)
  })
})

// A part inlined through the localize hook reads in the
// person's language - field labels, menu and slots.actions texts, groups with
// an id, the axis label -, each falling back to its own; the cache keeps the
// part as answered, so another language re-inlines from it with no call.
describe('a part inlined in the person language', () => {
  const uri = 'ktw-x:/data-type/rows'
  const rows = {
    name: 'rows', root: 'Row', types: { Row: {
      type: 'object',
      fields: { n: { type: 'number', label: 'Number' } },
      extensions: [
        { type: 'ui-table', name: 'default', menu: [{ id: 'hold', label: 'Hold', prompt: 'Why?', confirm: 'Hold it?' }, { label: 'No id' }] },
        { type: 'ui-view', name: 'header', slots: { actions: [{ id: 'next', label: '${item.next_label}' }, { id: 'freeze', label: 'Freeze' }] },
          groups: [{ id: 'ships', title: 'Ships in this release' }, { title: 'No id' }] },
        { type: 'ui-timeline-lanes', name: 'lanes', axisLabel: 'Stream' },
      ],
    } },
  }
  const fr: Record<string, string> = {
    [`${uri}#types.Row.fields.n.label`]: 'Nombre',
    [`${uri}#types.Row.extensions.default.menu.hold.label`]: 'Suspendre',
    [`${uri}#types.Row.extensions.default.menu.hold.confirm`]: 'La suspendre ?',
    [`${uri}#types.Row.extensions.header.slots.actions.freeze.label`]: 'Geler',
    [`${uri}#types.Row.extensions.header.groups.ships.title`]: 'Livré dans cette version',
    [`${uri}#types.Row.extensions.lanes.axisLabel`]: 'Flux',
  }
  const bundles: Record<string, Record<string, string>> = { fr, en: {} }
  const localizeIn = (lang: string) => (u: string, part: Record<string, unknown>) =>
    ctSchema((uu, path, fb) => bundles[lang][`${uu}#${path}`] ?? fb, u, part)
  const tableRef = { kind: 'table', schema: uri }
  type Ext = { menu?: Array<Record<string, string>>; slots?: { actions: Array<Record<string, string>> }; groups?: Array<Record<string, string>>; axisLabel?: string }
  const draw = async (cache: SpecCache, lang: string) => {
    const tree = { kind: 'vbox', children: [{ kind: 'table', schema: 'rows' }] }
    const out = await inlineParts(tree, { rows: { uri, hash: 'h1' } }, cache, async () => { throw new Error('no call') }, localizeIn(lang)) as { children: Array<{ schema: typeof rows }> }
    return out.children[0].schema.types.Row as unknown as { fields: { n: { label: string } }; extensions: Ext[] }
  }

  test('the texts read in French, the cached part stays English, English re-inlines with no call', async () => {
    const cache: SpecCache = new Map([[partKey(tableRef), { hash: 'h1', part: rows }]])
    const t = await draw(cache, 'fr')
    expect(t.fields.n.label).toBe('Nombre')
    expect(t.extensions[0].menu).toEqual([{ id: 'hold', label: 'Suspendre', prompt: 'Why?', confirm: 'La suspendre ?' }, { label: 'No id' }])
    expect(t.extensions[1].slots?.actions).toEqual([{ id: 'next', label: '${item.next_label}' }, { id: 'freeze', label: 'Geler' }])
    expect(t.extensions[1].groups).toEqual([{ id: 'ships', title: 'Livré dans cette version' }, { title: 'No id' }])
    expect(t.extensions[2].axisLabel).toBe('Flux')
    // The cache holds the part as answered.
    const held = cache.get(partKey(tableRef))?.part as typeof rows
    expect(held).toBe(rows)
    expect(rows.types.Row.extensions[0].menu?.[0].label).toBe('Hold')
    expect(rows.types.Row.fields.n.label).toBe('Number')
    const en = await draw(cache, 'en')
    expect(en.extensions[0].menu?.[0].label).toBe('Hold')
    expect(en.extensions[2].axisLabel).toBe('Stream')
  })
})

describe('a section opened again carries only its data', () => {
  test('the second load asks neither ResolvePart nor the object pane', async () => {
    const calls: string[] = []
    const answer = { tree: { kind: 'table', schema: 'row' }, parts: { row: { uri: 'ktw-x:/data-type/row', hash: 'h1' } }, data: [{ n: 1 }] }
    const run = async (_u: string, uri: string) => { calls.push(uri); return { value: JSON.stringify(answer) } }
    ;(globalThis as unknown as { window: unknown }).window = {
      go: { main: { App: {
        WorkspaceRunAction: run, WorkspaceRunActionAs: run, WorkspaceCancelCall: async () => {},
        CallKTSPMethod: async (_u: string, slot: string) => { calls.push(slot); return { response: { part: row, hash: 'h1', cacheable: true } } },
      } } },
    }
    const host = guiHostServices('cache-test')
    const first = await host.loadSection!({ method: 'ktw-x:views/rows' }) as Record<string, unknown>
    expect((first.tree as { schema: unknown }).schema).toEqual(row)
    expect(first.parts).toBeUndefined()
    expect(calls).toEqual(['ktw-x:views/rows', RESOLVE_PART])
    calls.length = 0
    await host.loadSection!({ method: 'ktw-x:views/rows', args: { again: true } })
    expect(calls).toEqual(['ktw-x:views/rows'])
  })
})

// A tree an answer names by {uri, hash} - a page
// template - is kept in the same cache by its URI: Template is asked once.
describe('a page template named by an answer', () => {
  const uri = 'ktw-x:/page-template/board'
  const tree = { kind: 'vbox', id: 'root', children: [{ kind: 'text', id: 'msg', text: 'hello' }] }
  const counting = (answers: Array<{ tree?: Record<string, unknown>; hash: string }>) => {
    const asked: unknown[] = []
    const fetch: TemplateFetch = async (input) => { asked.push(input); return answers.shift() as never }
    return { asked, fetch }
  }

  test('the first draw asks Template once; a second at the same hash asks nothing', async () => {
    const cache: SpecCache = new Map()
    const { asked, fetch } = counting([{ tree, hash: 'h1' }])
    expect(await templateFor(cache, { uri, hash: 'h1' }, fetch)).toEqual(tree)
    expect(await templateFor(cache, { uri, hash: 'h1' }, fetch)).toEqual(tree)
    expect(asked).toEqual([{ uri }])
    expect(cache.get(templateKey(uri))?.hash).toBe('h1')
  })

  test('a stale hash asks once with the hash held; a hash-only answer keeps the held tree', async () => {
    const cache: SpecCache = new Map([[templateKey(uri), { hash: 'h1', part: tree }]])
    const { asked, fetch } = counting([{ hash: 'h1' }, { tree: { ...tree, id: 'v2' }, hash: 'h2' }])
    expect(await templateFor(cache, { uri, hash: 'h0' }, fetch)).toEqual(tree)
    expect(asked).toEqual([{ uri, hash: 'h1' }])
    expect((await templateFor(cache, { uri, hash: 'h2' }, fetch))?.id).toBe('v2')
    expect(asked).toEqual([{ uri, hash: 'h1' }, { uri, hash: 'h1' }])
    expect(cache.get(templateKey(uri))?.hash).toBe('h2')
  })

  test('an answer naming its tree gets it inline; another answer is left as it is', async () => {
    const cache: SpecCache = new Map([[templateKey(uri), { hash: 'h1', part: tree }]])
    const none: TemplateFetch = async () => { throw new Error('no call') }
    const answer = { tree: { uri, hash: 'h1' }, data: { n: 1 } }
    expect(await namedTrees(answer, cache, none)).toEqual({ tree, data: { n: 1 } })
    expect(answer.tree).toEqual({ uri, hash: 'h1' })
    const plain = { tree: { kind: 'text' } }
    expect(await namedTrees(plain, cache, none)).toBe(plain)
    expect(await namedTreeDef(answer, cache, none, async () => { throw new Error('no call') })).toEqual({ ...tree, data: { n: 1 } })
    expect(await namedTreeDef(plain, cache, none, async () => { throw new Error('no call') })).toBe(plain)
  })

  test("a panel's page lays the ops taken while its template was asked over it", () => {
    const page = templatePage(tree, 1, [{ seq: 2, op: 'replace', target: 'msg', node: { kind: 'text', text: 'bye' } } as never])
    expect(((page.tree as typeof tree).children[0] as { text: string }).text).toBe('bye')
    expect(page.seq).toBe(2)
  })

  test('a section naming its tree asks Template once, then nothing', async () => {
    const calls: string[] = []
    const answer = { tree: { uri, hash: 'h1' }, data: { n: 1 } }
    const run = async (_u: string, m: string) => { calls.push(m); return { value: JSON.stringify(answer) } }
    ;(globalThis as unknown as { window: unknown }).window = {
      go: { main: { App: {
        WorkspaceRunAction: run, WorkspaceRunActionAs: run, WorkspaceCancelCall: async () => {},
        CallKTSPMethod: async (_u: string, slot: string) => { calls.push(slot); return { response: { tree, hash: 'h1' } } },
      } } },
    }
    const host = guiHostServices('template-test', 'expert')
    const first = await host.loadSection!({ method: 'ktw-x:views/board' }) as Record<string, unknown>
    expect(calls).toEqual(['ktw-x:views/board', TEMPLATE])
    expect((first.tree as { kind?: string }).kind).toBe('vbox')
    calls.length = 0
    await host.loadSection!({ method: 'ktw-x:views/board', args: { again: true } })
    expect(calls).toEqual(['ktw-x:views/board'])
  })
})

// A tree the object pane resolved
// (ResolvePanel) - a connector's location form naming its data-type by URI -
// reads in the person's language: the node whose schema was a URI gets its
// inline schema through localize, its field labels and groups translated;
// a node whose schema came inline is left as it is.
describe('a resolved tree in the person language', () => {
  const uri = 'ktw-x:/data-type/where'
  const inline = { name: 'where', root: 'Where', types: { Where: { type: 'object',
    fields: { path: { type: 'string', label: 'Folder' } },
    extensions: [{ type: 'ui-form', name: 'default', groups: [{ id: 'advanced', title: 'Advanced' }] }] } } }
  const own = { name: 'own', root: 'Own', types: { Own: { type: 'object', fields: { n: { type: 'number', label: 'Count' } } } } }
  const fr: Record<string, string> = {
    [`${uri}#types.Where.fields.path.label`]: 'Dossier',
    [`${uri}#types.Where.extensions.default.groups.advanced.title`]: 'Avancé',
  }
  const localize = (u: string, part: Record<string, unknown>) => ctSchema((uu, path, fb) => fr[`${uu}#${path}`] ?? fb, u, part)
  const asked = { kind: 'vbox', children: [
    { kind: 'section', title: 'Où elle lit', children: [{ kind: 'form', id: 'location-x', schema: uri, dataField: '$.location' }] },
    { kind: 'form', id: 'own', schema: own },
  ] }
  const resolved = { kind: 'vbox', children: [
    { kind: 'section', title: 'Où elle lit', children: [{ kind: 'form', id: 'location-x', schema: inline, dataField: '$.location' }] },
    { kind: 'form', id: 'own', schema: own },
  ] }

  test('the form named by URI reads in French, the inline one stays its own', () => {
    type Node = { schema: { types: Record<string, { fields: Record<string, { label: string }>; extensions?: Array<{ groups: Array<{ title: string }> }> }> } }
    const out = localizeResolved(asked, resolved, localize) as { children: [{ children: [Node] }, Node] }
    const where = out.children[0].children[0].schema.types.Where
    expect(where.fields.path.label).toBe('Dossier')
    expect(where.extensions?.[0].groups[0].title).toBe('Avancé')
    expect(out.children[1].schema.types.Own.fields.n.label).toBe('Count')
    // The resolved tree is not changed.
    expect(inline.types.Where.fields.path.label).toBe('Folder')
  })
})

// A field's content.options reads in the person's language: each entry keyed
// by its value, its label and description translated, the English its own when
// the bundle lacks it; an entry without a value and the short map form stay as
// they are; the schema given is not changed.
describe('a field\'s options in the person language', () => {
  const uri = 'ktw-x:/data-type/settings'
  const schema = { name: 'settings', root: 'S', types: { S: { type: 'object', fields: {
    kinds: { type: 'array', label: 'Kinds', content: { type: 'string', options: [
      { value: 'a', label: 'Alpha', description: 'The first' },
      { value: 'b', label: 'Beta' },
      { label: 'No value' },
    ] } },
    short: { type: 'string', content: { options: { x: 'Ex' } } },
  } } } }
  const fr: Record<string, string> = {
    [`${uri}#types.S.fields.kinds.content.options.a.label`]: 'Alpha FR',
    [`${uri}#types.S.fields.kinds.content.options.a.description`]: 'La première',
    [`${uri}#types.S.fields.short.content.options.x.label`]: 'Ex FR',
  }

  test('a French label and description, English fallback, value-less and map form unchanged, input untouched', () => {
    const before = JSON.stringify(schema)
    const out = ctSchema((u, path, fb) => fr[`${u}#${path}`] ?? fb, uri, schema)
    expect(out.types.S.fields.kinds.content.options).toEqual([
      { value: 'a', label: 'Alpha FR', description: 'La première' },
      { value: 'b', label: 'Beta' },
      { label: 'No value' },
    ])
    expect(out.types.S.fields.kinds.content.type).toBe('string')
    expect(out.types.S.fields.short.content).toEqual({ options: { x: 'Ex' } })
    expect(JSON.stringify(schema)).toBe(before)
  })
})

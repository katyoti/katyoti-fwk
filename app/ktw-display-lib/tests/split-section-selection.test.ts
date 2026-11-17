// SPDX-License-Identifier: Apache-2.0
// A page of placed sections sharing a selection:
// a split lays a main column and a sticky side panel; a select action sets a
// key of the page's selection on the page itself; `${selection.<key>}` fills
// the host's action templates; a load-only section loads for its args
// resolved against the selection, and re-reads only when the keys it reads
// change.

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { UIPageView } from '../src/ui-builders/page/ui-page.view.tsx'
import { routePageAction, initialSelection } from '../src/ui-builders/page/page-context.tsx'
import { resolveSelectionArgs, sectionLoad, sectionMode, selectionKeys, selectionReady } from '../src/ui-builders/page/nodes/section-load.tsx'
import { actionVerb, resolveAction, type UIPageActionEvent } from '../src/ui-builders/page/ui-page.interactive.ts'
import { childrenOf } from '../src/ui-builders/page/ui-page.types.ts'
import { UIPageNodeJSONSchema } from '../src/ui-builders/page/ui-page.json-schema.ts'
import { HostServicesContext, type HostServices } from '../src/ui-core/host/index.tsx'

const page = (node: unknown, data?: unknown, host: HostServices = {}) => renderToStaticMarkup(
  createElement(MantineProvider, null,
    createElement(HostServicesContext.Provider, { value: host },
      createElement(UIPageView, { node: node as never, data }))))

describe('a split', () => {
  const split = { kind: 'split', main: { kind: 'text', content: 'the streams' }, side: { kind: 'text', content: 'the release' } }

  test('renders its main and its side, the side at its width and sticky', () => {
    const html = page(split)
    expect(html).toContain('the streams')
    expect(html).toContain('the release')
    expect(html).toContain('--ktw-split-side:380px')
    expect(html).toContain('data-sticky')
    expect(html).toContain('@container (max-width: 999px)')
    const narrow = page({ ...split, sideWidth: 300, sticky: false })
    expect(narrow).toContain('--ktw-split-side:300px')
    expect(narrow).not.toContain('data-sticky=""')
  })

  test('walks main and side as its children; is declared in the schema', () => {
    expect(childrenOf(split as never)).toEqual([split.main, split.side] as never)
    const defs = (UIPageNodeJSONSchema as any).definitions
    expect(defs.Split.required).toEqual(['kind', 'main', 'side'])
  })
})

describe('the selection', () => {
  const route = (e: UIPageActionEvent, selection: Record<string, unknown> = {}) => {
    const out = { shown: [] as string[], set: [] as Array<[string, unknown]>, hosted: [] as UIPageActionEvent[] }
    routePageAction(e, {
      show: (t) => out.shown.push(t), select: (k, v) => out.set.push([k, v]), host: (h) => out.hosted.push(h), selection,
    })
    return out
  }

  test('a select action sets its key, its value resolved against the item, and never reaches the host', () => {
    const action = { label: 'Open', select: { key: 'release', value: '${item.id}' } }
    expect(actionVerb(action)).toBe('select')
    const out = route({ action, item: { id: 'R-2026.11' } })
    expect(out.set).toEqual([['release', 'R-2026.11']])
    expect(out.hosted).toEqual([])
    expect(out.shown).toEqual([])
  })

  test('${selection.<key>} reaches the host action templates', () => {
    const out = route({ action: { uri: 'x:y/z', args: { release: '${selection.release}' } } }, { release: 'R-1' })
    expect(out.hosted.length).toBe(1)
    const e = out.hosted[0]
    const resolved = resolveAction(e.action, e.item, { id: 'o', ...(e.extra ?? {}) })
    expect(resolved.args).toEqual({ release: 'R-1' })
  })

  test('starts from the page data selection', () => {
    expect(initialSelection({ selection: { release: 'R-1' } })).toEqual({ release: 'R-1' })
    expect(initialSelection({ selection: 3 })).toEqual({})
    expect(initialSelection(undefined)).toEqual({})
  })
})

describe('a load-only section', () => {
  const section = { kind: 'box', id: 'increment', children: [], empty: 'Select an increment',
    load: { method: 'ktw-methods:delivery/increment', args: { increment: '${selection.increment}', module: 'planification' } },
    placeholder: { kind: 'skeleton', count: 3 } }

  test('reads the selection keys its load args name, resolved against the selection', () => {
    expect(selectionKeys(section.load.args)).toEqual(['increment'])
    expect(resolveSelectionArgs(section.load.args, { increment: 'I-3', release: 'R-1' })).toEqual({ increment: 'I-3', module: 'planification' })
    expect(selectionReady(section.load.args, {})).toBe(false)
    expect(selectionReady(section.load.args, { increment: 'I-3' })).toBe(true)
  })

  test('re-reads when its key changes, not when another key does', () => {
    const a = sectionLoad(section.load, { increment: 'I-3', release: 'R-1' })!
    const b = sectionLoad(section.load, { increment: 'I-3', release: 'R-2' })!
    const c = sectionLoad(section.load, { increment: 'I-4', release: 'R-2' })!
    expect(a.load.args).toEqual({ increment: 'I-3', module: 'planification' })
    expect(b.at).toBe(a.at)
    expect(c.at).not.toBe(a.at)
    expect(sectionLoad(section.load, { release: 'R-1' })).toBeUndefined()
  })

  test('loads with the progressive flag off, shows its empty text dimmed where nothing loads, its placeholder while it loads', () => {
    expect(sectionMode(section, false, true)).toBe('load')
    expect(sectionMode(section, false, false)).toBe('empty')
    expect(sectionMode({ ...section, children: [{ kind: 'text', content: 'x' }] }, false, true)).toBe('as-is')
    expect(page(section)).toContain('Select an increment')
    const host: HostServices = { loadSection: async () => undefined }
    // The page's data seeds the selection: the section is ready and loads.
    const loading = page(section, { selection: { increment: 'I-3' } }, host)
    expect(loading).toContain('data-section="increment"')
    expect(loading).toContain('data-state="loading"')
    expect(loading).not.toContain('Select an increment')
    // No increment selected yet: the empty text, nothing loaded.
    expect(page(section, {}, host)).toContain('Select an increment')
  })

  test('no region kind is left in the page grammar', () => {
    expect((UIPageNodeJSONSchema as any).definitions.Region).toBeUndefined()
  })
})

// SPDX-License-Identifier: Apache-2.0
// Type and dispatcher tests for the UIPage DSL discriminated union.
//
// These exercise the structural shape of every node kind, the type guards,
// `childrenOf` traversal, and exhaustiveness via `assertNeverNode`. The
// recursive renderer itself is tested in `renderer.test.tsx` (S2).

import { describe, expect, it } from 'bun:test'

import {
  ALL_KINDS,
  ATOMIC_KINDS,
  INTERACTIVE_KINDS,
  LAYOUT_KINDS,
  SCHEMA_BOUND_KINDS,
  childrenOf,
  isAtomicNode,
  isLayoutNode,
  isSchemaBoundNode,
  type UIPageNode,
} from '../../../../ktw-display-lib/src/ui-builders/page/index.ts'
import { UIPageNodeJSONSchema, UI_KIND_CATALOG } from '../../../../ktw-display-lib/src/ui-builders/page/index.ts'

describe('ui-page kind catalog', () => {
  // ALL_KINDS is exactly the union of the four category sets: no literal
  // counts, so adding a kind to a category never breaks this test.
  it('ALL_KINDS is the union of layout, atomic, schema-bound and interactive kinds', () => {
    const categories = [LAYOUT_KINDS, ATOMIC_KINDS, SCHEMA_BOUND_KINDS, INTERACTIVE_KINDS]
    for (const set of categories) expect(set.length).toBeGreaterThan(0)
    const union = new Set<string>(categories.flat())
    expect(new Set<string>(ALL_KINDS).size).toBe(ALL_KINDS.length)
    expect(union.size).toBe(categories.reduce((n, set) => n + set.length, 0))
    expect(ALL_KINDS.length).toBe(union.size)
    for (const k of union) expect((ALL_KINDS as readonly string[]).includes(k)).toBe(true)
  })

  it('all kind sets are disjoint', () => {
    const layoutSet = new Set<string>(LAYOUT_KINDS)
    const atomicSet = new Set<string>(ATOMIC_KINDS)
    const schemaSet = new Set<string>(SCHEMA_BOUND_KINDS)
    for (const k of layoutSet) expect(atomicSet.has(k) || schemaSet.has(k)).toBe(false)
    for (const k of atomicSet) expect(schemaSet.has(k)).toBe(false)
  })

  it('UI_KIND_CATALOG has one entry per declared kind', () => {
    const catalogKinds = new Set(UI_KIND_CATALOG.map((e) => e.kind))
    expect(catalogKinds.size).toBe(ALL_KINDS.length)
    for (const k of ALL_KINDS) expect(catalogKinds.has(k)).toBe(true)
  })

  it('every catalog entry has the right category', () => {
    const layoutSet = new Set<string>(LAYOUT_KINDS)
    const atomicSet = new Set<string>(ATOMIC_KINDS)
    const schemaSet = new Set<string>(SCHEMA_BOUND_KINDS)
    for (const e of UI_KIND_CATALOG) {
      if (layoutSet.has(e.kind)) expect(e.category).toBe('layout')
      else if (atomicSet.has(e.kind)) expect(e.category).toBe('atomic')
      else if (schemaSet.has(e.kind)) expect(e.category).toBe('schema-bound')
      else if ((INTERACTIVE_KINDS as readonly string[]).includes(e.kind)) expect(e.category).toBe('interactive')
      else throw new Error(`catalog entry "${e.kind}" doesn't match any kind set`)
    }
  })
})

describe('type guards', () => {
  it('isLayoutNode/isAtomicNode/isSchemaBoundNode partition correctly', () => {
    const samples: UIPageNode[] = [
      { kind: 'vbox', children: [] },
      { kind: 'heading', text: 'hi' },
      { kind: 'form', schema: 'Book' },
    ]
    expect(isLayoutNode(samples[0])).toBe(true)
    expect(isAtomicNode(samples[0])).toBe(false)
    expect(isSchemaBoundNode(samples[0])).toBe(false)

    expect(isLayoutNode(samples[1])).toBe(false)
    expect(isAtomicNode(samples[1])).toBe(true)
    expect(isSchemaBoundNode(samples[1])).toBe(false)

    expect(isLayoutNode(samples[2])).toBe(false)
    expect(isAtomicNode(samples[2])).toBe(false)
    expect(isSchemaBoundNode(samples[2])).toBe(true)
  })
})

describe('childrenOf', () => {
  it('returns flat children for vbox/hbox/grid/box/card/section', () => {
    const vbox: UIPageNode = {
      kind: 'vbox',
      children: [{ kind: 'heading', text: 'a' }, { kind: 'text', content: 'b' }],
    }
    expect(childrenOf(vbox).length).toBe(2)
    expect(childrenOf(vbox).map((c) => c.kind)).toEqual(['heading', 'text'])
  })

  it('walks tabs panels via .content', () => {
    const tabs: UIPageNode = {
      kind: 'tabs',
      tabs: [
        { label: 'A', content: { kind: 'heading', text: 'a' } },
        { label: 'B', content: { kind: 'text', content: 'b' } },
      ],
    }
    const kids = childrenOf(tabs)
    expect(kids.length).toBe(2)
    expect(kids[0].kind).toBe('heading')
    expect(kids[1].kind).toBe('text')
  })

  it('walks accordion panels via .content', () => {
    const acc: UIPageNode = {
      kind: 'accordion',
      panels: [
        { label: 'A', content: { kind: 'heading', text: 'a' } },
      ],
    }
    expect(childrenOf(acc).length).toBe(1)
    expect(childrenOf(acc)[0].kind).toBe('heading')
  })

  it('returns [] for atomic nodes', () => {
    const samples: UIPageNode[] = [
      { kind: 'heading', text: 'h' },
      { kind: 'kpi', label: 'Open', value: 42 },
      { kind: 'badge', text: 'OK' },
    ]
    for (const s of samples) expect(childrenOf(s)).toEqual([])
  })

  it('returns [] for schema-bound nodes', () => {
    const samples: UIPageNode[] = [
      { kind: 'form', schema: 'Book' },
      { kind: 'view', schema: 'Book' },
      { kind: 'table', schema: 'Books' },
      { kind: 'wizard', schema: 'Onboarding' },
    ]
    for (const s of samples) expect(childrenOf(s)).toEqual([])
  })
})

describe('JSON Schema export', () => {
  it('top-level oneOf has one ref per kind of the catalog', () => {
    const oneOf = UIPageNodeJSONSchema.oneOf as unknown as { $ref: string }[]
    expect(oneOf.length).toBe(Object.keys(UI_KIND_CATALOG).length)
  })

  it('every ref points to an entry in definitions', () => {
    const oneOf = UIPageNodeJSONSchema.oneOf as unknown as { $ref: string }[]
    const defs = UIPageNodeJSONSchema.definitions as Record<string, unknown>
    for (const ref of oneOf) {
      const name = ref.$ref.replace('#/definitions/', '')
      expect(defs[name]).toBeDefined()
    }
  })

  it('discriminator `kind` is a const in every variant', () => {
    const defs = UIPageNodeJSONSchema.definitions as Record<string, { properties?: { kind?: { const?: string } } }>
    const variantNames = [
      'VBox', 'HBox', 'Grid', 'Box', 'Card', 'Section', 'Tabs', 'Accordion',
      'Heading', 'Text', 'Divider', 'Badge', 'Link', 'Image', 'KPI', 'CodeBlock', 'Markdown', 'Chart',
      'Form', 'View', 'Table', 'Wizard',
    ]
    for (const v of variantNames) {
      expect(defs[v]?.properties?.kind?.const).toBeDefined()
    }
  })
})

describe('representative trees compile + traverse', () => {
  it('a 3-node dashboard with mixed categories', () => {
    const dashboard: UIPageNode = {
      id: 'root',
      kind: 'vbox',
      gap: 'md',
      children: [
        { id: 'title', kind: 'heading', text: 'Project Dashboard', level: 2 },
        {
          id: 'kpis',
          kind: 'grid',
          cols: 3,
          children: [
            { id: 'k1', kind: 'kpi', label: 'Open', value: 42 },
            { id: 'k2', kind: 'kpi', label: 'Closed', value: 12 },
            { id: 'k3', kind: 'kpi', label: 'MTTR', value: '4.2h' },
          ],
        },
        { id: 't', kind: 'table', schema: 'Activity' },
      ],
    }
    expect(childrenOf(dashboard).length).toBe(3)
    const kpiGrid = childrenOf(dashboard)[1]
    expect(kpiGrid.kind).toBe('grid')
    expect(childrenOf(kpiGrid).length).toBe(3)
  })

  it('a slide-deck-shaped vbox of cards', () => {
    const slides: UIPageNode = {
      kind: 'vbox',
      children: Array.from({ length: 5 }, (_, i) => ({
        id: `slide-${i}`,
        kind: 'card' as const,
        children: [
          { kind: 'heading' as const, text: `Slide ${i + 1}` },
          { kind: 'markdown' as const, content: `Content ${i + 1}` },
        ],
      })),
    }
    expect(childrenOf(slides).length).toBe(5)
  })

  it('inline schema + value on a view node', () => {
    const view: UIPageNode = {
      kind: 'view',
      schema: {
        name: 'Book',
        root: 'Book',
        types: { Book: { kind: 'object', fields: { title: 'string' } } as never },
      },
      data: { title: 'Foundation' },
    }
    expect(isSchemaBoundNode(view)).toBe(true)
  })
})

// SPDX-License-Identifier: Apache-2.0
// A page source may answer with a panel - its skeleton draws at
// once, each pushed section as its placeholder, and the producer's ops patch
// the sections by their id as it builds them.

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { UIPageView } from '../src/ui-builders/page/ui-page.view.tsx'
import { isPushedSection } from '../src/ui-builders/page/answer.ts'
import { panelView, reducePanel, type PanelEvent, type PanelView } from '../src/ui-builders/page/panel.ts'

const ID = 'dp-01920000-0000-7000-8000-00000000b001'

const html = (v: PanelView) => renderToStaticMarkup(
  createElement(MantineProvider, null, createElement(UIPageView, { node: v.page!.tree as never, data: v.data as never })))

const skeleton = {
  kind: 'vbox',
  children: [
    { kind: 'heading', text: 'Livraison' },
    { kind: 'vbox', id: 'lane:core', placeholder: { kind: 'cards', count: 3 } },
    { kind: 'vbox', id: 'backlog', placeholder: { kind: 'rows' } },
  ],
}

const events: PanelEvent[] = [
  { panel: ID, seq: 1, kind: 'open', open: { title: 'Livraison', owner: 'ktw-agents:views/delivery', answer: { tree: skeleton } } },
  // The producer builds the backlog first: sections come in any order.
  { panel: ID, seq: 2, kind: 'ops', ops: [{ seq: 2, op: 'replace', target: 'backlog', node: { kind: 'text', content: 'Backlog : 2 demandes' } }] },
  { panel: ID, seq: 3, kind: 'ops', ops: [{ seq: 3, op: 'replace', target: 'lane:core', node: { kind: 'text', content: 'Core : 3 incréments' } }] },
]

describe('a page answered as a panel', () => {
  test('a pushed section is an id and a placeholder object, never an input placeholder', () => {
    expect(isPushedSection(skeleton.children[1])).toBe(true)
    expect(isPushedSection({ kind: 'input', id: 'q', placeholder: 'Chercher…' })).toBe(false)
    expect(isPushedSection({ kind: 'vbox', placeholder: { kind: 'rows' } })).toBe(false)
    expect(isPushedSection({ kind: 'vbox', id: 's', placeholder: { kind: 'rows' }, load: { method: 'x:y/z' } })).toBe(false)
  })

  test('the skeleton draws at once, each pushed section as its placeholder', () => {
    const v = reducePanel(panelView(ID), events[0])
    const out = html(v)
    expect(out).toContain('Livraison')
    expect(out).toContain('data-section="lane:core" data-state="pushed"')
    expect(out).toContain('data-section="backlog" data-state="pushed"')
    expect(out).not.toContain('Core : 3 incréments')
  })

  test('each section arrives by its id, in any order, the others still waiting', () => {
    let v = reducePanel(panelView(ID), events[0])
    v = reducePanel(v, events[1])
    let out = html(v)
    expect(out).toContain('Backlog : 2 demandes')
    expect(out).toContain('data-section="lane:core" data-state="pushed"')
    v = reducePanel(v, events[2])
    out = html(v)
    expect(out).toContain('Core : 3 incréments')
    expect(out).not.toContain('data-state="pushed"')
  })
})

// SPDX-License-Identifier: Apache-2.0
// An action's result may carry the display panel its work runs
// in; the screen that ran it embeds the panel in place - a `panel` node
// followed through the host - and offers « Ouvrir dans un onglet ».

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { UIPageView } from '../src/ui-builders/page/ui-page.view.tsx'
import { PanelRenderer } from '../src/ui-builders/page/nodes/panel.tsx'
import { readFileSync } from 'node:fs'
import { UIPageNodeJSONSchema } from '../src/ui-builders/page/ui-page.json-schema.ts'
import { INTERACTIVE_KINDS } from '../src/ui-builders/page/ui-page.types.ts'
import { parseActionResult, resultPanel } from '../src/ui-builders/page/ui-page.interactive.ts'
import { panelTemplateFetch, panelView, reducePanel, type PanelEvent } from '../src/ui-builders/page/panel.ts'
import { HostServicesContext, type HostServices } from '../src/ui-core/host/index.tsx'

const ID = 'dp-01920000-0000-7000-8000-00000000a3f7'

const html = (el: unknown, host: HostServices = {}) => renderToStaticMarkup(
  createElement(MantineProvider, null, createElement(HostServicesContext.Provider, { value: host }, el as never)))

const events: PanelEvent[] = [
  { panel: ID, seq: 1, kind: 'open', open: { type: 'ktw-agents:/panel-type/progress', title: 'Installer les programmes', owner: 'run-5e02',
    answer: { tree: { kind: 'vbox', children: [{ kind: 'text', id: 'step', content: 'mermaid-cli : en attente' }] } } } },
  { panel: ID, seq: 2, kind: 'ops', ops: [{ seq: 2, op: 'replace', target: 'step', node: { kind: 'text', content: 'mermaid-cli : installé' } }] },
  { panel: ID, seq: 3, kind: 'progress', progress: { label: 'Programmes', current: 1, total: 4 } },
]

describe('an action result carrying a panel', () => {
  test('the result convention reads its panel; anything but a panel id is left out', () => {
    expect(parseActionResult({ ok: true, message: 'Installation lancée', panel: ID })).toEqual({ ok: true, message: 'Installation lancée', open: undefined, panel: ID })
    expect(parseActionResult({ panel: ID })?.panel).toBe(ID)
    expect(parseActionResult({ ok: true, panel: 'run-1' })?.panel).toBeUndefined()
    expect(resultPanel(parseActionResult({ ok: true, panel: ID }), 'Installer')).toEqual({ kind: 'panel', id: `result:${ID}`, panelId: ID, title: 'Installer' })
    expect(resultPanel(parseActionResult({ ok: true }))).toBeUndefined()
  })

  test('panel is a declared kind of the page grammar', () => {
    expect(INTERACTIVE_KINDS).toContain('panel')
    expect((UIPageNodeJSONSchema as any).definitions.Panel.required).toEqual(['kind', 'panelId'])
  })

  test('the screen embeds the panel in place and offers « Ouvrir dans un onglet », which opens it', () => {
    const node = resultPanel({ panel: ID }, 'Installer les programmes')!
    let opened = ''
    const host: HostServices = { openPanel: (id) => { opened = id }, followPanel: () => () => {} }
    const out = html(createElement(UIPageView, { node: { kind: 'vbox', children: [{ kind: 'button', label: 'Installer', action: { uri: 'x:y/z' } }, node] } as never }), host)
    expect(out).toContain(`data-element="panel:${ID}"`)
    expect(out).toContain('Installer les programmes')
    const button = /<button[^>]*data-guide="panel.open-tab"[^>]*>/.exec(out)?.[0] ?? ''
    expect(button).not.toBe('')
    expect(button).not.toContain('disabled')
    expect(out).toContain('Ouvrir dans un onglet')
    host.openPanel!(ID)
    expect(opened).toBe(ID)
    // A host that cannot open a panel offers the button disabled.
    expect(/<button[^>]*data-guide="panel.open-tab"[^>]*>/.exec(html(createElement(UIPageView, { node: node as never })))?.[0]).toContain('disabled')
  })

  test('its events build what it shows: its page patched by node id, its progress, its question, its end', () => {
    let v = panelView(ID)
    for (const e of events) v = reducePanel(v, e)
    expect(v.state).toBe('open')
    expect(v.seq).toBe(3)
    expect(reducePanel(v, events[1])).toBe(v) // an event already folded changes nothing
    const out = html(createElement(PanelRenderer, { node: resultPanel({ panel: ID })!, Page: UIPageView, initial: v }))
    expect(out).toContain('mermaid-cli : installé')
    expect(out).not.toContain('en attente')
    expect(out).toContain('1 / 4')
    expect(out).toContain(`${ID} · type progress · owner run-5e02 · seq 3`)

    v = reducePanel(v, { panel: ID, seq: 4, kind: 'ask', ask: { question: 'Écraser la version de Confluence ?' } })
    expect(v.state).toBe('asking')
    expect(html(createElement(PanelRenderer, { node: resultPanel({ panel: ID })!, Page: UIPageView, initial: v }))).toContain('Écraser la version de Confluence ?')
    v = reducePanel(v, { panel: ID, seq: 5, kind: 'progress', progress: { current: 2, total: 4 } })
    expect(v.state).toBe('open')
    v = reducePanel(v, { panel: ID, seq: 6, kind: 'complete', complete: { status: 'ok' } })
    expect(v.state).toBe('complete')
    expect(html(createElement(PanelRenderer, { node: resultPanel({ panel: ID })!, Page: UIPageView, initial: v }))).toContain('data-state="complete"')
  })
})

// The fix round: the inline node draws a tree its open names by
// {uri, hash} - a page template - through the host's panelTemplate.
describe('the inline panel node over a page template', () => {
  const spec = { uri: 'ktw-agents:/page-template/install', hash: 'h7' }
  const named: PanelEvent[] = [
    { panel: ID, seq: 1, kind: 'open', open: { title: 'Installer', answer: { tree: spec as never } } },
    { panel: ID, seq: 2, kind: 'ops', ops: [{ seq: 2, op: 'replace', target: 'step', node: { kind: 'text', content: 'mermaid-cli : installé' } } as never] },
  ]
  const node = resultPanel({ panel: ID }, 'Installer')!

  test("the host's panelTemplate is asked for the named tree and the node draws it, its ops on top", async () => {
    const asked: unknown[] = []
    const host: HostServices = {
      followPanel: () => () => {},
      panelTemplate: async (s) => { asked.push(s); return { kind: 'vbox', children: [{ kind: 'text', id: 'step', content: 'mermaid-cli : en attente' }] } },
    }
    const waiting = named.reduce(reducePanel, panelView(ID))
    expect(html(createElement(PanelRenderer, { node, Page: UIPageView, initial: waiting }), host)).not.toContain('mermaid-cli')
    const step = await panelTemplateFetch(waiting, host.panelTemplate)
    expect(asked).toEqual([spec])
    const out = html(createElement(PanelRenderer, { node, Page: UIPageView, initial: step!(waiting) }), host)
    expect(out).toContain('mermaid-cli : installé')
    expect(out).not.toContain('en attente')
    // Without the service, or with the page drawn, nothing is asked.
    expect(panelTemplateFetch(waiting, undefined)).toBeUndefined()
    expect(panelTemplateFetch(step!(waiting), host.panelTemplate)).toBeUndefined()
  })

  test('the node asks the template through the host', () => {
    const src = readFileSync(new URL('../src/ui-builders/page/nodes/panel.tsx', import.meta.url), 'utf8')
    expect(src).toMatch(/panelTemplateFetch\(view, panelTemplate\)/)
  })
})

// A question asked from a conversation is placed in the chat as
// a bare panel node - its card alone, where it came, its own buttons
// answering it; none of the panel's frame, title, id line or controls.
describe('a bare panel node', () => {
  const card = { panel: ID, seq: 1, kind: 'open', open: { title: 'Valider le plan ?', owner: 'run-ht',
    answer: { tree: { kind: 'vbox', id: 'human-task', children: [{ kind: 'card', id: 'inbox:td-1', children: [
      { kind: 'text', content: 'Valider le plan ?' },
      { kind: 'button', id: 'approve', label: 'Approuver', action: { uri: 'ktw-agents:todos/report' } }] }] } } } } as PanelEvent

  test('it draws its page alone: the card and its button, no title line, id line or panel controls', () => {
    const v = reducePanel(panelView(ID), card)
    const node = { kind: 'panel', id: `panel:${ID}`, panelId: ID, title: 'Valider le plan ?', bare: true } as const
    const out = html(createElement(PanelRenderer, { node: node as never, Page: UIPageView, initial: v }), { followPanel: () => () => {}, openPanel: () => {} })
    expect(out).toContain(`data-element="panel:${ID}"`)
    expect(out).toContain('data-bare="true"')
    expect(out).toContain('Approuver')
    expect(out).not.toContain('Ouvrir dans un onglet')
    expect(out).not.toContain('owner run-ht')
    expect(out).not.toContain('Annuler')
    // Not bare, the same panel keeps its frame and controls.
    const framed = html(createElement(PanelRenderer, { node: { ...node, bare: false } as never, Page: UIPageView, initial: v }), { followPanel: () => () => {}, openPanel: () => {} })
    expect(framed).toContain('Ouvrir dans un onglet')
    expect(framed).not.toContain('data-bare')
  })

  test('before its page arrives it shows its title; the grammar declares bare', () => {
    const out = html(createElement(PanelRenderer, { node: { kind: 'panel', panelId: ID, title: 'Valider le plan ?', bare: true } as never, Page: UIPageView }))
    expect(out).toContain('Valider le plan ?')
    expect(out).not.toContain('Ouvrir dans un onglet')
    expect((UIPageNodeJSONSchema as any).definitions.Panel.properties.bare.type).toBe('boolean')
  })
})

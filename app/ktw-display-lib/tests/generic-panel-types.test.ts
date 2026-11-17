// SPDX-License-Identifier: Apache-2.0
// KaTyoti ships generic panel types any task uses - steps with
// their state and a bar, a log that follows the work, an inline question
// whose answer goes back to the task. Each test drives a type by its panel
// events: the open carrying the type's tree (the fixture Go's
// internal/core/panel holds equal to its GenericTypes), then the ops on the
// nodes it names, the ask, the end.

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import trees from './fixtures/generic-panel-types.json'
import { UIPageView } from '../src/ui-builders/page/ui-page.view.tsx'
import { PanelRenderer } from '../src/ui-builders/page/nodes/panel.tsx'
import { applyDataBinding } from '../src/ui-builders/page/nodes/data-binding.ts'
import { UIPageNodeJSONSchema } from '../src/ui-builders/page/ui-page.json-schema.ts'
import { ATOMIC_KINDS } from '../src/ui-builders/page/ui-page.types.ts'
import { progressEnded } from '../src/ui-builders/page/ui-page.interactive.ts'
import { logRows, progressCount, progressPercent } from '../src/ui-builders/page/progress-log.ts'
import { panelAnswer, panelAskForm, panelCancel, panelView, reducePanel, withPanelTemplate, type PanelEvent, type PanelView } from '../src/ui-builders/page/panel.ts'
import { HostServicesContext, type HostServices } from '../src/ui-core/host/index.tsx'

const ID = 'dp-01920000-0000-7000-8000-0000000a3f70'
const TREES = trees as Record<string, unknown>

const html = (el: unknown, host: HostServices = {}) => renderToStaticMarkup(
  createElement(MantineProvider, null, createElement(HostServicesContext.Provider, { value: host }, el as never)))
const panel = (v: PanelView, host: HostServices = {}) =>
  html(createElement(PanelRenderer, { node: { kind: 'panel', panelId: ID } as never, Page: UIPageView, initial: v }), host)
const fold = (events: PanelEvent[]) => events.reduce(reducePanel, panelView(ID))
const open = (type: string, title: string): PanelEvent => ({ panel: ID, seq: 1, kind: 'open', open: { type, title, owner: 'run-91c4', answer: { tree: TREES[type] as never } } })
const sender = () => {
  const sent: unknown[] = []
  return { sent, host: { followPanel: () => () => {}, sendPanelInput: async (i: unknown) => { sent.push(i) } } as HostServices }
}

const pages = (states: string[]) => ['Vue d’ensemble', 'Contexte', 'Applications', 'Flux', 'Décisions']
  .map((label, i) => ({ id: `p${i + 1}`, label, state: states[i] }))

describe('the generic panel types', () => {
  test('KaTyoti ships progress, log, question and chat, each with its tree', () => {
    expect(Object.keys(TREES).sort()).toEqual([
      'ktw-agents:/panel-type/chat', 'ktw-agents:/panel-type/log', 'ktw-agents:/panel-type/progress', 'ktw-agents:/panel-type/question',
    ])
  })

  test('progress: the steps pushed by ops, their state in words under one bar, its question inline, its end', () => {
    const type = 'ktw-agents:/panel-type/progress'
    let v = fold([
      open(type, 'Publication Confluence · dossier cosmos'),
      { panel: ID, seq: 2, kind: 'ops', ops: [{ seq: 2, op: 'merge', target: 'steps', value: { unit: 'pages', stages: pages(['waiting', 'waiting', 'waiting', 'waiting', 'waiting']) } }] },
      { panel: ID, seq: 3, kind: 'ops', ops: [{ seq: 3, op: 'merge', target: 'steps', value: { stages: pages(['done', 'done', 'running', 'waiting', 'skipped']) } }] },
    ])
    let out = panel(v)
    expect(out).toContain('data-layout="rows"')
    expect(out).toContain('Publication Confluence · dossier cosmos')
    expect(out).toContain('3 / 5 pages') // done and skipped are ended
    expect(out).toContain('terminé')
    expect(out).toContain('en cours…')
    expect(out).toContain('en attente')
    expect(out).toContain('ignoré')
    expect(out).toContain('data-guide-name="p3" data-state="running"')

    v = reducePanel(v, { panel: ID, seq: 4, kind: 'ask', ask: {
      question: '2 pages ont changé des deux côtés depuis la dernière publication. Que faire ?',
      actions: [{ name: 'overwrite', label: 'Écraser Confluence' }, { name: 'keep', label: 'Garder la version de Confluence' }, { name: 'skip', label: 'Ignorer ces pages' }],
    } })
    expect(v.state).toBe('asking')
    const { sent, host } = sender()
    out = panel(v, host)
    expect(out).toContain('data-guide="panel.ask"')
    expect(out).toContain('Écraser Confluence')
    expect(out).toContain('Ignorer ces pages')
    const answer = /<button[^>]*data-guide="panel.answer"[^>]*>/.exec(out)?.[0] ?? ''
    expect(answer).not.toBe('')
    expect(answer).not.toContain('disabled')
    expect(out).toContain('data-guide="panel.cancel"')
    // A host that cannot send input offers the answer disabled.
    expect(/<button[^>]*data-guide="panel.answer"[^>]*>/.exec(panel(v))?.[0]).toContain('disabled')

    // The answer goes back to the task: the action chosen, then a cancel.
    void host.sendPanelInput!(panelAnswer(v, { action: 'keep' }, 'jdoe')!)
    void host.sendPanelInput!(panelCancel(ID))
    expect(sent).toEqual([{ panel: ID, kind: 'action', name: 'keep', by: 'jdoe' }, { panel: ID, kind: 'cancel' }])
    expect(panelAnswer(v, { action: 'nope' })).toBeNull()

    v = fold([
      open(type, 'Publication'),
      { panel: ID, seq: 2, kind: 'ops', ops: [{ seq: 2, op: 'merge', target: 'steps', value: { stages: pages(['done', 'done', 'done', 'done', 'done']) } }] },
      { panel: ID, seq: 3, kind: 'complete', complete: { status: 'ok' } },
    ])
    out = panel(v, host)
    expect(out).toContain('5 / 5')
    expect(out).toContain('data-guide="panel.complete"')
    expect(out).not.toContain('data-guide="panel.cancel"')
  })

  test('log: the lines appended by ops, a line repeated in a row counted once, the last ones kept', () => {
    const v = fold([
      open('ktw-agents:/panel-type/log', 'Indexer les sources'),
      { panel: ID, seq: 2, kind: 'ops', ops: [{ seq: 2, op: 'append', target: 'log', items: ['lecture de 12 documents', 'en attente du serveur'] }] },
      { panel: ID, seq: 3, kind: 'ops', ops: [{ seq: 3, op: 'append', target: 'log', items: ['en attente du serveur', { text: 'LeanIX ne répond pas', level: 'error' }] }] },
    ])
    const out = panel(v)
    expect(out).toContain('data-element="log:log"')
    expect(out).toContain('lecture de 12 documents')
    expect(out).toContain('×2')
    expect(out).toContain('data-level="error"')
    expect(out.match(/en attente du serveur/g)?.length).toBe(1)

    const { rows, hidden } = logRows(['a', 'a', 'b', { text: 'b', level: 'warn' }, 'c'], 2)
    expect(hidden).toBe(2)
    expect(rows).toEqual([{ text: 'b', level: 'warn', at: undefined, count: 1 }, { text: 'c', level: 'info', at: undefined, count: 1 }])
    expect(logRows(undefined).rows).toEqual([])
  })

  test('question: an ask with no choices takes a text, its answer a submit; its form is the page form', () => {
    let v = fold([
      open('ktw-agents:/panel-type/question', 'Nommer la release'),
      { panel: ID, seq: 2, kind: 'ask', ask: { question: 'Quel nom pour la release ?' } },
    ])
    const { host } = sender()
    let out = panel(v, host)
    expect(out).toContain('Quel nom pour la release ?')
    expect(out).toContain('<textarea')
    expect(panelAnswer(v, { text: '  ArchiFactory 2026.11 ' })).toEqual({ panel: ID, kind: 'submit', value: 'ArchiFactory 2026.11' })
    expect(panelAnswer(v, { text: ' ' })).toBeNull()
    expect(panelAnswer(v, { value: { name: 'x' } })).toEqual({ panel: ID, kind: 'submit', value: { name: 'x' } })

    v = reducePanel(v, { panel: ID, seq: 3, kind: 'ask', ask: { question: 'Le dossier', form: { uri: 'ktw-test:/data-type/dossier' }, value: { name: 'cosmos' } } })
    out = panel(v, host)
    expect(out).not.toContain('<textarea')
    expect(out).not.toContain('data-guide="panel.answer"') // the form submits itself
    // The question's form is built once, here: the page's
    // form node over its data type by URI - or inline -, prefilled, its
    // submit the answer; none when the question offers choices or no form.
    expect(panelAskForm(v.ask)).toEqual({ kind: 'form', id: 'panel:ask-form', schema: 'ktw-test:/data-type/dossier', data: { name: 'cosmos' }, hideTitle: true, mode: 'edit', submit: { uri: 'panel:answer' } } as never)
    expect((panelAskForm({ form: { root: 'R', types: {} } }) as unknown as { schema: unknown }).schema).toEqual({ root: 'R', types: {} })
    expect(panelAskForm({ form: { uri: 'u' }, actions: [{ name: 'keep' }] })).toBeUndefined()
    expect(panelAskForm({ question: 'Why?' })).toBeUndefined()
    expect(panelAskForm(undefined)).toBeUndefined()
    // What the person sends next says the run goes on: the question is gone.
    v = reducePanel(v, { panel: ID, seq: 4, kind: 'ops', ops: [{ seq: 4, op: 'replace', target: 'question', node: { kind: 'text', content: 'Merci' } }] })
    expect(v.state).toBe('open')
    expect(panel(v, host)).not.toContain('data-guide="panel.ask"')
  })

  test('progress and log are declared atomic kinds of the page grammar, bound and drawn on their own', () => {
    expect(ATOMIC_KINDS).toContain('progress')
    expect(ATOMIC_KINDS).toContain('log')
    const defs = (UIPageNodeJSONSchema as any).definitions
    expect(defs.Progress.properties.kind.const).toBe('progress')
    expect(defs.Log.properties.kind.const).toBe('log')
    expect(defs.ProgressList.properties.layout.enum).toEqual(['cards', 'rows'])

    expect(progressPercent({ current: 2, total: 5 })).toBe(40)
    expect(progressPercent({ current: 9, total: 5 })).toBe(100)
    expect(progressPercent({ current: 2 })).toBe(0)
    expect(progressPercent({ state: 'done' })).toBe(100)
    expect(progressCount({ current: 2, total: 4, unit: 'programmes' })).toBe('2 / 4 programmes')
    expect(progressCount({ current: 2 })).toBe('')
    expect(progressEnded(pages(['done', 'skipped', 'failed', 'running', 'waiting']) as never)).toEqual({ ended: 2, total: 5 })

    const out = html(createElement(UIPageView, { node: { kind: 'vbox', children: [
      { kind: 'progress', id: 'bar', label: 'Programmes', current: 1, total: 4, unit: 'programmes' },
      { kind: 'log', id: 'lines', dataField: '$.lines' },
    ] } as never, data: { lines: ['ok', 'ok'] } }))
    expect(out).toContain('data-element="progress:bar"')
    expect(out).toContain('1 / 4 programmes')
    expect(out).toContain('×2')
    expect(applyDataBinding({ kind: 'progress', dataField: '$.n' } as never, { n: 3 })).toEqual({ kind: 'progress', dataField: '$.n', current: 3 } as never)
  })
})

// The fix round: a panel's tree named by {uri, hash} - a page
// template - waits in the view with the ops taken meanwhile, once each, and a
// new open starts its page again.
describe('a panel whose open names its tree by {uri, hash}', () => {
  const spec = { uri: 'ktw-agents:/page-template/review', hash: 'h1' }
  const named = (seq: number, s = spec): PanelEvent => ({ panel: ID, seq, kind: 'open', open: { title: 'Revue', answer: { tree: s as never } } })
  const op = (seq: number, target: string, text: string) => ({ seq, op: 'replace', target, node: { kind: 'text', id: target, text } }) as never
  const opsAt = (seq: number, ...ops: never[]): PanelEvent => ({ panel: ID, seq, kind: 'ops', ops })
  const template = { kind: 'vbox', children: [{ kind: 'text', id: 'a', text: 'a0' }, { kind: 'text', id: 'b', text: 'b0' }] }

  test('a second open naming a tree drops the page of the first', () => {
    const first: PanelEvent = { panel: ID, seq: 1, kind: 'open', open: { answer: { tree: { kind: 'text', id: 'x', text: 'old' } as never } } }
    const v = fold([first, { panel: ID, seq: 2, kind: 'ask', ask: { question: '?' } }, named(3)])
    expect(v.page).toBeUndefined()
    expect(v.treeSpec).toEqual(spec)
    expect(v.ask).toBeUndefined()
    expect(v.waiting).toEqual({ seq: 3, ops: [] })
    // An inline tree after a named one leaves no stale name behind.
    const back = reducePanel(v, { panel: ID, seq: 4, kind: 'open', open: { answer: { tree: { kind: 'text', id: 'y' } as never } } })
    expect(back.treeSpec).toBeUndefined()
    expect(back.waiting).toBeUndefined()
    expect(back.page?.seq).toBe(4)
  })

  test('the ops taken while the template is asked are kept once, a replay dropped', () => {
    const e2 = opsAt(2, op(2, 'a', 'a1'))
    const v = fold([named(1), e2, e2, opsAt(3, op(3, 'b', 'b1')), opsAt(2, op(2, 'a', 'again'))])
    expect(v.waiting?.ops.length).toBe(2)
  })

  test('withPanelTemplate lays the kept ops over the template in order, and refuses a stale template', () => {
    const v = fold([named(1), opsAt(2, op(2, 'a', 'a1')), opsAt(3, op(3, 'a', 'a2')), opsAt(4, op(4, 'b', 'b1'))])
    const drawn = withPanelTemplate(v, spec, template)
    const kids = (drawn.page?.tree as { children: Array<{ text: string }> }).children
    expect(kids.map((k) => k.text)).toEqual(['a2', 'b1'])
    expect(drawn.waiting).toBeUndefined()
    // A template of an earlier open, or a page already drawn, changes nothing.
    expect(withPanelTemplate(v, { uri: spec.uri, hash: 'h0' }, template)).toBe(v)
    expect(withPanelTemplate(v, { uri: 'ktw-agents:/page-template/other' }, template)).toBe(v)
    expect(withPanelTemplate(drawn, spec, { kind: 'text' })).toBe(drawn)
  })
})

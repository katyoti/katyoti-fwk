// SPDX-License-Identifier: Apache-2.0
// The interactive nodes (ui-page.interactive.ts): their pure helpers — the
// action templates, the tree nesting, the graph layering — and their render
// (SSR, as the other renderer tests): a list repeats ONE template per element
// with `$.item` bound, a tree nests rows under their parent, lanes put cards in
// their lane, a gantt places bars on its axis, a graph draws boxes in layers;
// an action with no host shows disabled.

import { describe, expect, it } from 'bun:test'
import { renderToString } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import {
  UIPageView, layerGraph, nestRows, resolveActionArgs, ganttSpan,
} from '../../../../ktw-display-lib/src/ui-builders/page/index.ts'
import type { UIPageNode } from '../../../../ktw-display-lib/src/ui-builders/page/index.ts'

function render(node: UIPageNode, data?: unknown, onAction?: () => void): string {
  return renderToString(
    <MantineProvider>
      <UIPageView node={node} data={data} onAction={onAction} />
    </MantineProvider>,
  )
}

describe('actions — resolveActionArgs', () => {
  it('fills item paths and scope names, keeping a whole-template value as is', () => {
    const got = resolveActionArgs(
      { id: '${item.id}', n: '${item.size}', note: 'move ${item.title} to ${lane}', by: { id: '${person}' }, list: ['${target}'] },
      { id: 'billing.RQ-1', size: 3, title: 'Invoices' },
      { lane: 'rel-1.4', person: 'christophe', target: 'billing' },
    )
    expect(got).toEqual({ id: 'billing.RQ-1', n: 3, note: 'move Invoices to rel-1.4', by: { id: 'christophe' }, list: ['billing'] })
  })

  it('resolves an unknown name to the empty string', () => {
    expect(resolveActionArgs({ a: '${nope}', b: 'x${item.missing}y' }, {}, {})).toEqual({ a: '', b: 'xy' })
  })
})

describe('tree — nestRows', () => {
  it('nests rows under their parent; an unknown parent is a root; depths follow', () => {
    const forest = nestRows([
      { id: 'a', title: 'A' }, { id: 'b', parent: 'a' }, { id: 'c', parent: 'b' }, { id: 'd', parent: 'gone' },
    ])
    expect(forest.map((r) => r.id)).toEqual(['a', 'd'])
    expect(forest[0].children[0].id).toBe('b')
    expect(forest[0].children[0].children[0].depth).toBe(2)
  })

  it('breaks a cycle instead of losing its rows', () => {
    const forest = nestRows([{ id: 'x', parent: 'y' }, { id: 'y', parent: 'x' }])
    const all = (rs: typeof forest): string[] => rs.flatMap((r) => [r.id, ...all(r.children)])
    expect(all(forest).sort()).toEqual(['x', 'y'])
  })
})

describe('graph — layerGraph', () => {
  it('puts each box one layer after the deepest box it comes from', () => {
    const { boxes, layers } = layerGraph(
      [{ id: 't', title: 'Template' }, { id: 'w', title: 'Write' }, { id: 'p', title: 'Page' }, { id: 'g', title: 'Gate' }],
      [{ from: 't', to: 'w' }, { from: 't', to: 'p' }, { from: 'w', to: 'g' }, { from: 'p', to: 'g' }, { from: 'x', to: 'g' }],
    )
    const at = Object.fromEntries(boxes.map((b) => [b.id, [b.layer, b.slot]]))
    expect(layers).toBe(3)
    expect(at).toEqual({ t: [0, 0], w: [1, 0], p: [1, 1], g: [2, 0] })
  })
})

describe('list', () => {
  it('repeats its template once per element, $.item bound, $ the page data', () => {
    const html = render(
      { kind: 'list', dataField: '$.goals', item: { kind: 'heading', text: '', dataField: '$.item.title', level: 4 } },
      { goals: [{ id: 'g1', title: 'Minutes for every meeting' }, { id: 'g2', title: 'Decisions are findable' }] },
    )
    expect(html).toContain('Minutes for every meeting')
    expect(html).toContain('Decisions are findable')
    expect((html.match(/<h4/g) ?? []).length).toBe(2)
  })

  it('says so when empty', () => {
    expect(render({ kind: 'list', data: [], empty: 'No goals yet.', item: { kind: 'text', content: 'x' } })).toContain('No goals yet.')
  })
})

describe('button and actions', () => {
  it('a button with no host is disabled; with one it is live', () => {
    const node: UIPageNode = { kind: 'button', label: 'Tick now', action: { uri: 'ktw-agents:planner/tick' } }
    expect(render(node)).toMatch(/disabled/)
    expect(render(node, undefined, () => {})).not.toMatch(/disabled=""/)
  })
})

describe('tree', () => {
  it('nests the rows, the first level open, with their badge', () => {
    const html = render({
      kind: 'tree', badgeField: 'state', badgeColors: { split: 'violet' },
      data: [
        { id: 'r', title: 'Review the meeting module', state: 'split' },
        { id: 'r.m', parent: 'r', title: 'meeting — the module page', state: 'running' },
      ],
    })
    expect(html).toContain('Review the meeting module')
    expect(html).toContain('meeting — the module page')
    expect(html).toContain('split')
    expect(html).toContain('Collapse Review the meeting module')
  })
})

describe('lanes', () => {
  it('puts each card in its lane with the lane count', () => {
    const html = render({
      kind: 'lanes',
      lanes: [{ id: 'backlog', title: 'Backlog' }, { id: '1.4', title: 'project 1.4', meta: 'Due 15/10' }],
      data: [{ id: 'a', title: 'Minutes in French', lane: 'backlog' }, { id: 'b', title: 'Write minutes', lane: '1.4' }],
    })
    expect(html).toContain('Backlog')
    expect(html).toContain('Due 15/10')
    expect(html.indexOf('Minutes in French')).toBeLessThan(html.indexOf('project 1.4'))
    expect(html.indexOf('Write minutes')).toBeGreaterThan(html.indexOf('project 1.4'))
  })
})

describe('gauge, gantt, graph', () => {
  it('a gauge shows value / max and its hint', () => {
    const html = render({ kind: 'gauge', label: 'planning', value: 1, max: 4, hint: '{value} of {max} running' })
    expect(html).toContain('1 / 4')
    expect(html).toContain('1 of 4 running')
  })

  it('a gantt places each bar by its start over the span', () => {
    const tasks = [{ id: 'a', title: 'Template', start: 0, length: 2 }, { id: 'b', title: 'Write', start: 2, length: 6, critical: true }]
    expect(ganttSpan(tasks)).toBe(8)
    const html = render({ kind: 'gantt', unit: 'h', data: tasks })
    expect(html).toContain('left:25%')
    expect(html).toContain('width:75%')
    expect(html).toContain('8 h')
  })

  it('a graph draws its boxes and edges', () => {
    const html = render({
      kind: 'graph',
      data: { nodes: [{ id: 'a', title: 'Template', meta: 'slack 0', critical: true }, { id: 'b', title: 'Write minutes' }], edges: [{ from: 'a', to: 'b' }] },
    })
    expect(html).toContain('Template')
    expect(html).toContain('slack 0')
    expect((html.match(/<rect/g) ?? []).length).toBe(2)
    expect(html).toContain('<path d="M')
  })
})

describe('pulse and host overrides', () => {
  it('an element whose tone value is in pulse animates; the others do not', () => {
    const html = render({
      kind: 'list', toneField: 'state', toneColors: { running: 'blue', ready: 'teal' }, pulse: ['running'],
      data: [{ id: 'a', title: 'Runs now', state: 'running' }, { id: 'b', title: 'Waits', state: 'ready' }],
      item: { kind: 'text', content: '', dataField: '$.item.title' },
    })
    expect(html).toContain('@keyframes ktw-pulse')
    expect((html.match(/animation:ktw-pulse/g) ?? []).length).toBe(1)
  })

  it('a registered host renderer replaces the built-in one for its kind', async () => {
    const { registerPageNodeRenderer } = await import('../../../../ktw-display-lib/src/ui-builders/page/index.ts')
    registerPageNodeRenderer('gauge', ({ node }) => <span>{`host gauge ${(node as { label: string }).label}`}</span>)
    try {
      expect(render({ kind: 'gauge', label: 'planning', value: 1 })).toContain('host gauge planning')
    } finally {
      registerPageNodeRenderer('gauge', undefined as never)
    }
  })
})

describe('action verbs — uri | navigate | chat | create', () => {
  it('names the one verb an action has; none or two is no verb', async () => {
    const { actionVerb } = await import('../../../../ktw-display-lib/src/ui-builders/page/index.ts')
    expect(actionVerb({ uri: 'a:b/c' })).toBe('uri')
    expect(actionVerb({ navigate: { object: 'm.x' } })).toBe('navigate')
    expect(actionVerb({ chat: { task: 'chat' } })).toBe('chat')
    expect(actionVerb({ create: {} })).toBe('create')
    expect(actionVerb({ label: 'nothing' })).toBeUndefined()
    expect(actionVerb({ uri: 'a:b/c', navigate: { object: 'm.x' } })).toBeUndefined()
  })

  it('resolves the templates of the verb fields, not only of args', async () => {
    const { resolveAction } = await import('../../../../ktw-display-lib/src/ui-builders/page/index.ts')
    const got = resolveAction(
      { navigate: { object: '${item.id}', panel: 'dashboard' }, chat: { task: 'chat', target: '${id}' }, args: { v: '${value.title}' } },
      { id: 'billing.f-1' }, { id: 'billing', value: { title: 'Invoices' } },
    )
    expect(got.navigate).toEqual({ object: 'billing.f-1', panel: 'dashboard' })
    expect(got.chat).toEqual({ task: 'chat', target: 'billing' })
    expect(got.args).toEqual({ v: 'Invoices' })
  })

  it('reads the result convention {ok, message, open?}; anything else is not one', async () => {
    const { parseActionResult, navigateTarget } = await import('../../../../ktw-display-lib/src/ui-builders/page/index.ts')
    expect(parseActionResult('{"ok":false,"message":"No release open"}')).toEqual({ ok: false, message: 'No release open', open: undefined })
    expect(parseActionResult({ ok: true, open: 'billing.rq-1#dashboard' })).toEqual({ ok: true, message: undefined, open: { object: 'billing.rq-1', panel: 'dashboard' } })
    expect(parseActionResult({ message: 'Filed', open: { object: 'm.x' } })?.open).toEqual({ object: 'm.x' })
    expect(parseActionResult('{"runs":3}')).toBeUndefined()
    expect(parseActionResult('not json')).toBeUndefined()
    expect(parseActionResult({ ok: 'yes' })).toBeUndefined()
    expect(navigateTarget('m.x#')).toEqual({ object: 'm.x', panel: undefined })
  })

  it('a link or a badge with an action runs it through the host; without a host the link is text', () => {
    const link = { kind: 'link', text: 'Open the feature', action: { navigate: { object: 'm.f' } } } as UIPageNode
    const live = render(link, undefined, () => {})
    expect(live).toContain('<button')
    expect(live).toContain('Open the feature')
    expect(live).not.toContain('href=')
    const dead = render(link)
    expect(dead).not.toContain('<button')
    expect(dead).toContain('Runs in a panel')
    const badge = render({ kind: 'badge', text: 'blocked', action: { navigate: { object: 'm.f' } } } as UIPageNode, undefined, () => {})
    expect(badge).toContain('<button')
    // An href link stays an anchor.
    expect(render({ kind: 'link', text: 'Docs', href: 'https://docs.example.com' })).toContain('href="https://docs.example.com"')
  })

  it('an action with no verb shows disabled even with a host', () => {
    const html = render({ kind: 'button', label: 'Go', action: { label: 'Go' } } as UIPageNode, undefined, () => {})
    expect(html).toContain('disabled')
  })
})

describe('form → action', () => {
  it('a submit reads the value through ${value.*}; a method with no args takes the whole value, untemplated', async () => {
    const { resolveAction } = await import('../../../../ktw-display-lib/src/ui-builders/page/index.ts')
    const value = { title: 'Invoices ${id}', lines: [{ label: 'a' }] }
    const templated = resolveAction({ uri: 'x:y/file', args: { title: '${value.title}', module: '${id}' } }, undefined, { id: 'billing', value })
    expect(templated.args).toEqual({ title: 'Invoices ${id}', module: 'billing' })
    const whole = resolveAction({ uri: 'x:y/file' }, undefined, { id: 'billing', value })
    expect(whole.args).toEqual(value)
    // Not a form (no value): no args stays none.
    expect(resolveAction({ uri: 'x:y/tick' }, undefined, { id: 'billing' }).args).toBeUndefined()
  })
})

// A use-case chip that opens its object is a badge with an action: it shows
// the pointing hand, as a link does (it kept the badge's arrow).
describe('badge with an action', () => {
  const chip = { kind: 'badge', text: 'UC-001', action: { navigate: { object: 'm.UC-001' } } } as unknown as UIPageNode
  it('points when a host runs it', () => {
    expect(render(chip, undefined, () => {})).toMatch(/cursor:\s*pointer/)
  })
  it('keeps the arrow when nothing runs it', () => {
    expect(render({ kind: 'badge', text: 'UC-001' } as unknown as UIPageNode, undefined, () => {})).not.toMatch(/cursor:\s*pointer/)
  })
})

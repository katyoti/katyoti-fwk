// SPDX-License-Identifier: Apache-2.0
// A holder's children tab draws its children as the one
// object card, filled from the rows the pane resolved, a
// click opening the child; a « ⋯ » menu opens or retires it; the breadcrumb
// above the opened child leads back to the holder on the tab it showed.

import { describe, expect, test } from 'bun:test'
import { childCardsPage } from '../src/sections/workspace/childCards'
import { crumbPayload, crumbsOf, type BackablePayload } from '../src/sections/workspace/objectTabHistory'

const texts = { open: 'Ouvrir', retire: 'Retirer', retireConfirm: 'Retirer ?', empty: 'Rien ici.' }

describe('the children as object cards', () => {
  test('one card per child, filled from its row, in the pane order', () => {
    const page = childCardsPage({
      type: 'feature', icon: 'sparkles',
      objects: [{ id: 'm.F-2', title: 'Second' }, { id: 'm.F-1' }],
      rows: [{ id: 'm.F-1', state: { id: 'lifecycle.state', label: 'realized', color: 'green' }, badges: [{ id: 'plan.priority', label: 'P1' }, { label: '' }] }],
    }, 'p1', texts) as unknown as { kind: string; layout: string; empty: string; data: Array<Record<string, any>>; item: Record<string, any> }
    expect(page.kind).toBe('list')
    expect(page.layout).toBe('grid')
    expect(page.empty).toBe('Rien ici.')
    expect(page.item).toEqual({ kind: 'object-card', dataField: '$.item', open: { navigate: { object: '${item.id}' } } })
    expect(page.data.map((r) => r.id)).toEqual(['m.F-2', 'm.F-1'])
    const [second, first] = page.data
    expect(second.title).toBe('Second')
    expect(second.state).toBeUndefined()
    expect(first.title).toBe('m.F-1')
    expect(first.type).toBe('feature')
    expect(first.icon).toBe('sparkles')
    expect(first.state).toEqual({ id: 'lifecycle.state', label: 'realized', color: 'green' })
    expect(first.badges).toEqual([{ id: 'plan.priority', label: 'P1' }])
    expect(first.project).toBe('p1')
    expect(first.ref).toBe('p1:m.F-1')
    expect(first.menu.map((e: { id: string }) => e.id)).toEqual(['open', 'retire'])
    const retire = first.menu[1]
    expect(retire.confirm).toBe('Retirer ?')
    expect(retire.action).toEqual({ uri: 'ktw-agents:cwm/object-archive', args: { id: '${item.ref}' } })
  })

  test('no child: an empty grid with its sentence', () => {
    const page = childCardsPage({ type: 'goal' }, undefined, texts) as unknown as { data: unknown[]; empty: string }
    expect(page.data).toEqual([])
    const one = childCardsPage({ type: 'goal', objects: [{ id: 'm.G-1' }] }, undefined, texts) as unknown as { data: Array<Record<string, unknown>> }
    expect(one.data[0].ref).toBe('m.G-1')
    expect(one.data[0].project).toBeUndefined()
  })
})

describe('the breadcrumb back to the holder', () => {
  const opened: BackablePayload = {
    objectId: 'm.F-1', objectTitle: 'Child',
    back: [
      { from: 'home' },
      { from: 'object', label: 'Module', payload: { objectId: 'm', objectTitle: 'Module', activeKind: 'tab:children/goal' } },
      { from: 'object', label: 'Goal', payload: { objectId: 'm.G-1', activeKind: 'tab:children/feature' } },
    ],
  }
  test('lists the holders the object was opened through, oldest first', () => {
    expect(crumbsOf(opened)).toEqual([
      { objectId: 'm', title: 'Module', activeKind: 'tab:children/goal', step: 1 },
      { objectId: 'm.G-1', title: 'Goal', activeKind: 'tab:children/feature', step: 2 },
    ])
  })
  test('stops at a step that came from another tab, and is empty without one', () => {
    expect(crumbsOf({ objectId: 'x', back: [{ from: 'object', payload: { objectId: 'a' } }, { from: 'inbox' }] })).toEqual([])
    expect(crumbsOf({ objectId: 'x' })).toEqual([])
    expect(crumbsOf(undefined)).toEqual([])
  })
  test('a crumb restores its holder on its tab, the history cut there', () => {
    expect(crumbPayload(opened, 1)).toEqual({ objectId: 'm', objectTitle: 'Module', activeKind: 'tab:children/goal', back: [{ from: 'home' }] })
    expect(crumbPayload(opened, 0)).toBeUndefined()
  })
})

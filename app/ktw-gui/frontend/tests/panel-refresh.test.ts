// SPDX-License-Identifier: Apache-2.0
// Which CWM object events re-read a shown panel: its declared refresh scope
// (and types) against the object the event is about, as the GUI forwards it.

import { describe, expect, it } from 'bun:test'

import { moduleOfId, panelRefreshMatches, type WorkspaceChangedEvent } from '../src/hooks/usePanelRefresh'

const ev = (o: Partial<WorkspaceChangedEvent>): WorkspaceChangedEvent => ({
  universe: 'u', subject: 'ktw.t.universe.u.workspace.w.object.x.updated', action: 'updated', ...o,
})
const onModule = { universe: 'u', object: 'billing', module: 'billing' }
const onUC = { universe: 'u', object: 'billing.uc-1', module: 'billing' }

describe('panelRefreshMatches', () => {
  it('never re-reads with no refresh or scope none, or for another universe', () => {
    const e = ev({ object: 'billing.uc-1', module: 'billing' })
    expect(panelRefreshMatches(undefined, e, onModule)).toBe(false)
    expect(panelRefreshMatches({ scope: 'none' }, e, onModule)).toBe(false)
    expect(panelRefreshMatches({ scope: 'universe' }, { ...e, universe: 'v' }, onModule)).toBe(false)
  })

  it('module: an object of the module, or one that left it', () => {
    const r = { scope: 'module' }
    expect(panelRefreshMatches(r, ev({ object: 'billing.uc-1', module: 'billing' }), onModule)).toBe(true)
    expect(panelRefreshMatches(r, ev({ object: 'pay.uc-1', module: 'pay' }), onModule)).toBe(false)
    expect(panelRefreshMatches(r, ev({ object: 'pay.uc-1', module: 'pay', fromModule: 'billing', action: 'moved' }), onModule)).toBe(true)
  })

  it('self: the panel object only', () => {
    const r = { scope: 'self' }
    expect(panelRefreshMatches(r, ev({ object: 'billing.uc-1', module: 'billing' }), onUC)).toBe(true)
    expect(panelRefreshMatches(r, ev({ object: 'billing.uc-2', module: 'billing' }), onUC)).toBe(false)
  })

  it('children: the object, an object directly under it, or one moved away from it', () => {
    const r = { scope: 'children' }
    expect(panelRefreshMatches(r, ev({ object: 'billing.g-1', module: 'billing', parent: 'billing' }), onModule)).toBe(true)
    expect(panelRefreshMatches(r, ev({ object: 'billing.ac-1', module: 'billing', parent: 'billing.uc-1' }), onModule)).toBe(false)
    expect(panelRefreshMatches(r, ev({ object: 'pay.g-1', module: 'pay', parent: 'pay', fromParent: 'billing' }), onModule)).toBe(true)
  })

  it('types narrow any scope', () => {
    const r = { scope: 'universe', types: ['use-case', 'feature'] }
    expect(panelRefreshMatches(r, ev({ object: 'pay.uc-1', module: 'pay', type: 'use-case' }), onModule)).toBe(true)
    expect(panelRefreshMatches(r, ev({ object: 'pay.d-1', module: 'pay', type: 'deliverable' }), onModule)).toBe(false)
  })

  it('a global panel: every module with no target, its target module otherwise', () => {
    const r = { scope: 'module' }
    const e = ev({ object: 'pay.uc-1', module: 'pay' })
    expect(panelRefreshMatches(r, e, { universe: 'u', module: '' })).toBe(true)
    expect(panelRefreshMatches(r, e, { universe: 'u', module: 'billing' })).toBe(false)
    expect(panelRefreshMatches({ scope: 'self' }, e, { universe: 'u', module: 'pay' })).toBe(true)
  })

  it('what the event does not say matches (a re-read too many beats a stale panel)', () => {
    expect(panelRefreshMatches({ scope: 'module', types: ['goal'] }, ev({}), onModule)).toBe(true)
  })

  it('moduleOfId reads the module of an authored id', () => {
    expect(moduleOfId('billing.uc-1')).toBe('billing')
    expect(moduleOfId('billing')).toBe('billing')
    expect(moduleOfId('')).toBe('')
  })
})

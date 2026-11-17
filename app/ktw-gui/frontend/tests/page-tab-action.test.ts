// SPDX-License-Identifier: Apache-2.0
// A `tab` action (the display library's page DSL) is the page's own: it shows
// a tab of the same page — the planning deck's state tiles open By state —
// and never reaches the host; every other action does.

import { describe, expect, test } from 'bun:test'

import { askTab, routePageAction, tabShown } from '../../../ktw-display-lib/src/ui-builders/page/page-context'
import { ACTION_VERBS, actionVerb } from '../../../ktw-display-lib/src/ui-builders/page/ui-page.interactive'

describe('a tab action', () => {
  test('is a verb of its own', () => {
    expect(ACTION_VERBS).toContain('tab')
    expect(actionVerb({ tab: 'by-state' })).toBe('tab')
    expect(actionVerb({ tab: 'by-state', uri: 'a:b/c' })).toBeUndefined()
  })

  test('is run by the page, never by the host', () => {
    const shown: string[] = []
    const hosted: unknown[] = []
    const on = { show: (t: string) => shown.push(t), select: () => {}, host: (e: unknown) => hosted.push(e), selection: {} }
    routePageAction({ action: { tab: 'by-state' } }, on)
    routePageAction({ action: { uri: 'ktw-agents:plans/finish' } }, on)
    expect(shown).toEqual(['by-state'])
    expect(hosted).toHaveLength(1)
  })

  test('shows its tab in the tabs node that holds it, again when asked again', () => {
    const first = askTab(undefined, 'by-phase')
    expect(tabShown(first, ['overview', 'by-state', 'by-phase'])).toBe('by-phase')
    expect(tabShown(first, ['a', 'b'])).toBeUndefined()
    const again = askTab(first, 'by-phase')
    expect(again).not.toEqual(first)
    expect(tabShown(undefined, ['overview'])).toBeUndefined()
    // A tabs node mounted after the ask (since = its n) does not jump to it.
    expect(tabShown(again, ['by-phase'], again.n)).toBeUndefined()
    expect(tabShown(askTab(again, 'by-phase'), ['by-phase'], again.n)).toBe('by-phase')
  })
})

describe('an action argument that is a whole template', () => {
  test('passes its value as it is: an array, null; only an absent one is empty', async () => {
    const { resolveActionArgs } = await import('../../../ktw-display-lib/src/ui-builders/page/ui-page.interactive')
    const scope = { value: { deliverables: [{ title: 'a' }], cleared: null } }
    expect(resolveActionArgs({ d: '${value.deliverables}' }, undefined, scope)).toEqual({ d: [{ title: 'a' }] })
    expect(resolveActionArgs({ d: '${value.cleared}' }, undefined, scope)).toEqual({ d: null })
    expect(resolveActionArgs({ d: '${value.missing}' }, undefined, scope)).toEqual({ d: '' })
  })
})

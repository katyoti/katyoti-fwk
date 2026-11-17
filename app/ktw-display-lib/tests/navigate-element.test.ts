// SPDX-License-Identifier: Apache-2.0
// A navigate action to a panel of an object can name an
// element of it - templated like the rest, kept only with a panel, read the
// same from an action and from a method's {open} - for the host to bring into
// view.

import { describe, expect, test } from 'bun:test'

import { UIPageNodeJSONSchema } from '../src/ui-builders/page/ui-page.json-schema.ts'
import { navigateTarget, parseActionResult, resolveAction } from '../src/ui-builders/page/ui-page.interactive.ts'

describe('a navigate naming an element', () => {
  test('its element is templated over the element the action is on', () => {
    const a = resolveAction({ navigate: { object: '${item.study}', panel: 'sections', element: 'section:${item.n}' } }, { study: 'etude-erp', n: '03' })
    expect(a.navigate).toEqual({ object: 'etude-erp', panel: 'sections', element: 'section:03' })
  })
  test('navigateTarget keeps it with a panel, drops it without, trims it', () => {
    expect(navigateTarget({ object: 'e', panel: 'sections', element: ' section:03 ' })).toEqual({ object: 'e', panel: 'sections', element: 'section:03' })
    expect(navigateTarget({ object: 'e', element: 'section:03' })).toEqual({ object: 'e' })
    expect(navigateTarget({ object: 'e', panel: 'sections', element: '  ' })).toEqual({ object: 'e', panel: 'sections' })
    expect(navigateTarget('e#sections')).toEqual({ object: 'e', panel: 'sections' })
  })
  test("a method's {open} carries it too", () => {
    const r = parseActionResult({ ok: true, open: { object: 'e', panel: 'sections', element: 'section:03' } })
    expect(r.open).toEqual({ object: 'e', panel: 'sections', element: 'section:03' })
  })
  test('the action schema declares it', () => {
    const nav = (UIPageNodeJSONSchema as any).definitions.Action.properties.navigate
    expect(nav.properties.element.type).toBe('string')
  })
})

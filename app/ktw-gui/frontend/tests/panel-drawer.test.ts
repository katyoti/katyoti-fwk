// SPDX-License-Identifier: Apache-2.0
// A page's drawer action names an object's panel by its stem;
// a header panel is drawn above the tabs, never one of them.

import { describe, expect, test } from 'bun:test'
import { isHeaderPanel, panelOf } from '../src/sections/workspace/panelOf'

describe('the panel a drawer opens', () => {
  const tabs = [{ id: 'information' }, { id: 'tab:cadrage', name: 'Cadrage' }, { id: 'tab:header', header: true }]
  test('is found by its stem or its strip id', () => {
    expect(panelOf(tabs, 'cadrage')?.id).toBe('tab:cadrage')
    expect(panelOf(tabs, 'tab:cadrage')?.id).toBe('tab:cadrage')
    expect(panelOf(tabs, 'information')?.id).toBe('information')
    expect(panelOf(tabs, 'nope')).toBeUndefined()
  })
  test('a header panel is told apart from a tab', () => {
    expect(tabs.filter(isHeaderPanel).map((t) => t.id)).toEqual(['tab:header'])
  })
})

// PageHost renders the registered drawer view as a component - a
// call would run PanelDrawer's hooks as PageHost's own, and the render that
// opens the drawer would break the page (« Rendered more hooks »).
describe('the drawer a page opens', () => {
  test('is rendered as an element, never called', async () => {
    const src = await Bun.file(new URL('../src/components/compositions/PageHost.tsx', import.meta.url)).text()
    expect(src).not.toMatch(/panelDrawer\s*\(\s*\{/)
    expect(src).toMatch(/<DrawerView\s/)
  })
})

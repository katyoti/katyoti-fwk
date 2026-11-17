// SPDX-License-Identifier: Apache-2.0
// The object pane draws one header
// only - a declared header panel in place of the generic one, the generic one
// back only while the object's core is edited - and a header panel re-reads
// on its own object's events at least. Off, both headers as before.

import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'

import { drawsGenericHeader, headerPanelRefresh } from '../src/sections/workspace/panelOf'
import { panelRefreshMatches } from '../src/hooks/usePanelRefresh'
import { reread } from '../src/sections/workspace/RenderDefView'

describe('one header', () => {
  test('a declared header panel takes the generic header`s place behind the switch', () => {
    expect(drawsGenericHeader(true, 1, false)).toBe(false)
    // No declared header: the generic one stays.
    expect(drawsGenericHeader(true, 0, false)).toBe(true)
    // Editing the core needs the generic header, which holds it.
    expect(drawsGenericHeader(true, 1, true)).toBe(true)
    // Off: both, as before.
    expect(drawsGenericHeader(false, 1, false)).toBe(true)
  })

  test('a header panel follows its own object`s events at least', () => {
    expect(headerPanelRefresh(undefined)).toEqual({ scope: 'self' })
    expect(headerPanelRefresh({ scope: 'none' })).toEqual({ scope: 'self' })
    expect(headerPanelRefresh({ scope: 'module', types: ['study'] })).toEqual({ scope: 'module', types: ['study'] })
    const place = { universe: 'u', object: 'acme.DA-001', module: 'acme' }
    const ev = { universe: 'u', subject: 's', object: 'acme.DA-001', module: 'acme', action: 'transitioned' }
    expect(panelRefreshMatches(headerPanelRefresh({ scope: 'none' }), ev, place)).toBe(true)
    expect(panelRefreshMatches({ scope: 'none' }, ev, place)).toBe(false)
    expect(panelRefreshMatches(headerPanelRefresh(undefined), { ...ev, object: 'acme.DA-002' }, place)).toBe(false)
  })

  test('the pane is wired to them for every user (its switch retired)', () => {
    const src = readFileSync(join(import.meta.dir, '../src/sections/workspace/ObjectTab.tsx'), 'utf8')
    expect(src).not.toContain("useDisplaySwitch('in045')")
    expect(src).toContain('const formFirst = true')
    expect(src).toContain('drawsGenericHeader(formFirst, headerPanels.length, core !== null)')
    expect(src).toMatch(/formFirst\s*\?\s*headerPanelRefresh\(/)
    expect(src).toContain('data-guide="object.header-actions"')
  })

  // A header panel drawn as a page whose sections load
  // (the study's « En-tête de l'étude ») follows its refresh too - its data
  // read again, so its sections load again in place.
  test('a header panel drawn as a page re-reads on its refresh', () => {
    const def = { kind: 'page', dslTree: { kind: 'box', id: 'study-header' } }
    expect(reread(def, 0)).toBe(def)
    const again = reread(def, 1)
    expect(again.dslTree).toBe(def.dslTree)
    expect(again.data).toEqual({})
    expect(reread(again, 2).data).not.toBe(again.data)
    expect(reread({ ...def, data: [1] }, 1).data).toEqual([1])
    const src = readFileSync(join(import.meta.dir, '../src/sections/workspace/ObjectTab.tsx'), 'utf8')
    expect(src).toMatch(/<RenderDefView[^>]*?refresh=\{refresh\} module=\{detail\.module\}/s)
    const view = readFileSync(join(import.meta.dir, '../src/sections/workspace/RenderDefView.tsx'), 'utf8')
    expect(view).toMatch(/usePanelRefresh\(refresh, \{ universe: universeName, object: objectId, module \}, \(\) => setReads/)
  })
})

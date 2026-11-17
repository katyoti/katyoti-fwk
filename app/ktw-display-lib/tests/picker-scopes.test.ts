// SPDX-License-Identifier: Apache-2.0
// Behind IN-045's switch (the host's sourceModel), a picker
// shows its source's scopes as chips - the default ones from the
// configuration and those the carrier added - ticked for the search, with
// « + Ajouter » adding one for the carrier. Off, the former picker.

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { HostServicesContext } from '../src/ui-core/host/index.tsx'
import { addScope, loadScopes, PICKER_LABELS, PickerRenderer, saveScope, ScopeChips, scopeSearch, scopesInput, searchInput, tickedScopes, toggleScope, withoutScope } from '../src/ui-builders/page/nodes/picker.tsx'

const scopes = [
  { key: 'AUP', title: 'Architecture urbanisme', default: true },
  { key: 'ERP', title: 'ERP Arbor' },
]

const chips = (props: Partial<Parameters<typeof ScopeChips>[0]>) => renderToStaticMarkup(createElement(MantineProvider, null,
  createElement(ScopeChips, {
    scopes, ticked: new Set(['AUP', 'ERP']), add: true, adding: null, labels: PICKER_LABELS,
    onToggle: () => {}, onAdding: () => {}, onAdd: () => {}, ...props,
  })))

describe('a picker shows its scopes as chips', () => {
  test('the scopes method is asked with the args and the carrier', () => {
    expect(scopesInput({ args: { connector: 'confluence' }, object: 'etude-1' })).toEqual({ connector: 'confluence', object: 'etude-1' })
  })

  test('a search sends the ticked scopes, in their order, and none when no scopes show', () => {
    const node = { args: { connector: 'confluence' }, object: 'etude-1' }
    let ticked = new Set(['AUP', 'ERP'])
    ticked = toggleScope(ticked, 'AUP')
    expect(tickedScopes(scopes, ticked)).toEqual(['ERP'])
    ticked = toggleScope(ticked, 'AUP')
    expect(searchInput(node, ' flux ', tickedScopes(scopes, ticked))).toEqual({ connector: 'confluence', object: 'etude-1', query: 'flux', scopes: ['AUP', 'ERP'] })
    expect(searchInput(node, 'flux')).toEqual({ connector: 'confluence', object: 'etude-1', query: 'flux' })
  })

  test('« + Ajouter » adds a scope once, never a blank one', () => {
    expect(addScope(scopes, ' DSI ').map((s) => s.key)).toEqual(['AUP', 'ERP', 'DSI'])
    expect(addScope(scopes, 'ERP')).toBe(scopes)
    expect(addScope(scopes, '  ')).toBe(scopes)
  })

  test('draws one chip per scope, filled while ticked, the default one marked, and « + Ajouter »', () => {
    const html = chips({ ticked: new Set(['AUP']), unit: 'espace' })
    expect(html).toContain('data-guide="picker.scopes"')
    expect(html).toContain('Chercher dans (espace)')
    expect(html).toMatch(/data-guide="picker\.scope\.AUP"[^>]*data-default="true"[^>]*aria-pressed="true"/)
    expect(html).toMatch(/data-guide="picker\.scope\.ERP"[^>]*aria-pressed="false"/)
    expect(html).toContain('Architecture urbanisme')
    expect(html).toContain('data-guide="picker.scope-add"')
    expect(chips({ adding: '' })).toContain('data-guide="picker.scope-input"')
    expect(chips({ add: false })).not.toContain('picker.scope-add')
  })

  test('the scopes method is asked through the host with the args and the carrier, all ticked; none when it fails', async () => {
    const asked: unknown[] = []
    const host = async (uri: string, input: Record<string, unknown>) => { asked.push([uri, input]); return { scopes: [...scopes, { title: 'no key' }] } }
    const got = await loadScopes(host as any, 'confluence:sources/scopes', { args: { connector: 'confluence' }, object: 'etude-1' })
    expect(asked).toEqual([['confluence:sources/scopes', { connector: 'confluence', object: 'etude-1' }]])
    expect(got.scopes.map((s) => s.key)).toEqual(['AUP', 'ERP'])
    expect([...got.ticked]).toEqual(['AUP', 'ERP'])
    const failed = await loadScopes(async () => { throw new Error('down') }, 'x', {})
    expect(failed.scopes).toEqual([])
  })

  test('none ticked refuses the search, never sent with scopes: []', () => {
    expect(scopeSearch(scopes, new Set())).toBeNull()
    expect(scopeSearch(scopes, new Set(['ERP']))).toEqual(['ERP'])
    expect(scopeSearch(null, new Set())).toBeUndefined()
    expect(scopeSearch([], new Set())).toBeUndefined()
    expect(chips({ ticked: new Set() })).toContain('data-guide="picker.scope-none"')
    expect(chips({ ticked: new Set(['AUP']) })).not.toContain('picker.scope-none')
  })

  test('a refused save drops the chip it added', async () => {
    const node = { args: { connector: 'confluence' }, object: 'etude-1' }
    const saved: unknown[] = []
    expect(await saveScope(async (uri, input) => { saved.push([uri, input]); return { ok: true } }, 's', node, 'DSI')).toBeUndefined()
    expect(saved).toEqual([['s', { connector: 'confluence', object: 'etude-1', scope: 'DSI' }]])
    expect(await saveScope(async () => ({ ok: false, message: 'not allowed' }), 's', node, 'DSI')).toBe('not allowed')
    expect(await saveScope(async () => { throw new Error('down') }, 's', node, 'DSI')).toContain('down')
    const added = addScope(scopes, 'DSI')
    const dropped = withoutScope(added, new Set(['AUP', 'DSI']), 'DSI')
    expect(dropped.scopes.map((s) => s.key)).toEqual(['AUP', 'ERP'])
    expect([...dropped.ticked]).toEqual(['AUP'])
  })

  test('each chip is a button reached by the keyboard, out of reach while disabled', () => {
    expect(chips({})).toMatch(/data-guide="picker\.scope\.AUP"[^>]*role="button"[^>]*tabindex="0"/)
    expect(chips({ disabled: true })).toMatch(/data-guide="picker\.scope\.AUP"[^>]*tabindex="-1"/)
  })

  test('under a host without the source model, a picker given scopes draws its search and no chip', () => {
    const host = { callMethod: async () => ({ scopes }) }
    const node = { kind: 'picker', id: 'p', search: 'x:s/search', scopes: { list: 'x:s/scopes', add: true } }
    const html = renderToStaticMarkup(createElement(MantineProvider, null,
      createElement(HostServicesContext.Provider, { value: host as any }, createElement(PickerRenderer, { node: node as any }))))
    expect(html).toContain('data-guide="picker.search"')
    expect(html).not.toContain('picker.scopes')
  })
})

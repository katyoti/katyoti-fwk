// SPDX-License-Identifier: Apache-2.0
// A picker whose node names no search draws no search field -
// it browses (« Parcourir »), or holds what a proposal kept - and the
// source-set field hands each connector's browse to its picker.

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { HostServicesContext } from '../src/ui-core/host/index.tsx'
import { pickerFinds, PickerRenderer } from '../src/ui-builders/page/nodes/picker.tsx'
import { UIPageNodeJSONSchema } from '../src/ui-builders/page/ui-page.json-schema.ts'
import { connectorNode } from '../src/ui-builders/fields/source-set/renderer.tsx'

const render = (host: object, node: object) => renderToStaticMarkup(createElement(MantineProvider, null,
  createElement(HostServicesContext.Provider, { value: host as any }, createElement(PickerRenderer, { node: node as any }))))

const call = async () => ({ items: [] })

describe('a picker with no search', () => {
  test('finds by a search, a browse, both or neither as its node names them', () => {
    expect(pickerFinds({ search: 's', browse: 'b' })).toEqual({ search: true, browse: true })
    expect(pickerFinds({ browse: 'b' })).toEqual({ search: false, browse: true })
    expect(pickerFinds({ search: 's' })).toEqual({ search: true, browse: false })
    expect(pickerFinds({})).toEqual({ search: false, browse: false })
  })

  test('a browse-only picker draws « Parcourir » and no search field, no hint, no scope chips', () => {
    const node = { kind: 'picker', id: 'p', browse: 'x:s/browse', hint: 'Tapez au moins deux lettres', columns: true,
      scopes: { list: 'x:s/scopes', add: true } }
    for (const host of [{ callMethod: call }, { callMethod: call, sourceModel: true }]) {
      const html = render(host, node)
      expect(html).toContain('data-guide="picker.browse"')
      expect(html).not.toContain('data-guide="picker.search"')
      expect(html).not.toContain('picker.search-icon')
      expect(html).not.toContain('picker.hint')
      expect(html).not.toContain('picker.scopes')
      expect(html).not.toContain('Rechercher et ajouter')
    }
  })

  test('a picker with neither shows its selection only', () => {
    const html = render({ callMethod: call }, { kind: 'picker', id: 'p', selected: [{ key: 'A1', title: 'ZContact' }] })
    expect(html).not.toContain('data-guide="picker.search"')
    expect(html).not.toContain('data-guide="picker.browse"')
    expect(html).toContain('ZContact')
  })

  test('a picker with a search still draws its field', () => {
    const html = render({ callMethod: call }, { kind: 'picker', id: 'p', search: 'x:s/search', browse: 'x:s/browse' })
    expect(html).toContain('data-guide="picker.search"')
    expect(html).toContain('data-guide="picker.browse"')
  })

  test('the page schema no longer requires a search on a picker', () => {
    const picker = (UIPageNodeJSONSchema as any).definitions.Picker
    expect(picker.required).toEqual(['kind'])
    expect(picker.properties.search.type).toBe('string')
  })

  test('the source-set field hands a connector its browse, and its search only where it has one', () => {
    const browseOnly = connectorNode({ name: 'dynatrace', browse: 'ktw-agents:sources/source-browse', args: { connector: 'dynatrace' } }, 'acme-da.DA-001')
    expect(browseOnly.browse).toBe('ktw-agents:sources/source-browse')
    expect(browseOnly.search).toBeUndefined()
    expect(browseOnly.object).toBe('acme-da.DA-001')
    const both = connectorNode({ name: 'confluence', search: 'ktw-agents:sources/search', browse: 'ktw-agents:sources/source-browse' })
    expect(both.search).toBe('ktw-agents:sources/search')
    expect(both.browse).toBe('ktw-agents:sources/source-browse')
  })
})

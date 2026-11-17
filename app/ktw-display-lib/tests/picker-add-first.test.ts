// SPDX-License-Identifier: Apache-2.0
// In every picker list the « + » that adds an item (or its
// held box, its choice) sits first on the line, beside the name it adds, and
// « Retirer » in « Sélection » likewise before the name it removes.

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { HostServicesContext } from '../src/ui-core/host/index.tsx'
import { PICKER_LABELS, PickerAddControl, PickerLine, PickerRenderer } from '../src/ui-builders/page/nodes/picker.tsx'

const wrap = (el: any, host: object = {}) => renderToStaticMarkup(createElement(MantineProvider, null,
  createElement(HostServicesContext.Provider, { value: host as any }, el)))

describe('the picker draws its add first on each line', () => {
  test('a found item\'s line starts with its « + », then its name', () => {
    const add = createElement(PickerAddControl, { held: false, off: false, labels: PICKER_LABELS, onAdd: () => {} })
    const html = wrap(createElement(PickerLine, { add, guide: 'picker.match', name: 'k1', depth: 1 },
      createElement('span', null, 'Application Achats')))
    const plus = html.indexOf(`aria-label="${PICKER_LABELS.add}"`)
    const name = html.indexOf('Application Achats')
    expect(plus).toBeGreaterThan(-1)
    expect(name).toBeGreaterThan(-1)
    expect(plus).toBeLessThan(name)
    // Nothing pushes the « + » to the far end of a wide line.
    expect(html).not.toContain('space-between')
  })

  test('« Retirer » comes before the selected item\'s name', () => {
    const node = { kind: 'picker', id: 'p', selected: [{ key: 'a', title: 'Contrats fournisseurs' }] }
    const html = wrap(createElement(PickerRenderer, { node: node as any }), { callMethod: async () => ({}) })
    const line = html.slice(html.indexOf('data-guide="picker.selected"'))
    const remove = line.indexOf(PICKER_LABELS.remove)
    const name = line.indexOf('Contrats fournisseurs')
    expect(remove).toBeGreaterThan(-1)
    expect(name).toBeGreaterThan(-1)
    expect(remove).toBeLessThan(name)
  })
})

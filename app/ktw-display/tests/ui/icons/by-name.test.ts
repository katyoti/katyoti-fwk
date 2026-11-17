// SPDX-License-Identifier: Apache-2.0
// tablerIcon — an icon NAME resolves through the lazily loaded icon set
// (app/ktw-display-lib/src/ui-core/components/icons/byName.tsx).
import { describe, expect, it } from 'bun:test'
import { IconShape } from '@tabler/icons-react'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'

import {
  loadTablerIcons,
  tablerComponentName,
  tablerIcon,
} from '../../../../ktw-display-lib/src/ui-core/components/icons/byName.tsx'

describe('tablerIcon', () => {
  let standIn: ReturnType<typeof tablerIcon> | undefined

  it('names an icon in kebab case or by its component name', () => {
    expect(tablerComponentName('layout-grid')).toBe('IconLayoutGrid')
    expect(tablerComponentName('IconLayoutGrid')).toBe('IconLayoutGrid')
  })

  it('gives the fallback for no name', () => {
    expect(tablerIcon(undefined, IconShape)).toBe(IconShape)
    expect(tablerIcon('', IconShape)).toBe(IconShape)
  })

  it('gives one stable stand-in per name while the set loads', () => {
    const a = tablerIcon('layout-grid', IconShape)
    standIn = a
    expect(a).not.toBe(IconShape)
    expect(tablerIcon('layout-grid', IconShape)).toBe(a)
    expect(renderToStaticMarkup(createElement(a))).toContain('tabler-icon-shape')
  })

  it('gives the icon itself once loaded, and the fallback for an unknown name', async () => {
    const set = await loadTablerIcons()
    expect(tablerIcon('layout-grid', IconShape)).toBe(set.IconLayoutGrid!)
    expect(tablerIcon('no-such-icon', IconShape)).toBe(IconShape)
  })

  it('makes a stand-in handed out before the load draw the icon itself', async () => {
    await loadTablerIcons()
    expect(standIn).toBeDefined()
    expect(standIn).not.toBe(tablerIcon('layout-grid', IconShape))
    expect(renderToStaticMarkup(createElement(standIn!))).toContain('tabler-icon-layout-grid')
  })
})

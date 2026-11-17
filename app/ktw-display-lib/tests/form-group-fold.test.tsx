// SPDX-License-Identifier: Apache-2.0
// A form group declared `collapsible` in a data-type's
// ui-form extension folds under its title - drawn folded at first when
// declared `collapsed`, its fields hidden (kept mounted, so their values
// stay) until its title is clicked; a group not declared collapsible, or
// one with no title to click, draws open as before.

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { fol_group_layout, groupFold } from '../src/ui-builders/form/layouts/_shared/index.ts'
import { setRendererLookup, type FieldRendererFactory } from '../src/ui-builders/fields/index.ts'
import { buildUIForms } from '../src/ui-builders/form/index.ts'
import type { UIFormFields } from '../src/ui-builders/form/ui-form.types.ts'
import type { Schema } from '../src/schema/index.ts'

const field = (name: string) => () => createElement('label', null, name)
const def = (over: Partial<UIFormFields>): UIFormFields => ({
  getFieldsToRender: () => [field('Maximum'), field('Période')],
  ...over,
})
const draw = (d: UIFormFields, framed = true) => renderToStaticMarkup(createElement(MantineProvider, null,
  fol_group_layout(d, framed)({ isNew: false, isSubmitting: false, item: {} }) as never))

// Mantine's Collapse hides a folded body: height 0, aria-hidden and inert.
const hidden = (html: string) => /<div style="[^"]*height:0;overflow:hidden"[^>]*aria-hidden="true" inert="">.*Maximum/s.test(html)

describe('a collapsible form group', () => {
  test('groupFold: collapsible with a title folds, collapsed starts folded; no title draws open', () => {
    expect(groupFold({ title: 'Avancé', collapsible: true, collapsed: true })).toEqual({ foldable: true, open: false })
    expect(groupFold({ title: 'Avancé', collapsible: true })).toEqual({ foldable: true, open: true })
    expect(groupFold({ title: 'Avancé', collapsed: true })).toEqual({ foldable: false, open: true })
    expect(groupFold({ collapsible: true, collapsed: true })).toEqual({ foldable: false, open: true })
  })

  for (const framed of [true, false]) {
    const how = framed ? 'framed' : 'flush'
    test(`declared collapsed, it is drawn folded under a toggle (${how})`, () => {
      const html = draw(def({ title: 'Avancé', collapsible: true, collapsed: true }), framed)
      expect(html).toContain('data-form-group="Avancé"')
      expect(html).toContain('data-folded="true"')
      expect(html).toContain('data-form-group-toggle=""')
      expect(html).toContain('aria-expanded="false"')
      expect(html).toContain('Maximum') // kept mounted
      expect(hidden(html)).toBe(true)
    })

    test(`declared collapsible and not collapsed, it is drawn open under its toggle (${how})`, () => {
      const html = draw(def({ title: 'Avancé', collapsible: true }), framed)
      expect(html).toContain('data-folded="false"')
      expect(html).toContain('aria-expanded="true"')
      expect(hidden(html)).toBe(false)
    })

    test(`not collapsible, it draws open with no toggle, as before (${how})`, () => {
      const html = draw(def({ title: 'Avancé', collapsed: true }), framed)
      expect(html).not.toContain('data-form-group-toggle')
      expect(html).not.toContain('data-folded')
      expect(html).toContain('Avancé')
      expect(hidden(html)).toBe(false)
    })
  }

  test('the builder hands the declared fold from the ui-form extension to the group', () => {
    setRendererLookup(() => ({ view: () => () => null, form: () => () => null, header: () => () => null, column: () => () => null }) as FieldRendererFactory)
    const schema = {
      name: 'loc', root: 'Loc',
      types: { Loc: { type: 'object', fields: { selector: { type: 'string' }, max: { type: 'string' } },
        extensions: [{ type: 'ui-form', name: 'default', groups: [
          { fields: ['selector'] },
          { title: 'Avancé', collapsible: true, collapsed: true, fields: ['max'] },
        ] }] } },
    } as unknown as Schema
    const form = buildUIForms({ schema })['default']
    const html = renderToStaticMarkup(createElement(MantineProvider, null,
      form.renderer({ isNew: false, isSubmitting: false, item: {} }) as never))
    expect(html).toContain('data-form-group="Avancé"')
    expect(html).toContain('data-folded="true"')
  })
})

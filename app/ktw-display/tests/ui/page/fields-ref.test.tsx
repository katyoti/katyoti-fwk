// SPDX-License-Identifier: Apache-2.0
// Reference fields — object-ref (an object's id) and panel-ref
// (`<id>#<panel>`): with a host providing pickObjects the form searches the
// objects (a searchable select); without one it is a text input; the view is
// a link that opens the object through the host, plain text without one.

import { describe, expect, it } from 'bun:test'
import { renderToString } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { wireFieldRenderers } from '../../../../ktw-display-lib/src/ui-builders/fields/factory.ts'
import { UIPageView } from '../../../../ktw-display-lib/src/ui-builders/page/index.ts'
import type { UIPageNode } from '../../../../ktw-display-lib/src/ui-builders/page/index.ts'
import {
  HostServicesContext, refFilterOf, refTarget, type HostServices,
} from '../../../../ktw-display-lib/src/ui-core/host/index.tsx'
import { panelRef } from '../../../../ktw-display-lib/src/ui-builders/fields/panel-ref/renderer.tsx'
import { pickerData, pickerQuery } from '../../../../ktw-display-lib/src/ui-builders/fields/object-ref/renderer.tsx'

wireFieldRenderers()

const schema = {
  name: 'pick', root: 'In',
  types: {
    In: {
      type: 'object',
      fields: {
        owner: { type: 'string', content: 'object-ref', contentParams: { types: ['person'] } },
        view: { type: 'string', content: 'panel-ref', optional: true },
      },
    },
  },
}

function render(node: UIPageNode, host?: HostServices): string {
  const page = <UIPageView node={node} />
  return renderToString(
    <MantineProvider>
      {host ? <HostServicesContext.Provider value={host}>{page}</HostServicesContext.Provider> : page}
    </MantineProvider>,
  )
}

describe('reference fields', () => {
  it('without a host the form types the id', () => {
    const html = render({ kind: 'form', schema, mode: 'create' } as unknown as UIPageNode)
    expect(html).toContain('placeholder="Object id"')
    expect(html).toContain('placeholder="&lt;object id&gt;#&lt;panel&gt;"')
  })

  it('with a host the form searches the objects', () => {
    const host: HostServices = { pickObjects: async () => [], objectPanels: async () => [] }
    const html = render({ kind: 'form', schema, mode: 'create' } as unknown as UIPageNode, host)
    expect(html).toContain('Search an object…')
    expect(html).not.toContain('placeholder="Object id"')
    expect(html).toContain('Choose the object first')
  })

  it('the view opens the object through the host; text without one', () => {
    const node = { kind: 'view', schema, data: { owner: 'people.ana', view: 'billing#dashboard' } } as unknown as UIPageNode
    const live = render(node, { navigate: () => {} })
    expect(live).toContain('people.ana')
    expect(live).toContain('billing › dashboard')
    expect(live).toMatch(/<button[^>]*data-testid="view.owner"/)
    expect(render(node)).toMatch(/<p[^>]*data-testid="view.owner"/)
  })

  it('reads and writes a reference value; reads the filter from the params', () => {
    expect(refTarget('billing#dashboard')).toEqual({ object: 'billing', panel: 'dashboard' })
    expect(refTarget('billing')).toEqual({ object: 'billing' })
    expect(refTarget('')).toBeUndefined()
    expect(panelRef('billing', 'dashboard')).toBe('billing#dashboard')
    expect(panelRef(null, 'x')).toBe('')
    expect(refFilterOf({ types: 'feature|use-case', when: "state != 'done'", other: 1 })).toEqual({ types: ['feature', 'use-case'], when: "state != 'done'" })
  })

  it('a chosen object keeps its label, and the list is searched again from scratch', () => {
    const ana = { id: 'people.ana', name: 'Ana', type: 'person' }
    const known = new Map([[ana.id, ana]])
    // The select puts the chosen label in the search box: that is not a query.
    expect(pickerQuery('Ana (people.ana · person)', 'people.ana', known)).toBe('')
    expect(pickerQuery('An', 'people.ana', known)).toBe('An')
    expect(pickerQuery('An', null, known)).toBe('An')
    // The search no longer returns it: it stays a choice under its label.
    expect(pickerData([], 'people.ana', known)).toEqual([{ value: 'people.ana', label: 'Ana (people.ana · person)' }])
    // Never seen: the id, until the lookup answers.
    expect(pickerData([], 'people.bob', known)).toEqual([{ value: 'people.bob', label: 'people.bob' }])
  })
})

// SPDX-License-Identifier: Apache-2.0
// A picker can hand its selection to its apply method as it
// changes, with no « Appliquer » - after a short calm, one call at a time, the
// last state always sent.

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { UIPageNodeJSONSchema } from '../src/ui-builders/page/ui-page.json-schema.ts'
import { HostServicesContext } from '../src/ui-core/host/index.tsx'
import { changeApplier, PickerRenderer } from '../src/ui-builders/page/nodes/picker.tsx'

const wait = (ms: number) => new Promise((r) => setTimeout(r, ms))

const render = (node: unknown) => renderToStaticMarkup(createElement(MantineProvider, null,
  createElement(HostServicesContext.Provider, { value: { callMethod: async () => ({ ok: true }) } as any },
    createElement(PickerRenderer, { node: node as any }))))

describe('a picker applying as its selection changes', () => {
  test('is declared on the picker', () => {
    expect((UIPageNodeJSONSchema as any).definitions.Picker.properties.applyOn.enum).toEqual(['change'])
  })

  test('with applyOn change it draws no « Appliquer »; without, it does', () => {
    const node = { kind: 'picker', id: 'apps', search: 'x:apps/search', apply: 'ktw-agents:sources/hold', selected: [{ key: 'A1', title: 'Arbor' }] }
    expect(render(node)).toContain('data-guide="picker.apply"')
    expect(render({ ...node, applyOn: 'change' })).not.toContain('data-guide="picker.apply"')
  })

  test('changes are sent after a calm, the last state only', async () => {
    const sent: unknown[] = []
    const a = changeApplier(async (input) => { sent.push(input) }, 20)
    a.change({ items: ['A1'] })
    a.change({ items: ['A1', 'A2'] })
    expect(sent).toEqual([])
    await wait(40)
    expect(sent).toEqual([{ items: ['A1', 'A2'] }])
  })

  test('one call at a time: a change made during a call is sent once it answers', async () => {
    const sent: unknown[] = []
    let inFlight = 0
    let most = 0
    const a = changeApplier(async (input) => {
      inFlight++
      most = Math.max(most, inFlight)
      sent.push(input)
      await wait(40)
      inFlight--
    }, 5)
    a.change({ items: ['A1'] })
    await wait(15) // the first call is running
    a.change({ items: ['A2'] })
    await wait(15)
    a.change({ items: ['A3'] })
    await wait(120)
    expect(most).toBe(1)
    expect(sent).toEqual([{ items: ['A1'] }, { items: ['A3'] }])
  })

  test('a call that fails does not stop the next change', async () => {
    const sent: unknown[] = []
    const a = changeApplier(async (input) => { sent.push(input); if (sent.length === 1) throw new Error('hub unreachable') }, 5)
    a.change({ items: ['A1'] })
    await wait(20)
    a.change({ items: ['A2'] })
    await wait(20)
    expect(sent).toEqual([{ items: ['A1'] }, { items: ['A2'] }])
  })
})

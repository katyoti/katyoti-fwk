// SPDX-License-Identifier: Apache-2.0
// An inline select and an action menu: a select's choice runs
// its action with ${value} the option and the element it is on; a menu's
// plain entry runs its action, a choice entry lists its options (inline, a
// field of the element, or `a|b|c`), a prompt entry runs only with a text.

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { UIPageView } from '../src/ui-builders/page/ui-page.view.tsx'
import { applyDataBinding } from '../src/ui-builders/page/nodes/data-binding.ts'
import { UIPageNodeJSONSchema } from '../src/ui-builders/page/ui-page.json-schema.ts'
import {
  choiceEvent, choiceOptions, menuItemAction, promptEvent, resolveAction,
} from '../src/ui-builders/page/ui-page.interactive.ts'

const page = (node: unknown, data?: unknown, onAction?: (e: unknown) => void) => renderToStaticMarkup(
  createElement(MantineProvider, null, createElement(UIPageView, { node: node as never, data, onAction: onAction as never })))

const assess = { uri: 'ktw-agents:requests/assess', args: { id: '${item.id}', priority: '${value}' } }

describe('choice options', () => {
  test('come inline, from the element, else the page data, or from a|b|c', () => {
    expect(choiceOptions({ options: [{ value: 'a', label: 'A' }, { value: 'b' }] })).toEqual([{ value: 'a', label: 'A' }, { value: 'b', label: 'b' }])
    const row = { id: 'IN-1', options: [{ value: 'r1', label: 'Release 1' }] }
    expect(choiceOptions({ optionsField: 'options' }, row)).toEqual([{ value: 'r1', label: 'Release 1' }])
    expect(choiceOptions({ optionsField: '$.releases' }, row, { releases: ['x', 'y'] })).toEqual([{ value: 'x', label: 'x' }, { value: 'y', label: 'y' }])
    expect(choiceOptions('P0|P1| P2 ')).toEqual([{ value: 'P0', label: 'P0' }, { value: 'P1', label: 'P1' }, { value: 'P2', label: 'P2' }])
    expect(choiceOptions('options', row)).toEqual([{ value: 'r1', label: 'Release 1' }])
    expect(choiceOptions({ options: [{ label: 'no value' } as never] })).toEqual([])
    expect(choiceOptions(undefined)).toEqual([])
  })
})

describe('a choice and a prompt', () => {
  test('send the action, the element and the value; the host fills ${value}', () => {
    const row = { id: 'IN-7' }
    const e = choiceEvent(assess, row, 'P1')
    expect(e).toEqual({ action: assess, item: row, extra: { value: 'P1' } })
    expect(resolveAction(e.action, e.item, { ...e.extra }).args).toEqual({ id: 'IN-7', priority: 'P1' })
    expect(promptEvent(assess, row, '  waits on legal ')).toEqual({ action: assess, item: row, extra: { value: 'waits on legal' } })
    expect(promptEvent(assess, row, '   ')).toBeUndefined()
  })

  test('a menu entry runs its action or its own verb, its confirm and dangerous laid over', () => {
    expect(menuItemAction({ id: 'a', label: 'A', action: assess, confirm: 'Sure?', dangerous: true }))
      .toEqual({ ...assess, confirm: 'Sure?', dangerous: true })
    const flat = { id: 'p', label: 'Priority', uri: 'ktw-agents:requests/assess', args: { priority: '${value}' }, choice: 'P0|P1' }
    expect(menuItemAction(flat as never)).toEqual({ id: 'p', label: 'Priority', uri: 'ktw-agents:requests/assess', args: { priority: '${value}' } })
    expect(menuItemAction({ id: 'none', label: 'None' })).toBeUndefined()
  })
})

describe('the nodes', () => {
  test('are declared in the schema; a select binds its value, a menu its entries', () => {
    const defs = (UIPageNodeJSONSchema as any).definitions
    expect(defs.Select.properties.action.$ref).toBe('#/definitions/Action')
    expect(defs.Menu.required).toEqual(['kind', 'items'])
    expect(defs.Menu.properties.items.items.properties.prompt.type).toBe('string')
    expect((applyDataBinding({ kind: 'select', dataField: '$.p', action: assess } as never, { p: 'P2' }) as any).value).toBe('P2')
    expect((applyDataBinding({ kind: 'menu', dataField: '$.m', items: [] } as never, { m: [{ id: 'x', label: 'X' }] }) as any).items).toHaveLength(1)
  })

  test('a select renders its value; disabled without a host, enabled with one', () => {
    const node = { kind: 'select', label: 'Priority', value: 'P1', options: [{ value: 'P0' }, { value: 'P1' }], action: assess }
    const bare = page(node)
    expect(bare).toContain('Priority')
    expect(bare).toContain('value="P1"')
    expect(bare).toContain('disabled')
    expect(page(node, undefined, () => {})).not.toContain('disabled=""')
  })

  test('a menu renders its button, ⋯ by default', () => {
    const node = { kind: 'menu', items: [{ id: 'hold', label: 'Put on hold', prompt: 'Why?', action: assess }] }
    expect(page(node, undefined, () => {})).toContain('⋯')
    expect(page({ ...node, label: 'Actions' }, undefined, () => {})).toContain('Actions')
  })
})

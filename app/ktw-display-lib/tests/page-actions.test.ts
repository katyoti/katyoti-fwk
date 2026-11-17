// SPDX-License-Identifier: Apache-2.0
// An element's actions as the planning views place them: an
// action shown on a field's value (`when: next_step=freeze`), a label read
// from the element (`${item.next_label}`), a primary one filled and the
// `menu` ones under ⋯, a slots view's actions at its top right; the palette's
// legend and a section's small label.

import { describe, expect, test } from 'bun:test'
import { createElement, type ReactElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { wireFieldRenderers } from '../src/ui-builders/fields/factory.ts'
import { buildUIViews } from '../src/ui-builders/view/index.ts'
import { actionLabel, actionShown } from '../src/ui-builders/page/nodes/interactive.tsx'
import { UIPageView } from '../src/ui-builders/page/ui-page.view.tsx'
import { dataType, releaseRows } from './fixtures/delivery-sample.ts'

wireFieldRenderers()

const html = (el: ReactElement) => renderToStaticMarkup(createElement(MantineProvider, null, el))

describe('an action on its element', () => {
  test('`when` a field is set, or `field=value` when it holds that value', () => {
    const row = { next_step: 'freeze', held: '' }
    expect(actionShown({ when: 'next_step' }, row)).toBe(true)
    expect(actionShown({ when: 'held' }, row)).toBe(false)
    expect(actionShown({ when: 'next_step=freeze' }, row)).toBe(true)
    expect(actionShown({ when: 'next_step = deliver' }, row)).toBe(false)
    expect(actionShown({ when: 'missing=freeze' }, row)).toBe(false)
    expect(actionShown({}, row)).toBe(true)
  })

  test('its label reads ${item.<path>} from the element', () => {
    const row = { next_label: 'Finish 3 increments, then Freeze', release: { id: 'RL-033' } }
    expect(actionLabel('${item.next_label}', row)).toBe('Finish 3 increments, then Freeze')
    expect(actionLabel('Open ${item.release.id}', row)).toBe('Open RL-033')
    expect(actionLabel('${item.none}', row)).toBe('')
    expect(actionLabel('Reschedule', row)).toBe('Reschedule')
  })
})

describe('a slots view with its actions at the top', () => {
  test('the shown primary action filled, the menu ones under ⋯, beside the header', () => {
    const schema = dataType('release-rows') as any
    const header = schema.types.ReleaseRows.extensions.find((e: any) => e.name === 'header')
    header.slots = {
      ...header.slots,
      actionsAt: 'top',
      actions: [
        { id: 'freeze', label: '${item.next_label}', uri: 'x:releases/freeze', when: 'next_step=freeze', primary: true },
        { id: 'deliver', label: 'Deliver a candidate', uri: 'x:releases/deliver', when: 'next_step=deliver', primary: true },
        { id: 'reschedule', label: 'Reschedule', uri: 'x:releases/reschedule', prompt: 'Its new date' },
        { id: 'drop', label: 'Drop', uri: 'x:releases/drop', menu: true },
      ],
    }
    const view = buildUIViews({ schema }).header
    const item = { ...releaseRows, next_step: 'freeze', next_label: 'Finish 3 increments, then Freeze' }
    const out = html(createElement(() => view.renderer({ item }) as ReactElement))
    expect(out).toContain('data-at="top"')
    expect(out).toMatch(/data-primary="true"[^>]*>[\s\S]*?Finish 3 increments, then Freeze/)
    expect(out).not.toContain('Deliver a candidate')
    expect(out).toContain('Reschedule…')
    expect(out).toContain('data-guide="page.menu"')
    expect(out).not.toContain('>Drop<')
  })
})

describe('the legend and a section label', () => {
  test('a square in each word’s tone and its label', () => {
    const out = html(createElement(UIPageView, {
      node: { kind: 'legend', items: ['done', 'running', 'waiting', 'todo', { word: 'blocked', label: 'held up' }] } as never,
    }))
    expect(out).toContain('data-ktw-legend=""')
    for (const s of ['done', 'running', 'awaiting a person', 'to do', 'held up']) expect(out).toContain(s)
    expect((out.match(/data-word=/g) ?? []).length).toBe(5)
  })

  test('a heading drawn as a label is mono and upper case', () => {
    const out = html(createElement(UIPageView, { node: { kind: 'heading', text: 'Ships in this release (8)', variant: 'label' } as never }))
    expect(out).toContain('data-heading="label"')
    expect(out).toContain('text-transform:uppercase')
    expect(out).toContain('Ships in this release (8)')
  })
})

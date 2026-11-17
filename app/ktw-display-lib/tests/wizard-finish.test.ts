// SPDX-License-Identifier: Apache-2.0
// A page wizard declares its completion and cancel actions,
// can keep several steps visible together, and its stepper can be advanced
// by events - the step the producer's data names - rather than by « Suivant ».

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'
import i18next from 'i18next'

import { toUIWizard } from '../src/ui-builders/wizard/ui-wizard.builder.ts'
import { UIWizardView, stepState, wizardIndex } from '../src/ui-builders/wizard/ui-wizard.view.tsx'
import { makeUITexts } from '../src/ui-core/i18n/index.ts'
import { wireFieldRenderers } from '../src/ui-builders/fields/factory.ts'
import { wizardActions, wizardCurrent } from '../src/ui-builders/page/nodes/schema-bound.tsx'
import { UIPageNodeJSONSchema } from '../src/ui-builders/page/ui-page.json-schema.ts'
import { resolveAction } from '../src/ui-builders/page/ui-page.interactive.ts'

wireFieldRenderers()
await i18next.init({ lng: 'en', resources: {} })

const schema = {
  name: 'wizard-finish', version: 1, root: 'Study',
  types: { Study: { type: 'object', label: 'Study', fields: { title: { type: 'string', label: 'Title' } } } },
} as any

const node = (id: string) => ({ kind: 'text', id, content: id }) as any

function wizard() {
  return toUIWizard(schema, {
    type: 'ui-wizard',
    steps: [
      { id: 'subject', title: 'Sujet', fields: ['title'] },
      { id: 'sources', title: 'Sources proposées', node: node('sources-node') },
      { id: 'run', title: "Faire l'étude", node: node('run-node') },
    ],
  } as any, makeUITexts(i18next, 'wizard'))
}

const html = (props: Record<string, unknown>) => renderToStaticMarkup(createElement(MantineProvider, null,
  createElement(UIWizardView as any, { wizard: wizard(), renderNode: (n: any) => createElement('div', { id: n.id }), ...props })))

const states = (out: string) => [...out.matchAll(/data-testid="wizard\.step\.([a-z]+)" data-state="([a-z]+)"/g)].map((m) => `${m[1]}:${m[2]}`)

describe('a page wizard', () => {
  test('stacked, it draws every step together, each marked done, current or next', () => {
    const out = html({ layout: 'stacked', current: 'sources' })
    expect(states(out)).toEqual(['subject:done', 'sources:current', 'run:next'])
    expect(out).toContain('id="sources-node"')
    expect(out).toContain('id="run-node"')
    expect(out).not.toContain('data-testid="wizard.next"')
  })

  test('driven by events, the stepper follows the step the data names and no « Suivant » is drawn', () => {
    expect(wizardIndex(wizard().steps, 'run', 0)).toBe(2)
    expect(wizardIndex(wizard().steps, 'unknown', 1)).toBe(1)
    expect(wizardIndex(wizard().steps, undefined, 1)).toBe(1)
    expect(stepState(0, 1)).toBe('done')
    const out = html({ current: 'run' })
    expect(out).toContain('id="run-node"')
    expect(out).not.toContain('id="sources-node"')
    expect(out).not.toContain('data-testid="wizard.next"')
    expect(wizardCurrent({ currentField: '$.study.step' }, { study: { step: 'sources' } })).toBe('sources')
    expect(wizardCurrent({ current: 'run', currentField: '$.x' }, {})).toBe('run')
    expect(wizardCurrent({}, { step: 'run' })).toBeUndefined()
  })

  test('its completion and cancel are the declared actions, with their words', () => {
    const out = html({ layout: 'stacked', current: 'run', onComplete: () => {}, onCancel: () => {}, labels: { complete: "Faire l'étude", cancel: 'Abandonner' } })
    expect(out).toContain('data-testid="wizard.complete"')
    expect(out).toContain("Faire l&#x27;étude")
    expect(out).toContain('data-testid="wizard.cancel"')
    expect(out).toContain('Abandonner')
    expect(html({ layout: 'stacked' })).not.toContain('data-testid="wizard.complete"')
  })

  test('finishing runs the complete action with the accumulated value; leaving runs cancel', () => {
    const ran: { action: any; item: unknown; extra: any }[] = []
    const runOf = (a: any) => (a ? (item?: unknown, extra?: Record<string, unknown>) => { ran.push({ action: a, item, extra }) } : undefined)
    const complete = { label: "Faire l'étude", uri: 'ktw-example:studies/run', args: { title: '${item.title}' } }
    const cancel = { label: 'Abandonner', uri: 'ktw-example:studies/abandon', confirm: "Abandonner l'étude ?" }
    const { complete: done, cancel: leave } = wizardActions({ complete, cancel } as any, runOf as any)
    done!({ title: 'Facturation Arbor' })
    leave!()
    expect(ran[0].action).toBe(complete)
    expect(resolveAction(ran[0].action, ran[0].item, ran[0].extra).args).toEqual({ title: 'Facturation Arbor' })
    expect(ran[0].extra).toEqual({ value: { title: 'Facturation Arbor' } })
    expect(ran[1].action).toBe(cancel)
    expect(wizardActions({} as any, runOf as any)).toEqual({ complete: undefined, cancel: undefined })
  })

  test('the page grammar declares them', () => {
    const p = (UIPageNodeJSONSchema as any).definitions.Wizard.properties
    for (const k of ['complete', 'cancel', 'layout', 'current', 'currentField']) expect(p[k]).toBeDefined()
  })
})

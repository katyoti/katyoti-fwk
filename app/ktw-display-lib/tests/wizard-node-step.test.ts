// SPDX-License-Identifier: Apache-2.0
// A page wizard step may be any page node, not only form
// fields; « Suivant » accepts the step as it stands.

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { toUIWizard } from '../src/ui-builders/wizard/ui-wizard.builder.ts'
import { UIWizardView } from '../src/ui-builders/wizard/ui-wizard.view.tsx'
import { makeUITexts } from '../src/ui-core/i18n/index.ts'
import i18next from 'i18next'
import { wireFieldRenderers } from '../src/ui-builders/fields/factory.ts'

wireFieldRenderers()
await i18next.init({ lng: 'en', resources: {} })

const schema = {
  name: 'wizard-node-step',
  version: 1,
  root: 'Thing',
  types: { Thing: { type: 'object', label: 'Thing', fields: { title: { type: 'string', label: 'Title' } } } },
} as any

const picker = { kind: 'picker', id: 'sources', search: 'ktw-x:sources/search' } as any

function wizard() {
  return toUIWizard(schema, {
    type: 'ui-wizard',
    steps: [
      { id: 'sources', title: 'Sources', node: picker },
      { id: 'name', title: 'Name', fields: ['title'] },
    ],
  } as any, makeUITexts(i18next, 'wizard'))
}

const html = (props: Record<string, unknown>) =>
  renderToStaticMarkup(createElement(MantineProvider, null, createElement(UIWizardView as any, { wizard: wizard(), ...props })))

describe('a wizard step made of a page node', () => {
  test('is built with its node and no form; a field step keeps its form', () => {
    const w = wizard()
    expect(w.steps[0].node).toEqual(picker)
    expect(w.steps[0].form).toBeUndefined()
    expect(w.steps[1].form).toBeDefined()
    expect(w.steps[1].node).toBeUndefined()
  })

  test('is drawn by the page renderer, with « Suivant » accepting it as it stands', () => {
    const seen: unknown[] = []
    const out = html({ renderNode: (n: unknown) => { seen.push(n); return createElement('div', { id: 'drawn-node' }) } })
    expect(seen).toEqual([picker])
    expect(out).toContain('drawn-node')
    expect(out).toContain('data-testid="wizard.next"')
  })

  test('outside a page it says so, and still offers « Suivant »', () => {
    const out = html({})
    expect(out).not.toContain('drawn-node')
    expect(out).toContain('only inside a page')
    expect(out).toContain('data-testid="wizard.next"')
  })
})

describe('a wizard page as sources answers it', () => {
  test('draws the step of each source, its picker pre-ticked', async () => {
    const { UIPageView } = await import('../src/ui-builders/page/ui-page.view.tsx')
    const page = {
      kind: 'wizard', id: 'source-proposal',
      schema: {
        name: 'source-proposal', version: 1, root: 'Proposal',
        types: { Proposal: { type: 'object', label: 'Sources', fields: {}, extensions: [{ type: 'ui-wizard', navigation: 'linear', steps: [
          { id: 'apps', title: 'Applications', description: 'its interfaces',
            node: { kind: 'picker', id: 'apps', search: 'ktw-agents:sources/search', args: { connector: 'apps' }, layout: 'list',
              selected: [{ key: 'A1', title: 'Arbor ERP', why: 'named in Arbor' }] } },
        ] }] } },
      },
    }
    const out = renderToStaticMarkup(createElement(MantineProvider, null, createElement(UIPageView as any, { node: page })))
    expect(out).toContain('Applications')
    expect(out).toContain('Arbor ERP')
    expect(out).toContain('data-testid="wizard.next"')
  })
})

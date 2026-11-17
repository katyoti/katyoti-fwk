// SPDX-License-Identifier: Apache-2.0
// An enum-multi field edited as option cards (`editAs: cards`): a grid of
// cards, two columns or one on a phone, each with its checkbox, icon, label
// and description; a ticked card checked with an accent border.

import { beforeAll, describe, expect, test } from 'bun:test'

import { wireFieldRenderers } from '../src/ui-builders/fields/factory.ts'
import { buildUIForms } from '../src/ui-builders/form/index.ts'
import { wireFormLayouts } from '../src/ui-builders/form/layouts/factory.ts'
import type { Schema } from '../src/schema/index.ts'
import { byId, drawForm } from './_form-harness.tsx'

const schema = (editAs: string, maxSelected?: number) => ({
  name: 'study', root: 'Study',
  types: {
    Study: { type: 'object', fields: {
      sources: { type: 'array', items: 'Source', label: 'Sources ajoutées à la création', description: 'Les principales sont cochées.', optional: true,
        content: { type: 'enum-multi', editAs, ...(maxSelected ? { maxSelected } : {}), options: [
          { value: 'lx', label: 'Applications', description: 'Propose les applications', icon: 'apps' },
          { value: 'cf', label: 'Pages', description: 'Propose des pages', icon: 'file-text' },
          { value: 'doc', label: 'Documents', description: 'Vous y déposez vos fichiers' },
        ] } },
    }, extensions: [{ type: 'ui-form', name: 'default', groups: [{ fields: ['sources'] }] }] },
    Source: { type: 'string', enum: ['lx', 'cf', 'doc'] },
  },
}) as unknown as Schema

const draw = (value: string[], editAs = 'cards', max?: number) => drawForm(buildUIForms({ schema: schema(editAs, max) })['default'], { sources: value })
const card = (html: string, v: string) => { const c = byId(html, 'input.sources.' + v); return c.slice(0, c.indexOf('</button>')) }

describe('enum-multi edited as cards', () => {
  beforeAll(() => { wireFieldRenderers(); wireFormLayouts() })

  test('one card per option, icon, label, description; the ticked ones checked', () => {
    const html = draw(['lx'])
    for (const v of ['lx', 'cf', 'doc']) expect(card(html, v)).toContain('role="checkbox"')
    const lx = card(html, 'lx')
    expect(lx).toContain('aria-checked="true"')
    expect(lx).toContain('data-option-card="on"')
    expect(lx).toContain('data-option-icon="apps"')
    expect(lx).toContain('Applications')
    expect(lx).toContain('Propose les applications')
    expect(lx).toContain('inset')
    const doc = card(html, 'doc')
    expect(doc).toContain('aria-checked="false"')
    expect(doc).not.toContain('data-option-icon')
    expect(doc).not.toContain('inset')
    expect(html).toContain('Sources ajoutées à la création')
    expect(html).toContain('Les principales sont cochées.')
  })

  test('a grid of two columns, one on a phone; drawn on the body colour', () => {
    const html = draw([])
    expect(html).toContain('mantine-SimpleGrid-root')
    expect(html).toContain('--sg-cols:1;')
    expect(html).toMatch(/@media\(min-width: [^)]*\)\{[^}]*--sg-cols:2;/)
    expect(card(html, 'cf')).toContain('background:var(--mantine-color-body)')
  })

  test('maxSelected reached: the unticked cards are disabled', () => {
    const html = draw(['lx'], 'cards', 1)
    expect(card(html, 'cf')).toContain('disabled=""')
    expect(card(html, 'lx')).not.toContain('disabled=""')
  })

  test('without editAs: cards, no card is drawn', () => {
    expect(byId(draw(['lx'], 'checkboxes'), 'input.sources.lx')).toBe('')
  })
})

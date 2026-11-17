// SPDX-License-Identifier: Apache-2.0
// A form field derived from another while the person types
// (« Code » from the title, kept once edited), a field checked on its line
// by a host method, and a form saved at each change (`save: change`).

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import {
  argFields, askNote, asksCheck, checkAnswer, derivedState, lineAnswers, lineArgs, checkMessage, derivedValue, noteText, lineFields, nextDerived, savesOnChange, shouldSave, slugOf,
} from '../src/ui-builders/form/form-line.ts'
import { setRendererLookup, type FieldRendererFactory } from '../src/ui-builders/fields/index.ts'
import { buildUIForms, UIFormView } from '../src/ui-builders/form/index.ts'
import type { Schema } from '../src/schema/index.ts'
import type { UIFormDef } from '../src/ui-builders/form/ui-form.schema.ts'

describe('a derived field', () => {
  test('a code is the slug of the title', () => {
    expect(slugOf('Étude de l’existant — Prévoyance 2026')).toBe('etude-de-l-existant-prevoyance-2026')
    expect(slugOf('  --Ça va ?--  ')).toBe('ca-va')
    expect(slugOf(42)).toBe('')
    expect(slugOf('a'.repeat(30) + ' ' + 'b'.repeat(30))).toBe('a'.repeat(30) + '-' + 'b'.repeat(17))
    expect(derivedValue('Mon Titre', 'copy')).toBe('Mon Titre')
    expect(derivedValue('Mon Titre')).toBe('mon-titre')
  })

  test('it follows its source until the person edits it, then keeps their value', () => {
    // Empty: it takes the derived value.
    expect(nextDerived('', undefined, 'Prévoyance')).toBe('prevoyance')
    // Still what was derived last: it follows.
    expect(nextDerived('prevoyance', 'prevoyance', 'Prévoyance santé')).toBe('prevoyance-sante')
    // The person typed their own: kept.
    expect(nextDerived('prev', 'prevoyance', 'Prévoyance santé')).toBeUndefined()
    // A value the form was opened with is the person's: kept.
    expect(nextDerived('code-existant', undefined, 'Autre')).toBeUndefined()
    // Nothing changes: no write.
    expect(nextDerived('prevoyance', 'prevoyance', 'Prévoyance')).toBeUndefined()
    // Emptied by the person: it follows again.
    expect(nextDerived('', 'prevoyance', 'Santé')).toBe('sante')
  })
})

describe('a field checked on its line', () => {
  test('the refusal is the line message; a pass shows nothing', () => {
    expect(checkMessage({ ok: false, message: 'Ce code est déjà pris.' })).toBe('Ce code est déjà pris.')
    expect(checkMessage({ ok: false })).toBe('refused')
    expect(checkMessage({ error: 'module exists' })).toBe('module exists')
    expect(checkMessage({ ok: true, message: 'libre' })).toBeUndefined()
    expect(checkMessage(undefined)).toBeUndefined()
  })

  test('an answer reads refused, accepted or a plain note', () => {
    expect(checkAnswer({ ok: false, message: 'Déjà pris.' })).toEqual({ tone: 'refused', text: 'Déjà pris.' })
    expect(checkAnswer({ error: 'down' })).toEqual({ tone: 'refused', text: 'down' })
    expect(checkAnswer({ ok: true, text: 'Libre' })).toEqual({ tone: 'accepted', text: 'Libre' })
    expect(checkAnswer({ text: "Vide : l'identifiant sera tiré du titre." })).toEqual({ tone: 'note', text: "Vide : l'identifiant sera tiré du titre." })
    expect(checkAnswer({ ok: true })).toBeUndefined()
    expect(checkAnswer({ ok: true, text: '  ' })).toBeUndefined()
    expect(checkAnswer('x')).toBeUndefined()
    // A refusal wins over a text it carries.
    expect(checkAnswer({ ok: false, message: 'Non.', text: 'Libre' })?.tone).toBe('refused')
  })

  test('an empty value is asked only when the check says whenEmpty', () => {
    expect(asksCheck({ uri: 'ktw-example:/c' }, 'abc')).toBe(true)
    expect(asksCheck({ uri: 'ktw-example:/c' }, '')).toBe(false)
    expect(asksCheck({ uri: 'ktw-example:/c' }, null)).toBe(false)
    expect(asksCheck({ uri: 'ktw-example:/c', whenEmpty: true }, '')).toBe(true)
    expect(asksCheck(undefined, 'abc')).toBe(false)
  })
})

describe('a derived field told derived or edited', () => {
  test('derived while it holds what was derived last, edited once the person typed', () => {
    expect(derivedState('prevoyance', 'prevoyance')).toBe('derived')
    expect(derivedState('prev', 'prevoyance')).toBe('edited')
    expect(derivedState('code-existant', undefined)).toBe('edited')
    expect(derivedState('', 'prevoyance')).toBeUndefined()
    expect(derivedState(undefined, undefined)).toBeUndefined()
  })

  test('lineAnswers gives each checked field its answer, a derived one told derived or edited', () => {
    const fields = [{ name: 'code', derive: { from: 'title' } }, { name: 'keywords' }]
    const answers = { code: { tone: 'accepted' as const, text: 'Libre' }, keywords: { tone: 'note' as const, text: 'Trois idées.' } }
    expect(lineAnswers(fields, answers, { code: 'prevoyance' }, { code: 'prevoyance' })).toEqual({
      code: { tone: 'accepted', text: 'Libre', derived: 'derived' },
      keywords: { tone: 'note', text: 'Trois idées.' },
    })
    expect(lineAnswers(fields, answers, { code: 'prev' }, { code: 'prevoyance' }).code.derived).toBe('edited')
    expect(lineAnswers(fields, {}, { code: 'x' }, {})).toEqual({})
  })
})

describe('the form saved at each change', () => {
  test('a change the person made is saved once, never while a line shows an error', () => {
    expect(shouldSave({ a: 1 }, '{"a":0}', true, 0)).toBe(true)
    expect(shouldSave({ a: 1 }, '{"a":1}', true, 0)).toBe(false)
    expect(shouldSave({ a: 1 }, '{"a":0}', false, 0)).toBe(false)
    expect(shouldSave({ a: 1 }, '{"a":0}', true, 1)).toBe(false)
  })
})

const schema = {
  name: 'study', root: 'Study',
  types: { Study: { type: 'object', fields: { title: { type: 'string' }, code: { type: 'string' }, keywords: { type: 'string' } },
    extensions: [{ type: 'ui-form', name: 'default', layout: 'tabs', save: 'change', groups: [
      { title: 'Général', fields: ['title', { id: 'code', derive: { from: 'title' }, check: { uri: 'ktw-example:study/code-free', args: { kind: 'study' } } }] },
      { title: 'Périmètre', fields: ['keywords'] },
    ] }] } },
} as unknown as Schema

describe('the form declaration', () => {
  test('the line fields and the save on change are read from the ui-form extension', () => {
    setRendererLookup(() => ({ view: () => () => null, form: () => () => null, header: () => () => null, column: () => () => null }) as FieldRendererFactory)
    const form = buildUIForms({ schema })['default']
    expect(lineFields(form.definition)).toEqual([
      { name: 'code', derive: { from: 'title' }, check: { uri: 'ktw-example:study/code-free', args: { kind: 'study' } } },
    ])
    expect(savesOnChange(form.definition)).toBe(true)
    expect(savesOnChange({ type: 'ui-form' } as UIFormDef)).toBe(false)
    expect(lineFields(undefined)).toEqual([])
    // The view draws such a form (its line behaviours mounted with it).
    const html = renderToStaticMarkup(createElement(MantineProvider, null,
      createElement(UIFormView, { form, item: { title: '' }, isNew: true, onSubmit: () => {} })))
    expect(html).toContain('Général')
  })
})

describe('a note under a field editor', () => {
  test('noteText reads the answer text; none when it has none', () => {
    expect(noteText({ text: 'Deux pages trouvées.' })).toBe('Deux pages trouvées.')
    expect(noteText({ text: '  ' })).toBeUndefined()
    expect(noteText({ ok: true })).toBeUndefined()
    expect(noteText(null)).toBeUndefined()
  })

  test('askNote calls the method with the value over its args and answers the text; nothing on failure', async () => {
    const calls: Array<{ uri: string; input: unknown }> = []
    const call = async (uri: string, input?: unknown) => { calls.push({ uri, input }); return { text: 'Trois idées.' } }
    expect(await askNote(call, { uri: 'ktw-example:/hint', args: { kind: 'k' } }, ['a', 'b'])).toBe('Trois idées.')
    expect(calls).toEqual([{ uri: 'ktw-example:/hint', input: { kind: 'k', value: ['a', 'b'] } }])
    expect(await askNote(async () => { throw new Error('down') }, { uri: 'ktw-example:/hint' }, 'x')).toBeUndefined()
    expect(await askNote(undefined, { uri: 'ktw-example:/hint' }, 'x')).toBeUndefined()
    expect(await askNote(call, undefined, 'x')).toBeUndefined()
  })
})

describe('note and check args naming another field', () => {
  const values = { code: 's-1', owner: { name: 'Ana' }, size: 3, tags: ['a', 'b'] }
  test('lineArgs reads ${...} in the live form values, the value laid over', () => {
    expect(lineArgs({ study: '${code}' }, values, 'kw')).toEqual({ study: 's-1', value: 'kw' })
    expect(lineArgs({ who: '${owner.name}' }, values, 1)).toEqual({ who: 'Ana', value: 1 })
    expect(lineArgs({ n: '${size}', t: '${tags}' }, values, '')).toEqual({ n: 3, t: ['a', 'b'], value: '' })
    expect(lineArgs({ s: 'of ${code}' }, values, 'x')).toEqual({ s: 'of s-1', value: 'x' })
    expect(lineArgs({ x: '${missing}' }, values, 'x')).toEqual({ x: '', value: 'x' })
    expect(lineArgs({ kind: 'k', n: 7, on: true }, values, 'x')).toEqual({ kind: 'k', n: 7, on: true, value: 'x' })
    expect(lineArgs({ value: '${code}' }, values, 'mine')).toEqual({ value: 'mine' })
    expect(lineArgs(undefined, undefined, 'x')).toEqual({ value: 'x' })
  })
  test('argFields lists the fields the args read', () => {
    expect(argFields({ a: '${code}', b: { c: ['${owner.name} / ${code}'] }, d: '${item.id}', e: 'plain' })).toEqual(['code', 'owner'])
    expect(argFields(undefined)).toEqual([])
  })
  test('askNote sends the args read in the form values', async () => {
    let got: unknown
    const call = async (_uri: string, input?: unknown) => { got = input; return { text: 'ok' } }
    expect(await askNote(call, { uri: 'ktw-example:/hint', args: { study: '${code}' } }, 'kw', values)).toBe('ok')
    expect(got).toEqual({ study: 's-1', value: 'kw' })
  })
})

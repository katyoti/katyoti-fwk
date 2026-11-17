// SPDX-License-Identifier: Apache-2.0
// A list card can expand an inline editor (a remark) with its
// own confirm and cancel.

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { UIPageView } from '../src/ui-builders/page/ui-page.view.tsx'
import { EditorForm } from '../src/ui-builders/page/nodes/list-editor.tsx'
import { UIPageNodeJSONSchema } from '../src/ui-builders/page/ui-page.json-schema.ts'
import { editorEvent, editorReady, editorStart, resolveAction } from '../src/ui-builders/page/ui-page.interactive.ts'

const wrap = (el: unknown) => renderToStaticMarkup(createElement(MantineProvider, null, el as never))

const redo = { label: 'Refaire', uri: 'ktw-example:studies/redo-section', args: { section: '${item.id}', remark: '${value}' } }
const editor = { label: 'Refaire avec une remarque', confirm: redo, placeholder: 'Ce qui doit changer…', field: 'remark', when: 'state=draft' }
const sections = [
  { id: '03', title: '03 Architecture métier', state: 'draft', remark: 'Les flux vers la paie passent par l’EAI' },
  { id: '00', title: '00 Contexte', state: 'validated' },
]
const list = { kind: 'list', variant: 'cards', data: sections, editor, item: { kind: 'text', content: '', dataField: '$.item.title' } }

describe('the inline editor of a list card', () => {
  test('is declared on the list, its label and confirm required', () => {
    expect((UIPageNodeJSONSchema as any).definitions.List.properties.editor.required).toEqual(['label', 'confirm'])
  })

  test('each card it is for offers its button, closed; the others none', () => {
    const html = wrap(createElement(UIPageView, { node: list as never, onAction: () => {} }))
    expect(html.match(/data-guide="list.editor.open"/g)?.length).toBe(1)
    expect(html).toContain('Refaire avec une remarque')
    expect(html).not.toContain('data-guide="list.editor.text"')
  })

  test('open, it shows its text box with its own cancel and confirm, the confirm waiting for a text', () => {
    const form = (text: string) => wrap(createElement(EditorForm, { editor: editor as never, text, onText: () => {}, onCancel: () => {}, onConfirm: () => {} }))
    const open = form('Les flux passent par l’EAI')
    expect(open).toContain('data-guide="list.editor.text"')
    expect(open).toContain('data-guide="list.editor.cancel"')
    expect(open).toContain('Annuler')
    expect(open).toMatch(/<button[^>]*data-guide="list.editor.confirm"/)
    expect(open).not.toMatch(/<button[^>]*disabled=""[^>]*data-guide="list.editor.confirm"/)
    expect(form('   ')).toMatch(/<button[^>]*disabled=""[^>]*data-guide="list.editor.confirm"/)
    expect(editorReady({}, '  ')).toBe(false)
    expect(editorReady({ required: false }, '')).toBe(true)
  })

  test('it opens on the element’s remark, and confirming runs its action on the element with the text', () => {
    expect(editorStart(editor, sections[0])).toBe('Les flux vers la paie passent par l’EAI')
    expect(editorStart(editor, sections[1])).toBe('')
    const e = editorEvent(editor as never, sections[0], '  plus de détail sur la paie  ')
    expect(e.action).toBe(redo)
    expect(e.extra).toEqual({ value: 'plus de détail sur la paie' })
    expect(resolveAction(e.action, e.item, e.extra).args).toEqual({ section: '03', remark: 'plus de détail sur la paie' })
  })
})

// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'

import { answerLabel, completionMessage, panelChip, panelListItems, panelTypeName, selectedPanel } from '../src/sections/panels/panelList'
import { ELEMENTS } from '../src/elements'

// The panels screen is the approved display-panels mock-up - the
// list of open panels (title, `<id> · <type>`, the state as a chip) and the
// selected panel (its id, type, owner and sequence, its work, the answer
// sent, Cancel, Open in a tab, the completion line); the mock-up's demo
// column is not delivered; behind its display switch.

const src = (p: string) => readFileSync(join(import.meta.dir, '..', 'src', p), 'utf8')
const A = 'dp-01a16c25-7963-7ab3-b99f-8c1310f81ba1'
const B = 'dp-01a16c25-7963-7ab3-b99f-8c1310f81ba2'
const C = 'dp-01a16c25-7963-7ab3-b99f-8c1310f81ba3'

describe('the list of open panels', () => {
  test('each open panel is its title, its id and type, and its state as the mock-up chip', () => {
    const items = panelListItems([
      { panel: A, title: 'Publication Confluence', type: 'ktw-acme:/panel-type/confluence-publish', state: 'open', seq: 2 },
      { panel: B, title: 'Installer les programmes', type: 'ktw-agents:/panel-type/progress', state: 'asking', waiting: true },
      { panel: C, title: 'Chat', type: 'ktw-agents:/panel-type/chat', state: 'complete' },
      { panel: 'dp-closed', state: 'closed' },
    ])
    expect(items.map((i) => i.id)).toEqual([A, B, C])
    expect(items[0]).toEqual({ id: A, title: 'Publication Confluence', meta: `${A} · confluence-publish`, chip: 'running' })
    expect(items.map((i) => i.chip)).toEqual(['running', 'asking', 'complete'])
    expect(panelListItems(undefined)).toEqual([])
  })
  test('a state in words: asking while a question waits, complete once ended, running otherwise', () => {
    expect(panelChip('open')).toBe('running')
    expect(panelChip(undefined)).toBe('running')
    expect(panelChip('open', true)).toBe('asking')
    expect(panelChip('asking')).toBe('asking')
    expect(panelChip('complete')).toBe('complete')
    expect(panelTypeName('ktw-agents:/panel-type/progress')).toBe('progress')
    expect(panelTypeName(undefined)).toBe('')
  })
  test('the selection stays while its panel is listed, else the first', () => {
    const items = panelListItems([{ panel: A }, { panel: B }])
    expect(selectedPanel(items, B)).toBe(B)
    expect(selectedPanel(items, 'dp-gone')).toBe(A)
    expect(selectedPanel(items)).toBe(A)
    expect(selectedPanel([], A)).toBeUndefined()
  })
})

describe('the selected panel', () => {
  test('the answer sent is said as the question offered it', () => {
    const actions = [{ name: 'overwrite', label: 'Overwrite Confluence' }, { name: 'keep', label: 'Keep Confluence’s version' }]
    expect(answerLabel(actions, { action: 'keep' })).toBe('Keep Confluence’s version')
    expect(answerLabel(actions, {})).toBe('Overwrite Confluence')
    expect(answerLabel(undefined, { text: '  later  ' })).toBe('later')
    expect(answerLabel([], { text: ' ' })).toBeUndefined()
  })
  test('the completion line says what the work said when it ended', () => {
    expect(completionMessage({ status: 'ok', result: '5 pages published to space ARCHI.' })).toBe('5 pages published to space ARCHI.')
    expect(completionMessage({ status: 'ok', result: { ok: true, message: '4 programs installed.' } })).toBe('4 programs installed.')
    expect(completionMessage({ status: 'ok', result: { panel: A, value: { message: 'synced' } } })).toBe('synced')
    expect(completionMessage({ status: 'failed', result: { error: 'pandoc not found' } })).toBe('pandoc not found')
    expect(completionMessage({ status: 'ok', result: { outputPath: 'a.pdf' } })).toBeUndefined()
    expect(completionMessage(undefined)).toBeUndefined()
  })
  test('the panel draws the mock-up: its meta line, the answer sent, Cancel and Open in a tab, the completion line', () => {
    const panel = src('sections/panels/DisplayPanel.tsx')
    expect(panel).toContain("variant?: 'tab' | 'inline' | 'board'")
    expect(panel).toContain('data-guide="panel.meta"')
    expect(panel).toContain('data-guide="panel.answered"')
    expect(panel).toContain('data-guide="panel.cancel"')
    expect(panel).toContain('data-guide="panel.open-tab"')
    expect(panel).toContain("t('displayPanel.done.okSaid', { message: done })")
  })
})

describe('the panels screen', () => {
  test('it is the list and the selected panel, without the mock-up demo column', () => {
    // Its code, its comments aside (they name what is left out).
    const screen = src('sections/panels/PanelsScreen.tsx').replace(/\/\*[\s\S]*?\*\//g, '').replace(/\/\/.*$/gm, '')
    expect(screen).toContain('data-element="screen:panels"')
    expect(screen).toContain('data-guide="panels.list"')
    expect(screen).toContain('role="tab" aria-selected={on}')
    expect(screen).toContain('variant="board"')
    for (const demo of ['Simulate', 'Reset', 'hist', 'Disconnect']) expect(screen).not.toContain(demo)
  })
  test('the ribbon opens it for everyone, and its elements are declared', () => {
    const ribbon = src('sections/shell/IconRibbon.tsx')
    expect(ribbon).not.toContain("useDisplaySwitch('in038')")
    expect(ribbon).toContain("{!hidden('rail:panels') && (")
    expect(ribbon).toContain("kind: 'panels'")
    expect(src('sections/shell/MainPaneTabs.tsx')).toContain("case 'panels':     return displayedUniverseName ? <PanelsScreen")
    const ids = ELEMENTS.map((e) => e.id)
    expect(ids).toContain('rail:panels')
    expect(ids).toContain('screen:panels')
  })
  test('its words are in both languages', () => {
    for (const lang of ['en', 'fr']) {
      const c = JSON.parse(src(`locales/${lang}/components.json`)).displayPanel
      for (const k of ['running', 'asking', 'complete']) expect(typeof c.chip[k]).toBe('string')
      for (const k of ['aria', 'empty', 'none']) expect(typeof c.list[k]).toBe('string')
      // The mock-up's « type X » reads « template X » / « modèle X »: no jargon at Essentiel (a difference listed for the person).
      expect(c.meta.type).toContain('{{type}}')
      expect(typeof c.ask.yours).toBe('string')
      expect(c.done.okSaid).toContain('{{message}}')
      const s = JSON.parse(src(`locales/${lang}/shell.json`)).ribbon
      expect(typeof s.panels).toBe('string')
      expect(typeof s.panelsNeedsUniverse).toBe('string')
    }
  })
})

// SPDX-License-Identifier: Apache-2.0
// The planning renderers speak the GUI's language: the marks,
// the gates, the progress bar's tooltip and the legend read the library's
// `planning` namespace through the shared i18next instance — French in fr,
// English in en; a label the server sends still wins; data-* keep the raw word.

import { afterAll, beforeAll, describe, expect, test } from 'bun:test'
import { createElement, type ReactElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'
import i18next from 'i18next'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'

import { initI18n } from '../src/ui-core/i18n/index.ts'
import { GateDots, IdText, SegmentBar, StageChip, StatusChip, Stepper } from '../src/ui-builders/fields/_planning/marks.tsx'
import fr from '../src/ui-builders/fields/_planning/i18n/fr.json'
import { HostServicesContext } from '../src/ui-core/host/index.tsx'
import { LegendRenderer } from '../src/ui-builders/page/nodes/atomic.tsx'

const html = (el: ReactElement) => renderToStaticMarkup(createElement(MantineProvider, null, el))
const gates = [{ name: 'increments', passed: false, why: '3 open' }, { name: 'e2e', passed: true }]
const changeLog = [{ name: 'change-log', why: 'needs a candidate' }]
const stepper = [{ name: 'planned', state: 'done' }, { name: 'frozen', state: 'current' }, { name: 'released', state: 'todo' }]
const steps = [{ name: 'intake', state: 'done' }, { name: 'implement', state: 'current' }, { name: 'module-update', state: 'todo' }]

// goWords reads planification's French standing and stage words — the map
// standingWords of internal/extensions/planification/api/standing.go, keyed
// by its constants (standing.go, release.go) — so the planning namespace
// speaks the words every Go view speaks (StandingLabel).
function goWords(): Record<string, string> {
  const api = join(import.meta.dir, '../../../internal/extensions/planification/api')
  const src = ['standing.go', 'release.go'].map((f) => readFileSync(join(api, f), 'utf8')).join('\n')
  const consts: Record<string, string> = {}
  for (const m of src.matchAll(/^\s*(?:const\s+)?(\w+)\s*=\s*"([^"]+)"/gm)) consts[m[1]] = m[2]
  const map = src.slice(src.indexOf('var standingWords'))
  const words: Record<string, string> = {}
  for (const m of map.slice(0, map.indexOf('\n}')).matchAll(/^\s*(\w+):\s*\{"[^"]*",\s*"([^"]*)"/gm)) words[consts[m[1]]] = m[2]
  return words
}

let before = 'en'
beforeAll(async () => {
  const i = await initI18n({ language: 'en' })
  before = i.language || 'en'
})
// The instance is global: give the other suites back the language they had.
afterAll(async () => { await i18next.changeLanguage(before) })

describe('in French', () => {
  beforeAll(async () => { await i18next.changeLanguage('fr') })

  test('a status and a stage chip read French, data-* keep the raw word', () => {
    const s = html(createElement(StatusChip, { value: 'planned' }))
    expect(s).toContain('>prévue<')
    expect(s).toContain('data-ktw-mark="status"')
    expect(html(createElement(StatusChip, { value: 'on-hold' }))).toContain('>en pause<')
    expect(html(createElement(StatusChip, { value: 'running' }))).toContain('>en cours<')
    expect(html(createElement(StageChip, { value: 'frozen' }))).toContain('>gelée<')
  })

  test("the status and stage words are planification's (StandingLabel)", () => {
    const go = goWords()
    const keys = ['defined', 'running', 'delivering', 'on-hold', 'done', 'shipped', 'dropped', 'planned', 'frozen', 'candidate', 'released']
    expect(Object.keys(go).sort()).toEqual([...keys].sort())
    for (const k of keys) expect([k, (fr.status as Record<string, string>)[k]]).toEqual([k, go[k]])
    for (const k of ['planned', 'frozen', 'candidate', 'released']) expect([k, (fr.stage as Record<string, string>)[k]]).toEqual([k, go[k]])
    expect(fr.gate['change-log']).toBe('Journal des changements')
  })

  test("a gate dot's tooltip and the stepper's pills read French", () => {
    expect(html(createElement(GateDots, { value: changeLog }))).toContain('title="Journal des changements: needs a candidate"')
    const s = html(createElement(Stepper, { value: stepper }))
    expect(s).toContain('>prévue<')
    expect(s).toContain('>gelée<')
    expect(s).toContain('>publiée<')
    expect(s).toContain('data-name="frozen"')
    const r = html(createElement(Stepper, { value: steps }))
    expect(r).toContain('>accueil<')
    expect(r).toContain('>réalisation<')
    expect(r).toContain('>mise à jour du module<')
  })

  test('a capitalised word keeps its capital', () => {
    expect(html(createElement(StatusChip, { value: 'Running' }))).toContain('>En cours<')
    expect(html(createElement(StageChip, { value: 'Frozen' }))).toContain('>Gelée<')
    expect(html(createElement(Stepper, { value: [{ name: 'Implement', state: 'todo' }] }))).toContain('>Réalisation<')
    expect(html(createElement(StatusChip, { value: 'Unknown-word' }))).toContain('>Unknown-word<')
  })

  test('the gates, the bar tooltip and the legend read French', () => {
    const g = html(createElement(GateDots, { value: gates, labels: true }))
    expect(g).toContain('Incréments terminés (3 open)')
    expect(g).toContain('De bout en bout')
    const bar = html(createElement(SegmentBar, { value: [1, 0, 0, 2, 0] }))
    expect(bar).toContain('title="1 terminé · 0 en cours · 0 en attente · 2 à faire · 0 bloqué"')
    const legend = html(createElement(LegendRenderer, { node: { type: 'legend', items: ['todo', 'waiting', 'done', { word: 'blocked', label: 'server says' }] } as never }))
    expect(legend).toContain('à faire')
    expect(legend).toContain('en attente d&#x27;une personne')
    expect(legend).toContain('terminé')
    expect(legend).toContain('server says')
    expect(legend).not.toContain('to do')
    expect(legend).toContain('data-word="todo"')
  })

  test('an id reads « Ouvrir X »', () => {
    const s = html(createElement(HostServicesContext.Provider, { value: { navigate: () => {} } },
      createElement(IdText, { value: 'm.RQ-074' })))
    expect(s).toContain('title="Ouvrir m.RQ-074"')
  })
})

describe('in English', () => {
  beforeAll(async () => { await i18next.changeLanguage('en') })

  test('the same marks read English', () => {
    expect(html(createElement(StatusChip, { value: 'planned' }))).toContain('>planned<')
    expect(html(createElement(StageChip, { value: 'frozen' }))).toContain('>frozen<')
    const g = html(createElement(GateDots, { value: gates, labels: true }))
    expect(g).toContain('Increments done (3 open)')
    expect(g).toContain('End to end')
    expect(html(createElement(SegmentBar, { value: [1, 0, 0, 2, 0] })))
      .toContain('title="1 done · 0 running · 0 waiting · 2 to do · 0 blocked"')
    const legend = html(createElement(LegendRenderer, { node: { type: 'legend', items: ['todo', 'waiting'] } as never }))
    expect(legend).toContain('to do')
    expect(legend).toContain('awaiting a person')
  })

  test("a gate dot's tooltip, the stepper's pills and a capitalised word read English", () => {
    expect(html(createElement(GateDots, { value: changeLog }))).toContain('title="Change log: needs a candidate"')
    const s = html(createElement(Stepper, { value: stepper }))
    expect(s).toContain('>planned<')
    expect(s).toContain('>frozen<')
    expect(html(createElement(Stepper, { value: steps }))).toContain('>module-update<')
    expect(html(createElement(StatusChip, { value: 'Running' }))).toContain('>Running<')
    expect(html(createElement(StatusChip, { value: 'running' }))).toContain('>running<')
    expect(html(createElement(StatusChip, { value: 'on-hold' }))).toContain('>on-hold<')
  })
})

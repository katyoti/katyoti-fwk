// SPDX-License-Identifier: Apache-2.0
// The increment's Blockers tab - the tree planification draws from
// `increments blockers <id>` (fixtures/increment-blockers.json, written and
// compared by the Go test TestTheIncrementsBlockersTabDrawsEachKindWithWhoActs)
// - drawn by the display library as its mock-up
// (modules/research-planification-resilience/blockers-mockup.html): the state
// « bloqué · N blocages » with Recalculer and the person's next action, the
// verdict as a red callout with its counts and the repairs, the six groups
// K1-K6 with their counts and « Rien. » for the empty one, each root a card
// striped in its kind's colour with its reason in French, who acts and the
// act it calls for, and « Réparé automatiquement » folded last.

import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { UIPageView } from '../src/ui-builders/page/ui-page.view.tsx'

const tree = JSON.parse(readFileSync(new URL('./fixtures/increment-blockers.json', import.meta.url), 'utf8'))
const html = renderToStaticMarkup(createElement(MantineProvider, null,
  createElement(UIPageView, { node: tree as never, onAction: () => {} })))
const text = html.replace(/<[^>]+>/g, ' ').replace(/&#x27;|&#39;/g, "'").replace(/\s+/g, ' ')

describe("the increment's Blockers tab", () => {
  test('the state, Recalculer and the next action come first', () => {
    expect(text).toContain('bloqué · 6 blocages')
    expect(html).toContain('data-element="button:blockers-recompute"')
    expect(html).toContain('data-element="button:blockers-next"')
    expect(text.indexOf('bloqué · 6 blocages')).toBeLessThan(text.indexOf("Cet incrément ne peut pas finir aujourd'hui."))
  })

  test('the verdict: it cannot end, the counts, the repairs', () => {
    expect(html).toContain('data-guide-name="red"')
    expect(text).toContain("Cet incrément ne peut pas finir aujourd'hui.")
    for (const c of ['Pour vous 3', 'Agents 1', 'Extérieur 1', 'Incidents 2', 'Réparés 1']) expect(text).toContain(c)
  })

  test('six groups in reading order, the empty one says Rien.', () => {
    const groups = ['K1 · Vos réponses', 'K2 · Vos actions', 'K3 · Travail prêt pour un agent', "K4 · Travail en attente d'autre chose", "K5 · En dehors de l'incrément", 'K6 · Incidents']
    const at = groups.map(g => text.indexOf(g))
    expect(at.every(i => i >= 0)).toBe(true)
    expect([...at].sort((a, b) => a - b)).toEqual(at)
    const k4 = text.slice(at[3], at[4])
    expect(k4).toContain('Rien.')
  })

  test('each root a card striped in its kind, its reason in French, who acts and its act', () => {
    for (const [id, tone] of [['blocker-K1-shop.RQ-051', 'yellow'], ['blocker-K2-shop.F-049', 'yellow'], ['blocker-K3-docs.RQ-012', 'orange'], ['blocker-K5-shop.RQ-049', 'grape'], ['blocker-K6-ui.F-050', 'red'], ['blocker-K6-shop.F-011', 'red']]) {
      expect(html).toMatch(new RegExp(`data-element="card:${id.replace(/\./g, '\\.')}"[^>]*data-stripe="${tone}"|data-stripe="${tone}"[^>]*data-element="card:${id.replace(/\./g, '\\.')}"`))
    }
    expect(text).toContain('sa question attend une réponse dans mail.E-12')
    expect(text).toContain('Qui agit :')
    for (const w of ['Répondre', 'Confirmer', 'Lancer une boucle roadmap.RL-014', 'Ouvrir la demande', 'Créer la demande sur shop.F-032', "Retirer l'attente", 'Planifier', "Sortir de l'incrément", 'Système · le réconciliateur', 'Responsable · core.RQ-053']) expect(text).toContain(w)
    expect(html).toContain('data-element="button:blocker-open-shop.RQ-051"')
    expect(html).toContain('data-element="button:blocker-alt-ui.F-050"')
  })

  test('what the reconciler repaired is folded last', () => {
    expect(text).toContain('Réparé automatiquement (1) - par le réconciliateur, sans agent')
    expect(text.indexOf('Réparé automatiquement')).toBeGreaterThan(text.indexOf('K6 · Incidents'))
  })
})

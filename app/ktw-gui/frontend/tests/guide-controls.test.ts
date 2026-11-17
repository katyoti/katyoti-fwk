// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { controlLabel, sayControls, type ControlLabelSources } from '../src/guide/controls'
import { GUIDE_ANCHORS } from '../src/guide/anchors'

// A recipe's text names controls as {control:<anchor>}; the player says the
// label the GUI shows: the control on screen first, else its registry key,
// else the instance name.
const fr: Record<string, string> = {
  'shell:ribbon.inbox': 'Inbox',
  'universes:create.submit': 'Créer l’univers',
  'shell:project.objects': 'OBJETS',
}
const offScreen: ControlLabelSources = {
  live: () => null,
  translate: (k) => fr[k] ?? null,
}

describe('a recipe names controls by anchor', () => {
  test('off screen, the registry key in the GUI language, in bold', () => {
    expect(sayControls('Open the {control:ribbon.inbox} from the ribbon.', offScreen))
      .toBe('Open the **Inbox** from the ribbon.')
    expect(sayControls('Press {control:create-universe.submit}.', offScreen)).toBe('Press **Créer l’univers**.')
  })
  test('an instance fills the key of a list anchor', () => {
    expect(sayControls('The {control:project.tab|objects} tab.', offScreen)).toBe('The **OBJETS** tab.')
  })
  test('on screen, the control’s own label wins (a contribution’s text, a toggle’s word)', () => {
    const live: ControlLabelSources = {
      ...offScreen,
      live: (a, n) => (a === 'objtab.panel:agent-context' ? 'Contexte de l’agent' : a === 'page.action' && n === 'ktw-demo:signoff/approve' ? 'Approuver' : null),
    }
    expect(sayControls('Open its {control:objtab.panel:agent-context} tab.', live)).toBe('Open its **Contexte de l’agent** tab.')
    expect(sayControls('Press {control:page.action|ktw-demo:signoff/approve}.', live)).toBe('Press **Approuver**.')
  })
  test('no live label and no key: the instance, else the anchor, by its last segment', () => {
    expect(controlLabel('page.action', 'ktw-demo:signoff/approve', offScreen)).toBe('approve')
    expect(controlLabel('task.card', 'leanix-drawio-showcase', offScreen)).toBe('leanix-drawio-showcase')
    expect(controlLabel('objtab.panel:agent-context', undefined, offScreen)).toBe('agent-context')
  })
  test('a missing translation falls through, and markdown in a label is escaped', () => {
    expect(controlLabel('ribbon.chat', undefined, offScreen)).toBe('chat')
    const starred: ControlLabelSources = { live: () => 'a*b_c', translate: () => null }
    expect(sayControls('{control:ribbon.chat}', starred)).toBe('**a\\*b\\_c**')
  })
  test('a text without placeholders is unchanged; a malformed one is left as written', () => {
    expect(sayControls('Pick an *object* in the tree.', offScreen)).toBe('Pick an *object* in the tree.')
    expect(sayControls('Press {control:Ribbon Inbox}.', offScreen)).toBe('Press {control:Ribbon Inbox}.')
  })
  test('every registry label is a namespaced key', () => {
    for (const [id, a] of Object.entries(GUIDE_ANCHORS)) {
      if (a.label) expect(`${id} ${a.label}`).toMatch(/^\S+ [a-z]+:[A-Za-z0-9.{}]+$/)
    }
  })
})

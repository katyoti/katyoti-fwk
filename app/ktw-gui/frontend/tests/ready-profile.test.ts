// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { headerState, profileNamed, tabState, type ReadyInputs } from '../src/sections/ready/readiness'
import { draftFromSuggestion, draftOf, draftSavable, EMPTY_DRAFT } from '../src/sections/profile/profileDraft'
import type { GetOutput as ProfileOutput } from '../src/ktsp/profile'
import type { Profile } from '../src/store/profileStore'

// The first step of « Prêt à démarrer » asks the profile in place - handle,
// name, e-mail - and is green only once it is filled; the profile menu's view
// is a way back to it, never the only way.

const src = join(import.meta.dir, '..', 'src')
const person: ProfileOutput = { id: 'jdoe', firstName: 'Jane', lastName: 'Doe', email: 'jane.doe@example.com', source: 'local' } as ProfileOutput
const ready = (profile: ProfileOutput | undefined): ReadyInputs => ({
  loading: false, errors: {}, profile,
  projectsFolder: { folder: '/home/me/Katyoti', chosen: true } as ReadyInputs['projectsFolder'],
  projects: ['/home/me/Katyoti/main'],
})

describe('the profile step', () => {
  test('red until a handle, a name and an e-mail are said; green once they are', () => {
    expect(tabState(ready({ ...person, source: 'placeholder' } as ProfileOutput), 'profile')).toBe('todo')
    expect(tabState(ready({ ...person, email: '' }), 'profile')).toBe('todo')
    expect(tabState(ready({ ...person, firstName: '', lastName: '' }), 'profile')).toBe('todo')
    expect(tabState(ready({ ...person, id: '' }), 'profile')).toBe('todo')
    expect(profileNamed(person)).toBe(true)
    expect(tabState(ready(person), 'profile')).toBe('ok')
  })

  test('the form can be saved only once it says who the person is', () => {
    expect(draftSavable(EMPTY_DRAFT)).toBe(false)
    expect(draftSavable({ ...EMPTY_DRAFT, id: 'me', email: 'me@example.com' })).toBe(false)
    expect(draftSavable({ ...EMPTY_DRAFT, id: 'me', lastName: 'Me', email: ' ' })).toBe(false)
    expect(draftSavable({ ...EMPTY_DRAFT, id: 'me', lastName: 'Me', email: 'me@example.com' })).toBe(true)
  })

  test('it opens pre-filled from this computer while no profile is set, on the profile as it is otherwise', () => {
    const placeholder = { id: 'john-doe', email: 'john@doe', source: 'placeholder' } as Profile
    expect(draftOf(placeholder)).toEqual(EMPTY_DRAFT)
    expect(draftFromSuggestion({ id: 'me', firstName: 'Ma', lastName: 'Me', emails: [{ email: 'me@example.com', source: 'git' }] } as never))
      .toEqual({ id: 'me', firstName: 'Ma', lastName: 'Me', email: 'me@example.com', language: '' })
    expect(draftOf({ ...person, language: 'fr' } as Profile)).toEqual({ id: 'jdoe', firstName: 'Jane', lastName: 'Doe', email: 'jane.doe@example.com', language: 'fr' })
  })

  test('the step edits the profile in place, saved through the profile service - the modal is never the way to fill it', () => {
    const screen = readFileSync(join(src, 'sections', 'ready', 'ReadyScreen.tsx'), 'utf8')
    const tab = screen.slice(screen.indexOf('function ProfileTab('), screen.indexOf('function ProjectsFolderSection'))
    expect(tab).toContain('<ProfileForm')
    expect(tab).toContain('section:ready/profile-form')
    expect(tab).toContain('action:ready/profile-save')
    expect(tab).toContain('fieldElement="field:ready/profile"')
    expect(tab).not.toContain('openModal')
    const form = readFileSync(join(src, 'sections', 'profile', 'ProfileForm.tsx'), 'utf8')
    expect(form).toMatch(/useProfileStore\(\(s\) => s\.save\)/)
    expect(form).toContain('await save(draft)')
    for (const name of ['profile-handle', 'profile-first-name', 'profile-last-name', 'profile-email']) expect(form).toContain(`name="${name}"`)
    for (const f of ['handle', 'first-name', 'last-name', 'email']) expect(form).toContain(`data-element={field('${f}')}`)
    // The profile menu's view is the same form: a way back to it.
    expect(readFileSync(join(src, 'sections', 'profile', 'ProfileModal.tsx'), 'utf8')).toContain('<ProfileForm')
  })
})

// « Prêt à démarrer »'s header says everything is in place only when every
// tab is green: a red Profile tab beside green Programmes and
// Clés no longer reads « pleinement opérationnel ».
describe('the screen header', () => {
  const green = (profile: ProfileOutput | undefined): ReadyInputs => ({
    ...ready(profile),
    doctor: [], plan: { platform: 'windows', programs: [] } as unknown as ReadyInputs['plan'], keys: { passwordNeeded: false, remaining: [], universes: [] } as unknown as ReadyInputs['keys'],
  })

  test('every tab green: everything is in place', () => {
    expect(headerState(green(person))).toEqual({ state: 'ready', left: [] })
  })

  test('the profile tab red: not « everything is in place », the tab named', () => {
    const h = headerState(green({ ...person, email: '' }))
    expect(h.state).toBe('tabs-left')
    expect(h.left).toEqual(['profile'])
  })

  test('a part not ready keeps the summary\'s own state', () => {
    const inp = green(person)
    inp.keys = { passwordNeeded: false, remaining: [{ name: 'K', scope: 'universe', universeHome: '/u', configSet: 'c@1' }], universes: ['/u'] } as unknown as ReadyInputs['keys']
    expect(headerState(inp).state).toBe('not-ready')
  })

  test('every header state has its sentence in English and French', () => {
    for (const lang of ['en', 'fr']) {
      const msgs = JSON.parse(readFileSync(join(src, 'locales', lang, 'ready.json'), 'utf8'))
      for (const k of ['checking', 'ready', 'notReady', 'unknown', 'tabsLeft']) expect(msgs.state[k] ?? '').not.toBe('')
    }
  })
})

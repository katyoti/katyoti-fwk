// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { UNAVAILABLE_PROFILE_ENTRIES, profileEntryAvailable } from '../src/sections/shell/profileMenuEntries'

// Usage & Billing, Notifications and Theme do not work yet - kept
// in the user menu, greyed, unclickable, « Bientôt disponible » in their
// tooltip; the others work as before. Availability is data, read once: an
// entry works again by leaving the list, not by changing the menu.
describe('the user menu entries not available yet', () => {
  test('are the three that do not work, the others available', () => {
    for (const id of ['usage', 'notifications', 'theme']) expect(profileEntryAvailable(id)).toBe(false)
    for (const id of ['profile', 'apiKeys', 'appearance', 'help', 'signOut']) expect(profileEntryAvailable(id)).toBe(true)
  })

  test('an entry leaving the list works again, the menu unchanged', () => {
    const later = new Set([...UNAVAILABLE_PROFILE_ENTRIES].filter((id) => id !== 'theme'))
    expect(profileEntryAvailable('theme', later)).toBe(true)
  })

  test('the menu draws each of them through the list, disabled with the tooltip, in both languages', () => {
    const menu = readFileSync(join(import.meta.dir, '../src/sections/shell/IconRibbon.tsx'), 'utf8')
    for (const id of ['usage', 'notifications', 'theme']) expect(menu).toContain(`soonItem('${id}'`)
    expect(menu).toMatch(/profileEntryAvailable\(id\)[\s\S]*profile\.soon[\s\S]*disabled/)
    const fr = JSON.parse(readFileSync(join(import.meta.dir, '../src/locales/fr/shell.json'), 'utf8'))
    const en = JSON.parse(readFileSync(join(import.meta.dir, '../src/locales/en/shell.json'), 'utf8'))
    expect(fr.profile.soon).toBe('Bientôt disponible')
    expect(en.profile.soon).toBeTruthy()
  })
})

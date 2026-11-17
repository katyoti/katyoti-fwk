// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'

// The pane's toolbar button reads « + New » - once:
// its plus is the icon, so the label says « New » / « Nouveau » - and the menu
// it opens closes on Escape and on a click outside, each card showing the
// type's icon, its name and one line.
const src = (f: string) => readFileSync(join(import.meta.dir, '..', 'src', f), 'utf8')
const label = (lang: string) => JSON.parse(src(`locales/${lang}/workspace.json`)).objectTab.newButton as string

describe('« + New » reads once and its cards close', () => {
  test('the button draws its plus as the icon, so its label does not repeat it', () => {
    const menu = src('sections/workspace/NewMenu.tsx')
    expect(menu).toMatch(/<Popover\.Target>\s*<Button[^>]*leftSection=\{<IconPlus/)
    expect(menu).toContain("{t('objectTab.newButton')}")
    for (const lang of ['en', 'fr']) {
      expect(label(lang).trim().startsWith('+')).toBe(false)
    }
    expect(label('en')).toBe('New')
    expect(label('fr')).toBe('Nouveau')
  })
  test('the menu closes on Escape and on a click outside; each card has its icon, name and one line', () => {
    const menu = src('sections/workspace/NewMenu.tsx')
    expect(menu).toContain('closeOnEscape')
    expect(menu).toContain('closeOnClickOutside')
    // The former « + » cards: a grid of cards, each its icon,
    // its name and one line.
    expect(menu).toContain('<SimpleGrid cols={2}')
    expect(menu).toContain('<Icon size={18}')
    expect(menu).toContain('lineClamp={2}')
  })
})

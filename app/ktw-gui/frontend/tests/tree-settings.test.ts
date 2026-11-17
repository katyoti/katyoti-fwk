// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'fs'
import { DEFAULT_SETTINGS, mergeLoadedSettings } from '../src/types/settings'
import { treeRowBadges, TREE_SETTINGS, TREE_SETTING_SHOW_TYPES } from '../src/sections/workspace/treeSettings'

// The projects tree shows each object by its name and type,
// without its code; « Show kinds » in the tree's settings menu is a GUI
// setting (GuiSettings.yaml), read back at the next start.
const src = (path: string) => readFileSync(new URL(`../src/${path}`, import.meta.url), 'utf8')

describe('the tree setting « Show kinds » is a GUI setting', () => {
  test('types are shown by default', () => {
    expect(DEFAULT_SETTINGS.tree.hideTypes).toBe(false)
    expect(mergeLoadedSettings(undefined).tree.hideTypes).toBe(false)
  })
  test('a hidden choice read at start comes back', () => {
    const s = mergeLoadedSettings({ tree: { hideTypes: true } } as never)
    expect(s.tree.hideTypes).toBe(true)
    // A file written before the setting keeps the types.
    expect(mergeLoadedSettings({ general: { level: 'expert' } } as never).tree.hideTypes).toBe(false)
  })
  test('the setting goes through the persisted store, never the browser storage', () => {
    const hook = src('hooks/useTreePrefs.ts')
    expect(hook).toContain("updateSettings('tree', { hideTypes: !show })")
    expect(hook).not.toContain('localStorage')
    expect(src('hooks/useSettingsPersistence.ts')).toContain('SaveGuiSettings')
  })
  test('the Wails model carries it to GuiSettings.yaml', () => {
    const models = readFileSync(new URL('../wailsjs/go/models.ts', import.meta.url), 'utf8')
    expect(models).toContain('this.tree = this.convertValues(source["tree"], TreeSettings)')
    expect(models).toContain('this.hideTypes = source["hideTypes"]')
  })
})

describe('a tree row', () => {
  test('its type badge follows the setting and the universe', () => {
    expect(treeRowBadges({ hideTypes: false }, false).type).toBe(true)
    expect(treeRowBadges({ hideTypes: true }, false).type).toBe(false)
    expect(treeRowBadges({ hideTypes: false }, true).type).toBe(false)
    expect(treeRowBadges(undefined, false).type).toBe(true)
  })
  test('draws no code badge and the type badge only when shown', () => {
    const tree = src('sections/workspace/WorkspaceObjectsTree.tsx')
    expect(tree).not.toContain('nodeProps?.module as string')
    expect(tree).not.toContain('{mod}')
    expect(tree).toContain('treeRowBadges(useTreePrefs().settings, hideBadges)')
    expect(tree).toContain('{badges.type && (')
  })
  test('the settings menu sits in the project panel header, by element ids', () => {
    expect(TREE_SETTINGS).toBe('tree.settings')
    expect(TREE_SETTING_SHOW_TYPES).toBe('tree.settings.show-types')
    const menu = src('sections/workspace/TreeSettingsMenu.tsx')
    expect(menu).toContain('data-guide={TREE_SETTINGS}')
    expect(menu).toContain('data-guide={TREE_SETTING_SHOW_TYPES}')
    expect(src('sections/shell/ProjectPanel.tsx')).toContain("tab === 'objects' && <TreeSettingsMenu />")
  })
  test('both languages name the menu and its item', () => {
    for (const lang of ['en', 'fr']) {
      const tree = JSON.parse(src(`locales/${lang}/workspace.json`)).tree
      expect(tree.settings).toBeTruthy()
      expect(tree.showTypes).toBeTruthy()
    }
  })
})

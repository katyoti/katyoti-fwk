// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { panelMenuItems } from '../src/sections/shell/panelMenu'
import { isLeftOut } from '../src/sections/shell/universeGui'
import { CHECK_INSTALL_ELEMENT, ELEMENTS } from '../src/elements'

// « Vérifier l'installation » - always there, in the universe's
// « … » menus, it opens the full « Prêt à démarrer » screen (every tab), at
// every display level, never left out by a tag's view.
const src = (rel: string) => readFileSync(new URL(rel, import.meta.url), 'utf8')

// A solution's tag view, written here: the ids its tag lists, at every
// level (its view, at Expert) - never « Vérifier l'installation » itself.
const solutionView = {
  all: false,
  allowed: ['rail:chat', 'rail:inbox', 'home:actions', 'tab:information', 'tab:contents'],
}

describe('« Vérifier l\'installation »', () => {
  test('the left panel\'s « … » menu always offers it, with or without an attached project', () => {
    for (const attached of [[], ['D:/p']]) {
      expect(panelMenuItems(attached).find((i) => i.key === 'checkInstall')).toEqual({ key: 'checkInstall', disabled: false })
    }
  })

  test("it is an element of the GUI, never left out at any level by a solution's view", () => {
    expect(ELEMENTS.some((e) => e.id === CHECK_INSTALL_ELEMENT)).toBe(true)
    expect(solutionView.allowed).toContain('rail:chat')
    expect(solutionView.allowed).not.toContain(CHECK_INSTALL_ELEMENT)
    for (const level of ['essential', 'standard', 'expert'] as const) {
      expect(isLeftOut(CHECK_INSTALL_ELEMENT, level, solutionView)).toBe(false)
    }
  })

  test('both « … » menus launch the ready screen with it, unconditionally', () => {
    const panel = src('../src/sections/shell/ProjectPanel.tsx')
    expect(panel).toMatch(/checkInstall: \(\) => launchReady\(\)/)
    expect(panel).toMatch(/data-element=\{item\.key === 'checkInstall' \? CHECK_INSTALL_ELEMENT/)
    const ribbon = src('../src/sections/shell/IconRibbon.tsx')
    expect(ribbon).toMatch(/onCheckInstall=\{\(\) => \{ void activateUniverse\(u\.name\); launchReady\(u\.name\) \}\}/)
    const item = ribbon.match(/^.*<Menu\.Item[^\n]*data-element=\{CHECK_INSTALL_ELEMENT\}[^\n]*$/m)?.[0] ?? ''
    expect(item).toMatch(/onClick=\{onCheckInstall\}/)
    expect(item).not.toMatch(/hidden\(/)
    // The launcher opens the single ready tab: the full screen, every tab.
    expect(src('../src/sections/ready/ReadyBadge.tsx')).toMatch(/openTab\(name, \{ id: READY_TAB_ID, kind: 'ready'/)
  })

  test('its label is in French and English', () => {
    expect(JSON.parse(src('../src/locales/fr/shell.json')).project.checkInstall).toBe("Vérifier l'installation")
    expect(JSON.parse(src('../src/locales/en/shell.json')).project.checkInstall).toBe('Check installation')
  })
})

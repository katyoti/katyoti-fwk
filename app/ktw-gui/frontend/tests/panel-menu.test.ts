// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'fs'
import { panelMenuItems } from '../src/sections/shell/panelMenu'

// The left panel's menu: every item does something, or says it cannot.
describe('the panel menu', () => {
  test('with a project attached, both items act', () => {
    expect(panelMenuItems(['D:/Projets/ktw-agents'])).toEqual([
      { key: 'checkInstall', disabled: false },
      { key: 'openFolder', disabled: false },
      { key: 'remove', disabled: false },
    ])
  })
  test('a universe that is its own project has nothing to remove', () => {
    expect(panelMenuItems([]).find((i) => i.key === 'remove')?.disabled).toBe(true)
  })
  test('no menu item of the panel is left without an action', () => {
    const src = readFileSync(new URL('../src/sections/shell/ProjectPanel.tsx', import.meta.url), 'utf8')
    const items = src.match(/<Menu\.Item\b[\s\S]*?<\/Menu\.Item>/g) ?? []
    expect(items.length).toBeGreaterThan(0)
    // An item acts, or is disabled and says why — never a dead one.
    // (a bare `disabled` attribute marks an always-disabled line; one that is
    // disabled only sometimes must still have its action)
    for (const item of items) expect(/onClick=|\bdisabled(?=[\s>])/.test(item)).toBe(true)
  })
})

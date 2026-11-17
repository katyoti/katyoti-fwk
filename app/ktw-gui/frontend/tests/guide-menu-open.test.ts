// SPDX-License-Identifier: Apache-2.0
// Picking a display level in the avatar menu keeps the menu
// open - « Utiliser KaTyoti » goes on to « Aide et retours » in the same menu.

import { expect, test } from 'bun:test'

test('the level items of the avatar menu do not close it', async () => {
  const src = await Bun.file(new URL('../src/sections/shell/IconRibbon.tsx', import.meta.url)).text()
  const item = src.slice(src.indexOf('DISPLAY_LEVELS.map'), src.indexOf('data-guide="profile-menu.level"'))
  expect(item).toContain('closeMenuOnClick={false}')
})

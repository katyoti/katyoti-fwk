// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'

import { guiHostServices } from '../src/components/compositions/hostServices'
import { switchOn } from '../src/hooks/useDisplaySwitch'

// One source selector's source-model screens, once behind their
// display switch, show to every user since the increment's close:
// the GUI hands the display library its host's
// `sourceModel` flag on, reading no switch.
describe("the source model", () => {
  test('answers only to its own name', () => {
    expect(switchOn({}, 'in045')).toBe(false)
    expect(switchOn({ in046: true }, 'in045')).toBe(false)
    expect(switchOn({ in045: true }, 'in045')).toBe(true)
  })
  test('reaches the display library as the host flag, off by default', () => {
    expect(guiHostServices('u', 'standard', 'fr', 'jdoe').sourceModel).toBeUndefined()
    expect(guiHostServices('u', 'standard', 'fr', 'jdoe', false).sourceModel).toBeUndefined()
    expect(guiHostServices('u', 'standard', 'fr', 'jdoe', true).sourceModel).toBe(true)
  })
  test('the GUI provides it on, reading no switch', () => {
    const src = readFileSync(join(import.meta.dir, '../src/components/compositions/hostServices.tsx'), 'utf8')
    expect(src).not.toContain("useDisplaySwitch('in045')")
    expect(src).toContain('guiHostServices(universeName, level, language, personId, true)')
  })
})

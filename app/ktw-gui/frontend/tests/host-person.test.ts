// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'

import { guiHostServices } from '../src/components/compositions/hostServices'

// The GUI's host services name the person at the screen, as an
// actor - what the display library's picker sends as who applies.
describe('the person host service', () => {
  test('the person at the screen, or none', () => {
    expect(guiHostServices('archifactory', 'standard', 'fr', 'jdoe').person).toEqual({ id: 'jdoe', kind: 'person' })
    expect(guiHostServices('archifactory', 'standard', 'fr').person).toBeUndefined()
    expect(guiHostServices('archifactory', 'standard', 'fr', '').person).toBeUndefined()
  })
})

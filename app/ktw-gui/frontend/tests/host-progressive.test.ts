// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'

import { guiHostServices } from '../src/components/compositions/hostServices'

// Progressive pages show for everyone - their display switch is
// retired. The GUI's host always hands the display library its
// `progressive` flag.
describe('Progressive pages', () => {
  test('reach the display library as the host flag, always on', () => {
    expect(guiHostServices('archifactory', 'standard', 'fr', 'jdoe').progressive).toBe(true)
    expect(guiHostServices('archifactory').progressive).toBe(true)
  })
})

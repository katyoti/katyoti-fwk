// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'fs'

// The Objects panel's page is its source's (object-pane search);
// the GUI gives every panel source the person's display level beside their
// language, so the page offers no Advanced search at Essentiel.
describe('a panel source', () => {
  test('is called with the person\'s display level and re-read when it changes', () => {
    const panel = readFileSync(new URL('../src/sections/workspace/RenderDefView.tsx', import.meta.url), 'utf8')
    expect(panel).toContain('const level = useDisplayLevel()')
    expect(panel).toContain('...(args ?? { id: objectId }), language, level })')
  })
})

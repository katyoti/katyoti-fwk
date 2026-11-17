// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'

import { guiHostServices } from '../src/components/compositions/hostServices'
import { specCacheOf, templateKey } from '../src/lib/specCache'

// Display panels are the GUI's one display (their increment approved, its
// switch retired): the host always hands the display library its
// `displayPanels` flag.
describe('display panels', () => {
  test('reach the display library as the host flag, for everyone', () => {
    expect(guiHostServices('u', 'standard', 'fr', 'jdoe').displayPanels).toBe(true)
    expect(guiHostServices('u', 'standard', 'fr', 'jdoe').sourceModel).toBeUndefined()
  })
})

// A panel's tree named by {uri, hash} - a page
// template - reaches the display library's panel node through the host's
// panelTemplate, drawn from the universe's spec cache.
describe("the host's panelTemplate", () => {
  test('answers a template held at its hash from the spec cache', async () => {
    const uri = 'ktw-agents:/page-template/review'
    const tree = { kind: 'vbox', id: 'held' }
    specCacheOf('u-tpl').set(templateKey(uri), { hash: 'h1', part: tree })
    const host = guiHostServices('u-tpl', 'standard', 'fr', 'jdoe')
    expect(await host.panelTemplate!({ uri, hash: 'h1' })).toBe(tree)
  })
})

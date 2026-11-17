// SPDX-License-Identifier: Apache-2.0
// A `panel` action (the display library's page DSL) opens a
// display panel by its id in a tab of its own: a conversation of « À faire »
// opens its chat. It is handed to the host, which opens the panel's tab.

import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'

import { routePageAction } from '../../../ktw-display-lib/src/ui-builders/page/page-context'
import { ACTION_VERBS, actionVerb, resolveAction } from '../../../ktw-display-lib/src/ui-builders/page/ui-page.interactive'

describe('a panel action', () => {
  test('is a verb of its own', () => {
    expect(ACTION_VERBS).toContain('panel')
    expect(actionVerb({ panel: 'dp-1' })).toBe('panel')
    expect(actionVerb({ panel: 'dp-1', uri: 'a:b/c' })).toBeUndefined()
  })
  test('fills its id from the element it is on', () => {
    expect(resolveAction({ panel: '${item.panel}' }, { panel: 'dp-42' }).panel).toBe('dp-42')
  })
  test('reaches the host, which opens the panel in a tab', () => {
    const hosted: unknown[] = []
    routePageAction({ action: { panel: 'dp-1' } }, { show: () => {}, select: () => {}, host: (e: unknown) => hosted.push(e), selection: {} })
    expect(hosted).toHaveLength(1)
    const host = readFileSync(join(import.meta.dir, '../src/components/compositions/PageHost.tsx'), 'utf8')
    expect(host).toContain("case 'panel':")
    expect(host).toContain('openPanelTab(universeName, action.panel)')
  })
})

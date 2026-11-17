// SPDX-License-Identifier: Apache-2.0
// A `screen` action (the display library's page DSL)
// opens one of the host's own screens by its name: `todo` is « À faire », what
// the rail's To do opens - the planification deck's « Ouvrir À faire ». It is
// handed to the host, which opens the same tab as the rail.

import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'

import { routePageAction } from '../../../ktw-display-lib/src/ui-builders/page/page-context'
import { ACTION_VERBS, actionVerb } from '../../../ktw-display-lib/src/ui-builders/page/ui-page.interactive'
import { UIPageNodeJSONSchema } from '../../../ktw-display-lib/src/ui-builders/page/ui-page.json-schema'

describe('a screen action', () => {
  test('is a verb of its own', () => {
    expect(ACTION_VERBS).toContain('screen')
    expect(actionVerb({ screen: 'todo' })).toBe('screen')
    expect(actionVerb({ screen: 'todo', panel: 'dp-1' })).toBeUndefined()
  })
  test('the page schema names its screens', () => {
    const action = (UIPageNodeJSONSchema as { definitions: Record<string, { properties: Record<string, { enum?: string[] }> }> }).definitions.Action
    expect(action.properties.screen.enum).toEqual(['todo'])
  })
  test('reaches the host, which opens « À faire » as the rail does', () => {
    const hosted: unknown[] = []
    routePageAction({ action: { screen: 'todo' } }, { show: () => {}, select: () => {}, host: (e: unknown) => hosted.push(e), selection: {} })
    expect(hosted).toHaveLength(1)
    const host = readFileSync(join(import.meta.dir, '../src/components/compositions/PageHost.tsx'), 'utf8')
    expect(host).toContain("case 'screen':")
    expect(host).toContain("if (action.screen === 'todo') openInboxTab(universeName)")
    const launcher = readFileSync(join(import.meta.dir, '../src/hooks/useInboxLauncher.ts'), 'utf8')
    expect(launcher).toContain('openInboxTab(displayedUniverseName)')
  })
})

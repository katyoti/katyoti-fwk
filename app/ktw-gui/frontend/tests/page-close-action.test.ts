// SPDX-License-Identifier: Apache-2.0
// A `close` action (the display library's page DSL) closes
// what the page is drawn in: handed to the host, which tells its dialog -
// the create dialog's « Annuler ». The create dialog also closes when the
// page's action opened what it created, the workspace tree re-read.

import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'

import { routePageAction } from '../../../ktw-display-lib/src/ui-builders/page/page-context'
import { ACTION_VERBS, actionVerb } from '../../../ktw-display-lib/src/ui-builders/page/ui-page.interactive'

describe('a close action', () => {
  test('is a verb of its own', () => {
    expect(ACTION_VERBS).toContain('close')
    expect(actionVerb({ close: true })).toBe('close')
    expect(actionVerb({ close: false })).toBeUndefined()
    expect(actionVerb({ close: true, screen: 'todo' })).toBeUndefined()
  })
  test('reaches the host, which tells the dialog it is drawn in', () => {
    const hosted: unknown[] = []
    routePageAction({ action: { close: true } }, { show: () => {}, select: () => {}, host: (e: unknown) => hosted.push(e), selection: {} })
    expect(hosted).toHaveLength(1)
    const host = readFileSync(join(import.meta.dir, '../src/components/compositions/PageHost.tsx'), 'utf8')
    expect(host).toContain("case 'close':")
    expect(host).toContain('onDone?.(false)')
    expect(host).toContain('if (res.ok && res.open) { navigate(res.open); onDone?.(true) }')
  })
  test('the create dialog closes on it and on the created object opening', () => {
    const modal = readFileSync(join(import.meta.dir, '../src/sections/workspace/ObjectCreateModal.tsx'), 'utf8')
    expect(modal).toContain('onDone={(opened) => { if (opened) bumpWorkspaceTree(); onClose() }}')
  })
})

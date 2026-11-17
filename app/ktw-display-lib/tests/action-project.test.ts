// SPDX-License-Identifier: Apache-2.0
// An action's result may name a project the universe is to serve:
// `project`, a folder the host has the
// hub switch the universe to. Only a success switches.

import { describe, expect, test } from 'bun:test'

import { parseActionResult } from '../src/ui-builders/page/ui-page.interactive.ts'

describe('a result naming a project to serve', () => {
  test('keeps its project, from the JSON text or the value', () => {
    expect(parseActionResult({ ok: true, message: 'ERP Arbor', project: 'C:/Katyoti/erp-arbor' })?.project).toBe('C:/Katyoti/erp-arbor')
    expect(parseActionResult(JSON.stringify({ project: 'C:/Katyoti/cosmos' }))?.project).toBe('C:/Katyoti/cosmos')
  })
  test('a refusal or an empty project switches nothing', () => {
    expect(parseActionResult({ ok: false, message: 'no', project: 'C:/Katyoti/cosmos' })?.project).toBeUndefined()
    expect(parseActionResult({ ok: true, project: '  ' })?.project).toBeUndefined()
  })
})

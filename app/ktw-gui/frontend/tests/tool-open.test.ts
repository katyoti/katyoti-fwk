// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'fs'
import { openOf } from '../src/hooks/toolOpen'

// « Nouvelle étude de l'existant » ends on the new study's
// pane - a tool answering {ok, open} opens what it made, as a created object does.
describe('a tool result naming what to open', () => {
  test('opens the object, or the object on a panel', () => {
    expect(openOf('{"ok":true,"message":"Étude créée","open":"etude-erp-acme"}')).toEqual({ id: 'etude-erp-acme' })
    expect(openOf('{"ok":true,"open":"etude-erp-acme#panel:etude"}')).toEqual({ id: 'etude-erp-acme', panel: 'panel:etude' })
  })
  test('opens nothing when it failed, names nothing, or is not JSON', () => {
    expect(openOf('{"ok":false,"open":"x"}')).toBeNull()
    expect(openOf('{"ok":true}')).toBeNull()
    expect(openOf('plain text')).toBeNull()
    expect(openOf(undefined)).toBeNull()
  })
  test('the chat events open it', () => {
    const src = readFileSync(new URL('../src/hooks/useAgentEvents.ts', import.meta.url), 'utf8')
    expect(src).toContain('const target = openOf(update.output)')
    expect(src).toContain('openRunResult(uni, target)')
  })
})

// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { pickViewKind } from '../../src/runtime/dispatch'

// Extensions sit on the schema's root type.
const drawioSchema = { root: 'D', types: { D: { type: 'object', extensions: [{ type: 'ui-drawio' }] } } }

describe('pickViewKind', () => {
  test('a schema carrying the ui-drawio extension is a drawio view', () => {
    expect(pickViewKind(drawioSchema)).toBe('drawio')
  })
  test('the URL hint still wins', () => {
    expect(pickViewKind(drawioSchema, 'form')).toBe('form')
  })
})

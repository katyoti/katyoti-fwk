// SPDX-License-Identifier: Apache-2.0
// Smoke test: the package compiles and the schema lib types are coherent.
// The schema types live in ktw-display-lib (src/schema/).
//
// This is intentionally minimal — verifies the public surface imports cleanly
// and a minimal Schema literal type-checks. Real test coverage lands as the
// codegen targets and widget routes are built out (per `dsl-codegen-and-widget-engine`).

import { describe, expect, it } from 'bun:test'
import type { Schema, SchemaType, SchemaTypeRef } from '../../ktw-display-lib/src/schema/schema-types'

describe('schema lib smoke', () => {
  it('exposes Schema with name + root + types (DSL-aligned 2026-05-01)', () => {
    const s: Schema = {
      name: 'smoke',
      root: 'User',
      types: {
        User: { type: 'object' } as SchemaType,
      },
    }
    expect(s.name).toBe('smoke')
    expect(s.root).toBe('User')
    expect(typeof s.types['User']).toBe('object')
  })

  it('SchemaTypeRef accepts both string ref and inline type', () => {
    const refByName: SchemaTypeRef = 'User'
    const refInline: SchemaTypeRef = { type: 'string' } as SchemaType
    expect(typeof refByName).toBe('string')
    expect(typeof refInline).toBe('object')
  })
})

// SPDX-License-Identifier: Apache-2.0
// UI factory tests — getInfo() resolves a CrudFieldInfo from a schema,
// SchemaQuery handles the open content envelope (string ref + object form
// + default fallback).

import { describe, expect, it } from 'bun:test'
import { SchemaQuery } from '../../../ktw-display-lib/src/schema/schema-query.ts'
import { buildInfo, getInfo } from '../../../ktw-display-lib/src/ui-builders/fields/info.ts'
import type { Schema } from '../../../ktw-display-lib/src/schema/schema-types.ts'

const SCHEMA: Schema = {
  name: 'test',
  root: 'Demo',
  types: {
    Demo: {
      type: 'object',
      fields: {
        plain_string: { type: 'string', label: 'Plain' },
        with_email_ref: { type: 'string', label: 'Email', content: 'email' },
        with_email_object: {
          type: 'string',
          label: 'Email obj',
          content: { type: 'email' },
        },
        with_slider: {
          type: 'number',
          content: { type: 'slider', min: 0, max: 100 },
        },
        plain_bool: { type: 'boolean' },
      },
    },
  },
}

describe('SchemaQuery.getTypeContent — open envelope', () => {
  it('falls back to KIND_DEFAULT_CONTENT when content is omitted', () => {
    const ct = SchemaQuery.getTypeContent(SCHEMA, { type: 'string' })
    expect(ct).toEqual({ type: 'text' })
    const cn = SchemaQuery.getTypeContent(SCHEMA, { type: 'number' })
    expect(cn).toEqual({ type: 'number' })
    const cb = SchemaQuery.getTypeContent(SCHEMA, { type: 'boolean' })
    expect(cb).toEqual({ type: 'boolean' })
  })

  it('promotes a string ref to { type: <ref> }', () => {
    const ct = SchemaQuery.getTypeContent(SCHEMA, { type: 'string', content: 'email' })
    expect(ct).toEqual({ type: 'email' })
  })

  it('returns object content as-is', () => {
    const ct = SchemaQuery.getTypeContent(SCHEMA, {
      type: 'number',
      content: { type: 'slider', min: 0, max: 100 },
    })
    expect(ct).toEqual({ type: 'slider', min: 0, max: 100 })
  })

  it('returns undefined for non-content-bearing kinds', () => {
    expect(SchemaQuery.getTypeContent(SCHEMA, { type: 'object' })).toBeUndefined()
  })
})

describe('buildInfo / getInfo', () => {
  it('builds CrudFieldInfo for a plain string field, defaulting content to text', () => {
    const info = getInfo(SCHEMA, 'plain_string', { id: 'plain_string' })
    expect(info.name).toBe('plain_string')
    expect(info.type.type).toBe('string')
    expect(info.contentType.type).toBe('text')
    expect(info.label).toBe('Plain')
    expect(info.isArray).toBe(false)
    expect(info.schema).toBe(SCHEMA)
  })

  it('resolves string-ref content', () => {
    const info = getInfo(SCHEMA, 'with_email_ref', { id: 'with_email_ref' })
    expect(info.contentType.type).toBe('email')
  })

  it('resolves object content + carries params', () => {
    const info = getInfo(SCHEMA, 'with_email_object', { id: 'with_email_object' })
    expect(info.contentType.type).toBe('email')

    const slider = getInfo(SCHEMA, 'with_slider', { id: 'with_slider' })
    expect(slider.contentType.type).toBe('slider')
    expect((slider.contentType as { min?: number; max?: number }).min).toBe(0)
    expect((slider.contentType as { min?: number; max?: number }).max).toBe(100)
  })

  it('renders a field the schema does not declare as a plain string, never throws', () => {
    // A column naming a field the schema lacks (thin or data-derived
    // schemas) must not white-screen the whole table.
    const info = getInfo(SCHEMA, 'nope', { id: 'nope' })
    expect(info.contentType.type).toBe(getInfo(SCHEMA, 'plain_string', { id: 'plain_string' }).contentType.type)
  })

  it('label cascades from def → field → name', () => {
    const explicit = getInfo(SCHEMA, 'plain_string', { id: 'plain_string', label: 'Custom label' })
    expect(explicit.label).toBe('Custom label')
    const fromField = getInfo(SCHEMA, 'plain_string', { id: 'plain_string' })
    expect(fromField.label).toBe('Plain') // from schema
  })
})

// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { SchemaQuery } from '../../../ktw-display-lib/src/schema/schema-query.ts'

// A form opens with the values its schema states: a required field gets its
// default (or the type's zero), an optional field only when it declares one.
describe('SchemaQuery.getDefaultValue', () => {
  const schema = {
    name: 'form',
    root: 'In',
    types: {
      In: {
        type: 'object',
        fields: {
          subject: { type: 'string' },
          theme: { type: 'string', default: 'katyoti' },
          language: { type: 'string', optional: true, default: 'auto' },
          save: { type: 'boolean', optional: true, default: true },
          slides: { type: 'number', optional: true, default: 8 },
          tone: { type: 'string', optional: true },
          sources: { type: 'array', items: { type: 'string' }, optional: true, default: ['leanix', 'confluence'] },
          tags: { type: 'array', items: { type: 'string' }, optional: true },
        },
      },
    },
  } as any

  test('required fields and optional fields with a default', () => {
    expect(SchemaQuery.getDefaultValue(schema)).toEqual({
      subject: '',
      theme: 'katyoti',
      language: 'auto',
      save: true,
      slides: 8,
      sources: ['leanix', 'confluence'],
    })
  })
})

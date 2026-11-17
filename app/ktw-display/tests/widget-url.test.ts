// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from 'bun:test'
import {
  buildWidgetURL,
  InvalidWidgetURLInputError,
  type GetWidgetURLInput,
} from '../src/widget-url.ts'

describe('buildWidgetURL', () => {
  it('composes the simplest URL — kind only, no token', () => {
    const out = buildWidgetURL({ kind: 'page' }, 51234, '')
    expect(out.URL).toBe('http://127.0.0.1:51234/widget/page')
    expect(out.Token).toBe('')
    expect(out.ExpiresAt).toBe(0)
  })

  it('embeds the per-session token as a query param', () => {
    const out = buildWidgetURL({ kind: 'page' }, 51234, 'tok-abc')
    expect(out.URL).toBe('http://127.0.0.1:51234/widget/page?token=tok-abc')
    expect(out.Token).toBe('tok-abc')
  })

  it('serialises optional schemaName / value / mode params', () => {
    const out = buildWidgetURL(
      { kind: 'form', schemaName: 'Book', value: 'book-42', mode: 'edit' },
      9990,
      'tok',
    )
    // URLSearchParams sorts deterministically by insertion order in our impl;
    // assert the param presence rather than ordering.
    const url = new URL(out.URL)
    expect(url.pathname).toBe('/widget/form')
    expect(url.searchParams.get('schemaName')).toBe('Book')
    expect(url.searchParams.get('value')).toBe('book-42')
    expect(url.searchParams.get('mode')).toBe('edit')
    expect(url.searchParams.get('token')).toBe('tok')
  })

  it('omits empty optional params', () => {
    const out = buildWidgetURL({ kind: 'page', schemaName: '', mode: '' }, 9990, '')
    expect(out.URL).toBe('http://127.0.0.1:9990/widget/page')
  })

  it('URL-encodes special characters in optional params', () => {
    const out = buildWidgetURL({ kind: 'form', schemaName: 'a b&c=d' }, 9990, '')
    const url = new URL(out.URL)
    expect(url.searchParams.get('schemaName')).toBe('a b&c=d')
  })

  it('respects a custom hostname (test fixture support)', () => {
    const out = buildWidgetURL({ kind: 'page' }, 9990, '', 'localhost')
    expect(out.URL).toBe('http://localhost:9990/widget/page')
  })

  it('throws when kind is missing', () => {
    expect(() => buildWidgetURL({} as GetWidgetURLInput, 9990, '')).toThrow(
      InvalidWidgetURLInputError,
    )
  })

  it('throws when kind is empty string', () => {
    expect(() => buildWidgetURL({ kind: '' }, 9990, '')).toThrow(
      InvalidWidgetURLInputError,
    )
  })

  it('throws when kind is whitespace-only', () => {
    expect(() => buildWidgetURL({ kind: '   ' }, 9990, '')).toThrow(
      InvalidWidgetURLInputError,
    )
  })

  it('throws when kind contains a slash', () => {
    expect(() => buildWidgetURL({ kind: 'foo/bar' }, 9990, '')).toThrow(
      InvalidWidgetURLInputError,
    )
  })

  it('throws when kind contains a question mark', () => {
    expect(() => buildWidgetURL({ kind: 'foo?bar' }, 9990, '')).toThrow(
      InvalidWidgetURLInputError,
    )
  })

  it('exposes status + code on the thrown error', () => {
    try {
      buildWidgetURL({ kind: '' }, 9990, '')
    } catch (e) {
      expect(e).toBeInstanceOf(InvalidWidgetURLInputError)
      const err = e as InvalidWidgetURLInputError
      expect(err.status).toBe(400)
      expect(err.code).toBe('kind_required')
    }
  })

  it('accepts every production widget kind', () => {
    for (const kind of ['form', 'view', 'table', 'wizard', 'page', 'drawio']) {
      expect(() => buildWidgetURL({ kind }, 9990, '')).not.toThrow()
    }
  })
})

// SPDX-License-Identifier: Apache-2.0
// The deck theme store (src/theme-store.ts): the host posts a theme once, a
// page reads its declaration and its files by version.

import { describe, expect, test } from 'bun:test'
import { assetType, themeGetRoute, themeStoreRoute, validAssetPath } from '../../../src/theme-store.ts'

const post = (body: unknown) =>
  themeStoreRoute(new Request('http://x.test/widget/deck/theme', { method: 'POST', body: JSON.stringify(body) }))

const b64 = (s: string) => Buffer.from(s).toString('base64')

describe('theme store', () => {
  test('a posted theme is served: its declaration and its files, by version', async () => {
    const r = await post({ name: 'brand', version: 'v1', body: { name: 'brand' }, assets: { 'bg/sand.jpg': b64('JPG'), 'logo.png': b64('PNG') } })
    expect(r.status).toBe(200)
    const decl = themeGetRoute('brand/theme.json')
    expect(await decl.json()).toEqual({ name: 'brand', version: 'v1', body: { name: 'brand' } })
    const file = themeGetRoute('brand/v1/assets/bg/sand.jpg')
    expect(file.status).toBe(200)
    expect(file.headers.get('content-type')).toBe('image/jpeg')
    expect(file.headers.get('cache-control')).toContain('immutable')
    expect(await file.text()).toBe('JPG')
  })

  test('a new version replaces the old one; the old files are gone', async () => {
    await post({ name: 'swap', version: 'a', body: { name: 'swap' }, assets: { 'logo.png': b64('A') } })
    await post({ name: 'swap', version: 'b', body: { name: 'swap' }, assets: { 'logo.png': b64('B') } })
    expect((await themeGetRoute('swap/theme.json').json()).version).toBe('b')
    expect(themeGetRoute('swap/a/assets/logo.png').status).toBe(404)
    expect(await themeGetRoute('swap/b/assets/logo.png').text()).toBe('B')
  })

  test('an unpublished theme is a 404, which a page reads as "use the defaults"', () => {
    expect(themeGetRoute('nobody/theme.json').status).toBe(404)
  })

  test('refuses what is not a theme', async () => {
    expect((await post({ name: 'Bad Name', version: 'v', body: {} })).status).toBe(400)
    expect((await post({ name: 'ok', version: 'v' })).status).toBe(400)
    expect((await post({ name: 'ok', version: 'v', body: {}, assets: { '../escape.png': b64('x') } })).status).toBe(400)
    expect((await post({ name: 'ok', version: 'v', body: {}, assets: { 'a.png': 42 } })).status).toBe(400)
  })

  test('asset paths stay inside the theme', () => {
    for (const ok of ['logo.png', 'icons/camion.png', 'bg/a.b.jpg']) expect(validAssetPath(ok)).toBe(true)
    for (const bad of ['', '/abs.png', '../x.png', 'a/../../x', 'a//b', 'a\\b.png', './x']) expect(validAssetPath(bad)).toBe(false)
  })

  test('content types come from the extension', () => {
    expect(assetType('x.PNG')).toBe('image/png')
    expect(assetType('f/skeena.woff2')).toBe('font/woff2')
    expect(assetType('x.bin')).toBe('application/octet-stream')
  })
})

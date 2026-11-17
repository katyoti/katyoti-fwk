// SPDX-License-Identifier: Apache-2.0
// Files beside a deck: the path rule (ktw-display-lib deck/assets.ts, the same
// as deckapi.LocalAsset) and the display server's store (src/deck-assets.ts).

import { describe, expect, it } from 'bun:test'
import { localAssetPath } from '../../../../ktw-display-lib/src/ui-builders/deck/assets.ts'
import { deckAssetsGetRoute, deckAssetsStoreRoute } from '../../../src/deck-assets.ts'

describe('localAssetPath', () => {
  it('keeps a relative path, cleaned', () => {
    expect(localAssetPath('assets/topology.svg')).toBe('assets/topology.svg')
    expect(localAssetPath('./assets/a/../b.png')).toBe('assets/b.png')
    expect(localAssetPath('bg/x.jpg?v=2')).toBe('bg/x.jpg')
  })
  it('refuses URLs, absolute paths and a path leaving the folder', () => {
    for (const t of ['https://x.test/y.png', 'data:image/png;base64,AA', 'icon:truck', '/etc/x.png', 'C:/x.png', '../out.png', '#a', '']) {
      expect(localAssetPath(t)).toBeNull()
    }
  })
})

describe('localAssetPath — module paths', () => {
  it('names a file of the module from its root, never above it', () => {
    expect(localAssetPath('module:/dossiers/d1/travail/flux.drawio')).toBe('_module/dossiers/d1/travail/flux.drawio')
    expect(localAssetPath('module:dossiers/d1/./a.png?v=2')).toBe('_module/dossiers/d1/a.png')
    expect(localAssetPath('module:/dossiers/../../etc/passwd')).toBeNull()
    expect(localAssetPath('module:/')).toBeNull()
  })
})

describe('deck assets store', () => {
  it('serves the version it holds, by path, and only that', async () => {
    const post = (b: unknown) => deckAssetsStoreRoute(new Request('http://x.test/widget/deck/assets', { method: 'POST', body: JSON.stringify(b) }))
    expect((await post({ scope: 's1', version: 'v1', files: { 'assets/a.png': Buffer.from('png').toString('base64') } })).status).toBe(200)
    const ok = deckAssetsGetRoute('s1/v1/assets/a.png')
    expect(ok.status).toBe(200)
    expect(ok.headers.get('content-type')).toBe('image/png')
    expect(await ok.text()).toBe('png')
    expect(deckAssetsGetRoute('s1/v0/assets/a.png').status).toBe(404)
    expect(deckAssetsGetRoute('s1/v1/assets/b.png').status).toBe(404)
    expect((await post({ scope: 's1', version: 'v2', files: { '../x.png': 'AA' } })).status).toBe(400)
  })
})

// SPDX-License-Identifier: Apache-2.0
// The files beside a deck (modules/slidedeck/): the images and backgrounds a
// deck names by a relative path. The host reads them from the deck's folder
// and posts them here once per version; the deck page reads them by scope.
//
// Routes (under /widget/ so the query-string token authenticates them):
//
//   POST /widget/deck/assets                   {scope, version, files: {<path>: <base64>}}
//   GET  /widget/deck/assets/<scope>/<version>/<path>
//
// A scope names one deck's folder (the host derives it from the deck's object
// or file). In memory, per process, the latest version of each scope.

import { assetType, validAssetPath } from './theme-store.ts'

export interface DeckAssets {
  scope: string
  version: string
  files: Map<string, Uint8Array<ArrayBuffer>>
}

const stores = new Map<string, DeckAssets>()
const MAX_SCOPES = 64
const MAX_BYTES = 64 << 20
const TOKEN = /^[A-Za-z0-9_-]{1,64}$/

export function getDeckAssets(scope: string): DeckAssets | undefined {
  return stores.get(scope)
}

/** POST /widget/deck/assets */
export async function deckAssetsStoreRoute(req: Request): Promise<Response> {
  let body: { scope?: unknown; version?: unknown; files?: unknown }
  try {
    body = await req.json()
  } catch {
    return Response.json({ error: 'body must be JSON {scope, version, files}' }, { status: 400 })
  }
  const { scope, version, files } = body
  if (typeof scope !== 'string' || !TOKEN.test(scope)) return Response.json({ error: 'scope must match [A-Za-z0-9_-]{1,64}' }, { status: 400 })
  if (typeof version !== 'string' || !TOKEN.test(version)) return Response.json({ error: 'version must match [A-Za-z0-9_-]{1,64}' }, { status: 400 })
  if (!files || typeof files !== 'object') return Response.json({ error: 'files must be {<path>: <base64>}' }, { status: 400 })
  const map = new Map<string, Uint8Array<ArrayBuffer>>()
  let total = 0
  for (const [p, b64] of Object.entries(files as Record<string, unknown>)) {
    if (!validAssetPath(p) || typeof b64 !== 'string') return Response.json({ error: `bad file ${p}` }, { status: 400 })
    const bytes = new Uint8Array(Buffer.from(b64, 'base64'))
    total += bytes.length
    if (total > MAX_BYTES) return Response.json({ error: 'the files are above 64 MB' }, { status: 413 })
    map.set(p, bytes)
  }
  stores.delete(scope)
  stores.set(scope, { scope, version, files: map })
  while (stores.size > MAX_SCOPES) stores.delete(stores.keys().next().value as string)
  return Response.json({ scope, version, files: map.size })
}

/** GET /widget/deck/assets/<scope>/<version>/<path> */
export function deckAssetsGetRoute(rest: string): Response {
  const [scope, version, ...parts] = rest.split('/')
  const path = parts.map(decodeURIComponent).join('/')
  const store = stores.get(scope)
  if (!store || store.version !== version) return Response.json({ error: 'not published' }, { status: 404 })
  const bytes = store.files.get(path)
  if (!bytes) return Response.json({ error: `no file ${path}` }, { status: 404 })
  return new Response(bytes, { headers: { 'content-type': assetType(path), 'cache-control': 'private, max-age=31536000, immutable' } })
}

/** The URL of a file beside a deck, as the deck page asks for it. */
export const deckAssetURL = (scope: string, version: string, path: string, token: string) =>
  `/widget/deck/assets/${encodeURIComponent(scope)}/${encodeURIComponent(version)}/${path.split('/').map(encodeURIComponent).join('/')}${token ? `?token=${encodeURIComponent(token)}` : ''}`

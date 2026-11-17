// SPDX-License-Identifier: Apache-2.0
// The theme store — deck themes the display server holds so a deck page can
// style itself with one (modules/slidedeck/, OD-008).
//
// The display server fetches nothing: the host resolves a theme (its
// declaration and every file it names) and posts it here once per version.
// A deck page reads its own `theme:` and loads the theme from here — the
// embedded view, the presenting window and the PDF alike.
//
// Routes (under /widget/ so the query-string token authenticates them, as
// the page's CSS and <img> requests carry it):
//
//   POST /widget/deck/theme                                {name, version, body, assets: {path: base64}}
//   GET  /widget/deck/theme/<name>/theme.json              {name, version, body} — 404 until posted
//   GET  /widget/deck/theme/<name>/<version>/assets/<path> the file — immutable for its version
//
// In memory, per process, the latest version of each theme.

export interface StoredTheme {
  name: string
  version: string
  body: unknown
  assets: Map<string, Uint8Array<ArrayBuffer>>
}

const themes = new Map<string, StoredTheme>()

const MAX_THEMES = 32
const MAX_THEME_BYTES = 64 << 20
const NAME = /^[a-z0-9]+(-[a-z0-9]+)*$/
const VERSION = /^[A-Za-z0-9_-]{1,64}$/

export function getTheme(name: string): StoredTheme | undefined {
  return themes.get(name)
}

/** A path inside a theme's folder: relative, no `..`, no backslash. */
export function validAssetPath(p: string): boolean {
  return p.length > 0 && p.length <= 512 && !p.startsWith('/') && !p.includes('\\') &&
    p.split('/').every((seg) => seg !== '' && seg !== '.' && seg !== '..')
}

/** Stores a theme, replacing any other version of the same name. */
export function putTheme(t: StoredTheme): void {
  themes.delete(t.name)
  themes.set(t.name, t)
  while (themes.size > MAX_THEMES) themes.delete(themes.keys().next().value as string)
}

const TYPES: Record<string, string> = {
  png: 'image/png', jpg: 'image/jpeg', jpeg: 'image/jpeg', gif: 'image/gif', webp: 'image/webp',
  svg: 'image/svg+xml', woff2: 'font/woff2', woff: 'font/woff', ttf: 'font/ttf', otf: 'font/otf',
}

export function assetType(path: string): string {
  return TYPES[path.slice(path.lastIndexOf('.') + 1).toLowerCase()] ?? 'application/octet-stream'
}

/** POST /widget/deck/theme */
export async function themeStoreRoute(req: Request): Promise<Response> {
  let body: { name?: unknown; version?: unknown; body?: unknown; assets?: unknown }
  try {
    body = await req.json()
  } catch {
    return json({ error: 'body must be JSON {name, version, body, assets}' }, 400)
  }
  const { name, version } = body
  if (typeof name !== 'string' || !NAME.test(name)) return json({ error: 'name must be kebab-case ASCII' }, 400)
  if (typeof version !== 'string' || !VERSION.test(version)) return json({ error: 'version must match [A-Za-z0-9_-]{1,64}' }, 400)
  if (!body.body || typeof body.body !== 'object') return json({ error: 'body (the theme declaration) is required' }, 400)
  const assets = new Map<string, Uint8Array<ArrayBuffer>>()
  let total = 0
  for (const [path, b64] of Object.entries((body.assets ?? {}) as Record<string, unknown>)) {
    if (!validAssetPath(path)) return json({ error: `asset path ${JSON.stringify(path)} is not a path inside the theme` }, 400)
    if (typeof b64 !== 'string') return json({ error: `asset ${path}: base64 string expected` }, 400)
    const bytes = new Uint8Array(Buffer.from(b64, 'base64'))
    total += bytes.length
    if (total > MAX_THEME_BYTES) return json({ error: `theme ${name} is over ${MAX_THEME_BYTES} bytes` }, 413)
    assets.set(path, bytes)
  }
  putTheme({ name, version, body: body.body, assets })
  return json({ name, version })
}

/** GET /widget/deck/theme/<rest> — `<name>/theme.json` or `<name>/<version>/assets/<path>`. */
export function themeGetRoute(rest: string): Response {
  const parts = rest.split('/')
  const t = themes.get(parts[0])
  if (parts.length === 2 && parts[1] === 'theme.json') {
    if (!t) return json({ error: `theme ${parts[0]} is not published` }, 404)
    return json({ name: t.name, version: t.version, body: t.body })
  }
  if (parts.length >= 4 && parts[2] === 'assets') {
    const path = parts.slice(3).join('/')
    const bytes = t && t.version === parts[1] ? t.assets.get(path) : undefined
    if (!bytes) return json({ error: `no asset ${path} in theme ${parts[0]} version ${parts[1]}` }, 404)
    return new Response(bytes, {
      headers: {
        'content-type': assetType(path),
        'cache-control': 'public, max-age=31536000, immutable',
      },
    })
  }
  return json({ error: 'not found' }, 404)
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json', 'cache-control': 'no-cache' },
  })
}

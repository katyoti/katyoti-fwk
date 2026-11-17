// SPDX-License-Identifier: Apache-2.0
// HTTP handlers for /widget/runtime.{js,css}.
//
// The bundle is built AHEAD of `bun build --compile` (see
// `scripts/build-runtime-bundle.ts`) and embedded into the executable via
// the text-import attribute below. Doing the build at compile time avoids
// the "Bun.build() can't read sources at runtime in a compiled binary"
// failure mode (returned 500 — see the diagnostic that found this bug,
// 2026-05-17).
//
// Both endpoints are unauthenticated (loopback / iframe fetch from same
// origin); the auth gate happens on the parent /widget/<kind> page that
// loads them.

import path from 'node:path'

// Text imports — at `bun build` time, Bun reads the file contents and
// inlines them as a string literal. The output of
// `scripts/build-runtime-bundle.ts` is committed to `_compiled/` solely
// so this import has something to read at compile time; runtime never
// touches the on-disk file.
import runtimeJsBundle from './_compiled/runtime.bundle.js' with { type: 'text' }
import runtimeCssBundle from './_compiled/runtime.bundle.css' with { type: 'text' }

// Embedded vendor assets. Same reason as the runtime bundle —
// path.resolve(...vendor) does not resolve inside a compiled binary, so
// each vendor file we ship must be copied into `_compiled/` by the
// prebuild script and re-imported here as text. Keep this list tight —
// every entry inflates the executable.
import drawioViewerJs from './_compiled/drawio-viewer-static.min.js' with { type: 'text' }
import mermaidJs from './_compiled/mermaid.min.js' with { type: 'text' }

/** Map from on-disk vendor subpath (URL-side, lowercased) → embedded text
 *  bundle. Add an entry per file that the iframe runtime fetches at
 *  runtime (via /widget/vendor/<subpath>). The prebuild script must copy
 *  the source file into `_compiled/` first. */
const EMBEDDED_VENDOR: Record<string, { text: string; contentType: string }> = {
  'drawio/viewer-static.min.js': {
    text: drawioViewerJs,
    contentType: 'text/javascript; charset=utf-8',
  },
  'mermaid/mermaid.min.js': {
    text: mermaidJs,
    contentType: 'text/javascript; charset=utf-8',
  },
}

// Stable hash derived from bundle content — surfaced as `x-ktw-bundle-hash`
// so a hot-reload check could detect upgrades. Pure cosmetic for now.
const bundleHash = simpleHash(runtimeJsBundle + runtimeCssBundle)

export function serveRuntimeJs(): Response {
  return new Response(runtimeJsBundle, {
    headers: {
      'content-type': 'text/javascript; charset=utf-8',
      'cache-control': 'public, max-age=300',
      'x-ktw-bundle-hash': bundleHash,
    },
  })
}

export function serveRuntimeCss(): Response {
  return new Response(runtimeCssBundle, {
    headers: {
      'content-type': 'text/css; charset=utf-8',
      'cache-control': 'public, max-age=300',
      'x-ktw-bundle-hash': bundleHash,
    },
  })
}

function simpleHash(s: string): string {
  let hash = 5381
  for (let i = 0; i < s.length; i++) {
    hash = ((hash << 5) + hash) ^ s.charCodeAt(i)
  }
  return (hash >>> 0).toString(16)
}

// =================================================================================================
// /widget/vendor/<subpath> — vendored third-party assets served as-is.
//
// Used by lazy-loaded view kinds that pull in heavy third-party scripts
// (e.g. drawio's viewer-static.min.js, ~3.7 MB). Keeps these out of the
// main runtime bundle so non-drawio paths pay zero cost. Each vendored
// file ships with a NOTICE in its directory preserving the upstream
// license attribution.
//
// Path traversal guard: the resolved on-disk path must remain inside
// `vendor/` — any `..` in the request returns 400.
// =================================================================================================

const VENDOR_ROOT = path.resolve(import.meta.dir, '..', '..', 'vendor')

const VENDOR_CONTENT_TYPES: Record<string, string> = {
  '.js': 'text/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.svg': 'image/svg+xml',
  '.json': 'application/json',
  '.txt': 'text/plain; charset=utf-8',
  '.map': 'application/json',
  // Added for the drawio editor bundle (gui-drawio-editor / B-Heavy):
  // the full webapp ships HTML, fonts, images, stencils as XML, etc.
  '.html': 'text/html; charset=utf-8',
  '.htm':  'text/html; charset=utf-8',
  '.xml':  'text/xml; charset=utf-8',
  '.png':  'image/png',
  '.jpg':  'image/jpeg',
  '.jpeg': 'image/jpeg',
  '.gif':  'image/gif',
  '.ico':  'image/x-icon',
  '.webp': 'image/webp',
  '.woff':  'font/woff',
  '.woff2': 'font/woff2',
  '.ttf':   'font/ttf',
  '.otf':   'font/otf',
  '.eot':   'application/vnd.ms-fontobject',
  '.wasm':  'application/wasm',
}

// Sibling-of-executable assets dir: `<exe-dir>/ktw-display-assets/`. Used
// for large vendor blobs that we DON'T embed into the binary (the drawio
// editor webapp = 50 MB, hundreds of files including fonts/images that
// don't fit the text-import pattern). The build pipeline copies them in.
// In dev mode `process.execPath` points at bun.exe — its sibling won't
// have ktw-display-assets/, so we silently fall through to the on-disk
// vendor/ lookup.
const EXTERNAL_ASSETS_DIR = (() => {
  try {
    return path.join(path.dirname(process.execPath), 'ktw-display-assets')
  } catch {
    return ''
  }
})()

export async function serveVendor(subpath: string): Promise<Response> {
  // Strip leading slash, reject empty/dot-only paths.
  // The case is kept: on a case-sensitive filesystem (macOS, Linux) the
  // editor's `js/PreConfig.js` is not `js/preconfig.js`. Only the embedded
  // map is keyed in lower case.
  const cleaned = subpath.replace(/^\/+/, '').replace(/\\/g, '/')
  if (!cleaned || cleaned === '.' || cleaned === '..') {
    return new Response('bad request', { status: 400 })
  }
  if (cleaned.includes('..')) {
    // Path-traversal guard before any filesystem touch.
    return new Response('forbidden', { status: 403 })
  }

  // 1) Embedded-asset fast path. Used today for drawio's viewer-static.min.js
  // (~3.7 MB) — small enough to inline into the binary.
  const embedded = EMBEDDED_VENDOR[cleaned.toLowerCase()]
  if (embedded) {
    return new Response(embedded.text, {
      headers: {
        'content-type': embedded.contentType,
        'cache-control': 'public, max-age=86400, immutable',
      },
    })
  }

  // 2) Sibling-of-exe assets dir. Where `make ktw-display` puts the drawio
  // editor webapp + other large vendor blobs. Works for the compiled
  // binary (process.execPath is ktw-display.exe).
  if (EXTERNAL_ASSETS_DIR) {
    const extPath = path.resolve(EXTERNAL_ASSETS_DIR, cleaned)
    if (extPath.startsWith(EXTERNAL_ASSETS_DIR + path.sep) || extPath === EXTERNAL_ASSETS_DIR) {
      const file = Bun.file(extPath)
      if (await file.exists()) {
        const ext = path.extname(extPath).toLowerCase()
        const contentType = VENDOR_CONTENT_TYPES[ext] ?? 'application/octet-stream'
        return new Response(file, {
          headers: {
            'content-type': contentType,
            'cache-control': 'public, max-age=86400, immutable',
          },
        })
      }
    }
  }

  // 3) Dev-mode on-disk fallback — `bun run src/index.ts`. In a compiled
  // binary `import.meta.dir`-derived paths don't resolve, so this falls
  // through with a helpful 503 for editor paths.
  const fsPath = path.resolve(VENDOR_ROOT, cleaned)
  if (!fsPath.startsWith(VENDOR_ROOT + path.sep) && fsPath !== VENDOR_ROOT) {
    return new Response('forbidden', { status: 403 })
  }
  const file = Bun.file(fsPath)
  if (!(await file.exists())) {
    // 503 with an actionable hint when it's an editor path (the common
    // miss when the sibling assets dir wasn't populated by `make`).
    if (cleaned.startsWith('drawio/editor/')) {
      return new Response(
        `drawio editor assets not present. Expected at ${EXTERNAL_ASSETS_DIR}\\${cleaned.replace(/\//g, '\\')} ` +
          `(rebuild with \`make ktw-display\`) OR in the source vendor/ tree (run \`vendor/drawio/setup-editor.sh\`). ` +
          `The read-only drawio viewer is still served.`,
        { status: 503, headers: { 'content-type': 'text/plain; charset=utf-8' } },
      )
    }
    return new Response('not found', { status: 404 })
  }
  const ext = path.extname(fsPath).toLowerCase()
  const contentType = VENDOR_CONTENT_TYPES[ext] ?? 'application/octet-stream'
  return new Response(file, {
    headers: {
      'content-type': contentType,
      'cache-control': 'public, max-age=86400, immutable',
    },
  })
}

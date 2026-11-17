// SPDX-License-Identifier: Apache-2.0
// Bun.build wrapper — compiles src/runtime/index.tsx into a single ESM
// browser bundle (with all Mantine CSS extracted to a separate CSS asset).
//
// Cached in memory after first build; serves all /widget/runtime.{js,css}
// requests until the process restarts. (Future: invalidate when a renderer.tsx
// mtime changes.)

import { resolve } from 'node:path'

export interface RuntimeBundle {
  js: string
  css: string
  /** ISO timestamp of when the bundle was built (for diagnostics). */
  builtAt: string
  /** Total bytes (JS + CSS) for log output. */
  bytes: number
  /** Stable hash for cache busting in URLs (?v=…). */
  hash: string
}

let cached: RuntimeBundle | undefined

/** Build (or return cached) iframe runtime bundle.
 *  Emits log lines on first build with sizes + timing. */
export async function getRuntimeBundle(): Promise<RuntimeBundle> {
  if (cached) return cached
  cached = await buildRuntimeBundle()
  return cached
}

/** Force a rebuild (test-only). */
export function _resetRuntimeBundleCache(): void {
  cached = undefined
}

async function buildRuntimeBundle(): Promise<RuntimeBundle> {
  const entryPath = resolve(import.meta.dir, 'index.tsx')
  const start = performance.now()

  const result = await Bun.build({
    entrypoints: [entryPath],
    target: 'browser',
    format: 'esm',
    minify: false,
    sourcemap: 'none',
    splitting: false,
    define: {
      'process.env.NODE_ENV': '"production"',
    },
  })

  if (!result.success) {
    const messages = result.logs
      .map((log) => `[${log.level}] ${log.message}`)
      .join('\n')
    throw new Error(`runtime bundle failed:\n${messages}`)
  }

  let js = ''
  let css = ''

  for (const out of result.outputs) {
    const text = await out.text()
    if (out.kind === 'asset' && out.path.endsWith('.css')) {
      css += text
    } else if (out.kind === 'entry-point' || out.kind === 'chunk') {
      js += text
    }
  }

  const builtAt = new Date().toISOString()
  const bytes = js.length + css.length
  const hash = simpleHash(js + css)

  console.error(
    JSON.stringify({
      ts: builtAt,
      event: 'ktw-display.runtime.built',
      jsBytes: js.length,
      cssBytes: css.length,
      hash,
      ms: Math.round(performance.now() - start),
    }),
  )

  return { js, css, builtAt, bytes, hash }
}

/** Minimal non-cryptographic hash for cache-busting URLs. djb2-style. */
function simpleHash(s: string): string {
  let hash = 5381
  for (let i = 0; i < s.length; i++) {
    hash = ((hash << 5) + hash) ^ s.charCodeAt(i)
  }
  return (hash >>> 0).toString(16)
}

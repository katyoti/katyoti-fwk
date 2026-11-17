// SPDX-License-Identifier: Apache-2.0
// Build script — produces the iframe React runtime bundle ahead of
// `bun build --compile`. Output goes to `src/runtime/_compiled/` where
// `serve.ts` imports it via `with { type: 'text' }` so it gets embedded
// into the compiled ktw-display.exe.
//
// Why offline: when `serve.ts` calls `Bun.build()` at request time, the
// compiled binary has no source files on disk to bundle — it 500s. So
// we move the work to build-time. The runtime bundle changes only when
// ktw-display itself is rebuilt; that's the right cadence.
//
// Run via `bun run scripts/build-runtime-bundle.ts` (or the Makefile
// target before `bun build --compile`).

import { copyFile, mkdir, writeFile } from 'node:fs/promises'
import { resolve } from 'node:path'
import { existsSync } from 'node:fs'

const PKG_ROOT = resolve(import.meta.dir, '..')
const SRC_ENTRY = resolve(PKG_ROOT, 'src', 'runtime', 'index.tsx')
// The standalone deck presenter: only what presenting a deck needs, minified —
// it is inlined into every exported deck page (src/deck-export.ts).
const STANDALONE_ENTRY = resolve(PKG_ROOT, 'src', 'runtime', 'standalone-deck.tsx')
const OUT_DIR = resolve(PKG_ROOT, 'src', 'runtime', '_compiled')

// Vendored third-party assets that must travel inside the compiled
// ktw-display.exe — `path.resolve(import.meta.dir, ...)` against a vendor/
// path doesn't resolve at runtime in a `bun build --compile` binary. So we
// copy them to `_compiled/` where `serve.ts` re-imports via text-attribute.
// Keep this list TIGHT — every entry inflates the binary.
const VENDOR_COPIES: Array<{ srcRel: string; outName: string }> = [
  { srcRel: 'vendor/drawio/viewer-static.min.js', outName: 'drawio-viewer-static.min.js' },
  // The Apache 2.0 notice travels with the viewer it covers (vendor/ is
  // gitignored; _compiled/ is tracked).
  { srcRel: 'vendor/drawio/NOTICE', outName: 'drawio-viewer-static.NOTICE' },
  // mermaid (MIT), loaded by a deck only when a slide draws a mermaid diagram
  // — kept out of the bundles (it is larger than the rest of the deck).
  { srcRel: 'node_modules/mermaid/dist/mermaid.min.js', outName: 'mermaid.min.js' },
  { srcRel: 'node_modules/mermaid/LICENSE', outName: 'mermaid.LICENSE' },
]

async function main(): Promise<void> {
  const start = performance.now()

  const result = await Bun.build({
    entrypoints: [SRC_ENTRY],
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
    for (const log of result.logs) {
      console.error(`[${log.level}] ${log.message}`)
    }
    process.exit(1)
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

  await mkdir(OUT_DIR, { recursive: true })
  await writeFile(resolve(OUT_DIR, 'runtime.bundle.js'), js)
  await writeFile(resolve(OUT_DIR, 'runtime.bundle.css'), css)

  const standalone = await Bun.build({
    entrypoints: [STANDALONE_ENTRY],
    target: 'browser',
    format: 'esm',
    minify: true,
    sourcemap: 'none',
    splitting: false,
    define: { 'process.env.NODE_ENV': '"production"' },
  })
  if (!standalone.success) {
    for (const log of standalone.logs) console.error(`[${log.level}] ${log.message}`)
    process.exit(1)
  }
  let sjs = ''
  let scss = ''
  for (const out of standalone.outputs) {
    const text = await out.text()
    if (out.kind === 'asset' && out.path.endsWith('.css')) scss += text
    else if (out.kind === 'entry-point' || out.kind === 'chunk') sjs += text
  }
  await writeFile(resolve(OUT_DIR, 'deck-standalone.bundle.js'), sjs)
  await writeFile(resolve(OUT_DIR, 'deck-standalone.bundle.css'), scss)

  // Copy vendor assets into _compiled/ so `serve.ts` can import them with
  // `with { type: 'text' }` — the import is resolved at compile time and
  // the bytes get inlined into the executable.
  const vendorReports: Array<{ name: string; bytes: number }> = []
  for (const v of VENDOR_COPIES) {
    const src = resolve(PKG_ROOT, v.srcRel)
    if (!existsSync(src)) {
      console.error(`[prebuild] WARN: vendor asset missing: ${src} — skipping`)
      continue
    }
    const dst = resolve(OUT_DIR, v.outName)
    await copyFile(src, dst)
    vendorReports.push({ name: v.outName, bytes: Bun.file(dst).size })
  }

  const ms = Math.round(performance.now() - start)
  console.error(
    JSON.stringify({
      event: 'ktw-display.runtime.prebuilt',
      jsBytes: js.length,
      cssBytes: css.length,
      standaloneBytes: sjs.length + scss.length,
      vendor: vendorReports,
      ms,
    }),
  )
}

await main()

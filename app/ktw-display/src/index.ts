// SPDX-License-Identifier: Apache-2.0
// ktw-display — TypeScript sidecar for the data-DSL codegen + widget server.
//
// Spawned by ktw-agents on launch; stopped on shutdown. Same lifecycle pattern
// as Go sidecars (ktw-docs, ...) — only the implementation
// language differs.
//
// See README.md for the integration story.
//
// This file is the entry point: boots an HTTP server with five surfaces
// (/widget, /api/gen, /api/parse, /api/types, /healthz). Concrete handlers
// land in follow-up todos under ms06 (per `dsl-codegen-and-widget-engine`).

import { serve } from 'bun'
import { loadWidget } from './widgets/load.ts'
import { formWidget } from './widgets/form.ts'
import { viewWidget } from './widgets/view.ts'
import { tableWidget } from './widgets/table.ts'
import { treeTableWidget } from './widgets/tree-table.ts'
import { wizardWidget } from './widgets/wizard.ts'
import { pageWidget } from './widgets/page.ts'
import { drawioWidget } from './widgets/drawio.ts'
import { deckWidget } from './widgets/deck.ts'
import { deckStoreRoute, deckStreamRoute } from './deck-store.ts'
import { deckExportHtmlRoute, deckExportPptxRoute } from './deck-export.ts'
import { deckAssetsGetRoute, deckAssetsStoreRoute } from './deck-assets.ts'
import { themeGetRoute, themeStoreRoute } from './theme-store.ts'
import { harnessHTML } from './test/harness.ts'
import { listCases, getCase } from './test/registry.ts'
import { renderCasePage } from './test/case-page.ts'
import { serveRuntimeJs, serveRuntimeCss, serveVendor } from './runtime/serve.ts'
import { sseSchemaChanged } from './sse.ts'
import { buildWidgetURL, InvalidWidgetURLInputError, type GetWidgetURLInput } from './widget-url.ts'
import { LISTEN_HOSTNAME, corsHeaders, isAuthenticated, withCors } from './access.ts'

const PORT_ENV = Bun.env.KTW_DISPLAY_PORT
const PORT = PORT_ENV ? Number(PORT_ENV) : 0 // 0 = auto-pick free port
const TOKEN = Bun.env.KTW_DISPLAY_TOKEN ?? '' // per-session auth from ktw-agents; none = every authenticated surface refused
// Stamped at build time by the Makefile via
// `bun build --define 'process.env.KTW_DISPLAY_VERSION="<version>"'`, mirroring
// the -ldflags stamp the Go sidecars get. The define is substituted into the
// compiled binary, so a runtime env var cannot change it. The literal is the
// dev fallback for `bun run src/index.ts` and for a build with no --define —
// without this, a release would ship every Go sidecar at the release version
// and ktw-display stuck at whatever was hardcoded here.
const VERSION = process.env.KTW_DISPLAY_VERSION ?? '0.1.0'

// --test CLI flag enables the test harness at /test. Activated only via the
// flag — no env var fallback — so the harness can never accidentally leak
// into production through inherited shell state.
const TEST_MODE = Bun.argv.includes('--test')

// CLI verb dispatch — handled BEFORE the server boots. ktw-display is a
// Bun binary so it doesn't inherit cobra's free `version` command; without
// this guard `ktw-display version` is silently ignored and the process
// starts the (never-exiting) HTTP server instead. That hangs any caller
// probing it — notably `ktw-univers scan-inventory`, which runs
// `<binary> version` on every ktw-* it finds. Output matches the Go
// sidecars' format: `<name> <version>` on the first stdout line.
const CLI_ARGS = Bun.argv.slice(1).filter((a) => !a.startsWith('--')) // drop exec path + flags
if (CLI_ARGS.some((a) => a === 'version' || a === '-v') || Bun.argv.includes('--version')) {
  console.log(`ktw-display ${VERSION}`)
  process.exit(0)
}
// `describe` is what the marketplace indexer runs on every binary to read its
// catalog metadata. Without it the index build times out on the never-exiting
// server and drops ktw-display from the marketplace. Shape must match Go's
// appcore.SidecarDescription; the indexer always asks for --out json, so JSON
// is the only output mode.
if (CLI_ARGS.some((a) => a === 'describe')) {
  console.log(
    JSON.stringify(
      {
        name: 'ktw-display',
        displayName: 'ktw-display',
        description: 'ktw-display — data-DSL widget renderer + display server',
        icon: '🖼️',
        version: VERSION,
      },
      null,
      2,
    ),
  )
  process.exit(0)
}
// `sc` / `sidecar` is the Go sidecars' operator surface, run on every member by
// whoever writes a universe's defaults (a scenario case's install: `sc config
// set`; a solution's configure engine: `sidecar config check`). ktw-display has
// no config file and no key: `config set` has nothing to write, `config check`
// reports it healthy, `config get` has nothing to read. Answered here, or the
// caller waits on the never-exiting server.
const SC_AT = CLI_ARGS.findIndex((a) => a === 'sc' || a === 'sidecar')
if (SC_AT >= 0) {
  const [group, verb, name] = CLI_ARGS.slice(SC_AT + 1)
  const outAt = Bun.argv.indexOf('--out')
  const asJSON = outAt >= 0 && Bun.argv[outAt + 1] === 'json'
  if (group === 'config' && verb === 'check') {
    console.log(JSON.stringify({ healthy: true, summary: 'ktw-display has no configuration', items: [], deps: [], env_keys: [], config_files: [] }))
    process.exit(0)
  }
  if (group === 'config' && verb === 'set' && !Bun.argv.includes('--name')) {
    if (asJSON) console.log('{}')
    process.exit(0)
  }
  if (group === 'config' && (verb === 'get' || verb === 'set')) {
    console.error(`Error: ktw-display has no config file${name && name !== 'json' ? ' ' + name : ''}`)
    process.exit(1)
  }
  console.error(`Error: ktw-display ${CLI_ARGS.slice(SC_AT).join(' ')}: not a ktw-display command (it has no configuration)`)
  process.exit(2)
}
if (CLI_ARGS.some((a) => a === 'help' || a === '-h') || Bun.argv.includes('--help')) {
  console.log(`ktw-display ${VERSION}\n\nUsage: ktw-display [--test]\n\nBoots the data-DSL widget HTTP server. Spawned by ktw-agents.\n\n  version, --version, -v   print version and exit\n  describe                 print marketplace metadata as JSON and exit\n  help, --help, -h         print this help and exit\n  --test                   enable the /test harness surface`)
  process.exit(0)
}

if (PORT_ENV !== undefined && Number.isNaN(PORT)) {
  console.error(`ktw-display: invalid KTW_DISPLAY_PORT=${PORT_ENV}`)
  process.exit(2)
}

// Lightweight request log — one line per non-healthz, non-static-asset
// request. Helps diagnose "iframe set src but server never saw the GET"
// scenarios (e.g. webview blocking, port collision). Healthz + runtime.{js,css}
// are noisy and uninteresting, so they're suppressed.
async function loggedFetch(req: Request): Promise<Response> {
  const start = Date.now()
  const pathname = new URL(req.url).pathname
  // Diagnostic mode: include runtime.{js,css} to confirm the iframe
  // actually fetches them. Healthz stays quiet.
  const interesting =
    pathname !== '/healthz' &&
    !pathname.startsWith('/widget/vendor/')
  try {
    // Every answer carries the CORS headers of the request's origin (src/access.ts).
    const resp = withCors(req, await handleRequest(req))
    if (interesting) {
      console.error(`[req] ${req.method} ${pathname} → ${resp.status} (${Date.now() - start}ms)`)
    }
    return resp
  } catch (err) {
    if (interesting) {
      console.error(`[req] ${req.method} ${pathname} → THREW (${Date.now() - start}ms): ${(err as Error).message}`)
    }
    throw err
  }
}

const server = serve({
  port: PORT,
  hostname: LISTEN_HOSTNAME, // loopback only, never every interface
  fetch: loggedFetch,
  error(err: Error) {
    log({ event: 'ktw-display.error', message: err.message, stack: err.stack })
    return new Response(JSON.stringify({ error: 'internal error' }), {
      status: 500,
      headers: { 'content-type': 'application/json' },
    })
  },
})

log({
  event: 'ktw-display.listening',
  port: server.port,
  hostname: server.hostname,
  version: VERSION,
  pid: process.pid,
  testMode: TEST_MODE,
})

// Graceful shutdown on SIGTERM / SIGINT — sent by ktw-agents at app shutdown.
function shutdown(signal: string) {
  log({ event: 'ktw-display.shutdown', signal })
  server.stop(true)
  process.exit(0)
}
process.on('SIGTERM', () => shutdown('SIGTERM'))
process.on('SIGINT', () => shutdown('SIGINT'))

// --- handler dispatch ---

async function handleRequest(req: Request): Promise<Response> {
  const url = new URL(req.url)

  // CORS preflight: answered for the GUI's origin only (src/access.ts).
  if (req.method === 'OPTIONS') {
    return new Response(null, { status: 204, headers: corsHeaders(req.headers.get('origin')) })
  }

  // /healthz is unauthenticated — used by the lifecycle manager for readiness probes.
  if (url.pathname === '/healthz') {
    return jsonResponse({ status: 'ok', version: VERSION, port: server.port, testMode: TEST_MODE })
  }

  // /widget/runtime.{js,css} — the iframe runtime bundle. Unauthenticated:
  // these files are fetched by the iframe HTML (same origin); the auth gate
  // is on the iframe-mounting /widget/<kind> page that loads them.
  if (url.pathname === '/widget/runtime.js') {
    return serveRuntimeJs()
  }
  if (url.pathname === '/widget/runtime.css') {
    return serveRuntimeCss()
  }

  // /widget/vendor/<subpath> — vendored third-party assets (e.g. drawio's
  // viewer-static.min.js). Lazy-loaded by view kinds that need them; kept
  // out of the main runtime bundle to bound non-drawio bundle size.
  if (url.pathname.startsWith('/widget/vendor/')) {
    const sub = url.pathname.slice('/widget/vendor/'.length)
    return await serveVendor(sub)
  }

  // /test/* — harness routes. Active only when --test flag is set; otherwise
  // returns 404 so the test surface never leaks into production.
  // Unauthenticated (loopback only) for fast development access AND for
  // browser automation.
  if (url.pathname === '/test' || url.pathname.startsWith('/test/')) {
    if (!TEST_MODE) {
      return jsonResponse({ error: 'not found', path: url.pathname }, 404)
    }
    return await handleTestRoute(url)
  }

  // All other routes require the per-session token; without one configured,
  // none is served.
  if (!isAuthenticated(req, url, TOKEN)) {
    return jsonResponse({ error: 'unauthorized' }, 401)
  }

  // Schema-change SSE stream. The Go bridge subscribes here to invalidate
  // its in-process JSON Schema cache when bun mutates schemas. Phase 1:
  // heartbeat-only; future codegen-target-* todos call emitSchemaChanged
  // when persisting changes. Its CORS headers are set in loggedFetch, which
  // passes the long-lived stream through.
  if (url.pathname === '/sse/schema-changed') {
    return sseSchemaChanged(req)
  }

  // Route surfaces — each handler renders its injected payload, or the
  // shared loading screen when none has streamed in yet.
  if (url.pathname === '/widget/form') {
    return formWidget(req)
  }
  if (url.pathname === '/widget/view') {
    return viewWidget(req)
  }
  if (url.pathname === '/widget/table') {
    return tableWidget(req)
  }
  if (url.pathname === '/widget/tree-table') {
    return treeTableWidget(req)
  }
  if (url.pathname === '/widget/wizard') {
    return wizardWidget(req)
  }
  if (url.pathname === '/widget/page') {
    return pageWidget(req)
  }
  if (url.pathname === '/widget/drawio') {
    return drawioWidget(req)
  }
  if (url.pathname === '/widget/deck') {
    return deckWidget(req)
  }
  if (url.pathname === '/widget/deck/store' && req.method === 'POST') {
    return await deckStoreRoute(req)
  }
  if (url.pathname.startsWith('/widget/deck/stream/')) {
    return deckStreamRoute(req, decodeURIComponent(url.pathname.slice('/widget/deck/stream/'.length)))
  }
  if (url.pathname === '/widget/deck/export/pptx' && req.method === 'POST') {
    // The deck page is captured through this server's own loopback origin.
    return await deckExportPptxRoute(req, `http://127.0.0.1:${server.port ?? PORT}`, TOKEN)
  }
  if (url.pathname === '/widget/deck/assets' && req.method === 'POST') {
    return await deckAssetsStoreRoute(req)
  }
  if (url.pathname.startsWith('/widget/deck/assets/')) {
    return deckAssetsGetRoute(url.pathname.slice('/widget/deck/assets/'.length))
  }
  if (url.pathname === '/widget/deck/export/html' && req.method === 'POST') {
    return await deckExportHtmlRoute(req)
  }
  if (url.pathname === '/widget/deck/theme' && req.method === 'POST') {
    return await themeStoreRoute(req)
  }
  if (url.pathname.startsWith('/widget/deck/theme/')) {
    return themeGetRoute(decodeURIComponent(url.pathname.slice('/widget/deck/theme/'.length)))
  }
  if (url.pathname === '/widget/load') {
    return loadWidget(req)
  }
  if (url.pathname.startsWith('/widget/')) {
    return notImplemented(`widget surface (${url.pathname}) — see codegen-target-* + gui-* todos`)
  }
  // /api/parse, /api/types, /api/gen/* — REMOVED 2026-05-22 (schema-realign).
  // The TS-side parser never landed; made
  // the Go side the canonical owner of parsing, validation, and codegen. Zero
  // live callers verified before removal. Only /api/widget-url remains because
  // bun owns URL composition (it knows its own port + session token).
  if (url.pathname === '/api/widget-url' && req.method === 'POST') {
    return await handleWidgetURL(req)
  }

  return jsonResponse({ error: 'not found', path: url.pathname }, 404)
}

// /api/widget-url — composes the iframe URL for a (kind, schema, value, mode)
// tuple, with the per-session token already embedded as a query param.
// Mirrors the bridge's GetWidgetURL slot; the handler is the Phase-1
// implementation called by Go consumers via displaybridge.
async function handleWidgetURL(req: Request): Promise<Response> {
  let body: GetWidgetURLInput
  try {
    body = (await req.json()) as GetWidgetURLInput
  } catch (e) {
    return jsonResponse(
      { error: 'invalid request', detail: e instanceof Error ? e.message : String(e) },
      400,
    )
  }
  try {
    // server.port is typed `number | undefined` because bun.serve({port:0})
    // resolves the actual port asynchronously; in practice it's always set
    // by the time the server handles requests. Fall back to PORT (the
    // configured value) if for any reason it's missing.
    const port = server.port ?? PORT
    const out = buildWidgetURL(body, port, TOKEN)
    return jsonResponse(out)
  } catch (e) {
    if (e instanceof InvalidWidgetURLInputError) {
      return jsonResponse({ error: e.message, code: e.code }, e.status)
    }
    log({ event: 'ktw-display.widget-url.error', message: e instanceof Error ? e.message : String(e) })
    return jsonResponse({ error: 'internal error' }, 500)
  }
}

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json' },
  })
}

function notImplemented(detail: string): Response {
  return jsonResponse({ error: 'not implemented', detail }, 501)
}

// --- /test/* routing (active only when TEST_MODE) ---

async function handleTestRoute(url: URL): Promise<Response> {
  // Subtask 1: shell + healthz.
  // Subtask 2: case enumeration via /test/api/cases.
  // Subtask 3: per-case routing /test/<category>/<case> via renderCasePage.
  if (url.pathname === '/test' || url.pathname === '/test/') {
    return harnessHTML(new Request(url))
  }
  if (url.pathname === '/test/api/cases') {
    const cases = await listCases()
    return jsonResponse(cases)
  }
  // /test/api/cases/<category>/<case> — full TestCase JSON including the
  // optional `script` field. A browser automation fetches this to drive the case.
  if (url.pathname.startsWith('/test/api/cases/')) {
    const id = url.pathname.slice('/test/api/cases/'.length)
    const c = await getCase(id)
    if (!c) {
      return jsonResponse({ error: 'case not found', id }, 404)
    }
    return jsonResponse(c)
  }
  // /test/<category>/<case> — strip the /test/ prefix, look up by id.
  if (url.pathname.startsWith('/test/')) {
    const id = url.pathname.slice('/test/'.length)
    const c = await getCase(id)
    if (!c) {
      return jsonResponse({ error: 'case not found', id }, 404)
    }
    return renderCasePage(c)
  }
  return jsonResponse({ error: 'unknown test route', path: url.pathname }, 404)
}

function log(record: Record<string, unknown>) {
  // Single-line JSON to stderr — matches Go-side sidecar logging convention.
  console.error(JSON.stringify({ ts: new Date().toISOString(), ...record }))
}

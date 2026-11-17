// SPDX-License-Identifier: Apache-2.0
// Deck exports (modules/slidedeck/): a PowerPoint of pictures (th12) and a
// standalone HTML page (th13).
//
// ── PowerPoint, as pictures ──
//
// Each slide is shown by the deck page itself — the same page the GUI embeds
// and presents, with its theme — in a headless Chromium browser found on the
// machine (Edge, Chrome or Chromium; KTW_BROWSER overrides), driven over the
// DevTools protocol, and captured as a PNG. The PNGs become a .pptx with
// PptxGenJS: one full-bleed picture per slide, the slide's notes as speaker
// notes. What PowerPoint shows is exactly what the deck shows; the text is not
// editable (level 2 builds real slides).
//
// Route (under /widget/ so the query-string token authenticates it):
//
//   POST /widget/deck/export/pptx   {markdown, title?, editable?} → the .pptx bytes
//
// With `editable`, the slides are real PowerPoint slides — text boxes,
// pictures, shapes — measured from the same page (deck-pptx.ts).
//
// The deck's theme must already be in the theme store (the host publishes it
// before, as it does to present).
//
// ── A standalone HTML page ──
//
// One .html file that presents the deck with no server and no network: the
// presenter (the deck component, reveal.js — _compiled/deck-standalone.*), the
// deck's markdown and its theme with the files the deck uses (fonts, logo,
// backgrounds, the pictograms it names) as data: URIs, and the fonts'
// licences. Opened by a double click; sent by email.
//
//   POST /widget/deck/export/html   {markdown, title?} → the page

import { existsSync, mkdtempSync, readFileSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import PptxGenJS from 'pptxgenjs'
import { parseDeck } from '@ktw/display-lib/ui-builders/deck/parse.ts'
import { DEFAULT_THEME, deckAspect, isColour, type Theme } from '@ktw/display-lib/ui-builders/deck/theme.ts'
import { assetsRef, putDeck, type AssetsRef } from './deck-store.ts'
import { getDeckAssets } from './deck-assets.ts'
import { EXTRACT_JS, MATERIALIZE_JS, modelsToPptx, type SlideModel } from './deck-pptx.ts'
import { assetType, getTheme } from './theme-store.ts'
// The diagram drawers a standalone page carries, only when the deck draws
// such a diagram (each is larger than the rest of the page), with their
// licences.
import mermaidJs from './runtime/_compiled/mermaid.min.js' with { type: 'text' }
import mermaidLicence from './runtime/_compiled/mermaid.LICENSE' with { type: 'text' }
import drawioViewerJs from './runtime/_compiled/drawio-viewer-static.min.js' with { type: 'text' }
import drawioNotice from './runtime/_compiled/drawio-viewer-static.NOTICE' with { type: 'text' }

/** The diagram kinds a deck draws: what its standalone page must carry. */
export function diagramKinds(markdown: string): { mermaid: boolean; drawio: boolean } {
  return {
    mermaid: /^\s*(```|~~~)\s*mermaid\b/m.test(markdown),
    drawio: /\]\([^)\s]*\.drawio(\.xml)?\)/i.test(markdown),
  }
}
import standaloneJs from './runtime/_compiled/deck-standalone.bundle.js' with { type: 'text' }
import standaloneCss from './runtime/_compiled/deck-standalone.bundle.css' with { type: 'text' }

export const PPTX_MEDIA_TYPE = 'application/vnd.openxmlformats-officedocument.presentationml.presentation'

/** Pixel size each slide is captured at, by the deck's aspect. */
const CAPTURE: Record<string, { width: number; height: number; layout: string }> = {
  '16:9': { width: 1920, height: 1080, layout: 'LAYOUT_WIDE' },
  '4:3': { width: 1440, height: 1080, layout: 'LAYOUT_4x3' },
}

/** Where a Chromium-family browser is looked for, in order: KTW_BROWSER,
 *  then where Edge, Chrome and Chromium install. */
export function browserCandidates(): string[] {
  const env = Bun.env.KTW_BROWSER
  const pf = Bun.env.ProgramFiles ?? 'C:\\Program Files'
  const pf86 = Bun.env['ProgramFiles(x86)'] ?? 'C:\\Program Files (x86)'
  const local = Bun.env.LOCALAPPDATA ?? ''
  const candidates = process.platform === 'win32'
    ? [
        join(pf86, 'Microsoft', 'Edge', 'Application', 'msedge.exe'),
        join(pf, 'Microsoft', 'Edge', 'Application', 'msedge.exe'),
        join(pf, 'Google', 'Chrome', 'Application', 'chrome.exe'),
        join(pf86, 'Google', 'Chrome', 'Application', 'chrome.exe'),
        join(local, 'Google', 'Chrome', 'Application', 'chrome.exe'),
      ]
    : process.platform === 'darwin'
      ? [
          '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
          '/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge',
          '/Applications/Chromium.app/Contents/MacOS/Chromium',
        ]
      : ['google-chrome', 'google-chrome-stable', 'chromium', 'chromium-browser', 'microsoft-edge']
          .map((n) => Bun.which(n) ?? `${n} (on PATH)`)
  return env ? [env, ...candidates] : candidates
}

/** A Chromium-family browser on this machine, or undefined. */
export function findBrowser(): string | undefined {
  return browserCandidates().find((c) => c && existsSync(c))
}

/** The error of a machine with no browser: what is missing, where it was
 *  looked for, how to provide it. */
export function noBrowserMessage(): string {
  const env = Bun.env.KTW_BROWSER
  const looked = browserCandidates().filter((c) => c && c !== env)
  return `no Chromium browser found to capture the slides (Edge, Chrome or Chromium). ` +
    `Looked at: ${env ? `KTW_BROWSER=${env} (no such file); ` : 'KTW_BROWSER (unset); '}${looked.join('; ')}. ` +
    `Install one of them, or set KTW_BROWSER to a Chromium-family browser's executable in the display server's environment`
}

/** The style a captured page carries: every reveal step shown, as on the
 *  slide's last step. */
export const ALL_STEPS_CSS = '.reveal .slides section .fragment { opacity: 1 !important; visibility: inherit !important; transform: none !important; }'

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms))

/** How long one DevTools call may wait for the browser: a page that hangs
 *  (an endless script, a dialog) or a browser that died fails the export
 *  instead of holding the export queue for ever. */
const CALL_TIMEOUT_MS = 90_000

/** A DevTools-protocol session on one page. */
class Page {
  private id = 0
  private pending = new Map<number, { done: (v: { result?: any; error?: { message: string } }) => void; fail: (e: Error) => void }>()
  private inflight = new Set<string>()
  private lastActivity = Date.now()

  private constructor(private ws: WebSocket) {
    // The browser went away: every call still waiting fails now.
    const lost = () => {
      for (const p of this.pending.values()) p.fail(new Error('the browser closed the connection'))
      this.pending.clear()
    }
    ws.addEventListener('close', lost)
    ws.addEventListener('error', lost)
    ws.addEventListener('message', (e) => {
      const m = JSON.parse(String(e.data))
      if (m.id && this.pending.has(m.id)) {
        this.pending.get(m.id)!.done(m)
        this.pending.delete(m.id)
        return
      }
      if (m.method === 'Network.requestWillBeSent') this.inflight.add(m.params.requestId)
      else if (m.method === 'Network.loadingFinished' || m.method === 'Network.loadingFailed') this.inflight.delete(m.params.requestId)
      else return
      this.lastActivity = Date.now()
    })
  }

  static async open(url: string): Promise<Page> {
    const ws = new WebSocket(url)
    await new Promise<void>((resolve, reject) => {
      ws.addEventListener('open', () => resolve(), { once: true })
      ws.addEventListener('error', () => reject(new Error('cannot reach the browser')), { once: true })
    })
    return new Page(ws)
  }

  send(method: string, params: object = {}): Promise<any> {
    const i = ++this.id
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pending.delete(i)
        reject(new Error(`${method}: no answer from the browser in ${CALL_TIMEOUT_MS / 1000} s`))
      }, CALL_TIMEOUT_MS)
      this.pending.set(i, {
        done: (m) => {
          clearTimeout(timer)
          if (m.error) reject(new Error(`${method}: ${m.error.message}`))
          else resolve(m.result)
        },
        fail: (e) => {
          clearTimeout(timer)
          reject(e)
        },
      })
      try {
        this.ws.send(JSON.stringify({ id: i, method, params }))
      } catch (e) {
        clearTimeout(timer)
        this.pending.delete(i)
        reject(e instanceof Error ? e : new Error(String(e)))
      }
    })
  }

  async evaluate<T>(expression: string): Promise<T> {
    const r = await this.send('Runtime.evaluate', { expression, returnByValue: true, awaitPromise: true })
    if (r.exceptionDetails) throw new Error(`page: ${r.exceptionDetails.exception?.description ?? r.exceptionDetails.text}`)
    return r.result?.value as T
  }

  /** Waits until no request has been in flight for `quiet` ms (at most `max`). */
  async networkIdle(quiet = 300, max = 8000): Promise<void> {
    const start = Date.now()
    while (Date.now() - start < max) {
      if (this.inflight.size === 0 && Date.now() - this.lastActivity >= quiet) return
      await sleep(50)
    }
  }

  close() { this.ws.close() }
}

/** Starts a headless browser and returns its first page's session. */
async function launch(browser: string, size: { width: number; height: number }) {
  const profile = mkdtempSync(join(tmpdir(), 'ktw-deck-export-'))
  const proc = Bun.spawn([
    browser, '--headless=new', '--disable-gpu', '--hide-scrollbars', '--no-first-run', '--no-default-browser-check',
    '--remote-debugging-port=0', `--user-data-dir=${profile}`, `--window-size=${size.width},${size.height}`, 'about:blank',
  ], { stdout: 'ignore', stderr: 'ignore' })
  const stop = () => {
    try { proc.kill() } catch { /* gone */ }
    // The profile is the browser's: removed once it has let go of it.
    setTimeout(() => { try { rmSync(profile, { recursive: true, force: true }) } catch { /* still locked */ } }, 1500)
  }
  try {
    // The browser writes its debugging port to the profile once it listens.
    let port = 0
    for (let i = 0; i < 100 && !port; i++) {
      try { port = Number(readFileSync(join(profile, 'DevToolsActivePort'), 'utf8').split('\n')[0]) } catch { await sleep(100) }
    }
    if (!port) throw new Error(`the browser did not start: ${browser} (a Chromium browser; KTW_BROWSER names another)`)
    let wsURL = ''
    for (let i = 0; i < 50 && !wsURL; i++) {
      try {
        const list = (await (await fetch(`http://127.0.0.1:${port}/json`)).json()) as { type: string; webSocketDebuggerUrl: string }[]
        wsURL = list.find((t) => t.type === 'page')?.webSocketDebuggerUrl ?? ''
      } catch { await sleep(100) }
    }
    if (!wsURL) throw new Error('the browser has no page')
    const page = await Page.open(wsURL)
    await page.send('Emulation.setDeviceMetricsOverride', { ...size, deviceScaleFactor: 1, mobile: false })
    await page.send('Network.enable')
    return { page, stop }
  } catch (e) {
    stop()
    throw e
  }
}

/** The deck page's slides captured as PNG (base64), in order — or, when
 *  `measure`, each slide's measured model (deck-pptx.ts). */
export async function captureSlides(pageURL: string, aspect: string): Promise<string[]>
export async function captureSlides(pageURL: string, aspect: string, measure: true): Promise<SlideModel[]>
export async function captureSlides(pageURL: string, aspect: string, measure = false): Promise<string[] | SlideModel[]> {
  const browser = findBrowser()
  if (!browser) throw new Error(noBrowserMessage())
  const size = CAPTURE[aspect] ?? CAPTURE['16:9']
  const { page, stop } = await launch(browser, size)
  try {
    await page.send('Page.navigate', { url: pageURL })
    // Ready: reveal.js runs, and a named theme has brought its style.
    const ready = await page.evaluate<boolean>(`(async () => {
      for (let i = 0; i < 150; i++) {
        const d = document.querySelector('.ktw-deck'), r = document.querySelector('.reveal')?.revealDeck
        if (d && r && (d.getAttribute('data-deck-theme') === 'katyoti' || d.querySelector('style'))) return true
        await new Promise((ok) => setTimeout(ok, 100))
      }
      return false
    })()`)
    if (!ready) throw new Error('the deck page did not render')
    // Every reveal step shown, nothing moving, no navigation chrome.
    const count = await page.evaluate<number>(`(() => {
      const r = document.querySelector('.reveal').revealDeck
      r.configure({ controls: false, progress: false, slideNumber: false, fragments: false, transition: 'none', backgroundTransition: 'none' })
      // fragments: false marks the steps visible once, but the deck page
      // re-syncs reveal.js after each render and the marks go (a steps=list
      // slide was exported with its title only): every step is shown by
      // style, whatever its classes.
      const all = document.createElement('style')
      all.textContent = ${JSON.stringify(ALL_STEPS_CSS)}
      document.head.appendChild(all)
      return r.getTotalSlides()
    })()`)
    if (measure) await page.evaluate(MATERIALIZE_JS)
    const shots: (string | SlideModel)[] = []
    for (let n = 0; n < count; n++) {
      await page.evaluate(`(async () => {
        const r = document.querySelector('.reveal').revealDeck
        const s = r.getSlides()[${n}]
        const { h, v } = r.getIndices(s)
        r.slide(h, v)
        await document.fonts.ready
      })()`)
      await page.networkIdle()
      // Pictures decoded, then one frame for the layout to settle.
      await page.evaluate(`Promise.all([...document.images].map((i) => i.decode().catch(() => {}))).then(() => new Promise((ok) => requestAnimationFrame(() => setTimeout(ok, 150))))`)
      if (measure) {
        shots.push(await page.evaluate<SlideModel>(EXTRACT_JS))
      } else {
        const shot = await page.send('Page.captureScreenshot', { format: 'png', captureBeyondViewport: false })
        shots.push(shot.data as string)
      }
    }
    page.close()
    return shots as string[] | SlideModel[]
  } finally {
    stop()
  }
}

/** A .pptx of one full-bleed picture per slide, with the speaker notes. */
export async function picturesToPptx(shots: string[], opts: { aspect: string; title?: string; author?: string; notes?: (string | undefined)[] }): Promise<Uint8Array> {
  const pptx = new PptxGenJS()
  pptx.layout = (CAPTURE[opts.aspect] ?? CAPTURE['16:9']).layout
  if (opts.title) pptx.title = opts.title
  if (opts.author) pptx.author = opts.author
  shots.forEach((png, i) => {
    const slide = pptx.addSlide()
    slide.addImage({ data: `image/png;base64,${png}`, x: 0, y: 0, w: '100%', h: '100%' })
    const note = opts.notes?.[i]
    if (note) slide.addNotes(note)
  })
  const out = await pptx.write({ outputType: 'uint8array' })
  return out as Uint8Array
}

let queue: Promise<unknown> = Promise.resolve()

/** POST /widget/deck/export/pptx — one export at a time: each runs a browser. */
export async function deckExportPptxRoute(req: Request, origin: string, token: string): Promise<Response> {
  let body: { markdown?: unknown; title?: unknown; editable?: unknown; assets?: unknown }
  try {
    body = await req.json()
  } catch {
    return Response.json({ error: 'body must be JSON {markdown, title?, editable?}' }, { status: 400 })
  }
  if (typeof body.markdown !== 'string' || !body.markdown.trim()) {
    return Response.json({ error: 'markdown (string) is required' }, { status: 400 })
  }
  const markdown = body.markdown
  const editable = body.editable === true
  const deck = parseDeck(markdown)
  const settings = deck.settings as Record<string, unknown>
  const themeName = typeof settings.theme === 'string' && settings.theme ? settings.theme : DEFAULT_THEME
  const aspect = deckAspect(settings, getTheme(themeName)?.body as Theme | undefined)
  const title = typeof body.title === 'string' && body.title ? body.title : typeof settings.title === 'string' ? settings.title : undefined
  const author = typeof settings.author === 'string' && settings.author ? settings.author : undefined
  const run = queue.then(async () => {
    const { id } = putDeck(markdown, undefined, assetsRef(body.assets))
    const url = `${origin}/widget/deck?deck=${encodeURIComponent(id)}&present=1${token ? `&token=${encodeURIComponent(token)}` : ''}`
    if (editable) {
      const models = await captureSlides(url, aspect, true)
      const notes = models.length === deck.slides.length ? deck.slides.map((s) => s.notes) : undefined
      const name = typeof settings.theme === 'string' ? settings.theme : ''
      const theme = name ? (getTheme(name)?.body as Theme | undefined) : undefined
      return modelsToPptx(models, { title, author, notes, theme })
    }
    const shots = await captureSlides(url, aspect)
    // reveal.js and the parser count the same slides, in the same order.
    const notes = shots.length === deck.slides.length ? deck.slides.map((s) => s.notes) : undefined
    return picturesToPptx(shots, { aspect, title, author, notes })
  })
  queue = run.catch(() => {})
  try {
    const bytes = await run
    return new Response(new Blob([bytes as BlobPart]), { headers: { 'content-type': PPTX_MEDIA_TYPE } })
  } catch (e) {
    return Response.json({ error: e instanceof Error ? e.message : String(e) }, { status: 500 })
  }
}

/** The theme files a deck uses: its fonts, its chrome's logo, every look's
 *  background and logo, and the pictograms the deck names. */
export function usedAssets(theme: Theme, markdown: string): string[] {
  const out = new Set<string>()
  for (const f of theme.fonts ?? []) out.add(f.src)
  if (theme.chrome?.logo?.asset) out.add(theme.chrome.logo.asset)
  for (const l of Object.values(theme.layouts ?? {})) {
    for (const look of [l?.default, ...Object.values(l?.variants ?? {})]) {
      if (look?.background && !isColour(look.background)) out.add(look.background)
      if (look?.logo?.asset) out.add(look.logo.asset)
    }
  }
  for (const m of markdown.matchAll(/\]\(icon:([a-z0-9-]+)\)/g)) {
    const icon = theme.icons?.[m[1]]
    if (icon?.asset) out.add(icon.asset)
  }
  return [...out]
}

/** Text safe inside an inline <script>: no `</script` can close it early. */
const inlineScript = (text: string) => text.replace(/<\/(script)/gi, '<\\/$1')

const escapeHTML = (t: string) => t.replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' })[c]!)

/** The standalone page of a deck; its theme, when not katyoti, from the store. */
export function standalonePage(markdown: string, title?: string, assets?: AssetsRef): { html: string; error?: string } {
  const settings = parseDeck(markdown).settings as Record<string, unknown>
  const name = typeof settings.theme === 'string' && settings.theme ? settings.theme : DEFAULT_THEME
  let theme: { body: Theme; assets: Record<string, string> } | undefined
  let licences = ''
  if (name !== DEFAULT_THEME) {
    const stored = getTheme(name)
    if (!stored) return { html: '', error: `theme ${name} is not in the theme store: publish it first` }
    const body = stored.body as Theme
    const assets: Record<string, string> = {}
    for (const path of usedAssets(body, markdown)) {
      const bytes = stored.assets.get(path)
      if (bytes) assets[path] = `data:${assetType(path)};base64,${Buffer.from(bytes).toString('base64')}`
    }
    theme = { body, assets }
    // The licences of the files carried with them (a font's OFL).
    licences = (body.notices ?? [])
      .map((n) => stored.assets.get(n))
      .filter((b): b is Uint8Array<ArrayBuffer> => !!b)
      .map((b) => new TextDecoder().decode(b))
      .join('\n\n---\n\n')
  }
  const heading = title || (typeof settings.title === 'string' ? settings.title : '') || 'Presentation'
  // The files beside the deck, as data: URIs.
  const local: Record<string, string> = {}
  const store = assets ? getDeckAssets(assets.scope) : undefined
  if (store && store.version === assets!.version) {
    for (const [p, bytes] of store.files) local[p] = `data:${assetType(p)};base64,${Buffer.from(bytes).toString('base64')}`
  }
  const data = inlineScript(JSON.stringify({ markdown, theme, assets: local }))
  const kinds = diagramKinds(markdown)
  const drawers = (kinds.mermaid ? `<script>${inlineScript(mermaidJs)}</script>\n` : '') + (kinds.drawio ? `<script>${inlineScript(drawioViewerJs)}</script>\n` : '')
  if (kinds.mermaid) licences += `${licences ? '\n\n---\n\n' : ''}mermaid\n\n${mermaidLicence}`
  if (kinds.drawio) licences += `${licences ? '\n\n---\n\n' : ''}draw.io viewer\n\n${drawioNotice}`
  const html = `<!doctype html>
<html lang="${typeof settings.lang === 'string' ? escapeHTML(settings.lang) : 'fr'}">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="generator" content="KaTyoti — standalone deck">${typeof settings.author === 'string' && settings.author ? `
<meta name="author" content="${escapeHTML(settings.author)}">` : ''}
<title>${escapeHTML(heading)}</title>
<style>${standaloneCss}</style>
<style>html, body { margin: 0; height: 100%; background: #000; overflow: hidden; } #root { height: 100%; }</style>
</head>
<body>
<div id="root"></div>
<script type="application/json" id="ktw-deck">${data}</script>
${licences ? `<script type="text/plain" id="ktw-deck-licences">${inlineScript(escapeHTML(licences))}</script>\n` : ''}${drawers}<script type="module">${inlineScript(standaloneJs)}</script>
</body>
</html>
`
  return { html }
}

/** POST /widget/deck/export/html */
export async function deckExportHtmlRoute(req: Request): Promise<Response> {
  let body: { markdown?: unknown; title?: unknown; assets?: unknown }
  try {
    body = await req.json()
  } catch {
    return Response.json({ error: 'body must be JSON {markdown, title?, assets?}' }, { status: 400 })
  }
  if (typeof body.markdown !== 'string' || !body.markdown.trim()) {
    return Response.json({ error: 'markdown (string) is required' }, { status: 400 })
  }
  const { html, error } = standalonePage(body.markdown, typeof body.title === 'string' ? body.title : undefined, assetsRef(body.assets))
  if (error) return Response.json({ error }, { status: 409 })
  return new Response(html, { headers: { 'content-type': 'text/html; charset=utf-8' } })
}

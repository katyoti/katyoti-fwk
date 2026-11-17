// SPDX-License-Identifier: Apache-2.0
// Who may reach ktw-display: the server listens on loopback
// only, refuses every authenticated surface when it was started without a
// token, and answers a cross-origin caller with the GUI's origin only, never `*`.
import { afterAll, describe, expect, test } from 'bun:test'
import { join } from 'node:path'
import { LISTEN_HOSTNAME, corsHeaders, isAllowedOrigin, isAuthenticated } from '../src/access.ts'

const INDEX = join(import.meta.dir, '..', 'src', 'index.ts')

type Server = { base: string; hostname: string; stop: () => void }
const started: Server[] = []

/** Starts the server on a free port, with the token given (none when ''). */
async function start(token: string): Promise<Server> {
  const env: Record<string, string> = { ...(process.env as Record<string, string>), KTW_DISPLAY_PORT: '0' }
  delete env.KTW_DISPLAY_TOKEN
  if (token) env.KTW_DISPLAY_TOKEN = token
  const p = Bun.spawn(['bun', INDEX], { env, stdout: 'ignore', stderr: 'pipe' })
  const reader = p.stderr.getReader()
  const dec = new TextDecoder()
  let buf = ''
  const deadline = Date.now() + 20_000
  while (Date.now() < deadline) {
    const { value, done } = await reader.read()
    if (done) break
    buf += dec.decode(value)
    for (const line of buf.split('\n')) {
      if (!line.includes('ktw-display.listening')) continue
      const rec = JSON.parse(line) as { port: number; hostname: string }
      reader.releaseLock()
      const s = { base: `http://127.0.0.1:${rec.port}`, hostname: rec.hostname, stop: () => p.kill() }
      started.push(s)
      return s
    }
  }
  p.kill()
  throw new Error(`ktw-display did not start: ${buf}`)
}

afterAll(() => {
  for (const s of started) s.stop()
})

describe('ktw-display access', () => {
  test('the server listens on loopback only', async () => {
    expect(LISTEN_HOSTNAME).toBe('127.0.0.1')
    const s = await start('t0k')
    expect(s.hostname).toBe('127.0.0.1')
  }, 30_000)

  test('a server started without a token serves no authenticated surface', async () => {
    const s = await start('')
    expect((await fetch(`${s.base}/widget/view`)).status).toBe(401)
    expect((await fetch(`${s.base}/widget/view?token=`)).status).toBe(401)
    expect((await fetch(`${s.base}/api/widget-url`, { method: 'POST', body: '{}' })).status).toBe(401)
    expect((await fetch(`${s.base}/healthz`)).status).toBe(200)
  }, 30_000)

  test('with a token, a request carrying it is served and one without it refused', async () => {
    const s = await start('t0k')
    expect((await fetch(`${s.base}/widget/view?token=t0k`)).status).toBe(200)
    expect((await fetch(`${s.base}/widget/view?token=nope`)).status).toBe(401)
    expect((await fetch(`${s.base}/api/widget-url`, { method: 'POST', body: '{}' })).status).toBe(401)
  }, 30_000)

  test('a cross-origin answer names the GUI origin, never *', async () => {
    const s = await start('t0k')
    const gui = await fetch(`${s.base}/widget/view?token=t0k`, { headers: { origin: 'http://wails.localhost' } })
    expect(gui.headers.get('access-control-allow-origin')).toBe('http://wails.localhost')
    const pre = await fetch(`${s.base}/widget/deck/store`, { method: 'OPTIONS', headers: { origin: 'wails://wails' } })
    expect(pre.headers.get('access-control-allow-origin')).toBe('wails://wails')
    const evil = await fetch(`${s.base}/widget/view?token=t0k`, { headers: { origin: 'https://evil.example' } })
    expect(evil.headers.get('access-control-allow-origin')).toBeNull()
    const none = await fetch(`${s.base}/healthz`)
    expect(none.headers.get('access-control-allow-origin')).toBeNull()
  }, 30_000)
})

describe('access rules', () => {
  test('allowed origins are the GUI webview and loopback pages only', () => {
    for (const o of ['http://wails.localhost', 'https://wails.localhost', 'wails://wails', 'http://localhost:34115', 'http://127.0.0.1:5173']) {
      expect(isAllowedOrigin(o)).toBe(true)
    }
    for (const o of [null, '', 'null', 'https://evil.example', 'http://localhost.evil.example', 'http://127.0.0.1.evil.example:80']) {
      expect(isAllowedOrigin(o)).toBe(false)
    }
    expect(Object.values(corsHeaders('https://evil.example'))).not.toContain('*')
    expect(corsHeaders('http://wails.localhost')['access-control-allow-origin']).toBe('http://wails.localhost')
  })

  test('no token configured authenticates nothing', () => {
    const url = new URL('http://127.0.0.1/widget/view?token=')
    expect(isAuthenticated(new Request(url), url, '')).toBe(false)
    const api = new URL('http://127.0.0.1/api/widget-url')
    expect(isAuthenticated(new Request(api, { headers: { authorization: 'Bearer ' } }), api, '')).toBe(false)
    expect(isAuthenticated(new Request(api, { headers: { authorization: 'Bearer k' } }), api, 'k')).toBe(true)
  })
})

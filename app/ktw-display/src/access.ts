// SPDX-License-Identifier: Apache-2.0
// Who may reach ktw-display: the server listens on the loopback address only,
// every surface but the static runtime assets and /healthz needs the
// per-session token (a server started without one serves none of them), and
// a cross-origin answer names only the GUI's origin - never `*`.

/** The address the server listens on: loopback, never every interface. */
export const LISTEN_HOSTNAME = '127.0.0.1'

/** The origins the GUI's webview loads from (Wails v2: WebView2 on Windows
 *  maps its assets onto http(s)://wails.localhost, WebKit on macOS and Linux
 *  onto wails://wails). */
const GUI_ORIGINS = new Set([
  'http://wails.localhost',
  'https://wails.localhost',
  'wails://wails',
  'wails://wails.localhost',
])

/** A loopback origin: the GUI's dev front-end in a browser (wails dev,
 *  vite) and this server's own pages. A page from any other host never is. */
const LOOPBACK_ORIGIN = /^https?:\/\/(127\.0\.0\.1|localhost|\[::1\])(:\d+)?$/

/** Whether a cross-origin caller may read the answer. */
export function isAllowedOrigin(origin: string | null): origin is string {
  if (!origin) return false
  return GUI_ORIGINS.has(origin) || LOOPBACK_ORIGIN.test(origin)
}

/** The CORS headers for a request: none when it carries no allowed origin,
 *  else that origin itself (with Vary: Origin), never `*`. */
export function corsHeaders(origin: string | null): Record<string, string> {
  if (!isAllowedOrigin(origin)) return { vary: 'Origin' }
  return {
    'access-control-allow-origin': origin,
    'access-control-allow-methods': 'GET, POST, PUT, DELETE, OPTIONS',
    'access-control-allow-headers': 'authorization, content-type',
    'access-control-max-age': '86400',
    vary: 'Origin',
  }
}

/** The response with the request's CORS headers (any `*` a handler set is
 *  replaced or dropped). The body is passed through, so a stream stays one. */
export function withCors(req: Request, resp: Response): Response {
  const headers = new Headers(resp.headers)
  headers.delete('access-control-allow-origin')
  for (const [k, v] of Object.entries(corsHeaders(req.headers.get('origin')))) headers.set(k, v)
  return new Response(resp.body, { status: resp.status, statusText: resp.statusText, headers })
}

/** Whether a request carries the session token: a widget page by its
 *  `?token=` (iframe URLs), an API call by `Authorization: Bearer`. No token
 *  configured = nothing is authenticated. */
export function isAuthenticated(req: Request, url: URL, token: string): boolean {
  if (!token) return false
  if (url.pathname.startsWith('/widget/')) {
    return url.searchParams.get('token') === token
  }
  return (req.headers.get('authorization') ?? '') === `Bearer ${token}`
}

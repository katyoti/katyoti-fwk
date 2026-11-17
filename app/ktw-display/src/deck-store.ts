// SPDX-License-Identifier: Apache-2.0
// The deck store — decks the display server holds so a page can load one by
// id and follow it live (modules/slidedeck/, OD-004).
//
// Why a store: a deck presented in the system browser, and the speaker view's
// preview frames that reload the same URL, must get the deck from the URL.
// The markdown is too long for the URL, so the host posts it here once and the
// URL carries `?deck=<id>`. Every later post of the same id is pushed to every
// open page over SSE — an agent editing the deck, or the author, updates the
// running presentation.
//
// Routes (under /widget/ so the query-string token authenticates them, which
// EventSource needs — it cannot send an Authorization header):
//
//   POST /widget/deck/store          {id?, markdown} → {id, version}
//   GET  /widget/deck/stream/<id>    SSE: `event: deck` / `data: {markdown, version}`
//
// In memory, per process: the source of a deck stays its file or object; the
// store only relays it to pages.

/** Where the files beside a deck are in the deck-assets store. */
export interface AssetsRef { scope: string; version: string }

interface StoredDeck {
  markdown: string
  version: number
  assets?: AssetsRef
}

const decks = new Map<string, StoredDeck>()
const listeners = new Map<string, Set<(d: StoredDeck) => void>>()

const MAX_DECKS = 200
const ID = /^[A-Za-z0-9_-]{1,64}$/

export function getDeck(id: string): StoredDeck | undefined {
  return decks.get(id)
}

/** Stores a deck (a new id when none is given) and notifies its pages. */
export function putDeck(markdown: string, id?: string, assets?: AssetsRef): { id: string; version: number } {
  const key = id && ID.test(id) ? id : crypto.randomUUID().replace(/-/g, '').slice(0, 16)
  const prev = decks.get(key)
  const next = { markdown, version: (prev?.version ?? 0) + 1, assets }
  decks.delete(key)
  decks.set(key, next)
  while (decks.size > MAX_DECKS) {
    const oldest = decks.keys().next().value as string
    decks.delete(oldest)
  }
  for (const fn of listeners.get(key) ?? []) fn(next)
  return { id: key, version: next.version }
}

/** POST /widget/deck/store */
export async function deckStoreRoute(req: Request): Promise<Response> {
  let body: { id?: unknown; markdown?: unknown; assets?: unknown }
  try {
    body = await req.json()
  } catch {
    return json({ error: 'body must be JSON {id?, markdown}' }, 400)
  }
  if (typeof body.markdown !== 'string') return json({ error: 'markdown (string) is required' }, 400)
  const id = typeof body.id === 'string' ? body.id : undefined
  if (id !== undefined && !ID.test(id)) return json({ error: 'id must match [A-Za-z0-9_-]{1,64}' }, 400)
  return json(putDeck(body.markdown, id, assetsRef(body.assets)))
}

/** GET /widget/deck/stream/<id> — the current deck, then every update. */
export function deckStreamRoute(req: Request, id: string): Response {
  const encoder = new TextEncoder()
  let heartbeat: ReturnType<typeof setInterval> | undefined
  let fn: ((d: StoredDeck) => void) | undefined
  const stream = new ReadableStream<Uint8Array>({
    start(controller) {
      const send = (d: StoredDeck) => {
        try {
          controller.enqueue(encoder.encode(`event: deck\ndata: ${JSON.stringify(d)}\n\n`))
        } catch {
          /* closed */
        }
      }
      fn = send
      let set = listeners.get(id)
      if (!set) listeners.set(id, (set = new Set()))
      set.add(send)
      const current = decks.get(id)
      if (current) send(current)
      heartbeat = setInterval(() => {
        try {
          controller.enqueue(encoder.encode(': heartbeat\n\n'))
        } catch {
          /* closed */
        }
      }, 25_000)
      req.signal.addEventListener('abort', () => {
        clearInterval(heartbeat)
        if (fn) listeners.get(id)?.delete(fn)
        try {
          controller.close()
        } catch {
          /* already closed */
        }
      })
    },
    cancel() {
      clearInterval(heartbeat)
      if (fn) listeners.get(id)?.delete(fn)
    },
  })
  return new Response(stream, {
    headers: {
      'content-type': 'text/event-stream',
      'cache-control': 'no-cache',
      connection: 'keep-alive',
    },
  })
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'content-type': 'application/json' } })
}

/** An `assets` value from a request: {scope, version}, else undefined. */
export function assetsRef(v: unknown): AssetsRef | undefined {
  const a = v as { scope?: unknown; version?: unknown } | null
  return a && typeof a.scope === 'string' && typeof a.version === 'string' ? { scope: a.scope, version: a.version } : undefined
}

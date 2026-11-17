// SPDX-License-Identifier: Apache-2.0
// Server-Sent Events stream for schema-change notifications.
//
// Phase 1 surface (heartbeat-only): the bridge subscribes from Go to
// keep its in-memory JSON Schema cache invalidated when bun mutates a
// schema. Today bun has no schema mutation — the InstallSchema handler
// is a 501 stub — so this stream emits a heartbeat every 30 seconds
// and nothing else. As codegen-target-* todos land, they call
// emitSchemaChanged(name, version) when persisting; the heartbeat keeps
// the connection alive in the meantime so the bridge's reconnect loop
// doesn't churn.
//
// Wire format (HTML5 SSE subset the Go bridge parses):
//
//   event: schema-changed
//   data: {"name":"User","version":3}
//
//   : heartbeat
//
//   <blank line ends each event>
//
// See internal/extensions/displaybridge/internal/sse.go for the matching
// Go-side parser.

const HEARTBEAT_MS = 30_000

// Active streams' write functions, captured per-connection. Each entry
// pushes one SSE record into a single client. Subscribers are removed on
// abort (client disconnect) or stream close (process exit). Module-level
// state is fine here — the sidecar is single-process.
const writers = new Set<(record: string) => void>()

/**
 * sseSchemaChanged builds the long-lived response for /sse/schema-changed.
 * Caller (route dispatcher in index.ts) is responsible for the auth gate;
 * this function trusts that the connection is authorized.
 */
export function sseSchemaChanged(req: Request): Response {
  const encoder = new TextEncoder()
  let heartbeat: ReturnType<typeof setInterval> | undefined

  const stream = new ReadableStream<Uint8Array>({
    start(controller) {
      const write = (record: string) => {
        try {
          controller.enqueue(encoder.encode(record))
        } catch {
          // Stream closed; nothing to do — the abort listener cleans up.
        }
      }

      // Initial connect comment so the bridge's parser sees data
      // immediately and registers the connection as healthy.
      write(': connected\n\n')

      heartbeat = setInterval(() => {
        write(': heartbeat\n\n')
      }, HEARTBEAT_MS)

      writers.add(write)

      // Bun signals client disconnect via the request's AbortSignal.
      req.signal.addEventListener('abort', () => {
        writers.delete(write)
        if (heartbeat) clearInterval(heartbeat)
        try {
          controller.close()
        } catch {
          // Already closed.
        }
      })
    },
    cancel() {
      // Reader detached (rare; the abort path is what we usually hit).
      writers.delete(/* noop sentinel — already removed above */ () => {})
      if (heartbeat) clearInterval(heartbeat)
    },
  })

  return new Response(stream, {
    status: 200,
    headers: {
      'content-type': 'text/event-stream',
      'cache-control': 'no-cache',
      'connection': 'keep-alive',
      'x-accel-buffering': 'no', // disable proxy buffering when behind one
    },
  })
}

/**
 * emitSchemaChanged broadcasts a schema-changed event to every active
 * subscriber. Called by future codegen-target-* / install-schema handlers
 * when they persist a schema change.
 *
 * Phase 1: no caller exists yet — the function is shipped now so future
 * handlers can use it without touching SSE wiring. Not exporting yet via
 * any other module means there's no risk of stale refs.
 */
export function emitSchemaChanged(name: string, version: number): void {
  if (!name) return
  const payload = JSON.stringify({ name, version })
  const record = `event: schema-changed\ndata: ${payload}\n\n`
  for (const write of writers) write(record)
}

/**
 * sseSubscriberCount is a test/debug accessor. Not surfaced via any
 * route; the harness uses it to assert connection lifecycle.
 */
export function sseSubscriberCount(): number {
  return writers.size
}

// SPDX-License-Identifier: Apache-2.0
/**
 * installDevCapture — monkey-patch console, error handlers, and the Wails
 * IPC surface so the in-app DevPanel sees console output, uncaught errors,
 * and every backend call. Idempotent; called once from main.tsx.
 */

import { useDevStore } from '../../store/devStore'

let installed = false

function fmt(args: unknown[]): string {
  return args
    .map((a) => {
      if (typeof a === 'string') return a
      if (a instanceof Error) return `${a.name}: ${a.message}`
      try { return JSON.stringify(a) } catch { return String(a) }
    })
    .join(' ')
}

// Truncate big payloads so the buffer stays readable.
function clip(s: string, n = 400): string {
  return s.length > n ? s.slice(0, n) + `… (+${s.length - n} chars)` : s
}

export function installDevCapture(): void {
  if (installed) return
  installed = true
  const push = useDevStore.getState().push

  // ── console.* ──
  const levels = ['log', 'info', 'warn', 'error'] as const
  for (const level of levels) {
    const orig = console[level].bind(console)
    console[level] = (...args: unknown[]) => {
      try { push(level, clip(fmt(args))) } catch { /* never break logging */ }
      orig(...args)
    }
  }

  // ── uncaught errors + promise rejections ──
  window.addEventListener('error', (e) => {
    push('error', `Uncaught: ${e.message} @ ${e.filename}:${e.lineno}:${e.colno}`)
  })
  window.addEventListener('unhandledrejection', (e) => {
    const r = e.reason
    push('error', `Unhandled rejection: ${r instanceof Error ? `${r.name}: ${r.message}` : String(r)}`)
  })

  // ── Wails IPC (window.go.main.App.*) — the backend "network" ──
  // Patched on a short delay because the Wails runtime injects `window.go`
  // after the bundle loads.
  const wrapIPC = () => {
    const app = (window as unknown as {
      go?: { main?: { App?: Record<string, unknown> } }
    }).go?.main?.App
    if (!app) return false
    for (const name of Object.keys(app)) {
      const fn = app[name]
      if (typeof fn !== 'function' || (fn as { __wrapped?: boolean }).__wrapped) continue
      const orig = fn as (...a: unknown[]) => unknown
      const wrapped = (...args: unknown[]) => {
        const argStr = clip(args.map((a) => (typeof a === 'string' ? a : (() => { try { return JSON.stringify(a) } catch { return String(a) } })())).join(', '), 160)
        const out = orig(...args)
        if (out && typeof (out as Promise<unknown>).then === 'function') {
          ;(out as Promise<unknown>).then(
            (res) => push('ipc', `${name}(${argStr}) → ${clip(safeJson(res), 240)}`),
            (err) => push('error', `${name}(${argStr}) ✗ ${err instanceof Error ? err.message : String(err)}`),
          )
        } else {
          push('ipc', `${name}(${argStr}) → (sync)`)
        }
        return out
      }
      ;(wrapped as { __wrapped?: boolean }).__wrapped = true
      app[name] = wrapped
    }
    return true
  }
  if (!wrapIPC()) {
    let tries = 0
    const iv = window.setInterval(() => {
      tries++
      if (wrapIPC() || tries > 50) window.clearInterval(iv)
    }, 100)
  }
}

function safeJson(v: unknown): string {
  if (v === undefined) return 'undefined'
  if (typeof v === 'string') return v
  try { return JSON.stringify(v) } catch { return String(v) }
}

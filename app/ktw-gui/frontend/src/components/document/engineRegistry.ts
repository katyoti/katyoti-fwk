// SPDX-License-Identifier: Apache-2.0
// The display-engine registry — which engine shows a media type
// (modules/display/document-display.md).
//
// React-free on purpose: the resolution rules are tested with `bun test`
// (frontend/tests/), and the component an engine mounts is just a value here.
//
// Resolution picks the most specific pattern that matches:
//   exact type with every pattern parameter equal ▸ exact type ▸ `type/*` ▸ a
//   structured suffix (`*/*+json`, `*/*+xml`) ▸ `*/*`.
// On a tie a built-in engine wins over a contributed one, then the earlier
// registration. No match falls back to the engine registered as the fallback
// (the download card) — never to nothing.

export interface EngineCapabilities {
  view: true
  /** Shows the document while its producer is still writing it. */
  stream?: boolean
  /** Lets the user edit the document. */
  edit?: boolean
  /** The document can be saved to a file. */
  save?: boolean
  /** The formats the document can be exported to — each a target media type
   *  and the label the Export menu shows (PDF, Word, SVG, …). The engine
   *  converts in the browser when it provides `exportDocument`; otherwise the
   *  host asks Go (`DocumentExportRun`, through ktw-docs). */
  export?: ExportFormat[]
}

export interface ExportFormat {
  mediaType: string
  label: string
  /** Only the engine makes this format: when it fails, the host does not
   *  convert instead (its result would be another thing under the same name). */
  engineOnly?: boolean
}

export interface RegisteredEngine<C, D = unknown> {
  id: string
  label: string
  /** Media-type patterns the engine shows. */
  accepts: string[]
  capabilities: EngineCapabilities
  /** Built in (true) or contributed by a sidecar (false). */
  builtin: boolean
  component: C
  /** Converts a document to one of `capabilities.export` in the browser;
   *  without it the host converts the document elsewhere. */
  exportDocument?: (doc: D, mediaType: string, host?: { universe?: string }) => Promise<D>
}

export interface ParsedMediaType {
  type: string
  subtype: string
  params: Record<string, string>
}

/** Parses `type/subtype; k=v` — lower-cased, parameters unquoted. */
export function parseMediaType(mt: string): ParsedMediaType {
  const [head, ...rest] = mt.split(';')
  const [type = '', subtype = ''] = head.trim().toLowerCase().split('/')
  const params: Record<string, string> = {}
  for (const p of rest) {
    const i = p.indexOf('=')
    if (i < 0) continue
    const k = p.slice(0, i).trim().toLowerCase()
    const v = p.slice(i + 1).trim().replace(/^"(.*)"$/, '$1')
    if (k) params[k] = v
  }
  return { type, subtype, params }
}

/** The base type of a media type: `Text/HTML; charset=utf-8` → `text/html`. */
export function baseMediaType(mt: string): string {
  const p = parseMediaType(mt)
  return p.subtype ? `${p.type}/${p.subtype}` : p.type
}

/** How specifically `pattern` matches `mediaType`: -1 for no match, higher is
 *  more specific. */
export function matchScore(pattern: string, mediaType: string): number {
  const p = parseMediaType(pattern)
  const m = parseMediaType(mediaType)
  if (p.type === '*' && p.subtype === '*') return 0
  if (p.type === '*' && p.subtype.startsWith('*+')) {
    return m.subtype.endsWith(p.subtype.slice(1)) ? 1 : -1
  }
  if (p.type !== m.type) return -1
  if (p.subtype === '*') return 2
  if (p.subtype !== m.subtype) return -1
  const keys = Object.keys(p.params)
  for (const k of keys) {
    if (m.params[k] !== p.params[k]) return -1
  }
  return keys.length > 0 ? 4 + keys.length : 3
}

export class EngineRegistry<C, D = unknown> {
  private engines: RegisteredEngine<C, D>[] = []
  private listeners = new Set<() => void>()
  private version = 0

  constructor(private readonly fallbackId: string) {}

  /** Registers an engine; one with the same id is replaced in place. */
  register(engine: RegisteredEngine<C, D>): void {
    const i = this.engines.findIndex((e) => e.id === engine.id)
    if (i >= 0) this.engines[i] = engine
    else this.engines.push(engine)
    this.changed()
  }

  unregister(id: string): void {
    const before = this.engines.length
    this.engines = this.engines.filter((e) => e.id !== id)
    if (this.engines.length !== before) this.changed()
  }

  get(id: string): RegisteredEngine<C, D> | undefined {
    return this.engines.find((e) => e.id === id)
  }

  list(): readonly RegisteredEngine<C, D>[] {
    return this.engines
  }

  /** The engine that shows `mediaType`: the most specific match, else the
   *  fallback. Undefined only when nothing matches and no fallback is
   *  registered. */
  resolve(mediaType: string): RegisteredEngine<C, D> | undefined {
    let best: RegisteredEngine<C, D> | undefined
    let bestScore = -1
    for (const e of this.engines) {
      let score = -1
      for (const pattern of e.accepts) score = Math.max(score, matchScore(pattern, mediaType))
      if (score < 0) continue
      const better =
        score > bestScore || (score === bestScore && best !== undefined && e.builtin && !best.builtin)
      if (better) {
        best = e
        bestScore = score
      }
    }
    return best ?? this.get(this.fallbackId)
  }

  /** Subscribe to registrations (contributed engines arrive after boot). */
  subscribe(fn: () => void): () => void {
    this.listeners.add(fn)
    return () => this.listeners.delete(fn)
  }

  /** Changes on every registration — a useSyncExternalStore snapshot. */
  getVersion(): number {
    return this.version
  }

  private changed(): void {
    this.version++
    for (const fn of this.listeners) fn()
  }
}

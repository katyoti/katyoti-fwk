// SPDX-License-Identifier: Apache-2.0
// postMessage protocol — the wire between ktw-gui (host) and iframe-mounted widgets.
//
// Authoritative spec: modules/display/host-iframe-wire.md
//
// 11 message kinds total, organized by direction. Each carries a `widgetId`
// (for routing when multiple widgets are mounted) plus a `widget` discriminator
// naming which widget surface emitted it (form / view / table / wizard / ...).
//
// All messages are JSON-serializable. Deliberate use of structural typing —
// the host's TS doesn't have to import this file directly; conformance is
// enforced via JSON shape, not nominal types.

// --- shared envelope ---

export interface MessageEnvelope {
  /** Logical surface emitting the message — 'form' / 'view' / 'table' / etc. */
  widget: string
  /** Unique identifier for this mounted widget instance (host-allocated). */
  widgetId: string
}

// --- iframe → host messages ---

export interface ReadyMessage extends MessageEnvelope {
  type: 'ready'
  /** Wall-clock timestamp the iframe finished bootstrapping, ms since epoch. */
  ts: number
}

export interface RequestValueMessage extends MessageEnvelope {
  type: 'request-value'
  /** Caller-supplied identifier (typically a record id) for the host to resolve. */
  valueRef: string | null
}

export interface ChangeMessage extends MessageEnvelope {
  type: 'change'
  value: Record<string, unknown>
  /** True if the value has been edited since initial load. */
  dirty: boolean
  /** True if the value passes the schema's validation rules. */
  valid: boolean
}

export interface SubmitMessage extends MessageEnvelope {
  type: 'submit'
  value: Record<string, unknown>
}

export interface ActionMessage extends MessageEnvelope {
  type: 'action'
  /** The action name (e.g. 'edit', 'delete', 'duplicate', 'export'). */
  action: string
  /** Action-specific arguments (e.g. row id for table actions). */
  args: Record<string, unknown>
}

export interface ResizeMessage extends MessageEnvelope {
  type: 'resize'
  /** New iframe content height in pixels. The host sets the iframe element to match. */
  height: number
}

export interface ErrorMessage extends MessageEnvelope {
  type: 'error'
  error: {
    name: string
    message: string
    stack?: string
  }
}

export type IframeMessage =
  | ReadyMessage
  | RequestValueMessage
  | ChangeMessage
  | SubmitMessage
  | ActionMessage
  | ResizeMessage
  | ErrorMessage

// --- host → iframe messages ---

export interface ThemeMessage {
  type: 'theme'
  theme: 'dark' | 'light'
  /** Optional Mantine-style colors override. */
  colors?: Record<string, string>
  /** Full theme tokens from the host's Mantine theme. When present, the
   *  iframe's MantineProvider mirrors these so the visual style matches
   *  the host. Added 2026-05-05 (display-dsl-framework polish). All
   *  fields optional — partial objects merge with Mantine defaults. */
  tokens?: {
    primaryColor?: string
    fontFamily?: string
    fontFamilyMonospace?: string
    /** Mantine font-size token map: { xs, sm, md, lg, xl } */
    fontSizes?: Record<string, string>
    /** Mantine spacing token map. */
    spacing?: Record<string, string>
    /** Mantine radius token map. */
    radius?: Record<string, string>
    defaultRadius?: string
    /** Heading sizes per `h1`-`h6`. */
    headings?: {
      fontFamily?: string
      sizes?: Record<string, { fontSize?: string; fontWeight?: string; lineHeight?: string }>
    }
  }
}

export interface ValueMessage extends MessageEnvelope {
  type: 'value'
  /** Accepts any JSON value — Record (form/view/wizard), Array (table),
   *  primitive (rare), or null (clear). Harnesses cast as appropriate. */
  value: unknown
}

export interface SchemaChangedMessage {
  type: 'schema-changed'
  /** If set, only this schema's widgets should reload. */
  schemaName?: string
  /** If set, only widgets using this content-type should reload. */
  contentTypeName?: string
}

export interface ReloadMessage {
  type: 'reload'
  /** Diagnostic for the host's logs. */
  reason: string
}

export type HostMessage =
  | ThemeMessage
  | ValueMessage
  | SchemaChangedMessage
  | ReloadMessage

// --- helpers (used by widget handlers in app/ktw-display/src/widgets/) ---

/**
 * Post a message from the iframe to the parent (host).
 * No-op in standalone contexts (when there's no `window.parent`).
 */
export function emitToHost(msg: IframeMessage): void {
  try {
    if (typeof window !== 'undefined' && window.parent && window.parent !== window) {
      window.parent.postMessage(msg, '*')
    }
  } catch {
    // best-effort; never throw out of postMessage
  }
}

/**
 * Subscribe to messages from the host. Returns a cleanup function.
 * Callers should validate the message shape; `event.data` is `unknown`.
 */
export function listenToHost(handler: (msg: HostMessage) => void): () => void {
  if (typeof window === 'undefined') return () => {}
  const wrapped = (event: MessageEvent) => {
    const data = event.data as unknown
    if (!data || typeof data !== 'object') return
    const t = (data as { type?: unknown }).type
    if (typeof t !== 'string') return
    if (
      t === 'theme' ||
      t === 'value' ||
      t === 'schema-changed' ||
      t === 'reload'
    ) {
      handler(data as HostMessage)
    }
  }
  window.addEventListener('message', wrapped)
  return () => window.removeEventListener('message', wrapped)
}

// --- type guards (useful for host-side handlers) ---

export function isIframeMessage(x: unknown): x is IframeMessage {
  if (!x || typeof x !== 'object') return false
  const t = (x as { type?: unknown }).type
  if (typeof t !== 'string') return false
  return (
    t === 'ready' ||
    t === 'request-value' ||
    t === 'change' ||
    t === 'submit' ||
    t === 'action' ||
    t === 'resize' ||
    t === 'error'
  )
}

export function isHostMessage(x: unknown): x is HostMessage {
  if (!x || typeof x !== 'object') return false
  const t = (x as { type?: unknown }).type
  if (typeof t !== 'string') return false
  return t === 'theme' || t === 'value' || t === 'schema-changed' || t === 'reload'
}

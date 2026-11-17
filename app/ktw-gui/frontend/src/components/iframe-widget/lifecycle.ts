// SPDX-License-Identifier: Apache-2.0
// Per-kind postMessage handlers for the iframe-widget host side.
//
// The IframeWidget core (Subtask 4) handles URL resolution + origin/widgetId
// validation. This file (Subtask 5) wires the *behaviour*: turning each
// validated inbound message into the corresponding host-side effect, plus
// the four host → iframe push helpers.
//
// All wire types are imported from app/ktw-display/src/postmessage.ts via
// the @display/* tsconfig path mapping (Subtask 3) — single source of
// truth, no copy/sync.

import type {
  ActionMessage,
  ChangeMessage,
  ErrorMessage,
  HostMessage,
  IframeMessage,
  ReadyMessage,
  RequestValueMessage,
  ResizeMessage,
  SchemaChangedMessage,
  SubmitMessage,
  ThemeMessage,
  ValueMessage,
} from '@display/postmessage'

// ---------------------------------------------------------------------------
// Per-kind callback bag — IframeWidget exposes these as optional props
// ---------------------------------------------------------------------------

export interface LifecycleCallbacks {
  /** Iframe finished bootstrapping. */
  onReady?: (msg: ReadyMessage) => void
  /** Iframe asks for a value (typically by record id). Host should
   *  respond via postValueToIframe. */
  onRequestValue?: (msg: RequestValueMessage) => void
  /** User edited the form/field. */
  onChange?: (msg: ChangeMessage) => void
  /** User submitted a form. */
  onSubmit?: (msg: SubmitMessage) => void
  /** Iframe-emitted action (button click, row action, etc.). */
  onAction?: (msg: ActionMessage) => void
  /** Iframe-reported runtime error. */
  onError?: (msg: ErrorMessage) => void
  /**
   * Iframe-driven height change. The IframeWidget already applies the
   * height to its container — this callback is for parents that want to
   * observe (e.g. for an outer layout that needs the total widget area).
   */
  onResize?: (msg: ResizeMessage) => void
}

// ---------------------------------------------------------------------------
// Inbound dispatcher — fans an IframeMessage out to per-kind callbacks
// ---------------------------------------------------------------------------

/**
 * dispatchIframeMessage routes one validated IframeMessage to the matching
 * callback. The `default` arm is exhaustive in the type checker — adding a
 * new message kind to IframeMessage forces a compile error here, ensuring
 * the host's switch keeps pace with the wire protocol.
 */
export function dispatchIframeMessage(
  msg: IframeMessage,
  cb: LifecycleCallbacks,
): void {
  switch (msg.type) {
    case 'ready':
      cb.onReady?.(msg)
      return
    case 'request-value':
      cb.onRequestValue?.(msg)
      return
    case 'change':
      cb.onChange?.(msg)
      return
    case 'submit':
      cb.onSubmit?.(msg)
      return
    case 'action':
      cb.onAction?.(msg)
      return
    case 'error':
      cb.onError?.(msg)
      return
    case 'resize':
      cb.onResize?.(msg)
      return
    default: {
      // Exhaustiveness check — TS errors here if a new kind is added.
      const _exhaustive: never = msg
      void _exhaustive
    }
  }
}

// ---------------------------------------------------------------------------
// Outbound senders — host → iframe
// ---------------------------------------------------------------------------

/**
 * postToIframe sends a HostMessage to the iframe's contentWindow with
 * the expected origin. Best-effort — if the iframe is gone or origin
 * mismatch (browser will reject), the call is silently swallowed.
 */
function postToIframe(
  iframe: HTMLIFrameElement | null,
  origin: string | null,
  msg: HostMessage,
): void {
  if (!iframe || !iframe.contentWindow || !origin) return
  try {
    iframe.contentWindow.postMessage(msg, origin)
  } catch {
    // Cross-origin throw or iframe-gone — best-effort.
  }
}

/** Push the current theme to the iframe. */
export function postTheme(
  iframe: HTMLIFrameElement | null,
  origin: string | null,
  theme: ThemeMessage['theme'],
  colors?: ThemeMessage['colors'],
  tokens?: ThemeMessage['tokens'],
): void {
  postToIframe(iframe, origin, {
    type: 'theme',
    theme,
    ...(colors ? { colors } : {}),
    ...(tokens ? { tokens } : {}),
  })
}

/** Send a value to the iframe (typically in response to request-value). */
export function postValue(
  iframe: HTMLIFrameElement | null,
  origin: string | null,
  widget: string,
  widgetId: string,
  value: ValueMessage['value'],
): void {
  postToIframe(iframe, origin, {
    type: 'value',
    widget,
    widgetId,
    value,
  })
}

/** Notify the iframe that a schema changed (it should reload itself). */
export function postSchemaChanged(
  iframe: HTMLIFrameElement | null,
  origin: string | null,
  filter?: { schemaName?: string; contentTypeName?: string },
): void {
  const msg: SchemaChangedMessage = { type: 'schema-changed' }
  if (filter?.schemaName) msg.schemaName = filter.schemaName
  if (filter?.contentTypeName) msg.contentTypeName = filter.contentTypeName
  postToIframe(iframe, origin, msg)
}

/** Force the iframe to reload (full URL re-fetch). */
export function postReload(
  iframe: HTMLIFrameElement | null,
  origin: string | null,
  reason: string,
): void {
  postToIframe(iframe, origin, { type: 'reload', reason })
}

// ---------------------------------------------------------------------------
// Theme resolution — collapses 'system' to a concrete dark|light per spec
// ---------------------------------------------------------------------------

/**
 * resolveTheme converts the GUI's tri-state theme setting (`'light' |
 * 'dark' | 'system'`) into the binary `'dark' | 'light'` the wire protocol
 * accepts (postmessage.ts `ThemeMessage.theme`). Uses matchMedia for the
 * 'system' case.
 *
 * Stable across SSR — when `window` is undefined, defaults to 'light'.
 */
export function resolveTheme(
  themeMode: 'light' | 'dark' | 'system',
): ThemeMessage['theme'] {
  if (themeMode === 'light' || themeMode === 'dark') return themeMode
  if (typeof window === 'undefined' || !window.matchMedia) return 'light'
  return window.matchMedia('(prefers-color-scheme: dark)').matches
    ? 'dark'
    : 'light'
}

/**
 * Clamp a resize-reported height to a sane range. The iframe controls
 * its height via `resize` messages; the host applies the value with this
 * bound to prevent runaway widgets from monopolising the viewport.
 */
export function clampHeight(
  reported: number,
  opts: { min?: number; max?: number } = {},
): number {
  const min = opts.min ?? 0
  const max = opts.max ?? 800
  if (!Number.isFinite(reported) || reported < min) return min
  if (reported > max) return max
  return Math.round(reported)
}

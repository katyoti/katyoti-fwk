// SPDX-License-Identifier: Apache-2.0
// postMessage helpers — the iframe ↔ host contract.
//
// The harness emits 3 events on the parent window:
//   - 'ready'  on mount
//   - 'change' on every field value change
//   - 'submit' on submit-button click
//
// All events carry { widget, widgetId, type, ts? }. ktw-gui's iframe holder
// listens for these and forwards into the host's event log.

export type WidgetMessage =
  | { type: 'ready'; ts: number }
  | { type: 'change'; value: unknown; dirty?: boolean; valid?: boolean }
  | { type: 'submit'; value: unknown }
  | { type: 'action'; action: string; args: Record<string, unknown> }

export interface EmitContext {
  widget: string
  widgetId: string
}

/** Build a memoized emit function for the given widget context. */
export function makeEmit(ctx: EmitContext): (msg: WidgetMessage) => void {
  return (msg: WidgetMessage) => {
    try {
      window.parent.postMessage({ widget: ctx.widget, widgetId: ctx.widgetId, ...msg }, '*')
    } catch {
      // Cross-origin / detached frame — silently swallow.
    }
  }
}

/** Resolve the widgetId from the iframe URL `?widgetId=...`, falling back to widget name. */
export function resolveWidgetId(fallback: string): string {
  try {
    return new URLSearchParams(window.location.search).get('widgetId') || fallback
  } catch {
    return fallback
  }
}

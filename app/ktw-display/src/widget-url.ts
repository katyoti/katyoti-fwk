// SPDX-License-Identifier: Apache-2.0
// /api/widget-url handler — composes the iframe URL for a given widget
// kind + schema/value/mode parameters, with the per-session token already
// embedded as a query param (bun's auth gate for /widget/* expects it
// there, not in a header — iframes can't set custom headers).
//
// Pure function so it can be unit-tested without booting the server.
//
// Wire shape mirrors the Go-side bridge_api.GetWidgetURLInput +
// WidgetURLOutput in internal/extensions/displaybridge/api/service.go.
//
// Phase 1 (this subtask of gui-schema-runtime): no expiration. The bridge
// already rotates tokens per spawn (publish.ExternalProcess), so the
// widget URL is implicitly invalidated when bun restarts.

export interface GetWidgetURLInput {
  kind: string
  schemaName?: string
  value?: string
  mode?: string
}

export interface WidgetURLOutput {
  URL: string
  Token: string
  ExpiresAt: number
}

export class InvalidWidgetURLInputError extends Error {
  readonly status: number
  readonly code: string
  constructor(code: string, message: string, status = 400) {
    super(message)
    this.name = 'InvalidWidgetURLInputError'
    this.code = code
    this.status = status
  }
}

/**
 * buildWidgetURL composes the iframe URL.
 *
 * @param input  parsed body matching GetWidgetURLInput
 * @param port   server port (server.port from bun's serve())
 * @param token  per-session token (KTW_DISPLAY_TOKEN); empty = dev mode (no auth)
 * @param hostname loopback hostname (default "127.0.0.1"; configurable for tests)
 *
 * @throws InvalidWidgetURLInputError when `kind` is missing or contains
 *         characters that would break URL path segmentation.
 */
export function buildWidgetURL(
  input: GetWidgetURLInput,
  port: number,
  token: string,
  hostname = '127.0.0.1',
): WidgetURLOutput {
  if (!input || typeof input.kind !== 'string' || input.kind.trim() === '') {
    throw new InvalidWidgetURLInputError('kind_required', 'kind is required')
  }
  // The kind becomes a URL path segment — reject any character that would
  // break segmentation or encode ambiguously. Bun routes /widget/<kind> as
  // an exact match, so '/' or '?' or '#' would never reach a handler.
  if (/[/?#\s]/.test(input.kind)) {
    throw new InvalidWidgetURLInputError(
      'kind_invalid',
      `kind must not contain '/', '?', '#', or whitespace; got: ${JSON.stringify(input.kind)}`,
    )
  }

  const params = new URLSearchParams()
  if (input.schemaName) params.set('schemaName', input.schemaName)
  if (input.value) params.set('value', input.value)
  if (input.mode) params.set('mode', input.mode)
  if (token) params.set('token', token)

  const qs = params.toString()
  const url = `http://${hostname}:${port}/widget/${encodeURIComponent(input.kind)}${qs ? `?${qs}` : ''}`

  return {
    URL: url,
    Token: token,
    ExpiresAt: 0, // no expiration in Phase 1 — token rotates with bun restart
  }
}

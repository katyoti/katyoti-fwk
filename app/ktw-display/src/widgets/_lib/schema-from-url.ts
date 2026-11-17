// SPDX-License-Identifier: Apache-2.0
// schemaFromURL — read an inline `?schema=<base64>` query param from a
// widget request URL. When the LLM-streaming-protocol artifact mounts an
// iframe, the artifact's schema is base64-encoded into the URL so the
// bun route can render it without a server-side schema registry lookup.
//
// Returns undefined when the param is absent or undecodable; routes
// fall back to their hardcoded demo schemas in that case.
//
// Wired by llm-streaming-protocol (v1 schema-render bridge, ms06,
// 2026-05-04). Companion: host-side typed wrappers (UIForm / UIView /
// UITable / UIWizard) base64-encode `schemaInline` + forward via
// IframeWidget's `extraParams`.
//
// **UTF-8 handling**: the host encodes via the classic
// `btoa(unescape(encodeURIComponent(json)))` trick, which produces a
// base64 string whose decoded bytes ARE the UTF-8 octets of the JSON.
// Decoding here MUST reverse that: `atob()` returns a Latin-1 string of
// raw bytes (each char's charCode is the byte value); we then read those
// bytes as UTF-8 via TextDecoder. Without this round-trip, accented
// characters appear as mojibake (`é` → `é`, `'` → `â\x80\x99`).

/** Decode a base64url string into a UTF-8 string, then JSON-parse. */
function decodeBase64URL(encoded: string): unknown {
  const normalized = encoded.replace(/-/g, '+').replace(/_/g, '/')
  const padded =
    normalized.length % 4 === 0
      ? normalized
      : normalized + '='.repeat(4 - (normalized.length % 4))
  const binary = atob(padded)
  // atob returns a Latin-1 byte-string. Convert to UTF-8 properly:
  // walk each char, take its byte value, then run through TextDecoder.
  const bytes = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i++) {
    bytes[i] = binary.charCodeAt(i)
  }
  const json = new TextDecoder('utf-8').decode(bytes)
  return JSON.parse(json)
}

/**
 * Decode a `?schema=<base64>` query param into a parsed JSON object.
 * Returns undefined if the param is missing or malformed.
 */
export function schemaFromURL(req: Request): unknown {
  try {
    const url = new URL(req.url)
    const encoded = url.searchParams.get('schema')
    if (!encoded) return undefined
    return decodeBase64URL(encoded)
  } catch {
    return undefined
  }
}

/**
 * Same as schemaFromURL but for the value (`?value=<base64>`). Used
 * when an artifact's initial value is non-trivial enough that the host
 * wants to inject it via URL rather than a postMessage push.
 */
export function valueFromURL(req: Request): unknown {
  try {
    const url = new URL(req.url)
    const encoded = url.searchParams.get('value')
    if (!encoded) return undefined
    return decodeBase64URL(encoded)
  } catch {
    return undefined
  }
}

/**
 * Decode a `?page=<base64>` query param into a parsed JSON DSL tree
 * (a `UIPageNode`). Used by the `/widget/page` route — the host emits
 * the DSL tree from a `display-ui-component` tool call, base64url-encodes
 * it, and forwards via the iframe URL. Returns undefined when missing
 * or malformed.
 *
 * Wired by S5 of display-dsl-framework (2026-05-05).
 */
export function pageFromURL(req: Request): unknown {
  try {
    const url = new URL(req.url)
    const encoded = url.searchParams.get('page')
    if (!encoded) return undefined
    return decodeBase64URL(encoded)
  } catch {
    return undefined
  }
}

/**
 * Decode a `?data=<base64>` query param into the parsed outer data tree
 * for `dataField: "$.path"` JSONPath bindings inside the DSL tree.
 * Used by the `/widget/page` route alongside `?page=`. Returns undefined
 * when missing or malformed.
 *
 * Wired by display-package-restructure Phase 3 (2026-05-06).
 */
export function dataFromURL(req: Request): unknown {
  try {
    const url = new URL(req.url)
    const encoded = url.searchParams.get('data')
    if (!encoded) return undefined
    return decodeBase64URL(encoded)
  } catch {
    return undefined
  }
}

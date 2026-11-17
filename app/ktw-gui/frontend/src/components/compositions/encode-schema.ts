// SPDX-License-Identifier: Apache-2.0
// encode-schema — base64url-encode a JSON object for the LLM-streaming-
// protocol's schema-via-URL injection path. Used by typed wrappers
// (UIForm / UIView / UITable / UIWizard) to forward an inline schema /
// value through IframeWidget's extraParams. Counterpart on the bun
// side: src/widgets/_lib/schema-from-url.ts.
//
// base64url variant (-/_/no-pad) keeps the URL clean.

export function encodeSchemaParam(obj: unknown): string {
  if (obj === undefined || obj === null) return ''
  const json = JSON.stringify(obj)
  // btoa requires a binary string; modern browsers handle UTF-8 via
  // unescape(encodeURIComponent(...)) — the standard workaround for
  // btoa's ASCII limitation.
  const utf8 = unescape(encodeURIComponent(json))
  const b64 = btoa(utf8)
  // base64url: replace +/ with -_, strip padding.
  return b64.replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

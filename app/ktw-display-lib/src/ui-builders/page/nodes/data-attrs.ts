// SPDX-License-Identifier: Apache-2.0
// dataAttrs: a page node's own data-* keys, for its renderer's root element.
//
// A producer marks a node for a probe or a host (`data-view`, `data-title`,
// `data-held`, …); every renderer spreads them FIRST on its root DOM element,
// so the attributes the renderer writes itself win on a clash. No wrapper
// element is ever added for them.

/** dataAttrs keeps the keys of `node` that start with `data-`, each value as
 *  a string (an object as its JSON); a null or undefined value is left out. */
export function dataAttrs(node: unknown): Record<string, string> {
  const out: Record<string, string> = {}
  if (!node || typeof node !== 'object') return out
  for (const [k, v] of Object.entries(node as Record<string, unknown>)) {
    if (!k.startsWith('data-') || v === undefined || v === null) continue
    out[k] = typeof v === 'string' ? v : typeof v === 'object' ? JSON.stringify(v) : String(v)
  }
  return out
}

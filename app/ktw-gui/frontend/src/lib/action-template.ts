// SPDX-License-Identifier: Apache-2.0
/**
 * action-template — mustache-lite substitution for Mode-B declarative
 * action shortcuts (US-S3 crud action wiring).
 *
 * Grammar (intentionally NOT Turing-complete — dotted path lookup
 * only, no logic / sections / helpers):
 *
 *   "{{item}}"            → the raw value at scope.item (type preserved)
 *   "{{item.id}}"         → the raw value at scope.item.id (type preserved)
 *   "Book {{item.title}}" → string interpolation (missing paths → "")
 *
 * Substitution walks arrays and plain objects recursively; any other
 * value passes through untouched. A full-string placeholder returns
 * the looked-up value AS-IS so objects/numbers/booleans survive (e.g.
 * `data: "{{item}}"` injects the whole clicked row).
 */

const FULL_PLACEHOLDER = /^\{\{\s*([\w.-]+)\s*\}\}$/
const EMBEDDED_PLACEHOLDER = /\{\{\s*([\w.-]+)\s*\}\}/g

/** Dotted-path lookup into the scope. Returns undefined on any miss. */
function lookupPath(scope: Record<string, unknown>, path: string): unknown {
  let cur: unknown = scope
  for (const seg of path.split('.')) {
    if (cur === null || cur === undefined || typeof cur !== 'object') return undefined
    cur = (cur as Record<string, unknown>)[seg]
  }
  return cur
}

/**
 * Substitute {{path}} placeholders in `template` against `scope`.
 * Returns a NEW structure; the template is never mutated.
 */
export function substituteTemplate(template: unknown, scope: Record<string, unknown>): unknown {
  if (typeof template === 'string') {
    const full = template.match(FULL_PLACEHOLDER)
    if (full) {
      // Whole-string placeholder → raw value, type preserved.
      return lookupPath(scope, full[1])
    }
    return template.replace(EMBEDDED_PLACEHOLDER, (_, path: string) => {
      const v = lookupPath(scope, path)
      return v === undefined || v === null ? '' : String(v)
    })
  }
  if (Array.isArray(template)) {
    return template.map((el) => substituteTemplate(el, scope))
  }
  if (template !== null && typeof template === 'object') {
    const out: Record<string, unknown> = {}
    for (const [k, v] of Object.entries(template as Record<string, unknown>)) {
      out[k] = substituteTemplate(v, scope)
    }
    return out
  }
  return template
}

/** One declared Mode-B shortcut (mirrors the Go ActionShortcut). */
export interface UIActionShortcut {
  tool: string
  argsTemplate?: Record<string, unknown>
}

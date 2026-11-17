// SPDX-License-Identifier: Apache-2.0
/**
 * Apply a JSON Merge Patch (RFC 7396) to a target - the one implementation,
 * read by a page op's `merge` (ui-builders/page/answer.ts).
 *
 * - patch is an object: each key in patch is applied to target.
 *   - patch[key] === null   → delete target[key]
 *   - patch[key] is object  → recursive merge
 *   - else                  → replace target[key]
 * - patch is non-object: replace target wholesale (per RFC).
 *
 * Pure: returns a new value; does not mutate target.
 */
export function mergePatch(target: unknown, patch: unknown): unknown {
  if (patch === null || typeof patch !== 'object' || Array.isArray(patch)) {
    return patch
  }
  const t =
    target && typeof target === 'object' && !Array.isArray(target)
      ? { ...(target as Record<string, unknown>) }
      : ({} as Record<string, unknown>)

  for (const [k, v] of Object.entries(patch as Record<string, unknown>)) {
    if (v === null) {
      delete t[k]
    } else if (typeof v === 'object' && !Array.isArray(v)) {
      t[k] = mergePatch(t[k], v)
    } else {
      t[k] = v
    }
  }
  return t
}

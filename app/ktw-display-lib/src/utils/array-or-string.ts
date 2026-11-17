// SPDX-License-Identifier: Apache-2.0
// Convert a string-or-string-array input to a normalized string array.
// Ported from _ktw_workspace/old_ui_main/.../utils/array-or-string.ts.
//
// - input is array: returned as-is (after optional trim/filter)
// - input is string + `split` given: returned as the split result
// - input is string + no split: returned as a single-element array
// - undefined / empty: returns []

export type ArrayOrString = string | string[]

export function arrayOrString(
  path?: ArrayOrString,
  split?: string | RegExp,
  filter?: boolean,
  trim?: boolean,
): string[] {
  if (!path) return []
  let result: string[] = []
  if (typeof path === 'string') {
    result = split ? path.split(split) : [path]
  } else {
    result = Array.isArray(path) ? path : []
  }
  if (trim) result = result.map((s) => s.trim())
  if (filter) result = result.filter(Boolean)
  return result
}

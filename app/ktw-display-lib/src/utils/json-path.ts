// SPDX-License-Identifier: Apache-2.0
// Minimal JSONPath resolver for the DSL data-binding mechanism.
//
// Supports the subset needed by `dataField: "$.path"` references in
// the DSL (display-package-restructure Phase 3, 2026-05-06):
//
//   $.field             — object property
//   $.field.subfield    — nested property
//   $.field[3]          — array index
//   $.field[*]          — array wildcard (returns flat array)
//   $.field[*].sub      — wildcard + downstream property (returns array)
//
// NOT supported (deferred to a future phase):
//   $.field[?(@.x==1)]  — filter expressions
//   $..deep             — recursive descent
//   $['quoted-key']     — bracket notation for keys
//
// Hand-rolled (~120 LOC) instead of pulling in `jsonpath` (8-10kB gzip)
// or `jsonpath-plus` (5-7kB) — bundle-neutral, sufficient for v1.
//
// Per the host-driven data flow decision (display-package-restructure
// Phase 3 ANALYZE D4): resolver runs INSIDE the iframe at render time,
// pure (no side effects), no network access, no tool dispatch.

/**
 * Resolve a JSONPath against a data object. Returns the value at the
 * path, an array of values for wildcard paths, or `undefined` when the
 * path doesn't resolve.
 *
 * @example
 * resolveJSONPath({ a: { b: 1 } }, '$.a.b')          // → 1
 * resolveJSONPath({ a: [10, 20, 30] }, '$.a[1]')     // → 20
 * resolveJSONPath({ a: [{ x: 1 }, { x: 2 }] }, '$.a[*].x')  // → [1, 2]
 * resolveJSONPath({ a: 1 }, '$.b')                   // → undefined
 *
 * Caller responsibility: expect array shapes when the path contains a
 * wildcard. Use `Array.isArray(result)` to discriminate.
 */
export function resolveJSONPath(data: unknown, path: string): unknown {
  // Strip leading `$.` or `$` — both are valid roots
  let p = path.trim()
  if (p === '$' || p === '') return data
  if (p.startsWith('$.')) p = p.slice(2)
  else if (p.startsWith('$')) p = p.slice(1)
  else if (p.startsWith('.')) p = p.slice(1)

  if (!p) return data

  return walk(data, parseSegments(p))
}

// =================================================================================================
// Segment parser — splits a path into typed segments
// =================================================================================================

type Segment =
  | { kind: 'prop'; name: string }
  | { kind: 'index'; index: number }
  | { kind: 'wildcard' }

/** Parse `field.subfield[3].x[*].y` into [prop:'field', prop:'subfield',
 *  index:3, prop:'x', wildcard, prop:'y']. */
function parseSegments(path: string): Segment[] {
  const out: Segment[] = []
  let i = 0
  while (i < path.length) {
    if (path[i] === '.') {
      i++
      continue
    }
    if (path[i] === '[') {
      // Bracket: [N] or [*]
      const end = path.indexOf(']', i)
      if (end === -1) throw new Error(`unclosed bracket in path: ${path}`)
      const inner = path.slice(i + 1, end).trim()
      if (inner === '*') {
        out.push({ kind: 'wildcard' })
      } else {
        const idx = Number(inner)
        if (!Number.isInteger(idx) || idx < 0) {
          throw new Error(`invalid bracket index "${inner}" in path: ${path}`)
        }
        out.push({ kind: 'index', index: idx })
      }
      i = end + 1
      continue
    }
    // Property name — read until . or [
    let j = i
    while (j < path.length && path[j] !== '.' && path[j] !== '[') j++
    const name = path.slice(i, j)
    if (name) out.push({ kind: 'prop', name })
    i = j
  }
  return out
}

// =================================================================================================
// Walker — applies segments to data
// =================================================================================================

function walk(data: unknown, segments: Segment[]): unknown {
  let current: unknown = data
  let inWildcard = false

  for (const seg of segments) {
    if (current === undefined || current === null) return undefined

    if (seg.kind === 'prop') {
      if (inWildcard) {
        // Map property access over the array
        if (!Array.isArray(current)) return undefined
        current = current.map((item) =>
          item && typeof item === 'object'
            ? (item as Record<string, unknown>)[seg.name]
            : undefined,
        )
      } else {
        if (typeof current !== 'object') return undefined
        current = (current as Record<string, unknown>)[seg.name]
      }
    } else if (seg.kind === 'index') {
      if (inWildcard) {
        // Index into each item of the wildcard result
        if (!Array.isArray(current)) return undefined
        current = current.map((item) =>
          Array.isArray(item) ? item[seg.index] : undefined,
        )
      } else {
        if (!Array.isArray(current)) return undefined
        current = current[seg.index]
      }
    } else if (seg.kind === 'wildcard') {
      if (!Array.isArray(current)) return undefined
      inWildcard = true
      // current stays the array; subsequent segments fan out
    }
  }

  return current
}

// =================================================================================================
// Writer — the inverse of resolveJSONPath for a single location
// =================================================================================================

/**
 * Return a copy of `data` with `value` at `path`; `data` is left untouched
 * (objects and arrays along the path are copied, the rest is shared).
 * Missing containers are created — an object for a property, an array for an
 * index. A wildcard names many locations, so it cannot be written: throws.
 *
 * @example
 * setJSONPath({ a: { b: 1 } }, '$.a.b', 2)   // → { a: { b: 2 } }
 * setJSONPath({ a: [1, 2] }, '$.a[1]', 5)    // → { a: [1, 5] }
 */
export function setJSONPath(data: unknown, path: string, value: unknown): unknown {
  let p = path.trim()
  if (p.startsWith('$.')) p = p.slice(2)
  else if (p.startsWith('$')) p = p.slice(1)
  else if (p.startsWith('.')) p = p.slice(1)
  const segments = p ? parseSegments(p) : []
  if (segments.some((s) => s.kind === 'wildcard')) {
    throw new Error(`cannot write through a wildcard: ${path}`)
  }
  return put(data, segments, 0, value)
}

function put(current: unknown, segments: Segment[], i: number, value: unknown): unknown {
  if (i === segments.length) return value
  const seg = segments[i]!
  if (seg.kind === 'index') {
    const arr = Array.isArray(current) ? [...current] : []
    arr[seg.index] = put(arr[seg.index], segments, i + 1, value)
    return arr
  }
  const obj: Record<string, unknown> =
    current && typeof current === 'object' && !Array.isArray(current)
      ? { ...(current as Record<string, unknown>) }
      : {}
  const name = (seg as { name: string }).name
  obj[name] = put(obj[name], segments, i + 1, value)
  return obj
}

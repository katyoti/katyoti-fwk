// SPDX-License-Identifier: Apache-2.0
// Helpers to make schema-AST-driven forms/views readable when the schema
// declares no field labels (e.g. an LLM-authored shorthand schema where a
// field is just `destination_type: "string@enum(...)"`). We derive a
// human label from the field name so the UI shows "Destination Type"
// instead of the raw `destination_type`. Schemas that DO declare labels
// (the dossier data-type, node types) are left untouched.

/** snake_case / camelCase / kebab-case → Title Case. */
export function humanizeFieldName(name: string): string {
  return name
    .replace(/[_-]+/g, ' ')
    .replace(/([a-z0-9])([A-Z])/g, '$1 $2')
    .trim()
    .replace(/\b\w/g, (c) => c.toUpperCase())
}

/** Return a deep clone of the schema with a derived `label` on every
 *  object field that lacks one. Non-mutating (clones via JSON so the
 *  shared store schema is never touched). */
export function withFieldLabels<T = any>(schema: T): T {
  const s = schema as any
  if (!s || typeof s !== 'object' || !s.types) return schema
  const clone = JSON.parse(JSON.stringify(s))
  for (const t of Object.values(clone.types) as any[]) {
    if (t && typeof t === 'object' && t.fields && typeof t.fields === 'object') {
      for (const [name, f] of Object.entries(t.fields)) {
        if (f && typeof f === 'object' && !(f as any).label) {
          ;(f as any).label = humanizeFieldName(name)
        }
      }
    }
  }
  return clone
}

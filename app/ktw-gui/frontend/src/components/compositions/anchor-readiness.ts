// SPDX-License-Identifier: Apache-2.0
// anchor-readiness — heuristics for the streaming-aware URL anchor in
// UIForm / UIView / UITable / UIWizard / UIPage. The "anchor once" pattern
// captures the first non-undefined schemaInline/dslTree, base64url-encodes
// it into the iframe URL, and locks. The capture has to wait until the
// streamed value is "ready" — i.e. meaningful enough to actually render —
// otherwise the partial-JSON parser's early auto-closed states (`{}`,
// `{name: "Topic"}`) freeze the URL on a useless skeleton and the iframe
// renders blank for the rest of the artifact's life.
//
// Per dsl-id-based-patching follow-up + display-demo Shape-1 form-never-
// displays bug (2026-05-18). True progressive streaming (schema pushed via
// postMessage to the bun harness) is the long-term fix; this is the
// short-term heuristic that keeps streamed artifacts renderable.

/**
 * Is a streamed schemaInline ready enough to anchor in the iframe URL?
 *
 * A schema is "ready" when:
 *   - it's an object, AND
 *   - it carries `types` (an object), AND
 *   - `types[root]` (or the first type when root isn't set yet) has at
 *     least one entry in its `fields` object
 *
 * That filters out the partial-JSON-parser's intermediate states
 * (`{}`, `{name: "X"}`, `{name, root}`, `{name, root, types: {}}`,
 * `{name, root, types: {Foo: {}}}`) which would render blank.
 */
export function isSchemaReady(schema: unknown): boolean {
  if (schema === null || typeof schema !== 'object') return false
  const s = schema as Record<string, unknown>
  const types = s.types
  if (types === null || typeof types !== 'object') return false
  const root = typeof s.root === 'string' ? s.root : undefined
  const typesObj = types as Record<string, unknown>
  const keys = Object.keys(typesObj)
  if (keys.length === 0) return false
  const targetKey = root && keys.includes(root) ? root : keys[0]
  return isTypeReady(typesObj[targetKey], typesObj, 0)
}

/**
 * Is a single type node ready to render?
 *   - object: has ≥1 entry in `fields`
 *   - array (table root): `items` resolves to a ready object type
 *     (named ref into `types`, or an inline type node)
 * Recursion is depth-capped to guard against cyclic `items` refs.
 */
function isTypeReady(
  t: unknown,
  typesObj: Record<string, unknown>,
  depth: number,
): boolean {
  if (depth > 4 || !t || typeof t !== 'object') return false
  const node = t as Record<string, unknown>
  const fields = node.fields
  if (fields && typeof fields === 'object') {
    return Object.keys(fields as Record<string, unknown>).length > 0
  }
  // Array shape — DSL arrays carry `items` (string ref or inline type),
  // never `fields`. The table root (BookList) is exactly this case.
  const items = node.items
  if (typeof items === 'string') {
    return isTypeReady(typesObj[items], typesObj, depth + 1)
  }
  if (items && typeof items === 'object') {
    return isTypeReady(items, typesObj, depth + 1)
  }
  return false
}

/**
 * Is a streamed page dslTree ready enough to anchor?
 *
 * Ready when the tree is an object with a non-empty `kind` discriminator.
 * For container nodes (vbox/hbox/grid/card/section/tabs/accordion) we
 * also require the children/tabs/panels array to be non-empty — an empty
 * vbox renders nothing and there's no point in anchoring on it.
 */
export function isDslTreeReady(tree: unknown): boolean {
  if (tree === null || typeof tree !== 'object') return false
  const t = tree as Record<string, unknown>
  const kind = t.kind
  if (typeof kind !== 'string' || kind.length === 0) return false

  const containerKinds = new Set([
    'vbox', 'hbox', 'grid', 'box', 'card', 'section', 'tabs', 'accordion',
  ])
  if (!containerKinds.has(kind)) return true // leaf node — kind is enough

  if (kind === 'tabs' || kind === 'accordion') {
    const arr = (t.tabs ?? t.panels) as unknown[] | undefined
    return Array.isArray(arr) && arr.length > 0
  }
  const children = t.children as unknown[] | undefined
  return Array.isArray(children) && children.length > 0
}

/**
 * Is a value object ready? Far looser than schema/dslTree — values can be
 * partial during streaming and still render usefully (the form just shows
 * whatever fields have arrived). Anchor on any non-undefined value; empty
 * object is fine.
 */
export function isValueReady(value: unknown): boolean {
  return value !== undefined && value !== null
}

// SPDX-License-Identifier: Apache-2.0
// The GUI's specification cache: every part an
// answer names by `{uri, hash}` - the schema a component of a kind and a
// variant draws - is kept once per universe, keyed by the component's kind,
// the part's URI and the variant, with the hash it was answered with. A part
// already held at that hash costs nothing: no call, only the answer's data
// crossed the wire. A miss asks object-pane's ResolvePart once, giving the
// hash held, if any, so a current one comes back as the hash alone. A part
// whose fields compute their choices (not cacheable) is never kept.

import { dispatcherFor } from '../ktsp'
import { SCHEMA_BOUND_KINDS, isNamedSpec } from '@ktw/display-lib/ui-builders/page/index.ts'

// A panel's page over its template is the display library's (panel.ts).
export { templatePage } from '@ktw/display-lib/ui-builders/page/index.ts'

/** The ResolvePart method of the object pane. */
export const RESOLVE_PART = 'ktw-agents:object-pane/resolve-part'

/** What names one part: the component kind that draws it, its schema URI,
 *  its ui variant. */
export interface PartRef {
  kind: string
  schema: string
  variant?: string
}

/** One part held: the hash it was answered with and its AST. */
export interface HeldPart {
  hash: string
  part: Record<string, unknown>
}

/** What ResolvePart answers. */
export interface PartAnswer {
  part?: Record<string, unknown>
  hash: string
  cacheable: boolean
}

/** Asks ResolvePart: {kind, schema, variant, hash} in. */
export type PartFetch = (input: { kind: string; schema: string; variant?: string; hash?: string }) => Promise<PartAnswer>

/** partKey is a part's place in the cache. */
export function partKey(r: PartRef): string {
  return `${r.kind}|${r.schema}|${r.variant ?? ''}`
}

/** A universe's held parts. */
export type SpecCache = Map<string, HeldPart>

const caches = new Map<string, SpecCache>()

/** specCacheOf is the one cache of a universe. */
export function specCacheOf(universe: string): SpecCache {
  let c = caches.get(universe)
  if (!c) caches.set(universe, (c = new Map()))
  return c
}

/** partFor is the part a component draws, from the cache when it holds the
 *  answer's hash, else asked once (with the hash held) and kept when it may
 *  be. */
export async function partFor(cache: SpecCache, ref: PartRef, hash: string | undefined, fetch: PartFetch): Promise<Record<string, unknown> | undefined> {
  const key = partKey(ref)
  const held = cache.get(key)
  if (held && hash && held.hash === hash) return held.part
  const got = await fetch({ kind: ref.kind, schema: ref.schema, ...(ref.variant ? { variant: ref.variant } : {}), ...(held ? { hash: held.hash } : {}) })
  const part = got.part ?? (held && got.hash === held.hash ? held.part : undefined)
  if (part && got.cacheable) cache.set(key, { hash: got.hash, part })
  else if (!got.cacheable) cache.delete(key)
  return part
}

const BOUND = new Set<string>(SCHEMA_BOUND_KINDS as readonly string[])

/** Localize gives a part in the reader's language (its data type's texts
 *  translated, ctSchema): the part's URI and the part as cached in; a new part
 *  out, the one given unchanged. */
export type Localize = (uri: string, part: Record<string, unknown>) => Record<string, unknown>

/** inlineParts is the tree with every schema-bound node whose `schema` names
 *  a part of `parts` by `{uri, hash}` given that part inline, from the cache
 *  or asked once - through `localize` when given, so it reads in the person's
 *  language while the cache keeps the part as answered (a language switch
 *  re-inlines from the cache, no call). A node laying its own `ui` block over
 *  its schema is left as it is (the object pane lays it). The tree given is
 *  not changed. */
export async function inlineParts(tree: unknown, parts: Record<string, unknown> | undefined, cache: SpecCache, fetch: PartFetch, localize?: Localize): Promise<unknown> {
  if (!parts) return tree
  const walk = async (v: unknown): Promise<unknown> => {
    if (Array.isArray(v)) return Promise.all(v.map(walk))
    if (!v || typeof v !== 'object') return v
    const n = v as Record<string, unknown>
    const out: Record<string, unknown> = {}
    for (const [k, c] of Object.entries(n)) out[k] = k === 'schema' || k === 'ui' || k === 'data' ? c : await walk(c)
    const kind = typeof n.kind === 'string' ? n.kind : ''
    const spec = typeof n.schema === 'string' ? parts[n.schema] : undefined
    if (BOUND.has(kind) && isNamedSpec(spec) && !('ui' in n)) {
      const part = await partFor(cache, { kind, schema: spec.uri, variant: typeof n.variant === 'string' ? n.variant : undefined }, spec.hash, fetch)
      if (part) out.schema = localize ? localize(spec.uri, part) : part
    }
    return out
  }
  return walk(tree)
}

/** localizeResolved is a tree the object pane resolved (ResolvePanel) read
 *  in the person's language: every node whose `schema` named a data-type by
 *  its URI in the tree asked, and is inline in the resolved one at the same
 *  place, gets that schema through `localize` (its field labels, groups and
 *  actions translated) - as inlineParts does for a part. The resolution
 *  inlines in place, so a node keeps its place. The trees given are not
 *  changed. */
export function localizeResolved(asked: unknown, resolved: unknown, localize: Localize): unknown {
  const walk = (a: unknown, r: unknown): unknown => {
    if (Array.isArray(r)) return Array.isArray(a) ? r.map((c, i) => walk(a[i], c)) : r
    if (!r || typeof r !== 'object' || !a || typeof a !== 'object' || Array.isArray(a)) return r
    const an = a as Record<string, unknown>
    const rn = r as Record<string, unknown>
    const out: Record<string, unknown> = {}
    for (const [k, c] of Object.entries(rn)) out[k] = k === 'schema' || k === 'ui' || k === 'data' ? c : walk(an[k], c)
    if (typeof an.schema === 'string' && rn.schema && typeof rn.schema === 'object' && !Array.isArray(rn.schema)) {
      out.schema = localize(an.schema, rn.schema as Record<string, unknown>)
    }
    return out
  }
  return walk(asked, resolved)
}

/** needsResolve says whether a tree still needs the object pane: a
 *  schema-bound node whose schema is a reference (a URI or an action's input),
 *  that lays a `ui` block, or whose inline schema computes choices; a list
 *  naming its elements' data type (`itemType`) by URI. The
 *  whole tree is walked - the node each element of a list holds in its
 *  `nodeField` included - never a node's own `schema`, `itemType`, `ui` or
 *  `data`. */
export function needsResolve(tree: unknown): boolean {
  if (Array.isArray(tree)) return tree.some(needsResolve)
  if (!tree || typeof tree !== 'object') return false
  const n = tree as Record<string, unknown>
  if (typeof n.kind === 'string' && BOUND.has(n.kind)) {
    const s = n.schema
    if (typeof s === 'string' || (s && typeof s === 'object' && 'action' in s) || 'ui' in n) return true
    if (s && typeof s === 'object' && JSON.stringify(s).includes('"options_from"')) return true
  }
  if (typeof n.itemType === 'string' && n.itemType !== '') return true
  const field = n.nodeField
  if (typeof field === 'string' && field !== '' && Array.isArray(n.data) &&
    n.data.some(el => !!el && typeof el === 'object' && needsResolve((el as Record<string, unknown>)[field]))) return true
  return Object.entries(n).some(([k, c]) => k !== 'schema' && k !== 'itemType' && k !== 'ui' && k !== 'data' && needsResolve(c))
}

// A tree an answer names by `{uri, hash}`: a page
// template - a panel open's tree, a page source's or a section's - is kept in
// the same cache by its URI, with the hash it was answered with. A template
// held at the answer's hash costs no call; a miss or a stale hash asks the
// panel server's Template once, giving the hash held, so a current one comes
// back as the hash alone.

/** The Template method of the hub's panel server. */
export const TEMPLATE = 'ktw-hub:panel-service/template'

/** What Template answers: the tree, left out when the hash given is current. */
export interface TemplateAnswer {
  tree?: Record<string, unknown>
  hash: string
}

/** Asks Template: {uri, hash} in. */
export type TemplateFetch = (input: { uri: string; hash?: string }) => Promise<TemplateAnswer>

/** templateKey is a template's place in the cache. */
export function templateKey(uri: string): string {
  return `template|${uri}`
}

/** templateFor is the tree a named tree draws, from the cache when it holds
 *  the answer's hash, else asked once (with the hash held) and kept. */
export async function templateFor(cache: SpecCache, spec: { uri: string; hash?: string }, fetch: TemplateFetch): Promise<Record<string, unknown> | undefined> {
  const key = templateKey(spec.uri)
  const held = cache.get(key)
  if (held && spec.hash && held.hash === spec.hash) return held.part
  const got = await fetch({ uri: spec.uri, ...(held ? { hash: held.hash } : {}) })
  const tree = got.tree ?? (held && got.hash === held.hash ? held.part : undefined)
  if (tree) cache.set(key, { hash: got.hash, part: tree })
  return tree
}

/** namedTrees is the answer with its tree (or dslTree) given inline when it
 *  names a template by `{uri, hash}`; any other answer is answered as it is.
 *  The answer given is not changed. */
export async function namedTrees(answer: Record<string, unknown>, cache: SpecCache, fetch: TemplateFetch): Promise<Record<string, unknown>> {
  let out = answer
  for (const key of ['dslTree', 'tree'] as const) {
    const spec = answer[key]
    if (!isNamedSpec(spec)) continue
    const tree = await templateFor(cache, spec, fetch)
    if (tree) out = { ...out, [key]: tree }
  }
  return out
}

/** namedTreeDef is a page source's answer naming its tree by `{uri, hash}`
 *  drawn as one component: the template's tree, the parts it names given
 *  inline (through `localize`, as inlineParts), over the answer's data. Any
 *  other answer is answered as it is. */
export async function namedTreeDef(answer: unknown, cache: SpecCache, fetchTemplate: TemplateFetch, fetchPart: PartFetch, localize?: Localize): Promise<unknown> {
  if (!answer || typeof answer !== 'object' || Array.isArray(answer)) return answer
  const a = answer as Record<string, unknown>
  if (!isNamedSpec(a.tree)) return answer
  const tree = await templateFor(cache, a.tree, fetchTemplate)
  if (!tree) return answer
  const parts = a.parts && typeof a.parts === 'object' ? a.parts as Record<string, unknown> : undefined
  const drawn = await inlineParts(tree, parts, cache, fetchPart, localize) as Record<string, unknown>
  return a.data === undefined ? drawn : { ...drawn, data: a.data }
}

/** partFetchOf asks a universe's object pane for a part (ResolvePart). */
export function partFetchOf(universe: string): PartFetch {
  const call = dispatcherFor(universe)
  return async (input) => (await call(RESOLVE_PART, input)) as PartAnswer
}

/** templateFetchOf asks a universe's panel server for a template (Template). */
export function templateFetchOf(universe: string): TemplateFetch {
  const call = dispatcherFor(universe)
  return async (input) => (await call(TEMPLATE, input)) as TemplateAnswer
}

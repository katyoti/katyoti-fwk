// SPDX-License-Identifier: Apache-2.0
/**
 * Recipes and actions of the guide, read through what the GUI already has:
 * the catalog bindings (`ListContributions` / `ReadContribution`) for the
 * `guide-recipe` contributions, and the one KTSP dispatcher for a step's
 * `check` and `do`. No guide binary, no new binding (modules/guide/tools.md).
 */

import { ListContributions, OpenContributionLayer, ReadContribution } from '../../wailsjs/go/main/App'
import { ADMIN_UNIVERSE, dispatcherFor } from '../ktsp'
import type { Recipe } from '../generated/guide-recipe'
import type { ErrorMsg } from '../i18n/errors'

export const RECIPE_KIND = 'guide-recipe'

/** The provider that carries the shell recipes into the admin catalog. */
/** The admin universe's member: its methods (the universe checks and setup
 *  actions) are reached in the admin universe. */
const ADMIN_PROVIDER = 'ktw-univers'
/** The carrier of the shell recipes and the welcome tour: the hub, a member of
 *  every universe's view (modules/guide). */
export const SHELL_RECIPE_PROVIDER = 'ktw-hub'

export interface RecipeEntry {
  /** `<provider>:/guide-recipe/<id>` — the recipe's identity. */
  uri: string
  provider: string
  /** The file name without extension; what `requires` names. */
  id: string
  /** The universe whose catalog it was read from. */
  universe: string
  recipe: LoadedRecipe
  /** The recipe's `translations` layer (language → texts), when it has one. */
  translations?: Translations
  /** A sourced guide's whole document, when its source names one (the
   *  installed letter, whose body continues with the full change log): the
   *  reader offers to open it. */
  document?: string
}

/** A recipe's text in one language (modules/guide): what its
 *  `<name>.i18n.yaml` carries per language, published as JSON. A step is its
 *  text alone, or `{say}` (the shape every translatable kind may use —
 *  modules/contributions/contribution-types.md, Translations). */
export interface RecipeText {
  name?: string
  description?: string
  hashtags?: string[]
  body?: string
  steps?: Record<string, string | { say?: string }>
}
/** Language code → texts; `source_lang` names the recipe's own language when
 *  it is not English. */
export type Translations = Record<string, RecipeText> & { source_lang?: string }

/** The recipe's text in `lang`, string by string, each falling back to the
 *  text the recipe itself is written in. */
export function recipeText(e: Pick<RecipeEntry, 'recipe' | 'translations'>, lang: string): {
  name: string
  description: string
  hashtags: string[]
  body: string
  say: (step: Step) => string
} {
  const source = (e.translations?.source_lang as string | undefined) || 'en'
  const tr = lang === source ? undefined : (e.translations?.[lang] as RecipeText | undefined)
  const r = e.recipe
  const stepText = (id: string | undefined) => {
    const v = id ? tr?.steps?.[id] : undefined
    return typeof v === 'string' ? v : v?.say
  }
  return {
    name: tr?.name || r.name,
    description: tr?.description || r.description || '',
    hashtags: tr?.hashtags?.length ? tr.hashtags : r.hashtags ?? [],
    body: tr?.body || r.body || '',
    say: (step: Step) => stepText(step.id) || step.say,
  }
}

/** Reads a recipe's translations layer; none, or one that cannot be read,
 *  leaves the recipe in English. */
async function readTranslations(universe: string, uri: string): Promise<Translations | undefined> {
  try {
    const l = await OpenContributionLayer(universe, uri, 'translations')
    if (l.error || l.refused || !l.content) return undefined
    return JSON.parse(l.content) as Translations
  } catch {
    return undefined
  }
}

export type Step = NonNullable<Recipe['steps']>[number]

/** A recipe as loaded: its steps always a list (empty for a Show me or an
 *  Explain me). */
export type LoadedRecipe = Omit<Recipe, 'steps'> & { steps: Step[] }

/** What a guide is, from what it holds (modules/guide): steps make a
 *  Help me, a video without steps a Show me, a markdown body alone an Explain
 *  me; '' for an empty one. */
export type GuideNature = 'help' | 'show' | 'explain' | ''

export function nature(r: Recipe): GuideNature {
  if ((r.steps ?? []).length > 0) return 'help'
  if (r.video?.trim()) return 'show'
  if (r.body?.trim() || r.source?.trim()) return 'explain'
  return ''
}

/** What a guide's row offers first: a Help me starts in the player, a Show me
 *  plays its video, an Explain me opens its text. */
export function primaryAction(n: GuideNature): 'start' | 'watch' | 'read' | '' {
  return n === 'help' ? 'start' : n === 'show' ? 'watch' : n === 'explain' ? 'read' : ''
}

/** A tour is a Help me with no check. */
export function isTour(r: Recipe): boolean {
  return nature(r) === 'help' && !(r.steps ?? []).some((s) => !!s.check)
}

function parseRecipeURI(uri: string): { provider: string; id: string } {
  const i = uri.indexOf(':/')
  const provider = i > 0 ? uri.slice(0, i) : ''
  const id = uri.slice(uri.lastIndexOf('/') + 1)
  return { provider, id }
}

/** Where a provider's recipe or method is reached: ktw-univers's methods in
 *  the admin universe, anything else in the universe on screen (the hub's
 *  recipes are in both). */
export function universeFor(provider: string, displayed: string | null): string {
  if (provider === ADMIN_PROVIDER || !displayed) return ADMIN_UNIVERSE
  return displayed
}

/** uri → entry, filled by every load; lets `startRecipe(uri)` skip a read. */
const cache = new Map<string, RecipeEntry>()

export function cachedRecipe(uri: string): RecipeEntry | undefined {
  return cache.get(uri)
}

async function readRecipe(universe: string, uri: string): Promise<RecipeEntry> {
  const d = await ReadContribution(universe, uri)
  if (d.error && !d.body) throw new Error(d.error)
  if (!d.body) throw new Error(`recipe ${uri} has no data layer`)
  const parsed = JSON.parse(d.body) as Recipe
  const recipe: LoadedRecipe = { ...parsed, steps: Array.isArray(parsed.steps) ? parsed.steps : [] }
  const { provider, id } = parseRecipeURI(uri)
  const translations = await readTranslations(universe, uri)
  const entry: RecipeEntry = { uri, provider: d.provider || provider, id: d.id || id, universe, recipe, translations }
  cache.set(uri, entry)
  return entry
}

/** Lists every recipe of the given universes' catalogs, deduplicated by URI
 *  (the first universe wins). A universe that cannot be reached is skipped;
 *  its error is returned beside the recipes that could be read. */
export async function loadRecipes(universes: string[]): Promise<{ entries: RecipeEntry[]; errors: string[] }> {
  const seen = new Set<string>()
  const entries: RecipeEntry[] = []
  const errors: string[] = []
  for (const u of universes) {
    try {
      const res = await ListContributions(u)
      if (res.error) {
        errors.push(`${u}: ${res.error}`)
        continue
      }
      const uris = (res.contributions ?? [])
        .filter((c) => c.type === RECIPE_KIND && !seen.has(c.uri))
        .map((c) => c.uri)
      uris.forEach((uri) => seen.add(uri))
      const read = await Promise.allSettled(uris.map((uri) => readRecipe(u, uri)))
      read.forEach((r, i) => {
        if (r.status === 'fulfilled') entries.push(r.value)
        else errors.push(`${uris[i]}: ${String(r.reason)}`)
      })
    } catch (e) {
      errors.push(`${u}: ${String(e)}`)
    }
  }
  return { entries, errors }
}

/** Reads one recipe by URI: from the cache, else from the universe that
 *  carries its provider, else from the other one. */
export async function fetchRecipe(uri: string, displayed: string | null): Promise<RecipeEntry> {
  const hit = cache.get(uri)
  if (hit) return hit
  const first = universeFor(parseRecipeURI(uri).provider, displayed)
  try {
    return await readRecipe(first, uri)
  } catch (e) {
    const other = first === ADMIN_UNIVERSE ? displayed : ADMIN_UNIVERSE
    if (!other || other === first) throw e
    return readRecipe(other, uri)
  }
}

/** What a check (or a step's `do` method) answers — guideapi.CheckResult.
 *  `detail` is English; `detailMsg` is the same sentence by key in the
 *  `guide` namespace (`checks.<name>`), said with `errorTextWith`. */
export interface CheckResult {
  ok: boolean
  detail: string
  detailMsg?: ErrorMsg | null
}

/** Runs a step's `check` or `do`: the action URI `<provider>:<slot>/<method>`
 *  IS the dispatcher's slot path (the generated clients call
 *  `ktw-univers:profile/guide-get` the same way). The player adds the
 *  universe on screen beside the step's `with`. */
export async function runStepAction(actionURI: string, displayed: string | null, withArgs?: Record<string, string>): Promise<unknown> {
  const provider = actionURI.slice(0, actionURI.indexOf(':'))
  const dispatch = dispatcherFor(universeFor(provider, displayed))
  const slot = actionURI.slice(actionURI.indexOf(':') + 1).split('/')[0]
  // A task or a script is not a slot of its provider: the provider's
  // ActionService runs it (`<provider>:action/run`), with the step's `with`
  // as the task's input. It starts the run and returns; the step's next
  // check sees its result.
  if (slot === 'tasks' || slot === 'scripts') {
    return dispatch(`${provider}:action/run`, { uri: actionURI, input: withArgs ?? {} })
  }
  return dispatch(actionURI, { universe: displayed ?? '', with: withArgs ?? {} })
}

export async function runCheck(actionURI: string, displayed: string | null, withArgs?: Record<string, string>): Promise<CheckResult> {
  try {
    const out = (await runStepAction(actionURI, displayed, withArgs)) as Partial<CheckResult> | null
    return { ok: !!out?.ok, detail: out?.detail ?? (out?.ok ? 'Done.' : 'Not done yet.'), detailMsg: out?.detailMsg }
  } catch (e) {
    return { ok: false, detail: e instanceof Error ? e.message : String(e) }
  }
}

/** True when `completed` (recipe identities as stored: URI or file name)
 *  holds the recipe with this id. */
export function completedHas(completed: Set<string>, id: string): boolean {
  if (completed.has(id)) return true
  for (const c of completed) if (c.endsWith(`/${RECIPE_KIND}/${id}`)) return true
  return false
}

export function isCompleted(completed: Set<string>, e: RecipeEntry): boolean {
  return completed.has(e.uri) || completedHas(completed, e.id)
}

/** What records that a shown-once guide was seen: its URI, and its
 *  `once_key` when it has one (a What's new is seen once per version). */
export function onceId(e: Pick<RecipeEntry, 'uri' | 'recipe'>): string {
  return e.recipe.once_key ? `${e.uri}@${e.recipe.once_key}` : e.uri
}

/** The guides to open by themselves at start: those marked `shown_once` not
 *  seen yet (for their key), in their declared order. */
export function startQueue(entries: RecipeEntry[], completed: Set<string>): RecipeEntry[] {
  return entries
    .filter((e) => (e.recipe.shown_once ?? 0) > 0 && nature(e.recipe) !== '' && !completed.has(onceId(e)))
    .sort((a, b) => (a.recipe.shown_once ?? 0) - (b.recipe.shown_once ?? 0) || a.uri.localeCompare(b.uri))
}

/** What a guide's source answers: its text and the key of what
 *  it shows - the newest change-log entry for What's new. */
export interface SourcedText {
  markdown: string
  key?: string
  /** The object the text comes from, opened whole. */
  document?: string
}

/** The key a person last saw of a sourced shown-once guide: the highest of
 *  the `<uri>@<key>` completions recorded for it (keys end in a number). */
export function lastKey(uri: string, completed: Set<string>): string {
  const num = (k: string) => Number(k.match(/(\d+)$/)?.[1] ?? 0)
  let best = ''
  for (const c of completed) {
    if (!c.startsWith(uri + '@')) continue
    const k = c.slice(uri.length + 1)
    if (!best || num(k) > num(best)) best = k
  }
  return best
}

/**
 * The shown-once guides whose text comes from a source, filled for this
 * start: each is asked what is new after the key last seen; one with nothing
 * new is left out, one with something new carries the text and that key - so
 * it is seen once per key. The others pass unchanged.
 */
export async function fillSourced(
  entries: RecipeEntry[], completed: Set<string>,
  fetch: (source: string, after: string) => Promise<SourcedText | null>,
): Promise<RecipeEntry[]> {
  const out: RecipeEntry[] = []
  for (const e of entries) {
    const source = e.recipe.source?.trim()
    if (!source || !(e.recipe.shown_once ?? 0)) { out.push(e); continue }
    const got = await fetch(source, lastKey(e.uri, completed)).catch(() => null)
    if (!got?.markdown?.trim() || !got.key) continue
    // Seen under this very key already: whatever its shape (a letter's ends
    // in a hash, so lastKey may not be it), it is not new.
    if (completed.has(`${e.uri}@${got.key}`)) continue
    out.push({ ...e, ...(got.document ? { document: got.document } : {}), recipe: { ...e.recipe, body: got.markdown, once_key: got.key } })
  }
  return out
}

/** Asks a guide's source in a universe (a KTSP method: `<provider>:<slot>/<method>`). */
export function fetchSourced(universe: string): (source: string, after: string) => Promise<SourcedText | null> {
  return async (source, after) => {
    const out = (await dispatcherFor(universe)(source, { after })) as SourcedText | undefined
    return out ?? null
  }
}

/** At the Essentiel display level, the essentials of the universe's wired
 *  sidecars: the guides each sidecar names essential, by name;
 *  none at Standard or Expert. Only wired sidecars' guides are loaded, so an
 *  unwired sidecar's essentials are gone with it. */
/** The hashtag of the guides every tagged universe offers: how to use KaTyoti
 *  itself (the usage tour, the Inbox, recording, what's new). */
export const USAGE_TAG = 'katyoti-usage'

/** The guides a universe's Guides tab offers. A universe with
 *  tags offers those carrying one of its priority tags (its view's `first`)
 *  or {@link USAGE_TAG}, and KaTyoti's own concept documents (the shell's
 *  guides marked concept), and nothing else; a universe without
 *  tags - or whose view is not read yet - offers them all. No guide or
 *  universe is named. */
export function offeredIn(entries: RecipeEntry[], view: { all: boolean; first?: string[] } | null | undefined): RecipeEntry[] {
  if (!view || view.all) return entries
  const tags = new Set([...(view.first ?? []), USAGE_TAG])
  return entries.filter((e) => (e.provider === SHELL_RECIPE_PROVIDER && !!e.recipe.concept)
    || (e.recipe.hashtags ?? []).some((h) => tags.has(h)))
}

export function essentials(entries: RecipeEntry[], level: string): RecipeEntry[] {
  if (level !== 'essential') return []
  return entries.filter((e) => e.recipe.essential).sort((a, b) => a.recipe.name.localeCompare(b.recipe.name))
}

/** The completion a universe's first guide is seen under: once per universe. */
export function firstGuideKey(uri: string, universe: string): string {
  return `${uri}@first:${universe}`
}

/** The first guide to play for a universe: the one it names,
 *  unless already seen (or skipped) for it; '' when there is none. */
export function firstGuideDue(universe: { name: string; firstGuide?: string } | undefined, completed: Set<string>): string {
  const uri = universe?.firstGuide?.trim()
  if (!universe || !uri) return ''
  return completed.has(firstGuideKey(uri, universe.name)) ? '' : uri
}

/** The topic the « Concepts KaTyoti » entry of the help menu opens the guide
 *  on: it keeps the concept documents only. */
export const CONCEPTS_TOPIC = 'concepts'

/** The documents explaining KaTyoti's concepts: the guides a
 *  sidecar marks `concept`, by name - the guide lists them first. */
export function concepts(entries: RecipeEntry[]): RecipeEntry[] {
  return entries.filter((e) => e.recipe.concept).sort((a, b) => a.recipe.name.localeCompare(b.recipe.name))
}

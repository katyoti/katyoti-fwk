// SPDX-License-Identifier: Apache-2.0
/**
 * Contribution texts in the GUI language — the i18next namespace `contrib`.
 *
 * What a person reads of a contribution (an object type's label, a panel's
 * name, an action's label…) is written once, in the contribution's own
 * language, and translated in its `<name>.i18n.yaml` beside it
 * (modules/contributions/contribution-types.md, Translations). The hub
 * answers every translation of a universe in one language as one flat bundle
 * (`ktw-hub:catalog-service/translations`, `<uri>#<path>` → text); it is the
 * `contrib` namespace, loaded for the displayed universe and the GUI language,
 * and re-read when the catalog changes (`catalog.changed`).
 *
 * `ct(uri, path, fallback)` reads one text: the translation when the bundle
 * has it, else `fallback` — the text the contribution carries, which IS its
 * source language. A path names the text in the entry: `label`, `name`,
 * `description`, `actions.<label>.label`, `states.<name>.name`.
 */
import { useCallback, useEffect } from 'react'
import { create } from 'zustand'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import { catalogClient } from '../ktsp'
import { useAppStore } from '../store'
import { appLanguage, i18nInstance, useAppLanguage } from '.'

export const CONTRIB_NAMESPACE = 'contrib'

/** `<kind>/<id>` → the URI of the contribution the bundle translates — so a
 *  surface that knows a panel's id or a type's name, not its provider, still
 *  finds its texts. An id two providers both translate keeps the last loaded. */
const uriIndex = new Map<string, string>()

/** Bumped on every bundle load, so `useCT` consumers re-render. */
const useBundleVersion = create<{ version: number }>(() => ({ version: 0 }))

/** The bundle key of one text: `<uri>#<path>` (contribapi.TranslationKey). */
export function contribKey(uri: string, path: string): string {
  return `${uri}#${path}`
}

/**
 * Loads a universe's translation bundle in `lang` into the `contrib`
 * namespace. Bundles of several universes merge: a key names its provider,
 * which is the same contribution in every universe that wires it.
 */
export async function loadContribTexts(universe: string, lang: string): Promise<void> {
  const i = i18nInstance()
  if (!i) return
  const out = await catalogClient(universe).translations({ language: lang })
  const texts = out?.texts ?? {}
  for (const key of Object.keys(texts)) {
    const uri = key.slice(0, key.indexOf('#'))
    const m = /^[^:]+:\/([^/]+)\/(.+)$/.exec(uri)
    if (m) uriIndex.set(`${m[1]}/${m[2]}`, uri)
  }
  i.addResourceBundle(lang, CONTRIB_NAMESPACE, texts, true, true)
  useBundleVersion.setState((s) => ({ version: s.version + 1 }))
}

/** The URI of contribution `<kind>/<id>` when the bundle translates it. */
export function contribURI(kind: string, id: string | undefined): string | undefined {
  return id ? uriIndex.get(`${kind}/${id}`) : undefined
}

/** One contribution text in `lang` (default: the GUI language), else `fallback`. */
export function ct(uri: string | undefined, path: string, fallback: string, lang: string = appLanguage()): string {
  const i = i18nInstance()
  if (!i || !uri) return fallback
  // keySeparator off: a key is `<uri>#<path>`, whose `.` and `:` are not
  // i18next's nesting or namespace separators.
  const v = i.getResource(lang, CONTRIB_NAMESPACE, contribKey(uri, path), { keySeparator: false } as never)
  return typeof v === 'string' && v !== '' ? v : fallback
}

/** `ct` bound to the GUI language, re-rendering when the language or the
 *  bundle changes. */
export function useCT(): (uri: string | undefined, path: string, fallback: string) => string {
  const lang = useAppLanguage()
  const version = useBundleVersion((s) => s.version)
  // eslint-disable-next-line react-hooks/exhaustive-deps
  return useCallback((uri, path, fallback) => ct(uri, path, fallback, lang), [lang, version])
}

/**
 * Keeps the `contrib` namespace loaded: for the displayed universe in the GUI
 * language, again when either changes, and when that universe's catalog
 * reloads. Mounted once (App.tsx). A universe the GUI language is English for
 * still loads: a French-source pack translates INTO English.
 */
export function useContribTextsSync(): void {
  const universe = useAppStore((s) => s.displayedUniverseName)
  const lang = useAppLanguage()
  useEffect(() => {
    if (!universe) return
    const load = () => {
      loadContribTexts(universe, lang).catch((err) => {
        // eslint-disable-next-line no-console
        console.warn('[i18n] contribution texts not loaded; showing their own language:', err)
      })
    }
    load()
    const off = EventsOn('catalog.changed', (evt: { universe?: string }) => {
      if (!evt?.universe || evt.universe === universe) load()
    })
    return () => { off() }
  }, [universe, lang])
}

/** An action label, translated by the first of `declarers` (the URIs of the
 *  entries that may bind it: the object's type, then its capabilities) whose
 *  translations name it — an action is keyed by its label (`actions.<label>.label`). */
export function ctActionLabel(
  ctFn: (uri: string | undefined, path: string, fallback: string) => string,
  declarers: (string | undefined)[],
  label: string,
): string {
  for (const uri of declarers) {
    const t = ctFn(uri, `actions.${label}.label`, '')
    if (t) return t
  }
  return label
}

/** The URI the bundle translates a contribution under, from the URI a
 *  surface holds (a data-type URI whose provider may be implicit, `:/…`):
 *  the bundle's own when it names `<kind>/<id>`, else the URI as given. */
export function contribURIOf(uri: string | undefined): string | undefined {
  if (!uri) return undefined
  const m = /^[^:]*:\/([^/]+)\/(.+)$/.exec(uri)
  return (m && contribURI(m[1], m[2])) || uri
}

type CT = (uri: string | undefined, path: string, fallback: string) => string

/** A data-type schema (DSL AST) in the GUI language: each field's label and
 *  description (`types.<Type>.fields.<field>.label|description`), the label
 *  and description of each entry of a field's `content.options` list, by its
 *  `value` (`types.<Type>.fields.<field>.content.options.<value>.label`; an
 *  entry without a value and the short map form stay as they are), and, in each
 *  of a type's extensions (by its `name`), the label, prompt and confirm of
 *  each menu and slots.actions entry (by its `id`), the title and description
 *  of each group with an `id`, and the axis label
 *  (`types.<Type>.extensions.<name>.menu.<id>.label`, `….groups.<id>.title`,
 *  `….axisLabel`) - so a form, a table's menu or a view built from it reads
 *  translated. Every text falls back to its own; the schema given is not
 *  changed. */
export function ctSchema<S>(ctFn: CT, uri: string | undefined, schema: S): S {
  const u = contribURIOf(uri)
  const s = schema as unknown as { types?: Record<string, { fields?: Record<string, { label?: string; description?: string }>; extensions?: unknown }> }
  if (!u || !s?.types) return schema
  const types: Record<string, unknown> = {}
  for (const [tn, tdef] of Object.entries(s.types)) {
    if (!tdef || typeof tdef !== 'object') { types[tn] = tdef; continue }
    let out: Record<string, unknown> = tdef
    if (tdef.fields) {
      const fields: Record<string, unknown> = {}
      for (const [fn, f] of Object.entries(tdef.fields)) {
        if (!f || typeof f !== 'object') { fields[fn] = f; continue }
        const base = `types.${tn}.fields.${fn}`
        const content = (f as Texts).content as Texts | undefined
        fields[fn] = {
          ...f,
          ...(f.label ? { label: ctFn(u, `${base}.label`, f.label) } : {}),
          ...(f.description ? { description: ctFn(u, `${base}.description`, f.description) } : {}),
          ...(content && typeof content === 'object' && Array.isArray(content.options)
            ? { content: { ...content, options: ctKeyed(ctFn, u, `${base}.content.options`, content.options, OPTION_TEXTS, 'value') } }
            : {}),
        }
      }
      out = { ...out, fields }
    }
    if (Array.isArray(tdef.extensions)) {
      out = { ...out, extensions: tdef.extensions.map((e) => ctExtension(ctFn, u, `types.${tn}.extensions`, e)) }
    }
    types[tn] = out
  }
  return { ...(schema as object), types } as S
}

type Texts = Record<string, unknown>

/** One text field of `o` translated at `<base>.<field>`, when it holds one. */
function ctTexts(ctFn: CT, uri: string, base: string, o: Texts, fields: readonly string[]): Texts {
  let out = o
  for (const f of fields) {
    const v = o[f]
    if (typeof v === 'string' && v !== '') out = { ...out, [f]: ctFn(uri, `${base}.${f}`, v) }
  }
  return out
}

/** Each entry of the list `items` keyed by its `key` field (`id` by default)
 *  with its texts translated at `<base>.<key>.<field>`; an entry without a
 *  key stays as it is. */
function ctKeyed(ctFn: CT, uri: string, base: string, items: unknown, fields: readonly string[], key = 'id'): unknown {
  if (!Array.isArray(items)) return items
  return items.map((it) => {
    if (!it || typeof it !== 'object') return it
    const k = (it as Texts)[key]
    return typeof k === 'string' && k !== '' ? ctTexts(ctFn, uri, `${base}.${k}`, it as Texts, fields) : it
  })
}

const ENTRY_TEXTS = ['label', 'prompt', 'confirm'] as const
const GROUP_TEXTS = ['title', 'description'] as const
const OPTION_TEXTS = ['label', 'description'] as const

/** A type's extension, by its `name`, with its menu, slots.actions, groups
 *  and axis label in the GUI language (the data-type kind's `translatable`). */
function ctExtension(ctFn: CT, uri: string, prefix: string, ext: unknown): unknown {
  if (!ext || typeof ext !== 'object') return ext
  const e = ext as Texts
  const name = e.name
  if (typeof name !== 'string' || name === '') return ext
  const base = `${prefix}.${name}`
  let out: Texts = ctTexts(ctFn, uri, base, e, ['axisLabel'])
  if (Array.isArray(e.menu)) out = { ...out, menu: ctKeyed(ctFn, uri, `${base}.menu`, e.menu, ENTRY_TEXTS) }
  if (Array.isArray(e.groups)) out = { ...out, groups: ctKeyed(ctFn, uri, `${base}.groups`, e.groups, GROUP_TEXTS) }
  const slots = e.slots
  if (slots && typeof slots === 'object' && Array.isArray((slots as Texts).actions)) {
    out = { ...out, slots: { ...(slots as Texts), actions: ctKeyed(ctFn, uri, `${base}.slots.actions`, (slots as Texts).actions, ENTRY_TEXTS) } }
  }
  return out
}

/** A header badge in the GUI language: its prefix (`<prefix>: <value>`) and
 *  its tooltip, from the translations of whoever declared it — its
 *  capability (`badges.<field>`), else the object's type (`pane.badges.<field>`).
 *  The value is data and stays as it is. */
export function ctBadge(
  ctFn: CT,
  b: { id: string; label: string; tooltip?: string; capability?: string },
  objectType?: string,
): { label: string; tooltip?: string } {
  const uri = b.capability ? contribURI('capability', b.capability) : contribURI('object-type', objectType)
  if (!uri) return { label: b.label, tooltip: b.tooltip }
  const field = b.capability && b.id.startsWith(`${b.capability}.`) ? b.id.slice(b.capability.length + 1) : b.id
  const base = b.capability ? `badges.${field}` : `pane.badges.${field}`
  let label = b.label
  const sep = label.indexOf(': ')
  if (sep > 0) {
    const prefix = ctFn(uri, `${base}.label`, '')
    if (prefix) label = `${prefix}: ${label.slice(sep + 2)}`
  }
  return { label, tooltip: b.tooltip ? ctFn(uri, `${base}.tooltip`, b.tooltip) : b.tooltip }
}

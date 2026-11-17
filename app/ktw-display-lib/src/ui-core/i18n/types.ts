// SPDX-License-Identifier: Apache-2.0
// UITexts — i18n facade exposed to renderer factories.
//
// Replaces the legacy hardcoded text-bag from
// _ktw_workspace/old_ui_main/.../cpm-core/texts/types.ts (which had a fixed
// French-default set of `selected`/`emptyMessage`/`buttons.*` keys).
//
// At runtime, `texts` is a thin facade over `react-i18next`'s `t()` from
// `useTranslation(namespace)`. Each renderer typically calls
// `useTranslation('email')` (or whatever its content-type name is) and gets
// a `texts` object scoped to its namespace. Cross-renderer messages live in
// the `core` namespace.
//
// Resource files: `content-types/<name>/i18n/<locale>.json` are loaded as
// i18next namespaces at bundle time (see ../runtime/build.ts in S5r2).

/** Options forwarded to i18next's `t()`. Subset that's useful in renderers. */
export interface TranslateOptions {
  /** Default value if the key is missing. */
  defaultValue?: string
  /** Plural form selector. */
  count?: number
  /** Variable interpolation ({{name}} → value). */
  [key: string]: unknown
}

/**
 * Thin facade over react-i18next's `t()`. Each renderer consumes a UITexts
 * scoped to its content-type's namespace.
 */
export interface UITexts {
  /** Translate a key. Returns the translation or `key` itself if missing. */
  t(key: string, options?: TranslateOptions): string

  /** Current language code, e.g., 'en' / 'fr'. */
  language: string

  /** Build a UITexts scoped to a different namespace. Useful when a renderer
   *  needs to access the shared `core` namespace from inside its own. */
  withNamespace(namespace: string): UITexts
}

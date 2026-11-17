// SPDX-License-Identifier: Apache-2.0
// i18n adapters — wrap react-i18next + expose the UITexts facade.
//
// Two consumption patterns:
//   1. Renderer factories receive `texts: UITexts` via their factory params.
//      Authored once at factory time, captured in closures used by render fns.
//   2. Render functions (which run inside the React tree) can call `useTexts()`
//      directly to access translations bound to the current locale via React
//      context.
//
// `initI18n()` is called once at runtime bootstrap (see src/runtime/index.tsx
// in S5r2). It loads bundled locale resources from
// `content-types/<name>/i18n/<locale>.json` (one namespace per content type)
// plus the shared `core` namespace.

import i18next, { type i18n as I18nInstance, type TFunction } from 'i18next'
import { initReactI18next, useTranslation } from 'react-i18next'
import type { TranslateOptions, UITexts } from './types.ts'
import planningEn from '../../ui-builders/fields/_planning/i18n/en.json'
import planningFr from '../../ui-builders/fields/_planning/i18n/fr.json'
import formEn from '../action/i18n/en.json'
import formFr from '../action/i18n/fr.json'

export type { TranslateOptions, UITexts } from './types.ts'

/** Build a UITexts adapter from an i18next instance + namespace. */
export function makeUITexts(instance: I18nInstance, namespace: string): UITexts {
  const t: TFunction = instance.getFixedT(null, namespace)
  return {
    t(key: string, options?: TranslateOptions): string {
      return t(key, options as never) as unknown as string
    },
    language: instance.language || 'en',
    withNamespace(otherNamespace: string): UITexts {
      return makeUITexts(instance, otherNamespace)
    },
  }
}

/** React hook — returns a UITexts scoped to the given namespace using the
 *  current i18n locale via context. Use inside renderer render functions. */
export function useTexts(namespace: string): UITexts {
  const { t, i18n } = useTranslation(namespace)
  return {
    t(key: string, options?: TranslateOptions): string {
      return t(key, options as never) as unknown as string
    },
    language: i18n.language || 'en',
    withNamespace(otherNamespace: string): UITexts {
      return makeUITexts(i18n, otherNamespace)
    },
  }
}

export interface InitI18nOptions {
  /** Active locale at boot. Default: 'en'. */
  language?: string
  /** Resources keyed by `[locale][namespace] = { key: translation }` (a
   *  translation may nest: `{ status: { done: '…' } }` reads `status.done`). */
  resources?: Record<string, Record<string, Record<string, unknown>>>
  /** Fallback locale chain when a key is missing. Default: ['en']. */
  fallbackLng?: string | string[]
}

/** The namespaces the library itself owns, by locale: `form` (a form's
 *  standard actions - ui-core/action/defaults.ts, ui-core/action/i18n) and `planning` (the
 *  planning field renderers' words — ui-builders/fields/_planning/i18n). */
export const LIB_RESOURCES: Record<string, Record<string, Record<string, unknown>>> = {
  en: { form: formEn, planning: planningEn },
  fr: { form: formFr, planning: planningFr },
}

/** withLibResources is the caller's resources with the library's namespaces
 *  added where the caller has none of its own. */
function withLibResources(resources: InitI18nOptions['resources'] = {}): NonNullable<InitI18nOptions['resources']> {
  const out: NonNullable<InitI18nOptions['resources']> = {}
  for (const lng of new Set([...Object.keys(LIB_RESOURCES), ...Object.keys(resources)])) {
    out[lng] = { ...LIB_RESOURCES[lng], ...resources[lng] }
  }
  return out
}

/** addLibResources adds the library's namespaces to a running instance —
 *  deep, never overwriting a translation already there. */
function addLibResources(instance: I18nInstance): void {
  for (const [lng, nss] of Object.entries(LIB_RESOURCES)) {
    for (const [ns, res] of Object.entries(nss)) instance.addResourceBundle(lng, ns, res, true, false)
  }
}

/** Initialize the global i18next instance. Idempotent — safe to call once at
 *  runtime startup. The runtime bootstrap (S5r2) wires this with bundled
 *  resources from each renderer's `i18n/<locale>.json`. The library's own
 *  namespaces (LIB_RESOURCES) are always loaded: at init, or — when another
 *  consumer started the instance first — added to it where missing. */
export async function initI18n(opts: InitI18nOptions = {}): Promise<I18nInstance> {
  if (i18next.isInitialized) {
    addLibResources(i18next)
    return i18next
  }
  const resources = withLibResources(opts.resources)
  await i18next
    .use(initReactI18next)
    .init({
      lng: opts.language ?? 'en',
      fallbackLng: opts.fallbackLng ?? ['en'],
      resources: resources as never,
      interpolation: { escapeValue: false }, // React already escapes
      defaultNS: 'core',
      ns: [...new Set([...(opts.resources?.[opts.language ?? 'en'] ? [] : ['core']), ...Object.keys(resources[opts.language ?? 'en'] ?? {})])],
    })
  return i18next
}

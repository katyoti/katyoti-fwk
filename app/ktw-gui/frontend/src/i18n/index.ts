// SPDX-License-Identifier: Apache-2.0
/**
 * The GUI's language — ONE i18next instance (the display library's), set up
 * once at boot with the bundled texts of every namespace
 * (`src/locales/<lang>/<ns>.json`), in the person's language:
 *
 *   Profile.language (hub-owned, en | fr)  →  else the OS language
 *   (the WebView reports it as `navigator.language`)  →  else English.
 *
 * The language follows the profile LIVE: profileStore is re-read on
 * `profile.changed` (hooks/useProfileSync.ts) and every change of its language
 * is an `i18next.changeLanguage`, which re-renders every `useT` consumer. A key
 * missing in a language falls back to English.
 *
 * A component reads its namespace with `useT('<ns>')`; outside React,
 * `tFor('<ns>')`. Contribution texts (object types, panels, actions) are not
 * here: they come from the hub's translation bundle (contrib.ts).
 */
import { useMemo } from 'react'
import { initI18n, useTexts } from '@ktw/display-lib/ui-core/i18n/index.ts'
import { useProfileStore } from '../store/profileStore'
import enGuide from '../locales/en/guide.json'
import frGuide from '../locales/fr/guide.json'
import enShell from '../locales/en/shell.json'
import frShell from '../locales/fr/shell.json'
import enCards from '../locales/en/cards.json'
import frCards from '../locales/fr/cards.json'
import enProfile from '../locales/en/profile.json'
import frProfile from '../locales/fr/profile.json'
import enWorkspace from '../locales/en/workspace.json'
import frWorkspace from '../locales/fr/workspace.json'
import enHome from '../locales/en/home.json'
import frHome from '../locales/fr/home.json'
import enRuns from '../locales/en/runs.json'
import frRuns from '../locales/fr/runs.json'
import enInbox from '../locales/en/inbox.json'
import frInbox from '../locales/fr/inbox.json'
import enSettings from '../locales/en/settings.json'
import frSettings from '../locales/fr/settings.json'
import enUniverses from '../locales/en/universes.json'
import frUniverses from '../locales/fr/universes.json'
import enChat from '../locales/en/chat.json'
import frChat from '../locales/fr/chat.json'
import enErrors from '../locales/en/errors.json'
import frErrors from '../locales/fr/errors.json'
import enMarketplace from '../locales/en/marketplace.json'
import frMarketplace from '../locales/fr/marketplace.json'
import enTeam from '../locales/en/team.json'
import frTeam from '../locales/fr/team.json'
import enComponents from '../locales/en/components.json'
import frComponents from '../locales/fr/components.json'
import enScreens from '../locales/en/screens.json'
import frScreens from '../locales/fr/screens.json'
import enReady from '../locales/en/ready.json'
import frReady from '../locales/fr/ready.json'

/** The languages the GUI speaks — profile/api SupportedLanguages. */
export const APP_LANGUAGES = ['en', 'fr'] as const
export type AppLanguage = (typeof APP_LANGUAGES)[number]

/** Every bundled namespace, per language. English holds every key. */
const RESOURCES = {
  en: {
    guide: enGuide, shell: enShell, cards: enCards, profile: enProfile,
    workspace: enWorkspace, home: enHome, runs: enRuns, inbox: enInbox, settings: enSettings, universes: enUniverses, chat: enChat, errors: enErrors, marketplace: enMarketplace,
    team: enTeam, components: enComponents, screens: enScreens, ready: enReady,
  },
  fr: {
    guide: frGuide, shell: frShell, cards: frCards, profile: frProfile,
    workspace: frWorkspace, home: frHome, runs: frRuns, inbox: frInbox, settings: frSettings, universes: frUniverses, chat: frChat, errors: frErrors, marketplace: frMarketplace,
    team: frTeam, components: frComponents, screens: frScreens, ready: frReady,
  },
} as const

/** The display library's i18next instance. */
export type I18nInstance = Awaited<ReturnType<typeof initI18n>>

/** A translation function: a key of a namespace, with i18next options. */
export type TFn = (key: string, options?: Record<string, unknown>) => string

let instance: I18nInstance | null = null

/** The OS language mapped to a GUI language: French when it starts with `fr`. */
export function browserLanguage(): AppLanguage {
  const l = typeof navigator === 'undefined' ? '' : navigator.language || navigator.languages?.[0] || ''
  return l.toLowerCase().startsWith('fr') ? 'fr' : 'en'
}

/** The GUI language for a profile language (empty → the OS language). */
export function languageOf(profileLanguage: string | undefined): AppLanguage {
  return (APP_LANGUAGES as readonly string[]).includes(profileLanguage ?? '')
    ? (profileLanguage as AppLanguage)
    : browserLanguage()
}

/** The GUI language now, outside React. */
export function appLanguage(): AppLanguage {
  return languageOf(useProfileStore.getState().profile.language)
}

/** The GUI language, re-rendering when the profile changes it. */
export function useAppLanguage(): AppLanguage {
  return languageOf(useProfileStore((s) => s.profile.language))
}

/**
 * Set the GUI's i18n up — once, before the first render (main.tsx). Idempotent.
 *
 * Another display-library consumer may have initialised the shared instance
 * first (a renderer calling `initI18n()` at import): the bundles are then added
 * to it and its language set, so the boot order never decides the language.
 */
export async function initAppI18n(): Promise<I18nInstance> {
  if (instance) return instance
  const lang = appLanguage()
  const i = await initI18n({ language: lang, fallbackLng: ['en'], resources: RESOURCES as never })
  for (const [lng, nss] of Object.entries(RESOURCES)) {
    for (const [ns, res] of Object.entries(nss)) {
      if (!i.hasResourceBundle(lng, ns)) i.addResourceBundle(lng, ns, res, true, false)
    }
  }
  if (i.language !== lang) await i.changeLanguage(lang)
  instance = i
  // Live switch: the profile's language is the GUI's.
  useProfileStore.subscribe((s) => {
    const next = languageOf(s.profile.language)
    if (i.language !== next) void i.changeLanguage(next)
  })
  return i
}

/** The instance once set up; null before `initAppI18n` resolved. */
export function i18nInstance(): I18nInstance | null {
  return instance
}

/** A namespace's `t` in the GUI language, re-rendering on a language change.
 *  Stable while the language is: safe in an effect's dependencies. */
export function useT(ns: string): TFn {
  const texts = useTexts(ns)
  // eslint-disable-next-line react-hooks/exhaustive-deps
  return useMemo(() => texts.t as TFn, [ns, texts.language])
}

/** A namespace's `t` outside React, in `lang` (default: the GUI language). */
export function tFor(ns: string, lang: AppLanguage = appLanguage()): TFn {
  const i = instance
  if (!i) return (key) => key
  const t = i.getFixedT(lang, ns)
  return (key, options) => t(key, options as never) as unknown as string
}

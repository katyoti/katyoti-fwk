// SPDX-License-Identifier: Apache-2.0
/**
 * The guide's own UI text: the i18next namespace `guide`
 * (`locales/<lang>/guide.json`), in the GUI's language — the guide speaks the
 * language the rest of the GUI speaks (i18n/index.ts). A key missing in a
 * language falls back to English.
 */
import { tFor, useT, type TFn } from '../i18n'

export const GUIDE_NAMESPACE = 'guide'

/** A translation function of the `guide` namespace. */
export type GuideT = TFn

/** The guide's `t`, re-rendering when the language changes. */
export function useGuideT(): GuideT {
  return useT(GUIDE_NAMESPACE)
}

/** The guide's `t` in the GUI language now, outside React. */
export function guideT(): GuideT {
  return tFor(GUIDE_NAMESPACE)
}

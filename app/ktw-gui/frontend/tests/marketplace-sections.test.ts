// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'

// One Marketplace entry: a single 'marketplace' tab whose
// MarketplaceRoot shows the sections Infra, Sidecars, Config-sets and Seeds
// (and Solutions), each a tab with its panel and its label in every language.
const src = (p: string) => readFileSync(new URL(`../src/${p}`, import.meta.url), 'utf8')
const json = (lang: string) =>
  JSON.parse(readFileSync(new URL(`../src/locales/${lang}/marketplace.json`, import.meta.url), 'utf8'))

const root = src('sections/marketplace/MarketplaceRoot.tsx')
const declared = (): string[] => {
  const m = root.match(/MARKETPLACE_SECTIONS:\s*MarketplaceSection\[\]\s*=\s*\[([^\]]*)\]/)
  if (!m) throw new Error('MARKETPLACE_SECTIONS not found in MarketplaceRoot.tsx')
  return [...m[1].matchAll(/'([a-z-]+)'/g)].map((x) => x[1])
}

describe('the Marketplace entry', () => {
  test('its sections start with Infra, Sidecars, Config-sets, Seeds', () => {
    expect(declared().slice(0, 4)).toEqual(['infra', 'sidecars', 'configsets', 'seeds'])
  })
  test('every section has its panel', () => {
    for (const s of declared()) expect(root).toContain(`<Tabs.Panel value="${s}"`)
  })
  for (const lang of ['en', 'fr']) {
    test(`${lang}: every section has a label`, () => {
      const tabs = json(lang).tabs ?? {}
      for (const s of declared()) expect(typeof tabs[s] === 'string' && tabs[s].length > 0).toBe(true)
    })
  }
  test('it is one tab, opened by one launcher', () => {
    const launcher = src('hooks/useMarketplaceLauncher.ts')
    expect(launcher).toContain("const MARKETPLACE_TAB_ID = 'marketplace'")
    expect(src('sections/shell/MainPaneTabs.tsx')).toContain("case 'marketplace': return <MarketplaceTabBody")
  })
})

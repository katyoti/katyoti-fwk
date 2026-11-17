// SPDX-License-Identifier: Apache-2.0
// Deck themes, the component's half (ktw-display-lib deck/theme.ts): the CSS a
// theme becomes and the look each slide takes.

import { describe, expect, it } from 'bun:test'
import {
  deckAspect,
  allowsColour, boxStyle, chromeText, DEFAULT_WATERMARK, deckWatermark, iconAsset, isColour, slideLook, themeCSS, type LoadedTheme, type Theme,
} from '../../../../ktw-display-lib/src/ui-builders/deck/theme.ts'

const brand: Theme = {
  name: 'brand',
  description: 'A test brand.',
  tokens: { accent: '#FF2822', 'heading-font': 'Skeena, Arial, sans-serif' },
  dark: { bg: '#111' },
  fonts: [{ family: 'Skeena', src: 'fonts/skeena.woff2', weight: '700' }],
  chrome: {
    logo: { asset: 'logo.png', box: { x: 32, y: 648, w: 228, h: 72 } },
    footer: { template: '{title} · {n}/{total}', box: { x: 870, y: 667, w: 336, h: 38 } },
  },
  layouts: {
    cover: {
      chrome: false,
      default: { background: 'bg/bordeaux.jpg', tokens: { heading: '#FFFFFF' } },
      variants: {
        sand: { background: 'bg/sand.jpg' },
        plain: { background: '#F2EBE2', chrome: true, logo: { asset: 'logo-white.png', box: { x: 0, y: 0, w: 128, h: 72 } } },
      },
    },
    content: {
      variants: { wide: { slots: { right: { x: 640, y: 72, w: 572, h: 576 }, title: { x: 68, y: 72, w: 1144, h: 100 } } } },
    },
  },
  icons: { camion: { asset: 'icons/camion.png', label: 'Camion' } },
}

const loaded: LoadedTheme = { body: brand, assetURL: (p) => `/t/brand/v1/assets/${p}` }

describe('themeCSS', () => {
  const css = themeCSS(loaded)
  it('sets the theme tokens on decks that name the theme, and only those', () => {
    expect(css).toContain('.ktw-deck[data-deck-theme="brand"] { --deck-accent: #FF2822; --deck-heading-font: Skeena, Arial, sans-serif; }')
  })
  it('sets the dark tokens under the dark scheme', () => {
    expect(css).toContain(`.ktw-deck[data-deck-theme="brand"][data-scheme='dark'] { --deck-bg: #111; }`)
  })
  it("a theme with chrome owns the slide number — its own or none — so reveal.js's is hidden", () => {
    // brand has chrome (a logo, a footer) and no slide number: none is shown.
    expect(css).toContain('.ktw-deck[data-deck-theme="brand"] .reveal .slide-number { display: none !important; }')
    const bare = themeCSS({ body: { name: 'bare', description: '', tokens: { accent: 'red' } }, assetURL: (p) => p })
    expect(bare).not.toContain('.slide-number')
  })
  it('declares the fonts from their files', () => {
    expect(css).toContain('@font-face { font-family: "Skeena"; src: url("/t/brand/v1/assets/fonts/skeena.woff2") format("woff2"); font-weight: 700;')
  })
  it("scopes a layout's default look and a variant's slot boxes, in % of the theme's slide", () => {
    expect(css).toContain('.deck-slide[data-layout="cover"] { --deck-heading: #FFFFFF; }')
    expect(css).toContain('.deck-slide[data-layout="content"][data-variant="wide"] .deck-slot[data-slot="right"] { position: absolute; grid-area: auto; left: 50%; top: 10%; width: 44.6875%; height: 80%; margin: 0; }')
    expect(css).toContain('.deck-slide[data-layout="content"][data-variant="wide"] .deck-title { position: absolute;')
  })
  it('drops what could escape the deck: bad token names, values closing a rule, bad names', () => {
    const evil = themeCSS({
      body: { name: 'x', description: '', tokens: { 'Bad Name': 'red', ok: 'red; } body { display: none' }, layouts: { '} body {': {} } },
      assetURL: (p) => p,
    })
    expect(evil).toBe('')
    expect(themeCSS({ body: { name: '"]{}', description: '' }, assetURL: (p) => p })).toBe('')
  })
})

describe('slideLook', () => {
  it("a layout's default look: its background, its chrome setting — a hidden chrome takes its logo with it", () => {
    expect(slideLook(loaded, 'cover')).toEqual({
      background: { image: '/t/brand/v1/assets/bg/bordeaux.jpg' }, chrome: false, logo: undefined,
    })
  })
  it("a look's own logo shows even where the chrome is hidden (a cover's white logo)", () => {
    const white = { asset: 'logo-white.png', box: { x: 80, y: 549, w: 351, h: 148 } }
    const t: LoadedTheme = {
      body: { ...brand, layouts: { cover: { chrome: false, default: { logo: white } } } },
      assetURL: (p) => p,
    }
    expect(slideLook(t, 'cover')).toEqual({ background: undefined, chrome: false, logo: white })
  })
  it('a variant overrides the background, and may bring the chrome back with its own logo', () => {
    expect(slideLook(loaded, 'cover', 'sand').background).toEqual({ image: '/t/brand/v1/assets/bg/sand.jpg' })
    const plain = slideLook(loaded, 'cover', 'plain')
    expect(plain.background).toEqual({ colour: '#F2EBE2' })
    expect(plain.chrome).toBe(true)
    expect(plain.logo?.asset).toBe('logo-white.png')
  })
  it('a variant the theme does not declare falls back to the default look', () => {
    expect(slideLook(loaded, 'cover', 'bordeaux')).toEqual(slideLook(loaded, 'cover'))
  })
  it('a layout the theme does not mention shows the chrome and no background', () => {
    expect(slideLook(loaded, 'two-cols')).toEqual({ background: undefined, chrome: true, logo: brand.chrome!.logo })
  })
  it('no chrome at all when the theme declares none', () => {
    expect(slideLook({ body: { name: 'bare', description: '' }, assetURL: (p) => p }, 'content').chrome).toBe(false)
  })
})

describe('helpers', () => {
  it('fills the chrome texts', () => {
    expect(chromeText('{title} · {n}/{total}', { title: 'Rapport', n: 3, total: 12 })).toBe('Rapport · 3/12')
    // The deck's author (its `author` setting), and nothing when it has none.
    expect(chromeText('{title} — {author}', { title: 'Rapport', author: 'Ann', n: 1, total: 2 })).toBe('Rapport — Ann')
    expect(chromeText('{author}', { title: 'Rapport', n: 1, total: 2 })).toBe('')
  })
  it('finds an icon file', () => {
    expect(iconAsset(brand, 'camion')).toBe('icons/camion.png')
    expect(iconAsset(brand, 'grue')).toBeUndefined()
  })
  it('tells a colour from a file, as the Go side does', () => {
    for (const [bg, want] of [['#C8B39F', true], ['white', true], ['rgb(1, 2, 3)', true], ['bg/sand.jpg', false], ['sand.png', false]] as const) {
      expect(isColour(bg)).toBe(want)
    }
  })
  it('turns boxes into % of the slide', () => {
    expect(boxStyle({ x: 0, y: 360, w: 1280, h: 360 })).toEqual({ left: '0%', top: '50%', width: '100%', height: '50%' })
    expect(boxStyle({ x: 512, y: 0, w: 512, h: 768 }, '4:3')).toEqual({ left: '50%', top: '0%', width: '50%', height: '100%' })
  })
})

describe('allowsColour', () => {
  it('allows any colour without a palette, only its own colours with one', () => {
    expect(allowsColour(undefined, '#0000ff')).toBe(true)
    expect(allowsColour({ name: 'x', description: '' }, '#0000ff')).toBe(true)
    const charter = { name: 'c', description: '', palette: [{ name: 'Rouge cavalier', value: '#82072A' }] }
    expect(allowsColour(charter, '#82072a')).toBe(true)
    expect(allowsColour(charter, '#0000ff')).toBe(false)
  })
})

describe('deckWatermark', () => {
  it('the default mark is katyoti.yaml watermark', async () => {
    const { parse } = await import('yaml')
    const { readFileSync } = await import('node:fs')
    const { resolve } = await import('node:path')
    const yaml = parse(readFileSync(resolve(import.meta.dir, '../../../../../internal/extensions/slidedeck/contrib/themes/deck-themes/katyoti.yaml'), 'utf8'))
    expect(DEFAULT_WATERMARK).toEqual(yaml.watermark)
  })
  it('takes the theme mark and text, which a deck changes', () => {
    const loaded = { body: { name: 'b', description: '', watermark: { mark: { template: 'Made', box: { x: 0, y: 0, w: 1, h: 1 } }, text: 'INTERNE' } }, assetURL: (p: string) => p }
    expect(deckWatermark(loaded, {})).toEqual({ text: 'INTERNE', mark: loaded.body.watermark.mark, logo: undefined })
    expect(deckWatermark(loaded, { watermark: 'BROUILLON' }).text).toBe('BROUILLON')
    expect(deckWatermark(loaded, { watermark: 'none' }).text).toBeUndefined()
    expect(deckWatermark(loaded, { mark: 'off' }).mark).toBeUndefined()
    expect(deckWatermark(undefined, {}).mark).toEqual(DEFAULT_WATERMARK.mark)
    expect(deckWatermark({ body: { name: 'x', description: '' }, assetURL: (p: string) => p }, {})).toEqual({ text: undefined, mark: undefined, logo: undefined })
  })
  it('draws the production mark ArchiFactory wrote in at the theme place, unless the deck switches it off', () => {
    const loaded = { body: { name: 'b', description: '', watermark: { mark: { template: 'Made', box: { x: 0, y: 0, w: 1, h: 1 } } } }, assetURL: (p: string) => p }
    const line = 'Une production ArchiFactory · 2026.10 · 11/10/2026'
    expect(deckWatermark(loaded, { production_mark: line }).mark).toEqual({ template: line, box: { x: 0, y: 0, w: 1, h: 1 } })
    expect(deckWatermark(undefined, { production_mark: line }).mark?.box).toEqual(DEFAULT_WATERMARK.mark?.box)
    expect(deckWatermark(loaded, { production_mark: line, mark: 'off' }).mark).toBeUndefined()
    expect(deckWatermark({ body: { name: 'x', description: '' }, assetURL: (p: string) => p }, { production_mark: line }).mark).toBeUndefined()
  })
})

describe('deckAspect', () => {
  it('takes the deck setting, else the theme aspect, else 16:9', () => {
    const t43 = { name: 't', description: '', aspect: '4:3' as const }
    expect(deckAspect({ aspect: '16:9' }, t43)).toBe('16:9')
    expect(deckAspect({}, t43)).toBe('4:3')
    expect(deckAspect({ aspect: 'bogus' }, t43)).toBe('4:3')
    expect(deckAspect({}, { name: 't', description: '' })).toBe('16:9')
    expect(deckAspect({})).toBe('16:9')
  })
})

// SPDX-License-Identifier: Apache-2.0
// Deck themes, the component's half (modules/slidedeck/, OD-008…OD-014).
//
// A theme is a declaration (the generated `Theme` type, from
// internal/extensions/slidedeck/api ThemeBody) plus the files it names. The
// component turns it into:
//
//   - CSS (themeCSS): the theme's token overrides on the deck root, dark
//     overrides under [data-scheme='dark'], each layout's default look and
//     variants on [data-layout][data-variant] (their tokens and slot boxes),
//     and the theme's fonts;
//   - per-slide decisions (slideLook): which background, whether the chrome
//     shows, which logo — a variant the theme does not declare falls back to
//     the layout's default look;
//   - the chrome's text (chromeText) and the icon files (iconAsset).
//
// Pure functions: no DOM, tested by bun. katyoti is deck.css's own defaults
// and needs no theme at all.

import type { Theme } from './generated/deck-theme.ts'

export type { Theme }
type Layouts = NonNullable<Theme['layouts']>
type ThemeLayout = Layouts[string]
type ThemeVariant = NonNullable<ThemeLayout['variants']>[string]
export type ThemeImage = NonNullable<ThemeVariant['logo']>
export type Box = ThemeImage['box']

/** A theme ready for a deck: its declaration and where its files are. */
export interface LoadedTheme {
  body: Theme
  /** The URL of a file the declaration names (a path in the theme's folder). */
  assetURL: (path: string) => string
}

export const DEFAULT_THEME = 'katyoti'

/** The slide size a theme's boxes are drawn in. */
export const THEME_SIZES: Record<string, { width: number; height: number }> = {
  '16:9': { width: 1280, height: 720 },
  '4:3': { width: 1024, height: 768 },
}

/** A deck's aspect: its own setting, else the one its theme is drawn for, else 16:9. */
export function deckAspect(settings: Record<string, unknown>, theme?: Theme): string {
  if (typeof settings.aspect === 'string' && THEME_SIZES[settings.aspect]) return settings.aspect
  if (theme?.aspect && THEME_SIZES[theme.aspect]) return theme.aspect
  return '16:9'
}

const COLOUR = /^(#[0-9a-fA-F]{3,8}|(rgb|rgba|hsl|hsla)\(.*\)|[a-zA-Z]+)$/

/** A background is a colour or a file; the same rule as the Go side's IsColour. */
export function isColour(background: string): boolean {
  const b = background.trim()
  return COLOUR.test(b) && !/[./]/.test(b)
}

/** Whether a deck in this theme may set colour c: any colour without a
 *  palette, else one of the palette's (the lint's theme-palette rule). */
export function allowsColour(theme: Theme | undefined, c: string): boolean {
  const palette = theme?.palette ?? []
  if (palette.length === 0) return true
  const v = c.trim().toLowerCase()
  return palette.some((p) => p.value.trim().toLowerCase() === v)
}

const TOKEN = /^[a-z0-9]+(-[a-z0-9]+)*$/
const NAME = TOKEN
// A value may not close a declaration or a rule: a theme styles its deck and
// nothing else.
const SAFE_VALUE = /^[^;{}<>]*$/

function declarations(tokens: Record<string, string> | undefined): string {
  if (!tokens) return ''
  return Object.entries(tokens)
    .filter(([k, v]) => TOKEN.test(k) && typeof v === 'string' && SAFE_VALUE.test(v))
    .map(([k, v]) => `--deck-${k}: ${v};`)
    .join(' ')
}

function pct(n: number, of: number): string {
  return `${+((n / of) * 100).toFixed(4)}%`
}

/** A box as CSS left/top/width/height, in % of the theme's slide. */
export function boxStyle(box: Box, aspect?: string): { left: string; top: string; width: string; height: string } {
  const s = THEME_SIZES[aspect ?? '16:9'] ?? THEME_SIZES['16:9']
  return { left: pct(box.x, s.width), top: pct(box.y, s.height), width: pct(box.w, s.width), height: pct(box.h, s.height) }
}

function slotRules(sel: string, slots: Record<string, Box> | undefined, aspect?: string): string[] {
  if (!slots) return []
  const out: string[] = []
  for (const [slot, box] of Object.entries(slots)) {
    if (!(slot === 'title' || NAME.test(slot)) || !box) continue
    const b = boxStyle(box, aspect)
    // `title` is the slide's title region; any other name is a slot. The grid
    // placement is reset: an absolute grid item placed in an area is boxed by
    // that area, not by the slide.
    const target = slot === 'title' ? '.deck-title' : `.deck-slot[data-slot="${slot}"]`
    out.push(`${sel} ${target} { position: absolute; grid-area: auto; left: ${b.left}; top: ${b.top}; width: ${b.width}; height: ${b.height}; margin: 0; }`)
  }
  if (out.length) out.unshift(`${sel} { position: relative; }`)
  return out
}

function fontFormat(src: string): string {
  const ext = src.slice(src.lastIndexOf('.') + 1).toLowerCase()
  return ({ woff2: 'woff2', woff: 'woff', ttf: 'truetype', otf: 'opentype' } as Record<string, string>)[ext] ?? ext
}

/** The theme's CSS, scoped to decks that name it. */
export function themeCSS(t: LoadedTheme): string {
  const { body, assetURL } = t
  if (!NAME.test(body.name)) return ''
  const root = `.ktw-deck[data-deck-theme="${body.name}"]`
  const rules: string[] = []
  for (const f of body.fonts ?? []) {
    if (!f.family || !f.src || !SAFE_VALUE.test(f.family)) continue
    rules.push(
      `@font-face { font-family: "${f.family.replace(/"/g, '')}"; src: url("${assetURL(f.src)}") format("${fontFormat(f.src)}");` +
        ` font-weight: ${/^\d{3}( \d{3})?$/.test(f.weight ?? '') ? f.weight : 400}; font-style: ${f.style === 'italic' ? 'italic' : 'normal'}; font-display: swap; }`,
    )
  }
  const light = declarations(body.tokens)
  if (light) rules.push(`${root} { ${light} }`)
  const dark = declarations(body.dark)
  if (dark) rules.push(`${root}[data-scheme='dark'] { ${dark} }`)
  // A theme with chrome owns the slide number: it places its own
  // (chrome.slideNumber) or none, and reveal.js's is hidden either way.
  if (body.chrome) rules.push(`${root} .reveal .slide-number { display: none !important; }`)
  for (const [layout, entry] of Object.entries(body.layouts ?? {})) {
    if (!NAME.test(layout) || !entry) continue
    const sel = `${root} .deck-slide[data-layout="${layout}"]`
    const def = declarations(entry.default?.tokens)
    if (def) rules.push(`${sel} { ${def} }`)
    rules.push(...slotRules(sel, entry.default?.slots, body.aspect))
    for (const [variant, look] of Object.entries(entry.variants ?? {})) {
      if (!NAME.test(variant) || !look) continue
      const vsel = `${sel}[data-variant="${variant}"]`
      const v = declarations(look.tokens)
      if (v) rules.push(`${vsel} { ${v} }`)
      rules.push(...slotRules(vsel, look.slots, body.aspect))
    }
  }
  return rules.join('\n')
}

/** How one slide looks under a theme. */
export interface SlideLook {
  /** A colour, or the URL of an image, behind the slide; none when the theme sets none. */
  background?: { colour: string } | { image: string }
  /** The theme's chrome (rules, footer, number, its logo) shows on the slide. */
  chrome: boolean
  /** The logo on the slide: the look's own (the variant's, else the default
   *  look's) — shown even where the chrome is hidden, as a cover's is — else
   *  the chrome's when the chrome shows. */
  logo?: ThemeImage
}

/** The look of a slide of `layout`, picking `variant` when the theme declares it. */
export function slideLook(t: LoadedTheme, layout: string, variant?: string): SlideLook {
  const entry = t.body.layouts?.[layout]
  const def = entry?.default
  const v = variant ? entry?.variants?.[variant] : undefined
  const bg = v?.background || def?.background
  const background = bg ? (isColour(bg) ? { colour: bg } : { image: t.assetURL(bg) }) : undefined
  const chrome = !!t.body.chrome && (v?.chrome ?? def?.chrome ?? entry?.chrome ?? true)
  const logo = v?.logo ?? def?.logo ?? (chrome ? t.body.chrome?.logo : undefined)
  return { background, chrome, logo }
}

/** A chrome text with {title}, {n} and {total} filled in. */
export function chromeText(template: string, vars: { title: string; n: number; total: number; author?: string }): string {
  return template.replace(/\{title\}/g, vars.title).replace(/\{author\}/g, vars.author ?? '')
    .replace(/\{n\}/g, String(vars.n)).replace(/\{total\}/g, String(vars.total))
}

/** The file of icon `name`, when the theme has it. */
export function iconAsset(t: Theme | undefined, name: string): string | undefined {
  return t?.icons?.[name]?.asset
}

/** The default theme's production mark: katyoti.yaml's `watermark`, which a
 *  test holds equal (the default theme is never loaded; its values are the
 *  component's own). */
export const DEFAULT_WATERMARK: NonNullable<Theme['watermark']> = {
  mark: { template: 'Une production KaTyoti', box: { x: 440, y: 686, w: 400, h: 24 }, size: '12px', align: 'center', token: 'muted' },
}

/** What marks a deck's slides: the watermark text across each slide and the
 *  production mark — the theme's, its text the deck's `production_mark:` when
 *  ArchiFactory wrote one, changed by the deck's `watermark:` (a text,
 *  or `none`) and `mark:` (`off` removes it). */
export interface DeckWatermark {
  text?: string
  mark?: NonNullable<Theme['watermark']>['mark']
  logo?: NonNullable<Theme['watermark']>['logo']
}

export function deckWatermark(theme: LoadedTheme | undefined, settings: Record<string, unknown>): DeckWatermark {
  const own = theme ? theme.body.watermark : DEFAULT_WATERMARK
  const asked = typeof settings.watermark === 'string' ? settings.watermark.trim() : undefined
  const text = asked === undefined ? own?.text : asked.toLowerCase() === 'none' ? undefined : asked || undefined
  const markOn = String(settings.mark ?? 'on').toLowerCase() !== 'off'
  // The production mark's line ArchiFactory wrote in (docs ProductionMark,
  // deckapi.MarkDeck) replaces the theme's text; the theme keeps its place.
  const line = typeof settings.production_mark === 'string' ? settings.production_mark.trim() : ''
  const mark = markOn && own?.mark ? (line ? { ...own.mark, template: line } : own.mark) : undefined
  return { text, mark, logo: markOn ? own?.logo : undefined }
}

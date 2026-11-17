// SPDX-License-Identifier: Apache-2.0
// No light box on the dark theme. A callout and a box with a
// light `bg` are rendered, their background resolved as the dark scheme
// paints it (the `light-dark()` dark branch, Mantine's dark CSS variables
// over the dark body) and checked against the dark body text: WCAG AA
// (4.5:1), and in the light scheme too.

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { DEFAULT_THEME, MantineProvider } from '@mantine/core'

import { UIPageView } from '../src/ui-builders/page/ui-page.view.tsx'
import { QuickActionButton } from '../src/ui-core/action/quick-actions/QuickActionButton.tsx'

type Scheme = 'light' | 'dark'
type RGBA = [number, number, number, number]

const page = (node: unknown) => renderToStaticMarkup(
  createElement(MantineProvider, null, createElement(UIPageView, { node: node as never })))

// The background the rendered box declares (the first div painting one).
function boxBackground(html: string): string {
  const bg = /<div[^>]*style="[^"]*background:\s*([^;"]+)/.exec(html.replace(/<style[^>]*>[\s\S]*?<\/style>/g, ''))
  if (!bg) throw new Error('the box has no background: ' + html)
  return bg[1].trim()
}

const hex = (h: string): RGBA => {
  const n = parseInt(h.replace('#', ''), 16)
  return [(n >> 16) & 255, (n >> 8) & 255, n & 255, 1]
}

// Mantine's CSS variables as the scheme defines them (default theme,
// primary shade 6 light / 8 dark: `-light` is shade 6 at 10 % / 15 %).
function variable(name: string, scheme: Scheme): RGBA {
  if (name === 'body') return scheme === 'dark' ? hex(DEFAULT_THEME.colors.dark[7]) : hex('#ffffff')
  if (name === 'text') return scheme === 'dark' ? hex(DEFAULT_THEME.colors.dark[0]) : hex('#000000')
  const shade = /^([a-z]+)-(\d)$/.exec(name)
  if (shade) return hex((DEFAULT_THEME.colors as any)[shade[1]][Number(shade[2])])
  // `-light-color`: shade 6 in the light scheme, shade 3 (8 - 5) in the dark.
  const lightColor = /^([a-z]+)-light-color$/.exec(name)
  if (lightColor) return hex((DEFAULT_THEME.colors as any)[lightColor[1]][scheme === 'dark' ? 3 : 6])
  const light = /^([a-z]+)-light$/.exec(name)
  if (light) {
    const [r, g, b] = hex((DEFAULT_THEME.colors as any)[light[1]][6])
    return [r, g, b, scheme === 'dark' ? 0.15 : 0.1]
  }
  throw new Error('unknown variable ' + name)
}

// Resolve a CSS background value in a scheme: light-dark() picks its branch.
function resolve(css: string, scheme: Scheme): RGBA {
  const ld = /^light-dark\((.*)\)$/.exec(css)
  if (ld) {
    let depth = 0, cut = -1
    for (let i = 0; i < ld[1].length; i++) {
      const ch = ld[1][i]
      if (ch === '(') depth++
      else if (ch === ')') depth--
      else if (ch === ',' && depth === 0) { cut = i; break }
    }
    return resolve((scheme === 'light' ? ld[1].slice(0, cut) : ld[1].slice(cut + 1)).trim(), scheme)
  }
  const v = /^var\(--mantine-color-([a-z0-9-]+)\)$/.exec(css)
  if (v) return variable(v[1], scheme)
  if (/^#[0-9a-f]{6}$/i.test(css)) return hex(css)
  if (/^#[0-9a-f]{3}$/i.test(css)) return hex('#' + [...css.slice(1)].map((c) => c + c).join(''))
  throw new Error('cannot resolve ' + css)
}

const over = ([r, g, b, a]: RGBA, [R, G, B]: RGBA): RGBA => [r * a + R * (1 - a), g * a + G * (1 - a), b * a + B * (1 - a), 1]
const lum = ([r, g, b]: RGBA) => {
  const c = (x: number) => { x /= 255; return x <= 0.03928 ? x / 12.92 : ((x + 0.055) / 1.055) ** 2.4 }
  return 0.2126 * c(r) + 0.7152 * c(g) + 0.0722 * c(b)
}
function contrast(css: string, scheme: Scheme): number {
  const bg = over(resolve(css, scheme), variable('body', scheme))
  const [a, b] = [lum(bg), lum(variable('text', scheme))]
  return (Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05)
}

const TONES = ['yellow', 'blue', 'orange', 'red', 'green', 'gray', 'violet', 'grape', 'teal', 'cyan']

describe('no light box on the dark theme', () => {
  test('a callout keeps the body text readable in both schemes', () => {
    for (const tone of TONES) {
      const bg = boxBackground(page({ kind: 'box', callout: tone, children: [{ kind: 'text', content: 'Aperçu - données fictives' }] }))
      expect({ tone, dark: contrast(bg, 'dark') >= 4.5 }).toEqual({ tone, dark: true })
      expect({ tone, light: contrast(bg, 'light') >= 4.5 }).toEqual({ tone, light: true })
    }
  })

  test('a box whose bg is a light shade is not light in the dark scheme', () => {
    for (const bg of ['yellow.0', 'blue.1', 'gray.0', 'orange.2', 'white']) {
      const css = boxBackground(page({ kind: 'box', bg, children: [] }))
      expect({ bg, dark: contrast(css, 'dark') >= 4.5 }).toEqual({ bg, dark: true })
    }
  })

  test('a mid or dark shade and a CSS variable are left as given', () => {
    expect(boxBackground(page({ kind: 'box', bg: 'orange.6', children: [] }))).toBe('var(--mantine-color-orange-6)')
    expect(boxBackground(page({ kind: 'box', bg: 'var(--mantine-color-blue-light)', children: [] }))).toBe('var(--mantine-color-blue-light)')
  })
})

// The quick-action button's 48 px icon tile follows the
// scheme too - its light shade in the light scheme, the colour's dark tint in
// the dark one, enabled or disabled.
const texts = { t: (_k: string, o?: { defaultValue?: string }) => o?.defaultValue ?? '', language: 'en', withNamespace: () => texts } as never

function quickActionTile(color: string, disabled: boolean): string {
  const html = renderToStaticMarkup(createElement(MantineProvider, null,
    createElement(QuickActionButton, { action: { id: 'go', scope: 'global', label: 'Go', icon: 'plus', color } as never, onClick: () => {}, texts, disabled })))
  const bg = /width:\s*48px;[^"]*background-color:\s*([^;"]+)/.exec(html.replace(/<style[^>]*>[\s\S]*?<\/style>/g, ''))
  if (!bg) throw new Error('the tile has no background: ' + html)
  return bg[1].trim()
}

describe('the quick-action icon tile is no light square on the dark theme', () => {
  test('the tile keeps the body text readable in the dark scheme and its light shade in the light one', () => {
    for (const color of ['blue', 'green', 'red', 'gray', 'violet', 'dark']) {
      for (const disabled of [false, true]) {
        const css = quickActionTile(color, disabled)
        expect({ color, disabled, dark: contrast(css, 'dark') >= 4.5 }).toEqual({ color, disabled, dark: true })
        expect({ color, disabled, light: resolve(css, 'light') }).toEqual({ color, disabled, light: hex((DEFAULT_THEME.colors as any)[disabled ? 'gray' : color][disabled ? 1 : 0]) })
      }
    }
  })
})

// The enabled icon keeps 3:1 on its tile in both schemes, in
// every colour of the theme (the `dark` palette's included).
function quickActionIcon(color: string): { tile: string, icon: string } {
  const html = renderToStaticMarkup(createElement(MantineProvider, null,
    createElement(QuickActionButton, { action: { id: 'go', scope: 'global', label: 'Go', icon: 'plus', color } as never, onClick: () => {}, texts })))
  const m = /width:\s*48px;[^"]*background-color:\s*([^;"]+);\s*color:\s*([^;"]+)/.exec(html.replace(/<style[^>]*>[\s\S]*?<\/style>/g, ''))
  if (!m) throw new Error('the tile has no icon colour: ' + html)
  return { tile: m[1].trim(), icon: m[2].trim() }
}

function iconContrast(color: string, scheme: Scheme): number {
  const { tile, icon } = quickActionIcon(color)
  const bg = over(resolve(tile, scheme), variable('body', scheme))
  const [a, b] = [lum(bg), lum(over(resolve(icon, scheme), bg))]
  return (Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05)
}

describe('the quick-action icon is readable on its tile', () => {
  test('the enabled icon keeps 3:1 on its tile in both schemes, in every colour', () => {
    for (const color of Object.keys(DEFAULT_THEME.colors)) {
      for (const scheme of ['light', 'dark'] as Scheme[]) {
        const ratio = iconContrast(color, scheme)
        expect({ color, scheme, readable: ratio >= 3 }).toEqual({ color, scheme, readable: true })
      }
    }
  })
})

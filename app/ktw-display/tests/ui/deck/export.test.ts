// SPDX-License-Identifier: Apache-2.0
// Deck export to PowerPoint as pictures (src/deck-export.ts): the file made
// from captured slides. The capture itself runs a browser and is checked live.

import { describe, expect, it } from 'bun:test'
import JSZip from 'jszip'
import { browserCandidates, noBrowserMessage, picturesToPptx } from '../../../src/deck-export.ts'

// A 1×1 PNG.
const PNG = 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=='

describe('picturesToPptx', () => {
  it('makes one slide per picture, with its notes, at the deck aspect', async () => {
    const bytes = await picturesToPptx([PNG, PNG], { aspect: '16:9', title: 'Deck', notes: ['say this', undefined] })
    const zip = await JSZip.loadAsync(bytes)
    const slides = Object.keys(zip.files).filter((f) => /^ppt\/slides\/slide\d+\.xml$/.test(f))
    expect(slides).toHaveLength(2)
    const notes = await zip.file('ppt/notesSlides/notesSlide1.xml')!.async('string')
    expect(notes).toContain('say this')
    const pres = await zip.file('ppt/presentation.xml')!.async('string')
    expect(pres).toContain('cx="12192000"') // 13.333 in: 16:9
    const media = Object.keys(zip.files).filter((f) => f.startsWith('ppt/media/'))
    expect(media.length).toBeGreaterThanOrEqual(1)
  })
  it('takes the 4:3 page for a 4:3 deck', async () => {
    const zip = await JSZip.loadAsync(await picturesToPptx([PNG], { aspect: '4:3' }))
    expect(await zip.file('ppt/presentation.xml')!.async('string')).toContain('cx="9144000"') // 10 in
  })
})

describe('noBrowserMessage', () => {
  it('names what is missing, where it looked and how to provide it', () => {
    const before = Bun.env.KTW_BROWSER
    Bun.env.KTW_BROWSER = '/nowhere/edge.exe'
    try {
      const msg = noBrowserMessage()
      expect(msg).toContain('no Chromium browser')
      expect(msg).toContain('KTW_BROWSER=/nowhere/edge.exe (no such file)')
      for (const c of browserCandidates().slice(1)) expect(msg).toContain(c)
      expect(msg).toContain('Install one of them, or set KTW_BROWSER')
    } finally {
      if (before === undefined) delete Bun.env.KTW_BROWSER
      else Bun.env.KTW_BROWSER = before
    }
  })
})

describe('standalonePage', () => {
  it('holds the presenter and the deck, and no closing script tag escapes', async () => {
    const { standalonePage } = await import('../../../src/deck-export.ts')
    const md = '---\ntitle: T </script> x\n---\n\n## One'
    const { html, error } = standalonePage(md)
    expect(error).toBeUndefined()
    expect(html.startsWith('<!doctype html>')).toBe(true)
    expect(html).toContain('<title>T &lt;/script&gt; x</title>')
    const json = html.slice(html.indexOf('id="ktw-deck">') + 14, html.indexOf('</script>', html.indexOf('id="ktw-deck">')))
    expect(JSON.parse(json.split('<\\/').join('</')).markdown).toBe(md)
  })
  it('embeds only the theme files the deck uses, and the fonts licences', async () => {
    const { standalonePage } = await import('../../../src/deck-export.ts')
    const { putTheme } = await import('../../../src/theme-store.ts')
    const bytes = (s: string) => new TextEncoder().encode(s) as Uint8Array<ArrayBuffer>
    putTheme({
      name: 'brand', version: 'v1',
      body: {
        name: 'brand', description: 'b',
        fonts: [{ family: 'F', src: 'f.woff2' }], notices: ['ofl.txt'],
        icons: { rocket: { asset: 'icons/rocket.png', label: 'R' }, flower: { asset: 'icons/flower.png', label: 'F' } },
      },
      assets: new Map([['f.woff2', bytes('font')], ['ofl.txt', bytes('SIL Open Font License')], ['icons/rocket.png', bytes('r')], ['icons/flower.png', bytes('f')]]),
    })
    const { html } = standalonePage('---\ntitle: T\ntheme: brand\n---\n\n## A\n![R](icon:rocket)')
    expect(html).toContain('"icons/rocket.png":"data:image/png;base64,')
    expect(html).not.toContain('"icons/flower.png":"data:')
    expect(html).toContain('"f.woff2":"data:font/woff2;base64,')
    expect(html).toContain('SIL Open Font License')
    expect(standalonePage('---\ntitle: T\ntheme: nowhere\n---\n\n## A').error).toContain('publish it first')
  })
})

describe('modelsToPptx (editable slides)', () => {
  it('maps screen fonts to the theme Office fonts', async () => {
    const { officeFont } = await import('../../../src/deck-pptx.ts')
    const theme = { name: 's', description: '', tokens: { 'heading-font': "'Proza Libre', Skeena", font: "'Red Hat Text', Aptos", 'office-heading-font': 'Skeena', 'office-font': 'Aptos' } }
    expect(officeFont('Proza Libre', theme)).toBe('Skeena')
    expect(officeFont('Red Hat Text', theme)).toBe('Aptos')
    expect(officeFont('Inter', theme)).toBe('Inter')
    expect(officeFont('Proza Libre', undefined)).toBe('Proza Libre')
  })
  it('makes text boxes, shapes and a background from a measured slide', async () => {
    const { modelsToPptx } = await import('../../../src/deck-pptx.ts')
    const run = { text: 'Bonjour', bold: true, italic: false, color: '82072A', font: 'Proza Libre', size: 40, underline: false }
    const bytes = await modelsToPptx([{
      width: 1280, height: 720, background: { color: 'C8B39F' },
      items: [
        { kind: 'rect', x: 68, y: 72, w: 1144, h: 1, fill: '000000', alpha: 1, radius: 0 },
        { kind: 'text', x: 68, y: 100, w: 400, h: 50, align: 'left', lineHeight: 50, lines: 1, runs: [run] },
      ],
    }], { theme: { name: 's', description: '', tokens: { 'heading-font': 'Proza Libre', 'office-heading-font': 'Skeena' } } })
    const zip = await JSZip.loadAsync(bytes)
    const slide = await zip.file('ppt/slides/slide1.xml')!.async('string')
    expect(slide).toContain('Bonjour')
    expect(slide).toContain('typeface="Skeena"')
    expect(slide).toContain('82072A')
    expect(slide).toContain('C8B39F')
    expect(slide).toContain('prstGeom prst="rect"')
  })
})

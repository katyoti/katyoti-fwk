// SPDX-License-Identifier: Apache-2.0
// Deck export to PowerPoint as real slides (modules/slidedeck/, th12 level 2).
//
// The deck page lays every slide out (its layouts, its theme); this module
// MEASURES that layout and rebuilds it with PptxGenJS, so the PowerPoint
// follows deck.css and any theme with no second geometry to keep in step:
//
//   - every run of text becomes a text box at its place, with its font, size,
//     weight, style, colour, capitals, alignment, line height and bullet —
//     editable in PowerPoint;
//   - a pictogram becomes a picture recoloured as the theme shows it; an
//     image, a picture of its file;
//   - a filled box, a rule or a border becomes a shape;
//   - the slide's background colour or image becomes the slide's background.
//
// The fonts are the theme's Office ones when it names them (tokens
// `office-heading-font`, `office-font`: the brand's Microsoft fonts, which
// PowerPoint fetches itself), else the screen font's first family.

import PptxGenJS from 'pptxgenjs'
import type { Theme } from '@ktw/display-lib/ui-builders/deck/theme.ts'

/** What the page reports for one slide, in the deck's own pixels. */
export interface SlideModel {
  width: number
  height: number
  background?: { color?: string; image?: string }
  items: Item[]
}

type Box = { x: number; y: number; w: number; h: number }
export type Item =
  | (Box & { kind: 'rect'; fill: string; alpha: number; radius: number })
  | (Box & { kind: 'image'; data: string; cover?: boolean })
  | (Box & { kind: 'chart'; spec: ChartSpec })
  | (Box & { kind: 'text'; align: string; lineHeight: number; lines: number; rotate?: number; runs: Run[] })
/** A chart as the page drew it (chart.tsx): its data and colours. */
export interface ChartSpec {
  kind: 'bar' | 'line' | 'pie'
  categories: string[]
  series: string[]
  values: number[][]
  colours: string[]
  pieColours: string[]
  text?: string
  font?: string
}
export interface Run { text: string; bold: boolean; italic: boolean; color: string; alpha?: number; font: string; size: number; underline: boolean; br?: boolean }

const hex = (c?: string) => {
  if (!c) return undefined
  const m = c.match(/rgba?\(([^)]+)\)/)
  if (m) return m[1]!.split(/[ ,/]+/).filter(Boolean).slice(0, 3).map((v) => Math.round(Number(v)).toString(16).padStart(2, '0')).join('').toUpperCase()
  const h = c.trim().replace(/^#/, '')
  return /^[0-9a-f]{6}$/i.test(h) ? h.toUpperCase() : /^[0-9a-f]{3}$/i.test(h) ? h.split('').map((x) => x + x).join('').toUpperCase() : undefined
}

/** A chart, rebuilt as a native PowerPoint chart: editable, its data in it. */
function addChart(pptx: PptxGenJS, slide: PptxGenJS.Slide, it: Box & { spec: ChartSpec }, theme?: Theme) {
  const { spec } = it
  const inch = (v: number) => v / PX
  const data = spec.series.map((name, j) => ({ name, labels: spec.categories, values: spec.values.map((r) => r[j] ?? 0) }))
  const text = hex(spec.text) ?? '333333'
  const font = officeFont(spec.font ?? '', theme)
  const type = spec.kind === 'pie' ? pptx.ChartType.pie : spec.kind === 'line' ? pptx.ChartType.line : pptx.ChartType.bar
  const colours = (spec.kind === 'pie' ? spec.pieColours : spec.colours).map((c) => hex(c) ?? '888888')
  slide.addChart(type, data, {
    x: inch(it.x), y: inch(it.y), w: inch(it.w), h: inch(it.h),
    chartColors: colours,
    showLegend: spec.kind === 'pie' || spec.series.length > 1, legendPos: 'b', legendColor: text, legendFontFace: font, legendFontSize: 12,
    catAxisLabelColor: text, valAxisLabelColor: text, catAxisLabelFontFace: font, valAxisLabelFontFace: font,
    catAxisLabelFontSize: 12, valAxisLabelFontSize: 12,
    ...(spec.kind === 'bar' ? { barDir: 'col' } : {}),
    ...(spec.kind === 'line' ? { lineSize: 3, lineDataSymbolSize: 8 } : {}),
    ...(spec.kind === 'pie' ? { showPercent: true, dataLabelColor: 'FFFFFF' } : {}),
  })
}

/** Run once in the page: pseudo-elements that carry content (a counter, a
 *  quote mark, an empty bar) become real elements with the same computed
 *  style, so they can be measured like the rest. */
export const MATERIALIZE_JS = `(() => {
  if (document.documentElement.dataset.ktwMaterialized) return
  document.documentElement.dataset.ktwMaterialized = '1'
  const style = document.createElement('style')
  style.textContent = '.ktw-nob::before{content:none!important}.ktw-noa::after{content:none!important}'
  document.head.appendChild(style)
  const fmt = (n, t) => t === 'decimal-leading-zero' ? String(n).padStart(2, '0') : t === 'upper-roman' ? ['I','II','III','IV','V','VI','VII','VIII','IX','X'][n-1] || String(n) : String(n)
  const jobs = []
  for (const el of document.querySelectorAll('.reveal .slides *')) {
    for (const which of ['::before', '::after']) {
      const ps = getComputedStyle(el, which)
      const c = ps.content
      if (!c || c === 'none' || c === 'normal' || ps.display === 'none') continue
      let text = ''
      const q = c.match(/^"(.*)"$/s) || c.match(/^'(.*)'$/s)
      if (q) text = q[1].replace(/\\\\"/g, '"')
      else {
        const m = c.match(/counter\\(\\s*[\\w-]+\\s*(?:,\\s*([\\w-]+))?\\s*\\)/)
        if (!m) continue
        const sibs = [...el.parentElement.children].filter((x) => x.tagName === el.tagName)
        text = fmt(sibs.indexOf(el) + 1, m[1])
      }
      const decl = []
      for (let i = 0; i < ps.length; i++) { const p = ps[i]; if (p !== 'content') decl.push(p + ':' + ps.getPropertyValue(p)) }
      jobs.push({ el, which, text, css: decl.join(';') })
    }
  }
  for (const j of jobs) {
    const span = document.createElement('span')
    span.setAttribute('style', j.css)
    span.textContent = j.text
    span.dataset.pseudo = j.which
    if (j.which === '::before') j.el.prepend(span); else j.el.append(span)
    j.el.classList.add(j.which === '::before' ? 'ktw-nob' : 'ktw-noa')
  }
})()`

/** Run per slide in the page: the current slide as a SlideModel. */
export const EXTRACT_JS = `(async () => {
  const r = document.querySelector('.reveal').revealDeck
  const cfg = r.getConfig()
  const W = cfg.width, H = cfg.height
  const base = document.querySelector('.reveal .slides').getBoundingClientRect()
  const scale = base.width / W
  const box = (rc) => ({ x: (rc.left - base.left) / scale, y: (rc.top - base.top) / scale, w: rc.width / scale, h: rc.height / scale })
  const rgba = (c) => { const m = c && c.match(/rgba?\\(([^)]+)\\)/); if (!m) return null; const p = m[1].split(/[ ,\\/]+/).filter(Boolean).map(Number); return { hex: p.slice(0, 3).map((v) => Math.round(v).toString(16).padStart(2, '0')).join('').toUpperCase(), a: p.length > 3 ? p[3] : 1 } }
  const toData = async (url) => { try { const b = await (await fetch(url)).blob(); return await new Promise((ok) => { const f = new FileReader(); f.onload = () => ok(f.result); f.readAsDataURL(b) }) } catch { return '' } }
  const urlOf = (css) => { const m = css && css.match(/url\\(["']?(.*?)["']?\\)/); return m ? m[1] : '' }
  const recolor = async (url, color, w, h) => {
    const img = new Image(); img.src = url
    try { await img.decode() } catch { return '' }
    const k = 3, c = document.createElement('canvas'); c.width = Math.max(1, Math.round(w * k)); c.height = Math.max(1, Math.round(h * k))
    const g = c.getContext('2d'); const s = Math.min(c.width / img.width, c.height / img.height)
    const dw = img.width * s, dh = img.height * s
    g.drawImage(img, (c.width - dw) / 2, (c.height - dh) / 2, dw, dh)
    g.globalCompositeOperation = 'source-in'; g.fillStyle = color; g.fillRect(0, 0, c.width, c.height)
    return c.toDataURL('image/png')
  }
  const rasterSVG = async (svg, w, h) => {
    const copy = svg.cloneNode(true)
    copy.setAttribute('xmlns', 'http://www.w3.org/2000/svg')
    const r = svg.getBoundingClientRect()
    copy.setAttribute('width', String(r.width)); copy.setAttribute('height', String(r.height))
    const img = new Image(); img.src = 'data:image/svg+xml;charset=utf-8,' + encodeURIComponent(new XMLSerializer().serializeToString(copy))
    try { await img.decode() } catch { return '' }
    const k = 2, c = document.createElement('canvas'); c.width = Math.max(1, Math.round(r.width / scale * k)); c.height = Math.max(1, Math.round(r.height / scale * k))
    c.getContext('2d').drawImage(img, 0, 0, c.width, c.height)
    try { return c.toDataURL('image/png') } catch { return '' }
  }
  const visible = (cs) => cs.display !== 'none' && cs.visibility !== 'hidden' && Number(cs.opacity) > 0.01
  const items = []
  const pending = []
  const sec = r.getCurrentSlide()
  const idx = r.getIndices()
  // The slide's background: its colour or image, as reveal.js draws it.
  let background
  const bg = r.getSlideBackground(idx.h, idx.v)
  if (bg) {
    const content = bg.querySelector('.slide-background-content') || bg
    const img = urlOf(getComputedStyle(content).backgroundImage)
    const col = rgba(getComputedStyle(bg).backgroundColor)
    if (img) background = { image: await toData(img) }
    else if (col && col.a > 0.01) background = { color: col.hex }
  }
  const textOf = (n, cs) => {
    let t = n.textContent.replace(/[\\s\\u00a0]+/g, (m) => m.includes('\\u00a0') ? '\\u00a0' : ' ')
    if (cs.textTransform === 'uppercase') t = t.toUpperCase()
    else if (cs.textTransform === 'lowercase') t = t.toLowerCase()
    return t
  }
  // The opacity an element is drawn with: its own and its ancestors'.
  const opacityOf = (el) => { let o = 1; for (let e = el; e && e !== sec.parentElement; e = e.parentElement) o *= Number(getComputedStyle(e).opacity); return o }
  const runOf = (text, el) => {
    const cs = getComputedStyle(el)
    const col = rgba(cs.color)
    return { text, bold: Number(cs.fontWeight) >= 600, italic: cs.fontStyle === 'italic', color: col ? col.hex : '000000', alpha: (col ? col.a : 1) * opacityOf(el), font: cs.fontFamily.split(',')[0].replace(/["']/g, '').trim(), size: parseFloat(cs.fontSize), underline: cs.textDecorationLine.includes('underline') }
  }
  // A rotated block (a watermark): its angle, from its transform matrix.
  const angleOf = (cs) => { const m = cs.transform && cs.transform.match(/matrix\\(([^)]+)\\)/); if (!m) return 0; const [a, b] = m[1].split(',').map(Number); return Math.round(Math.atan2(b, a) * 180 / Math.PI) }
  const walk = (el) => {
    const cs = getComputedStyle(el)
    if (!visible(cs)) return
    if (el.matches('aside.notes, .deck-toolbar, .deck-status, script, style')) return
    const rc = el.getBoundingClientRect()
    const sized = rc.width > 0.5 && rc.height > 0.5
    // A chart carries its data: the PowerPoint rebuilds it as a native chart.
    if (el.dataset && el.dataset.chart) {
      try { if (sized) items.push({ kind: 'chart', ...box(rc), spec: JSON.parse(el.dataset.chart) }) } catch {}
      return
    }
    // A diagram (mermaid, draw.io) is an SVG drawn in the page: a picture of
    // it, rendered at twice its size so it stays sharp when enlarged.
    if (el.classList.contains('deck-diagram')) {
      const svg = el.querySelector('svg')
      const mark = { kind: 'placeholder' }; items.push(mark)
      if (sized && svg) pending.push(rasterSVG(svg, rc.width / scale, rc.height / scale).then((d) => d && items.splice(items.indexOf(mark), 1, { kind: 'image', ...box(svg.getBoundingClientRect()), data: d })))
      return
    }
    if (el.tagName === 'IMG') {
      const mark = { kind: 'placeholder' }; items.push(mark)
      if (sized) pending.push(toData(el.currentSrc || el.src).then((d) => d && items.splice(items.indexOf(mark), 1, { kind: 'image', ...box(rc), data: d, cover: cs.objectFit === 'cover' })))
      return
    }
    if (el.classList.contains('deck-icon')) {
      const url = urlOf(cs.maskImage || cs.webkitMaskImage); const col = cs.backgroundColor
      const mark = { kind: 'placeholder' }; items.push(mark)
      if (sized && url) pending.push(recolor(url, col, rc.width / scale, rc.height / scale).then((d) => d && items.splice(items.indexOf(mark), 1, { kind: 'image', ...box(rc), data: d })))
      return
    }
    if (sized) {
      const fill = rgba(cs.backgroundColor)
      if (fill && fill.a > 0.01) items.push({ kind: 'rect', ...box(rc), fill: fill.hex, alpha: fill.a, radius: parseFloat(cs.borderTopLeftRadius) || 0 })
      for (const side of ['Left', 'Top', 'Right', 'Bottom']) {
        const w = parseFloat(cs['border' + side + 'Width']); const c = rgba(cs['border' + side + 'Color'])
        if (!(w > 0) || cs['border' + side + 'Style'] === 'none' || !c || c.a < 0.01) continue
        const b = box(rc)
        const r2 = side === 'Left' ? { x: b.x, y: b.y, w, h: b.h } : side === 'Right' ? { x: b.x + b.w - w, y: b.y, w, h: b.h } : side === 'Top' ? { x: b.x, y: b.y, w: b.w, h: w } : { x: b.x, y: b.y + b.h - w, w: b.w, h: w }
        items.push({ kind: 'rect', ...r2, fill: c.hex, alpha: c.a, radius: 0 })
      }
    }
    // Text: runs of inline content between block children.
    let seg = []
    let first = true
    const flush = () => {
      const nodes = seg; seg = []
      if (!nodes.length) return
      const runs = []
      const collect = (n) => {
        if (n.nodeType === 3) { const t = textOf(n, getComputedStyle(n.parentElement)); if (t) runs.push(runOf(t, n.parentElement)) }
        else if (n.nodeType === 1) {
          if (n.tagName === 'BR') { runs.push({ ...runOf('', n.parentElement), br: true }); return }
          const ncs = getComputedStyle(n); if (!visible(ncs)) return
          for (const k of n.childNodes) collect(k)
        }
      }
      for (const n of nodes) collect(n)
      if (runs.length) { runs[0].text = runs[0].text.replace(/^ +/, ''); const l = runs[runs.length - 1]; l.text = l.text.replace(/ +$/, '') }
      if (!runs.some((x) => x.text.trim())) return
      const range = document.createRange(); range.setStartBefore(nodes[0]); range.setEndAfter(nodes[nodes.length - 1])
      const tr = range.getBoundingClientRect()
      if (tr.width < 0.5 || tr.height < 0.5) return
      let b = box(tr)
      const lh = parseFloat(cs.lineHeight) || parseFloat(cs.fontSize) * 1.2
      // Rotated: its own size around the centre of what is drawn.
      const rotate = angleOf(cs)
      if (rotate) {
        const w = el.offsetWidth, h = el.offsetHeight, c = box(rc)
        b = { x: c.x + c.w / 2 - w / 2, y: c.y + c.h / 2 - h / 2, w, h }
      }
      // A list item's marker: its own text, in the marker's colour, left of the item.
      if (el.tagName === 'LI' && first && cs.listStyleType !== 'none') {
        const ms = getComputedStyle(el, '::marker')
        const m = rgba(ms.color)
        const lb = box(rc)
        const sibs = [...el.parentElement.children].filter((x) => x.tagName === 'LI')
        const mark = el.parentElement.tagName === 'OL' ? (sibs.indexOf(el) + 1) + '.' : '\u2022'
        items.push({ kind: 'text', x: lb.x - lh * 1.0, y: b.y, w: lh * 0.6, h: lh, align: 'right', lineHeight: lh, lines: 1,
          runs: [{ ...runOf(mark, el), color: m ? m.hex : runOf('', el).color, bold: false }] })
      }
      first = false
      const align = cs.textAlign === 'center' ? 'center' : cs.textAlign === 'right' || cs.textAlign === 'end' ? 'right' : cs.textAlign === 'justify' ? 'justify' : 'left'
      items.push({ kind: 'text', ...b, align, lineHeight: lh, lines: Math.max(1, Math.round(b.h / lh)), rotate: rotate || undefined, runs })
    }
    for (const n of el.childNodes) {
      if (n.nodeType === 3) { if (n.textContent) seg.push(n); continue }
      if (n.nodeType !== 1) continue
      if (n.tagName === 'BR') { seg.push(n); continue }
      const ncs = getComputedStyle(n)
      const inline = ncs.display === 'inline' && !n.classList.contains('deck-icon') && n.tagName !== 'IMG' && ncs.position !== 'absolute'
      if (inline) seg.push(n)
      else { flush(); walk(n) }
    }
    flush()
  }
  walk(sec)
  await Promise.all(pending)
  return { width: W, height: H, background, items: items.filter((i) => i.kind !== 'placeholder') }
})()`

/** The Office font for a family the page used: the theme's Office fonts
 *  stand for its screen fonts. */
export function officeFont(family: string, theme?: Theme): string {
  const t = (theme?.tokens ?? {}) as Record<string, string>
  const first = (stack?: string) => (stack ?? '').split(',')[0].replace(/["']/g, '').trim()
  const same = (a: string, b?: string) => !!b && a.trim().toLowerCase() === first(b).toLowerCase()
  if (t['office-heading-font'] && same(family, t['heading-font'])) return first(t['office-heading-font'])
  if (t['office-font'] && same(family, t['font'])) return first(t['office-font'])
  return family
}

const PX = 96 // deck pixels per inch: 1280 px = 13.333 in

/** The PowerPoint of real slides, from the models the page measured. */
export async function modelsToPptx(models: SlideModel[], opts: { title?: string; author?: string; notes?: (string | undefined)[]; theme?: Theme }): Promise<Uint8Array> {
  const pptx = new PptxGenJS()
  const first = models[0] ?? { width: 1280, height: 720 }
  pptx.defineLayout({ name: 'DECK', width: first.width / PX, height: first.height / PX })
  pptx.layout = 'DECK'
  if (opts.title) pptx.title = opts.title
  // The deck's `author`: the PowerPoint's own (File > Info).
  if (opts.author) pptx.author = opts.author
  const inch = (v: number) => v / PX
  const pt = (px: number) => Math.round(px * 0.75 * 10) / 10
  models.forEach((m, i) => {
    const slide = pptx.addSlide()
    if (m.background?.image) slide.background = { data: m.background.image.replace(/^data:/, '') }
    else if (m.background?.color) slide.background = { color: m.background.color }
    for (const it of m.items) {
      if (it.kind === 'rect') {
        // A full-slide white box is the page itself, not a shape.
        if (it.w >= m.width - 1 && it.h >= m.height - 1 && !m.background) { slide.background = { color: it.fill }; continue }
        slide.addShape(it.radius > 0 ? pptx.ShapeType.roundRect : pptx.ShapeType.rect, {
          x: inch(it.x), y: inch(it.y), w: inch(it.w), h: inch(it.h),
          fill: { color: it.fill, transparency: Math.round((1 - it.alpha) * 100) },
          line: { type: 'none' },
          ...(it.radius > 0 ? { rectRadius: Math.min(0.5, it.radius / Math.min(it.w, it.h)) } : {}),
        })
      } else if (it.kind === 'chart') {
        addChart(pptx, slide, it, opts.theme)
      } else if (it.kind === 'image') {
        slide.addImage({
          data: it.data.replace(/^data:/, ''),
          x: inch(it.x), y: inch(it.y), w: inch(it.w), h: inch(it.h),
          ...(it.cover ? { sizing: { type: 'cover', w: inch(it.w), h: inch(it.h) } } : {}),
        })
      } else {
        const runs = it.runs.map((r) => ({
          text: r.br ? '' : r.text,
          options: {
            bold: r.bold, italic: r.italic, underline: r.underline ? { style: 'sng' as const } : undefined,
            color: r.color, fontFace: officeFont(r.font, opts.theme), fontSize: pt(r.size),
            ...(r.alpha !== undefined && r.alpha < 0.99 ? { transparency: Math.round((1 - r.alpha) * 100) } : {}),
            breakLine: r.br || undefined,
          },
        }))
        // The Office fonts are not the screen ones: a line stays one line (it
        // may run a little longer), a paragraph gets room to breathe.
        const single = it.lines <= 1
        const grow = single ? 0 : it.w * 0.08
        const x = it.align === 'right' ? it.x - grow : it.align === 'center' ? it.x - grow / 2 : it.x
        slide.addText(runs, {
          x: inch(x), y: inch(it.y), w: inch(it.w + grow + (single ? 2 : 0)), h: inch(it.h + 2),
          margin: 0, valign: 'top', align: it.align as 'left' | 'center' | 'right' | 'justify',
          lineSpacing: pt(it.lineHeight), paraSpaceBefore: 0, paraSpaceAfter: 0, wrap: !single, fit: 'none',
          ...(it.rotate ? { rotate: it.rotate } : {}),
        })
      }
    }
    const note = opts.notes?.[i]
    if (note) slide.addNotes(note)
  })
  return (await pptx.write({ outputType: 'uint8array' })) as Uint8Array
}

// SPDX-License-Identifier: Apache-2.0
// A draw.io drawing read as a SKETCH: its boxes (a label, a shape, a tone)
// and the arrows between them, in the drawing's own coordinates - what a
// thumbnail draws as a still SVG in the page's colours, without the draw.io
// viewer. The viewer draws on a fixed white
// sheet sized to the drawing's page, behind a loading splash while its 3.7 MB
// script loads: on a card it is a white portrait sheet on the dark theme and
// a dark splash on the light one. A sketch has none of that: it is the
// diagram's boxes and arrows, fitted to the card, each box in the Mantine
// hue nearest its draw.io colour, every colour a variable of the colour
// scheme.
//
// Pure functions only (no React, no DOM): bun tests them directly. A drawing
// whose page is compressed (draw.io's deflate + base64) is inflated first by
// `inflateDiagram`, asynchronously.

import { DEFAULT_THEME } from '@mantine/core'

export type SketchShape = 'rect' | 'rounded' | 'ellipse' | 'rhombus' | 'text'

export interface SketchBox {
  id: string
  x: number
  y: number
  w: number
  h: number
  label: string
  shape: SketchShape
  /** The Mantine hue nearest the box's draw.io colour; '' when it has none
   *  (`fillColor=none`, a text). */
  tone: string
  /** A box holding others (a container, a swimlane): drawn as an outline,
   *  its label at its top. */
  container: boolean
}

export interface SketchEdge {
  x1: number
  y1: number
  x2: number
  y2: number
}

export interface Sketch {
  boxes: SketchBox[]
  edges: SketchEdge[]
  /** The drawing's bounds, a margin around it: the SVG's viewBox. */
  view: { x: number; y: number; w: number; h: number }
}

/** The answer of `sketchOf` for a drawing it cannot read now. */
export type SketchResult = { sketch: Sketch } | { compressed: string } | { none: true }

// ── XML reading (no DOMParser: a thumbnail draws in bun and server-side too) ──

const ENTITIES: Record<string, string> = { amp: '&', lt: '<', gt: '>', quot: '"', apos: "'", nbsp: ' ' }

function decodeEntities(s: string): string {
  return s.replace(/&(#x[0-9a-f]+|#\d+|[a-z]+);/gi, (m, e: string) => {
    if (e[0] === '#') {
      const n = e[1] === 'x' || e[1] === 'X' ? parseInt(e.slice(2), 16) : parseInt(e.slice(1), 10)
      // A code point past Unicode's range stays as text: String.fromCodePoint
      // would throw, and one corrupt label would break the page.
      return Number.isInteger(n) && n >= 0 && n <= 0x10ffff ? String.fromCodePoint(n) : m
    }
    return ENTITIES[e.toLowerCase()] ?? m
  })
}

function attrs(s: string): Record<string, string> {
  const out: Record<string, string> = {}
  for (const m of s.matchAll(/([\w:-]+)\s*=\s*("([^"]*)"|'([^']*)')/g)) out[m[1]] = decodeEntities(m[3] ?? m[4] ?? '')
  return out
}

function styleOf(style: string): Record<string, string> {
  const out: Record<string, string> = {}
  for (const part of style.split(';')) {
    if (!part) continue
    const i = part.indexOf('=')
    if (i < 0) out[part] = '1'
    else out[part.slice(0, i)] = part.slice(i + 1)
  }
  return out
}

/** A label as plain text: draw.io labels may be HTML (`html=1`). */
export function plainLabel(value: string): string {
  return decodeEntities(value.replace(/<br\s*\/?>/gi, ' ').replace(/<[^>]+>/g, ''))
    .replace(/\s+/g, ' ').trim()
}

interface Cell {
  id: string
  parent: string
  vertex: boolean
  edge: boolean
  source: string
  target: string
  value: string
  style: Record<string, string>
  geo: { x: number; y: number; w: number; h: number; relative: boolean } | null
  points: { source?: [number, number]; target?: [number, number] }
}

const num = (s: string | undefined): number => {
  const n = Number(s)
  return Number.isFinite(n) ? n : 0
}

function readCells(model: string): Cell[] {
  // A cell wrapped in <UserObject> or <object> carries its id and label on
  // the wrapper: unwrap it first.
  const unwrapped = model.replace(/<(UserObject|object)\b([^>]*)>([\s\S]*?)<\/\1>/g, (_m, _t, wrap: string, inner: string) => {
    const w = attrs(wrap)
    return inner.replace(/<mxCell\b/, `<mxCell id="${(w.id ?? '').replace(/"/g, '&quot;')}" value="${(w.label ?? '').replace(/&/g, '&amp;').replace(/"/g, '&quot;')}"`)
  })
  const cells: Cell[] = []
  for (const m of unwrapped.matchAll(/<mxCell\b([^>]*?)(\/>|>([\s\S]*?)<\/mxCell>)/g)) {
    const a = attrs(m[1])
    const inner = m[3] ?? ''
    const g = /<mxGeometry\b([^>]*?)(\/>|>([\s\S]*?)<\/mxGeometry>)/.exec(inner)
    const ga = g ? attrs(g[1]) : null
    const points: Cell['points'] = {}
    for (const p of (g?.[3] ?? '').matchAll(/<mxPoint\b([^>]*?)\/?>/g)) {
      const pa = attrs(p[1])
      if (pa.as === 'sourcePoint') points.source = [num(pa.x), num(pa.y)]
      if (pa.as === 'targetPoint') points.target = [num(pa.x), num(pa.y)]
    }
    cells.push({
      id: a.id ?? '',
      parent: a.parent ?? '',
      vertex: a.vertex === '1',
      edge: a.edge === '1',
      source: a.source ?? '',
      target: a.target ?? '',
      value: a.value ?? '',
      style: styleOf(a.style ?? ''),
      geo: ga ? { x: num(ga.x), y: num(ga.y), w: num(ga.width), h: num(ga.height), relative: ga.relative === '1' } : null,
      points,
    })
  }
  return cells
}

// ── colour: the nearest Mantine hue ──

function rgbOf(hex: string): [number, number, number] | null {
  const h = hex.trim().replace(/^#/, '')
  const full = h.length === 3 ? h.split('').map((c) => c + c).join('') : h
  if (!/^[0-9a-f]{6}$/i.test(full)) return null
  const n = parseInt(full, 16)
  return [(n >> 16) & 255, (n >> 8) & 255, n & 255]
}

function hsl([r, g, b]: [number, number, number]): [number, number, number] {
  const [R, G, B] = [r / 255, g / 255, b / 255]
  const max = Math.max(R, G, B), min = Math.min(R, G, B)
  const l = (max + min) / 2
  if (max === min) return [0, 0, l]
  const d = max - min
  const s = l > 0.5 ? d / (2 - max - min) : d / (max + min)
  const h = max === R ? (G - B) / d + (G < B ? 6 : 0) : max === G ? (B - R) / d + 2 : (R - G) / d + 4
  return [h * 60, s, l]
}

/** The Mantine hues a sketch may draw a box in (dark and white excepted). */
const HUES = ['red', 'pink', 'grape', 'violet', 'indigo', 'blue', 'cyan', 'teal', 'green', 'lime', 'yellow', 'orange']

let hueTable: Array<[string, number]> | null = null
function hues(): Array<[string, number]> {
  hueTable ??= HUES.map((c) => [c, hsl(rgbOf((DEFAULT_THEME.colors as Record<string, readonly string[]>)[c][6])!)[0]] as [string, number])
  return hueTable
}

/** toneOf is the Mantine hue nearest a draw.io colour: `gray` for a colour
 *  without hue (white, black, grey), '' for none or what it cannot read. */
export function toneOf(color: string | undefined): string {
  if (!color || color === 'none') return ''
  const rgb = rgbOf(color)
  if (!rgb) return 'gray'
  const [h, s, l] = hsl(rgb)
  if (s < 0.18 || l > 0.97 || l < 0.06) return 'gray'
  let best = 'gray', dist = Infinity
  for (const [c, ch] of hues()) {
    const d = Math.min(Math.abs(h - ch), 360 - Math.abs(h - ch))
    if (d < dist) { dist = d; best = c }
  }
  return best
}

// ── the model ──

/** modelOf is the drawing's first page model (`<mxGraphModel>…`), or the
 *  compressed text of its first page, or nothing. */
export function modelOf(xml: string): { model: string } | { compressed: string } | null {
  const m = /<mxGraphModel\b[\s\S]*?<\/mxGraphModel>/.exec(xml)
  if (m) return { model: m[0] }
  const d = /<diagram\b[^>]*>([\s\S]*?)<\/diagram>/.exec(xml)
  const body = d?.[1].trim()
  return body && !body.startsWith('<') ? { compressed: body } : null
}

function shapeOf(st: Record<string, string>): SketchShape {
  if ('text' in st || st.shape === 'text') return 'text'
  if ('ellipse' in st || st.shape === 'ellipse') return 'ellipse'
  if ('rhombus' in st || st.shape === 'rhombus') return 'rhombus'
  return st.rounded === '1' ? 'rounded' : 'rect'
}

/** The point where the segment from a box's centre towards (tx, ty) leaves it. */
function exit(b: SketchBox, tx: number, ty: number): [number, number] {
  const cx = b.x + b.w / 2, cy = b.y + b.h / 2
  const dx = tx - cx, dy = ty - cy
  if (dx === 0 && dy === 0) return [cx, cy]
  const k = Math.min(dx === 0 ? Infinity : (b.w / 2) / Math.abs(dx), dy === 0 ? Infinity : (b.h / 2) / Math.abs(dy))
  return [cx + dx * Math.min(k, 1), cy + dy * Math.min(k, 1)]
}

/** sketchFromModel reads an uncompressed page model: its boxes in absolute
 *  coordinates (a child's geometry is relative to its parent box), its
 *  arrows from box to box or between their free points. Null when it holds
 *  no box. */
export function sketchFromModel(model: string): Sketch | null {
  const cells = readCells(model)
  const byId = new Map(cells.map((c) => [c.id, c]))
  const parents = new Set(cells.filter((c) => c.vertex).map((c) => c.parent))
  const boxes = new Map<string, SketchBox>()
  const origin = (id: string, depth = 0): [number, number] => {
    const p = byId.get(id)
    if (!p || !p.vertex || !p.geo || depth > 32) return [0, 0]
    const [ox, oy] = origin(p.parent, depth + 1)
    return [ox + p.geo.x, oy + p.geo.y]
  }
  for (const c of cells) {
    if (!c.vertex || !c.geo || c.geo.relative || c.geo.w <= 0 || c.geo.h <= 0) continue
    if (byId.get(c.parent)?.edge) continue // an edge's label
    const [ox, oy] = origin(c.parent)
    const shape = shapeOf(c.style)
    const fill = c.style.fillColor
    const stroke = c.style.strokeColor
    // Its fill's hue; a fill without hue (white, grey) takes its stroke's.
    const fillTone = fill === 'none' ? '' : toneOf(fill ?? '#ffffff')
    const tone = shape === 'text' ? ''
      : fillTone === 'gray' || fillTone === '' ? (toneOf(stroke) || fillTone) : fillTone
    boxes.set(c.id, {
      id: c.id, x: ox + c.geo.x, y: oy + c.geo.y, w: c.geo.w, h: c.geo.h,
      label: plainLabel(c.value), shape, tone,
      container: parents.has(c.id) || 'swimlane' in c.style || c.style.container === '1',
    })
  }
  if (boxes.size === 0) return null
  const edges: SketchEdge[] = []
  for (const c of cells) {
    if (!c.edge) continue
    const s = boxes.get(c.source), t = boxes.get(c.target)
    const [ox, oy] = origin(c.parent)
    const sp = c.points.source ? [ox + c.points.source[0], oy + c.points.source[1]] as [number, number] : null
    const tp = c.points.target ? [ox + c.points.target[0], oy + c.points.target[1]] as [number, number] : null
    const sc = s ? [s.x + s.w / 2, s.y + s.h / 2] as [number, number] : sp
    const tc = t ? [t.x + t.w / 2, t.y + t.h / 2] as [number, number] : tp
    if (!sc || !tc) continue
    const [x1, y1] = s ? exit(s, tc[0], tc[1]) : sc
    const [x2, y2] = t ? exit(t, sc[0], sc[1]) : tc
    edges.push({ x1, y1, x2, y2 })
  }
  const all = [...boxes.values()]
  const xs = all.flatMap((b) => [b.x, b.x + b.w]).concat(edges.flatMap((e) => [e.x1, e.x2]))
  const ys = all.flatMap((b) => [b.y, b.y + b.h]).concat(edges.flatMap((e) => [e.y1, e.y2]))
  const minX = Math.min(...xs), maxX = Math.max(...xs), minY = Math.min(...ys), maxY = Math.max(...ys)
  const pad = Math.max(8, 0.05 * Math.max(maxX - minX, maxY - minY))
  // Containers first, so the boxes they hold are drawn over them.
  const ordered = all.filter((b) => b.container).concat(all.filter((b) => !b.container))
  return { boxes: ordered, edges, view: { x: minX - pad, y: minY - pad, w: maxX - minX + 2 * pad, h: maxY - minY + 2 * pad } }
}

/** sketchOf reads a drawing: its sketch, or the compressed page to inflate
 *  first (`inflateDiagram`), or none - nothing a sketch can draw. */
export function sketchOf(xml: string): SketchResult {
  const m = modelOf(xml ?? '')
  if (!m) return { none: true }
  if ('compressed' in m) return { compressed: m.compressed }
  const sketch = sketchFromModel(m.model)
  return sketch ? { sketch } : { none: true }
}

/** inflateDiagram reads a compressed draw.io page - base64 of the raw
 *  deflate of its URI-encoded model - with the platform's decompression
 *  stream. */
export async function inflateDiagram(compressed: string): Promise<string> {
  const bin = Uint8Array.from(atob(compressed), (c) => c.charCodeAt(0))
  const stream = new Blob([bin]).stream().pipeThrough(new DecompressionStream('deflate-raw'))
  return decodeURIComponent(await new Response(stream).text())
}

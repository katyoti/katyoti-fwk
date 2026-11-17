// SPDX-License-Identifier: Apache-2.0
// The slide-deck splitter — the TypeScript reader of the deck grammar.
//
// It mirrors internal/extensions/slidedeck/api/parse.go rule for rule: the
// splitting rules are the one part of the grammar both readers hold, and the
// shared corpus (internal/extensions/slidedeck/api/testdata/corpus, run by
// app/ktw-display/tests/ui/deck/parse.test.ts) holds them together. The model
// type is generated from the Go struct. What a deck may declare (settings,
// layouts) is checked by the Go lint, not here.
//
// parseDeck never throws: a source cut anywhere — mid-stream — parses to the
// slides it holds so far, and `open` names the construct it ended inside.

import { parse as parseYAML } from 'yaml'

import type { Deck } from './generated/deck-model.ts'

export type Slide = Deck['slides'][number]
export type SlotContent = Slide['slots'][number]

export const DEFAULT_SLOT = 'default'
export const DEFAULT_LAYOUT = 'content'

const settingsRe = /^<!--\s*slide:(.*?)-->$/
const pairRe = /([A-Za-z][A-Za-z0-9_-]*)=("([^"]*)"|[^\s"]+)/g
const slotRe = /^::([a-z][a-z0-9-]*)::$/
const noteRe = /^Note:/

const enum K { Text, Blank, Fence, Code, SepH, SepV, Settings, Slot, Note }

const trimRight = (s: string) => s.replace(/[ \t]+$/, '')

function fenceRun(s: string, c: string): number {
  let n = 0
  while (n < s.length && s[n] === c) n++
  return n
}

/** Tags each line, fence-aware; returns the line (from 1) of a fence left open, or 0. */
function classify(lines: string[]): { kinds: K[]; openFence: number } {
  const kinds: K[] = new Array(lines.length)
  let fenceChar = ''
  let fenceLen = 0
  let fenceLine = 0
  lines.forEach((raw, i) => {
    const line = trimRight(raw)
    const trimmed = line.replace(/^ +/, '')
    const indent = line.length - trimmed.length
    if (fenceLen > 0) {
      const closes = indent <= 3 && fenceRun(trimmed, fenceChar) >= fenceLen &&
        trimmed.split(fenceChar).join('') === ''
      if (closes) { kinds[i] = K.Fence; fenceLen = 0 } else kinds[i] = K.Code
      return
    }
    if (indent <= 3 && (trimmed[0] === '`' || trimmed[0] === '~')) {
      const n = fenceRun(trimmed, trimmed[0])
      if (n >= 3) { kinds[i] = K.Fence; fenceChar = trimmed[0]; fenceLen = n; fenceLine = i + 1; return }
    }
    if (line === '') kinds[i] = K.Blank
    else if (line === '---') kinds[i] = K.SepH
    else if (line === '--') kinds[i] = K.SepV
    else if (settingsRe.test(line.trim())) kinds[i] = K.Settings
    else if (slotRe.test(line)) kinds[i] = K.Slot
    else if (noteRe.test(line)) kinds[i] = K.Note
    else kinds[i] = K.Text
  })
  return { kinds, openFence: fenceLen > 0 ? fenceLine : 0 }
}

/** The key=value pairs of a settings line body; rest is what is not a pair. */
export function parseSettings(body: string): { pairs: Record<string, string>; rest: string } {
  const pairs: Record<string, string> = {}
  for (const m of body.matchAll(pairRe)) pairs[m[1]] = m[2].startsWith('"') ? m[3] : m[2]
  return { pairs, rest: body.replace(pairRe, '').trim() }
}

function trimBlank(lines: string[]): { md: string; first: number } {
  let a = 0
  let b = lines.length
  while (a < b && lines[a].trim() === '') a++
  while (b > a && lines[b - 1].trim() === '') b--
  return { md: lines.slice(a, b).map(trimRight).join('\n'), first: a }
}

function buildSlide(lines: string[], kinds: K[], first: number): Slide | null {
  const s: Slide = { h: 0, v: 0, line: first, slots: [] }
  let i = 0
  while (i < lines.length && kinds[i] === K.Blank) i++
  if (i < lines.length && kinds[i] === K.Settings) {
    const m = settingsRe.exec(lines[i].trim())!
    const { pairs } = parseSettings(m[1])
    if (Object.keys(pairs).length > 0) s.settings = pairs
    s.settingsLine = first + i
    i++
  }
  let name = DEFAULT_SLOT
  let slotLine = 0
  let from = i
  const flush = (to: number) => {
    const { md, first: firstContent } = trimBlank(lines.slice(from, to))
    if (name === DEFAULT_SLOT) {
      if (md === '') return
      slotLine = first + from + firstContent
    }
    s.slots.push({ name, line: slotLine, markdown: md })
  }
  for (; i < lines.length; i++) {
    if (kinds[i] === K.Slot) {
      flush(i)
      name = slotRe.exec(trimRight(lines[i]))![1]
      slotLine = first + i
      from = i + 1
    } else if (kinds[i] === K.Note) {
      flush(i)
      const { md } = trimBlank([lines[i].replace(noteRe, '').trim(), ...lines.slice(i + 1)])
      if (md !== '') s.notes = md
      return s
    }
  }
  flush(lines.length)
  return s.settingsLine || s.slots.length > 0 || s.notes ? s : null
}

/** Splits a deck. Never throws; see the file comment. */
export function parseDeck(src: string): Deck {
  const d: Deck = { settings: {}, slides: [] }
  const lines = src.replace(/\r\n/g, '\n').split('\n')
  let start = 0
  if (lines.length > 0 && trimRight(lines[0]) === '---') {
    let end = -1
    for (let j = 1; j < lines.length; j++) if (trimRight(lines[j]) === '---') { end = j; break }
    if (end < 0) { d.open = 'frontmatter'; d.openLine = 1; return d }
    try {
      const m = parseYAML(lines.slice(1, end).join('\n'))
      if (m && typeof m === 'object' && !Array.isArray(m)) d.settings = m as Record<string, unknown>
    } catch { /* the lint reports invalid YAML */ }
    start = end + 1
  }
  const body = lines.slice(start)
  const { kinds, openFence } = classify(body)
  if (openFence > 0) { d.open = 'code'; d.openLine = start + openFence }

  const chunks: { vertical: boolean; from: number; to: number }[] = []
  let cur = { vertical: false, from: 0, to: 0 }
  kinds.forEach((k, i) => {
    if (k === K.SepH || k === K.SepV) {
      chunks.push({ ...cur, to: i })
      cur = { vertical: k === K.SepV, from: i + 1, to: 0 }
    }
  })
  chunks.push({ ...cur, to: body.length })

  // A blank slide is dropped. A vertical slide whose horizontal parent was
  // dropped starts a new horizontal slide: the `---` still opened a section.
  let h = -1
  let v = 0
  let parentDropped = false
  for (const c of chunks) {
    const s = buildSlide(body.slice(c.from, c.to), kinds.slice(c.from, c.to), start + c.from + 1)
    if (!s) { if (!c.vertical) parentDropped = true; continue }
    if (c.vertical && h >= 0 && !parentDropped) v++
    else { h++; v = 0 }
    parentDropped = false
    s.h = h
    s.v = v
    d.slides.push(s)
  }
  return d
}

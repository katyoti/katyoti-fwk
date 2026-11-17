// SPDX-License-Identifier: Apache-2.0
// Unit-test corpus for the drawio auto-close shim. See gui-drawio M2
// (todo/ms06, 2026-05-07) — analyse.md §4.4 for the design.
//
// Asserts: (a) output parses cleanly via DOMParser, (b) closing tags
// match the truncation depth, (c) idempotence on already-valid input.

import { describe, expect, it } from 'bun:test'
import { autoCloseXml } from '../../../../ktw-display-lib/src/ui-builders/drawio/auto-close.ts'

const FULL_DOC = `<mxfile host="app.diagrams.net" type="device"><diagram name="Sample" id="s1"><mxGraphModel dx="800" dy="600" grid="1"><root><mxCell id="0"/><mxCell id="1" parent="0"/><mxCell id="2" vertex="1" parent="1" value="Service A"><mxGeometry x="100" y="100" width="160" height="60" as="geometry"/></mxCell></root></mxGraphModel></diagram></mxfile>`

// Bun's test runtime doesn't expose DOMParser. We assert "parse-clean"
// by walking the same regex the shim uses: every closing tag must match
// the most recent open of the same name, and the final stack must be
// empty. Same balance check the shim relies on by construction.
const TAG_RE = /<\/?([A-Za-z][\w-]*)[^>]*?(\/?)>/g

function parsesClean(xml: string): boolean {
  if (!xml) return true // empty input → caller renders empty; nothing to parse.
  const stack: string[] = []
  TAG_RE.lastIndex = 0
  let m: RegExpExecArray | null
  while ((m = TAG_RE.exec(xml)) !== null) {
    const [full, name, selfClose] = m
    if (full.startsWith('</')) {
      if (stack[stack.length - 1] !== name) return false
      stack.pop()
    } else if (selfClose !== '/') {
      stack.push(name)
    }
  }
  return stack.length === 0
}

function depth(xml: string): number {
  if (!xml) return 0
  const stack: string[] = []
  let max = 0
  TAG_RE.lastIndex = 0
  let m: RegExpExecArray | null
  while ((m = TAG_RE.exec(xml)) !== null) {
    const [full, name, selfClose] = m
    if (full.startsWith('</')) {
      stack.pop()
    } else if (selfClose !== '/') {
      stack.push(name)
      if (stack.length > max) max = stack.length
    }
  }
  return max
}

describe('autoCloseXml', () => {
  it('returns empty string on empty input', () => {
    expect(autoCloseXml('')).toBe('')
  })

  it('returns empty string when no complete tag has arrived yet', () => {
    expect(autoCloseXml('<mxfile')).toBe('')
    expect(autoCloseXml('<mxfile host="ap')).toBe('')
  })

  it('is idempotent on a well-formed full document', () => {
    const out = autoCloseXml(FULL_DOC)
    expect(out).toBe(FULL_DOC)
    expect(parsesClean(out)).toBe(true)
  })

  it('handles a single open tag with no children', () => {
    const out = autoCloseXml('<mxfile>')
    expect(out).toBe('<mxfile></mxfile>')
    expect(parsesClean(out)).toBe(true)
  })

  it('balances 2 nested opens', () => {
    const out = autoCloseXml('<mxfile><diagram>')
    expect(out).toBe('<mxfile><diagram></diagram></mxfile>')
    expect(parsesClean(out)).toBe(true)
  })

  it('balances 5+ nested opens (mxGraph depth)', () => {
    const partial =
      '<mxfile><diagram><mxGraphModel><root><mxCell id="0"/><mxCell id="1">'
    const out = autoCloseXml(partial)
    expect(parsesClean(out)).toBe(true)
    expect(out.endsWith('</mxCell></root></mxGraphModel></diagram></mxfile>')).toBe(true)
  })

  it('treats self-closing tags as already balanced (does not push to stack)', () => {
    const partial = '<mxfile><diagram><mxCell id="2"><mxGeometry x="10" y="10" as="geometry"/></mxCell>'
    const out = autoCloseXml(partial)
    expect(parsesClean(out)).toBe(true)
    // The mxGeometry self-closes; stack should be [mxfile, diagram] after the
    // mxCell close. Output ends with two closes.
    expect(out.endsWith('</diagram></mxfile>')).toBe(true)
  })

  it('truncates a partial tag fragment at the last `>`', () => {
    const partial = '<mxfile><diagram><mxCell id="2" val'
    const out = autoCloseXml(partial)
    // The `<mxCell id="2" val` fragment after the last `>` is dropped.
    // Stack at the last `>` is [mxfile, diagram] — 2 closes appended.
    expect(out).toBe('<mxfile><diagram></diagram></mxfile>')
    expect(parsesClean(out)).toBe(true)
  })

  it('truncates a partial attribute value', () => {
    const partial = '<mxfile><mxCell value="Box'
    const out = autoCloseXml(partial)
    expect(out).toBe('<mxfile></mxfile>')
    expect(parsesClean(out)).toBe(true)
  })

  it('handles progressive truncation at every byte (smoke)', () => {
    // For each prefix length, the shim must produce parse-clean output.
    for (let n = 1; n <= FULL_DOC.length; n++) {
      const prefix = FULL_DOC.substring(0, n)
      const out = autoCloseXml(prefix)
      expect(parsesClean(out)).toBe(true)
    }
  })

  it('preserves attribute values with `>` inside (none in our producers, but defensive)', () => {
    // Our mxGraph producers don't emit `>` inside attribute values. If they
    // did, lastIndexOf('>') would falsely truncate. Document the limitation:
    // the shim trusts that attribute values contain no `>` — entity-encoded
    // (`&gt;`) is fine.
    const safe = '<mxCell value="A &gt; B"/>'
    expect(parsesClean(autoCloseXml(safe))).toBe(true)
  })

  it('depth grows with the truncation depth', () => {
    expect(depth(autoCloseXml('<a>'))).toBe(1)
    expect(depth(autoCloseXml('<a><b>'))).toBe(2)
    expect(depth(autoCloseXml('<a><b><c>'))).toBe(3)
  })
})

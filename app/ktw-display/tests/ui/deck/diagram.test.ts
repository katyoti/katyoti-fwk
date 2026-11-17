// SPDX-License-Identifier: Apache-2.0
// draw.io diagrams on a slide (ui-builders/deck/diagram.tsx): drawn in the
// colours their file sets, whatever the page's or the system's theme. The
// viewer is faked: it records what the slide asks of it.

import { describe, expect, it } from 'bun:test'
import { drawioSVG, type DrawioViewerStatic } from '@ktw/display-lib/ui-builders/deck/diagram.tsx'

/** Just enough of a document for the off-screen viewer element. */
function fakeDocument() {
  const el = { style: { cssText: '' }, className: '', setAttribute() {}, remove() {} }
  ;(globalThis as any).document = { createElement: () => el, body: { appendChild() {} } }
}

describe('drawioSVG', () => {
  it('exports the diagram with adaptive colours off and holds the SVG light', async () => {
    const calls: string[] = []
    const svg = { style: {} as Record<string, string> }
    const viewer: DrawioViewerStatic = {
      createViewerForElement: (_el, cb) => cb({
        graph: {
          setAdaptiveColors: (mode) => { calls.push(`adaptive:${mode}`) },
          getSvg: () => { calls.push('getSvg'); return svg as unknown as SVGSVGElement },
        },
      }),
    }
    const before = (globalThis as any).document
    let got: SVGSVGElement
    fakeDocument()
    try {
      got = await drawioSVG('<mxfile/>', viewer)
    } finally {
      ;(globalThis as any).document = before
    }
    expect(calls).toEqual(['adaptive:none', 'getSvg'])
    expect((got as unknown as typeof svg).style.colorScheme).toBe('light')
  })
})

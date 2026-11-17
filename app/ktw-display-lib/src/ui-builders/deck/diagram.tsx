// SPDX-License-Identifier: Apache-2.0
// Diagrams in a slide (modules/slidedeck/): the architecture itself, drawn
// from its source each time the deck renders, so a changed diagram shows in
// the deck with nothing exported by hand.
//
//   ```mermaid … ```                      a mermaid diagram written in the slide
//   ![C4 context](diagrams/context.drawio) a draw.io file beside the deck
//
// Both become an SVG inside `.deck-diagram`: what presenting, the PDF, the
// picture exports and the standalone page show, and what the editable
// PowerPoint takes as a picture (deck-pptx.ts).

import { useEffect, useRef, useState } from 'react'

import { loadDrawioViewer } from '../drawio/viewer-shim.ts'

/** A draw.io file: the diagram is drawn, not shown as an image. */
export function isDrawioPath(src: string): boolean {
  return /\.drawio(\.xml)?$/i.test(src.split(/[?#]/)[0] ?? '')
}

/** The slice of mermaid the deck uses. */
interface MermaidAPI {
  initialize(config: Record<string, unknown>): void
  render(id: string, code: string): Promise<{ svg: string }>
}

declare global {
  interface Window { mermaid?: MermaidAPI }
}

/** The URL mermaid is served at (the display server's vendor files); a
 *  standalone page carries it inline instead, so window.mermaid is already
 *  there. Loaded on the first mermaid diagram, not with the deck: it is
 *  larger than the rest of the deck. */
export const MERMAID_URL = '/widget/vendor/mermaid/mermaid.min.js'

let mermaidLib: Promise<MermaidAPI> | undefined
const loadMermaid = (): Promise<MermaidAPI> =>
  (mermaidLib ??= new Promise<MermaidAPI>((resolve, reject) => {
    if (window.mermaid) return resolve(window.mermaid)
    const script = document.createElement('script')
    script.src = MERMAID_URL
    script.async = true
    script.addEventListener('load', () => (window.mermaid ? resolve(window.mermaid) : reject(new Error('mermaid loaded but not defined'))))
    script.addEventListener('error', () => reject(new Error('mermaid could not be loaded')))
    document.head.appendChild(script)
  }).catch((e: unknown) => {
    mermaidLib = undefined // a later diagram may try again
    throw e
  }))

let mermaidSeq = 0

/** The theme's tokens, as mermaid's theme variables: a diagram in the deck's
 *  colours and font. */
function mermaidTheme(el: HTMLElement) {
  const css = getComputedStyle(el)
  const v = (name: string, fallback: string) => css.getPropertyValue(name).trim() || fallback
  return {
    fontFamily: v('--deck-font', 'sans-serif'),
    primaryColor: v('--deck-surface', '#f3f4f6'),
    primaryBorderColor: v('--deck-accent', '#4f46e5'),
    primaryTextColor: v('--deck-fg', '#111827'),
    secondaryColor: v('--deck-accent-soft', '#eef2ff'),
    tertiaryColor: v('--deck-bg', '#ffffff'),
    lineColor: v('--deck-muted', '#6b7280'),
    textColor: v('--deck-fg', '#111827'),
    mainBkg: v('--deck-surface', '#f3f4f6'),
    nodeBorder: v('--deck-accent', '#4f46e5'),
    clusterBkg: v('--deck-bg', '#ffffff'),
    clusterBorder: v('--deck-border', '#d1d5db'),
  }
}

/** A mermaid block, drawn in the theme's colours. */
export function MermaidDiagram({ code }: { code: string }) {
  const host = useRef<HTMLDivElement>(null)
  const [error, setError] = useState('')
  useEffect(() => {
    let live = true
    const el = host.current
    if (!el) return
    loadMermaid()
      .then(async (mermaid) => {
        // mermaid sizes each box from its text measured in the theme's font:
        // measured before that font loads, a label overflows its box.
        const family = getComputedStyle(el).getPropertyValue('--deck-font').trim()
        if (family) await document.fonts?.load(`16px ${family}`).catch(() => undefined)
        await document.fonts?.ready
        mermaid.initialize({ startOnLoad: false, securityLevel: 'strict', theme: 'base', themeVariables: mermaidTheme(el) })
        const { svg } = await mermaid.render(`deck-mermaid-${++mermaidSeq}`, code)
        if (!live) return
        el.innerHTML = svg
        // mermaid caps the drawing at its natural width: let it fill the
        // slide's box instead (its view box keeps the proportions).
        const drawn = el.querySelector('svg')
        if (drawn) fill(drawn)
        setError('')
      })
      .catch((e: unknown) => {
        if (live) setError(e instanceof Error ? e.message : String(e))
      })
    return () => {
      live = false
    }
  }, [code])
  return (
    <div className="deck-diagram" data-diagram="mermaid">
      {error && <div className="deck-diagram-error">Diagram not drawn: {error}</div>}
      <div ref={host} className="deck-diagram-svg" />
    </div>
  )
}

/** The slice of the draw.io viewer a slide uses: its graph's SVG export. */
interface DrawioGraph {
  getSvg(background: string | null, scale: number, border: number): SVGSVGElement
  setAdaptiveColors?(mode: string | null): void
}
export interface DrawioViewerStatic {
  createViewerForElement?: (el: HTMLElement, cb: (v: { graph?: DrawioGraph }) => void) => unknown
}

/** A diagram's XML drawn by the draw.io viewer off screen, then exported as
 *  one still SVG: the whole diagram, sized to the slide like a picture — the
 *  interactive viewer would fit it to a width and scroll the rest.
 *
 *  In the colours its file sets: the viewer writes adaptive colours
 *  (`light-dark()` under `color-scheme: light dark`), which a dark page or a
 *  dark system turns into black boxes and pale text. A slide is not the
 *  page's theme: adaptive colours are off and the SVG is held light. */
export function drawioSVG(xml: string, GraphViewer = (window as unknown as { GraphViewer?: DrawioViewerStatic }).GraphViewer): Promise<SVGSVGElement> {
  if (!GraphViewer?.createViewerForElement) return Promise.reject(new Error('the draw.io viewer is not loaded'))
  return new Promise((resolve, reject) => {
    const el = document.createElement('div')
    el.style.cssText = 'position:absolute;left:-20000px;top:0;width:1600px;height:1200px;visibility:hidden'
    el.className = 'mxgraph'
    el.setAttribute('data-mxgraph', JSON.stringify({ xml, resize: false, 'auto-fit': false, toolbar: '', nav: false, 'check-visible-state': false }))
    document.body.appendChild(el)
    const done = (f: () => void) => { el.remove(); f() }
    try {
      GraphViewer.createViewerForElement!(el, (v) => {
        try {
          v.graph?.setAdaptiveColors?.('none')
          const svg = v.graph?.getSvg(null, 1, 8)
          if (svg) svg.style.colorScheme = 'light'
          done(() => (svg ? resolve(svg) : reject(new Error('the diagram has no drawing'))))
        } catch (e) {
          done(() => reject(e))
        }
      })
    } catch (e) {
      done(() => reject(e))
    }
  })
}

/** An SVG made to fill its box, its proportions kept. */
function fill(svg: SVGSVGElement) {
  if (!svg.getAttribute('viewBox')) {
    const w = parseFloat(svg.getAttribute('width') ?? ''), h = parseFloat(svg.getAttribute('height') ?? '')
    if (w > 0 && h > 0) svg.setAttribute('viewBox', `0 0 ${w} ${h}`)
  }
  svg.removeAttribute('width')
  svg.removeAttribute('height')
  svg.style.maxWidth = '100%'
  svg.style.width = '100%'
  svg.style.height = '100%'
}

/** A draw.io file beside the deck, drawn as a still SVG. */
export function DrawioDiagram({ url, alt }: { url: string; alt?: string }) {
  const host = useRef<HTMLDivElement>(null)
  const [error, setError] = useState('')
  useEffect(() => {
    let live = true
    ;(async () => {
      const res = await fetch(url)
      if (!res.ok) throw new Error(`${alt || url}: not found beside the deck`)
      const xml = await res.text()
      await loadDrawioViewer()
      const svg = await drawioSVG(xml)
      const el = host.current
      if (!live || !el) return
      fill(svg)
      el.replaceChildren(svg)
      setError('')
    })().catch((e: unknown) => {
      if (live) setError(e instanceof Error ? e.message : String(e))
    })
    return () => {
      live = false
    }
  }, [url, alt])
  return (
    <div className="deck-diagram" data-diagram="drawio" role="img" aria-label={alt}>
      {error && <div className="deck-diagram-error">Diagram not drawn: {error}</div>}
      <div ref={host} className="deck-diagram-svg" />
    </div>
  )
}

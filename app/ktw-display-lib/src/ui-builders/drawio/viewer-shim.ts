// SPDX-License-Identifier: Apache-2.0
// viewer-shim.ts — lazy loader + mount helper for the vendored drawio
// `viewer-static.min.js`.
//
// The vendored file is a 3.7 MB IIFE that puts `GraphViewer` (plus
// supporting classes) onto `window`. We do NOT bundle it into the main
// runtime — it's served as a static asset by the bun runtime at
// `/widget/vendor/drawio/viewer-static.min.js` (see runtime/serve.ts
// `serveVendor`). This shim:
//
//   1. Injects a <script> tag on demand. Idempotent — repeated calls
//      share the same in-flight Promise, never re-inject.
//   2. Resolves once `window.GraphViewer` is available.
//   3. Provides `createDrawioViewer(element, xml)` — mounts a viewer
//      onto an empty element via the `data-mxgraph` JSON-config pattern
//      that drawio's processElements uses internally.
//
// Re-rendering with new XML: drawio's GraphViewer doesn't expose an
// in-place update API, so the wrapper destroys + recreates on each
// `setXml` call. With the 200ms debounce in DrawioView this is fine
// for streaming updates.

declare global {
  interface Window {
    GraphViewer?: GraphViewerStatic
  }
}

interface MxGraph {
  /** Scale + translate so the diagram fits in the visible viewport.
   *  `border` is the margin in px. mxGraph's standard fit method. */
  fit?: (border?: number, keepOrigin?: boolean, margin?: number) => number
  center?: (horizontal?: boolean, vertical?: boolean) => void
  refresh?: () => void
  /** Upper bound on the fit scale. drawio's GraphViewer defaults this to a
   *  value that prevents zooming a small diagram up to fill the container —
   *  we clear it so the diagram fills the available width. */
  maxFitScale?: number | null
}

interface GraphViewerInstance {
  /** The underlying mxGraph instance. fit() lives here, NOT on the viewer
   *  itself — earlier attempts to call viewer.fit() were no-ops. */
  graph?: MxGraph
}

interface GraphViewerStatic {
  /** Scan the DOM for elements with class `mxgraph` and a `data-mxgraph`
   *  JSON config attribute, instantiating viewers in place. */
  processElements(): void
  /** Create a viewer for a single element — preferred over
   *  processElements when we want a handle on the viewer instance. */
  createViewerForElement?: (element: HTMLElement, callback?: (v: GraphViewerInstance) => void) => GraphViewerInstance
}

let loadingPromise: Promise<void> | null = null

/** Inject the vendored viewer-static.min.js into the document head.
 *  Idempotent — multiple callers share the same Promise. Resolves once
 *  the script has fired its `load` event AND `window.GraphViewer` is
 *  populated. Rejects on script error or load timeout (10s). */
export function loadDrawioViewer(): Promise<void> {
  if (typeof window === 'undefined') {
    return Promise.reject(new Error('drawio viewer requires a browser context'))
  }
  if (window.GraphViewer) return Promise.resolve()
  if (loadingPromise) return loadingPromise

  loadingPromise = new Promise<void>((resolve, reject) => {
    const script = document.createElement('script')
    script.src = '/widget/vendor/drawio/viewer-static.min.js'
    script.async = true

    const timeout = window.setTimeout(() => {
      reject(new Error('drawio viewer load timed out (10s)'))
    }, 10_000)

    script.addEventListener('load', () => {
      window.clearTimeout(timeout)
      if (!window.GraphViewer) {
        reject(new Error('viewer-static.min.js loaded but window.GraphViewer is undefined'))
        return
      }
      resolve()
    })
    script.addEventListener('error', () => {
      window.clearTimeout(timeout)
      reject(new Error('failed to load /widget/vendor/drawio/viewer-static.min.js'))
    })

    document.head.appendChild(script)
  })

  return loadingPromise
}

/** Wrapper for a single mounted viewer. Use `setXml` to swap the XML
 *  (destroys + remounts under the hood — drawio has no in-place API).
 *  Call `destroy` on unmount. */
export interface DrawioViewerHandle {
  setXml(xml: string): void
  destroy(): void
}

/** Mount a drawio viewer onto an empty element using the documented
 *  data-mxgraph config pattern. Caller must have awaited
 *  loadDrawioViewer() first.
 *
 *  The element is repurposed: this function clears its children, sets
 *  the `mxgraph` class + `data-mxgraph` JSON attribute, and calls
 *  GraphViewer.processElements() to instantiate. setXml replays the
 *  same flow with new XML. */
export function createDrawioViewer(element: HTMLElement, xml: string): DrawioViewerHandle {
  const GraphViewer = window.GraphViewer
  if (!GraphViewer) {
    throw new Error('drawio viewer not loaded — call loadDrawioViewer() first')
  }

  // Closure-scoped so the deferred fit can act on the CURRENT viewer's graph.
  let currentViewer: GraphViewerInstance | undefined
  let fitRAF = 0

  function fitGraph() {
    const g = currentViewer?.graph
    if (g && typeof g.fit === 'function') {
      try {
        // Clear the GraphViewer fit cap so a small diagram scales UP to fill
        // the available width (otherwise it renders tiny + centered).
        g.maxFitScale = null
        g.fit(8)
        if (typeof g.center === 'function') g.center(true, true)
      } catch {
        // viewer destroyed by a fast re-mount; ignore.
      }
    }
  }

  function mount(currentXml: string) {
    // Cancel a pending deferred fit from a prior mount before rebuilding.
    if (fitRAF) cancelAnimationFrame(fitRAF)
    currentViewer = undefined
    // Clear any prior render artifacts (drawio inserts SVG + chrome divs).
    while (element.firstChild) element.removeChild(element.firstChild)
    element.classList.add('mxgraph')
    element.setAttribute(
      'data-mxgraph',
      JSON.stringify({
        highlight: '#0000ff',
        nav: true,
        // resize:false → do NOT grow the container to the diagram's native
        // size. With resize:true drawio expands the host element to fit the
        // whole diagram; inside a fixed-height tab that overflow is clipped,
        // so a large diagram renders entirely outside the visible slice
        // (looks blank). Instead we fit the diagram INTO the container.
        resize: false,
        'auto-fit': true,
        center: true,
        // Render immediately. The default check-visible-state defers the
        // first render behind a MutationObserver that watches the parent
        // for ATTRIBUTE changes — but our container gets its width from a
        // stylesheet, not an attribute, so that observer never fires and the
        // canvas stays blank forever. Disable it so v() runs synchronously.
        'check-visible-state': false,
        toolbar: '',
        xml: currentXml,
      }),
    )

    // createViewerForElement delivers the viewer via its CALLBACK, not its
    // return value (which is undefined). Capture it there so the fit()
    // below (and the ResizeObserver) have a graph to act on.
    const onViewer = (v: GraphViewerInstance) => {
      currentViewer = v
      // ONE fit, deferred to the next frame so drawio has finished laying out
      // the graph + the container has its final box. Fitting immediately AND
      // again on rAF caused a visible double adjust (fit, then a small re-fit
      // a frame later). We do NOT observe the container with a ResizeObserver:
      // fit() toggles the overflow scrollbar, which changes the element's
      // content box, which would re-fire it → an endless zoom in/out loop.
      // Fit is driven by DATA (each new XML re-mounts the viewer).
      if (fitRAF) cancelAnimationFrame(fitRAF)
      fitRAF = requestAnimationFrame(fitGraph)
    }
    if (typeof GraphViewer!.createViewerForElement === 'function') {
      GraphViewer!.createViewerForElement(element, onViewer)
    } else {
      GraphViewer!.processElements()
    }
  }

  mount(xml)

  return {
    setXml(newXml: string) {
      mount(newXml)
    },
    destroy() {
      if (fitRAF) cancelAnimationFrame(fitRAF)
      currentViewer = undefined
      while (element.firstChild) element.removeChild(element.firstChild)
      element.classList.remove('mxgraph')
      element.removeAttribute('data-mxgraph')
    },
  }
}

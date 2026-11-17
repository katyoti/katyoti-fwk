// SPDX-License-Identifier: Apache-2.0
// The slice of reveal.js 5 the deck view uses. reveal.js ships no types.

declare module 'reveal.js/dist/reveal.esm.js' {
  export interface RevealIndices { h: number; v: number; f?: number }
  export interface RevealOptions {
    embedded?: boolean
    hash?: boolean
    history?: boolean
    respondToHashChanges?: boolean
    keyboardCondition?: 'focused' | null
    controls?: boolean
    controlsLayout?: 'edges' | 'bottom-right'
    progress?: boolean
    slideNumber?: boolean | string
    center?: boolean
    transition?: string
    width?: number
    height?: number
    margin?: number
    pdfSeparateFragments?: boolean
    plugins?: unknown[]
    showNotes?: boolean
    display?: string
    /** Chalkboard plugin options (reveal.js-plugins). */
    chalkboard?: Record<string, unknown>
  }
  export default class Reveal {
    constructor(el: HTMLElement, options?: RevealOptions)
    initialize(): Promise<void>
    configure(options: RevealOptions): void
    sync(): void
    layout(): void
    slide(h: number, v?: number, f?: number): void
    getIndices(): RevealIndices
    getTotalSlides(): number
    on(event: string, fn: (e: unknown) => void): void
    off(event: string, fn: (e: unknown) => void): void
    destroy(): void
  }
}

declare module 'reveal.js/plugin/notes/notes.esm.js' {
  const Notes: () => unknown
  export default Notes
}

declare module 'reveal.js/plugin/zoom/zoom.esm.js' {
  const Zoom: () => unknown
  export default Zoom
}

declare module 'reveal.js/plugin/highlight/highlight.esm.js' {
  /** The slice of highlight.js the deck uses. */
  export interface Hljs {
    getLanguage(name: string): unknown
    highlight(code: string, options: { language: string; ignoreIllegals?: boolean }): { value: string }
  }
  const Highlight: () => { hljs: Hljs }
  export default Highlight
}

declare module 'reveal.js-plugins/chalkboard/plugin.js'
declare module 'reveal.js/dist/reveal.css'
declare module '*.css'

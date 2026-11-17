// SPDX-License-Identifier: Apache-2.0
// DeckView — a markdown slide deck rendered live with reveal.js
// (modules/slidedeck/).
//
// The markdown is the only state: every render parses it (parseDeck), React
// draws the slides, and reveal.js is synced over them and put back on the
// slide and fragment it was showing — an edit or a streamed chunk never
// resets the presentation. While the producer is still writing, the view
// follows the newest slide until the user navigates.
//
// Layouts are styled by deck.css through [data-layout] / [data-slot]:
// reveal.js rewrites a section's classes as it navigates, so nothing the
// view needs is carried by a class on a section.
//
// A theme (theme.ts) adds its CSS, each slide's look (a variant's background,
// whether the chrome shows), the chrome overlay inside each slide, and its
// pictograms written as `![label](icon:<name>)`.

import ReactMarkdown, { defaultUrlTransform } from 'react-markdown'
import remarkGfm from 'remark-gfm'
import Reveal, { type RevealIndices } from 'reveal.js/dist/reveal.esm.js'
import RevealNotes from 'reveal.js/plugin/notes/notes.esm.js'
// Zoom: Alt+click (Ctrl+click on Linux) zooms on a detail of a slide — a
// dense diagram — while presenting; the same again zooms back out.
import RevealZoom from 'reveal.js/plugin/zoom/zoom.esm.js'
// highlight.js, as reveal.js bundles it: the code a slide writes in a fenced
// block with its language is highlighted when React draws it (the plugin's
// own pass would be undone by the next render); deck.css colours the tokens
// from the theme.
import RevealHighlight from 'reveal.js/plugin/highlight/highlight.esm.js'
import 'reveal.js/dist/reveal.css'
// Chalkboard (rajgoel/reveal.js-plugins, MIT) — annotations: a pen over the
// slide and a whiteboard. A classic script: it sets window.RevealChalkboard.
import 'reveal.js-plugins/chalkboard/plugin.js'
import { createContext, useContext, useEffect, useLayoutEffect, useMemo, useRef, useState, type ReactNode } from 'react'

import './deck.css'
import { DEFAULT_LAYOUT, DEFAULT_SLOT, parseDeck, type Slide } from './parse.ts'
import { localAssetPath, MODULE_SCHEME } from './assets.ts'
import { DrawioDiagram, isDrawioPath, MermaidDiagram } from './diagram.tsx'
import { ChartView } from './chart.tsx'
import { openThreads, ReviewPanel } from './review-panel.tsx'
import { parseReview, type DeckReview } from './review.ts'
import { expandGroups } from './groups.ts'
import { fitSlides } from './fit.ts'
import { allowsColour, boxStyle, chromeText, DEFAULT_THEME, deckAspect, deckWatermark, iconAsset, slideLook, themeCSS, type DeckWatermark, type LoadedTheme, type SlideLook } from './theme.ts'
import { timelineSteps } from './timeline.ts'

export interface DeckViewProps {
  /** The deck's markdown. */
  source: string
  /** The producer is still writing the source. */
  streaming?: boolean
  /** Colour scheme of the theme. */
  scheme?: 'light' | 'dark'
  /** Draw on the slides (Chalkboard). On by default. */
  annotations?: boolean
  /** Where the drawings are kept for the session — one key per deck. */
  annotationsKey?: string
  /** Fill the page without the view's own buttons (a deck presented alone in
   *  a browser window). */
  presenting?: boolean
  /** The drawings to start from (Chalkboard's JSON) — a deck object's saved
   *  drawings; they replace the session's. */
  drawings?: string
  /** Called, debounced, with the drawings (Chalkboard's JSON) after a stroke,
   *  an erase or a clear — how a host keeps them with the deck. */
  onDrawingsChange?: (drawings: string) => void
  /** Called with reveal.js's indices of the slide shown, when it changes —
   *  how a host follows the slide (the deck settings panel's Slide tab). */
  onSlideChange?: (at: { h: number; v: number }) => void
  /** The deck's theme, when it names one other than katyoti and the host has
   *  it: its declaration and where its files are. Without it the deck shows
   *  in the default look. */
  theme?: LoadedTheme
  /** Where a file beside the deck is (an image or a background named by a
   *  relative path), when the host has published them. */
  localAsset?: (path: string) => string
  /** The deck's review (its JSON: comment threads per slide, the reviews
   *  marked done — OD-017), when the host keeps one: a deck object's
   *  `review` named file. With onReviewChange, the view offers 💬 Review. */
  review?: string
  /** Who comments: the person's name. */
  reviewer?: string
  /** Called with the review's JSON after each comment, reply, resolve or
   *  review marked done — how a host keeps it beside the deck. */
  onReviewChange?: (review: string) => void
  /** The slides of each slide group the deck includes, by name (th22) — the
   *  host's, as it hands the theme. An include slide shows them in its place;
   *  without them it shows where the group goes. */
  groups?: Record<string, string>
}

interface Chalkboard {
  toggleNotesCanvas(): void
  toggleChalkboard(): void
  clear(): void
  getData(): string
  colorNext(): void
  /** Selects pen `idx` of the current mode (set on the plugin at init). */
  colorIndex(idx: number): void
}

/** Chalkboard is one object per page: the first deck mounted on a page owns it. */
let chalkboardOwner: object | null = null
const chalkboard = () => (window as unknown as { RevealChalkboard?: Chalkboard }).RevealChalkboard

/** Pens: CSS cursors, no image files (the plugin's own images resolve against
 *  a script path a bundle does not have). */
const PENS = ['#1f2937', '#4f46e5', '#dc2626', '#16a34a', '#ea580c'].map((color) => ({
  color,
  cursor: 'crosshair',
}))

const SIZES: Record<string, { width: number; height: number }> = {
  '16:9': { width: 1280, height: 720 },
  '4:3': { width: 1024, height: 768 },
}

const COLOUR = /^(#[0-9a-f]{3,8}|rgba?\(|hsla?\(|[a-z]+$)/i

/** A leading heading of the default slot becomes the slide's title region,
 *  so a column layout can run the title across its columns. */
function splitTitle(md: string): { title: string; rest: string } {
  const m = /^(#{1,6} [^\n]*)\n?/.exec(md)
  return m ? { title: m[1], rest: md.slice(m[0].length).replace(/^\n+/, '') } : { title: '', rest: md }
}

/** The deck's theme, for what the markdown draws from it (its icons). */
const ThemeContext = createContext<LoadedTheme | undefined>(undefined)
/** Where the files beside the deck are (DeckViewProps.localAsset). */
const AssetsContext = createContext<((path: string) => string) | undefined>(undefined)

/** A relative image: the host's copy of the file beside the deck. */
function LocalImage({ src, alt, ...props }: { src: string; alt?: string } & Record<string, unknown>) {
  const resolve = useContext(AssetsContext)
  const p = localAssetPath(src)
  return <img src={p && resolve ? resolve(p) : src} alt={alt} {...props} />
}

/** A draw.io file beside the deck, drawn where the slide names it. */
function LocalDiagram({ src, alt }: { src: string; alt?: string }) {
  const resolve = useContext(AssetsContext)
  const p = localAssetPath(src)
  return <DrawioDiagram url={p && resolve ? resolve(p) : src} alt={alt} />
}

/** The text of a fenced block's code element (react-markdown's hast node). */
type HastNode = { type?: string; value?: string; tagName?: string; properties?: { className?: unknown }; children?: HastNode[] }
const hastText = (n?: HastNode): string => (n?.value ?? '') + (n?.children ?? []).map(hastText).join('')

const ICON = 'icon:'

/** Keeps `icon:<name>` (a theme's pictogram) past react-markdown's URL guard. */
const urlTransform = (url: string) => (url.startsWith(ICON) || url.startsWith(MODULE_SCHEME) ? url : defaultUrlTransform(url))

/** `![label](icon:<name>)`: the theme's pictogram as a mask its colour fills;
 *  the label when the theme has no such icon (the lint names it). */
function Icon({ name, label }: { name: string; label: string }) {
  const theme = useContext(ThemeContext)
  const asset = iconAsset(theme?.body, name)
  if (!theme || !asset) return <span className="deck-icon-missing">{label || name}</span>
  const mask = `url("${theme.assetURL(asset)}")`
  return (
    <span
      className="deck-icon"
      role="img"
      aria-label={label || theme.body.icons?.[name]?.label || name}
      style={{ maskImage: mask, WebkitMaskImage: mask }}
    />
  )
}

const hljs = RevealHighlight().hljs

/** A fenced block's code, highlighted when its language is known. */
function Code({ className, children, ...props }: { className?: string; children?: ReactNode }) {
  const lang = /language-([\w-]+)/.exec(className ?? '')?.[1]
  if (lang && hljs.getLanguage(lang)) {
    const html = hljs.highlight(String(children ?? '').replace(/\n$/, ''), { language: lang, ignoreIllegals: true }).value
    return <code className={`${className} hljs`} dangerouslySetInnerHTML={{ __html: html }} />
  }
  return <code className={className} {...props}>{children}</code>
}

function Markdown({ children, steps }: { children: string; steps: boolean }) {
  return (
    <ReactMarkdown
      remarkPlugins={[remarkGfm]}
      urlTransform={urlTransform}
      components={{
        ...(steps ? { li: ({ node: _n, ...props }) => <li className="fragment" {...props} /> } : {}),
        code: ({ node: _n, className, children }) => <Code className={className}>{children}</Code>,
        // A ```mermaid block is a diagram, drawn — not code.
        pre: ({ node, children, ...props }) => {
          const code = (node as HastNode | undefined)?.children?.find((c) => c.tagName === 'code')
          const cls = code?.properties?.className
          if (Array.isArray(cls) && cls.includes('language-mermaid')) return <MermaidDiagram code={hastText(code).replace(/\n$/, '')} />
          // A ```chart <kind> block is a chart, drawn from its data.
          if (Array.isArray(cls) && cls.includes('language-chart')) {
            const meta = (code as { data?: { meta?: string } } | undefined)?.data?.meta ?? ''
            return <ChartView kind={meta.trim().split(/\s+/)[0] ?? ''} body={hastText(code)} />
          }
          return <pre {...props}>{children}</pre>
        },
        img: ({ node: _n, src, alt, ...props }) =>
          typeof src === 'string' && src.startsWith(ICON)
            ? <Icon name={src.slice(ICON.length)} label={alt ?? ''} />
            : typeof src === 'string' && isDrawioPath(src)
              ? <LocalDiagram src={src} alt={alt} />
              : <LocalImage src={typeof src === 'string' ? src : ''} alt={alt} {...props} />,
      }}
    >
      {children}
    </ReactMarkdown>
  )
}

function SlideBody({ slide }: { slide: Slide }) {
  const settings = slide.settings ?? {}
  const layout = settings.layout || DEFAULT_LAYOUT
  const steps = settings.steps === 'list'
  const regions: ReactNode[] = []
  for (const slot of slide.slots) {
    let md = slot.markdown
    if (slot.name === DEFAULT_SLOT) {
      const { title, rest } = splitTitle(md)
      if (title) regions.push(<div key="title" className="deck-title"><Markdown steps={false}>{title}</Markdown></div>)
      md = rest
      // cover, section and quote show their heading inside the slot.
      if (layout === 'cover' || layout === 'section' || layout === 'quote') md = slot.markdown
    }
    // A timeline step's name stands on its own line: its separator goes.
    if (layout === 'timeline' && slot.name === 'steps') md = timelineSteps(md)
    regions.push(
      <div key={slot.name} className="deck-slot" data-slot={slot.name}>
        <Markdown steps={steps}>{md}</Markdown>
      </div>,
    )
  }
  return (
    <div
      className={['deck-slide', settings.class].filter(Boolean).join(' ')}
      data-layout={layout}
      data-variant={settings.variant || undefined}
    >
      {regions}
    </div>
  )
}

/** The section's reveal.js attributes: its transition, and its background —
 *  the slide's own, else its look's under the theme. A colour outside the
 *  theme's palette is not shown: the theme's look stays (the lint says why). */
function sectionProps(slide: Slide, look?: SlideLook, theme?: LoadedTheme, localAsset?: (path: string) => string): Record<string, string> {
  const s = slide.settings ?? {}
  const p: Record<string, string> = {}
  if (s.transition) p['data-transition'] = s.transition
  const own = s.background && (!COLOUR.test(s.background) || allowsColour(theme?.body, s.background)) ? s.background : ''
  const local = own && !COLOUR.test(own) ? localAssetPath(own) : null
  if (own) p[COLOUR.test(own) ? 'data-background-color' : 'data-background-image'] = local && localAsset ? localAsset(local) : own
  else if (look?.background && 'colour' in look.background) p['data-background-color'] = look.background.colour
  else if (look?.background && 'image' in look.background) p['data-background-image'] = look.background.image
  return p
}

const TOKEN = /^[a-z0-9]+(-[a-z0-9]+)*$/
const tokenColour = (token: string | undefined, fallback: string) => `var(--deck-${token && TOKEN.test(token) ? token : fallback})`
const justify = (align?: string) => (align === 'right' ? 'flex-end' : align === 'center' ? 'center' : 'flex-start')

/** The theme's chrome on one slide: its logo, rules, footer and number, placed
 *  in % of the theme's slide so they scale, present and print with it. */
function Chrome({ theme, look, title, author, n, total }: {
  theme: LoadedTheme; look: SlideLook; title: string; author: string; n: number; total: number
}) {
  const c = theme.body.chrome
  const aspect = theme.body.aspect
  if (!look.logo && !(look.chrome && c)) return null
  const text = (t: NonNullable<NonNullable<typeof c>['footer']>, key: string) => (
    <div
      key={key}
      className="deck-chrome-text"
      style={{ ...boxStyle(t.box, aspect), justifyContent: justify(t.align), fontSize: t.size, color: tokenColour(t.token, 'muted') }}
    >
      {chromeText(t.template, { title, author, n, total })}
    </div>
  )
  return (
    <div className="deck-chrome" aria-hidden="true">
      {look.logo ? (
        <img className="deck-chrome-logo" src={theme.assetURL(look.logo.asset)} alt="" style={boxStyle(look.logo.box, aspect)} />
      ) : null}
      {look.chrome && c
        ? (c.rules ?? []).map((r, i) => {
            const b = boxStyle({ x: r.x1, y: r.y, w: r.x2 - r.x1, h: 0 }, aspect)
            return (
              <div
                key={`rule-${i}`}
                className="deck-chrome-rule"
                style={{ left: b.left, top: b.top, width: b.width, height: `${r.width || 1}px`, background: tokenColour(r.token, 'fg') }}
              />
            )
          })
        : null}
      {look.chrome && c?.footer ? text(c.footer, 'footer') : null}
      {look.chrome && c?.slideNumber ? text(c.slideNumber, 'number') : null}
    </div>
  )
}

/** The deck's marks on one slide: the watermark text across it, and the
 *  production mark where the theme's chrome shows (on every slide for a theme
 *  without chrome). Inside the slide: they scale, present and print with it. */
function Marks({ wm, theme, look, title, author, n, total }: {
  wm: DeckWatermark; theme?: LoadedTheme; look?: SlideLook; title: string; author: string; n: number; total: number
}) {
  const aspect = theme?.body.aspect
  const markHere = !theme?.body.chrome || !!look?.chrome
  if (!wm.text && !(markHere && (wm.mark || wm.logo))) return null
  const size = wm.text ? `${Math.max(48, Math.min(160, Math.round(1500 / Math.max(wm.text.length, 1))))}px` : undefined
  return (
    <div className="deck-marks" aria-hidden="true">
      {wm.text ? <div className="deck-watermark"><span style={{ fontSize: size }}>{wm.text}</span></div> : null}
      {markHere && wm.logo && theme ? (
        <img className="deck-chrome-logo" src={theme.assetURL(wm.logo.asset)} alt="" style={boxStyle(wm.logo.box, aspect)} />
      ) : null}
      {markHere && wm.mark ? (
        <div
          className="deck-chrome-text deck-mark"
          style={{ ...boxStyle(wm.mark.box, aspect), justifyContent: justify(wm.mark.align), fontSize: wm.mark.size, color: tokenColour(wm.mark.token, 'muted') }}
        >
          {chromeText(wm.mark.template, { title, author, n, total })}
        </div>
      ) : null}
    </div>
  )
}

function SlideSection({ slide, theme, wm, title, author, n, total }: {
  slide: Slide; theme?: LoadedTheme; wm: DeckWatermark; title: string; author: string; n: number; total: number
}) {
  const settings = slide.settings ?? {}
  const look = theme ? slideLook(theme, settings.layout || DEFAULT_LAYOUT, settings.variant) : undefined
  const localAsset = useContext(AssetsContext)
  return (
    <section {...sectionProps(slide, look, theme, localAsset)} data-line={slide.line} data-group={slide.group || undefined}>
      <SlideBody slide={slide} />
      {theme && look ? <Chrome theme={theme} look={look} title={title} author={author} n={n} total={total} /> : null}
      <Marks wm={wm} theme={theme} look={look} title={title} author={author} n={n} total={total} />
      {slide.notes ? <aside className="notes"><Markdown steps={false}>{slide.notes}</Markdown></aside> : null}
    </section>
  )
}

export function DeckView({
  source,
  streaming = false,
  scheme = 'light',
  annotations = true,
  annotationsKey,
  drawings,
  onDrawingsChange,
  onSlideChange,
  presenting = false,
  theme: loadedTheme,
  localAsset,
  review: reviewText,
  reviewer = '',
  onReviewChange,
  groups,
}: DeckViewProps) {
  const deck = useMemo(() => expandGroups(parseDeck(source ?? ''), groups), [source, groups])
  const rootRef = useRef<HTMLDivElement | null>(null)
  const revealRef = useRef<HTMLDivElement | null>(null)
  const instance = useRef<Reveal | null>(null)
  const position = useRef<RevealIndices>({ h: 0, v: 0 })
  const follow = useRef(true)
  const ours = useRef(false)
  const [ready, setReady] = useState(false)
  const [drawing, setDrawing] = useState(false)
  const [pen, setPen] = useState(0)
  const [presentError, setPresentError] = useState(false)
  // The review: the host's copy, edited here and handed back on each change.
  const [shownAt, setShownAt] = useState({ h: 0, v: 0 })
  const [reviewing, setReviewing] = useState(false)
  const [review, setReview] = useState<DeckReview>(() => parseReview(reviewText))
  useEffect(() => { setReview(parseReview(reviewText)) }, [reviewText])
  const owner = useRef<object>({})
  const slideCb = useRef(onSlideChange)
  slideCb.current = onSlideChange
  const shown = useRef('')
  // Tells the host the slide shown, once per change of slide.
  const reportSlide = () => {
    const { h, v } = position.current
    if (shown.current === `${h}/${v}`) return
    shown.current = `${h}/${v}`
    setShownAt({ h, v })
    slideCb.current?.({ h, v })
  }
  const withBoard = annotations && (chalkboardOwner === null || chalkboardOwner === owner.current)

  const settings = deck.settings as Record<string, unknown>
  const transition = typeof settings.transition === 'string' ? settings.transition : 'slide'
  const theme = typeof settings.theme === 'string' && settings.theme ? settings.theme : DEFAULT_THEME
  const title = typeof settings.title === 'string' ? settings.title : ''
  const author = typeof settings.author === 'string' ? settings.author : ''
  // The loaded theme applies only to the deck that names it: a deck switched
  // to another theme shows in the default look until that one is loaded.
  const active = loadedTheme && loadedTheme.body.name === theme ? loadedTheme : undefined
  const aspect = deckAspect(settings, active?.body)
  const css = useMemo(() => (active ? themeCSS(active) : ''), [active])
  // A named theme still loading has no marks yet; the default theme has its own.
  const wm = useMemo(() => (active || theme === DEFAULT_THEME ? deckWatermark(active, settings) : {}),
    [active, theme, settings.watermark, settings.mark, settings.production_mark])
  const numberOf = useMemo(() => new Map(deck.slides.map((s, i) => [s, i + 1])), [deck])

  // Group vertical slides under their horizontal slide.
  const stacks = useMemo(() => {
    const out: Slide[][] = []
    for (const s of deck.slides) (out[s.h] ??= []).push(s)
    return out.filter(Boolean)
  }, [deck])

  // reveal.js — and Chalkboard, which reads the current slide as soon as it
  // starts — need a slide to exist. A deck still waiting for its markdown (the
  // host pushes it after the iframe is ready) or for its first streamed slide
  // shows the empty stage until then.
  const hasSlides = stacks.length > 0

  useEffect(() => {
    if (!revealRef.current || !hasSlides) return
    const board = withBoard ? chalkboard() : undefined
    if (board) chalkboardOwner = owner.current
    // Saved drawings seed the board's session storage before it starts.
    if (board && drawings && annotationsKey) {
      try { sessionStorage.setItem(`ktw-deck:${annotationsKey}`, drawings) } catch { /* storage off: start blank */ }
    }
    const r = new Reveal(revealRef.current, {
      embedded: true,
      hash: false,
      history: false,
      respondToHashChanges: false,
      keyboardCondition: 'focused',
      controls: true,
      progress: true,
      slideNumber: 'c/t',
      center: false,
      transition,
      margin: 0,
      // A printed slide (⎙ PDF) is one page with every step shown, as the
      // picture and PowerPoint exports show it — not one page per step.
      pdfSeparateFragments: false,
      ...SIZES[aspect],
      plugins: board ? [RevealNotes, RevealZoom, board] : [RevealNotes, RevealZoom],
      chalkboard: board
        ? {
            theme: 'whiteboard',
            boardmarkers: PENS,
            boardmarkerWidth: 4,
            grid: false,
            background: ['rgba(127,127,127,.08)', ''],
            eraser: { src: '', radius: 20 },
            toggleChalkboardButton: false,
            toggleNotesButton: false,
            colorButtons: false,
            boardHandle: false,
            storage: annotationsKey ? `ktw-deck:${annotationsKey}` : undefined,
          }
        : undefined,
    })
    const onMove = () => {
      position.current = r.getIndices()
      if (!ours.current) follow.current = false
      reportSlide()
      // The slides laid out now (the shown one and its neighbours) fit their page.
      if (rootRef.current) fitSlides(rootRef.current)
    }
    r.on('slidechanged', onMove)
    // Printing lays every slide out as a page: fit them once that is done
    // (measured while the pages were being made, they all looked too tall).
    r.on('pdf-ready', () => { if (rootRef.current) fitSlides(rootRef.current) })
    r.on('fragmentshown', onMove)
    r.on('fragmenthidden', onMove)
    let alive = true
    r.initialize().then(() => {
      if (!alive) return
      // reveal.js merges the page's query string into its config, and the
      // widget URL carries the host's own `display=ui` — which reveal reads
      // as the CSS display of a shown slide. Put it back.
      r.configure({ display: 'block' })
      r.sync()
      instance.current = r
      // Exposed on the element so a test (or the host) can drive the deck.
      ;(revealRef.current as (HTMLDivElement & { revealDeck?: Reveal }) | null)!.revealDeck = r
      setReady(true)
    })
    return () => {
      alive = false
      if (chalkboardOwner === owner.current) chalkboardOwner = null
      instance.current = null
      setReady(false)
      try { r.destroy() } catch { /* already gone */ }
    }
    // A new aspect ratio or default transition needs a new instance; so does
    // the first slide arriving.
  }, [aspect, transition, hasSlides])

  // Chalkboard announces every stroke, erase and clear as a `broadcast` DOM
  // event; the host hears the resulting drawings, debounced.
  const drawingsCb = useRef(onDrawingsChange)
  drawingsCb.current = onDrawingsChange
  useEffect(() => {
    if (!ready || !withBoard) return
    let t: ReturnType<typeof setTimeout> | undefined
    const on = () => {
      clearTimeout(t)
      t = setTimeout(() => {
        const data = chalkboard()?.getData()
        if (data && drawingsCb.current) drawingsCb.current(data)
      }, 800)
    }
    document.addEventListener('broadcast', on)
    return () => {
      clearTimeout(t)
      document.removeEventListener('broadcast', on)
    }
  }, [ready, withBoard])

  // After every render: resync reveal.js over the new DOM and put it back.
  useLayoutEffect(() => {
    const r = instance.current
    if (!r || !ready) return
    ours.current = true
    try {
      r.sync()
      if (streaming && follow.current && stacks.length > 0) {
        const last = stacks.length - 1
        r.slide(last, stacks[last].length - 1)
      } else {
        const { h, v, f } = position.current
        r.slide(Math.min(h, Math.max(stacks.length - 1, 0)), v, f)
      }
      position.current = r.getIndices()
    } finally {
      ours.current = false
    }
    reportSlide()
    if (rootRef.current) fitSlides(rootRef.current)
  })

  // Text measured in a fallback font is measured again in the theme's.
  useEffect(() => {
    if (!ready || typeof document === 'undefined' || !document.fonts) return
    let alive = true
    const refit = () => { if (alive && rootRef.current) fitSlides(rootRef.current) }
    document.fonts.ready.then(refit).catch(() => {})
    document.fonts.addEventListener?.('loadingdone', refit)
    return () => {
      alive = false
      document.fonts.removeEventListener?.('loadingdone', refit)
    }
  }, [ready])

  useEffect(() => {
    if (!streaming) follow.current = true
  }, [streaming])

  // A theme's chrome holds the slide's corners (logo, footer): reveal.js's
  // arrows go to the edges, off the footer.
  const chromed = !!active?.body.chrome
  useEffect(() => {
    instance.current?.configure({ controlsLayout: chromed ? 'edges' : 'bottom-right' })
  }, [chromed, ready])

  // reveal.js takes keys once it has seen a pointer on the deck.
  const focusDeck = () => revealRef.current?.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true }))

  // Full screen on this screen. A host that does not let its frame go full
  // screen refuses it: the view says so rather than doing nothing.
  const present = () => {
    setPresentError(false)
    const go = rootRef.current?.requestFullscreen?.()
    if (!go) { setPresentError(true); return }
    go.then(() => {
      instance.current?.layout()
      focusDeck()
    }).catch(() => setPresentError(true))
  }
  const pickPen = (i: number) => {
    board?.colorIndex(i)
    setPen(i)
    focusDeck()
  }

  const board = withBoard ? chalkboard() : undefined
  const draw = () => {
    board?.toggleNotesCanvas()
    setDrawing((d) => !d)
    focusDeck()
  }

  const count = deck.slides.length
  const shownIndex = Math.max(0, deck.slides.findIndex((s) => s.h === shownAt.h && s.v === shownAt.v))
  const changeReview = (r: DeckReview) => {
    setReview(r)
    onReviewChange?.(JSON.stringify(r, null, 2))
  }
  const goTo = (i: number) => {
    const s = deck.slides[i]
    if (s) instance.current?.slide(s.h, s.v)
  }
  const openCount = onReviewChange ? openThreads(review) : 0
  return (
    <div
      ref={rootRef}
      className={presenting ? 'ktw-deck ktw-deck--presenting' : 'ktw-deck'}
      data-deck-theme={theme}
      data-scheme={scheme}
      tabIndex={0}
      style={{ aspectRatio: `${SIZES[aspect].width} / ${SIZES[aspect].height}` }}
    >
      {presenting ? null : (
        <div className="deck-toolbar">
          <button type="button" onClick={present} title="Present full screen (Esc to leave)">▶ Present</button>
          {board ? (
            <>
              <button type="button" aria-pressed={drawing} onClick={draw} title="Draw on the slide (C)">✎ Draw</button>
              <span className="deck-pens" role="radiogroup" aria-label="Pen colour">
                {PENS.map((p, i) => (
                  <button
                    key={p.color}
                    type="button"
                    role="radio"
                    aria-checked={pen === i}
                    aria-label={`Pen ${i + 1}`}
                    title="Pen colour"
                    className="deck-pen"
                    style={{ background: p.color }}
                    onClick={() => pickPen(i)}
                  />
                ))}
              </span>
              <button type="button" onClick={() => { board.toggleChalkboard(); focusDeck() }} title="Whiteboard (B)">▭ Board</button>
              <button type="button" onClick={() => { board.clear(); focusDeck() }} title="Clear this slide's drawing (Del)">⌫ Clear</button>
            </>
          ) : null}
          {onReviewChange ? (
            <button type="button" aria-pressed={reviewing} onClick={() => setReviewing((r) => !r)}
              title="Comments per slide, and what changed since the last review">
              💬 Review{openCount ? ` (${openCount})` : ''}
            </button>
          ) : null}
        </div>
      )}
      {reviewing && onReviewChange && !presenting ? (
        <ReviewPanel review={review} slides={deck.slides} current={shownIndex} author={reviewer || 'me'}
          onChange={changeReview} onGo={goTo} onClose={() => setReviewing(false)} />
      ) : null}
      {presentError && !presenting ? (
        <div className="deck-status deck-status--notice" role="status" onClick={() => setPresentError(false)}>
          Full screen is not allowed here — use ⧉ Present in a window
        </div>
      ) : null}
      {streaming ? <div className="deck-status">{count} / streaming…</div> : null}
      {css ? <style>{css}</style> : null}
      <AssetsContext.Provider value={localAsset}>
      <ThemeContext.Provider value={active}>
        <div className="deck-stage">
          <div ref={revealRef} className="reveal">
            <div className="slides">
              {stacks.map((stack) => {
                const section = (s: Slide, key: string) => (
                  <SlideSection key={key} slide={s} theme={active} wm={wm} title={title} author={author} n={numberOf.get(s) ?? 0} total={count} />
                )
                return stack.length === 1 ? (
                  section(stack[0], `${stack[0].h}`)
                ) : (
                  <section key={`${stack[0].h}`}>{stack.map((s) => section(s, `${s.h}.${s.v}`))}</section>
                )
              })}
            </div>
          </div>
        </div>
      </ThemeContext.Provider>
      </AssetsContext.Provider>
    </div>
  )
}

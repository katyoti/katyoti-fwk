// SPDX-License-Identifier: Apache-2.0
// Per-kind renderInner functions for <UniversalHarness>.
//
// Each function is ~20 LOC: build the schema artifact via buildUIxxx(),
// resolve the named variant from URL, mount the corresponding view
// component with the right handler shape. The harness owns all shared
// logic (postMessage, value subscription, debug panes, layout chrome);
// these are PURE per-kind dispatch.
//
// Replaces the per-kind logic that was duplicated across the 5 old
// harness files (ui-form-harness.tsx etc.).

import { useEffect, useMemo, useState, type ReactNode } from 'react'

import {
  buildUIForms,
  UIFormView,
  type UIForm as UIFormBuilt,
} from '@ktw/display-lib/ui-builders/form/index.ts'
import {
  buildUIViews,
  UIViewView,
  type UIView as UIViewBuilt,
  type UIViewEmptyStrategy,
} from '@ktw/display-lib/ui-builders/view/index.ts'
import {
  buildUITables,
  UITableView,
  type UITable as UITableBuilt,
} from '@ktw/display-lib/ui-builders/table/index.ts'
import {
  buildUITreeTables,
  UITreeTableView,
  type UITreeTable as UITreeTableBuilt,
} from '@ktw/display-lib/ui-builders/tree-table/index.ts'
import {
  buildUIWizards,
  UIWizardView,
  type UIWizard as UIWizardBuilt,
} from '@ktw/display-lib/ui-builders/wizard/index.ts'
import { UIPageView, type UIPageNode } from '@ktw/display-lib/ui-builders/page/index.ts'
import { DrawioView, type DrawioValue } from '@ktw/display-lib/ui-builders/drawio/index.ts'
import { DEFAULT_THEME, DeckView, parseDeck, type DeckViewProps, type LoadedTheme, type Theme } from '@ktw/display-lib/ui-builders/deck/index.ts'
import type { UIActionParams } from '@ktw/display-lib/ui-core/action/index.ts'

import { CannotRender, type RenderInnerCtx } from './universal-harness.tsx'

// =================================================================================================
// URL-param helpers — pick the named variant when the schema declares many
// =================================================================================================

function resolveNamedVariant(key: string): string {
  try {
    return new URLSearchParams(window.location.search).get(key) || 'default'
  } catch {
    return 'default'
  }
}

function resolveEmptyStrategy(): UIViewEmptyStrategy {
  try {
    const v = new URLSearchParams(window.location.search).get('empty')
    if (v === 'omit' || v === 'em-dash' || v === 'explicit-empty') return v
  } catch {
    // fall through
  }
  return 'em-dash'
}

// =================================================================================================
// renderInner per kind
// =================================================================================================

export function renderForm(ctx: RenderInnerCtx): ReactNode {
  const { config, item, emit, appendOutcome } = ctx
  const forms: Record<string, UIFormBuilt> = buildUIForms({ schema: config.schema })
  const formName = resolveNamedVariant('form')
  const form = forms[formName] ?? forms['default']
  if (!form) {
    return (
      <CannotRender
        kind="form"
        message="No form extension found and the synthesized 'default' form failed to build."
      />
    )
  }
  const isNew = config.mode === 'create'
  return (
    <UIFormView
      form={form}
      item={item as Record<string, unknown> | undefined}
      isNew={isNew}
      onSubmit={(value: unknown) => {
        emit({ type: 'submit', value })
        appendOutcome('submit', value)
      }}
      onChange={(value, meta) => {
        emit({ type: 'change', value, dirty: meta.dirty, valid: meta.valid })
        appendOutcome('change', value)
      }}
    />
  )
}

export function renderView(ctx: RenderInnerCtx): ReactNode {
  const { config, item, emit, appendOutcome } = ctx
  const views: Record<string, UIViewBuilt> = buildUIViews({ schema: config.schema })
  const viewName = resolveNamedVariant('view')
  const view = views[viewName] ?? views['default']
  if (!view) {
    return (
      <CannotRender
        kind="view"
        message="No view extension found and the synthesized 'default' view failed to build."
      />
    )
  }
  const actionProcessor = async (params: UIActionParams): Promise<void> => {
    emit({
      type: 'action',
      action: params.action.id,
      args: { item: 'item' in params ? params.item : undefined },
    })
    appendOutcome(`action: ${params.action.id}`, params)
  }
  return (
    <UIViewView
      view={view}
      item={(item ?? {}) as Record<string, unknown>}
      defaultEmptyStrategy={resolveEmptyStrategy()}
      actionProcessor={actionProcessor}
    />
  )
}

export function renderTable(ctx: RenderInnerCtx): ReactNode {
  const { config, item, emit, appendOutcome } = ctx
  const tables: Record<string, UITableBuilt> = buildUITables({ schema: config.schema })
  const tableName = resolveNamedVariant('table')
  const table = tables[tableName] ?? tables['default']
  if (!table) {
    return (
      <CannotRender
        kind="table"
        message="No table extension found and the synthesized 'default' table failed to build."
      />
    )
  }
  // Tables expect Array data. Defensive: wrap a single object; default to [].
  const data: unknown[] = Array.isArray(item)
    ? item
    : item && typeof item === 'object'
      ? [item]
      : []
  const actionProcessor = async (params: UIActionParams): Promise<void> => {
    emit({
      type: 'action',
      action: params.action.id,
      args: { item: 'item' in params ? params.item : undefined },
    })
    appendOutcome(`action: ${params.action.id}`, params)
  }
  return <UITableView table={table} data={data} actionProcessor={actionProcessor} />
}

export function renderTreeTable(ctx: RenderInnerCtx): ReactNode {
  const { config, item, emit, appendOutcome } = ctx
  const trees: Record<string, UITreeTableBuilt> = buildUITreeTables({ schema: config.schema, data: asRows(item) })
  const treeName = resolveNamedVariant('tree-table')
  const tree = trees[treeName] ?? trees['default']
  if (!tree) {
    return (
      <CannotRender
        kind="tree-table"
        message="No ui-tree-table extension found and the synthesized 'default' tree-table failed to build."
      />
    )
  }
  const actionProcessor = async (params: UIActionParams): Promise<void> => {
    emit({
      type: 'action',
      action: params.action.id,
      args: { item: 'item' in params ? params.item : undefined },
    })
    appendOutcome(`action: ${params.action.id}`, params)
  }
  return <UITreeTableView tree={tree} data={asRows(item)} actionProcessor={actionProcessor} />
}

// asRows coerces the harness `item` (array | single object | undefined) into the
// row array a table/tree-table expects.
function asRows(item: unknown): unknown[] {
  return Array.isArray(item) ? item : item && typeof item === 'object' ? [item] : []
}

export function renderWizard(ctx: RenderInnerCtx): ReactNode {
  const { config, item, emit, appendOutcome } = ctx
  const wizards: Record<string, UIWizardBuilt> = buildUIWizards({ schema: config.schema })
  const wizardName = resolveNamedVariant('wizard')
  const wizard = wizards[wizardName] ?? wizards['default']
  if (!wizard) {
    return (
      <CannotRender
        kind="wizard"
        message="No ui-wizard extension found and the synthesized 'default' wizard failed to build."
      />
    )
  }
  return (
    <UIWizardView
      wizard={wizard}
      item={item as Record<string, unknown> | undefined}
      onComplete={(value: unknown) => {
        emit({ type: 'submit', value })
        appendOutcome('wizard complete', value)
      }}
    />
  )
}

export function renderPage(ctx: RenderInnerCtx): ReactNode {
  const { data, emit, appendOutcome } = ctx
  // Coerce a stringified tree back to an object — LLMs sometimes emit
  // `dslTree` as a JSON string rather than an inline object, which would
  // otherwise reach <UIPageView> as a string and render "Unknown node
  // kind". The Go stream plugin already coerces at the source; this is
  // the last-line guard for any other emit path (cross-turn patches).
  const tree = coerceTree(ctx.tree)
  if (!tree) {
    return <CannotRender kind="page" message="No DSL tree provided." />
  }
  // Pass outer `data` so dataField references inside the tree resolve
  // via JSONPath; a form node's submit reaches the host as `submit`.
  return (
    <UIPageView
      node={tree as UIPageNode}
      data={data}
      onSubmit={(value, meta) => {
        emit({ type: 'submit', value })
        appendOutcome(`submit${meta.path ? ` ${meta.path}` : ''}`, value)
      }}
    />
  )
}

function coerceTree(tree: unknown): unknown {
  if (typeof tree !== 'string') return tree
  const t = tree.trim()
  if (t[0] !== '{' && t[0] !== '[') return tree
  try {
    return JSON.parse(t)
  } catch {
    return tree
  }
}

/** Drawio kind — `item` carries the value (`{ xml, filename? }`). The
 *  XML may also arrive as a bare string from the schema's initialValue;
 *  we coerce to the `DrawioValue` shape so the view doesn't have to.
 *  Reached by the `drawio` URL hint or a schema carrying the `ui-drawio`
 *  extension. Design: modules/drawio/docs/viewer-and-streaming.md. */
export function renderDrawio(ctx: RenderInnerCtx): ReactNode {
  const { item } = ctx
  const value = coerceDrawioValue(item)
  return <DrawioView value={value} />
}

function coerceDrawioValue(item: unknown): DrawioValue {
  if (typeof item === 'string') {
    // LLMs sometimes emit valueInline as a JSON STRING — e.g.
    // `{"xml":"<mxfile>…","filename":"x.drawio"}` — rather than an inline
    // object. Left as-is, the whole JSON blob is treated as the diagram
    // XML and the viewer renders blank. Recover the object when the string
    // parses to one carrying an `xml` field; otherwise treat it as raw XML.
    const t = item.trim()
    if (t[0] === '{') {
      try {
        const parsed = JSON.parse(t)
        if (parsed && typeof parsed === 'object') return coerceDrawioValue(parsed)
      } catch {
        // not JSON — fall through to raw-XML handling
      }
    }
    return { xml: item }
  }
  if (item && typeof item === 'object') {
    const o = item as Record<string, unknown>
    return {
      xml: typeof o.xml === 'string' ? o.xml : '',
      filename: typeof o.filename === 'string' ? o.filename : undefined,
      format: o.format === 'svg+xml' || o.format === 'mxgraph' ? o.format : undefined,
      page: typeof o.page === 'number' ? o.page : undefined,
    }
  }
  return { xml: '' }
}

/** Deck kind — a markdown slide deck (modules/slidedeck/). `item` is the
 *  markdown, or `{ markdown, streaming?, themeVersion? }` when the host says
 *  the producer is still writing it, or that it has just published the deck's
 *  theme. */
export function renderDeck(ctx: RenderInnerCtx): ReactNode {
  const { item } = ctx
  if (typeof item === 'string') return <ThemedDeck source={item} />
  const o = (item && typeof item === 'object' ? item : {}) as Record<string, unknown>
  const markdown = typeof o.markdown === 'string' ? o.markdown : ''
  if (typeof o.deckId === 'string' && o.deckId) {
    return <StoredDeck id={o.deckId} initial={markdown} initialAssets={assetsOf(o.assets)} presenting={o.presenting === true} />
  }
  return (
    <ThemedDeck
      themeVersion={typeof o.themeVersion === 'string' ? o.themeVersion : undefined}
      source={markdown}
      streaming={o.streaming === true}
      scheme={o.scheme === 'dark' ? 'dark' : 'light'}
      annotationsKey={typeof o.annotationsKey === 'string' ? o.annotationsKey : undefined}
      drawings={typeof o.drawings === 'string' ? o.drawings : undefined}
      localAsset={localAssets(assetsOf(o.assets))}
      // The host keeps the drawings (a deck object's named file).
      onDrawingsChange={(drawings) => ctx.emit({ type: 'change', value: { drawings }, dirty: true, valid: true })}
      // The host follows the slide shown (the deck settings panel).
      onSlideChange={(slide) => ctx.emit({ type: 'change', value: { slide }, dirty: false, valid: true })}
      // The host keeps the review (a deck object's `review` named file).
      review={typeof o.review === 'string' ? o.review : undefined}
      reviewer={typeof o.reviewer === 'string' ? o.reviewer : undefined}
      onReviewChange={o.reviewable === true ? (review) => ctx.emit({ type: 'change', value: { review }, dirty: true, valid: true }) : undefined}
      // The slide groups the deck includes, from the host (th22).
      groups={o.groups && typeof o.groups === 'object' ? (o.groups as Record<string, string>) : undefined}
    />
  )
}

/** A deck held by the display server's deck store: starts from the markdown
 *  the page was served with and follows every update over SSE. */
function StoredDeck({ id, initial, initialAssets, presenting }: { id: string; initial: string; initialAssets?: AssetsRef; presenting: boolean }) {
  const [markdown, setMarkdown] = useState(initial)
  const [assets, setAssets] = useState<AssetsRef | undefined>(initialAssets)
  useEffect(() => {
    const token = new URLSearchParams(window.location.search).get('token') ?? ''
    const es = new EventSource(`/widget/deck/stream/${encodeURIComponent(id)}?token=${encodeURIComponent(token)}`)
    es.addEventListener('deck', (e) => {
      try {
        const d = JSON.parse((e as MessageEvent).data)
        setMarkdown(d.markdown ?? '')
        setAssets(assetsOf(d.assets))
      } catch {
        /* a malformed event changes nothing */
      }
    })
    return () => es.close()
  }, [id])
  // `&print-pdf&autoprint=1` (the GUI's PDF button): once reveal.js has laid
  // the slides out as print pages, open the browser's print dialog.
  useEffect(() => {
    const q = new URLSearchParams(window.location.search)
    if (!q.has('print-pdf') || q.get('autoprint') !== '1') return
    let tries = 0
    const t = setInterval(() => {
      tries++
      if (document.querySelector('.pdf-page') || tries > 40) {
        clearInterval(t)
        setTimeout(() => window.print(), 500)
      }
    }, 250)
    return () => clearInterval(t)
  }, [])
  return <ThemedDeck source={markdown} presenting={presenting} annotationsKey={id} localAsset={localAssets(assets)} />
}

const pageToken = () => new URLSearchParams(window.location.search).get('token') ?? ''

/** Where the files beside a deck are in the display server's deck-assets
 *  store (src/deck-assets.ts). */
interface AssetsRef { scope: string; version: string }
const assetsOf = (v: unknown): AssetsRef | undefined => {
  const a = v as { scope?: unknown; version?: unknown } | null | undefined
  return a && typeof a.scope === 'string' && typeof a.version === 'string' ? { scope: a.scope, version: a.version } : undefined
}
const localAssets = (a?: AssetsRef) => a
  ? (p: string) => `/widget/deck/assets/${encodeURIComponent(a.scope)}/${encodeURIComponent(a.version)}/${p.split('/').map(encodeURIComponent).join('/')}?token=${encodeURIComponent(pageToken())}`
  : undefined

/** The theme a deck names, from the display server's theme store
 *  (theme-store.ts) — the host publishes it there. None for katyoti (the
 *  component's own defaults) or a theme not published yet; `themeVersion`
 *  changes when the host has just published it, and it is read again. */
function useDeckTheme(markdown: string, themeVersion?: string): LoadedTheme | undefined {
  const name = useMemo(() => {
    const t = parseDeck(markdown).settings?.theme
    return typeof t === 'string' && t && t !== DEFAULT_THEME ? t : ''
  }, [markdown])
  const [theme, setTheme] = useState<LoadedTheme | undefined>(undefined)
  useEffect(() => {
    if (!name) {
      setTheme(undefined)
      return
    }
    let alive = true
    const token = encodeURIComponent(pageToken())
    const base = `/widget/deck/theme/${encodeURIComponent(name)}`
    fetch(`${base}/theme.json?token=${token}`)
      .then((r) => (r.ok ? r.json() : undefined))
      .then((t: { version: string; body: Theme } | undefined) => {
        if (!alive) return
        if (!t?.body || typeof t.version !== 'string') return setTheme(undefined)
        const version = encodeURIComponent(t.version)
        setTheme({
          body: t.body,
          assetURL: (p) => `${base}/${version}/assets/${p.split('/').map(encodeURIComponent).join('/')}?token=${token}`,
        })
      })
      .catch(() => alive && setTheme(undefined))
    return () => {
      alive = false
    }
  }, [name, themeVersion])
  return theme
}

/** DeckView with the theme its deck names. */
function ThemedDeck({ themeVersion, ...props }: DeckViewProps & { themeVersion?: string }) {
  const theme = useDeckTheme(props.source, themeVersion)
  return <DeckView {...props} theme={theme} />
}

// =================================================================================================
// Kind → renderInner map (used by index.tsx after dispatch.ts picks a kind)
//
// 'proto' is intentionally NOT in this map — the proto-walker fallback
// uses the legacy `Harness` component directly (it has its own internal
// chrome). UniversalHarness handles the 6 typed kinds.
// =================================================================================================

import type { UniversalHarnessKind, RenderInnerCtx as Ctx } from './universal-harness.tsx'

export type TypedKind = Exclude<UniversalHarnessKind, 'proto'>

export const renderByKind: Record<TypedKind, (ctx: Ctx) => ReactNode> = {
  form: renderForm,
  view: renderView,
  table: renderTable,
  'tree-table': renderTreeTable,
  wizard: renderWizard,
  page: renderPage,
  drawio: renderDrawio,
  deck: renderDeck,
}

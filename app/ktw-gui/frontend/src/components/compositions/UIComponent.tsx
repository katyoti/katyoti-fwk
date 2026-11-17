// SPDX-License-Identifier: Apache-2.0
// <UIComponent def={...}> — generic LLM-emit-target wrapper.
//
// Single component name an LLM can emit; the discriminated `def` picks
// the right typed wrapper (UIForm / UIView / future UITable / UIWizard /
// UIDeck / etc.). Keeps the LLM's surface narrow (one import, one tag,
// one JSON shape) while preserving per-kind behavior under the hood.
//
// 2026-05-04 follow-up to gui-view (Option C from the section-shell
// design discussion). Future evolution path: when `llm-streaming-protocol`
// (todo) lands, the streamed message kinds (`schema`, `value`, `patch`,
// etc.) will likely deliver a JSON `def` matching this shape; this
// component then becomes the artifact-mounting layer for streamed
// compositions.
//
// Layering:
//   <UIComponent def={...}>    ← LLM-emit narrow surface (this file)
//        ↓ dispatch on def.kind
//   <UIForm> / <UIView> / ...  ← typed product wrappers
//        ↓
//   <IframeWidget>             ← primitive (URL resolution, sentinel UX, postMessage plumbing)
//
// Human product code SHOULD prefer the typed wrappers (UIForm/UIView) for
// type safety on per-kind props. UIComponent is for runtime dispatch from
// JSON — primarily LLM streams, secondarily dynamic UI configurations.

import type { IframeWidgetStatus } from '../iframe-widget'
import { UIForm, type UIFormMode } from './UIForm'
import { UIView, type UIViewMode } from './UIView'
import { UITable, type UITableMode } from './UITable'
import { UITreeTable, type UITreeTableMode } from './UITreeTable'
import { UIWizard, type UIWizardMode } from './UIWizard'
import { UIPage } from './UIPage'

// =================================================================================================
// Discriminated def — the JSON-shaped contract an LLM (or runtime config)
// emits to mount a composition. Each variant carries the per-kind subset
// of UIForm/UIView/etc. props that are JSON-serializable. Function
// callbacks (onSubmit, onAction, etc.) live OUTSIDE the def, on the host
// component's props — they aren't part of the LLM-emittable JSON.
// =================================================================================================

/** Common fields all schema-bound defs share — the one component vocabulary
 *  (render.Def): the schema inline (a schema named by id is resolved by the
 *  document entry point before it reaches here), the variant of its
 *  `ui-<kind>` extension, and the data. */
interface BaseDef {
  schema?: unknown
  variant?: string
  data?: unknown
}

export interface UIFormDef extends BaseDef {
  kind: 'form'
  mode?: UIFormMode
}

export interface UIViewDef extends BaseDef {
  kind: 'view'
  mode?: UIViewMode
  emptyStrategy?: 'omit' | 'em-dash' | 'explicit-empty'
}

export interface UITableDef extends BaseDef {
  kind: 'table'
  mode?: UITableMode
  filterName?: string
}

export interface UITreeTableDef extends BaseDef {
  kind: 'tree-table'
  mode?: UITreeTableMode
}

export interface UIWizardDef extends BaseDef {
  kind: 'wizard'
  mode?: UIWizardMode
}

/** Page-shaped def — carries a DSL tree (a `UIPageNode`). Used by the
 *  `display-ui-component` tool when the LLM emits a multi-node layout
 *  (vbox/grid/tabs of mixed atomic + schema-bound nodes). The embedded
 *  schema-bound nodes inside `dslTree` render INLINE inside the single
 *  page iframe — no nested iframes (per the 2026-05-05 ADR). For a
 *  single-composition surface (just one form/view/table/wizard), prefer
 *  the dedicated UIFormDef/UIViewDef/UITableDef/UIWizardDef variants for
 *  shorthand. */
export interface UIPageDef {
  kind: 'page'
  /** A DSL tree node — one of 21 kinds across layout/atomic/schema-bound
   *  categories. Typed as `unknown` here because the host doesn't import
   *  ktw-display's type module directly; the bun-side harness validates. */
  dslTree: unknown
  /** Optional outer data tree — `dataField: "$.path"` references inside
   *  the dslTree resolve against this. Forwarded to the iframe via
   *  `?data=<base64>` URL param. Wired by display-package-restructure
   *  Phase 3 (2026-05-06). */
  data?: unknown
}

/** Discriminated union of every ui-component type the iframe renders:
 *  standalone single-composition mounts via the typed wrappers and
 *  tree-shaped compositions via `UIPageDef`. A draw.io diagram is a document
 *  of its own media type (the drawio display engine), not a component. */
export type UIComponentDef =
  | UIFormDef
  | UIViewDef
  | UITableDef
  | UITreeTableDef
  | UIWizardDef
  | UIPageDef

// =================================================================================================
// UIComponent — host props
// =================================================================================================

export interface UIComponentProps {
  /** The composition def — JSON-shaped, picks which typed wrapper to mount. */
  def: UIComponentDef

  /** Optional cross-universe override. Forwarded to the chosen wrapper. */
  universe?: string

  // -- Cross-kind callbacks --
  // Each callback fires only on compositions that emit it (e.g. onSubmit
  // only fires on form; onAction fires on view + table; onChange fires on
  // form). Callbacks for kinds the current def doesn't support are
  // silently ignored at dispatch time.

  /** Fires when the iframe finishes bootstrapping. All kinds. */
  onReady?: () => void
  /** Fires on every form-field value change. Form only. */
  onChange?: (
    value: Record<string, unknown>,
    meta: { dirty: boolean; valid: boolean },
  ) => void
  /** Fires on form submission OR wizard completion. The terminal "save"
   *  callback for both kinds — semantically equivalent (host receives
   *  the validated/merged value object). Wizard maps this internally. */
  onSubmit?: (value: Record<string, unknown>) => void
  /** Fires on item-scope action clicks (edit / delete / etc). View, table. */
  onAction?: (action: string, args: Record<string, unknown>) => void
  /** Fires on iframe-reported runtime errors. All kinds. */
  onError?: (error: { name: string; message: string; stack?: string }) => void
  /** Optional status callback for parents that want to surface an
   *  ok/loading/down badge. */
  onStatusChange?: (status: IframeWidgetStatus, detail?: string) => void

  // -- Layout --

  /** Initial container height in px. Default 480 (matches typed wrappers). */
  initialHeight?: number
  /** Maximum container height in px. Default 1600. */
  maxHeight?: number

  /** Live-update push to the iframe. When this changes, the host posts a
   *  `value` message to the mounted iframe; the harness applies the new
   *  value reactively. Wire this from the LLM-streaming-protocol patch
   *  pipeline so streamed updates render live. All kinds. */
  pushValue?: unknown

  /** When true, the iframe fills 100% of the parent's height (artifact-
   *  tab use case). When false (default), the iframe sizes to content. */
  fillHeight?: boolean

  /** When false, schema-bound wrappers (form / view / table / wizard) +
   *  page wait for streaming to complete before anchoring schema /
   *  dslTree into the iframe URL. Prevents the partial-JSON parser from
   *  freezing the URL on an incomplete intermediate state during stream.
   *  Default `undefined` (treated as true). Pass false from streaming
   *  consumers like DocumentTab during `artifact.status === 'streaming'`. */
  streamingComplete?: boolean
}

/**
 * Mount a composition described by a JSON def. Dispatches to the right
 * typed wrapper based on `def.kind`.
 *
 * @example LLM-emit JSON:
 * ```ts
 * const def = { kind: 'form', schema: BOOK_SCHEMA, mode: 'edit' }
 * <UIComponent def={def} onSubmit={(v) => save(v)} />
 * ```
 *
 * @example Static product code (rare — prefer the typed wrapper directly):
 * ```tsx
 * <UIComponent
 *   def={{ kind: 'view', schema: BOOK_SCHEMA, variant: 'compact' }}
 *   onAction={(action) => console.log(action)}
 * />
 * ```
 */
export function UIComponent(props: UIComponentProps) {
  const {
    def,
    universe,
    onReady,
    onChange,
    onSubmit,
    onAction,
    onError,
    onStatusChange,
    initialHeight,
    maxHeight,
    pushValue,
    fillHeight,
    streamingComplete,
  } = props

  switch (def.kind) {
    case 'form':
      return (
        <UIForm
          schemaInline={def.schema}
          valueInline={def.data}
          mode={def.mode}
          formName={def.variant}
          universe={universe}
          onReady={onReady}
          onChange={onChange}
          onSubmit={onSubmit}
          onError={onError}
          onStatusChange={onStatusChange}
          initialHeight={initialHeight}
          maxHeight={maxHeight}
          pushValue={pushValue}
          fillHeight={fillHeight}
          streamingComplete={streamingComplete}
        />
      )
    case 'view':
      return (
        <UIView
          schemaInline={def.schema}
          valueInline={def.data}
          mode={def.mode}
          viewName={def.variant}
          emptyStrategy={def.emptyStrategy}
          universe={universe}
          onReady={onReady}
          onAction={onAction}
          onError={onError}
          onStatusChange={onStatusChange}
          initialHeight={initialHeight}
          maxHeight={maxHeight}
          pushValue={pushValue}
          fillHeight={fillHeight}
          streamingComplete={streamingComplete}
        />
      )
    case 'table':
      return (
        <UITable
          schemaInline={def.schema}
          valueInline={def.data}
          mode={def.mode}
          tableName={def.variant}
          filterName={def.filterName}
          universe={universe}
          onReady={onReady}
          onRowClick={
            // Map UIComponent's onAction → UITable's onRowClick. The
            // host's `onAction(action, args)` callback fires for both
            // view's edit/delete clicks AND table's row clicks. UITable
            // emits only row clicks; we synthesize an `'click'` action.
            onAction
              ? (item) => onAction('click', { item })
              : undefined
          }
          onError={onError}
          onStatusChange={onStatusChange}
          initialHeight={initialHeight}
          maxHeight={maxHeight}
          pushValue={pushValue}
          fillHeight={fillHeight}
          streamingComplete={streamingComplete}
        />
      )
    case 'tree-table':
      return (
        <UITreeTable
          schemaInline={def.schema}
          valueInline={def.data}
          mode={def.mode}
          treeName={def.variant}
          universe={universe}
          onReady={onReady}
          onRowClick={onAction ? (item) => onAction('click', { item }) : undefined}
          onError={onError}
          onStatusChange={onStatusChange}
          initialHeight={initialHeight}
          maxHeight={maxHeight}
          pushValue={pushValue}
          fillHeight={fillHeight}
          streamingComplete={streamingComplete}
        />
      )
    case 'wizard':
      return (
        <UIWizard
          schemaInline={def.schema}
          valueInline={def.data}
          mode={def.mode}
          wizardName={def.variant}
          universe={universe}
          onReady={onReady}
          // Wizard's onComplete maps to the host's onSubmit — it's the
          // terminal "validated value" callback, semantically the same
          // as form's onSubmit (both receive a finalized value object).
          onComplete={onSubmit}
          onError={onError}
          onStatusChange={onStatusChange}
          initialHeight={initialHeight}
          maxHeight={maxHeight}
          pushValue={pushValue}
          fillHeight={fillHeight}
          streamingComplete={streamingComplete}
        />
      )
    case 'page':
      return (
        <UIPage
          dslTree={def.dslTree}
          data={def.data}
          universe={universe}
          onReady={onReady}
          onSubmit={onSubmit}
          onAction={onAction}
          onError={onError}
          onStatusChange={onStatusChange}
          initialHeight={initialHeight}
          maxHeight={maxHeight}
          pushValue={pushValue}
          fillHeight={fillHeight}
          streamingComplete={streamingComplete}
        />
      )
    default: {
      // Exhaustiveness: when a new composition (gui-wizard, gui-deck) adds
      // a kind to the union, this default arm becomes a TS error until
      // the new case is handled. Explicit dispatch, no silent
      // fall-through.
      const _exhaustive: never = def
      void _exhaustive
      return null
    }
  }
}

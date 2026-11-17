// SPDX-License-Identifier: Apache-2.0
// <UIForm> — typed host wrapper for the bun-side form widget.
//
// Specialization of <IframeWidget> that hardcodes `kind="form"` and
// reshapes the postMessage callback API to be value-centric: hosts pass
// `onSubmit={(value) => ...}` instead of fishing through the raw
// `IframeMessage` union for `msg.type === 'submit'`.
//
// Thin layer (~50 LOC of wiring) on top of <IframeWidget> + the
// LifecycleCallbacks contract from the iframe-widget package — does NOT
// duplicate URL resolution, dev-fallback, origin checks, or sandboxing
// logic. All of those live in the underlying primitive.
//
// Subtask 6 of gui-form (2026-05-03). The bun-side `/widget/form` route
// (Subtask 4) and the auto-detect runtime dispatch (Subtask 4) are what
// this wrapper invisibly drives — the host doesn't care which harness
// the iframe mounts internally; it only sees the submit payload.

import { useCallback, useRef } from 'react'
import {
  IframeWidget,
  type IframeWidgetStatus,
} from '../iframe-widget'
import type { IframeMessage } from '@display/postmessage'
import { encodeSchemaParam } from './encode-schema'
import { isSchemaReady, isValueReady } from './anchor-readiness'

/** Form CRUD modes. The bun-side runtime maps `'create'` → `isNew=true`
 *  and everything else → `isNew=false` (the form surface only
 *  differentiates between create and update modes — `'view'`/`'filter'`
 *  are read-only tokens honored by view/table widgets, not forms). */
export type UIFormMode = 'create' | 'edit' | 'update' | 'view'

export interface UIFormProps {
  /** Optional schema-name lookup (server-side registry). Without a
   *  schemaName or schemaInline, the bun route renders a skeleton
   *  placeholder (it never fabricates demo data). */
  schemaName?: string

  /** Optional value reference — typically a record id the bun side
   *  resolves to a full value object before mounting. */
  value?: string

  /** CRUD mode. Default: `'edit'`. */
  mode?: UIFormMode

  /** Optional cross-universe override. Defaults to the GUI's displayed
   *  universe. Forwarded to <IframeWidget>. */
  universe?: string

  /** Pick a named form variant (when the schema declares multiple
   *  `ui-form` extensions). Forwarded to the iframe URL as `?form=<name>`.
   *  The runtime defaults to `'default'` when omitted. */
  formName?: string

  /** Inline schema injection — used by the LLM-streaming-protocol
   *  artifact mount path. base64url-encoded into `?schema=<...>`; the
   *  bun-side route reads it and uses it instead of its hardcoded demo
   *  schema. Mutually exclusive with `schemaName` (URL injection wins
   *  when both are present). */
  schemaInline?: unknown

  /** Inline initial value (companion to schemaInline). base64url-encoded
   *  into `?value=<...>` for streamed-artifact mounts. */
  valueInline?: unknown

  /** Fired when the iframe finishes bootstrapping. */
  onReady?: () => void

  /** Fired on every field-value change. The shape of `value` follows
   *  the schema's root type. The optional `meta` carries dirty/valid
   *  flags lifted from the underlying ChangeMessage. */
  onChange?: (value: Record<string, unknown>, meta: { dirty: boolean; valid: boolean }) => void

  /** Fired when the user clicks the form's submit action. The `value`
   *  is the full validated form data — this IS the gui-form acceptance
   *  payload ("schema in → validated data object out"). */
  onSubmit?: (value: Record<string, unknown>) => void

  /** Fired on iframe-reported runtime errors. */
  onError?: (error: { name: string; message: string; stack?: string }) => void

  /** Status callback — useful for parents that want to surface a
   *  loading/ok/fallback/down badge. */
  onStatusChange?: (status: IframeWidgetStatus, detail?: string) => void

  /** Initial container height in px (until the iframe sends a resize).
   *  Default 480 — taller than the iframe-widget default (320) because
   *  forms typically have multiple sections. */
  initialHeight?: number

  /** Maximum container height in px — clamps runaway resize messages.
   *  Default 1600. */
  maxHeight?: number

  /** Live-update push: when this changes, the host posts a `value`
   *  message to the iframe. The harness applies the new value reactively.
   *  Used by the LLM-streaming-protocol patch propagation pipeline —
   *  every streamed patch updates the artifact's value, and the demo
   *  page re-renders this prop so the iframe receives the post. */
  pushValue?: unknown

  /** When true, the iframe fills 100% of the parent's height. Use for
   *  tab-embedded mounts where the parent provides bounded dimensions. */
  fillHeight?: boolean

  /** Display mode forwarded to the iframe via `?display=`. `'ui'` (default
   *  for host integrations) strips the harness chrome — only the rendered
   *  widget. `'schema'` shows only the source schema. `'full'` keeps the
   *  legacy debug surface. */
  display?: 'ui' | 'schema' | 'full'

  /** When false, the URL-anchor (schema/value → iframe URL) waits until
   *  the artifact has finished streaming. The partial-JSON parser emits
   *  schema states that satisfy `isSchemaReady` (one field present) long
   *  before the LLM has streamed in the remaining fields; anchoring then
   *  freezes the iframe on a single-field schema. Set this false during
   *  streaming and flip to true on `tool_use_done`. Default `undefined`
   *  (treated as true) keeps non-streaming callers — dev demo pages —
   *  working as before. */
  streamingComplete?: boolean
}

/**
 * Mount the bun-served form widget at `/widget/form` for a given
 * (schemaName, value, mode) tuple. The iframe renders through the new
 * `buildUIForm()` + `<UIFormView>` pipeline (auto-detected runtime-side
 * by the presence of a `ui-form` extension on the schema's root type).
 *
 * @example
 * ```tsx
 * <UIForm
 *   schemaName="Book"
 *   mode="create"
 *   onSubmit={(value) => save(value)}
 *   onChange={(value, meta) => console.log('dirty:', meta.dirty)}
 * />
 * ```
 */
export function UIForm({
  schemaName,
  value,
  mode = 'edit',
  universe,
  formName,
  schemaInline,
  valueInline,
  onReady,
  onChange,
  onSubmit,
  onError,
  onStatusChange,
  initialHeight = 480,
  maxHeight = 1600,
  pushValue,
  fillHeight,
  display,
  streamingComplete,
}: UIFormProps) {
  // Mount-time + every-render diagnostic. Helps identify why the form
  // sometimes renders empty after a tab-switch + return. Logs the
  // anchor decision (whether the schema URL got captured this render).
  if (typeof window !== 'undefined') {
    // eslint-disable-next-line no-console
    console.debug('[UIForm] render', {
      schemaInlineKind: schemaInline === undefined ? 'undefined' : schemaInline === null ? 'null' : typeof schemaInline,
      valueInlineKind:  valueInline  === undefined ? 'undefined' : valueInline  === null ? 'null' : typeof valueInline,
      streamingComplete,
      isSchemaReadyResult: isSchemaReady(schemaInline),
      isValueReadyResult: isValueReady(valueInline),
    })
  }

  const onIframeMessage = useCallback(
    (msg: IframeMessage) => {
      switch (msg.type) {
        case 'ready':
          onReady?.()
          return
        case 'change':
          onChange?.(msg.value, { dirty: msg.dirty ?? false, valid: msg.valid ?? true })
          return
        case 'submit':
          onSubmit?.(msg.value)
          return
        case 'error':
          onError?.(msg.error)
          return
        // 'resize' is handled inside <IframeWidget> (sets the container
        // height); 'request-value' / 'action' aren't part of the form
        // surface contract — silently ignored.
      }
    },
    [onReady, onChange, onSubmit, onError],
  )

  // CAPTURE INITIAL schemaInline + valueInline ONCE for URL injection.
  // Live updates MUST flow via `pushValue` (postMessage) — putting the
  // live value in extraParams would cause IframeWidget to rebuild the
  // URL on every render, fully remounting the iframe (the bun runtime
  // re-bundles + drawio/recharts/etc re-init each remount). For
  // streaming patches (the chat panel's replaces) this saturates the
  // renderer and visually flickers. Encoding once here keeps
  // `JSON.stringify(extraParams)` stable across re-renders.
  // See `modules/display/host-iframe-wire.md#rules`.
  //
  // For streaming-mounted artifacts (display-ui-component), schemaInline
  // arrives `undefined` on the first mount (partial-JSON parse hasn't
  // seen it yet) then fills in via a later patch. We deliberately DO NOT
  // lock an empty capture — wait for a meaningful value to arrive, then
  // freeze. The iframe remounts once when schema/value materialize, then
  // stays stable for the rest of the artifact's lifetime.
  const initialSchemaEnc = useRef<string | null>(null)
  const initialValueEnc = useRef<string | null>(null)
  // Anchor schema only when (a) it's "ready" (has types + non-empty
  // fields) AND (b) streaming has completed. The partial-JSON parser
  // emits intermediate states like `{name, root, types: {Foo: {fields: {id}}}}`
  // that pass `isSchemaReady` (one field is technically ready) but are
  // missing the other 7 fields the LLM is still streaming in. Anchoring
  // there freezes the iframe URL on a single-field schema and the rest
  // never appears. Waiting for `streamingComplete` ensures we anchor on
  // the FINAL schema. Default true keeps non-streaming callers (the dev
  // demo pages) working as before. See ./anchor-readiness.ts.
  if (
    initialSchemaEnc.current === null &&
    (streamingComplete ?? true) &&
    isSchemaReady(schemaInline)
  ) {
    const enc = encodeSchemaParam(schemaInline)
    if (enc) {
      initialSchemaEnc.current = enc
      // eslint-disable-next-line no-console
      console.debug('[UIForm] schema anchored', { encLen: enc.length })
    }
  }
  if (
    initialValueEnc.current === null &&
    (streamingComplete ?? true) &&
    isValueReady(valueInline)
  ) {
    const enc = encodeSchemaParam(valueInline)
    if (enc) {
      initialValueEnc.current = enc
      // eslint-disable-next-line no-console
      console.debug('[UIForm] value anchored', { encLen: enc.length })
    }
  }

  const extraParams: Record<string, string> = {}
  // Default to 'ui' — host-embedded iframes want the widget alone, no
  // debug chrome. The bun harness's own default is 'full' (legacy debug
  // view with schema panel), which is wrong for production hosts: it
  // shows the schema instead of the rendered form. Dev demo pages can
  // opt into 'schema' or 'full' explicitly.
  extraParams['display'] = display ?? 'ui'
  if (formName) extraParams['form'] = formName
  if (initialSchemaEnc.current) extraParams['schema'] = initialSchemaEnc.current
  if (initialValueEnc.current) extraParams['value'] = initialValueEnc.current

  return (
    <IframeWidget
      kind="form"
      schemaName={schemaName}
      value={value}
      mode={mode}
      universe={universe}
      extraParams={extraParams}
      onIframeMessage={onIframeMessage}
      onStatusChange={onStatusChange}
      initialHeight={initialHeight}
      maxHeight={maxHeight}
      pushValue={pushValue}
      fillHeight={fillHeight}
    />
  )
}

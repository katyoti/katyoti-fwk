// SPDX-License-Identifier: Apache-2.0
// <UIWizard> — typed host wrapper for the bun-side wizard widget.
//
// Specialization of <IframeWidget> for `kind="wizard"`. Reshapes the
// postMessage `submit` event to a typed `onComplete(value)` callback —
// hosts pass the merged-accumulator handler directly without unwrapping
// the IframeMessage union.
//
// Sister to <UIForm> + <UIView> + <UITable>. Same layering principle.

import { useCallback, useRef } from 'react'
import {
  IframeWidget,
  type IframeWidgetStatus,
} from '../iframe-widget'
import type { IframeMessage } from '@display/postmessage'
import { encodeSchemaParam } from './encode-schema'
import { isSchemaReady, isValueReady } from './anchor-readiness'

/** Wizard CRUD modes. The bun-side runtime accepts `'create'` (typical
 *  onboarding) or `'edit'` (re-running the wizard against an existing
 *  record). */
export type UIWizardMode = 'create' | 'edit'

export interface UIWizardProps {
  /** Optional schema-name lookup. Omit to render the bundled demo schema. */
  schemaName?: string

  /** Optional value reference — typically a record id when re-running
   *  the wizard against existing data (mode='edit'). */
  value?: string

  /** CRUD mode. Default: `'create'`. */
  mode?: UIWizardMode

  /** Cross-universe override. */
  universe?: string

  /** Pick a named wizard variant. Forwarded as `?wizard=<name>`. */
  wizardName?: string

  /** Inline schema injection (LLM-streaming-protocol artifact mount). */
  schemaInline?: unknown
  /** Inline value (companion to schemaInline). */
  valueInline?: unknown

  /** Fired when the iframe finishes bootstrapping. */
  onReady?: () => void

  /** Fired when the user clicks Save on the FINAL step. The `value` is
   *  the fully merged accumulator (every step's submitted fields rolled
   *  into one object). This is the wizard's terminal callback — analogous
   *  to UIForm's onSubmit. */
  onComplete?: (value: Record<string, unknown>) => void

  /** Fired on iframe-reported runtime errors. */
  onError?: (error: { name: string; message: string; stack?: string }) => void

  /** Status callback for parent debug panels. */
  onStatusChange?: (status: IframeWidgetStatus, detail?: string) => void

  /** Initial container height in px. Default 600 (taller than form/view). */
  initialHeight?: number

  /** Maximum container height in px. Default 2000. */
  maxHeight?: number

  /** Live-update push (LLM-streaming-protocol). See UIForm for details. */
  pushValue?: unknown

  /** When true, the iframe fills 100% of the parent's height. */
  fillHeight?: boolean

  /** Display mode forwarded via `?display=`. */
  display?: 'ui' | 'schema' | 'full'

  /** Defer URL anchoring until streaming completes — see UIForm. */
  streamingComplete?: boolean
}

/**
 * Mount the bun-served wizard widget at `/widget/wizard`. Renders a
 * multi-step form whose final-step Save emits the merged accumulator
 * back to the host as `onComplete(value)`.
 *
 * @example
 * ```tsx
 * <UIWizard
 *   schemaName="OnboardingProfile"
 *   onComplete={(profile) => createUser(profile)}
 * />
 * ```
 */
export function UIWizard({
  schemaName,
  value,
  mode = 'create',
  universe,
  wizardName,
  schemaInline,
  valueInline,
  onReady,
  onComplete,
  onError,
  onStatusChange,
  initialHeight = 600,
  maxHeight = 2000,
  pushValue,
  fillHeight,
  display,
  streamingComplete,
}: UIWizardProps) {
  const onIframeMessage = useCallback(
    (msg: IframeMessage) => {
      switch (msg.type) {
        case 'ready':
          onReady?.()
          return
        case 'submit':
          // Wizard final-step Save → merged accumulator value.
          onComplete?.(msg.value as Record<string, unknown>)
          return
        case 'error':
          onError?.(msg.error)
          return
      }
    },
    [onReady, onComplete, onError],
  )

  // Capture initial schemaInline + valueInline once — see iframe-stable-
  // extra-params pattern (`modules/display/host-iframe-wire.md#rules`). Live updates flow
  // via `pushValue` postMessage; URL stays stable so the iframe doesn't
  // remount on every render.
  //
  // For streaming-mounted artifacts (display-ui-component), schemaInline
  // arrives `undefined` on the first mount (partial-JSON parse hasn't
  // seen it yet) then fills in via a later patch. We deliberately DO NOT
  // lock an empty capture — wait for a meaningful value to arrive, then
  // freeze. The iframe remounts once when schema/value materialize, then
  // stays stable for the rest of the artifact's lifetime.
  const initialSchemaEnc = useRef<string | null>(null)
  const initialValueEnc = useRef<string | null>(null)
  if (
    initialSchemaEnc.current === null &&
    (streamingComplete ?? true) &&
    isSchemaReady(schemaInline)
  ) {
    const enc = encodeSchemaParam(schemaInline)
    if (enc) initialSchemaEnc.current = enc
  }
  if (
    initialValueEnc.current === null &&
    (streamingComplete ?? true) &&
    isValueReady(valueInline)
  ) {
    const enc = encodeSchemaParam(valueInline)
    if (enc) initialValueEnc.current = enc
  }

  const extraParams: Record<string, string> = {}
  extraParams['display'] = display ?? 'ui'
  if (wizardName) extraParams['wizard'] = wizardName
  if (initialSchemaEnc.current) extraParams['schema'] = initialSchemaEnc.current
  if (initialValueEnc.current) extraParams['value'] = initialValueEnc.current
  // While no schema has anchored, the iframe mounts with no `?schema=`
  // param; the bun route renders the shared loading screen until the host
  // swaps in the real schema (URL change → reload).

  return (
    <IframeWidget
      kind="wizard"
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

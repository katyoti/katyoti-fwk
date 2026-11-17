// SPDX-License-Identifier: Apache-2.0
/**
 * The GUI's KTSP access point.
 *
 * ONE generic Wails binding (`CallKTSPMethod`) carries every service call; the
 * typing lives in the generated clients beside this file. Publishing a service
 * method therefore needs no Go change in ktw-gui — regenerate and call it typed.
 *
 * See rule `management-plane-hub-authority` invariant 5. Do NOT add a Wails
 * binding per service method; do NOT hand-edit the generated clients.
 *
 *   const mkt = marketplaceClient(ADMIN_UNIVERSE)
 *   const out = await mkt.applyConfigSet({ name: 'acme-base', values })
 */

import { type KtspDispatch, makeMarketplaceClient } from './marketplace'
import { makeOrchestratorClient } from './orchestrator'
import { makeProfileClient } from './profile'
import { makeCatalogServiceClient } from './catalog-service'
import { makePanelServiceClient } from './panel-service'

/** The control-plane universe hosting the management slots. */
export const ADMIN_UNIVERSE = 'admin'

/** What the Wails binding returns: exactly one of response / error. */
interface CallResult {
  response?: unknown
  error?: string
  /** The error by key, when the service gave one (i18n/errors.ts). */
  errorMsg?: { key: string; params?: Record<string, unknown> }
}

/** Thrown when the service (or the transport) reports a failure. Carries the
 *  slot path so a caller does not have to reconstruct which call failed, and
 *  the refusal's message key when the service gave one — `errorText(err)`
 *  (i18n/errors.ts) says it in the person's language. */
export class KtspError extends Error {
  constructor(
    readonly slotPath: string,
    message: string,
    readonly msg?: { key: string; params?: Record<string, unknown> },
  ) {
    super(`${slotPath}: ${message}`)
    this.name = 'KtspError'
  }
}

/** Binds a dispatcher to one universe. Errors REJECT rather than resolving to an
 *  `{error}` shape — a typed client whose happy path can silently be an error
 *  object is worse than no typing at all. */
export function dispatcherFor(universe: string): KtspDispatch {
  return async (slotPath: string, input: unknown): Promise<unknown> => {
    const win = window as unknown as {
      go?: {
        main?: {
          App?: {
            CallKTSPMethod?: (u: string, slot: string, inputJSON: string) => Promise<CallResult>
          }
        }
      }
    }
    const call = win.go?.main?.App?.CallKTSPMethod
    if (!call) {
      // Dev/mock outside Wails — fail loudly rather than resolve undefined into a
      // typed shape the caller will then destructure.
      throw new KtspError(slotPath, 'Wails bridge unavailable (CallKTSPMethod missing)')
    }
    const res = await call(universe, slotPath, input === undefined ? '' : JSON.stringify(input))
    if (res?.error) throw new KtspError(slotPath, res.error, res.errorMsg)
    return res?.response
  }
}

/** Typed marketplace client against the admin (control-plane) universe. */
export function marketplaceClient(universe: string = ADMIN_UNIVERSE) {
  return makeMarketplaceClient(dispatcherFor(universe))
}

/** Typed universe-registry client. Management slots live in the admin universe;
 *  pass a universe name to address that universe's own services instead. */
export function orchestratorClient(universe: string = ADMIN_UNIVERSE) {
  return makeOrchestratorClient(dispatcherFor(universe))
}

/** Typed client for the profile of the person using the GUI (admin universe). */
export function profileClient(universe: string = ADMIN_UNIVERSE) {
  return makeProfileClient(dispatcherFor(universe))
}

/** Typed client for a universe's catalog on its hub (`ktw-hub:catalog-service`):
 *  the index, the content lanes and the translation bundle. */
export function catalogClient(universe: string) {
  return makeCatalogServiceClient(dispatcherFor(universe))
}

/** Typed client for a universe's panel server on its hub
 *  (`ktw-hub:panel-service`): a display panel's snapshot, its
 *  log, the panels kept. */
export function panelClient(universe: string) {
  return makePanelServiceClient(dispatcherFor(universe))
}

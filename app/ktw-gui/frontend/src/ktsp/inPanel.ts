// SPDX-License-Identifier: Apache-2.0
/**
 * A control-plane call drawn in its panel.
 *
 * The welcome screen's and the Marketplace's installs (install, update,
 * set-version, install-solution, install-prereqs, doctor) are ktw-univers slots
 * of the admin universe, which has no action surface. ktw-gui opens a
 * progress panel itself on that
 * universe's panel server (`OpenCallPanel`), the screen draws it (DisplayPanel)
 * at once, and the call is made lent that panel (`CallKTSPMethodInPanel`, the
 * ktw.meta.panel header): the method's steps go into it, its end completes it.
 * Cancel - the panel's own, or `cancelCall` (WorkspaceCancelCall) - gives the
 * call up.
 *
 * Still the ONE generic dispatcher per universe: the typed client is the
 * generated one, over a dispatcher that lends a panel to each of its calls.
 */

import { type KtspDispatch, makeMarketplaceClient } from './marketplace'
import { ADMIN_UNIVERSE, KtspError } from './index'

/** The panel a call was lent, as the screen draws it. */
export interface CallPanel {
  universe: string
  panelId: string
  callId: string
  title: string
}

interface CallResult {
  response?: unknown
  error?: string
  errorMsg?: { key: string; params?: Record<string, unknown> }
}

/** The Wails bindings a call in its panel goes through. */
export interface CallPanelBridge {
  OpenCallPanel: (universe: string, title: string, callId: string) => Promise<string>
  CallKTSPMethodInPanel: (universe: string, slotPath: string, inputJSON: string, callId: string) => Promise<CallResult>
  WorkspaceCancelCall?: (callId: string) => Promise<void>
}

/** The bindings, when this GUI has them (a GUI built before them has not). */
export function callPanelBridge(): CallPanelBridge | undefined {
  const app = (globalThis as unknown as { window?: { go?: { main?: { App?: Partial<CallPanelBridge> } } } })
    .window?.go?.main?.App
  if (!app?.OpenCallPanel || !app.CallKTSPMethodInPanel) return undefined
  return app as CallPanelBridge
}

let callSeq = 0
function nextCallId(): string {
  callSeq += 1
  return `mkt-${Date.now().toString(36)}-${callSeq}`
}

/** The method a slot path names: `ktw-univers:marketplace/install` → `install`. */
const methodOf = (slotPath: string) => slotPath.slice(slotPath.lastIndexOf('/') + 1)

/** A dispatcher lending each call a panel the GUI opens: `onPanel` hears the
 *  panel before the call starts - what the screen draws - and the call then
 *  answers, or rejects, as the direct one does. `title` names the panel (by
 *  default its method). */
export function inPanelDispatcherFor(
  universe: string,
  onPanel: (p: CallPanel) => void,
  title?: (slotPath: string, input: unknown) => string,
  bridge: CallPanelBridge | undefined = callPanelBridge(),
): KtspDispatch {
  return async (slotPath: string, input: unknown): Promise<unknown> => {
    if (!bridge) throw new KtspError(slotPath, 'Wails bridge unavailable (CallKTSPMethodInPanel missing)')
    const callId = nextCallId()
    const name = title?.(slotPath, input) || methodOf(slotPath)
    const panelId = await bridge.OpenCallPanel(universe, name, callId)
    onPanel({ universe, panelId, callId, title: name })
    const res = await bridge.CallKTSPMethodInPanel(universe, slotPath, input === undefined ? '' : JSON.stringify(input), callId)
    if (res?.error) throw new KtspError(slotPath, res.error, res.errorMsg)
    return res?.response
  }
}

/** The marketplace client the installs use: each call lent a panel drawn by
 *  the screen. */
export function installClient(
  onPanel: (p: CallPanel) => void,
  title?: (slotPath: string, input: unknown) => string,
  bridge: CallPanelBridge | undefined = callPanelBridge(),
  universe: string = ADMIN_UNIVERSE,
) {
  return makeMarketplaceClient(inPanelDispatcherFor(universe, onPanel, title, bridge))
}

/** Gives up the call a panel was lent (WorkspaceCancelCall). */
export function cancelCall(p: CallPanel, bridge: CallPanelBridge | undefined = callPanelBridge()): void {
  void bridge?.WorkspaceCancelCall?.(p.callId)
}

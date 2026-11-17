// SPDX-License-Identifier: Apache-2.0
// What the person sends a display panel and which of its controls take input:
// the pure part of DisplayPanel. A panel never disables the rest
// of the screen - only its own controls wait, while their input goes out or
// while the panel is not followed live.

import { panelAnswer, panelAskForm, panelCancel, type PanelView } from '@ktw/display-lib/ui-builders/page/panel.ts'
import type { UIPageNode } from '@ktw/display-lib/ui-builders/page/index.ts'

import type { PanelInput } from '../../lib/displayPanels'

/** The chat panel type. */
export const CHAT_PANEL_TYPE = 'ktw-agents:/panel-type/chat'

/** isChatPanel tells a run's conversation: a message box under it. */
export function isChatPanel(v: Pick<PanelView, 'type'>): boolean {
  return v.type === CHAT_PANEL_TYPE || /\/panel-type\/chat$/.test(v.type ?? '')
}

/** answerInput is the person's answer to the panel's question: the action
 *  chosen among those it offers, else the value its form submitted, else the
 *  text written - the display library's panelAnswer, the one the inline panel
 *  node sends. */
export function answerInput(view: Pick<PanelView, 'id' | 'ask'>, answer: { action?: string; value?: unknown; text?: string }, by?: string): PanelInput | null {
  return panelAnswer(view, answer, by)
}

/** askFormNode is the form a question carrying one draws: the
 *  display library's panelAskForm, the one the inline panel node draws;
 *  undefined when the question offers choices or carries no form. */
export function askFormNode(ask: PanelView['ask']): UIPageNode | undefined {
  return panelAskForm(ask)
}

/** inputBy names the person at the screen on an input that names no one: an
 *  input the display library's panel node sends carries no `by`. */
export function inputBy(input: PanelInput, by?: string): PanelInput {
  return input.by || !by ? input : { ...input, by }
}

/** messageInput is one chat turn; null for an empty one. */
export function messageInput(panel: string, text: string, by?: string): PanelInput | null {
  const t = text.trim()
  return t === '' ? null : { panel, kind: 'message', text: t, ...(by ? { by } : {}) }
}

/** cancelInput asks the owning run to stop its work. */
export function cancelInput(panel: string, by?: string): PanelInput {
  return panelCancel(panel, by)
}

/** The panel's own controls: whether each takes input now. */
export interface PanelControls {
  answer: boolean
  cancel: boolean
  message: boolean
}

/** panelControls says which of the panel's controls take input: the answer
 *  while it asks, cancel while its work runs, the message box of a chat
 *  until it ends - none while it is not followed live or an input of it is
 *  on its way. Nothing else on the screen depends on it. */
export function panelControls(view: Pick<PanelView, 'state' | 'type'>, live: boolean, sending: boolean): PanelControls {
  const ready = live && !sending
  const running = view.state === 'open' || view.state === 'asking'
  return {
    answer: ready && view.state === 'asking',
    cancel: ready && running,
    message: ready && running && isChatPanel(view),
  }
}

/** progressShare is the bar's width in percent; 0 when the total is unknown. */
export function progressShare(p: PanelView['progress']): number {
  if (!p || !p.total || p.total <= 0) return 0
  return Math.max(0, Math.min(100, Math.round((p.current / p.total) * 100)))
}

/** panelTabId is the tab a panel opens in, by its element address. */
export function panelTabId(panel: string): string {
  return `panel:${panel}`
}

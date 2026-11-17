// SPDX-License-Identifier: Apache-2.0
/**
 * openPanelTab — a display panel in a tab of its own, like a document:
 * one tab per panel, by its element address `panel:<id>`; a
 * panel already open is focused.
 */
import { useTabsStore } from '../../store/tabsSlice'
import { tFor } from '../../i18n'
import { panelTabId } from './panelInput'

/** What a panel tab carries: the panel it shows. */
export interface PanelTabPayload {
  panelId: string
  title?: string
}

export function openPanelTab(universeName: string, panelId: string, title?: string): void {
  const payload: PanelTabPayload = { panelId, ...(title ? { title } : {}) }
  useTabsStore.getState().openTab(universeName, { id: panelTabId(panelId), kind: 'panel', label: title ?? tFor('components')('displayPanel.tab'), closeable: true, payload })
}

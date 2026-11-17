// SPDX-License-Identifier: Apache-2.0
/** panelOf finds a declared panel among an object's tabs by its stem
 *  (`tab:<stem>`) or its strip id - what a page's drawer action names. */
export function panelOf<T extends { id: string }>(tabs: T[] | undefined, panel: string): T | undefined {
  return (tabs ?? []).find((t) => t.id === panel || t.id === `tab:${panel}`)
}

/** isHeaderPanel: a declared panel that is the object's header, drawn above
 *  its tabs, never a tab. */
export function isHeaderPanel(t: object): boolean {
  return (t as { header?: boolean }).header === true
}

/** A declared panel's refresh, as usePanelRefresh reads it. */
export interface HeaderRefresh {
  scope?: string
  types?: string[]
}

/** headerPanelRefresh is when a header panel re-reads:
 *  as declared, and at least on its own object's events - a header declaring
 *  none follows `self`, so its stepper and next action move as soon as the
 *  object does. */
export function headerPanelRefresh(refresh: HeaderRefresh | undefined | null): HeaderRefresh {
  if (!refresh?.scope || refresh.scope === 'none') return { ...(refresh ?? {}), scope: 'self' }
  return refresh
}

/** drawsGenericHeader says whether the pane draws its generic header:
 * always, but where the one-header display shows one header
 *  only - there a declared header panel takes its place, the pane's actions
 *  kept beside it; the generic one comes back only while the object's core
 *  is edited, which it holds. */
export function drawsGenericHeader(oneHeader: boolean, headerPanels: number, editingCore: boolean): boolean {
  return !oneHeader || headerPanels === 0 || editingCore
}

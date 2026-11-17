// SPDX-License-Identifier: Apache-2.0
/**
 * openObjectTab — wire object selection to the single Object tab.
 *
 * ONE tab per universe, whose payload swaps on the next selection. Each swap
 * records a step back (objectTabHistory): what the tab showed and the tab the
 * person was on, so closing the tab returns there — a module's panel, the
 * Inbox, a Global panel — as it was. It lives in its own module so the tree,
 * the Links section and the children lists can all open an object without
 * importing the pane that renders it (and without a cycle). The open object
 * is also the scope a chat launched now works on.
 */
import { useTabsStore } from '../../store/tabsSlice'
import { useAppStore } from '../../store'
import type { ObjectTabPayload } from './ObjectTab'
import { objectTabLabel } from './objectTabTitle'
import { tFor } from '../../i18n'
import { backMove, openedObjectPayload } from './objectTabHistory'
import { objectTabId } from '../../elements'
import { paneFocusOf } from './paneFocus'

export function openObjectTab(
  universeName: string,
  target: { id: string; title?: string; panel?: string; query?: Record<string, unknown>; element?: string },
): void {
  // `_target` in a chat's plugin context — the object-context plugin reads it as
  // an object id.
  const app = useAppStore.getState()
  app.selectNode(target.id)
  app.setActiveNodeName(target.title ?? null)
  const { getForUniverse, openTab, updateTabPayload, setActive } = useTabsStore.getState()
  // No title (a page navigating by id): the tab names itself once the object's
  // detail is read (ObjectTab, retitledObjectPayload).
  const label = objectTabLabel(target.title, tFor('workspace'))
  // `panel` opens the pane on one tab or section: a panel's stem, or a full
  // tab id (`tab:children/<type>`, `section:fields`) - every tab is reachable.
  const asked: ObjectTabPayload = {
    objectId: target.id, objectTitle: target.title,
    ...(target.panel ? { activeKind: objectTabId(target.panel) } : {}),
    // `query` opens that panel as a query action would (its source read with
    // these args over its input): the Files tab with one file selected.
    ...(target.panel && target.query ? { panelQuery: { panel: objectTabId(target.panel), query: target.query } } : {}),
    // `element` brings an element of that panel into view.
    ...paneFocusOf(target, Date.now()),
  }
  const current = getForUniverse(universeName)
  const payload = openedObjectPayload(asked, current)
  if (current.openTabs.some((t) => t.id === 'object')) {
    updateTabPayload(universeName, 'object', payload, label)
    setActive(universeName, 'object')
    return
  }
  openTab(universeName, { id: 'object', kind: 'object', label, closeable: true, payload })
}

/**
 * closeObjectTab — the Object tab's close (and back): it takes the last step
 * back, showing the object, panel and query the tab showed before and
 * focusing the tab the person came from; with no step left it closes.
 */
export function closeObjectTab(universeName: string, tabId = 'object'): void {
  const { getForUniverse, closeTab, updateTabPayload, setActive } = useTabsStore.getState()
  const move = backMove(getForUniverse(universeName), tabId)
  switch (move.kind) {
    case 'close':
      closeTab(universeName, tabId)
      return
    case 'close-to':
      closeTab(universeName, tabId)
      setActive(universeName, move.activate)
      return
    case 'restore': {
      updateTabPayload(universeName, tabId, move.payload, move.label)
      setActive(universeName, move.activate)
      // The restored object is the scope a chat launched now works on.
      const app = useAppStore.getState()
      app.selectNode(move.payload.objectId)
      app.setActiveNodeName(move.payload.objectTitle ?? null)
    }
  }
}

// SPDX-License-Identifier: Apache-2.0
/**
 * Opens what an address names: the universe is activated first
 * when it is not the one shown, then the object tab, Home's entry or the
 * screen is opened on its section. An address that opens nothing is said in
 * the console and leaves the GUI as it is.
 */
import { useAppStore } from '../../store'
import { useTabsStore } from '../../store/tabsSlice'
import { parseAddress, homeStripId, type AddressTarget } from '../../address'
import { openObjectTab } from '../workspace/openObjectTab'

/** The tab each screen opens, with its payload. */
function openScreen(universe: string, t: Extract<AddressTarget, { kind: 'screen' }>): void {
  const { openTab, updateTabPayload } = useTabsStore.getState()
  switch (t.screen) {
    case 'settings':
      useAppStore.getState().openSettingsModal((t.section ?? 'general') as never)
      return
    case 'marketplace':
      openTab(universe, { id: 'marketplace', kind: 'marketplace', label: 'Marketplace', closeable: true, payload: { section: t.section ?? 'sidecars' } })
      if (t.section) updateTabPayload(universe, 'marketplace', { section: t.section })
      return
    case 'debug':
      openTab(universe, { id: 'debug', kind: 'debug', label: 'Debug', closeable: true, payload: t.section ? { section: t.section } : {} })
      if (t.section) updateTabPayload(universe, 'debug', { section: t.section })
      return
    case 'inbox':
      openTab(universe, { id: 'inbox', kind: 'inbox', label: 'Inbox', closeable: true, payload: {} })
      return
    case 'guide':
      openTab(universe, { id: 'guide', kind: 'guide', label: 'Guide', closeable: true, payload: { topics: [] } })
      return
    case 'ready':
      // Its section is the tab it opens on: profile, programs or keys.
      openTab(universe, { id: 'ready', kind: 'ready', label: 'ready', closeable: true, payload: t.section ? { section: t.section } : {} })
      if (t.section) updateTabPayload(universe, 'ready', { section: t.section })
      return
  }
}

/** Opens an address; false (with why in the console) when it opens nothing. */
export async function openAddress(raw: string): Promise<boolean> {
  const t = parseAddress(raw)
  if ('error' in t) {
    // eslint-disable-next-line no-console
    console.warn('[address]', t.error)
    return false
  }
  const app = useAppStore.getState()
  if (!app.universes.some((u) => u.name === t.universe)) {
    // eslint-disable-next-line no-console
    console.warn('[address] no universe', t.universe)
    return false
  }
  if (app.displayedUniverseName !== t.universe) await app.activateUniverse(t.universe)
  switch (t.kind) {
    case 'object':
      // A section is shown inside its tab (Information): the tab opens.
      openObjectTab(t.universe, { id: t.id, panel: t.tab })
      return true
    case 'home': {
      const { setActive, updateTabPayload, getForUniverse } = useTabsStore.getState()
      const home = getForUniverse(t.universe).openTabs.find((x) => x.id === 'home')
      updateTabPayload(t.universe, 'home', { ...(home?.payload ?? {}), activeKind: homeStripId(t.entry) })
      setActive(t.universe, 'home')
      return true
    }
    case 'screen':
      openScreen(t.universe, t)
      return true
  }
}

// SPDX-License-Identifier: Apache-2.0
/**
 * An element's address (the study gui.element-addresses §3): the
 * path from a universe to what is shown, `<universe>/<place>/<path>`.
 *
 *   dev/object/m.F-23                          an object, on its type's first tab
 *   dev/object/m.F-23/links                    an object's tab (its panel stem)
 *   dev/object/m.F-23/children/record          an object's children of a type
 *   dev/object/m.F-23/information/fields       a section of its Information
 *   dev/object/m.F-23/information/cap/plan     a capability's section
 *   archi/home/mes-etudes                      an entry of Home's strip
 *   archi/screen/marketplace/sidecars          a screen and its section
 *   archi/rail/inbox                           what a rail entry opens
 *
 * The object id is data and is kept as is (its dots included). A link from
 * outside the GUI is `katyoti://open/<address>`; ktw-gui starts on one with
 * `--open <address>`, and the development page takes it as
 * `#/open/<address>`. Pure: openAddress.ts applies what this parses.
 */

/** The screens an address opens, each with the sections it may name. */
export const SCREENS = ['inbox', 'marketplace', 'debug', 'guide', 'settings', 'ready'] as const
export type Screen = (typeof SCREENS)[number]

/** What an address opens. */
export type AddressTarget =
  | { kind: 'object'; universe: string; id: string; tab?: string; section?: string }
  | { kind: 'home'; universe: string; entry: string }
  | { kind: 'screen'; universe: string; screen: Screen; section?: string }

/** The rail entries an address can name, and the screen each opens. */
const RAIL_SCREENS: Record<string, Screen> = {
  inbox: 'inbox', marketplace: 'marketplace', debug: 'debug', settings: 'settings',
}

const LINK = 'katyoti://open/'

/** An address without its link prefix or the page's `#/open/`. */
export function addressOf(raw: string): string {
  let s = raw.trim()
  if (s.startsWith(LINK)) s = s.slice(LINK.length)
  else if (s.startsWith('#/open/')) s = s.slice('#/open/'.length)
  try {
    s = decodeURIComponent(s)
  } catch {
    // a malformed escape is read as written
  }
  return s.replace(/^\/+|\/+$/g, '')
}

/** What an address opens, or why it opens nothing. */
export function parseAddress(raw: string): AddressTarget | { error: string } {
  const segs = addressOf(raw).split('/').filter(Boolean)
  const [universe, place, ...rest] = segs
  if (!universe || !place) return { error: `"${raw}" is no address: want <universe>/<place>/<path>` }
  switch (place) {
    case 'object': {
      const [id, tab, ...more] = rest
      if (!id) return { error: `"${raw}": an object address names the object` }
      // No tab: the pane opens on the object's first tab, as from a card.
      if (!tab) return { kind: 'object', universe, id }
      if (tab === 'children') {
        if (!more[0]) return { error: `"${raw}": children names their type` }
        return { kind: 'object', universe, id, tab: `tab:children/${more[0]}` }
      }
      if (more.length === 0) return { kind: 'object', universe, id, tab: `tab:${tab}` }
      // A section lives in the Information tab: `information/fields`,
      // `information/cap/<capability>`.
      const section = more[0] === 'cap' && more[1] ? `section:cap/${more[1]}` : `section:${more[0]}`
      return { kind: 'object', universe, id, tab: `tab:${tab}`, section }
    }
    case 'home':
      return { kind: 'home', universe, entry: rest[0] ?? 'actions' }
    case 'screen': {
      const screen = rest[0] as Screen
      if (!(SCREENS as readonly string[]).includes(screen)) return { error: `"${raw}": no screen "${rest[0] ?? ''}" (${SCREENS.join(', ')})` }
      return { kind: 'screen', universe, screen, ...(rest[1] ? { section: rest[1] } : {}) }
    }
    case 'rail': {
      const screen = RAIL_SCREENS[rest[0] ?? '']
      if (!screen) return { error: `"${raw}": the rail entry "${rest[0] ?? ''}" opens no screen` }
      return { kind: 'screen', universe, screen }
    }
    default:
      return { error: `"${raw}": no place "${place}" (object, home, screen, rail)` }
  }
}

/** A guide step's `open` (an address in the person's universe, without it)
 *  as a full address in the universe shown. */
export function stepAddress(universe: string, open: string): string {
  return `${universe}/${open.trim().replace(/^\/+/, '')}`
}

/** Home's strip id an address's entry names: its own entries by name, a
 *  global panel as `home:<stem>` (object-pane GlobalPanels). */
export function homeStripId(entry: string): string {
  return ['actions', 'runs', 'errors', 'chat'].includes(entry) || entry.startsWith('home:') ? entry : `home:${entry}`
}

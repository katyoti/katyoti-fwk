// SPDX-License-Identifier: Apache-2.0
/**
 * The user menu's entries that do not work yet: kept in the
 * menu, greyed and unclickable with a « Bientôt disponible » tooltip, never
 * opening an empty screen. An entry works again by being taken off this list
 * - data, read once where the menu is built - not by a change to the menu.
 */

/** The menu entries not available yet, by id. */
export const UNAVAILABLE_PROFILE_ENTRIES: ReadonlySet<string> = new Set(['usage', 'notifications', 'theme'])

/** profileEntryAvailable says whether a user-menu entry works. */
export function profileEntryAvailable(id: string, unavailable: ReadonlySet<string> = UNAVAILABLE_PROFILE_ENTRIES): boolean {
  return !unavailable.has(id)
}

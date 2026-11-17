// SPDX-License-Identifier: Apache-2.0
/**
 * The left panel's « … » menu: what it offers for the universe on display.
 * Every key has its action in ProjectPanel (a Record over PanelMenuKey, so a
 * key without one does not compile); an item is disabled, never shown dead.
 */
export type PanelMenuKey = 'checkInstall' | 'restart' | 'openFolder' | 'remove'

export interface PanelMenuItem {
  key: PanelMenuKey
  disabled: boolean
}

/**
 * attached: the project folders the universe attaches (none when it is its own
 * project); restart: « Redémarrer l'univers » is offered (a running
 * universe: universeRuntime.offersRestart).
 */
export function panelMenuItems(attached: string[], restart = false): PanelMenuItem[] {
  return [
    // « Vérifier l'installation »: always there, it opens the
    // full « Prêt à démarrer » screen, every tab.
    { key: 'checkInstall', disabled: false },
    ...(restart ? [{ key: 'restart' as const, disabled: false }] : []),
    // The universe's own folder opens when it attaches no project.
    { key: 'openFolder', disabled: false },
    // Only an attached project can be removed from the universe.
    { key: 'remove', disabled: attached.length === 0 },
  ]
}

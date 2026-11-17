// SPDX-License-Identifier: Apache-2.0
/**
 * What the GUI offers on a running universe's runtime: « Redémarrer
 * l'univers » - its hub and
 * every member ended, even a hub the GUI found running, then started again -
 * and the notice that its hub keeps running once the GUI closes, when the GUI
 * found it running rather than starting it (`katyoti universe start`, a
 * command's `--hub keep`, another program).
 */

/** The runtime facts a universe carries in the GUI. */
export interface UniverseRuntimeFacts {
  /** This GUI runs a core for the universe. */
  busRunning: boolean
  /** The GUI found the universe's hub running: it outlives the GUI. */
  hubKeepsRunning?: boolean
}

/** Whether « Redémarrer l'univers » is offered: on a running universe. */
export function offersRestart(u: UniverseRuntimeFacts | null | undefined): boolean {
  return u?.busRunning === true
}

/** Whether the GUI says the universe's hub keeps running after it closes. */
export function saysHubKeepsRunning(u: UniverseRuntimeFacts | null | undefined): boolean {
  return u?.busRunning === true && u.hubKeepsRunning === true
}

// SPDX-License-Identifier: Apache-2.0
import { useAppStore } from '../../store'
import { dispatcherFor, orchestratorClient } from '../../ktsp'
import { serveChange, type ServeOutcome, type ServeWant } from './serveWait'

/**
 * awaitServed applies a change the hub recorded - an attach, a detach, a
 * creation, a switch: the hub restarts the member serving the
 * universe's objects, which binds its projects when it starts, and the GUI
 * waits until it serves expected - the attached, created or switched folder,
 * or after a detach { gone: folder }: that folder served no more, the
 * universe's other members served still; the universe-scoped
 * views then re-read it. Recorded but not served (the restart
 * failed, or did not end in time) rejects with a NotServedError saying why.
 */
export async function awaitServed(universe: string, out: ServeOutcome, expected: ServeWant): Promise<void> {
  await serveChange(out, expected, () => servedProjects(universe))
  useAppStore.setState((s) => ({ universeRevision: s.universeRevision + 1 }))
}

/**
 * « Ouvrir un dossier »: the universe serves another project.
 * A universe serves one project, so the hub switches it (orchestrator
 * switch-project: the served one detached, this one attached - refused, the
 * first kept) and restarts the member serving it; awaitServed
 * waits until that member serves the project. Switched but not served rejects
 * with a NotServedError saying why.
 * Already served: nothing to do.
 */
export async function switchServedProject(universe: string, projectDir: string): Promise<void> {
  const out = await orchestratorClient().switchProject({ universe, projectDir })
  if (!out.changed) return
  await awaitServed(universe, out, projectDir)
}

/** servedProjects is every folder the universe's serving member serves now
 *  (CWM ProjectsKnown: each served member marked). */
async function servedProjects(universe: string): Promise<string[]> {
  const out = await dispatcherFor(universe)('ktw-agents:cwm/projects-known', {}) as { projects?: { dir: string; served: boolean }[] }
  return (out?.projects ?? []).filter((p) => p.served && p.dir).map((p) => p.dir)
}

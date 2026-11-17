// SPDX-License-Identifier: Apache-2.0
import type { AttachProjectInput, AttachProjectOutput } from '../../ktsp/orchestrator'

/**
 * Linking a project that is already its own repository to a universe:
 * the person picks its folder, the hub attaches it -
 * one call; the universe then serves it beside its other projects. With
 * creating one as its own repository, the two ways « + New project » makes a
 * project the universe's: no project is ever made inside another.
 */

/** The OS folder picker: a Wails binding, a native dialog KTSP cannot express. */
export type FolderPicker = () => Promise<{ path?: string; error?: string }>

/** folderPicker is the GUI's folder picker; undefined where there is none
 *  (a browser build, a test). */
export function folderPicker(): FolderPicker | undefined {
  const w = globalThis as unknown as {
    go?: { main?: { App?: { PickWorkspaceFolder?: FolderPicker } } }
  }
  return w.go?.main?.App?.PickWorkspaceFolder
}

/** The orchestrator call a link makes. */
export interface ProjectLinker {
  attachProject: (input: AttachProjectInput) => Promise<AttachProjectOutput>
}

/** linkExistingProject asks the folder, then attaches it to the universe;
 *  null when the person cancels the picker - nothing attached. A picker that
 *  fails rejects with its error. */
export async function linkExistingProject(client: ProjectLinker, universe: string, pick: FolderPicker): Promise<AttachProjectOutput | null> {
  const picked = await pick()
  if (picked?.error) throw new Error(picked.error)
  if (!picked?.path) return null
  return client.attachProject({ universe, projectDir: picked.path })
}

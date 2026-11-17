// SPDX-License-Identifier: Apache-2.0
/**
 * openRunResult — a run's result names the object it made (a tool's
 * `{ok, open}`, a workflow's answer, an agent's final result: run.open):
 * the GUI opens it, on the panel named after `#`, and the task's entry form
 * gives way to it, as a page form's submit navigates. Whatever
 * place opened the form - « + Nouveau », a panel's chat button, a home quick
 * action - the form closes and the Object tab shows what was made.
 */
import { useAppStore } from '../store'
import { openObjectTab } from '../sections/workspace/openObjectTab'

export function openRunResult(universeName: string, target: { id: string; panel?: string; title?: string }): void {
  const app = useAppStore.getState()
  app.bumpWorkspaceTree()
  // The entry form takes the whole pane while it is open: it closes, so the
  // object it created is what the person sees.
  if (app.pendingTask) app.closeRunTask()
  openObjectTab(universeName, target)
}

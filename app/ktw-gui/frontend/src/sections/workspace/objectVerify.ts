// SPDX-License-Identifier: Apache-2.0
/**
 * The object pane's Verify: Verify - a long method - runs
 * through the action surface (WorkspaceObjectVerifyRun): it answers at once
 * with its run and the display panel it reports each check into, drawn under
 * the check, its outcome said there once it completes.
 */

/** The pane's check result (CapCheckResult, app/ktw-gui). */
export interface CheckResult {
  status: string
  desc?: string
  error?: string
}

/** WorkspaceObjectVerifyRun's answer (app_workspace_object_detail.go). */
export interface VerifyRunResult {
  taskId?: string
  panel?: string
  check?: CheckResult
}

/** A check result as the pane's badge says it. */
export function checkText(r: CheckResult): string {
  return `${r.status}${r.desc ? ` — ${r.desc}` : ''}${r.error ? ` (${r.error})` : ''}`
}

/** What a verification shows: the run's panel, or its outcome as text. */
export type VerifyOutcome = { panel: string } | { text: string }

export function verifyRunOutcome(r: VerifyRunResult): VerifyOutcome {
  if (r.panel) return { panel: r.panel }
  if (r.check) return { text: checkText(r.check) }
  return { text: checkText({ status: 'harness-error', error: 'the verification answered neither a run nor an outcome' }) }
}

type VerifyBindings = {
  go?: { main?: { App?: {
    WorkspaceObjectVerifyRun?: (universeName: string, objectId: string) => Promise<VerifyRunResult>
  } } }
}
const verifyApp = () => (window as unknown as VerifyBindings).go?.main?.App

/** Verifies an object: its run's panel, or the outcome as text. */
export async function verifyObject(universeName: string, objectId: string): Promise<VerifyOutcome> {
  const run = verifyApp()?.WorkspaceObjectVerifyRun
  if (!run) return { text: checkText({ status: 'harness-error', error: 'the verify binding is missing: rebuild the GUI' }) }
  return verifyRunOutcome(await run(universeName, objectId))
}

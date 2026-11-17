// SPDX-License-Identifier: Apache-2.0
// The glue of the generic capability MethodPanel: its actions' shapes and how
// one runs - through the action surface, a long method then answering the
// display panel its run reports into, any other its value. Kept apart from MethodPanel so it is unit-testable
// with no Wails binding.

/** A reference to an addressable method: its URI + templated args. */
export interface MethodRef {
  label?: string
  method: string
  args?: Record<string, unknown>
}

/** The result of a WorkspaceCallMethod call: a raw JSON string or an error. */
export interface CallResult {
  result?: string
  error?: string
}

/** One capability-panel action button. */
export interface ActionSpec extends MethodRef {
  variant?: string
  refreshStateOnSuccess?: boolean

}

/** A method-panel action run through the action surface (WorkspaceRunMethod):
 *  a method declared long answers its run and the display panel
 *  it reports into; any other its value. */
export interface RunResult {
  result?: string
  taskId?: string
  panel?: string
  error?: string
}

/** The panel's action runner: runs a MethodRef through the action surface. */
export type RunFn = (ref: MethodRef) => Promise<RunResult>

/** What a run through the action surface shows: the display panel its run
 *  reports into, drawn in place - whatever method, sidecar or panel type -,
 *  or, for a method that is not long, its value or error as text. */
export type RunOutcome = { panel: string } | { result: CallResult }

export function runOutcome(r: RunResult): RunOutcome {
  if (r.error) return { result: { error: r.error } }
  if (r.panel) return { panel: r.panel }
  return { result: { result: r.result } }
}

/** Whether the panel's state is read again once a long method's run panel
 *  ends: when it completed ok and the action asks it (refreshStateOnSuccess). */
export function refreshAfterRun(act: Pick<ActionSpec, 'refreshStateOnSuccess'>, status: string): boolean {
  return status === 'ok' && Boolean(act.refreshStateOnSuccess)
}

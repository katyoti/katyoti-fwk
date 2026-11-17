// SPDX-License-Identifier: Apache-2.0
/**
 * A panel's empty state: the pack that declares a panel gives the
 * sentence a person reads when it has nothing to show and the one action that
 * starts filling it (object-pane PanelEmpty); the GUI draws it in place of the
 * panel's page when its source answers nothing.
 */

/** A declared empty state: a sentence, and a button that runs an action or opens a task. */
export interface PanelEmpty {
  text: string
  label?: string
  action?: string
  input?: Record<string, unknown>
}

/**
 * Whether a source's answer holds nothing to show: no answer, an empty list,
 * or data whose lists are all empty (an answer naming its `kind` is judged by
 * its `data`). Data without any list is something to show.
 */
export function isEmptyAnswer(answer: unknown): boolean {
  if (answer && typeof answer === 'object' && !Array.isArray(answer) && typeof (answer as { kind?: unknown }).kind === 'string') {
    return isEmptyAnswer((answer as { data?: unknown }).data)
  }
  if (answer === null || answer === undefined) return true
  const lists: unknown[][] = []
  const walk = (v: unknown) => {
    if (Array.isArray(v)) lists.push(v)
    else if (v && typeof v === 'object') Object.values(v).forEach(walk)
  }
  walk(answer)
  if (lists.length === 0) return typeof answer === 'object' && Object.keys(answer as object).length === 0
  return lists.every((l) => l.length === 0)
}

/** Whether the action opens a task's run form rather than running a method. */
export function isTaskURI(uri: string): boolean {
  return /(^|[:/])tasks?\//.test(uri)
}

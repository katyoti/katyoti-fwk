// SPDX-License-Identifier: Apache-2.0
/**
 * taskInitialInput — what a task's run opens with. A task with
 * a form (its input contract) opens its form; a task tagged
 * `no-initial-input` asks its questions itself, in its chat (a wizard whose
 * every question is a form card), and starts at once - no free-text
 * « Entrée de la tâche » box; any other task opens that box, its initial
 * message optional. The same from every entry point: they all open the run
 * through RunTaskPanel.
 */

/** The task tag that says the task takes no initial message. */
export const NO_INITIAL_INPUT_TAG = 'no-initial-input'

export interface TaskInputShape {
  inputSchemaURI?: string
  tags?: string[] | null
}

/** takesNoInitialInput: the task declares that it takes no initial message. */
export function takesNoInitialInput(task: TaskInputShape | null | undefined): boolean {
  return !!task?.tags?.includes(NO_INITIAL_INPUT_TAG)
}

/** runOpening: what the run surface shows first - the task's form, nothing
 *  (the run starts at once), or the free-text initial-message box. */
export function runOpening(task: TaskInputShape | null | undefined): 'form' | 'start' | 'input-box' {
  if (task?.inputSchemaURI) return 'form'
  if (takesNoInitialInput(task)) return 'start'
  return 'input-box'
}

// SPDX-License-Identifier: Apache-2.0
/**
 * taskForm — a task with a form asks for it first, from wherever it is
 * started: « + Nouveau » (a type's create task), a page's chat button. The
 * run then starts from the form (RunTaskPanel) with its values, never with a
 * bare prompt that a no-model task cannot read (a workflow fed by its form).
 */

import { WorkspaceActionTask } from '../../wailsjs/go/main/App'
import { useAppStore } from '../store'
import { taskActionURI } from './taskActionURI'
import { formInitial } from './formInitial'

/** openTaskForm opens the task's form when it declares one (its input
 *  contract), the run starting on targetId once submitted; false when the
 *  task has no form, so the caller starts it as before. With fillParent
 *  (the create-task display) the form's `parent` field opens filled
 *  with targetId - a create task started under an object. With trail
 *  (« + Nouveau »), the form shows in a dialog over the object, its header
 *  on that line (TaskEntryDialog), never in the main area. */
export async function openTaskForm(universeName: string, ref: string, targetId?: string, title?: string, fillParent = false, trail?: string): Promise<boolean> {
  const uri = taskActionURI(ref)
  if (!uri || !universeName) return false
  const task = await WorkspaceActionTask(universeName, uri).catch(() => null)
  if (!task?.task || !task.inputSchemaURI) return false
  const name = uri.slice(uri.indexOf(':tasks/') + ':tasks/'.length)
  const form = {
    name: task.task,
    taskName: name,
    uri: task.task,
    title: title || task.title || name,
    description: task.description ?? '',
    inputSchemaURI: task.inputSchemaURI,
    targetId: targetId || undefined,
    initial: formInitial(targetId, fillParent),
  }
  if (trail) useAppStore.getState().openEntryForm({ ...form, trail })
  else useAppStore.getState().openRunTask(form)
  return true
}

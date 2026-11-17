// SPDX-License-Identifier: Apache-2.0
// What « Create » does for the project type chosen in the new-project dialog:
// a type declaring a create task runs it, a type
// whose create_task is a method answering a page opens that form in the
// dialog, another is created by the hub and typed as before.

import { createTaskName, createTaskPage } from './createChoices'

export { createTaskPage }
import { DEFAULT_PROJECT_TYPE } from './newTarget'
import { projectName } from '../univers/newProject'

export type NewProjectPick =
  | { kind: 'task'; task: string; input: string }
  | { kind: 'page'; source: string }
  | { kind: 'create'; retype: string | null }

export function newProjectPick(type: string, typeLabel: string, name: string, createTask: string | undefined): NewProjectPick {
  const page = createTaskPage(createTask)
  if (page) return { kind: 'page', source: page }
  if (createTask) {
    return { kind: 'task', task: createTaskName(createTask), input: `Create a new ${typeLabel} named "${name.trim()}" (its project folder: ${projectName(name)}).` }
  }
  return { kind: 'create', retype: type === DEFAULT_PROJECT_TYPE ? null : type }
}

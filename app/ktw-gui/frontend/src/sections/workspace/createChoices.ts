// SPDX-License-Identifier: Apache-2.0
/**
 * What « + New » offers: the object types allowed under
 * the parent and the aspects that name the task creating an object carrying
 * them (an Étude de l'existant), each kept when the universe's view allows
 * `new:<name>` at the person's level — a type and an aspect alike, by name.
 * A type the caller preset is always kept.
 */
export interface CreateChoice {
  name: string
  createTask?: string
}

export function offeredChoices<T extends CreateChoice>(
  choices: T[] | null,
  presetType: string | undefined,
  allows: (id: string) => boolean,
): T[] | null {
  return choices?.filter((c) => c.name === presetType || allows(`new:${c.name}`)) ?? null
}

/** The task a choice's create task names, as RunTaskInUniverse takes it. */
export function createTaskName(createTask: string): string {
  return createTask.split('/').pop() || createTask
}

/** formFirst says whether picking a choice in the create dialog tries its
 *  create task's form at once (the create-task display): a choice naming
 *  a task, never a page method, which the new-project dialog draws itself. */
export function formFirst(choice: CreateChoice, displayOn: boolean): boolean {
  if (!displayOn || !choice.createTask) return false
  const ref = choice.createTask
  const i = ref.indexOf(':')
  return i < 0 || ref.slice(i + 1).startsWith('tasks/') || ref.slice(i + 1).startsWith('/task/') || !ref.slice(i + 1).includes('/')
}

/** createTaskPage is the method a create_task names when it answers a page -
 *  a method URI `<provider>:<slot>/<method>` - and undefined for a task (a
 *  bare name or `<provider>:tasks/<name>`). */
export function createTaskPage(createTask: string | undefined): string | undefined {
  if (!createTask) return undefined
  const i = createTask.indexOf(':')
  if (i <= 0) return undefined
  const rest = createTask.slice(i + 1)
  if (rest.startsWith('tasks/') || !rest.includes('/')) return undefined
  return createTask
}

/** createPage is the page the create dialog draws for a choice whose
 *  create_task answers a page: its form under the parent - an
 *  information source's connector, a choice the holder cannot take greyed
 *  with its reason. */
export function createPage(choice: CreateChoice | undefined): string | undefined {
  return createTaskPage(choice?.createTask)
}

/** entryTrail is the header line of a create task's entry dialog: what
 *  « + New » created, and under which object (« + Nouveau › Étude de
 *  l'existant · sous « Dossier X » »). t is the workspace texts. */
export function entryTrail(t: (key: string, options?: Record<string, unknown>) => string, what: string, under?: string): string {
  return under ? t('create.entryTrailUnder', { what, under }) : t('create.entryTrail', { what })
}

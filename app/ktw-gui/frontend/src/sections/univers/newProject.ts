// SPDX-License-Identifier: Apache-2.0
import type { CreateProjectInput, CreateProjectOutput } from '../../ktsp/orchestrator'

/**
 * A new project from its name: the hub holds the rule
 * (orchestrator `create-project`) - the name is a universe name,
 * no project of that name is registered, its folder does not exist. The form
 * asks the hub's check as the name is typed, shows the folder it will make or
 * why it is refused, and creates only a name the check passed: nothing is
 * created before.
 */

/** The orchestrator calls the form makes. */
export interface ProjectCreator {
  createProject: (input: CreateProjectInput) => Promise<CreateProjectOutput>
}

/** What the form shows for the name typed. */
export interface NameCheck {
  /** The name the check answered for: a check of an older name is stale. */
  name: string
  /** The folder the project will be made in: <projects folder>/<name>. */
  folder: string
  /** Why it cannot be created, the hub's sentence; empty when it can. */
  refused: string
  /** What refuses it, said in the form's language: invalid-name, registered,
   *  folder-exists or inside-installation; empty when it can. */
  reason: string
}

/** The refusal reasons a form says in its language (the hub's codes). */
export const REFUSAL_REASONS = ['invalid-name', 'registered', 'folder-exists', 'inside-installation'] as const

/** The name as the hub takes it: trimmed. */
export function projectName(typed: string): string {
  return typed.trim()
}

/** The hub's check of a name for a universe: where it would go and why it is
 *  refused - nothing created. A universe attaching a member already takes the
 *  new project beside it. */
export async function checkName(client: ProjectCreator, typed: string, universe: string): Promise<NameCheck> {
  const name = projectName(typed)
  const out = await client.createProject({ name, universe, check: true })
  return { name, folder: out.projectDir ?? '', refused: out.refused ?? '', reason: out.reason ?? '' }
}

/** Create is offered only for the name typed, once its check passed. */
export function canCreate(typed: string, check: NameCheck | undefined, busy: boolean): boolean {
  const name = projectName(typed)
  return !busy && name !== '' && check !== undefined && check.name === name && check.refused === ''
}

/** The locale key of a refusal; undefined when the hub's sentence is all there is. */
export function refusalKey(check: NameCheck): string | undefined {
  return (REFUSAL_REASONS as readonly string[]).includes(check.reason) ? `projects.refused.${check.reason}` : undefined
}

/** Creates the project, attached to the universe: one hub call. */
export async function createNewProject(client: ProjectCreator, universe: string, typed: string): Promise<CreateProjectOutput> {
  return client.createProject({ name: projectName(typed), universe })
}

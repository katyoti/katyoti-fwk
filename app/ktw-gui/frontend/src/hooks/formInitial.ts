// SPDX-License-Identifier: Apache-2.0
// What a task's form opens with: a create task started under an
// object - « + New », a global action at the project's root - opens its form
// with its `parent` field filled from the place. Kept apart from the Wails
// bindings so it is tested alone.

/** formInitial is what a task's form opens with when it is started on
 *  targetId: its `parent`, when filling it is asked (the create-task display). */
export function formInitial(targetId: string | undefined, fillParent: boolean): Record<string, unknown> | undefined {
  return fillParent && targetId ? { parent: targetId } : undefined
}

/** formItem is the value a form opens with: its defaults, with the initial
 *  values of the fields it has - a value for a field it does not have is
 *  dropped, never sent with the run. Undefined when no initial value lands. */
export function formItem(
  defaults: Record<string, unknown> | undefined,
  initial: Record<string, unknown> | undefined,
  fields: string[],
): Record<string, unknown> | undefined {
  if (!initial) return undefined
  const kept = Object.entries(initial).filter(([k, v]) => fields.includes(k) && v !== undefined && v !== '')
  if (kept.length === 0) return undefined
  return { ...(defaults ?? {}), ...Object.fromEntries(kept) }
}

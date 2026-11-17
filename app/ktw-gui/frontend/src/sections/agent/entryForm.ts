// SPDX-License-Identifier: Apache-2.0
/**
 * entryForm — what a create task's entry form shows in its « + Nouveau »
 * dialog (TaskEntryDialog), read from the form's own `ui-form` declaration:
 *  - its title: the title of its one group, lifted into the dialog's header
 *    (the group then draws untitled, so the title shows once);
 *  - its buttons: the form's declared `submit_label` / `cancel_label` on the
 *    submit and cancel actions, never the task launcher's « Lancer ».
 * Generic: any type whose create task has a form.
 */

import type { Schema } from '@ktw/display-lib/schema'
import type { UIActionRef } from '@ktw/display-lib/ui-core/action/index.ts'

interface FormDecl {
  type: string
  name?: string
  submit_label?: string
  cancel_label?: string
  groups?: { title?: string }[]
}

/** formDecl is the `ui-form` extension the panel renders: the one named
 *  formName, else the unnamed or `default` one. */
function formDecl(schema: Schema, formName: string): FormDecl | undefined {
  const root = schema.types?.[schema.root]
  if (!root || typeof root !== 'object') return undefined
  const forms = ((root as { extensions?: FormDecl[] }).extensions ?? []).filter((e) => e?.type === 'ui-form')
  return forms.find((f) => (f.name || 'default') === formName) ?? forms.find((f) => !f.name || f.name === 'default')
}

/** entryTitle lifts the title of a form's single titled group: the title
 *  and the schema whose group no longer carries it. A form with no group
 *  title, or several groups, keeps its schema and has no title. */
export function entryTitle(schema: Schema, formName = 'default'): { title?: string; schema: Schema } {
  const decl = formDecl(schema, formName)
  const title = decl?.groups?.length === 1 ? decl.groups[0].title : undefined
  if (!decl || !title) return { schema }
  const root = schema.types[schema.root] as { extensions?: FormDecl[] }
  const extensions = (root.extensions ?? []).map((e) =>
    e === decl ? { ...e, groups: e.groups!.map(({ title: _drop, ...g }) => g) } : e)
  return { title, schema: { ...schema, types: { ...schema.types, [schema.root]: { ...root, extensions } } } as Schema }
}

/** declaredActions puts the form's declared labels on the submit and cancel
 *  actions; the others, and a form declaring none, keep theirs. */
export function declaredActions(schema: Schema | null, refs: UIActionRef[], formName = 'default'): UIActionRef[] {
  const decl = schema ? formDecl(schema, formName) : undefined
  if (!decl?.submit_label && !decl?.cancel_label) return refs
  return refs.map((r) => {
    if (typeof r === 'string') return r
    if (r.id === 'submit' && decl.submit_label) return { ...r, label: decl.submit_label }
    if (r.id === 'cancel' && decl.cancel_label) return { ...r, label: decl.cancel_label }
    return r
  })
}

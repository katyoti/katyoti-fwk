// SPDX-License-Identifier: Apache-2.0
// FieldLine — what a form draws around a field on its line, applied once
// by `createFieldRenderer` (./orchestrator.ts) for every field type:
//
//   - the field's label with its description beside it on one line
//     (`descriptionAt: beside`: « Code — facultatif · … »), the editor
//     drawn bare below;
//   - under the field, a host check's answer that is not a refusal
//     (form-line.ts checkAnswer): an accepted text green with a check icon,
//     a derived field adding « · rempli d'après le titre » or
//     « · modifié par vous », a note muted. A refusal stays the field's
//     error (react-hook-form), so saving and submitting read it as before.
//
// The answers come from the form's line behaviours (UIFormView's
// FormLineBehaviours) through FieldLineContext, by the field's name.

import { Text } from '@mantine/core'
import { IconCheck } from '@tabler/icons-react'
import { createContext, useContext, type ReactNode } from 'react'
import type { JsonObject } from '../../types/json-types.ts'
import type { UITexts } from '../../ui-core/i18n/types.ts'
import type { UIFormFieldRenderer } from './types.ts'

/** A check's answer shown under a field: accepted or a note; `derived`
 *  tells a derived field's value from the person's own. */
export type FieldLineAnswer = {
  tone: 'accepted' | 'note'
  text: string
  derived?: 'derived' | 'edited'
}

/** The line answers of a form's fields, by field name. */
export const FieldLineContext = createContext<Record<string, FieldLineAnswer>>({})

/** say reads a text through the form's texts, falling back to its default
 *  (its `{{name}}` read in `vars`) when no translation is loaded. */
export function say(texts: UITexts | undefined, key: string, def: string, vars: Record<string, string> = {}): string {
  let out: unknown
  try { out = texts?.t(key, { defaultValue: def, ...vars }) } catch { out = undefined }
  if (typeof out === 'string' && out !== '' && out !== key) return out
  return def.replace(/\{\{(\w+)\}\}/g, (_, k: string) => vars[k] ?? '')
}

/** The line under a field: its check's accepted text or note, nothing
 *  otherwise. `source` names what a derived field is filled from. */
export function FieldLineAnswerText({ name, source, texts }: { name: string; source?: string; texts?: UITexts }) {
  const answer = useContext(FieldLineContext)[name]
  if (!answer) return null
  const mark = answer.tone === 'accepted' && answer.derived
    ? answer.derived === 'derived'
      ? say(texts, 'fields.derivedFrom', 'filled from {{source}}', { source: source ?? '' })
      : say(texts, 'fields.editedByYou', 'edited by you')
    : undefined
  if (answer.tone === 'note') {
    return <Text size="xs" c="dimmed" mt={4} data-testid={'line.' + name} data-tone="note">{answer.text}</Text>
  }
  return (
    <Text size="xs" mt={4} data-testid={'line.' + name} data-tone="accepted">
      <Text span inherit c="teal" style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}>
        <IconCheck size={12} stroke={2.4} aria-hidden data-line-icon="check" />
        {answer.text}
      </Text>
      {mark ? <Text span inherit c="dimmed" data-line-mark={answer.derived}>{' · ' + mark}</Text> : null}
    </Text>
  )
}

/** The label line of a field whose description sits beside its label. */
export function FieldLabelBeside({ name, label, description, required }: { name: string; label: string; description?: string; required?: boolean }) {
  return (
    <Text size="sm" fw={500} mb={4} data-testid={'label.' + name}>
      {label}
      {required ? <span style={{ color: 'var(--mantine-color-error)' }} aria-hidden> *</span> : null}
      {description ? <Text span size="xs" c="dimmed" fw={400}>{' — ' + description}</Text> : null}
    </Text>
  )
}

/** wrapFieldLine frames a form field's renderer: the label line above when
 *  its description sits beside it (`beside`), the check's line below when
 *  the field is checked (`checked`). Called by `createFieldRenderer`; the
 *  inner renderer is called as before (its hooks in the same order). */
export function wrapFieldLine<TData extends JsonObject>(
  inner: UIFormFieldRenderer<TData>,
  opts: {
    name: string
    texts?: UITexts
    beside?: { label: string; description?: string; required?: boolean }
    checked?: boolean
    source?: string
  },
): UIFormFieldRenderer<TData> {
  if (!opts.beside && !opts.checked) return inner
  return (item, isNew, isSubmitting) => {
    const field: ReactNode = inner(item, isNew, isSubmitting)
    return (
      <div data-field-line={opts.name}>
        {opts.beside ? <FieldLabelBeside name={opts.name} {...opts.beside} /> : null}
        {field}
        {opts.checked ? <FieldLineAnswerText name={opts.name} source={opts.source} texts={opts.texts} /> : null}
      </div>
    )
  }
}

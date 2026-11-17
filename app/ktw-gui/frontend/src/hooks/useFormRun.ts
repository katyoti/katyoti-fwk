// SPDX-License-Identifier: Apache-2.0
import { useState } from 'react'
import { useAppStore } from '../store'
import { useT } from '../i18n'
import { RunTaskInUniverse, RunTaskInUniverseWithContext, ValidateAgainstSchema } from '../../wailsjs/go/main/App'

interface Violation {
  severity?: string
  code?: string
  message: string
  path?: string
}

export type FormTask = { name: string; inputSchemaURI?: string; targetId?: string }

/** useFormRun runs a task from its submitted form: the substrate validates
 *  the value first (its findings shown on the form, the form kept open),
 *  then the run starts on the task's target, if any, and its chat opens;
 *  onStarted then closes what showed the form (the entry dialog). Shared by
 *  the main area's form and the « + Nouveau » entry dialog. */
export function useFormRun(task: FormTask | null, universeName: string | undefined, onStarted?: () => void) {
  const t = useT('chat')
  const startSession = useAppStore((s) => s.startSession)
  const [submitting, setSubmitting] = useState(false)
  // Server-side validation findings from the pre-dispatch
  // SchemaService.ValidateAgainst gate. Non-empty ⇒ the last submit was
  // rejected; FormWidgetPanel surfaces them (alert + per-field).
  const [violations, setViolations] = useState<Violation[]>([])
  const submit = async (value: Record<string, unknown>) => {
    if (!task?.inputSchemaURI || !universeName) return
    setSubmitting(true)
    try {
      // Server-side gate: the substrate's validator is the
      // contract authority — client-side RHF validation alone
      // can drift from the registered schema. Reject ⇒ surface
      // violations (alert + per-field) and keep the form open.
      const check = await ValidateAgainstSchema(universeName, task.inputSchemaURI, JSON.stringify(value))
      if (check.error) {
        setViolations([{ message: t('runTask.validationUnavailable', { error: check.error }) }])
        setSubmitting(false)
        return
      }
      if (!check.valid) {
        setViolations(
          (check.violations ?? []).map((v) => ({
            severity: v.severity, code: v.code, message: v.message, path: v.path,
          }))
        )
        setSubmitting(false)
        return
      }
      setViolations([])
      // Phase 1 transport hack: the structured-input task-run path
      // (TaskRunRequest.Input map[string]any) is a Phase 2 follow-up.
      // For now we squeeze the form values through the existing
      // free-text prompt parameter, wrapped in explicit framing so
      // the agent can't miss them. The body of any input-schema
      // task should READ the FORM_INPUT block as the source of
      // truth and skip `ask-user`.
      const prompt =
        `=== FORM_INPUT (the user submitted these values via the schema-driven form — use them VERBATIM, do not ask for any of them) ===\n` +
        JSON.stringify(value, null, 2) +
        `\n=== END FORM_INPUT ===\n\nNow follow the task instructions using the values above.`
      // An object's action: the chat starts on that object.
      if (task.targetId) {
        await RunTaskInUniverseWithContext(universeName, task.name, prompt, { _target: task.targetId })
      } else {
        await RunTaskInUniverse(universeName, task.name, prompt)
      }
      startSession(task.name)
      setSubmitting(false)
      onStarted?.()
    } catch (err) {
      console.error('[RunTaskPanel/widget] RunTaskInUniverse failed:', err)
      setSubmitting(false)
    }
  }
  return { submitting, violations: violations.length > 0 ? violations : undefined, submit }
}

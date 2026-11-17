// SPDX-License-Identifier: Apache-2.0
// What a form does on a field's line while the person types:
// a field derived from another (« Code » pre-filled from the title, kept once
// the person edits it), a field checked on its line by a host method (« ce
// code est déjà pris »), and a form saved at each change (`save: change`).
// Kept free of React so it is tested on its own (tests/form-line.test.ts);
// UIFormView runs it (FormLineBehaviours).

import type { FieldLineAnswer, UIFieldDef, UIFieldRef } from '../fields/index.ts'
import type { UIFormDef } from './ui-form.schema.ts'
import type { FieldOptionList } from '../../schema/schema-contents.ts'
import { resolveAction, resolveActionArgs, type UIPageAction } from '../page/ui-page.interactive.ts'

/** How a derived field takes its value from its source: `slug` - lowercase,
 *  accents dropped, words joined by `-` (a code, a module id) -, `copy` the
 *  source as typed. */
export type UIFieldDeriveHow = 'slug' | 'copy'

/** A field pre-filled from another field while the person types. */
export interface UIFieldDerive {
  /** The schema field it follows. */
  from: string
  /** How (default `slug`). */
  as?: UIFieldDeriveHow
  /** How the line names the source in « rempli d'après … » (« le titre »);
   *  default the source field's label, lower-cased. */
  fromLabel?: string
}

/** A field checked on its line: the host method `uri` called with the
 *  field's `value` (its `args` laid under it, their `${field}` templates
 *  read in the form's live values - lineArgs) a moment after the person
 *  stops typing or a field the args name changes; an answer
 *  `{ok: false, message}` is the line's error, `{ok: true, text}` an
 *  accepted text drawn under the field (green, a check icon), `{text}` a
 *  plain note under it (checkAnswer). An empty value is asked only with
 *  `whenEmpty: true`. */
export interface UIFieldCheck {
  uri: string
  args?: Record<string, unknown>
  /** Ask the check about an empty value too (its answer then explains
   *  what an empty value does). */
  whenEmpty?: boolean
}

/** A note shown under a field's open editor: the host method `uri`
 *  called with the field's `value` (its `args` laid under it, their
 *  `${field}` templates read in the form's live values - lineArgs) when the
 *  editor opens and a moment after the person stops typing or a field the
 *  args name changes; the answer's `text` is the note. */
export interface UIFieldNote {
  uri: string
  args?: Record<string, unknown>
}

/** noteText reads a note's answer: its `text`, undefined when it has none. */
export function noteText(answer: unknown): string | undefined {
  if (!answer || typeof answer !== 'object') return undefined
  const t = (answer as { text?: unknown }).text
  return typeof t === 'string' && t.trim() !== '' ? t : undefined
}

/** lineArgs is what a note or a line check sends its host method: its
 *  `args` with their `${...}` templates read in the form's live `values`
 *  (the page DSL's grammar, resolveActionArgs: `${code}` a field,
 *  `${owner.name}` a dotted path, a whole-string template keeps the value's
 *  type, an embedded one is text, an unknown name is ''; `item.` is
 *  reserved by that grammar and reads nothing here), then the field's own
 *  `value` laid over them. */
export function lineArgs(args: Record<string, unknown> | undefined, values: Record<string, unknown> | undefined, value: unknown): Record<string, unknown> {
  return { ...resolveActionArgs(args, undefined, values ?? {}), value }
}

const ARG_REF = /\$\{([^}]+)\}/g

/** argFields lists the form fields a note's or a check's args read (the
 *  first segment of each `${...}` name, `item` left out), sorted: a change
 *  of one asks again. */
export function argFields(args: Record<string, unknown> | undefined): string[] {
  const names = new Set<string>()
  const walk = (v: unknown): void => {
    if (typeof v === 'string') {
      for (const m of v.matchAll(ARG_REF)) {
        const head = m[1].trim().split('.')[0]
        if (head !== '' && head !== 'item') names.add(head)
      }
    } else if (Array.isArray(v)) v.forEach(walk)
    else if (v && typeof v === 'object') Object.values(v).forEach(walk)
  }
  walk(args)
  return [...names].sort()
}

/** askNote calls a field's note method with the field's value over its
 *  args read in the form's values (lineArgs) and answers its text;
 *  undefined when there is no host, no note, or the call fails. */
export async function askNote(call: ((uri: string, input?: unknown) => Promise<unknown>) | undefined, note: UIFieldNote | undefined, value: unknown, values?: Record<string, unknown>): Promise<string | undefined> {
  if (!call || !note || typeof note.uri !== 'string' || note.uri === '') return undefined
  try {
    return noteText(await call(note.uri, lineArgs(note.args, values, value)))
  } catch {
    return undefined
  }
}

/** choicesList reads a choices method's answer: its `options`, each
 *  `{value, label, description?}` (a bare value is its own label), or
 *  undefined when it lists none. */
export function choicesList(answer: unknown): FieldOptionList | undefined {
  if (!answer || typeof answer !== 'object') return undefined
  const list = (answer as { options?: unknown }).options
  if (!Array.isArray(list)) return undefined
  const out: Array<{ value: string | number | boolean; label: string; description?: string }> = []
  for (const o of list) {
    if (typeof o === 'string' || typeof o === 'number' || typeof o === 'boolean') {
      out.push({ value: o, label: String(o) })
      continue
    }
    if (!o || typeof o !== 'object') continue
    const { value, label, description } = o as { value?: unknown; label?: unknown; description?: unknown }
    if (typeof value !== 'string' && typeof value !== 'number' && typeof value !== 'boolean') continue
    out.push({ value, label: typeof label === 'string' && label !== '' ? label : String(value), ...(typeof description === 'string' && description !== '' ? { description } : {}) })
  }
  return out.length > 0 ? out as FieldOptionList : undefined
}

/** askChoices calls a field's choices method with its args read in the
 *  form's values and answers its options, or the failure's message when
 *  the call fails or answers an `error`; nothing without a host. */
export async function askChoices(call: ((uri: string, input?: unknown) => Promise<unknown>) | undefined, choices: { uri: string; args?: Record<string, unknown> } | undefined, values?: Record<string, unknown>): Promise<{ options?: FieldOptionList; error?: string }> {
  if (!call || !choices || typeof choices.uri !== 'string' || choices.uri === '') return {}
  try {
    const answer = await call(choices.uri, resolveActionArgs(choices.args, undefined, values ?? {}))
    const error = (answer as { error?: unknown } | null)?.error
    if (typeof error === 'string' && error !== '') return { error }
    return { options: choicesList(answer) }
  } catch (e) {
    return { error: e instanceof Error ? e.message : String(e) }
  }
}

/** pressButton calls a row button's method with the field's value over its
 *  args read in the form's values (lineArgs) and reads its answer with the
 *  check grammar (checkAnswer); a call that fails is refused with its
 *  message. */
export async function pressButton(call: ((uri: string, input?: unknown) => Promise<unknown>) | undefined, button: { uri: string; args?: Record<string, unknown> } | undefined, value: unknown, values?: Record<string, unknown>): Promise<{ tone: CheckTone; text: string } | undefined> {
  if (!call || !button || typeof button.uri !== 'string' || button.uri === '') return undefined
  try {
    return checkAnswer(await call(button.uri, lineArgs(button.args, values, value)))
  } catch (e) {
    return { tone: 'refused', text: e instanceof Error ? e.message : String(e) }
  }
}

/** rowAction is the page action a read row's pencil runs: the field's
 *  action with its `${field}` templates read in the form's live values
 *  (`${item.x}` in the form's item). */
export function rowAction(action: UIPageAction, values: Record<string, unknown> | undefined, item?: unknown): UIPageAction {
  return resolveAction(action, item, values ?? {})
}

/** The slug of a text: lowercase, accents dropped, every run of other
 *  characters one `-`, none at either end, at most `max` characters. */
export function slugOf(text: unknown, max = 48): string {
  if (typeof text !== 'string') return ''
  const s = text.normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase()
    .replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '')
  return s.slice(0, max).replace(/-+$/g, '')
}

/** The value a derived field takes from its source. */
export function derivedValue(source: unknown, how: UIFieldDeriveHow = 'slug'): string {
  if (how === 'copy') return typeof source === 'string' ? source : ''
  return slugOf(source)
}

/** nextDerived says what a derived field becomes when its source changes:
 *  the new derived value while the field still holds what was derived last
 *  (or nothing), undefined once the person edited it - their value is kept. */
export function nextDerived(current: unknown, lastDerived: string | undefined, source: unknown, how?: UIFieldDeriveHow): string | undefined {
  const cur = typeof current === 'string' ? current : current == null ? '' : String(current)
  if (cur !== '' && cur !== (lastDerived ?? '')) return undefined
  const next = derivedValue(source, how)
  return next === cur ? undefined : next
}

/** derivedState says where a derived field's value comes from:
 *  `derived` while it holds what was derived last, `edited` once the
 *  person typed their own (or the form opened with one), undefined when
 *  it is empty. */
export function derivedState(current: unknown, lastDerived: string | undefined): 'derived' | 'edited' | undefined {
  const cur = typeof current === 'string' ? current : current == null ? '' : String(current)
  if (cur === '') return undefined
  return lastDerived !== undefined && cur === lastDerived ? 'derived' : 'edited'
}

/** How a check's answer reads on the field's line: `refused` (the line's
 *  error), `accepted` (green with a check icon), `note` (muted). */
export type CheckTone = 'refused' | 'accepted' | 'note'

/** checkAnswer reads a check's answer: `refused` with its message when it
 *  refuses the value (`ok: false`, or an `error`), `accepted` with its
 *  `text` when it passes it with one (`ok: true, text`), `note` for a
 *  `text` without `ok`; undefined when it says nothing to show. */
export function checkAnswer(answer: unknown): { tone: CheckTone; text: string } | undefined {
  if (!answer || typeof answer !== 'object') return undefined
  const a = answer as { ok?: unknown; message?: unknown; error?: unknown; text?: unknown }
  if (typeof a.error === 'string' && a.error !== '') return { tone: 'refused', text: a.error }
  if (a.ok === false) return { tone: 'refused', text: typeof a.message === 'string' && a.message !== '' ? a.message : 'refused' }
  const text = typeof a.text === 'string' && a.text.trim() !== '' ? a.text : undefined
  if (!text) return undefined
  return { tone: a.ok === true ? 'accepted' : 'note', text }
}

/** checkMessage reads a check's answer: its message when it refuses the
 *  value (`ok: false`, or an `error`), undefined when the value passes. */
export function checkMessage(answer: unknown): string | undefined {
  const a = checkAnswer(answer)
  return a?.tone === 'refused' ? a.text : undefined
}

/** asksCheck says whether a check is asked about a value: always when it
 *  holds one, an empty one only when the check says `whenEmpty`. */
export function asksCheck(check: UIFieldCheck | undefined, value: unknown): boolean {
  if (!check) return false
  const empty = value === undefined || value === null || value === ''
  return !empty || check.whenEmpty === true
}

/** lineAnswers is what each checked field shows under it: its check's
 *  last answer that is not a refusal (`answers`, by field), a derived
 *  field's answer told derived or edited (derivedState) from its live
 *  value and what was derived last. */
export function lineAnswers(
  fields: Array<{ name: string; derive?: UIFieldDerive }>,
  answers: Record<string, { tone: 'accepted' | 'note'; text: string }>,
  values: Record<string, unknown>,
  lastDerived: Record<string, string>,
): Record<string, FieldLineAnswer> {
  const out: Record<string, FieldLineAnswer> = {}
  for (const f of fields) {
    const a = answers[f.name]
    if (!a) continue
    const derived = f.derive ? derivedState(values[f.name], lastDerived[f.name]) : undefined
    out[f.name] = { tone: a.tone, text: a.text, ...(derived ? { derived } : {}) }
  }
  return out
}

/** checkPath reads where a check's refusal lands: its answer's `path`
 *  (`risks[2].mitigation`, a row's cell of an array field) as the form's
 *  dotted path (`risks.2.mitigation`), undefined when it names none - the
 *  message then stands on the checked field itself. */
export function checkPath(answer: unknown): string | undefined {
  if (!answer || typeof answer !== 'object') return undefined
  const p = (answer as { path?: unknown }).path
  if (typeof p !== 'string' || p.trim() === '') return undefined
  return p.trim().replace(/\[(\d+)\]/g, '.$1').replace(/^\./, '')
}

/** The fields of a form that act on their line: those derived from another
 *  and those checked by a host method, by the schema field they bind. */
export function lineFields(def: UIFormDef | undefined): Array<{ name: string; derive?: UIFieldDerive; check?: UIFieldCheck }> {
  const out: Array<{ name: string; derive?: UIFieldDerive; check?: UIFieldCheck }> = []
  for (const g of def?.groups ?? []) {
    for (const ref of g.fields ?? []) {
      if (typeof ref !== 'object' || !ref) continue
      const f = ref as UIFieldDef
      const derive = f.derive && typeof f.derive.from === 'string' && f.derive.from !== '' ? f.derive : undefined
      const check = f.check && typeof f.check.uri === 'string' && f.check.uri !== '' ? f.check : undefined
      if (derive || check) out.push({ name: f.name || f.id, ...(derive ? { derive } : {}), ...(check ? { check } : {}) })
    }
  }
  return out
}

/** Whether the form saves each change as it is made (`save: change`). */
export function savesOnChange(def: UIFormDef | undefined): boolean {
  return (def as { save?: unknown } | undefined)?.save === 'change'
}

/** The actions a form saved at each change does without: its changes are
 *  saved as made, so it draws no « Enregistrer », « Annuler » nor
 *  « Réinitialiser ». */
const SAVE_BAR_ACTIONS = new Set(['submit', 'save', 'cancel', 'reset'])

/** formActionsOf is the actions a form draws: every one, except the save
 *  bar's when the form saves each change (`save: change`). */
export function formActionsOf<A extends { id: string }>(def: UIFormDef | undefined, actions: A[]): A[] {
  if (!savesOnChange(def)) return actions
  return actions.filter((a) => !SAVE_BAR_ACTIONS.has(a.id))
}

/** shouldSave says whether a change is saved now: a change the person made
 *  (dirty), different from what was saved last, while no field shows an
 *  error on its line and no line is open for editing (a read row or a
 *  table row,: its OK saves). */
export function shouldSave(values: unknown, lastSaved: string | undefined, dirty: boolean, lineErrors: number, editing = false): boolean {
  if (!dirty || lineErrors > 0 || editing) return false
  return JSON.stringify(values) !== lastSaved
}

export type { UIFieldRef }

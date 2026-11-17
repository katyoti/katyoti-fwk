// SPDX-License-Identifier: Apache-2.0
// Read rows (ui-form `edit: row`): a group's fields drawn
// as rows - the label with its hint, the value read, a pencil per row. The
// pencil opens that field's own editor in the row with OK / Annuler: the
// editor works on the live form value, a snapshot taken on open; Annuler
// puts the snapshot back, OK closes the row. While a row is open the form's
// `save: change` waits, so OK is what saves. One row is open at a time.
// A field that edits in place itself (an array as a table) is drawn as its
// own editor; a `readOnly` field has no pencil.
// A field with an `action` keeps the pencil, which runs that page action
// (opens an object, a panel, a tab) instead of the editor; a field with a
// `button` draws that button in place of the pencil, its method's answer
// in the row; a field with `choices` lists the options its method answers;
// a field with `read` draws its closed value as it declares (a worded
// template, a tone pill and a muted note, a brand mark, a muted lead).

import { ActionIcon, Box, Button, Group, Stack, Text } from '@mantine/core'
import { IconCheck, IconPencil } from '@tabler/icons-react'
import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { useFormContext, useWatch } from 'react-hook-form'

import { useHostServices } from '../../../../ui-core/host/index.tsx'
import { argFields, askChoices, askNote, pressButton, rowAction, type CheckTone } from '../../form-line.ts'
import { useRunAction } from '../../../page/page-context.tsx'

import { readRowActions, rowKey, useRowEditing, type EditForm } from '../../../fields/editing.ts'
import type { UIFormRowRenderers } from '../../../fields/index.ts'
import type { UITexts } from '../../../../ui-core/i18n/index.ts'
import type { FieldOptionList } from '../../../../schema/schema-contents.ts'

const say = (texts: UITexts | undefined, key: string, fallback: string): string =>
  texts?.t(key, { defaultValue: fallback }) || fallback

export interface FormReadRowProps {
  row: UIFormRowRenderers
  item: any
  isNew: boolean
  isSubmitting: boolean
  texts?: UITexts
}

/** FormReadRow draws one field as a read row, its editor behind the pencil
 *  (or its action, or its button); a field with `choices` first asks its
 *  options. */
export function FormReadRow(props: FormReadRowProps) {
  const { row } = props
  if (row.choices && row.withOptions && !row.inPlace) return <ChoicesReadRow {...props} />
  return <ReadRow {...props} />
}

/** ChoicesReadRow asks the field's choices method when the row is drawn and
 *  again when a field its args name changes, then draws the row with the
 *  options it answers (the value read by its label); a call that fails
 *  keeps the declared options and says why under the open editor. */
function ChoicesReadRow(props: FormReadRowProps) {
  const { row } = props
  const host = useHostServices()
  const form = useFormContext()
  const choices = row.choices!
  const reads = useMemo(() => argFields(choices.args), [choices.args])
  const read = useWatch({ control: form.control, name: reads, disabled: reads.length === 0 })
  const [answer, setAnswer] = useState<{ options?: FieldOptionList; error?: string }>({})
  const key = JSON.stringify(reads.length === 0 ? null : read)
  useEffect(() => {
    let live = true
    void askChoices(host.callMethod, choices, form.getValues() as Record<string, unknown>).then((a) => { if (live) setAnswer(a) })
    return () => { live = false }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key, host.callMethod, choices.uri])
  const shown = useMemo(() => (answer.options ? row.withOptions!(answer.options) : row), [answer.options, row])
  return <ReadRow {...props} row={shown} choiceError={answer.error} />
}

function ReadRow({ row, item, isNew, isSubmitting, texts, choiceError }: FormReadRowProps & { choiceError?: string }) {
  const { editing, setEditing } = useRowEditing()
  const form = useFormContext()
  const host = useHostServices()
  const runOf = useRunAction()
  const snapshot = useRef<unknown>(undefined)
  const [pressed, setPressed] = useState<{ tone: CheckTone; text: string } | undefined>(undefined)
  const [pressing, setPressing] = useState(false)
  const key = rowKey(row.name)
  const id = `form.row.${row.name}`

  if (row.inPlace) {
    return <Box data-testid={id}>{row.form(item, isNew, isSubmitting)}</Box>
  }

  const open = editing === key
  const inError = !!form?.getFieldState(row.name, form.formState).error
  const act = readRowActions(form as unknown as EditForm, setEditing, row.name, snapshot)
  const editLabel = `${say(texts, 'buttons.edit', 'Modifier')} ${row.label}`
  const waits = editing !== null || isSubmitting

  let trailing: ReactNode
  if (open) {
    trailing = (
      <Group gap={4} wrap="nowrap">
        <Button size="compact-sm" data-testid={`${id}.ok`} disabled={inError} onClick={act.ok}>
          {say(texts, 'fields.ok', 'OK')}
        </Button>
        <Button size="compact-sm" variant="subtle" color="gray" data-testid={`${id}.cancel`} onClick={act.cancel}>
          {say(texts, 'buttons.cancel', 'Annuler')}
        </Button>
      </Group>
    )
  } else if (row.button) {
    const button = row.button
    const press = () => {
      setPressing(true)
      const values = form.getValues() as Record<string, unknown>
      void pressButton(host.callMethod, button, values?.[row.name], values)
        .then((a) => setPressed(a))
        .finally(() => setPressing(false))
    }
    trailing = (
      <Button size="compact-sm" variant="subtle" color="gray" data-testid={`${id}.button`} loading={pressing}
        disabled={!host.callMethod || waits} onClick={press}>
        {button.label}
      </Button>
    )
  } else if (row.readOnly) {
    trailing = <Box w={28} />
  } else if (row.action) {
    // The pencil runs the field's action; the page runs it, or it waits
    // disabled where no one can.
    const action = row.action
    const runs = runOf(action) !== undefined
    const run = () => runOf(rowAction(action, form.getValues() as Record<string, unknown>, item))?.(item)
    trailing = (
      <ActionIcon variant="subtle" color="gray" data-testid={`${id}.edit`} data-row-action=""
        aria-label={editLabel} disabled={!runs || waits} onClick={run}>
        <IconPencil size={15} />
      </ActionIcon>
    )
  } else {
    trailing = (
      <ActionIcon variant="subtle" color="gray" data-testid={`${id}.edit`}
        aria-label={editLabel} disabled={waits} onClick={act.edit}>
        <IconPencil size={15} />
      </ActionIcon>
    )
  }

  return (
    <Group data-testid={id} data-editing={open ? 'true' : 'false'} align="flex-start" wrap="nowrap" gap="md"
      py={8} style={{ borderBottom: '1px solid var(--mantine-color-default-border)' }}>
      <Stack gap={0} style={{ flex: '0 0 200px' }}>
        <Text size="sm" fw={500}>{row.label}</Text>
        {row.description ? <Text size="xs" c="dimmed">{row.description}</Text> : null}
      </Stack>
      <Box style={{ flex: '1 1 auto', minWidth: 0 }}>
        {open ? row.form(item, isNew, isSubmitting) : row.view(item ?? {})}
        {open && row.note ? <FieldNote id={`${id}.note`} name={row.name} note={row.note} /> : null}
        {open ? <NoteLine id={`${id}.choices`} text={choiceError} /> : null}
        {!open && row.button ? <RowAnswer id={`${id}.answer`} answer={pressed} /> : null}
      </Box>
      {trailing}
    </Group>
  )
}

/** RowAnswer draws a row button's answer under the row's value with the
 *  check grammar: refused red, accepted green with a check icon, a note
 *  muted; nothing without one. */
export function RowAnswer({ id, answer }: { id: string; answer: { tone: CheckTone; text: string } | undefined }) {
  if (!answer) return null
  if (answer.tone === 'note') return <Text size="xs" c="dimmed" mt={4} data-testid={id} data-tone="note">{answer.text}</Text>
  if (answer.tone === 'refused') return <Text size="xs" c="red" mt={4} data-testid={id} data-tone="refused">{answer.text}</Text>
  return (
    <Text size="xs" c="teal" mt={4} data-testid={id} data-tone="accepted" style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}>
      <IconCheck size={12} stroke={2.4} aria-hidden />
      {answer.text}
    </Text>
  )
}

/** NoteLine draws a field's note dimmed under its editor, nothing without
 *  a text. */
export function NoteLine({ id, text }: { id: string; text: string | undefined }) {
  if (!text) return null
  return <Text size="xs" c="dimmed" mt={4} data-testid={id}>{text}</Text>
}

/** FieldNote asks the field's note method when the editor opens, then a
 *  moment after the person stops typing in the field or in a field its
 *  args name (`${code}`, read in the form's live values), and draws its
 *  text. */
function FieldNote({ id, name, note }: { id: string; name: string; note: { uri: string; args?: Record<string, unknown> } }) {
  const host = useHostServices()
  const form = useFormContext()
  const value = useWatch({ control: form.control, name })
  const reads = useMemo(() => argFields(note.args), [note.args])
  const read = useWatch({ control: form.control, name: reads, disabled: reads.length === 0 })
  const [text, setText] = useState<string | undefined>(undefined)
  const first = useRef(true)
  const key = JSON.stringify([value ?? null, reads.length === 0 ? null : read])
  useEffect(() => {
    let live = true
    const timer = setTimeout(() => {
      void askNote(host.callMethod, note, value, form.getValues() as Record<string, unknown>).then((t) => { if (live) setText(t) })
    }, first.current ? 0 : 400)
    first.current = false
    return () => { live = false; clearTimeout(timer) }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key, host.callMethod, note.uri])
  return <NoteLine id={id} text={text} />
}

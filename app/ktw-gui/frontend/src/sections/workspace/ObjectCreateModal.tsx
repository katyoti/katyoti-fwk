// SPDX-License-Identifier: Apache-2.0
/**
 * ObjectCreateModal — "+ New" under an object: pick a type, name it, create.
 *
 * Step 1 lists the types allowed under the parent (`WorkspaceObjectChildTypes`
 * → `cwm/ObjectAllowedChildren`, the type's allowed children ∪ the types that
 * accept this parent) and the aspects naming a create task (a study), one
 * card each; a caller that already knows the type
 * (a children tab's "Add") skips it. Step 2 takes the name and description and
 * creates through `WorkspaceObjectCreate`, which runs the type's before_create
 * gate first — its refusal, like any substrate message, is shown as written.
 * The new object then opens: its fields tabs are where the rest is filled.
 * With the create-task display on, a type whose create task has a form
 * opens that form at once, its `parent` filled from the place - no
 * « created by the assistant » step in between.
 * A type whose create_task answers a page (an information
 * source's: its connector asked, the ones the holder cannot take greyed with
 * their reason, « déjà dans l'étude ») draws that page under the parent.
 * The dialog closes when the
 * page's action opens what it created (the tree re-read) or when its
 * « Annuler » (a `close` action) asks.
 */
import { useEffect, useState } from 'react'
import {
  Alert, Button, Card, Code, Group, Loader, Modal, SimpleGrid, Stack, Text, TextInput, Textarea,
} from '@mantine/core'
import { IconAlertCircle, IconArrowLeft } from '@tabler/icons-react'
import { RunTaskInUniverse, WorkspaceObjectChildTypes, WorkspaceObjectCreate } from '../../../wailsjs/go/main/App'
import type { main } from '../../../wailsjs/go/models'
import { useAppStore } from '../../store'
import { iconForType, colorForType } from './type-style'
import { openObjectTab } from './openObjectTab'
import { objectRef, projectOfRef } from './objectRef'
import { openTaskForm } from '../../hooks/taskForm'
import { tFor, useT } from '../../i18n'
import { errorText } from '../../i18n/errors'
import { useUniverseGui } from '../../hooks/useUniverseGui'
import { createPage, createTaskName, createTaskPage, entryTrail, formFirst as opensFormFirst, offeredChoices } from './createChoices'
import { PageHost } from '../../components/compositions/PageHost'
import type { UIPageNode } from '@ktw/display-lib/ui-builders/page/index.ts'

export interface ObjectCreateModalProps {
  opened: boolean
  onClose: () => void
  universeName: string
  parentId: string
  parentTitle?: string
  /** A type chosen by the caller: the picker is skipped. */
  presetType?: string
}

/** runCreateTask runs the create task a type or an aspect names (an assistant
 *  chat or an action that writes the object through CWM) under the parent -
 *  the create dialog's assistant and « + New »'s menu. */
export function runCreateTask(universeName: string, choice: main.WorkspaceNodeTypeChoice, parentId: string, parentTitle: string | undefined, toast: string, fillParent = false) {
  if (!choice.createTask) return
  const what = choice.label || choice.name
  const input = parentId
    ? `Create a new ${what} under the object "${parentTitle ?? parentId}" (parent_id: ${parentId}).`
    : `Create a new ${what} at the root.`
  const createTask = choice.createTask
  // A create task with a form asks for it first (taskForm), in a dialog
  // over the object; otherwise its assistant starts on the prompt.
  const trail = entryTrail(tFor('workspace'), what, parentId ? (parentTitle ?? parentId) : undefined)
  void openTaskForm(universeName, createTask, parentId || undefined, undefined, fillParent, trail).then((opened) => {
    if (opened) return
    useAppStore.getState().pushToast({ message: toast, color: 'blue' })
    void RunTaskInUniverse(universeName, createTaskName(createTask), input)
  })
}

export function ObjectCreateModal({ opened, onClose, universeName, parentId, parentTitle, presetType }: ObjectCreateModalProps) {
  const bumpWorkspaceTree = useAppStore((s) => s.bumpWorkspaceTree)
  const [choices, setChoices] = useState<main.WorkspaceNodeTypeChoice[] | null>(null)
  const [type, setType] = useState(presetType ?? '')
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [error, setError] = useState('')
  const [creating, setCreating] = useState(false)
  const t = useT('workspace')
  // « + New » offers the types and the creatable aspects the universe's view
  // allows at the person's level.
  const { hidden } = useUniverseGui(universeName)
  const offered = offeredChoices(choices, presetType, (id) => !hidden(id))
  // A create task's form first, its parent filled: for every user.
  const formFirst = true

  useEffect(() => {
    if (!opened) return
    setType(presetType ?? '')
    setName('')
    setDescription('')
    setError('')
    setChoices(null)
    WorkspaceObjectChildTypes(universeName, parentId)
      .then((c) => setChoices(c ?? []))
      .catch(() => setChoices([]))
  }, [opened, universeName, parentId, presetType])

  const chosen = choices?.find((c) => c.name === type)
  // A create_task answering a page: drawn under the parent; a
  // task's create_task is run as before.
  const page = createPage(chosen)
  const byTask = !!chosen?.createTask && !createTaskPage(chosen.createTask)

  const create = async () => {
    setCreating(true)
    setError('')
    try {
      const r = await WorkspaceObjectCreate(universeName, parentId, type, '', name.trim(), description.trim(), '')
      if (r.error) { setError(errorText(r)); return }
      bumpWorkspaceTree()
      onClose()
      // Created under another project's object (a qualified parent): it is
      // that project's, opened by its qualified id.
      openObjectTab(universeName, { id: objectRef(r.object.id, projectOfRef(parentId)), title: r.object.title || name.trim() })
    } finally {
      setCreating(false)
    }
  }

  // A type that declares a create task is created by that task (an assistant
  // chat that asks what it needs and writes the object through CWM), never by
  // the plain name form — a dossier needs its project code, its folder, … .
  const openAssistant = () => {
    if (!chosen?.createTask) return
    runCreateTask(universeName, chosen, parentId, parentTitle, t('create.openingAssistant', { what: chosen.label || chosen.name }), formFirst)
    onClose()
  }

  // A type is picked: with the create-task display on, one whose create task has a
  // form opens it at once, its parent filled; otherwise its step.
  const pick = (c: main.WorkspaceNodeTypeChoice) => {
    if (!opensFormFirst(c, formFirst) || !c.createTask) { setType(c.name); return }
    void openTaskForm(universeName, c.createTask, parentId || undefined, undefined, true, entryTrail(t, c.label || c.name, parentId ? (parentTitle ?? parentId) : undefined))
      .then((opened) => { if (opened) onClose(); else setType(c.name) })
      .catch(() => setType(c.name))
  }

  const title = parentTitle ? t('create.titleUnder', { parent: parentTitle }) : t('objectTab.newObject')

  return (
    <Modal opened={opened} onClose={onClose} title={<Text fw={600}>{title}</Text>} size={page ? 'lg' : type ? 'md' : 'xl'} centered>
      {!type ? (
        offered === null ? (
          <Loader size="sm" />
        ) : offered.length === 0 ? (
          <Text size="sm" c="dimmed">{t('create.noType')}</Text>
        ) : (
          <SimpleGrid cols={{ base: 1, sm: 2, md: 3 }} spacing="md">
            {offered.map((c) => {
              const TypeIcon = iconForType(c.name)
              const color = colorForType(c.name)
              return (
                <Card key={c.name} withBorder padding="md" radius="md" style={{ cursor: 'pointer' }} onClick={() => pick(c)} data-guide={`create.type.${c.name}`}>
                  <Stack gap={6}>
                    <Group gap="xs">
                      <TypeIcon size={20} color={`var(--mantine-color-${color}-6)`} />
                      <Text fw={600} size="sm" style={{ flex: 1, minWidth: 0 }} truncate>{c.label || c.name}</Text>
                    </Group>
                    <Code style={{ fontSize: 10 }}>{c.name}</Code>
                    {c.description && <Text size="xs" c="dimmed" lineClamp={3}>{c.description}</Text>}
                  </Stack>
                </Card>
              )
            })}
          </SimpleGrid>
        )
      ) : (
        <Stack gap="sm">
          <Group gap="xs">
            {(() => { const TypeIcon = iconForType(type); return <TypeIcon size={18} color={`var(--mantine-color-${colorForType(type)}-6)`} /> })()}
            <Text fw={600} size="sm" style={{ flex: 1 }}>{chosen?.label || type}</Text>
            {!presetType && (
              <Button size="xs" variant="subtle" leftSection={<IconArrowLeft size={14} />} onClick={() => setType('')}>
                {t('create.changeType')}
              </Button>
            )}
          </Group>
          {page ? (
            // The form is found by its section (data-section="object-create-form").
            <PageHost universeName={universeName} node={{ kind: 'box', id: 'object-create-form', children: [], load: { method: page, args: { parent: parentId } }, placeholder: { kind: 'skeleton', count: 4 } } as UIPageNode}
              onDone={(opened) => { if (opened) bumpWorkspaceTree(); onClose() }} />
          ) : byTask && chosen ? (
            <>
              <Text size="sm">
                {t('create.byAssistant', { what: chosen.label || chosen.name })}
              </Text>
              <Group justify="flex-end" gap="sm">
                <Button variant="default" onClick={onClose}>{t('common.cancel')}</Button>
                <Button onClick={openAssistant}>{t('create.openAssistant')}</Button>
              </Group>
            </>
          ) : (
            <>
              <TextInput label={t('objectTab.name')} required value={name} onChange={(e) => setName(e.currentTarget.value)} data-autofocus />
              <Textarea label={t('objectTab.description')} autosize minRows={2} value={description} onChange={(e) => setDescription(e.currentTarget.value)} />
              {error && <Alert icon={<IconAlertCircle size={16} />} color="red" variant="light">{error}</Alert>}
              <Group justify="flex-end" gap="sm">
                <Button variant="default" onClick={onClose} disabled={creating}>{t('common.cancel')}</Button>
                <Button onClick={() => void create()} loading={creating} disabled={!name.trim()}>{t('create.create')}</Button>
              </Group>
            </>
          )}
        </Stack>
      )}
    </Modal>
  )
}

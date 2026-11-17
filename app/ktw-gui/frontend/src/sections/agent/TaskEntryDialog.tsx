// SPDX-License-Identifier: Apache-2.0
/**
 * TaskEntryDialog — the entry form of a create task opened from
 * « + Nouveau », in a dialog over the object the person is on (the
 * store's entryForm): its header line (« + Nouveau › Étude de l'existant ·
 * sous « X » ») and its form's title, its fields, then « Annuler » and the
 * form's declared submit label (« Créer l'étude »). No task name nor URI.
 * One dialog, one form - never a wizard. Submitting runs the task as the
 * main area's form does (useFormRun); the dialog closes once it started.
 */
import { useMemo } from 'react'
import { Box, Modal } from '@mantine/core'
import type { UIActionRef } from '@ktw/display-lib/ui-core/action/index.ts'
import { useAppStore } from '../../store'
import { useT } from '../../i18n'
import { FormWidgetPanel } from './FormWidgetPanel'
import { useFormRun } from '../../hooks/useFormRun'

export function TaskEntryDialog() {
  const entry = useAppStore((s) => s.entryForm)
  const close = useAppStore((s) => s.closeEntryForm)
  const universeName = useAppStore((s) => s.displayedUniverseName) ?? undefined
  return (
    <Modal opened={entry !== null} onClose={close} withCloseButton={false} size="lg" centered>
      {entry?.inputSchemaURI && universeName && (
        // Keyed by the form shown: a new entry starts with no finding.
        <EntryForm key={`${entry.name}|${entry.targetId ?? ''}`} entry={entry} universeName={universeName} close={close} />
      )}
    </Modal>
  )
}

type Entry = NonNullable<ReturnType<typeof useAppStore.getState>['entryForm']>

function EntryForm({ entry, universeName, close }: { entry: Entry; universeName: string; close: () => void }) {
  const t = useT('chat')
  const formRun = useFormRun(entry, universeName, close)
  // The form's declared labels replace these (FormWidgetPanel's entry).
  const actionRefs = useMemo<UIActionRef[]>(() => [
    { type: 'ui-action', id: 'cancel', scope: 'item', label: t('form.cancel') },
    { type: 'ui-action', id: 'submit', scope: 'item', label: t('form.create'), icon: 'IconPlus' },
  ], [t])
  return (
    <Box data-testid="task.entry">
      <FormWidgetPanel
        universeName={universeName}
        schemaURI={entry.inputSchemaURI!}
        header={{ title: entry.title || entry.taskName || entry.name }}
        entry={{ trail: entry.trail }}
        actionMode="replace"
        actionRefs={actionRefs}
        submitting={formRun.submitting}
        serverViolations={formRun.violations}
        initial={entry.initial}
        onCancel={close}
        onSubmit={formRun.submit}
      />
    </Box>
  )
}

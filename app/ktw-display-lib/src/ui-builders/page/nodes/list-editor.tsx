// SPDX-License-Identifier: Apache-2.0
// The inline editor of a list card: its button opens a text
// box in the card - the element's remark kept, when its field holds one -,
// « Annuler » closes it and its confirm runs the editor's action on the
// element with `${value}` the text, then closes it. Clicks inside never open
// the card. The list decides on which elements it shows (`when`).

import { useState, type ReactNode } from 'react'
import { Button, Group, Stack, Textarea } from '@mantine/core'
import { IconPointFilled } from '@tabler/icons-react'

import { usePageContext } from '../page-context.tsx'
import { editorEvent, editorReady, editorStart, type UIListEditor } from '../ui-page.interactive.ts'
import { tablerIcon } from '../../../ui-core/components/icons/index.ts'

export function ListEditor({ editor, item }: { editor: UIListEditor; item: unknown }): ReactNode {
  const { onAction, hasHost } = usePageContext()
  const [open, setOpen] = useState(false)
  const [text, setText] = useState('')
  const can = !!onAction && hasHost
  const Icon = editor.icon ? tablerIcon(editor.icon, IconPointFilled) : undefined
  const stop = (e: { stopPropagation: () => void }) => e.stopPropagation()
  if (!open) {
    return (
      <Group onClick={stop}>
        <Button size="compact-xs" variant="subtle" disabled={!can} leftSection={Icon ? <Icon size={12} /> : undefined}
          onClick={() => { setText(editorStart(editor, item)); setOpen(true) }} data-guide="list.editor.open">
          {editor.label}
        </Button>
      </Group>
    )
  }
  return (
    <EditorForm editor={editor} text={text} onText={setText} onCancel={() => setOpen(false)}
      onConfirm={() => { onAction?.(editorEvent(editor, item, text)); setOpen(false) }} />
  )
}

/** EditorForm is the open editor: its text box, « Annuler » and its confirm,
 *  waiting for a text unless the editor says otherwise. */
export function EditorForm({ editor, text, onText, onCancel, onConfirm }: {
  editor: UIListEditor
  text: string
  onText: (t: string) => void
  onCancel: () => void
  onConfirm: () => void
}): ReactNode {
  return (
    <Stack gap={6} onClick={(e) => e.stopPropagation()} data-guide="list.editor">
      <Textarea autosize minRows={2} value={text} placeholder={editor.placeholder} onChange={(e) => onText(e.currentTarget.value)}
        data-guide="list.editor.text" />
      <Group justify="flex-end" gap="xs">
        <Button size="compact-sm" variant="default" onClick={onCancel} data-guide="list.editor.cancel">
          {editor.cancelLabel ?? 'Annuler'}
        </Button>
        <Button size="compact-sm" disabled={!editorReady(editor, text)} onClick={onConfirm} data-guide="list.editor.confirm">
          {editor.confirm.label ?? 'OK'}
        </Button>
      </Group>
    </Stack>
  )
}

// SPDX-License-Identifier: Apache-2.0
import { Card, Stack, Text } from '@mantine/core'
import type { MsgItem, AskFormMsg } from '../../../types/messages'
import { SendUIEvent } from '../../../../wailsjs/go/main/App'
import { useAppStore } from '../../../store'
import { InlineUIForm } from '../InlineUIForm'
import { InlineUIView } from '../InlineUIView'
import { useT } from '../../../i18n'
import { usePersonId } from '../../../store/profileStore'
import { sendPanelInput } from '../../../lib/displayPanels'
import { askFormAnswer, askFormRetry } from './askFormAnswer'

/**
 * AskFormContent — an ask-user-data form rendered as a bordered card in the
 * chat thread. While unanswered it shows the interactive form (Next /
 * Cancel); on Next it posts the answer to the agent and flips IN PLACE to a read-only view of the submitted value. The resolved
 * state lives on the MESSAGE (store), not local React state, so the
 * form↔view transition survives a tab switch / re-mount.
 *
 * The form is the question of a display panel (its artifact id the panel's
 * id): the card answers that panel's input. A form whose artifact is no
 * panel (an older run) sends the ui_event the terminal room turns into it.
 */
export function AskFormContent({ msg }: { msg: MsgItem }) {
  const m = msg as AskFormMsg
  const patchAskForm = useAppStore((s) => s.patchAskForm)
  const t = useT('chat')
  const universeName = useAppStore((s) => s.displayedUniverseName)
  const personId = usePersonId()
  const answer = (event: 'submit' | 'cancel', value?: unknown) => {
    const input = universeName ? askFormAnswer(m.artifactId, event, value, personId || undefined) : null
    if (input) {
      // A refused input leaves the question open: the card shows the form again.
      sendPanelInput(universeName!, input).catch(() => patchAskForm(m.id, askFormRetry(event, value, m.value)))
      return
    }
    void SendUIEvent(m.artifactId ?? '', event, event === 'submit' ? JSON.stringify(value) : '')
  }

  if (m.submitted) {
    return (
      <Card withBorder padding="md" radius="md">
        <Stack gap={6}>
          {m.title && <Text size="sm" fw={600} c="gray.3">{m.title}</Text>}
          <InlineUIView schema={m.schema} item={m.submitted} />
        </Stack>
      </Card>
    )
  }

  if (m.cancelled) {
    return (
      <Card withBorder padding="sm" radius="md">
        <Text size="sm" c="dimmed">{m.title ? t('askForm.cancelledTitled', { title: m.title }) : t('askForm.cancelled')}</Text>
      </Card>
    )
  }

  return (
    <Card withBorder padding="md" radius="md">
      <InlineUIForm
        schema={m.schema}
        value={m.value}
        title={m.title}
        description={m.prompt}
        onSubmit={(v) => {
          patchAskForm(m.id, { submitted: v })
          answer('submit', v)
        }}
        onCancel={() => {
          patchAskForm(m.id, { cancelled: true })
          answer('cancel')
        }}
      />
    </Card>
  )
}

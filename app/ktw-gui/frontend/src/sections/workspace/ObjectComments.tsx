// SPDX-License-Identifier: Apache-2.0
/**
 * ObjectComments — what people, agents and modules said about the object, and
 * the answers, as threads. Comments are part of every object (CWM keeps them
 * beside its file; a comment never changes the object). The person commenting
 * is the profile's.
 */
import { useCallback, useEffect, useState } from 'react'
import { Alert, Box, Button, Card, Group, Stack, Text, Textarea } from '@mantine/core'
import { IconAlertTriangle, IconMessageReply, IconRobot, IconUser } from '@tabler/icons-react'
import { WorkspaceObjectCommentAdd, WorkspaceObjectCommentList } from '../../../wailsjs/go/main/App'
import { main } from '../../../wailsjs/go/models'
import { usePersonId } from '../../store/profileStore'
import { useAppStore } from '../../store'
import { useT } from '../../i18n'
import { errorText } from '../../i18n/errors'

export function ObjectComments({ universeName, objectId }: { universeName: string; objectId: string }) {
  const personId = usePersonId()
  const treeRev = useAppStore((s) => s.workspaceTreeRevision)
  const [comments, setComments] = useState<main.WorkspaceObjectComment[]>([])
  const [error, setError] = useState('')
  const [text, setText] = useState('')
  const [replyTo, setReplyTo] = useState('')
  const [saving, setSaving] = useState(false)
  const t = useT('workspace')

  const load = useCallback(async () => {
    const r = await WorkspaceObjectCommentList(universeName, objectId)
    setError(r.error ? errorText(r) : '')
    setComments(r.comments ?? [])
  }, [universeName, objectId])
  useEffect(() => { void load() }, [load, treeRev])

  const send = async () => {
    setSaving(true)
    try {
      const r = await WorkspaceObjectCommentAdd(universeName, objectId, text, replyTo, personId)
      if (r.error) { setError(errorText(r)); return }
      setError('')
      setComments(r.comments ?? [])
      setText('')
      setReplyTo('')
    } finally {
      setSaving(false)
    }
  }

  const threads = comments.filter((c) => !c.replyTo)
  const answers = (id: string) => comments.filter((c) => c.replyTo === id)

  const one = (c: main.WorkspaceObjectComment, nested: boolean) => (
    <Card key={c.id} withBorder padding="xs" radius="sm" ml={nested ? 'lg' : 0}>
      <Group gap={6} mb={4} wrap="nowrap">
        {c.byKind === 'agent' ? <IconRobot size={14} /> : <IconUser size={14} />}
        <Text size="xs" fw={600}>{c.by}</Text>
        <Text size="xs" c="dimmed">{new Date(c.at).toLocaleString()}</Text>
        {!nested && (
          <Button size="compact-xs" variant="subtle" ml="auto" leftSection={<IconMessageReply size={12} />}
            onClick={() => setReplyTo(c.id)}>
            {t('comments.answer')}
          </Button>
        )}
      </Group>
      <Text size="sm" style={{ whiteSpace: 'pre-wrap' }}>{c.text}</Text>
    </Card>
  )

  return (
    <Stack gap="xs">
      {error && <Alert color="red" variant="light" icon={<IconAlertTriangle size={14} />} p="xs">{error}</Alert>}
      {threads.length === 0 && <Text size="xs" c="dimmed">{t('comments.none')}</Text>}
      {threads.map((c) => (
        <Stack key={c.id} gap={4}>
          {one(c, false)}
          {answers(c.id).map((a) => one(a, true))}
        </Stack>
      ))}
      <Box>
        {replyTo && (
          <Group gap={6} mb={4}>
            <Text size="xs" c="dimmed">{t('comments.answering', { id: replyTo })}</Text>
            <Button size="compact-xs" variant="subtle" onClick={() => setReplyTo('')}>{t('comments.cancel')}</Button>
          </Group>
        )}
        <Textarea autosize minRows={2} placeholder={replyTo ? t('comments.answerPlaceholder') : t('comments.placeholder')}
          value={text} onChange={(e) => setText(e.currentTarget.value)} />
        <Group justify="flex-end" mt={6}>
          <Button size="xs" onClick={() => void send()} loading={saving} disabled={!text.trim()}>
            {replyTo ? t('comments.answer') : t('comments.comment')}
          </Button>
        </Group>
      </Box>
    </Stack>
  )
}

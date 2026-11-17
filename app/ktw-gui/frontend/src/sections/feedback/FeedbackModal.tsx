// SPDX-License-Identifier: Apache-2.0
/**
 * FeedbackModal — file a bug, a request or a piece of information, with the
 * screenshot taken when the form was asked for (store/feedbackStore.ts).
 *
 * The backend fills in who files it, the build and the OS, and writes the
 * entry into the configured feedback folder (FeedbackSubmit). On an error the
 * form keeps what was typed; a missing or unreachable folder points at the
 * Feedback settings.
 */

import { useEffect, useState } from 'react'
import { feedbackErrorText } from './feedbackError'
import {
  ActionIcon,
  Alert,
  Box,
  Button,
  Group,
  Image,
  Modal,
  SegmentedControl,
  Stack,
  Text,
  TextInput,
  Textarea,
  Tooltip,
} from '@mantine/core'
import { IconAlertTriangle, IconCheck, IconX } from '@tabler/icons-react'
import { FeedbackSubmit } from '../../../wailsjs/go/main/App'
import { useGuideT } from '../../guide/i18n'
import { useAppStore } from '../../store'
import { useFeedbackStore } from '../../store/feedbackStore'

const KINDS = ['bug', 'request', 'information', 'other'] as const
type Kind = (typeof KINDS)[number]

interface Failure {
  message: string
  /** True when the fix is in the Feedback settings. */
  settings: boolean
}

export function FeedbackModal() {
  const t = useGuideT()
  const { open, screenshot, universe, route, close, dropScreenshot } = useFeedbackStore()
  const openSettingsModal = useAppStore((s) => s.openSettingsModal)

  const [kind, setKind] = useState<Kind>('bug')
  const [title, setTitle] = useState('')
  const [description, setDescription] = useState('')
  const [sending, setSending] = useState(false)
  const [failure, setFailure] = useState<Failure | null>(null)
  const [sent, setSent] = useState('')

  // A fresh form each time it opens; a failed one keeps its text until then.
  useEffect(() => {
    if (!open) return
    setKind('bug')
    setTitle('')
    setDescription('')
    setFailure(null)
    setSent('')
  }, [open])

  const send = async () => {
    setSending(true)
    setFailure(null)
    try {
      const res = await FeedbackSubmit({ kind, title, description, universe, route, screenshotPng: screenshot })
      if (res.error) {
        const settings = res.errorCode === 'folder-unset' || res.errorCode === 'folder-unreachable'
        setFailure({ message: feedbackErrorText(t, res.errorCode, res.error), settings })
        return
      }
      setSent(res.name ?? '')
      window.setTimeout(close, 1500)
    } catch (err) {
      setFailure({ message: String(err), settings: false })
    } finally {
      setSending(false)
    }
  }

  return (
    <Modal opened={open} onClose={close} title={<Text fw={600} size="sm">{t('feedback.title')}</Text>} size={560}>
      <Stack gap="sm">
        <SegmentedControl
          fullWidth
          size="xs"
          value={kind}
          onChange={(v) => setKind(v as Kind)}
          data={KINDS.map((k) => ({ value: k, label: t(`feedback.kinds.${k}`) }))}
          aria-label={t('feedback.kind')}
        />
        <TextInput
          label={t('feedback.titleLabel')}
          placeholder={t('feedback.titlePlaceholder')}
          value={title}
          onChange={(e) => setTitle(e.currentTarget.value)}
          required
          data-autofocus
        />
        <Textarea
          label={t('feedback.description')}
          placeholder={t('feedback.descriptionPlaceholder')}
          value={description}
          onChange={(e) => setDescription(e.currentTarget.value)}
          autosize
          minRows={4}
          maxRows={10}
        />

        <Box>
          <Text size="sm" fw={500} mb={4}>{t('feedback.screenshot')}</Text>
          {screenshot ? (
            <Box pos="relative" style={{ border: '1px solid var(--mantine-color-default-border)', borderRadius: 6, overflow: 'hidden' }}>
              <Image src={`data:image/png;base64,${screenshot}`} alt={t('feedback.screenshot')} mah={200} fit="contain" />
              <Tooltip label={t('feedback.removeScreenshot')} withArrow>
                <ActionIcon
                  pos="absolute"
                  top={6}
                  right={6}
                  variant="filled"
                  color="dark"
                  size="sm"
                  aria-label={t('feedback.removeScreenshot')}
                  onClick={dropScreenshot}
                >
                  <IconX size={14} />
                </ActionIcon>
              </Tooltip>
            </Box>
          ) : (
            <Text size="xs" c="dimmed">{t('feedback.noScreenshot')}</Text>
          )}
        </Box>

        <Text size="xs" c="dimmed">
          {t('feedback.context', { universe: universe || '—', route: route || '—' })}
        </Text>

        {failure && (
          <Alert color="red" icon={<IconAlertTriangle size={16} />} p="xs">
            <Group justify="space-between" wrap="nowrap" gap="xs">
              <Text size="sm">{failure.message}</Text>
              {failure.settings && (
                <Button size="compact-xs" variant="light" color="red" onClick={() => openSettingsModal('feedback')}>
                  {t('feedback.openSettings')}
                </Button>
              )}
            </Group>
          </Alert>
        )}
        {sent && (
          <Alert color="teal" icon={<IconCheck size={16} />} p="xs">
            <Text size="sm">{t('feedback.sent', { name: sent })}</Text>
          </Alert>
        )}

        <Group justify="flex-end" gap="xs">
          <Button variant="default" size="xs" onClick={close}>{t('feedback.cancel')}</Button>
          <Button size="xs" onClick={() => void send()} loading={sending} disabled={!title.trim() || !!sent}>
            {t('feedback.send')}
          </Button>
        </Group>
      </Stack>
    </Modal>
  )
}

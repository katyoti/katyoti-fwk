// SPDX-License-Identifier: Apache-2.0
/**
 * FeedbackSettingsPanel — the "Feedback" section of the App Settings modal:
 * the folder feedback is written to (Feedback.yaml, global config), and the
 * import that files the folder's entries as planification requests in the
 * displayed universe (`ktw-agents:feedback/import`).
 */

import { useEffect, useState } from 'react'
import { Button, Divider, Group, Stack, Text, TextInput } from '@mantine/core'
import { IconFileImport, IconFolder } from '@tabler/icons-react'
import { FeedbackGetConfig, FeedbackPickFolder, FeedbackSaveConfig, WorkspaceRunAction } from '../../../wailsjs/go/main/App'
import { useGuideT } from '../../guide/i18n'
import { feedbackErrorText } from './feedbackError'
import { useAppStore } from '../../store'

/** The import's action URI (internal/extensions/feedback/api.FeedbackImportServiceDef). */
const IMPORT_URI = 'ktw-agents:feedback/import'

/** An example network folder: a path, the same in every language. */
const FOLDER_EXAMPLE = '\\\\server\\share\\feedback'

interface ImportOutput {
  imported?: { entry: string; title: string; request?: string; warning?: string }[]
  skipped?: { entry: string; reason: string }[]
}

export function FeedbackSettingsPanel() {
  const t = useGuideT()
  const [folder, setFolder] = useState('')
  const [status, setStatus] = useState<{ ok: boolean; text: string } | null>(null)
  const universe = useAppStore((s) => s.displayedUniverseName)
  const [importing, setImporting] = useState(false)
  const [imported, setImported] = useState<{ ok: boolean; lines: string[] } | null>(null)

  useEffect(() => {
    FeedbackGetConfig()
      .then((res) => {
        if (res.error) setStatus({ ok: false, text: feedbackErrorText(t, res.errorCode, res.error) })
        else setFolder(res.folder)
      })
      .catch((err) => setStatus({ ok: false, text: String(err) }))
  }, [t])

  const browse = async () => {
    const res = await FeedbackPickFolder()
    if (res.error) setStatus({ ok: false, text: res.error })
    else if (res.path) setFolder(res.path)
  }

  const save = async () => {
    const err = await FeedbackSaveConfig(folder.trim())
    setStatus(err ? { ok: false, text: feedbackErrorText(t, err, err) } : { ok: true, text: t('feedback.saved') })
  }

  const runImport = async () => {
    if (!universe) return
    setImporting(true)
    setImported(null)
    try {
      const res = await WorkspaceRunAction(universe, IMPORT_URI, '')
      if (res.error) {
        setImported({ ok: false, lines: [res.error] })
        return
      }
      const out: ImportOutput = res.value ? JSON.parse(res.value) : {}
      const ok = out.imported ?? []
      const lines = [t('feedback.importDone', { imported: ok.length, skipped: out.skipped?.length ?? 0 })]
      for (const i of ok) lines.push(`${i.request} — ${i.title}${i.warning ? ` (${i.warning})` : ''}`)
      for (const s of out.skipped ?? []) lines.push(`${s.entry}: ${s.reason}`)
      setImported({ ok: true, lines })
    } catch (err) {
      setImported({ ok: false, lines: [String(err)] })
    } finally {
      setImporting(false)
    }
  }

  return (
    <Stack gap="sm">
      <TextInput
        label={t('feedback.folderLabel')}
        description={t('feedback.folderHint')}
        placeholder={FOLDER_EXAMPLE}
        value={folder}
        onChange={(e) => { setFolder(e.currentTarget.value); setStatus(null) }}
        rightSectionWidth={96}
        rightSection={
          <Button size="compact-xs" variant="subtle" leftSection={<IconFolder size={12} />} onClick={() => void browse()}>
            {t('feedback.browse')}
          </Button>
        }
      />
      <Group justify="space-between">
        <Text size="xs" c={status?.ok ? 'teal' : 'red'}>{status?.text ?? ''}</Text>
        <Button size="xs" onClick={() => void save()}>{t('feedback.save')}</Button>
      </Group>

      <Divider my="xs" />
      <Text size="sm" fw={500}>{t('feedback.importTitle')}</Text>
      <Text size="xs" c="dimmed">{t('feedback.importHint')}</Text>
      <Group>
        <Button
          size="xs"
          variant="light"
          leftSection={<IconFileImport size={14} />}
          loading={importing}
          disabled={!universe}
          onClick={() => void runImport()}
        >
          {universe ? t('feedback.importInto', { universe }) : t('feedback.importNoUniverse')}
        </Button>
      </Group>
      {imported && (
        <Stack gap={2}>
          {imported.lines.map((l, i) => (
            <Text key={i} size="xs" c={imported.ok ? undefined : 'red'}>{l}</Text>
          ))}
        </Stack>
      )}
    </Stack>
  )
}

// SPDX-License-Identifier: Apache-2.0
/**
 * ConfigFileEditor — edit one of an installed sidecar's config files as raw
 * YAML text, in the universe on screen.
 *
 * The floor the schema-driven form will sit on (sidecar-config-schema-and-form):
 * same get/set path, no schema. Reads through `get-item-config` (live file, or
 * the bundled default when the file does not exist in this universe yet) and
 * writes through `set-item-config` — the SIDECAR validates the YAML before
 * anything is written, so an invalid edit is refused with its parse error and
 * the file on disk is untouched.
 */
import { useCallback, useEffect, useState } from 'react'
import { Alert, Button, Group, Loader, Modal, Stack, Text, Textarea } from '@mantine/core'
import { IconAlertCircle, IconDeviceFloppy, IconFileImport } from '@tabler/icons-react'
import { useAppStore } from '../../store'
import { useT } from '../../i18n'
import type { marketplaceClient } from '../../ktsp'
import type { ConfigureItemOutput, SetItemConfigOutput } from '../../ktsp/marketplace'

/** One entry of the config check's `config_files` — the shape comes from the
 *  generated client, never re-declared. */
export type ConfigFileRef = ConfigureItemOutput['config_files'][number]

export interface ConfigEditTarget {
  /** Installed item name. */
  name: string
  file: ConfigFileRef
}

interface Props {
  /** What to edit; null = closed. */
  target: ConfigEditTarget | null
  /** Workspace folder of the universe on screen ('' = the hub's own workspace). */
  universeHome: string
  /** Display name for the universe the write lands in. */
  universeLabel: string
  mkt: ReturnType<typeof marketplaceClient>
  onClose: () => void
  /** The item's config check re-run after a successful write. */
  onSaved: (name: string, report: SetItemConfigOutput['report']) => void
}

const errText = (e: unknown) => (e instanceof Error ? e.message : String(e))

export function ConfigFileEditor({ target, universeHome, universeLabel, mkt, onClose, onSaved }: Props) {
  const t = useT('workspace')
  const pushToast = useAppStore((s) => s.pushToast)
  const [content, setContent] = useState('')
  const [path, setPath] = useState('')
  const [origin, setOrigin] = useState<'live' | 'default' | ''>('')
  const [notice, setNotice] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)

  const load = useCallback(
    async (t: ConfigEditTarget, useDefault: boolean) => {
      setLoading(true)
      setError('')
      try {
        const out = await mkt.getItemConfig({ name: t.name, file: t.file.name, default: useDefault, universeHome })
        setContent(out.content)
        setPath(out.path ?? '')
        setOrigin(useDefault ? 'default' : 'live')
        return true
      } catch (e) {
        setError(errText(e))
        return false
      } finally {
        setLoading(false)
      }
    },
    [mkt, universeHome],
  )

  // On open: the live file; when it does not exist in THIS universe yet, fall
  // back to the bundled default and say so. (The report's `exists` was computed
  // by the hub against its own workspace, so it cannot be trusted here.)
  useEffect(() => {
    if (!target) return
    setContent('')
    setPath('')
    setOrigin('')
    setNotice('')
    void (async () => {
      if (await load(target, false)) return
      if (!target.file.has_default) return
      if (await load(target, true)) {
        setNotice(t('configEditor.noLiveFile'))
      }
    })()
  }, [target, load])

  const loadDefault = async () => {
    if (!target) return
    if (await load(target, true)) setNotice(t('configEditor.defaultLoaded'))
  }

  const save = async () => {
    if (!target) return
    setSaving(true)
    setError('')
    try {
      const out = await mkt.setItemConfig({ name: target.name, file: target.file.name, content, universeHome })
      pushToast({ message: t('configEditor.written', { name: target.file.name }) + (out.path ? ' — ' + out.path : ''), color: 'green' })
      onSaved(target.name, out.report)
      onClose()
    } catch (e) {
      // The sidecar refused it (typically invalid YAML): keep the editor open with
      // the reason; nothing was written.
      setError(errText(e))
    } finally {
      setSaving(false)
    }
  }

  return (
    <Modal
      opened={!!target}
      onClose={onClose}
      title={target ? `${target.name} · ${target.file.name}` : ''}
      size="xl"
      closeOnClickOutside={false}
    >
      <Stack gap="xs">
        <Text size="xs" c="dimmed">
          {t('configEditor.universe', { universe: universeLabel })}
          {path ? ` · ${path}` : ''}
          {origin === 'default' ? ' · ' + t('configEditor.showingDefault') : ''}
        </Text>
        {notice && <Alert color="yellow" p="xs">{notice}</Alert>}
        {error && (
          <Alert color="red" p="xs" icon={<IconAlertCircle size={16} />} title={t('configEditor.notWritten')}>
            <Text size="xs" style={{ whiteSpace: 'pre-wrap' }}>{error}</Text>
          </Alert>
        )}
        {loading ? (
          <Group justify="center" py="lg"><Loader size="sm" /></Group>
        ) : (
          <Textarea
            value={content}
            onChange={(e) => setContent(e.currentTarget.value)}
            autosize
            minRows={12}
            maxRows={30}
            spellCheck={false}
            styles={{ input: { fontFamily: 'monospace', fontSize: 12 } }}
            aria-label={t('configEditor.contentLabel')}
          />
        )}
        <Group justify="space-between">
          <Button
            variant="subtle"
            size="xs"
            leftSection={<IconFileImport size={14} />}
            disabled={!target?.file.has_default || loading || saving}
            onClick={() => void loadDefault()}
          >
            {t('configEditor.loadDefault')}
          </Button>
          <Group gap="xs">
            <Button variant="default" size="xs" onClick={onClose} disabled={saving}>{t('common.cancel')}</Button>
            <Button
              size="xs"
              leftSection={<IconDeviceFloppy size={14} />}
              loading={saving}
              disabled={loading || !content.trim()}
              onClick={() => void save()}
            >
              {t('common.save')}
            </Button>
          </Group>
        </Group>
      </Stack>
    </Modal>
  )
}

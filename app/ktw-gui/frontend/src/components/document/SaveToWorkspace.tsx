// SPDX-License-Identifier: Apache-2.0
/**
 * SaveToWorkspace — keeps a document shown in a chat as an object of the
 * workspace (th25): a deck becomes a slide-deck, markdown a document, under
 * the object the user picks or in « Brouillons » (the drafts module). Then
 * the object opens, with its panels and its actions. A document that is
 * already an object never shows it.
 */
import { useEffect, useState } from 'react'
import { Alert, Button, Group, Modal, Select, Stack, Text, TextInput, Tooltip } from '@mantine/core'
import { IconFolderPlus } from '@tabler/icons-react'
import { WorkspaceDocumentSave, WorkspaceObjectFind } from '../../../wailsjs/go/main/App'
import { MEDIA_TYPE_DECK, MEDIA_TYPE_MARKDOWN } from '../../generated/media-types'
import { openObjectTab } from '../../sections/workspace/openObjectTab'
import { documentText } from './content'
import type { Document } from './engines'
import { useT } from '../../i18n'
import { visibleObjects } from '../../sections/workspace/systemModules'
import { useDisplayLevel } from '../../hooks/useUniverseGui'

const DRAFTS = '__drafts__'
const SAVABLE = new Set([MEDIA_TYPE_DECK, MEDIA_TYPE_MARKDOWN])

/** Whether a document can be kept as an object. */
export const savableToWorkspace = (doc: Document) => SAVABLE.has((doc.mediaType ?? '').trim())

const nameOf = (doc: Document) => (doc.title ?? '').replace(/\.deck\.md$|\.md$/i, '').trim()

export function SaveToWorkspaceButton({ doc, universe }: { doc: Document; universe: string }) {
  const t = useT('workspace')
  const tc = useT('components')
  const [open, setOpen] = useState(false)
  const [name, setName] = useState(nameOf(doc))
  const [where, setWhere] = useState<string>(DRAFTS)
  const [search, setSearch] = useState('')
  const [found, setFound] = useState<{ value: string; label: string }[]>([])
  const level = useDisplayLevel()
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  // The objects matching what the user types, a moment after they stop.
  useEffect(() => {
    if (!open || search.trim().length < 2) { setFound([]); return }
    let cancelled = false
    const t = setTimeout(() => {
      WorkspaceObjectFind(universe, search)
        .then((r) => {
          if (cancelled || r.error) return
          setFound(visibleObjects(r.objects ?? [], level).slice(0, 30).map((o) => ({ value: o.id, label: `${o.title || o.id} · ${o.type}${o.module ? ` · ${o.module}` : ''}` })))
        })
        .catch(() => {})
    }, 250)
    return () => { cancelled = true; clearTimeout(t) }
  }, [open, search, universe, level])

  const save = async () => {
    setSaving(true); setError('')
    try {
      const r = await WorkspaceDocumentSave(universe, where === DRAFTS ? '' : where, doc.mediaType ?? '', name, documentText(doc))
      if (r.error) { setError(r.error); return }
      setOpen(false)
      openObjectTab(universe, { id: r.object.id, title: r.object.title })
    } catch (e) {
      setError(String(e))
    } finally {
      setSaving(false)
    }
  }

  const options = [{ value: DRAFTS, label: tc('saveToWorkspace.drafts') }, ...found.filter((o) => o.value !== DRAFTS)]
  return (
    <>
      <Tooltip label={tc('saveToWorkspace.hint')} withArrow>
        <Button size="compact-xs" variant="light" leftSection={<IconFolderPlus size={12} />} onClick={() => { setName(nameOf(doc)); setError(''); setOpen(true) }}>
          {tc('saveToWorkspace.title')}
        </Button>
      </Tooltip>
      <Modal opened={open} onClose={() => setOpen(false)} title={tc('saveToWorkspace.title')} centered>
        <Stack gap="sm">
          <TextInput label={tc('saveToWorkspace.name')} value={name} onChange={(e) => setName(e.currentTarget.value)} data-autofocus />
          <Select
            label={tc('saveToWorkspace.where')}
            description={tc('saveToWorkspace.whereHint')}
            data={options}
            value={where}
            onChange={(v) => setWhere(v ?? DRAFTS)}
            searchable
            searchValue={search}
            onSearchChange={setSearch}
            nothingFoundMessage={search.trim().length < 2 ? tc('saveToWorkspace.typeTwoLetters') : tc('saveToWorkspace.noObjectFound')}
          />
          <Text size="xs" c="dimmed">
            {(doc.mediaType ?? '').trim() === MEDIA_TYPE_DECK
              ? tc('saveToWorkspace.deckNote')
              : tc('saveToWorkspace.markdownNote')}
          </Text>
          {error && <Alert color="red" variant="light">{error}</Alert>}
          <Group justify="flex-end">
            <Button variant="default" onClick={() => setOpen(false)}>{t('common.cancel')}</Button>
            <Button onClick={save} loading={saving} disabled={!name.trim()}>{t('common.save')}</Button>
          </Group>
        </Stack>
      </Modal>
    </>
  )
}

// SPDX-License-Identifier: Apache-2.0
/**
 * AttachedFilesView — the Files tab (ktw-agents' `object-panels/files.yaml`,
 * filter: an outgoing `attaches` edge). Its input is the object pane's
 * AttachedFiles answer: the files the object's attaches edges name and its
 * named files that exist (a feedback's screenshot, its name shown beside it),
 * paths in its module, by folder. The list is on the left; the file picked is shown on
 * the right through the one document entry point (DocumentView), read from
 * the object's module by its path — markdown with its relative images and
 * diagrams drawn from the file's folder, any other type by its engine, the
 * download card when none shows it — plus « Open the file » with the
 * system's application. A relative link inside a document opens its target
 * here. A file is never an object: the edge is the only link.
 */

import { useEffect, useMemo, useState } from 'react'
import { Badge, Box, Button, Center, Group, ScrollArea, Stack, Text, Tooltip, UnstyledButton } from '@mantine/core'
import {
  IconExternalLink, IconFile, IconFileText, IconFolder, IconPhoto, IconSitemap,
} from '@tabler/icons-react'
import { DocumentView } from '../../components/document/DocumentView'
import type { Document } from '../../components/document/engines'
import { WorkspaceObjectFileOpen } from '../../../wailsjs/go/main/App'
import { useT } from '../../i18n'

/** One file, as object-pane/AttachedFiles answers it. */
export interface AttachedFile {
  path: string
  project_path: string
  name: string
  folder: string
  size_bytes: number
  media_type: string
  kind: string
  /** The named file's name (a request's `screenshot`); absent for an edge's file. */
  named?: string
  missing?: boolean
  outside?: boolean
}

interface AttachedFiles {
  id: string
  module: string
  files: AttachedFile[]
  selected?: string
}

/** The files by folder, the module root first. */
export function byFolder(files: AttachedFile[]): Array<{ folder: string; files: AttachedFile[] }> {
  const groups = new Map<string, AttachedFile[]>()
  for (const f of files) {
    const g = groups.get(f.folder) ?? []
    g.push(f)
    groups.set(f.folder, g)
  }
  return [...groups.entries()]
    .sort(([a], [b]) => (a === '' ? -1 : b === '' ? 1 : a.localeCompare(b)))
    .map(([folder, fs]) => ({ folder, files: fs }))
}

/** The icon of a file, by its media type. */
function fileIconOf(mediaType: string): typeof IconFile {
  if (mediaType.startsWith('image/')) return IconPhoto
  if (mediaType.includes('mxfile') || mediaType.includes('mermaid')) return IconSitemap
  if (mediaType.startsWith('text/')) return IconFileText
  return IconFile
}

function sizeLabel(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`
}

export function AttachedFilesView({ universeName, objectId, input }: { universeName: string; objectId: string; input: unknown }) {
  const t = useT('workspace')
  const data = (input ?? {}) as AttachedFiles
  const files = useMemo(() => data.files ?? [], [data.files])
  const [selected, setSelected] = useState(data.selected ?? '')
  const [openError, setOpenError] = useState('')
  // A re-read with another file asked (a navigation) selects it.
  useEffect(() => { setSelected(data.selected ?? '') }, [data.selected])
  useEffect(() => { setOpenError('') }, [selected])

  if (files.length === 0) {
    return <Center p="xl"><Text size="sm" c="dimmed">{t('files.empty')}</Text></Center>
  }
  const current = files.find((f) => f.path === selected)
  const title = current?.name ?? selected.split('/').pop() ?? selected

  return (
    <Group gap={0} align="stretch" wrap="nowrap" style={{ height: '100%' }}>
      <Box style={{ width: 300, flexShrink: 0, borderRight: '1px solid var(--mantine-color-default-border)' }}>
        <ScrollArea style={{ height: '100%' }}>
          <Stack gap={10} p="sm">
            <Text size="xs" c="dimmed">{t('files.count', { count: files.length })}</Text>
            {byFolder(files).map((g) => (
              <Stack key={g.folder} gap={2}>
                <Group gap={6} wrap="nowrap">
                  <IconFolder size={13} color="var(--mantine-color-dimmed)" />
                  <Text size="xs" c="dimmed" ff="monospace" truncate>{g.folder || t('files.root')}</Text>
                </Group>
                {g.files.map((f) => {
                  const active = f.path === selected
                  const FileIcon = fileIconOf(f.media_type)
                  const row = (
                    <UnstyledButton disabled={f.missing || f.outside} data-file={f.path}
                      onClick={() => setSelected(f.path)}
                      px={8} py={4}
                      style={{ borderRadius: 6, background: active ? 'var(--mantine-color-blue-light)' : undefined, opacity: f.missing || f.outside ? 0.6 : 1 }}>
                      <Group gap={6} wrap="nowrap" justify="space-between">
                        <Group gap={6} wrap="nowrap" style={{ minWidth: 0 }}>
                          <FileIcon size={14} />
                          <Text size="sm" fw={active ? 600 : 400} truncate>{f.name}</Text>
                          {f.named && <Badge size="xs" variant="light" color="gray" style={{ flexShrink: 0 }}>{f.named}</Badge>}
                        </Group>
                        {f.missing
                          ? <Badge size="xs" color="red" variant="light">{t('files.missing')}</Badge>
                          : f.outside
                            ? <Badge size="xs" color="gray" variant="light">{t('files.outside')}</Badge>
                            : <Text size="xs" c="dimmed" style={{ flexShrink: 0 }}>{sizeLabel(f.size_bytes)}</Text>}
                      </Group>
                    </UnstyledButton>
                  )
                  const hint = f.missing ? t('files.missingHint') : f.outside ? t('files.outsideHint') : ''
                  return (
                    <Tooltip key={f.path} label={hint} disabled={!hint} withArrow multiline w={260}>{row}</Tooltip>
                  )
                })}
              </Stack>
            ))}
          </Stack>
        </ScrollArea>
      </Box>
      <Box style={{ flex: 1, minWidth: 0, height: '100%' }}>
        {selected ? (
          <DocumentView
            key={selected}
            doc={{ source: { path: selected }, title } as Document}
            universe={universeName}
            nodeId={objectId}
            header
            host={{ fillHeight: true }}
            onOpenFile={setSelected}
            headerExtra={(
              <Group gap={6} wrap="nowrap">
                {openError && <Text size="xs" c="red">{openError}</Text>}
                <Tooltip label={t('files.openWithSystemHint')} withArrow>
                  <Button size="compact-xs" variant="default" leftSection={<IconExternalLink size={12} />}
                    onClick={() => { void WorkspaceObjectFileOpen(universeName, objectId, selected).then((e) => setOpenError(e || '')) }}>
                    {t('files.openWithSystem')}
                  </Button>
                </Tooltip>
              </Group>
            )}
          />
        ) : (
          <Center p="xl"><Text size="sm" c="dimmed">{t('files.pick')}</Text></Center>
        )}
      </Box>
    </Group>
  )
}

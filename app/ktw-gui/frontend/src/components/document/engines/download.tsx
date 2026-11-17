// SPDX-License-Identifier: Apache-2.0
// Download card: the fallback for a media type no engine shows — never a
// blank pane. The host header's Save writes the bytes.
import { Center, Stack, Text } from '@mantine/core'
import { IconFileUnknown } from '@tabler/icons-react'
import { DOWNLOAD_ENGINE_ID, type DisplayEngine, type EngineProps } from '../engines'
import { documentSize } from '../content'
import { useT } from '../../../i18n'

function DownloadCard({ doc }: EngineProps) {
  const t = useT('components')
  return (
    <Center style={{ height: '100%' }} p="lg">
      <Stack align="center" gap={6}>
        <IconFileUnknown size={40} stroke={1.3} />
        <Text size="sm" fw={600}>{doc.title || t('download.document')}</Text>
        <Text size="xs" c="dimmed" ff="monospace">{doc.mediaType || t('download.unknownType')} · {t('documentView.sizeKb', { size: (documentSize(doc) / 1024).toFixed(1) })}</Text>
        <Text size="xs" c="dimmed">{t('download.noViewer')}</Text>
      </Stack>
    </Center>
  )
}

export const downloadEngine: DisplayEngine = {
  id: DOWNLOAD_ENGINE_ID,
  label: 'File',
  accepts: [],
  capabilities: { view: true, save: true },
  builtin: true,
  component: DownloadCard,
}

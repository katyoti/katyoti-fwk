// SPDX-License-Identifier: Apache-2.0
import { useEffect, useState } from 'react'
import { Alert, Badge, Button, Checkbox, Code, Group, Paper, Stack, Text, ThemeIcon, Title } from '@mantine/core'
import { IconFolderShare } from '@tabler/icons-react'
import { marketplaceClient } from '../../ktsp'
import { useAppLanguage, type TFn } from '../../i18n'
import { importable, importPicked, lastUseDay, offerRows, type EarlierProject, type ImportOutcome } from './earlierProjects'

/**
 * The earlier installations' projects, offered on « Prêt à démarrer »:
 * shown only when there are some; each with its installation,
 * folder and last use; nothing picked for the person; « Importer » copies the
 * picked ones into the projects folder - the originals are never written.
 */
export function EarlierProjectsSection({ t }: { t: TFn }) {
  const lang = useAppLanguage()
  const [rows, setRows] = useState<EarlierProject[] | null>(null)
  const [picked, setPicked] = useState<Set<string>>(() => new Set())
  const [busy, setBusy] = useState(false)
  const [outcomes, setOutcomes] = useState<ImportOutcome[]>([])
  const [error, setError] = useState('')

  const load = async () => {
    try {
      setRows(offerRows(await marketplaceClient().earlierProjects()))
      setError('')
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    }
  }
  useEffect(() => { void load() }, [])

  if (!error && (!rows || rows.length === 0)) return null

  const toggle = (folder: string, on: boolean) => setPicked((s) => {
    const n = new Set(s)
    if (on) n.add(folder)
    else n.delete(folder)
    return n
  })

  const run = async () => {
    if (!rows) return
    setBusy(true)
    try {
      setOutcomes(await importPicked(marketplaceClient(), rows, picked))
      setPicked(new Set())
      await load()
    } finally {
      setBusy(false)
    }
  }

  return (
    <Paper withBorder radius="md" p="md" data-element="section:ready/earlier-projects">
      <Group gap="xs" mb={4}>
        <ThemeIcon variant="light" size="sm" radius="sm"><IconFolderShare size={14} /></ThemeIcon>
        <Title order={5}>{t('earlier.title')}</Title>
      </Group>
      <Text size="xs" c="dimmed" mb="sm">{t('earlier.hint')}</Text>
      {error && <Alert color="red" variant="light" mb="xs">{t('earlier.error', { error })}</Alert>}
      <Stack gap={6}>
        {(rows ?? []).map((p) => (
          <Group key={p.folder} gap="sm" wrap="nowrap" align="flex-start">
            <Checkbox
              mt={2}
              checked={picked.has(p.folder)}
              disabled={!importable(p) || busy}
              onChange={(e) => toggle(p.folder, e.currentTarget.checked)}
              aria-label={p.name}
            />
            <Stack gap={2} style={{ minWidth: 0 }}>
              <Group gap="xs">
                <Text size="sm" fw={600}>{p.name}</Text>
                <Badge size="xs" variant="light" color="gray">{p.installation}</Badge>
                {lastUseDay(p, lang) && <Text size="xs" c="dimmed">{t('earlier.lastUse', { day: lastUseDay(p, lang) })}</Text>}
              </Group>
              <Code style={{ wordBreak: 'break-all' }}>{p.folder}</Code>
              {p.imported_to && <Text size="xs" c="teal">{t('earlier.imported', { folder: p.imported_to })}</Text>}
            </Stack>
          </Group>
        ))}
      </Stack>
      {outcomes.filter((o) => o.error).map((o) => (
        <Alert key={o.folder} color="red" variant="light" mt="xs">{t('earlier.failed', { folder: o.folder, error: o.error })}</Alert>
      ))}
      <Group justify="flex-end" mt="sm">
        <Button size="xs" onClick={() => void run()} disabled={picked.size === 0} loading={busy} data-element="action:ready/import-projects">
          {picked.size === 0 ? t('earlier.importNone') : t('earlier.import', { count: picked.size })}
        </Button>
      </Group>
    </Paper>
  )
}

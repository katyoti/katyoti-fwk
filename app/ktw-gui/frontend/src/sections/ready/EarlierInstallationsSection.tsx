// SPDX-License-Identifier: Apache-2.0
import { useEffect, useState } from 'react'
import { Alert, Badge, Button, Checkbox, Code, Group, Paper, Stack, Text, ThemeIcon, Title } from '@mantine/core'
import { IconTrash, IconHistory } from '@tabler/icons-react'
import { marketplaceClient } from '../../ktsp'
import type { DeleteInstallationOutput } from '../../ktsp/marketplace'
import { useAppLanguage, type TFn } from '../../i18n'
import { canAskDelete, confirmDelete, earlierRows, humanSize, keptByDefault, planDelete, refusalKey, type Installation } from './earlierInstallations'

/** A last use as a day in the GUI's language. */
function day(iso: string | undefined, lang: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? '' : d.toLocaleDateString(lang, { year: 'numeric', month: 'short', day: 'numeric' })
}

/**
 * The earlier installations of this machine, on « Prêt à démarrer »:
 * shown only when there are some; each with its version, folder,
 * size and last use; the most recently used one kept unless unticked; « Supprimer… »
 * shows what goes and deletes only once the person ticked that they understood.
 */
export function EarlierInstallationsSection({ t }: { t: TFn }) {
  const lang = useAppLanguage()
  const [rows, setRows] = useState<Installation[] | null>(null)
  const [kept, setKept] = useState<Set<string>>(() => new Set())
  const [plan, setPlan] = useState<{ of: Installation; out: DeleteInstallationOutput } | null>(null)
  const [ack, setAck] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const load = async (first: boolean) => {
    try {
      const r = earlierRows(await marketplaceClient().earlierInstallations())
      setRows(r)
      if (first) setKept(keptByDefault(r))
      setError('')
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    }
  }
  useEffect(() => { void load(true) }, [])

  if (!error && (!rows || rows.length === 0)) return null

  const keep = (name: string, on: boolean) => setKept((s) => {
    const n = new Set(s)
    if (on) n.add(name)
    else n.delete(name)
    return n
  })

  const ask = async (i: Installation) => {
    setBusy(true)
    setError('')
    try {
      setPlan({ of: i, out: await planDelete(marketplaceClient(), i) })
      setAck(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  const confirm = async () => {
    if (!plan) return
    setBusy(true)
    setError('')
    try {
      if (await confirmDelete(marketplaceClient(), plan.out, ack, plan.of.reason === 'last')) {
        setPlan(null)
        await load(false)
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <Paper withBorder radius="md" p="md" data-element="section:ready/earlier-installations">
      <Group gap="xs" mb={4}>
        <ThemeIcon variant="light" size="sm" radius="sm"><IconHistory size={14} /></ThemeIcon>
        <Title order={5}>{t('installations.title')}</Title>
      </Group>
      <Text size="xs" c="dimmed" mb="sm">{t('installations.hint')}</Text>
      {error && <Alert color="red" variant="light" mb="xs">{t('installations.error', { error })}</Alert>}
      <Stack gap="sm">
        {(rows ?? []).map((i) => (
          <Stack key={i.name} gap={4}>
            <Group gap="xs" wrap="nowrap" justify="space-between" align="flex-start">
              <Stack gap={2} style={{ minWidth: 0 }}>
                <Group gap="xs">
                  <Text size="sm" fw={600}>{i.title || i.name}</Text>
                  {i.version && <Badge size="xs" variant="light" color="gray">{i.version}</Badge>}
                  <Text size="xs" c="dimmed">{humanSize(i.bytes)}</Text>
                  {day(i.last_used, lang) && <Text size="xs" c="dimmed">{t('installations.lastUse', { day: day(i.last_used, lang) })}</Text>}
                </Group>
                <Code style={{ wordBreak: 'break-all' }}>{i.folder}</Code>
                {i.reason && i.reason !== 'last' && <Text size="xs" c="orange">{refusalKey(i) ? t(refusalKey(i)!) : i.refused}</Text>}
              </Stack>
              <Group gap="xs" wrap="nowrap">
                <Checkbox size="xs" label={t('installations.keep')} checked={kept.has(i.name)} onChange={(e) => keep(i.name, e.currentTarget.checked)}
                  disabled={busy} data-element="field:ready/installation-keep" />
                <Button size="compact-xs" color="red" variant="light" leftSection={<IconTrash size={12} />} disabled={busy || !canAskDelete(i, kept)}
                  onClick={() => void ask(i)} data-element="action:ready/installation-delete">
                  {t('installations.delete')}
                </Button>
              </Group>
            </Group>
            {plan?.of.name === i.name && (
              <Alert color="red" variant="light">
                <Text size="sm" fw={600} mb={4}>{t('installations.goes', { name: i.title || i.name })}</Text>
                <Stack gap={2} mb="xs">
                  {plan.out.folders.map((f) => (
                    <Text key={f.path} size="xs"><Code>{f.path}</Code> - {humanSize(f.bytes)}</Text>
                  ))}
                  <Text size="xs">{t('installations.shortcuts', { name: plan.out.shortcuts })}</Text>
                  <Text size="xs">{t('installations.stays')}</Text>
                </Stack>
                <Checkbox size="xs" label={t('installations.ack')} checked={ack} onChange={(e) => setAck(e.currentTarget.checked)} data-element="field:ready/installation-ack" />
                <Group justify="flex-end" gap="xs" mt="xs">
                  <Button size="xs" variant="subtle" onClick={() => setPlan(null)}>{t('installations.cancel')}</Button>
                  <Button size="xs" color="red" disabled={!ack} loading={busy} onClick={() => void confirm()} data-element="action:ready/installation-delete-confirm">
                    {t('installations.confirm')}
                  </Button>
                </Group>
              </Alert>
            )}
          </Stack>
        ))}
      </Stack>
    </Paper>
  )
}

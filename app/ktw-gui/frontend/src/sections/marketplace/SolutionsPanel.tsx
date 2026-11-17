// SPDX-License-Identifier: Apache-2.0
/**
 * SolutionsPanel — the Marketplace's Solutions tab. A solution is a fixed
 * version of a working installation: the infra version, its sidecars at their
 * versions, its config-sets, its seed, its vaults — and its documents (README,
 * Quoi de neuf…) per language. Each is listed with what it pins; the form below
 * installs one into a universe (its vaults' passwords and the keys asked).
 * What it needs — programs, keys, logins, summed from its members by the
 * catalog — is listed before it is installed. Its « Modèle de projet » line
 * carries the « Démo » badge when that template is demo data.
 */

import { useCallback, useEffect, useState } from 'react'
import { Badge, Box, Divider, Group, ScrollArea, Stack, Text, Title } from '@mantine/core'
import { useAppStore } from '../../store'
import type { MarketplaceClient } from './SubscriptionsDrawer'
import type { GetItemOutput } from '../../ktsp/marketplace'
import { listUniversesWithStatus } from '../../ktsp/universes'
import { SolutionInstallForm } from '../univers/SolutionInstallForm'
import { useT } from '../../i18n'
import { isDemoSeed, type ListedSeed } from './demoSeeds'

type SolutionVersion = NonNullable<GetItemOutput['versions']>[number]
type Translate = ReturnType<typeof useT>

/** What a solution needs before it is installed: its programs (version, an
 *  optional one marked), its keys and its logins, as the catalog sums them. */
function SolutionNeeds({ v, t }: { v: SolutionVersion; t: Translate }) {
  const req = v.requires
  const tools = req?.tools ?? []
  const keys = req?.apiKeys ?? []
  const logins = req?.auth ?? []
  if (tools.length + keys.length + logins.length === 0) return null
  const mark = (name: string, required?: boolean, version?: string) =>
    `${name}${version ? ` ${version}` : ''}${required ? '' : ` (${t('solutions.optional')})`}`
  return (
    <Box mt={4} data-guide="solution.needs">
      <Text size="xs" fw={600}>{t('solutions.needs')}</Text>
      {tools.length > 0 && <Text size="xs" c="dimmed">{t('solutions.programs', { list: tools.map((p) => mark(p.name, p.required, p.version)).join(', ') })}</Text>}
      {keys.length > 0 && <Text size="xs" c="dimmed">{t('solutions.keys', { list: keys.map((k) => mark(k.name, k.required)).join(', ') })}</Text>}
      {logins.length > 0 && <Text size="xs" c="dimmed">{t('solutions.logins', { list: logins.map((l) => l.label || l.name).join(', ') })}</Text>}
    </Box>
  )
}
interface Row { name: string; displayName?: string; marketplace: string; latest?: SolutionVersion; seedDemo?: boolean }

export function SolutionsPanel({ mkt, reloadKey }: { mkt: MarketplaceClient; reloadKey: number }) {
  const t = useT('marketplace')
  const pushToast = useAppStore((s) => s.pushToast)
  const setUniverses = useAppStore((s) => s.setUniverses)
  const activateUniverse = useAppStore((s) => s.activateUniverse)
  const [rows, setRows] = useState<Row[]>([])
  const [formKey, setFormKey] = useState(0)

  const load = useCallback(async () => {
    try {
      const items = ((await mkt.searchItems({ query: '' })) ?? []).filter((i) => i.kind === 'solution')
      // The seeds' demo flag, for the « Démo » badge on a solution's template line;
      // a list that fails only drops the badge.
      const seeds: ListedSeed[] = items.length > 0 ? await mkt.listSeeds().then((l) => l ?? [], () => []) : []
      const out: Row[] = []
      for (const it of items) {
        const full = await mkt.getItem({ name: it.name })
        const latest = full.versions?.[0]
        const seedDemo = latest?.seed ? isDemoSeed(seeds, latest.seed.name, it.marketplace) : false
        out.push({ name: it.name, displayName: it.displayName, marketplace: it.marketplace, latest, seedDemo })
      }
      setRows(out)
    } catch (e) {
      pushToast({ message: e instanceof Error ? e.message : String(e), color: 'red' })
    }
  }, [mkt, pushToast])
  useEffect(() => { void load() }, [load, reloadKey])

  return (
    <ScrollArea style={{ height: '100%' }}>
      <Stack gap="md" pb="md">
        <Box>
          <Title order={5}>{t('solutions.title')}</Title>
          <Text size="xs" c="dimmed">{t('solutions.hint')}</Text>
        </Box>
        {rows.length === 0 && <Text size="xs" c="dimmed">{t('solutions.none')}</Text>}
        {rows.map((r) => {
          const v = r.latest
          return (
            <Box key={`${r.marketplace}/${r.name}`} data-guide="solution.row" data-guide-name={r.name} p="xs"
              style={{ border: '1px solid var(--mantine-color-default-border)', borderRadius: 6 }}>
              <Group gap={6} wrap="nowrap">
                <Text fw={600} size="sm">{r.displayName || r.name}</Text>
                {v && <Badge size="xs" variant="light">{v.version}</Badge>}
                {v?.infra && <Badge size="xs" variant="outline">{t('solutions.infra', { version: v.infra })}</Badge>}
                <Badge size="xs" variant="outline" color="gray">{r.marketplace}</Badge>
              </Group>
              {v && (
                <Stack gap={2} mt={4}>
                  <Text size="xs" c="dimmed">{t('solutions.sidecars', { list: (v.includes ?? []).map((m) => `${m.name}@${m.version}`).join(', ') || '—' })}</Text>
                  {(v.configSets ?? []).length > 0 && <Text size="xs" c="dimmed">{t('solutions.configSets', { list: (v.configSets ?? []).map((c) => `${c.name}@${c.version}`).join(', ') })}</Text>}
                  {v.seed && (
                    <Group gap={6} wrap="nowrap" data-guide="solution.seed">
                      <Text size="xs" c="dimmed">{t('solutions.seed', { name: `${v.seed.name}@${v.seed.version}` })}</Text>
                      {r.seedDemo && <Badge size="xs" variant="filled" color="grape" data-guide="solution.seed-demo">{t('seeds.demo')}</Badge>}
                    </Group>
                  )}
                  {(v.vaults ?? []).length > 0 && <Text size="xs" c="dimmed">{t('solutions.vaults', { list: (v.vaults ?? []).map((c) => `${c.name}@${c.version}`).join(', ') })}</Text>}
                  {(v.documents ?? []).length > 0 && <Text size="xs" c="dimmed">{t('solutions.documents', { list: (v.documents ?? []).map((d) => `${d.lang}/${d.name}`).join(', ') })}</Text>}
                  <SolutionNeeds v={v} t={t} />
                </Stack>
              )}
            </Box>
          )
        })}
        <Divider />
        <Box>
          <Title order={5}>{t('solutions.installTitle')}</Title>
          <Text size="xs" c="dimmed" mb="sm">{t('solutions.installHint')}</Text>
          <SolutionInstallForm
            key={formKey}
            onCreated={(name) => {
              void (async () => {
                setUniverses(await listUniversesWithStatus())
                // Activation starts the universe's core, which bootstraps it.
                await activateUniverse(name)
                pushToast({ message: t('solutions.created', { name }), color: 'green' })
              })()
            }}
            onCancel={() => setFormKey((k) => k + 1)}
          />
        </Box>
      </Stack>
    </ScrollArea>
  )
}

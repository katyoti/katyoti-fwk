// SPDX-License-Identifier: Apache-2.0
/**
 * SeedsPanel — the Marketplace's « Project templates » tab (a seed, by its
 * internal name): a universe's starting content. A seed its publisher marks as
 * demo data carries a « Démo » badge.
 *
 *   - A **seed** (a project — its objects, documents and files — and its tasks)
 *     applies to the universe on screen; a universe serves one project, so the
 *     service refuses a universe that already serves one, before writing.
 */

import { useCallback, useEffect, useMemo, useState } from 'react'
import { Alert, Badge, Box, Button, Group, ScrollArea, Stack, Text, TextInput, Title } from '@mantine/core'
import { IconInfoCircle, IconPlant2 } from '@tabler/icons-react'
import { useAppStore } from '../../store'
import type { MarketplaceClient } from './SubscriptionsDrawer'
import type { ListSeedsOutput } from '../../ktsp/marketplace'
import { useT } from '../../i18n'

type Seed = ListSeedsOutput[number]

export function SeedsPanel({ mkt, reloadKey }: { mkt: MarketplaceClient; reloadKey: number }) {
  const t = useT('marketplace')
  const pushToast = useAppStore((s) => s.pushToast)
  const universe = useAppStore((s) => s.displayedUniverseName)
  const universes = useAppStore((s) => s.universes)
  const universeHome = useMemo(() => universes.find((u) => u.name === universe)?.homePath ?? '', [universes, universe])

  const [seeds, setSeeds] = useState<Seed[]>([])
  const [selected, setSelected] = useState<string | null>(null)
  const [projectDir, setProjectDir] = useState('')
  const [busy, setBusy] = useState(false)

  const load = useCallback(async () => {
    try { setSeeds((await mkt.listSeeds()) ?? []) } catch (e) {
      pushToast({ message: e instanceof Error ? e.message : String(e), color: 'red' })
    }
  }, [mkt, pushToast])
  useEffect(() => { void load() }, [load, reloadKey])

  const apply = async (name: string) => {
    if (!universeHome) return
    setBusy(true)
    try {
      const out = await mkt.applySeed({ name, universeHome, projectDir: projectDir.trim() || undefined })
      pushToast({ message: t('seeds.applied', { name: out.name, project: out.project, tasks: out.tasks ?? 0 }), color: 'green' })
      if (out.differs?.length) pushToast({ message: t('seeds.differs', { members: out.differs.join(', ') }), color: 'yellow' })
      setSelected(null)
      setProjectDir('')
    } catch (e) {
      pushToast({ message: e instanceof Error ? e.message : String(e), color: 'red' })
    } finally {
      setBusy(false)
    }
  }

  return (
    <ScrollArea style={{ height: '100%' }}>
      <Stack gap="md" pb="md">
        <Box>
          <Title order={5}>{t('seeds.title')}</Title>
          <Text size="xs" c="dimmed">{t('seeds.hint')}</Text>
        </Box>
        {seeds.length === 0 && (
          <Alert color="gray" variant="light" icon={<IconInfoCircle size={16} />}>
            <Text size="xs">{t('seeds.none')}</Text>
          </Alert>
        )}
        {seeds.map((sd) => (
          <Box key={`${sd.marketplace}/${sd.name}`} data-guide="seed.row" data-guide-name={sd.name} p="xs" style={{ border: '1px solid var(--mantine-color-default-border)', borderRadius: 6 }}>
            <Group justify="space-between" wrap="nowrap" align="flex-start">
              <Box style={{ minWidth: 0 }}>
                <Group gap={6} wrap="nowrap">
                  <IconPlant2 size={14} />
                  <Text fw={600} size="sm" truncate>{sd.displayName || sd.name}</Text>
                  {sd.demo && <Badge size="xs" variant="filled" color="grape" data-guide="seed.demo">{t('seeds.demo')}</Badge>}
                  {sd.latest && <Badge size="xs" variant="light">{sd.latest}</Badge>}
                  <Badge size="xs" variant="outline" color="gray">{sd.marketplace}</Badge>
                </Group>
                {sd.description && <Text size="xs" c="dimmed" lineClamp={2}>{sd.description}</Text>}
              </Box>
              <Button size="xs" variant="light" disabled={!universeHome} onClick={() => setSelected(selected === sd.name ? null : sd.name)}>
                {t('seeds.applyTo', { universe: universe ?? '—' })}
              </Button>
            </Group>
            {selected === sd.name && (
              <Group mt="sm" align="flex-end" gap="xs">
                <TextInput
                  size="xs"
                  style={{ flex: 1 }}
                  label={t('seeds.projectDir')}
                  description={t('seeds.projectDirHint')}
                  value={projectDir}
                  onChange={(e) => setProjectDir(e.currentTarget.value)}
                />
                <Button size="xs" loading={busy} onClick={() => void apply(sd.name)}>{t('seeds.apply')}</Button>
              </Group>
            )}
          </Box>
        ))}

      </Stack>
    </ScrollArea>
  )
}

// SPDX-License-Identifier: Apache-2.0
/**
 * ContributionsView — the Contributions panel of the system module that
 * operates the project's contributions (sys-contributions; the
 * project-contributions pack's `object-panels/contributions.yaml`). Its input
 * is project-contributions/Panel's answer: what the universe publishes under
 * the project's provider token, a group per kind. The groups are on the left,
 * with a filter; the entry picked is read in place on the right (its meta,
 * layers and provider, as the Debug tab's browser shows it). The hub serves
 * the folder as a member, so an edit shows once the catalog changes: the view
 * re-reads its source on `catalog.changed` (the hub's contributions-changed).
 */

import { useEffect, useMemo, useState } from 'react'
import { Badge, Box, Center, Group, ScrollArea, Stack, Text, TextInput, UnstyledButton } from '@mantine/core'
import { IconPackage, IconSearch } from '@tabler/icons-react'
import { EventsOn } from '../../../wailsjs/runtime/runtime'
import { main } from '../../../wailsjs/go/models'
import { ContributionReader } from '../dev/ContributionBrowserPanel'
import { useT } from '../../i18n'
import { asContributions, countOf, filterGroups, filterOverrides, partsOf, type ContributionEntry } from './contributionsModel'

interface Props {
  universeName: string
  input: unknown
  refresh?: () => void
}

export function ContributionsView({ universeName, input, refresh }: Props) {
  const t = useT('workspace')
  const data = useMemo(() => asContributions(input), [input])
  const [filter, setFilter] = useState('')
  const [picked, setPicked] = useState<{ kind: string; entry: ContributionEntry; provider?: string; layers?: string[] } | null>(null)
  const groups = useMemo(() => filterGroups(data.groups, filter), [data.groups, filter])
  const overrides = useMemo(() => filterOverrides(data.overrides, filter), [data.overrides, filter])

  // The catalog changed (a file of the folder edited, added, removed): re-read.
  useEffect(() => {
    if (!refresh) return
    const off = EventsOn('catalog.changed', (evt: { universe?: string }) => {
      if (!evt?.universe || evt.universe === universeName) refresh()
    })
    return () => { off() }
  }, [universeName, refresh])

  const total = countOf(data.groups)
  if (total === 0 && data.overrides.length === 0) {
    return (
      <Center p="xl" style={{ height: '100%' }}>
        <Stack align="center" gap={6} maw={420}>
          <IconPackage size={32} color="var(--mantine-color-dimmed)" />
          <Text size="sm" fw={500}>{t('contributions.empty')}</Text>
          <Text size="xs" c="dimmed" ta="center">{t('contributions.emptyHint', { folder: data.folder, provider: data.provider })}</Text>
        </Stack>
      </Center>
    )
  }
  return (
    <Group align="stretch" gap={0} wrap="nowrap" style={{ height: '100%' }}>
      <Stack gap="xs" p="sm" w={340} style={{ borderRight: '1px solid var(--mantine-color-default-border)', flexShrink: 0 }}>
        <Group gap={6} justify="space-between" wrap="nowrap">
          <Group gap={6} wrap="nowrap">
            <Badge variant="light" ff="monospace" style={{ textTransform: 'none' }}>{data.provider}</Badge>
            <Text size="xs" c="dimmed">{t('contributions.count', { count: total })}</Text>
          </Group>
          <Text size="xs" c="dimmed" ff="monospace" truncate maw={140}>{data.folder}</Text>
        </Group>
        <TextInput size="xs" leftSection={<IconSearch size={14} />} placeholder={t('contributions.filter')}
          value={filter} onChange={(e) => setFilter(e.currentTarget.value)} />
        <ScrollArea style={{ flex: 1 }}>
          <Stack gap="sm">
            {groups.map((g) => (
              <Box key={g.kind}>
                <Group gap={6} mb={4}>
                  <Text size="xs" fw={600} tt="uppercase" c="dimmed">{g.kind}</Text>
                  <Badge size="xs" variant="default">{g.entries.length}</Badge>
                </Group>
                <Stack gap={2}>
                  {g.entries.map((e) => {
                    const on = picked?.entry.uri === e.uri
                    return (
                      <UnstyledButton key={e.uri} onClick={() => setPicked({ kind: g.kind, entry: e })} px={8} py={6}
                        style={{
                          borderRadius: 6,
                          background: on ? 'var(--mantine-color-default-hover)' : undefined,
                          borderLeft: `3px solid ${on ? 'var(--mantine-primary-color-filled)' : 'transparent'}`,
                        }}>
                        <Text size="sm" fw={500} truncate>{e.name || e.id}</Text>
                        {e.description ? <Text size="xs" c="dimmed" lineClamp={2}>{e.description}</Text> : null}
                      </UnstyledButton>
                    )
                  })}
                </Stack>
              </Box>
            ))}
            {groups.length === 0 && overrides.length === 0 ? <Text size="xs" c="dimmed">{t('contributions.noMatch')}</Text> : null}
            {overrides.length > 0 ? (
              <Box>
                <Group gap={6} mb={4}>
                  <Text size="xs" fw={600} tt="uppercase" c="dimmed">{t('contributions.overrides')}</Text>
                  <Badge size="xs" variant="default">{overrides.length}</Badge>
                </Group>
                <Stack gap={2}>
                  {overrides.map((o) => {
                    const at = partsOf(o.uri)
                    const on = picked?.entry.uri === o.uri
                    return (
                      <UnstyledButton key={o.uri} px={8} py={6}
                        onClick={() => setPicked({ kind: at.kind, entry: { uri: o.uri, id: at.id }, provider: at.provider, layers: o.layers })}
                        style={{
                          borderRadius: 6,
                          background: on ? 'var(--mantine-color-default-hover)' : undefined,
                          borderLeft: `3px solid ${on ? 'var(--mantine-primary-color-filled)' : 'transparent'}`,
                        }}>
                        <Text size="sm" fw={500} ff="monospace" truncate>{o.uri}</Text>
                        <Group gap={4} mt={2}>
                          {o.layers.map((l) => <Badge key={l} size="xs" variant="light" color="orange">{l}</Badge>)}
                        </Group>
                      </UnstyledButton>
                    )
                  })}
                </Stack>
              </Box>
            ) : null}
          </Stack>
        </ScrollArea>
      </Stack>
      <Box style={{ flex: 1, minWidth: 0 }}>
        {picked ? (
          <Stack gap={0} style={{ height: '100%' }}>
            {picked.layers ? (
              <Group gap={6} px="sm" py={6} style={{ borderBottom: '1px solid var(--mantine-color-default-border)' }}>
                <Badge variant="light" color="orange">{t('contributions.overriddenBadge')}</Badge>
                <Text size="xs" c="dimmed">{t('contributions.overriddenLayers', { layers: picked.layers.join(', ') })}</Text>
              </Group>
            ) : null}
            <Box style={{ flex: 1, minHeight: 0 }}>
              <ContributionReader universeName={universeName} summary={main.ContributionSummary.createFrom({
                provider: picked.provider ?? data.provider, type: picked.kind, id: picked.entry.id, uri: picked.entry.uri,
                name: picked.entry.name, description: picked.entry.description,
              })} />
            </Box>
          </Stack>
        ) : (
          <Center style={{ height: '100%' }}><Text size="sm" c="dimmed">{t('contributions.pick')}</Text></Center>
        )}
      </Box>
    </Group>
  )
}

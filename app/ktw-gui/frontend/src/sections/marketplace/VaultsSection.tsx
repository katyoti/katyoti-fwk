// SPDX-License-Identifier: Apache-2.0
/**
 * VaultsSection — the vaults your marketplaces offer (sealed keys files), each
 * imported into the universe on screen with its password: its keys written,
 * each into its scope's .env. Shown in the Config-sets tab, where keys are
 * needed.
 */

import { useCallback, useEffect, useMemo, useState } from 'react'
import { Badge, Box, Button, Group, PasswordInput, Stack, Text, Title } from '@mantine/core'
import { useAppStore } from '../../store'
import { marketplaceClient } from '../../ktsp'
import type { SearchItemsOutput } from '../../ktsp/marketplace'
import { useT } from '../../i18n'
import { RestartForKeys } from './RestartForKeys'

type Vault = SearchItemsOutput[number]

export function VaultsSection() {
  const t = useT('marketplace')
  const pushToast = useAppStore((s) => s.pushToast)
  const universe = useAppStore((s) => s.displayedUniverseName)
  const universes = useAppStore((s) => s.universes)
  const universeHome = useMemo(() => universes.find((u) => u.name === universe)?.homePath ?? '', [universes, universe])
  const mkt = useMemo(() => marketplaceClient(), [])
  const [vaults, setVaults] = useState<Vault[]>([])
  const [passwords, setPasswords] = useState<Record<string, string>>({})
  const [busy, setBusy] = useState('')
  const [keysWritten, setKeysWritten] = useState(false)

  const load = useCallback(async () => {
    try {
      setVaults(((await mkt.searchItems({ query: '' })) ?? []).filter((i) => i.kind === 'vault'))
    } catch { /* no vault list: the config-sets above still work */ }
  }, [mkt])
  useEffect(() => { void load() }, [load])

  const importVault = async (name: string) => {
    setBusy(name)
    try {
      const out = await mkt.importVault({ name, universeHome, password: passwords[name] ?? '' })
      pushToast({ message: t('vaults.imported', { count: out.written?.length ?? 0, name }), color: 'green' })
      setPasswords((p) => ({ ...p, [name]: '' }))
      if (out.written?.length) setKeysWritten(true)
    } catch (e) {
      pushToast({ message: e instanceof Error ? e.message : String(e), color: 'red' })
    } finally {
      setBusy('')
    }
  }

  return (
    <Box mt="md">
      <Title order={5}>{t('vaults.title')}</Title>
      <Text size="xs" c="dimmed" mb="xs">{t('vaults.hint')}</Text>
      {keysWritten && universe && <RestartForKeys universe={universe} onDone={() => setKeysWritten(false)} />}
      {vaults.length === 0 && <Text size="xs" c="dimmed">{t('vaults.none')}</Text>}
      <Stack gap="xs">
        {vaults.map((v) => (
          <Group key={`${v.marketplace}/${v.name}`} align="flex-end" gap="xs" wrap="nowrap" data-guide="vault.row" data-guide-name={v.name}>
            <Group gap={6} wrap="nowrap" style={{ minWidth: 160 }}>
              <Text size="sm" fw={600}>{v.name}</Text>
              {v.latest && <Badge size="xs" variant="light">{v.latest}</Badge>}
            </Group>
            <PasswordInput size="xs" style={{ flex: 1 }} placeholder={t('vaults.password')}
              value={passwords[v.name] ?? ''} onChange={(e) => setPasswords((p) => ({ ...p, [v.name]: e.currentTarget.value }))} />
            <Button size="xs" variant="light" disabled={!universeHome || !(passwords[v.name] ?? '')} loading={busy === v.name}
              onClick={() => void importVault(v.name)}>
              {t('vaults.import', { universe: universe ?? '—' })}
            </Button>
          </Group>
        ))}
      </Stack>
    </Box>
  )
}

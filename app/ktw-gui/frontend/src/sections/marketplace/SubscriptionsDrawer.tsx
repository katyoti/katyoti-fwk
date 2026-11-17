// SPDX-License-Identifier: Apache-2.0
/**
 * SubscriptionsDrawer — the marketplaces this machine subscribes to and their
 * trust: add (a pasted public key pins trust), enable / disable, refresh a
 * catalog, remove. Shared by every Marketplace tab (the subscriptions are
 * global, not per universe); `onChanged` lets the tabs reload their catalogs.
 */

import { useCallback, useEffect, useState } from 'react'
import { ActionIcon, Badge, Button, Drawer, Group, Stack, Table, Text, TextInput, Tooltip } from '@mantine/core'
import { IconRefresh, IconX } from '@tabler/icons-react'
import { useAppStore } from '../../store'
import type { marketplaceClient } from '../../ktsp'
import type { ListMarketplacesOutput } from '../../ktsp/marketplace'
import { useT } from '../../i18n'

type Marketplace = ListMarketplacesOutput[number]
export type MarketplaceClient = ReturnType<typeof marketplaceClient>

interface Props {
  mkt: MarketplaceClient
  opened: boolean
  onClose: () => void
  onChanged: () => void
  /** Reports the subscription count for the header button. */
  onCount: (n: number) => void
}

export function SubscriptionsDrawer({ mkt, opened, onClose, onChanged, onCount }: Props) {
  const t = useT('marketplace')
  const pushToast = useAppStore((s) => s.pushToast)
  const [markets, setMarkets] = useState<Marketplace[]>([])
  const [busy, setBusy] = useState('')
  const [addName, setAddName] = useState('')
  const [addUrl, setAddUrl] = useState('')
  const [addPubKey, setAddPubKey] = useState('')

  const fail = useCallback((e: unknown) => {
    pushToast({ message: e instanceof Error ? e.message : String(e), color: 'red' })
  }, [pushToast])

  // `?? []`: an older hub can answer `null` for an empty list.
  const refresh = useCallback(async () => {
    try {
      const m = (await mkt.listMarketplaces()) ?? []
      setMarkets(m)
      onCount(m.length)
    } catch (e) { fail(e) }
  }, [mkt, fail, onCount])

  useEffect(() => { void refresh() }, [refresh])

  const op = async (name: string, run: () => Promise<unknown>, ok?: string) => {
    setBusy(name)
    try {
      await run()
      if (ok) pushToast({ message: ok, color: 'green' })
      await refresh()
      onChanged()
    } catch (e) {
      fail(e)
    } finally {
      setBusy('')
    }
  }

  const add = async () => {
    if (!addName.trim() || !addUrl.trim()) { pushToast({ message: t('subscriptions.nameUrlRequired'), color: 'red' }); return }
    // A pasted public key pins trust — mirrors the CLI's `--pubkey implies --trust pinned`.
    const pk = addPubKey.trim()
    await op(addName, () => mkt.addMarketplace({ name: addName, url: addUrl, pubKey: pk, trust: pk ? 'pinned' : 'none' }), t('subscriptions.added', { name: addName }))
    setAddName(''); setAddUrl(''); setAddPubKey('')
  }

  return (
    <Drawer opened={opened} onClose={onClose} title={t('subscriptions.title')} position="right" size="lg">
      <Stack gap="xs" mb="sm">
        <Group align="flex-end" gap="xs">
          <TextInput label={t('subscriptions.name')} size="xs" value={addName} onChange={(e) => setAddName(e.currentTarget.value)} style={{ width: 140 }} />
          <TextInput label={t('subscriptions.url')} size="xs" value={addUrl} onChange={(e) => setAddUrl(e.currentTarget.value)} style={{ flex: 1 }} />
          <Button size="xs" onClick={() => void add()}>{t('subscriptions.add')}</Button>
        </Group>
        <TextInput
          label={t('subscriptions.pubKey')}
          placeholder={t('subscriptions.pubKeyPlaceholder')}
          size="xs"
          value={addPubKey}
          onChange={(e) => setAddPubKey(e.currentTarget.value)}
          rightSection={addPubKey.trim() ? <Badge size="xs" color="green">{t('subscriptions.pinned')}</Badge> : undefined}
          rightSectionWidth={addPubKey.trim() ? 64 : 0}
        />
      </Stack>
      <Table highlightOnHover withRowBorders={false}>
        <Table.Thead>
          <Table.Tr>
            <Table.Th>{t('subscriptions.name')}</Table.Th><Table.Th>{t('subscriptions.trust')}</Table.Th>
            <Table.Th>{t('subscriptions.source')}</Table.Th><Table.Th>{t('subscriptions.enabled')}</Table.Th><Table.Th />
          </Table.Tr>
        </Table.Thead>
        <Table.Tbody>
          {markets.map((m) => (
            <Table.Tr key={m.name}>
              <Table.Td><Text size="sm" fw={600}>{m.name}</Text><Text size="xs" c="dimmed" truncate style={{ maxWidth: 260 }}>{m.url}</Text></Table.Td>
              <Table.Td><Badge size="xs" color={m.trust === 'pinned' ? 'green' : 'gray'}>{m.trust || 'none'}</Badge></Table.Td>
              <Table.Td><Text size="xs" c="dimmed">{m.source}</Text></Table.Td>
              <Table.Td>
                <Badge size="xs" color={m.enabled ? 'green' : 'gray'} style={{ cursor: 'pointer' }} onClick={() => void op(m.name, () => mkt.setMarketplaceEnabled({ name: m.name, enabled: !m.enabled }))}>
                  {m.enabled ? t('subscriptions.on') : t('subscriptions.off')}
                </Badge>
              </Table.Td>
              <Table.Td>
                <Group gap={4} justify="flex-end" wrap="nowrap">
                  <Tooltip label={t('subscriptions.refresh')}><ActionIcon variant="subtle" loading={busy === m.name} onClick={() => void op(m.name, () => mkt.refreshCatalog({ name: m.name }), t('subscriptions.refreshed'))} aria-label={t('subscriptions.refresh')}><IconRefresh size={15} /></ActionIcon></Tooltip>
                  <Tooltip label={t('subscriptions.remove')}><ActionIcon variant="subtle" color="red" loading={busy === m.name} onClick={() => void op(m.name, () => mkt.removeMarketplace({ name: m.name }), t('subscriptions.removed', { name: m.name }))} aria-label={t('subscriptions.remove')}><IconX size={15} /></ActionIcon></Tooltip>
                </Group>
              </Table.Td>
            </Table.Tr>
          ))}
          {markets.length === 0 && (
            <Table.Tr><Table.Td colSpan={5}><Text c="dimmed" size="sm">{t('subscriptions.none')}</Text></Table.Td></Table.Tr>
          )}
        </Table.Tbody>
      </Table>
    </Drawer>
  )
}

// SPDX-License-Identifier: Apache-2.0
/**
 * InfraPanel — the Marketplace's Infra tab: the control plane (katyoti, ktw-hub,
 * ktw-gui — it moves as a set), and the snapshots with the infra version each
 * was made with.
 *
 * Reading is plain KTSP (infraVersions, listInstalled, getItem). MOVING the
 * control plane is not: it replaces the running ktw-gui and ktw-hub, so every
 * move hands off to `katyoti gui` through ONE Wails binding
 * (RelaunchWithInfra) — the window closes, katyoti moves the control plane with
 * nothing running, and the new version opens. The verbs:
 *
 *   - update to the channel's current version, roll back to its previous one
 *     (`--infra-version`), reinstall the installed one (`--repair`);
 *   - align the control plane to a snapshot's infra version, then create the
 *     universe from the snapshot (Seeds) once the new version is open.
 *
 * Each is confirmed first, saying it is MACHINE-WIDE: every universe of this
 * machine runs on this control plane. Sidecars are never moved from here: they
 * update in the Sidecars tab.
 */

import { useCallback, useEffect, useState } from 'react'
import { Alert, Badge, Button, Group, Modal, Stack, Table, Text } from '@mantine/core'
import { ItemCard, SectionCard } from '@ktw/display-lib/ui-core/components/other/index.ts'
import { IconAlertTriangle, IconArrowBackUp, IconRefresh, IconShieldCheck, IconTool } from '@tabler/icons-react'
import { useAppStore } from '../../store'
import type { MarketplaceSection } from '../../store/tabsSlice'
import type { MarketplaceClient } from './SubscriptionsDrawer'
import type { InfraVersionsOutput, ListInstalledOutput } from '../../ktsp/marketplace'
import { useT } from '../../i18n'

/** What install-infra stamps on a control-plane record (core.SourceInfraChannel). */
const INFRA_SOURCE = 'infra-channel'

interface Snapshot { name: string; displayName?: string; version: string; infra?: string }

/** A move of the control plane, waiting for confirmation. */
interface Move { version: string; repair: boolean; what: string }

interface Props {
  mkt: MarketplaceClient
  reloadKey: number
  onSection: (s: MarketplaceSection) => void
}

export function InfraPanel({ mkt, reloadKey, onSection }: Props) {
  const t = useT('marketplace')
  const universe = useAppStore((s) => s.displayedUniverseName)
  const [info, setInfo] = useState<InfraVersionsOutput | null>(null)
  const [binaries, setBinaries] = useState<ListInstalledOutput>([])
  const [snapshots, setSnapshots] = useState<Snapshot[]>([])
  const [error, setError] = useState<string | null>(null)
  const [pending, setPending] = useState<Move | null>(null)
  const [moving, setMoving] = useState(false)

  const load = useCallback(async () => {
    try {
      setInfo(await mkt.infraVersions())
      setError(null)
    } catch (err) {
      // A channel this machine cannot reach must not break the tab.
      setError(err instanceof Error ? err.message : String(err))
    }
    try {
      setBinaries(((await mkt.listInstalled()) ?? []).filter((i) => i.source === INFRA_SOURCE))
    } catch { /* the versions above already say what matters */ }
    try {
      const solutions = ((await mkt.searchItems({ query: '' })) ?? []).filter((i) => i.kind === 'solution')
      const out: Snapshot[] = []
      for (const r of solutions) {
        const item = await mkt.getItem({ name: r.name })
        const v = item.versions?.[0]
        if (v) out.push({ name: r.name, displayName: r.displayName, version: v.version, infra: v.infra })
      }
      setSnapshots(out)
    } catch { /* no snapshot list: the control plane is still shown */ }
  }, [mkt])

  useEffect(() => { void load() }, [load, reloadKey])

  const relaunch = async (m: Move) => {
    setMoving(true)
    const w = window as unknown as {
      go?: { main?: { App?: { RelaunchWithInfra?: (v: string, r: boolean, u: string) => Promise<{ error?: string }> } } }
    }
    const fn = w.go?.main?.App?.RelaunchWithInfra
    if (!fn) {
      setError(t('infra.relaunchUnavailable', { cmd: m.repair ? 'katyoti gui --repair' : `katyoti gui --infra-version ${m.version}` }))
      setMoving(false)
      setPending(null)
      return
    }
    const res = await fn(m.version, m.repair, universe ?? '')
    // On success this window is closing; only a failure needs the buttons back.
    if (res?.error) {
      setError(res.error)
      setMoving(false)
      setPending(null)
    }
  }

  const installed = info?.installed ?? ''
  // A control plane make dev placed is shown as the dev build
  // it is, never « not installed » nor behind.
  const devBuild = !!info?.devBuild
  const behind = !devBuild && !!installed && !!info && installed !== info.current
  const canMoveTo = (v?: string) => !!v && !!info && (v === info.current || v === info.previous)

  return (
    <Stack gap="md" pb="md">
      {error && (
        <Alert color="gray" variant="light" icon={<IconAlertTriangle size={16} />}>
          <Text size="xs">{t('infra.unavailable', { error })}</Text>
        </Alert>
      )}

      {info && (
        <SectionCard title={t('infra.title')} icon="server" color={devBuild ? 'violet' : behind ? 'orange' : installed ? 'green' : 'gray'}
          actions={<>
            {devBuild ? (
              <Badge size="sm" color="violet" variant="light" data-guide="infra.dev-build">{t('infra.devBuild', { version: installed })}</Badge>
            ) : !installed ? (
              <Badge size="sm" color="gray" variant="light">{t('infra.notInstalled')}</Badge>
            ) : behind ? (
              <Badge size="sm" color="orange" variant="light">{installed} → {info.current}</Badge>
            ) : (
              <Badge size="sm" color="green" variant="light" leftSection={<IconShieldCheck size={11} />}>{installed}</Badge>
            )}
            <Button size="xs" variant="subtle" leftSection={<IconRefresh size={13} />} onClick={() => void load()}>{t('infra.check')}</Button>
          </>}>
          <Text size="xs" c="dimmed" mb="xs">{t('infra.set', { binaries: (info.binaries ?? []).join(' + ') })}</Text>
          {devBuild && <Text size="xs" mb="xs" data-guide="infra.dev-hint">{t('infra.devHint')}</Text>}

          <Table withRowBorders={false} verticalSpacing={2}>
            <Table.Tbody>
              <Table.Tr><Table.Td><Text size="xs">{t('infra.channel')}</Text></Table.Td><Table.Td><Text size="xs" c="dimmed">{info.channel}</Text></Table.Td></Table.Tr>
              <Table.Tr><Table.Td><Text size="xs">{t('infra.current')}</Text></Table.Td><Table.Td><Text size="xs">{info.current}</Text></Table.Td></Table.Tr>
              <Table.Tr><Table.Td><Text size="xs">{t('infra.previous')}</Text></Table.Td><Table.Td><Text size="xs">{info.previous || '—'}</Text></Table.Td></Table.Tr>
              {binaries.map((b) => (
                <Table.Tr key={b.name}><Table.Td><Text size="xs" fw={600}>{b.name}</Text></Table.Td><Table.Td><Text size="xs">{b.version}</Text></Table.Td></Table.Tr>
              ))}
            </Table.Tbody>
          </Table>

          <Group gap="xs" mt="sm">
            {devBuild && !!info.current && (
              <Button size="xs" variant="light" color="orange" data-guide="infra.replace-dev" onClick={() => setPending({ version: info.current, repair: false, what: t('infra.whatReplaceDev', { from: installed, to: info.current }) })}>
                {t('infra.replaceDev', { version: info.current })}
              </Button>
            )}
            {behind && (
              <Button size="xs" color="orange" onClick={() => setPending({ version: info.current, repair: false, what: t('infra.whatUpdate', { from: installed, to: info.current }) })}>
                {t('infra.update', { version: info.current })}
              </Button>
            )}
            {!devBuild && !!info.previous && info.previous !== installed && (
              <Button size="xs" variant="light" leftSection={<IconArrowBackUp size={13} />} onClick={() => setPending({ version: info.previous ?? '', repair: false, what: t('infra.whatRollback', { from: installed, to: info.previous }) })}>
                {t('infra.rollback', { version: info.previous })}
              </Button>
            )}
            {!devBuild && !!installed && (
              <Button size="xs" variant="subtle" leftSection={<IconTool size={13} />} onClick={() => setPending({ version: '', repair: true, what: t('infra.whatRepair', { version: info.current }) })}>
                {t('infra.repair')}
              </Button>
            )}
          </Group>
          <Text size="xs" c="dimmed" mt={6}>{t('infra.sidecarsElsewhere')}</Text>
        </SectionCard>
      )}

      <SectionCard title={t('infra.snapshotsTitle')} icon="camera" color="blue" description={t('infra.snapshotsHint')}>
        {snapshots.length === 0 && <Text size="xs" c="dimmed">{t('infra.noSnapshots')}</Text>}
        <Stack gap="xs">
          {snapshots.map((s) => {
            const aligned = !s.infra || s.infra === installed
            return (
              <ItemCard key={s.name} padding="xs" data-guide="infra.snapshot" data-guide-name={s.name}>
                <Group justify="space-between" wrap="nowrap">
                <Group gap={6} wrap="nowrap">
                  <Text size="sm" fw={600}>{s.displayName || s.name}</Text>
                  <Badge size="xs" variant="light">{s.version}</Badge>
                  {s.infra && <Badge size="xs" variant="outline" color={aligned ? 'green' : 'orange'}>{t('infra.madeWith', { version: s.infra })}</Badge>}
                </Group>
                {aligned ? (
                  <Button size="xs" onClick={() => onSection('solutions')}>{t('infra.createUniverse')}</Button>
                ) : canMoveTo(s.infra) ? (
                  <Button size="xs" color="orange" onClick={() => setPending({ version: s.infra ?? '', repair: false, what: t('infra.whatAlign', { name: s.name, from: installed, to: s.infra }) })}>
                    {t('infra.align', { version: s.infra })}
                  </Button>
                ) : (
                  <Text size="xs" c="dimmed">{t('infra.notOnChannel', { version: s.infra })}</Text>
                )}
              </Group>
              </ItemCard>
            )
          })}
        </Stack>
      </SectionCard>

      <Modal opened={!!pending} onClose={() => !moving && setPending(null)} title={t('infra.confirmTitle')} centered>
        {pending && (
          <Stack gap="sm">
            <Text size="sm">{pending.what}</Text>
            <Alert color="orange" variant="light" icon={<IconAlertTriangle size={16} />}>
              <Text size="xs">{t('infra.machineWide')}</Text>
            </Alert>
            <Group justify="flex-end" gap="xs">
              <Button variant="subtle" size="xs" disabled={moving} onClick={() => setPending(null)}>{t('infra.cancel')}</Button>
              <Button size="xs" color="orange" loading={moving} onClick={() => void relaunch(pending)}>{t('infra.confirm')}</Button>
            </Group>
          </Stack>
        )}
      </Modal>
    </Stack>
  )
}

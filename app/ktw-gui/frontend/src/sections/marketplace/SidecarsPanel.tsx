// SPDX-License-Identifier: Apache-2.0
/**
 * SidecarsPanel — the Marketplace's Sidecars tab, styled like the VS-Code
 * Extensions view: one unified list of sidecar cards with a text search (name +
 * title + description), a segmented filter (All / Installed / Updatable /
 * In-universe), per-item state badges, and contextual actions; the doctor of
 * every installed item and the updates (all at once) on top. The subscriptions
 * are the Marketplace header's (SubscriptionsDrawer), shared by every tab.
 *
 * Backed by the GENERATED, typed KTSP client (src/ktsp) over the ONE generic
 * Wails dispatcher — there is no per-method Go binding behind this panel, and
 * publishing a service method makes it callable here with no Go change at all.
 * See rule management-plane-hub-authority invariant 5. Calls REJECT on failure,
 * so operations are wrapped once in op() / try-catch rather than each call site
 * testing an error field.
 */

import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  ActionIcon,
  Badge,
  Box,
  Alert,
  Button,
  Group,
  Loader,
  Menu,
  ScrollArea,
  SegmentedControl,
  Stack,
  Text,
  TextInput,
  Tooltip,
} from '@mantine/core'
import {
  IconCheck,
  IconChevronDown,
  IconDownload,
  IconPencil,
  IconSettings,
  IconStethoscope,
  IconTrash,
  IconX,
} from '@tabler/icons-react'
import { useAppStore } from '../../store'
import { ConfigFileEditor, type ConfigEditTarget, type ConfigFileRef } from './ConfigFileEditor'
import type { MarketplaceClient } from './SubscriptionsDrawer'
import type {
  ConfigureItemOutput,
  DoctorOutput,
  ListInstalledOutput,
  OutdatedOutput,
  SearchItemsOutput,
} from '../../ktsp/marketplace'
import { useT } from '../../i18n'
import { installClient, type CallPanel } from '../../ktsp/inPanel'
import { DisplayPanel } from '../panels/DisplayPanel'

// Shapes come FROM the generated client — never re-declared here. A hand-copied
// interface is exactly the drift this design removes.
type ItemSummary = SearchItemsOutput[number]
type Installed = ListInstalledOutput[number]
type ConfigCheckReport = ConfigureItemOutput

// Row is one unified card: a catalog item joined with its installed state.
interface Row {
  name: string
  displayName?: string
  description?: string
  kind?: string
  marketplace?: string
  latest?: string
  installed?: Installed
}

const kindColor = (k?: string) =>
  k === 'sidecar' ? 'blue' : k === 'content' ? 'grape' : k === 'solution' ? 'orange' : 'gray'

/** What install-infra stamps on a control-plane record (core.SourceInfraChannel). */
const INFRA_SOURCE = 'infra-channel'

const isUpdatable = (r: Row) => !!r.installed && !!r.latest && r.installed.version !== r.latest

/** A program of the installation's dev channel (make dev): shown as that build. */
const isDevBuild = (r: Row) => !!r.installed?.devBuild

/** `reloadKey` changes when the subscriptions do: the catalog is read again. */
export function SidecarsPanel({ mkt, reloadKey }: { mkt: MarketplaceClient; reloadKey: number }) {
  const t = useT('marketplace')
  const pushToast = useAppStore((s) => s.pushToast)
  // A dev installation's programs are its dev builds - one row
  // each, never « update available » against a marketplace - and the control
  // plane stays in the Infra tab.
  const updatable = useCallback((r: Row) => !isDevBuild(r) && isUpdatable(r), [])
  // The universe the user is LOOKING AT — what "in universe" has to mean.
  const displayedUniverse = useAppStore((s) => s.displayedUniverseName)

  const [items, setItems] = useState<ItemSummary[]>([])
  const [installed, setInstalled] = useState<Installed[]>([])
  const [query, setQuery] = useState('')
  const [segment, setSegment] = useState('all')
  const [busy, setBusy] = useState<string>('') // name currently mutating
  // The long operations (install, update, set-version, doctor)
  // are drawn in the panel the GUI lends their call - their steps, Cancel,
  // their end - above the list.
  const [callPanel, setCallPanel] = useState<CallPanel | null>(null)
  const longMkt = useMemo(() => {
    const label: Record<string, string> = {
      install: t('sidecars.install'), update: t('sidecars.update'), 'set-version': t('sidecars.setVersion'), doctor: t('sidecars.doctor'),
    }
    return installClient(setCallPanel, (slot, input) => {
      const method = slot.slice(slot.lastIndexOf('/') + 1)
      const name = (input as { name?: string })?.name
      return `${label[method] ?? method}${name ? ` ${name}` : ''}`
    })
  }, [t])
  const [loading, setLoading] = useState(false)
  const [reports, setReports] = useState<Record<string, ConfigCheckReport>>({})
  const [versions, setVersions] = useState<Record<string, string[]>>({})
  const [doctor, setDoctor] = useState<DoctorOutput | null>(null)
  const [outdated, setOutdated] = useState<OutdatedOutput>([])
  // Raw config-file editing (sidecar-config-raw-edit): which item + file is open.
  const [editing, setEditing] = useState<ConfigEditTarget | null>(null)
  // The write must land in the universe ON SCREEN. Over KTSP the marketplace
  // slot is served by the admin hub, whose "current workspace" is ADMIN — so the
  // home is passed explicitly, resolved from the registry row the store holds.
  const universes = useAppStore((s) => s.universes)
  const universeHome = useMemo(
    () => universes.find((u) => u.name === displayedUniverse)?.homePath ?? '',
    [universes, displayedUniverse],
  )
  // No universe on screen → no universe-targeting action. An empty home would
  // fall back to the admin hub's own workspace, silently.
  const noUniverse = !universeHome


  // A failed load leaves the previous data in place and reports once — clearing
  // the list on a transient error would read as "nothing is installed".
  const fail = useCallback((e: unknown) => {
    pushToast({ message: e instanceof Error ? e.message : String(e), color: 'red' })
  }, [pushToast])

  // `?? []` at EVERY list boundary, and it is not belt-and-braces.
  //
  // A Go nil slice marshals to JSON `null`, so a backend method that builds its
  // result with `var out []T` and appends nothing hands us `null` where the
  // generated client's type says array. We then iterate it and the ENTIRE PANEL
  // dies with "x is not iterable" — no list, no toast, no way back. That is
  // what happened here on a machine with no marketplace subscribed and nothing
  // installed: the emptiest, most first-run state there is.
  //
  // The backend was fixed to return `[]`. This stays because the GUI talks to a
  // hub it does not version together with: an older or remote hub can still
  // send `null`, and the right failure for that is an empty list, not a dead
  // view.
  const refreshInstalled = useCallback(async () => {
    try { setInstalled((await mkt.listInstalled()) ?? []) } catch (e) { fail(e) }
  }, [mkt, fail])


  // Load the full catalog once; filtering is client-side (over title + desc).
  const loadCatalog = useCallback(async () => {
    try { setItems((await mkt.searchItems({ query: '' })) ?? []) } catch (e) { fail(e) }
  }, [mkt, fail])

  const loadOutdated = useCallback(async () => {
    try { setOutdated((await mkt.outdated()) ?? []) } catch (e) { fail(e) }
  }, [mkt, fail])

  const reloadAll = useCallback(async () => {
    setLoading(true)
    await Promise.all([refreshInstalled(), loadCatalog(), loadOutdated()])
    setLoading(false)
  }, [refreshInstalled, loadCatalog, loadOutdated])

  useEffect(() => { void reloadAll() }, [reloadKey]) // eslint-disable-line react-hooks/exhaustive-deps

  // The doctor of every installed item, at once: what each still needs.
  const runDoctor = async () => {
    setBusy('(doctor)')
    setCallPanel(null)
    try { setDoctor((await longMkt.doctor({ universeHome })) ?? []) } catch (e) { fail(e) } finally { setBusy('') }
  }
  // Every update at once (the per-row Update button takes one).
  const updateAll = async () => {
    if (await op('(update)', () => { setCallPanel(null); return longMkt.update({ names: outdated.map((o) => o.name) }) }, t('sidecars.updated', { count: outdated.length }))) {
      await reloadAll()
    }
  }

  // Runs one client call with the busy marker and a toast. Takes a THUNK rather
  // than a method name + args: the call is then type-checked at the call site,
  // which is the whole point of the generated client.
  const op = useCallback(
    async (name: string, run: () => Promise<unknown>, okMessage?: string) => {
      setBusy(name)
      try {
        await run()
        if (okMessage) pushToast({ message: okMessage, color: 'green' })
        return true
      } catch (e) {
        fail(e)
        return false
      } finally {
        setBusy('')
      }
    },
    [pushToast, fail],
  )

  // Drives the item's own config surface and renders the report inline. Called
  // both by the explicit Configure button and automatically right after an
  // install (see `install`).
  //
  // Every universe-targeting call below passes `universeHome` EXPLICITLY. The
  // marketplace slot is served by the admin hub, whose "current workspace" is
  // ADMIN: without the home, wire/unwire/configure/set-version silently acted
  // on admin's Univers.yaml and config dir instead of the universe on screen
  // (fixed 2026-09-18, sidecar-config-raw-edit).
  const configure = async (name: string) => {
    setBusy(name)
    try {
      const report = await mkt.configureItem({ name, universeHome })
      setReports((prev) => ({ ...prev, [name]: report }))
      pushToast({
        message: name + ' — ' + (report.summary ?? t('sidecars.checked')),
        color: report.healthy ? 'green' : 'yellow',
      })
    } catch (e) {
      fail(e)
    } finally {
      setBusy('')
    }
  }

  // Adding a capability is ONE action: install, then IMMEDIATELY drive its config
  // surface so any required setup surfaces now rather than waiting to be
  // discovered later (configure-at-add, not lazy — the locked user model in
  // universe-mgmt-model-and-design). The report renders inline exactly as a
  // manual Configure would.
  const install = async (r: Row) => {
    const ok = await op(
      r.name,
      () => { setCallPanel(null); return longMkt.install({ name: r.name, version: r.latest ?? '', marketplace: r.marketplace ?? '' }) },
      t('sidecars.installed', { name: r.name }),
    )
    if (!ok) return
    await refreshInstalled()
    await configure(r.name)
  }
  const uninstall = async (name: string) => {
    if (await op(name, () => mkt.uninstall({ name }), t('sidecars.uninstalled', { name }))) await refreshInstalled()
  }
  const wire = async (name: string) => {
    if (await op(name, () => mkt.wireIntoUniverse({ name, universeHome }), t('sidecars.wired', { name }))) await refreshInstalled()
  }
  // Moves the open universe onto the installed version: wiring again rewrites
  // its pin from the installed record, through the hub's marketplace service.
  const repinHere = async (r: Row) => {
    const version = r.installed?.version ?? ''
    if (await op(r.name, () => mkt.wireIntoUniverse({ name: r.name, universeHome }), t('sidecars.pinUpdated', { name: r.name, version, universe: displayedUniverse ?? '' }))) await refreshInstalled()
  }
  const unwire = async (name: string) => {
    if (await op(name, () => mkt.unwire({ name, universeHome }), t('sidecars.unwired', { name }))) await refreshInstalled()
  }
  const loadVersions = async (name: string) => {
    if (versions[name]) return
    try {
      const out = await mkt.itemVersions({ name })
      setVersions((prev) => ({ ...prev, [name]: out.versions ?? [] }))
    } catch (e) {
      fail(e)
    }
  }
  const setItemVersion = async (name: string, version: string) => {
    if (await op(name, () => { setCallPanel(null); return longMkt.setVersion({ name, version, universeHome }) }, name + ' -> ' + version)) await refreshInstalled()
  }


  // Merge catalog items ∪ installed rows into one unified list, by name.
  //
  // Control-plane binaries are EXCLUDED. install-infra records katyoti, ktw-hub,
  // ktw-gui and ktw-univers in the same installed registry (InfraOutdated reads
  // them to know what this machine runs), so they arrived here as ordinary rows —
  // badged SIDECAR, sourced from the infra CHANNEL under a "managed" label that
  // means "from a catalog", and carrying an uninstall button. The panel was
  // offering to delete the launcher and the window it is drawn in.
  //
  // They are not missing from the UI: the Infra tab owns
  // their entire lifecycle, and it is the only place they can be acted on
  // coherently, because infra moves as a SET.
  const rows = useMemo(() => {
    const byName: Record<string, Row> = {}
    for (const it of items) {
      byName[it.name] = { name: it.name, displayName: it.displayName, description: it.description, kind: it.kind, marketplace: it.marketplace, latest: it.latest }
    }
    for (const ins of installed) {
      if (ins.source === INFRA_SOURCE || ins.controlPlane) continue
      const r = byName[ins.name] ?? (byName[ins.name] = { name: ins.name })
      r.installed = ins
      if (!r.kind) r.kind = ins.kind
      if (!r.marketplace) r.marketplace = ins.marketplace
    }
    return Object.values(byName).sort((a, b) => a.name.localeCompare(b.name))
  }, [items, installed])

  // "In universe" must mean the universe ON SCREEN. `installed.wired` used to be
  // computed by the engine against "the current universe", which over KTSP is
  // always admin — so this badge and count described admin's membership while
  // the user had another universe open. The engine now reports wiredIn (every
  // registered universe that pins the item) and the answer is taken here, where
  // the open universe is actually known.
  const inOpenUniverse = useCallback(
    (r: Row) => !!displayedUniverse && !!r.installed?.wiredIn?.includes(displayedUniverse),
    [displayedUniverse],
  )

  // The version the open universe RUNS the item at, when it is another store
  // version than the installed one: its Univers.yaml pins a
  // store path, so the registry's version alone said « in universe ✓ » for a
  // universe still on an older - or deleted - version. Empty: in step, or a
  // program outside the store (a development build).
  const pinnedBehind = useCallback(
    (r: Row) => {
      if (isDevBuild(r)) return ''
      const pin = r.installed?.pins?.find((p) => p.universe === displayedUniverse)
      return pin?.version && r.installed?.version && pin.version !== r.installed.version ? pin.version : ''
    },
    [displayedUniverse],
  )

  const counts = useMemo(() => ({
    all: rows.length,
    installed: rows.filter((r) => r.installed).length,
    updatable: rows.filter(updatable).length,
    inuniverse: rows.filter(inOpenUniverse).length,
  }), [rows, inOpenUniverse, updatable])

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    return rows.filter((r) => {
      if (q && !`${r.name} ${r.displayName ?? ''} ${r.description ?? ''}`.toLowerCase().includes(q)) return false
      if (segment === 'installed') return !!r.installed
      if (segment === 'updatable') return updatable(r)
      if (segment === 'inuniverse') return inOpenUniverse(r)
      return true
    })
  }, [rows, query, segment, inOpenUniverse, updatable])

  return (
    <Box p="md" style={{ height: '100%', overflow: 'hidden', display: 'flex', flexDirection: 'column' }}>
      <Group justify="flex-end" gap="xs" mb="sm">
        {loading && <Loader size={14} />}
        <Button variant="light" size="xs" leftSection={<IconStethoscope size={14} />} loading={busy === '(doctor)'} onClick={() => void runDoctor()} data-guide="marketplace.doctor">
          {t('sidecars.doctor')}
        </Button>
        <Button size="xs" color="orange" disabled={outdated.length === 0} loading={busy === '(update)'} onClick={() => void updateAll()}>
          {t('sidecars.updateAll', { count: outdated.length })}
        </Button>
      </Group>

      {callPanel && (
        <Box mb="sm" data-guide="marketplace.call-panel">
          <DisplayPanel universeName={callPanel.universe} panelId={callPanel.panelId} variant="inline" title={callPanel.title} />
        </Box>
      )}

      {doctor && (
        <Alert color={doctor.every((d) => d.report.healthy) ? 'green' : 'yellow'} variant="light" mb="sm" withCloseButton onClose={() => setDoctor(null)} title={t('sidecars.doctorTitle')}>
          <Stack gap={2}>
            {doctor.length === 0 && <Text size="xs">{t('sidecars.doctorNone')}</Text>}
            {doctor.map((d) => (
              <Group key={d.name} gap={6} wrap="nowrap">
                <Badge size="xs" color={d.report.healthy ? 'green' : 'yellow'}>{d.report.healthy ? t('sidecars.healthy') : t('sidecars.needsSetup')}</Badge>
                <Text size="xs" fw={600}>{d.name}</Text>
                <Text size="xs" c="dimmed" truncate>{d.report.summary}</Text>
              </Group>
            ))}
          </Stack>
        </Alert>
      )}

      <TextInput
        placeholder={t('sidecars.search')}
        value={query}
        onChange={(e) => setQuery(e.currentTarget.value)}
        mb="xs"
      />
      <SegmentedControl
        fullWidth
        size="xs"
        value={segment}
        onChange={setSegment}
        mb="sm"
        data={[
          { label: t('sidecars.segAll', { n: counts.all }), value: 'all' },
          { label: t('sidecars.segInstalled', { n: counts.installed }), value: 'installed' },
          { label: t('sidecars.segUpdatable', { n: counts.updatable }), value: 'updatable' },
          { label: t('sidecars.segInUniverse', { n: counts.inuniverse }), value: 'inuniverse' },
        ]}
      />

      <ScrollArea style={{ flex: 1, minHeight: 0 }}>
        <Stack gap="xs">
          {filtered.length === 0 && (
            <Text c="dimmed" size="sm">
              {rows.length === 0 ? t('sidecars.none') : t('sidecars.noMatch')}
            </Text>
          )}
          {filtered.map((r) => {
            const inst = r.installed
            const managed = inst?.source === 'marketplace'
            const dev = isDevBuild(r)
            const canUpdate = updatable(r)
            const report = reports[r.name]
            return (
              <Box key={r.name} data-guide="marketplace.row" data-guide-name={r.name} p="xs" style={{ border: '1px solid var(--mantine-color-default-border)', borderRadius: 6 }}>
                <Group justify="space-between" wrap="nowrap" align="flex-start">
                  <Box style={{ minWidth: 0 }}>
                    <Group gap={6} wrap="nowrap">
                      <Text fw={600} size="sm" truncate>{r.displayName || r.name}</Text>
                      {r.kind && <Badge size="xs" color={kindColor(r.kind)}>{r.kind}</Badge>}
                      {inst && dev && (
                        <Tooltip label={t('sidecars.devBuildHint')}>
                          <Badge size="xs" variant="light" color="violet" data-guide="marketplace.dev-build">{t('sidecars.devBuild', { version: inst.devBuild?.version ?? '' })}</Badge>
                        </Tooltip>
                      )}
                      {inst && dev && managed && (
                        <Tooltip label={t('sidecars.storeKeptHint', { version: inst.version ?? '' })}>
                          <Badge size="xs" variant="outline" color="gray" data-guide="marketplace.store-kept">{t('sidecars.storeKept', { version: inst.version ?? '' })}</Badge>
                        </Tooltip>
                      )}
                      {inst && !dev && <Badge size="xs" variant="light" color={managed ? 'blue' : 'gray'}>{managed ? t('sidecars.managed') : t('sidecars.discovered')}</Badge>}
                      {inOpenUniverse(r) && !pinnedBehind(r) && <Badge size="xs" variant="light" color="teal">{t('sidecars.inUniverse')}</Badge>}
                      {pinnedBehind(r) && (
                        <Tooltip label={t('sidecars.pinBehindHint', { version: pinnedBehind(r), installed: inst?.version ?? '' })}>
                          <Badge size="xs" variant="light" color="orange" data-guide="marketplace.pin-behind">{t('sidecars.pinBehind', { version: pinnedBehind(r), universe: displayedUniverse ?? '' })}</Badge>
                        </Tooltip>
                      )}
                      {canUpdate && <Badge size="xs" variant="light" color="orange">{t('sidecars.updateAvailable')}</Badge>}
                    </Group>
                    <Text c="dimmed" size="xs" truncate>
                      {r.description || t('sidecars.noDescription')}{r.marketplace ? ` · ${r.marketplace}` : ''}
                    </Text>
                  </Box>
                  <Group gap={4} wrap="nowrap">
                    {/* version: menu for a managed install, else plain latest/version */}
                    {inst && dev && (managed || !!r.latest) ? (
                      // Another version replaces the dev build in the universe on
                      // screen: said so, and only with a universe open.
                      <Menu shadow="md" width={230} position="bottom-end" onOpen={() => void loadVersions(r.name)}>
                        <Menu.Target>
                          <Button variant="subtle" size="xs" px={6} disabled={noUniverse} rightSection={<IconChevronDown size={12} />} data-guide="marketplace.replace-dev">{inst.devBuild?.version || t('sidecars.dev')}</Button>
                        </Menu.Target>
                        <Menu.Dropdown>
                          <Menu.Label>{t('sidecars.replaceDev', { universe: displayedUniverse ?? '' })}</Menu.Label>
                          {!versions[r.name] && <Menu.Item disabled>{t('sidecars.loading')}</Menu.Item>}
                          {versions[r.name]?.length === 0 && <Menu.Item disabled>{t('sidecars.noVersions')}</Menu.Item>}
                          {(versions[r.name] ?? []).map((v, idx) => (
                            <Menu.Item
                              key={v}
                              rightSection={idx === 0 ? <Badge size="xs" variant="light" color="blue">{t('sidecars.latest')}</Badge> : undefined}
                              onClick={() => void setItemVersion(r.name, v)}
                            >{v}</Menu.Item>
                          ))}
                        </Menu.Dropdown>
                      </Menu>
                    ) : inst && dev ? (
                      <Text size="xs" c="dimmed">{inst.devBuild?.version ?? ''}</Text>
                    ) : inst && managed ? (
                      <Menu shadow="md" width={190} position="bottom-end" onOpen={() => void loadVersions(r.name)}>
                        <Menu.Target>
                          <Button variant="subtle" size="xs" px={6} rightSection={<IconChevronDown size={12} />}>{inst.version || '—'}</Button>
                        </Menu.Target>
                        <Menu.Dropdown>
                          <Menu.Label>{t('sidecars.setVersion')}</Menu.Label>
                          {!versions[r.name] && <Menu.Item disabled>{t('sidecars.loading')}</Menu.Item>}
                          {versions[r.name]?.length === 0 && <Menu.Item disabled>{t('sidecars.noVersions')}</Menu.Item>}
                          {(versions[r.name] ?? []).map((v, idx) => (
                            <Menu.Item
                              key={v}
                              leftSection={v === inst.version ? <IconCheck size={13} /> : <span style={{ display: 'inline-block', width: 13 }} />}
                              rightSection={idx === 0 ? <Badge size="xs" variant="light" color="blue">{t('sidecars.latest')}</Badge> : undefined}
                              onClick={() => void setItemVersion(r.name, v)}
                            >{v}</Menu.Item>
                          ))}
                        </Menu.Dropdown>
                      </Menu>
                    ) : (
                      <Text size="xs" c="dimmed">{inst?.version || r.latest || ''}</Text>
                    )}

                    {!inst && r.marketplace && (
                      <Button size="xs" leftSection={<IconDownload size={13} />} loading={busy === r.name} onClick={() => void install(r)} data-guide="marketplace.install">{t('sidecars.install')}</Button>
                    )}
                    {pinnedBehind(r) && (
                      <Button size="xs" color="orange" variant="light" loading={busy === r.name} onClick={() => void repinHere(r)} data-guide="marketplace.pin-update">{t('sidecars.pinUpdate', { universe: displayedUniverse ?? '' })}</Button>
                    )}
                    {canUpdate && (
                      <Button size="xs" color="orange" loading={busy === r.name} onClick={() => void setItemVersion(r.name, '')}>{t('sidecars.update')}</Button>
                    )}
                    {inst && (
                      <>
                        {/* Universe-targeting actions need a universe on screen; the
                            wire/unwire toggle reads the SAME answer as the badge
                            (wiredIn against the displayed universe), not the engine's
                            `wired`, which is computed against the admin hub's workspace. */}
                        <Tooltip label={noUniverse ? t('sidecars.openUniverseFirst') : t('sidecars.configureIn', { universe: displayedUniverse })}><ActionIcon variant="subtle" disabled={noUniverse} loading={busy === r.name} onClick={() => void configure(r.name)} aria-label={t('sidecars.configure')}><IconSettings size={15} /></ActionIcon></Tooltip>
                        {inOpenUniverse(r)
                          ? <Tooltip label={t('sidecars.unwireFrom', { universe: displayedUniverse })}><ActionIcon variant="subtle" color="orange" loading={busy === r.name} onClick={() => void unwire(r.name)} aria-label={t('sidecars.unwire')}><IconX size={15} /></ActionIcon></Tooltip>
                          : <Tooltip label={noUniverse ? t('sidecars.openUniverseFirst') : t('sidecars.wire', { universe: displayedUniverse })}><ActionIcon variant="subtle" color="teal" disabled={noUniverse} loading={busy === r.name} onClick={() => void wire(r.name)} aria-label={t('sidecars.wireLabel')} data-guide="marketplace.wire" data-guide-label={t('sidecars.wire', { universe: displayedUniverse ?? '' })}><IconCheck size={15} /></ActionIcon></Tooltip>}
                        {managed
                          ? <Tooltip label={t('sidecars.uninstall')}><ActionIcon variant="subtle" color="red" loading={busy === r.name} onClick={() => void uninstall(r.name)} aria-label={t('sidecars.uninstall')}><IconTrash size={15} /></ActionIcon></Tooltip>
                          : !dev && <Tooltip label={t('sidecars.devHint')}><Badge size="xs" variant="outline" color="gray">{t('sidecars.dev')}</Badge></Tooltip>}
                      </>
                    )}
                  </Group>
                </Group>
                {report && <ConfigReport report={report} editDisabled={noUniverse} onEdit={(file) => setEditing({ name: r.name, file })} />}
              </Box>
            )
          })}
        </Stack>
      </ScrollArea>

      <ConfigFileEditor
        target={editing}
        universeHome={universeHome}
        universeLabel={displayedUniverse ?? t('sidecars.controlPlaneLabel')}
        mkt={mkt}
        onClose={() => setEditing(null)}
        onSaved={(name, rep) => setReports((prev) => ({ ...prev, [name]: rep as ConfigCheckReport }))}
      />

    </Box>
  )
}

function ConfigReport({ report, editDisabled, onEdit }: { report: ConfigCheckReport; editDisabled: boolean; onEdit: (file: ConfigFileRef) => void }) {
  const t = useT('marketplace')
  const sevColor = (s?: string) => (s === 'error' ? 'red' : s === 'warn' ? 'yellow' : 'green')
  const files = report.config_files ?? []
  return (
    <Stack gap={2} mt={6} pl={4} style={{ borderLeft: '2px solid var(--mantine-color-default-border)' }}>
      {(report.items ?? []).map((it, idx) => (
        <Group key={idx} gap={6} wrap="nowrap">
          <Badge size="xs" color={sevColor(it.severity)}>{it.category}</Badge>
          <Text size="xs" fw={500}>{it.name}</Text>
          <Text size="xs" c="dimmed" truncate>{it.status}{it.hint ? ` — ${it.hint}` : ''}</Text>
        </Group>
      ))}
      {/* Each config file the sidecar declares opens in the raw YAML editor — the
          text floor under the schema-driven form to come. */}
      {files.length > 0 && (
        <Group gap={4} mt={2} wrap="wrap">
          {files.map((f) => (
            <Button
              key={f.name}
              variant="subtle"
              size="compact-xs"
              leftSection={<IconPencil size={12} />}
              disabled={editDisabled}
              onClick={() => onEdit(f)}
              aria-label={t('sidecars.editFile', { name: f.name })}
            >
              {f.name}
            </Button>
          ))}
        </Group>
      )}
    </Stack>
  )
}

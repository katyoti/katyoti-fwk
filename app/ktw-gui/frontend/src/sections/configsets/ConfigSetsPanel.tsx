// SPDX-License-Identifier: Apache-2.0
/**
 * ConfigSetsPanel — the config-sets offered by your marketplaces, and applying
 * one to the universe you are looking at.
 *
 * APPLY IS THE ONLY VERB, and that is a design decision rather than a missing
 * feature. A config-set drops templates into a universe and collects the keys
 * they need; nothing records which universe holds which set, so there is
 * nothing coherent to "update" or "remove" — an update would land in the store
 * and leave every universe on its old templates, and a delete could not know
 * which files were the set's and which the user has since edited.
 *
 * Re-applying IS the update path, and it is safe to repeat: templates already
 * present are reported rather than overwritten (Force is the explicit opt-out),
 * and keys keep their current values unless you change them.
 *
 * Two-phase by construction: a DRY RUN returns every key the set and the
 * universe's wired sidecars need, with which are already set, and writes
 * nothing; the form collects the rest and applies once. Every key the set
 * declares that is not optional must have a value: Apply stays disabled until
 * it does, and the service refuses it anyway. Keys can come from a sealed keys
 * file (`katyoti keys export`), imported into their scopes first. The
 * non-interactive path (`--set K=V`, `--keys`) is the same call, so this panel
 * is not a special case of the CLI.
 */

import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  Alert,
  Badge,
  Box,
  Button,
  Checkbox,
  FileButton,
  Group,
  Loader,
  PasswordInput,
  ScrollArea,
  Stack,
  Text,
  TextInput,
  Title,
  Tooltip,
} from '@mantine/core'
import { IconCheck, IconInfoCircle, IconKey, IconRefresh } from '@tabler/icons-react'
import { useAppStore } from '../../store'
import { useT } from '../../i18n'
import { marketplaceClient } from '../../ktsp'
import { fileToBase64 } from '../../lib/bytes'
import { VaultsSection } from '../marketplace/VaultsSection'
import { RestartForKeys } from '../marketplace/RestartForKeys'
import type { ApplyConfigSetOutput, ListConfigSetsOutput } from '../../ktsp/marketplace'

type ConfigSet = ListConfigSetsOutput[number]
type ApplyReport = ApplyConfigSetOutput
type KeyRow = NonNullable<ApplyReport['keys']>[number]

export function ConfigSetsPanel() {
  const t = useT('marketplace')
  const universe = useAppStore((s) => s.displayedUniverseName)
  const universes = useAppStore((s) => s.universes)
  // THE TARGET UNIVERSE'S HOME, not its name.
  //
  // `applyConfigSet` takes `universeHome`, and this panel never sent it: it
  // showed "Applying to <universe>" from a local variable that went nowhere.
  // An empty universeHome does not fail — the service falls back to its OWN
  // workspace, which for a hub-served call is the ADMIN universe. So apply
  // silently wrote templates and keys into the control plane, aggregated the
  // ADMIN universe's sidecar keys into the form, and left the universe the
  // user was looking at untouched. Nothing errored, because nothing was wrong
  // from the service's point of view.
  const universeHome = useMemo(
    () => universes.find((u) => u.name === universe)?.homePath ?? '',
    [universes, universe],
  )
  const mkt = useMemo(() => marketplaceClient(), [])

  const [sets, setSets] = useState<ConfigSet[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const [selected, setSelected] = useState<string | null>(null)
  const [report, setReport] = useState<ApplyReport | null>(null)
  const [values, setValues] = useState<Record<string, string>>({})
  const [force, setForce] = useState(false)
  const [busy, setBusy] = useState(false)
  const [done, setDone] = useState<string | null>(null)
  const [keysWritten, setKeysWritten] = useState(false)
  const [keysFile, setKeysFile] = useState<File | null>(null)
  const [keysPassword, setKeysPassword] = useState('')

  const load = useCallback(async () => {
    setLoading(true)
    try {
      setSets(await mkt.listConfigSets())
      setError(null)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setLoading(false)
    }
  }, [mkt])

  useEffect(() => {
    void load()
  }, [load])

  // PROBE: a dry run. The service does not prompt (it has no TTY and no user
  // over KTSP), so it reports what it needs — writing nothing — and we render
  // that.
  const probe = async (name: string) => {
    // REFUSE rather than let the service pick a default. An unresolved home is
    // the exact condition that used to send this at the admin universe.
    if (!universeHome) {
      setError(t('configsets.noHome'))
      return
    }
    setSelected(name)
    setReport(null)
    setDone(null)
    setValues({})
    setError(null)
    setBusy(true)
    try {
      setReport(await mkt.applyConfigSet({ name, universeHome, dryRun: true }))
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  const apply = async () => {
    if (!selected) return
    if (!universeHome) {
      setError(t('configsets.noHome'))
      return
    }
    setBusy(true)
    setError(null)
    try {
      const res = await mkt.applyConfigSet({ name: selected, values, force, universeHome })
      setReport(res)
      setDone(
        t('configsets.applied', { name: res.name }) +
          (res.templatesCopied?.length ? t('configsets.appliedTemplates', { count: res.templatesCopied.length }) : '') +
          (res.keysWritten?.length ? t('configsets.appliedKeys', { count: res.keysWritten.length }) : ''),
      )
      setValues({})
      if (res.keysWritten?.length) setKeysWritten(true)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  // Keys from a sealed file land in their scopes' .env; the dry run is then
  // re-read, so the form shows them as set.
  const importKeys = async () => {
    if (!selected || !keysFile || !universeHome) return
    setBusy(true)
    setError(null)
    try {
      const file = await fileToBase64(keysFile)
      const res = await mkt.importKeys({ universeHome, file, password: keysPassword })
      if (res.written?.length) setKeysWritten(true)
      const kept = res.keys.filter((k) => k.state === 'differs').map((k) => k.name)
      setDone(
        t('configsets.imported', { name: keysFile.name, count: res.written?.length ?? 0 }) +
          (kept.length ? t('configsets.importedKept', { keys: kept.join(', ') }) : ''),
      )
      setKeysFile(null)
      setKeysPassword('')
      setReport(await mkt.applyConfigSet({ name: selected, universeHome, dryRun: true }))
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  const keys: KeyRow[] = report?.keys ?? []
  const missing = new Set(report?.missing ?? [])
  // The set's non-optional keys still without a value, here or in the form.
  const blockingLeft = (report?.blocking ?? []).filter((k) => !(values[k] ?? '').trim())

  return (
    <Box p="md" style={{ height: '100%', overflow: 'hidden', display: 'flex', flexDirection: 'column' }}>
      <Group justify="space-between" mb="sm">
        <Box>
          <Title order={4}>{t('tabs.configsets')}</Title>
          <Text size="xs" c="dimmed">
            {/* The PATH, not just the name. This panel said "Applying to
              * myuniverse" while writing into the admin universe, and the
              * screen gave no way to notice. Showing where it actually lands
              * makes that class of mistake self-evident. */}
            {universe
              ? <>{t('configsets.applyingTo')} <b>{universe}</b>{universeHome ? <> — <code>{universeHome}</code></> : <> — <b>{t('configsets.homeUnknown')}</b></>}</>
              : t('configsets.noUniverse')}
          </Text>
        </Box>
        <Button variant="subtle" size="xs" leftSection={loading ? <Loader size={12} /> : <IconRefresh size={14} />} onClick={() => void load()}>
          {t('reload')}
        </Button>
      </Group>

      {error && <Alert color="red" variant="light" mb="sm"><Text size="xs">{error}</Text></Alert>}
      {done && <Alert color="green" variant="light" icon={<IconCheck size={16} />} mb="sm"><Text size="xs">{done}</Text></Alert>}
      {keysWritten && universe && <RestartForKeys universe={universe} onDone={() => setKeysWritten(false)} />}

      <ScrollArea style={{ flex: 1 }}>
        <Stack gap="xs">
          {sets.length === 0 && !loading && (
            <Alert color="gray" variant="light" icon={<IconInfoCircle size={16} />}>
              <Text size="xs">
                {t('configsets.none')}
                {' '}<code>{'ktw-compose build-config-set'}</code>.
              </Text>
            </Alert>
          )}

          {sets.map((cs) => (
            <Box key={`${cs.marketplace}/${cs.name}`} data-guide="configset.row" data-guide-name={cs.name} p="xs" style={{ border: '1px solid var(--mantine-color-default-border)', borderRadius: 6 }}>
              <Group justify="space-between" wrap="nowrap" align="flex-start">
                <Box style={{ minWidth: 0 }}>
                  <Group gap={6} wrap="nowrap">
                    <Text fw={600} size="sm" truncate>{cs.displayName || cs.name}</Text>
                    {cs.latest && <Badge size="xs" variant="light">{cs.latest}</Badge>}
                    <Badge size="xs" variant="outline" color="gray">{cs.marketplace}</Badge>
                  </Group>
                  {cs.description && <Text size="xs" c="dimmed" lineClamp={2}>{cs.description}</Text>}
                </Box>
                <Tooltip
                  multiline
                  w={300}
                  label={t('configsets.applyHint')}
                >
                  <Button
                    size="xs"
                    disabled={!universe}
                    loading={busy && selected === cs.name}
                    onClick={() => void probe(cs.name)}
                    data-guide="configset.apply"
                  >
                    {t('configsets.apply')}
                  </Button>
                </Tooltip>
              </Group>

              {selected === cs.name && report && (
                <Box mt="sm" pl="xs" style={{ borderLeft: '2px solid var(--mantine-color-default-border)' }}>
                  {report.templatesCopied?.length ? (
                    <Text size="xs" c="dimmed" mb={4}>{t('configsets.templates', { list: report.templatesCopied.join(', ') })}</Text>
                  ) : null}
                  {keys.length === 0 ? (
                    <Text size="xs" c="dimmed">{t('configsets.noKeys')}</Text>
                  ) : (
                    <Stack gap={6}>
                      {keys.map((k) => {
                        // `current` is REDACTED for secrets by the service — it says a
                        // value exists without sending it, because this crosses KTSP.
                        const placeholder = k.present ? (k.current || t('configsets.set')) : t('configsets.notSet')
                        const Field = k.secret ? PasswordInput : TextInput
                        return (
                          <Field
                            key={k.name}
                            size="xs"
                            label={
                              <Group gap={4}>
                                <Text size="xs" fw={600}>{k.name}</Text>
                                {missing.has(k.name) && !k.optional && <Badge size="xs" color="orange" variant="light">{t('configsets.needed')}</Badge>}
                                {k.optional && <Badge size="xs" color="gray" variant="light">{t('configsets.optional')}</Badge>}
                                {k.scope && <Badge size="xs" variant="outline" color="gray">{k.scope}</Badge>}
                              </Group>
                            }
                            description={k.description}
                            placeholder={placeholder}
                            value={values[k.name] ?? ''}
                            onChange={(e) => setValues((p) => ({ ...p, [k.name]: e.currentTarget.value }))}
                          />
                        )
                      })}
                    </Stack>
                  )}
                  <Group mt="sm" gap="xs" align="flex-end">
                    <FileButton onChange={setKeysFile} accept=".keys,.sealed,.yaml,application/octet-stream">
                      {(props) => (
                        <Button {...props} size="xs" variant="light" leftSection={<IconKey size={14} />}>
                          {keysFile ? keysFile.name : t('configsets.importKeys')}
                        </Button>
                      )}
                    </FileButton>
                    {keysFile && (
                      <>
                        <PasswordInput size="xs" placeholder={t('configsets.password')} value={keysPassword} onChange={(e) => setKeysPassword(e.currentTarget.value)} />
                        <Button size="xs" variant="light" loading={busy} disabled={!keysPassword} onClick={() => void importKeys()}>
                          {t('configsets.import')}
                        </Button>
                      </>
                    )}
                  </Group>
                  <Group mt="sm" gap="xs">
                    <Tooltip label={t('configsets.needsValue', { keys: blockingLeft.join(', ') })} disabled={blockingLeft.length === 0}>
                      <Button size="xs" loading={busy} disabled={blockingLeft.length > 0} onClick={() => void apply()}>
                        {t('configsets.applyTo', { universe })}
                      </Button>
                    </Tooltip>
                    <Tooltip label={t('configsets.forceHint')}>
                      <Checkbox size="xs" label={t('configsets.force')} checked={force} onChange={(e) => setForce(e.currentTarget.checked)} />
                    </Tooltip>
                    <Button size="xs" variant="subtle" onClick={() => { setSelected(null); setReport(null) }}>
                      {t('infra.cancel')}
                    </Button>
                  </Group>
                  <Text size="xs" c="dimmed" mt={6}>
                    {t('configsets.blankHint')}
                  </Text>
                </Box>
              )}
            </Box>
          ))}
          <VaultsSection />
        </Stack>
      </ScrollArea>
    </Box>
  )
}

// SPDX-License-Identifier: Apache-2.0
/**
 * SolutionInstallForm — stand up a whole universe from a marketplace solution:
 * pick it, open its vaults, fill ONE form of every key its config-sets need,
 * create + install on submit.
 *
 * A solution is a fixed version of a working installation — its sidecars at
 * their versions, the wiring, its config-sets, its seed, its vaults, its
 * documents. Installing it reproduces it exactly; the new machine supplies the
 * vaults' passwords and the keys no vault holds.
 *
 * A DRY RUN first (and again when a vault password changes): `installSolution`
 * with `dryRun` opens the vaults with the passwords given and reads every
 * config-set's keys against the named universe — which are set (a vault's
 * count), which are optional, which still block — installing and creating
 * NOTHING. Then ONE mutating call, which the service refuses while a vault does
 * not open or a key that is not optional has no value.
 */

import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  Alert,
  Badge,
  Box,
  Button,
  FileInput,
  Group,
  Loader,
  PasswordInput,
  Select,
  Stack,
  Text,
  TextInput,
} from '@mantine/core'
import { IconAlertTriangle, IconCheck, IconInfoCircle } from '@tabler/icons-react'
import { marketplaceClient } from '../../ktsp'
import { fileToBase64 } from '../../lib/bytes'
import type { InstallSolutionOutput, SearchItemsOutput } from '../../ktsp/marketplace'
import { useT } from '../../i18n'
import { installClient, type CallPanel } from '../../ktsp/inPanel'
import { DisplayPanel } from '../panels/DisplayPanel'

type Solution = SearchItemsOutput[number]

// A solution is told apart by its marketplace and its name.
export const solutionKey = (s: Solution) => `${s.marketplace}/${s.name}`
const solutionRef = (s: Solution) => ({ marketplace: s.marketplace, name: s.name })
type KeyRow = NonNullable<NonNullable<InstallSolutionOutput['configSets']>[number]['keys']>[number]
type VaultRow = NonNullable<InstallSolutionOutput['vaults']>[number]

interface Props {
  /** Called with the created universe's name once the bootstrap succeeds. */
  onCreated: (name: string) => void
  onCancel: () => void
}

export function SolutionInstallForm({ onCreated, onCancel }: Props) {
  const t = useT('universes')
  const mkt = useMemo(() => marketplaceClient(), [])
  // The install (not its dry run) is drawn in the panel the GUI
  // lends its call - its steps, Cancel, its end.
  const [callPanel, setCallPanel] = useState<CallPanel | null>(null)
  const installMkt = useMemo(() => installClient(setCallPanel, () => t('solution.createApply')), [t])

  const [solutions, setSolutions] = useState<Solution[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const [picked, setPicked] = useState<Solution | null>(null)
  // The universe the solution goes into: always named — a solution that brings
  // config-sets, vaults or a seed is refused without one. Defaults to the solution's
  // name; a registered name is that universe, a new one is created.
  const [universeName, setUniverseName] = useState('')
  const [keys, setKeys] = useState<KeyRow[]>([])
  const [keysLoading, setKeysLoading] = useState(false)
  const [configSetName, setConfigSetName] = useState<string | null>(null)
  const [vaults, setVaults] = useState<VaultRow[]>([])
  const [vaultPasswords, setVaultPasswords] = useState<Record<string, string>>({})
  const [values, setValues] = useState<Record<string, string>>({})
  const [submitting, setSubmitting] = useState(false)
  const [missing, setMissing] = useState<string[]>([])
  const [blocking, setBlocking] = useState<string[]>([])
  const [keysFile, setKeysFile] = useState<File | null>(null)
  const [keysPassword, setKeysPassword] = useState('')

  // An empty query lists everything; the KIND filter is what matters here.
  useEffect(() => {
    mkt
      .searchItems({ query: '' })
      .then((items) => setSolutions(items.filter((i) => i.kind === 'solution')))
      .catch((err) => setError(err instanceof Error ? err.message : String(err)))
      .finally(() => setLoading(false))
  }, [mkt])

  // Read the keys with a dry run — see the file comment.
  const loadKeys = useCallback(
    async (s: Solution, universe: string, passwords: Record<string, string> = {}) => {
      if (nameError(universe)) return
      setKeysLoading(true)
      setKeys([])
      setBlocking([])
      setConfigSetName(null)
      setError(null)
      try {
        const dry = await mkt.installSolution({ ref: solutionRef(s), universeHome: universe.trim(), vaultPasswords: passwords, dryRun: true })
        setVaults(dry.vaults ?? [])
        // Legitimate: a solution need not pin a config-set — nothing to collect.
        const sets = dry.configSets ?? []
        if (sets.length === 0) return
        setConfigSetName(sets.map((c) => c.name).join(', '))
        // One form for every config-set: each key once (the first set's
        // description wins), blocking if any set blocks on it.
        const byName = new Map<string, KeyRow>()
        for (const c of sets) for (const k of c.keys ?? []) if (!byName.has(k.name)) byName.set(k.name, k)
        setKeys([...byName.values()])
        setBlocking([...new Set(sets.flatMap((c) => c.blocking ?? []))])
      } catch (err) {
        setError(err instanceof Error ? err.message : String(err))
      } finally {
        setKeysLoading(false)
      }
    },
    [mkt],
  )

  const submit = async () => {
    if (!picked) return
    setSubmitting(true)
    setError(null)
    setMissing([])
    try {
      // The universe is NAMED (a bare name: the service resolves a registered
      // universe, or creates one under the global universes root). Never empty:
      // this call is served by the admin hub, and an empty home once made it
      // rewrite ADMIN's own Univers.yaml.
      const name = universeName.trim()
      const keys = keysFile ? await fileToBase64(keysFile) : undefined
      setCallPanel(null)
      const out = await installMkt.installSolution({
        ref: solutionRef(picked),
        universeHome: name,
        values,
        keys,
        keysPassword: keysFile ? keysPassword : undefined,
        vaultPasswords,
      })
      // `missing` is not a failure: the universe exists and is wired. It names
      // what still has no value, which the Config-sets tab can finish.
      setMissing((out.configSets ?? []).flatMap((c) => c.missing ?? []))
      onCreated(name)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setSubmitting(false)
    }
  }

  // The config-set's non-optional keys still without a value. A keys file may
  // bring them: the service, which opens it, has the last word.
  const blockingLeft = blocking.filter((k) => !(values[k] ?? '').trim())

  if (loading) return <Group justify="center" p="lg"><Loader size="sm" /></Group>

  if (solutions.length === 0) {
    return (
      <Alert color="gray" variant="light" icon={<IconInfoCircle size={16} />}>
        <Text size="xs">
          {t('solution.noSolutions')} <code>{'ktw-compose snapshot-universe'}</code>{t('solution.noSolutionsAfter')} {/* i18n-ignore */}
        </Text>
      </Alert>
    )
  }

  return (
    <Stack gap="md">
      {error && (
        <Alert color="red" variant="light" icon={<IconAlertTriangle size={16} />}>
          <Text size="xs">{error}</Text>
        </Alert>
      )}

      <Select
        label={t('solution.label')}
        description={t('solution.description')}
        placeholder={t('solution.placeholder')}
        // One option per marketplace and solution: two marketplaces may publish
        // a solution of the same name.
        data={solutions.map((r) => ({
          value: solutionKey(r),
          label: `${r.displayName || r.name}${r.latest ? ` (${r.latest})` : ''} — ${r.marketplace}`,
        }))}
        value={picked ? solutionKey(picked) : null}
        onChange={(v) => {
          const s = solutions.find((r) => solutionKey(r) === v) ?? null
          setPicked(s)
          if (s) {
            const universe = universeName.trim() === '' || universeName === picked?.name ? s.name : universeName
            setUniverseName(universe)
            setValues({})
            setVaultPasswords({})
            void loadKeys(s, universe)
          }
        }}
      />

      {picked && (
        <TextInput
          size="xs"
          label={t('solution.universeName')}
          description={t('solution.universeNameHint')}
          value={universeName}
          error={nameError(universeName) ? t('solution.universeNameInvalid') : undefined}
          onChange={(e) => setUniverseName(e.currentTarget.value)}
          // The keys already set depend on the universe: re-read them.
          onBlur={() => picked && void loadKeys(picked, universeName, vaultPasswords)}
        />
      )}

      {picked && vaults.length > 0 && (
        <Stack gap={6}>
          <Text size="xs" fw={600}>{t('solution.vaults')}</Text>
          {vaults.map((v) => (
            <PasswordInput
              key={v.name}
              size="xs"
              label={
                <Group gap={4}>
                  <Text size="xs" fw={600}>{v.name}</Text>
                  <Badge size="xs" variant="outline" color="gray">{v.version}</Badge>
                  {v.opened
                    ? <Badge size="xs" variant="light" color="green">{t('solution.vaultOpened', { count: v.keys?.length ?? 0 })}</Badge>
                    : <Badge size="xs" variant="light" color="orange">{t('solution.vaultClosed')}</Badge>}
                </Group>
              }
              value={vaultPasswords[v.name] ?? ''}
              onChange={(e) => setVaultPasswords((p) => ({ ...p, [v.name]: e.currentTarget.value }))}
              // The vault's keys count as given: read the keys again with it.
              onBlur={() => picked && void loadKeys(picked, universeName, vaultPasswords)}
            />
          ))}
        </Stack>
      )}

      {picked && configSetName && (
        <Group gap="xs" align="flex-end" grow>
          <FileInput
            size="xs"
            clearable
            label={t('solution.keysFile')}
            description={t('solution.keysFileHint')}
            value={keysFile}
            onChange={setKeysFile}
          />
          {keysFile && (
            <PasswordInput
              size="xs"
              label={t('solution.keysPassword')}
              value={keysPassword}
              onChange={(e) => setKeysPassword(e.currentTarget.value)}
            />
          )}
        </Group>
      )}

      {keysLoading && <Group gap="xs"><Loader size={12} /><Text size="xs" c="dimmed">{t('solution.fetchingKeys')}</Text></Group>}

      {picked && !keysLoading && (
        <Box>
          {keys.length === 0 ? (
            <Text size="xs" c="dimmed">
              {configSetName
                ? t('solution.noKeys', { name: configSetName })
                : t('solution.noConfigSet')}
            </Text>
          ) : (
            <Stack gap={6}>
              <Text size="xs" fw={600}>
                {t('solution.keysNeeded', { name: configSetName })}
              </Text>
              {keys.map((k) => {
                const Field = k.secret ? PasswordInput : TextInput
                return (
                  <Field
                    key={k.name}
                    size="xs"
                    label={
                      <Group gap={4}>
                        <Text size="xs" fw={600}>{k.name}</Text>
                        {k.scope && <Badge size="xs" variant="outline" color="gray">{k.scope}</Badge>}
                        {k.secret && <Badge size="xs" variant="light" color="orange">{t('solution.secret')}</Badge>}
                        {k.optional && <Badge size="xs" variant="light" color="gray">{t('solution.optional')}</Badge>}
                      </Group>
                    }
                    description={k.description}
                    placeholder={k.present ? (k.current || 'set') : undefined}
                    value={values[k.name] ?? ''}
                    onChange={(e) => setValues((p) => ({ ...p, [k.name]: e.currentTarget.value }))}
                  />
                )
              })}
              <Text size="xs" c="dimmed">
                {t('solution.blankAllowed')}
              </Text>
            </Stack>
          )}
        </Box>
      )}

      {missing.length > 0 && (
        <Alert color="yellow" variant="light" icon={<IconCheck size={16} />}>
          <Text size="xs">{t('solution.stillMissing', { keys: missing.join(', ') })}</Text>
        </Alert>
      )}

      {callPanel && (
        <Box data-guide="solution.install-panel">
          <DisplayPanel universeName={callPanel.universe} panelId={callPanel.panelId} variant="inline" title={callPanel.title} />
        </Box>
      )}

      <Group justify="flex-end" gap="xs">
        <Button variant="subtle" size="xs" onClick={onCancel} disabled={submitting}>
          {t('common.cancel')}
        </Button>
        <Button
          size="xs"
          disabled={!picked || nameError(universeName) || keysLoading || vaults.some((v) => !v.opened) || (blockingLeft.length > 0 && !keysFile) || (!!keysFile && !keysPassword)}
          title={blockingLeft.length > 0 && !keysFile ? t('solution.needsValue', { keys: blockingLeft.join(', ') }) : undefined}
          loading={submitting}
          onClick={() => void submit()}
        >
          {t('solution.createApply')}
        </Button>
      </Group>
    </Stack>
  )
}

// nameError: a universe name is one word — letters, digits, dashes,
// underscores (a folder would be taken as a path by the service).
function nameError(name: string): boolean {
  return !/^[A-Za-z0-9][A-Za-z0-9_-]*$/.test(name.trim())
}

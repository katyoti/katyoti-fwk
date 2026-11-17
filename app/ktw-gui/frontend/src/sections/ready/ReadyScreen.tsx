// SPDX-License-Identifier: Apache-2.0
/**
 * ReadyScreen — « Prêt à démarrer »: what the universe on screen
 * still needs before it is fully operational, made for a first-time person:
 * one tab per step, each green or red in its title, the screen
 * opening on the first red one.
 *
 *   1. Profil et premiers pas — who the person is (the profile, edited in the
 *      Profile modal), the installation's projects folder (kept as proposed
 *      or picked here, recorded through the hub's SetProjectsFolder; not
 *      asked on a development build), the project folder where the first
 *      study is created (the universe's attached project; changed on the
 *      universe's page), and the universe's first guide.
 *   2. Programmes — the plan of the programs the sidecars need on this machine
 *      (PlanPrereqs): « Tout installer » the required ones, each alone,
 *      optional ones offered unselected, the progress and the administrator
 *      note; the sidecars' config check (Doctor) problems no program or key
 *      answers stay below, as technical details.
 *   3. Clés — first « Importer les clés de l'équipe »: the solution's sealed
 *      key sets with the team password ONCE (ImportKeySets), or a keys file the
 *      team gave (ImportKeys); then only the keys still missing in ONE form,
 *      saved through ApplyConfigSet.
 *
 * A tab whose part could not be read is red « Non vérifié », never green.
 * A pure client of hub-owned services over KTSP; generic —
 * no sidecar, program or key is named here. The decisions live in readiness.ts.
 */

import { useEffect, useMemo, useRef, useState } from 'react'
import {
  Accordion,
  Alert,
  Anchor,
  Badge,
  Box,
  Button,
  Checkbox,
  Code,
  CopyButton,
  Group,
  Loader,
  Modal,
  Paper,
  PasswordInput,
  Progress,
  ScrollArea,
  Stack,
  Tabs,
  Text,
  TextInput,
  ThemeIcon,
  Title,
  Tooltip,
} from '@mantine/core'
import {
  IconAlertTriangle,
  IconBook,
  IconCheck,
  IconCopy,
  IconCircleCheck,
  IconDownload,
  IconFile,
  IconFolder,
  IconHelpCircle,
  IconKey,
  IconFileAlert,
  IconPackage,
  IconPlugConnected,
  IconRefresh,
  IconShieldLock,
  IconUser,
} from '@tabler/icons-react'
import { KtspError, marketplaceClient } from '../../ktsp'
import { installClient, type CallPanel } from '../../ktsp/inPanel'
import { DisplayPanel } from '../panels/DisplayPanel'
import type { InstallPrereqsOutput, PlanPrereqsOutput } from '../../ktsp/marketplace'
import { useAppStore } from '../../store'
import { useProfileStore } from '../../store/profileStore'
import { useGuideLauncher } from '../../hooks/useGuideLauncher'
import { PickKeysFile, PickProjectsFolder } from '../../../wailsjs/go/main/App'
import { useT, type TFn } from '../../i18n'
import { RestartForKeys } from '../marketplace/RestartForKeys'
import { ProfileForm } from '../profile/ProfileForm'
import { EarlierProjectsSection } from './EarlierProjectsSection'
import { EarlierInstallationsSection } from './EarlierInstallationsSection'
import {
  elevationAmong,
  errorReport,
  firstTab,
  initialSelection,
  offeredToInstall,
  keyId,
  keysBlocks,
  keysStep,
  orderedPrograms,
  lookedForAfterInstall,
  recheckedOf,
  profileDone,
  profileNamed,
  scopeBadge,
  shownInstalled,
  projectsFolderAsked,
  projectsFolderOk,
  programsToInstall,
  saveCalls,
  sidecarOk,
  sidecarProblems,
  stepOf,
  stepsWithoutCard,
  summarize,
  headerState,
  tabState,
  technicalProblems,
  TABS,
  type InstallStep,
  type Program,
  type ReadyInputs,
  type RemainingKey,
  type SidecarCheck,
  type TabKey,
  type TabState,
} from './readiness'
import { keySetsImported, markKeySetsImported, universeTarget, useReadyInputs, useReadyStore } from './readyStore'

const errText = (e: unknown) => (e instanceof Error ? e.message : String(e))

const TAB_ICON: Record<TabKey, React.ReactNode> = {
  profile: <IconUser size={16} />,
  programs: <IconPackage size={16} />,
  keys: <IconKey size={16} />,
}

/** `section` — the tab an address names (`screen/ready/keys`): the screen
 *  opens on it, as if the person had chosen it. */
export function ReadyScreen({ universe, section }: { universe: string; section?: string }) {
  const t = useT('ready')
  const inputs = useReadyInputs(universe)
  const check = useReadyStore((s) => s.check)
  const [keysWritten, setKeysWritten] = useState(false)
  const recheck = () => void check(universe)

  // Open on the first red tab once a check is read; a tab the person chose
  // stays chosen across the checks that follow.
  const named = (TABS as readonly string[]).includes(section ?? '') ? (section as TabKey) : null
  const [tab, setTab] = useState<TabKey | null>(named)
  const chosen = useRef(!!named)
  useEffect(() => { chosen.current = !!named; setTab(named) }, [universe, named])
  const read = !inputs.loading && !!(inputs.keys || inputs.plan || inputs.profile || Object.keys(inputs.errors).length)
  useEffect(() => {
    if (read && !chosen.current) setTab(firstTab(inputs))
  }, [read, inputs])
  const active = tab ?? firstTab(inputs)

  return (
    <ScrollArea h="100%" type="auto">
      <Box p="md" maw={1100} data-guide="ready.screen" data-element="screen:ready">
        <Header universe={universe} header={headerState(inputs)} loading={inputs.loading} onRefresh={recheck} t={t} />
        {keysWritten && <RestartForKeys universe={universe} onDone={() => { setKeysWritten(false); recheck() }} />}
        <Tabs mt="md" value={active} onChange={(v) => { if (v) { chosen.current = true; setTab(v as TabKey) } }}>
          <Tabs.List>
            {TABS.map((k) => {
              const state = tabState(inputs, k)
              return (
                <Tabs.Tab key={k} value={k} leftSection={TAB_ICON[k]} rightSection={<StateMark state={state} t={t} />}
                  data-element={`tab:ready/${k}`} data-state={state}>
                  {t(`tabs.${k}`)}
                </Tabs.Tab>
              )
            })}
          </Tabs.List>
          <Tabs.Panel value="profile" pt="md">
            <ProfileTab universe={universe} inputs={inputs} onDone={recheck} t={t} />
          </Tabs.Panel>
          <Tabs.Panel value="programs" pt="md">
            <ProgramsTab universe={universe} inputs={inputs} onDone={recheck} t={t} />
          </Tabs.Panel>
          <Tabs.Panel value="keys" pt="md">
            <KeysSection universe={universe} inputs={inputs} onWritten={() => { setKeysWritten(true); recheck() }} t={t} />
          </Tabs.Panel>
        </Tabs>
      </Box>
    </ScrollArea>
  )
}

/** A tab's state in its title: green when done, red when something is left,
 *  red « Non vérifié » when its part could not be read, a loader meanwhile. */
function StateMark({ state, t }: { state: TabState; t: TFn }) {
  if (state === 'checking') return <Loader size={12} color="gray" />
  if (state === 'ok') return <ThemeIcon size={16} radius="xl" color="teal" variant="filled"><IconCheck size={11} /></ThemeIcon>
  return (
    <Badge size="xs" color="red" variant={state === 'unverified' ? 'outline' : 'filled'}>
      {state === 'unverified' ? t('tabState.unverified') : t('tabState.todo')}
    </Badge>
  )
}

function Header({ universe, header, loading, onRefresh, t }: {
  universe: string; header: ReturnType<typeof headerState>; loading: boolean; onRefresh: () => void; t: TFn
}) {
  const look = {
    checking: { color: 'gray', icon: <Loader size={22} color="gray" />, text: t('state.checking', { universe }) },
    ready: { color: 'teal', icon: <IconCircleCheck size={26} />, text: t('state.ready', { universe }) },
    'not-ready': { color: 'orange', icon: <IconAlertTriangle size={26} />, text: t('state.notReady', { universe }) },
    unknown: { color: 'gray', icon: <IconHelpCircle size={26} />, text: t('state.unknown', { universe }) },
    'tabs-left': { color: 'orange', icon: <IconAlertTriangle size={26} />, text: t('state.tabsLeft', { universe, tabs: header.left.map((k) => `« ${t(`tabs.${k}`)} »`).join(', ') }) },
  }[header.state]
  return (
    <Group justify="space-between" wrap="nowrap" align="flex-start">
      <Group gap="md" wrap="nowrap">
        <ThemeIcon size={48} radius="xl" variant="light" color={look.color}>{look.icon}</ThemeIcon>
        <Box>
          <Title order={3}>{t('title')}</Title>
          <Text size="sm" c="dimmed">{look.text}</Text>
        </Box>
      </Group>
      <Button variant="light" size="xs" leftSection={<IconRefresh size={14} />} loading={loading} onClick={onRefresh}>
        {t('refresh')}
      </Button>
    </Group>
  )
}

/** What a tab asks of the person, in one plain sentence, above the rest. */
function Intro({ text }: { text: string }) {
  return <Text size="sm">{text}</Text>
}

// ── 1. Profil et premiers pas ────────────────────────────────────────────

function ProfileTab({ universe, inputs, onDone, t }: { universe: string; inputs: ReadyInputs; onDone: () => void; t: TFn }) {
  const modalOpen = useProfileStore((s) => s.modalOpen)
  const setMainPaneRoute = useAppStore((s) => s.setMainPaneRoute)
  const firstGuide = useAppStore((s) => s.universes.find((u) => u.name === universe)?.firstGuide)
  const { launchGuide } = useGuideLauncher()
  // The profile is asked here, in place; the profile menu's view
  // is a way back to it - when it closes, read the profile again.
  const wasOpen = useRef(false)
  useEffect(() => {
    if (wasOpen.current && !modalOpen) onDone()
    wasOpen.current = modalOpen
  }, [modalOpen, onDone])
  const [editing, setEditing] = useState(false)

  const p = inputs.profile
  const named = profileNamed(p)
  const projects = inputs.projects ?? []
  return (
    <Stack gap="md">
      <Intro text={t('profile.intro')} />
      <Section icon={<IconUser size={14} />} title={t('profile.whoTitle')} hint={t('profile.whoHint')}>
        {inputs.errors.profile && <Alert color="red" variant="light" mb="xs">{t('profile.error', { error: inputs.errors.profile })}</Alert>}
        {named && p && !editing ? (
          <Group gap="xs">
            <ThemeIcon size="sm" radius="xl" color="teal" variant="light"><IconCheck size={12} /></ThemeIcon>
            <Text size="sm" fw={600}>{[p.firstName, p.lastName].filter(Boolean).join(' ')}</Text>
            <Text size="sm" c="dimmed">{p.email}</Text>
            <Badge size="xs" variant="outline" color="gray">{p.id}</Badge>
            <Button size="compact-xs" variant="subtle" onClick={() => setEditing(true)} data-element="action:ready/profile">{t('profile.edit')}</Button>
          </Group>
        ) : p ? (
          <Box data-element="section:ready/profile-form" maw={520}>
            <ProfileForm
              onSaved={() => { setEditing(false); onDone() }}
              onCancel={named ? () => setEditing(false) : undefined}
              saveElement="action:ready/profile-save"
              fieldElement="field:ready/profile"
            />
          </Box>
        ) : !inputs.errors.profile && <Loader size="xs" />}
      </Section>
      {projectsFolderAsked(inputs) && <ProjectsFolderSection inputs={inputs} onDone={onDone} t={t} />}
      <EarlierProjectsSection t={t} />
      <EarlierInstallationsSection t={t} />
      <Section icon={<IconFolder size={14} />} title={t('profile.folderTitle')} hint={t('profile.folderHint')}>
        {inputs.errors.projects && <Alert color="red" variant="light" mb="xs">{t('profile.folderError', { error: inputs.errors.projects })}</Alert>}
        {inputs.projects ? (
          <Group gap="xs" align="center">
            {projects.length
              ? projects.map((d) => <Code key={d}>{d}</Code>)
              : <Text size="sm" c="red">{t('profile.noFolder')}</Text>}
            <Button size="compact-xs" variant="subtle" onClick={() => setMainPaneRoute({ kind: 'edit', name: universe })}>
              {projects.length ? t('profile.changeFolder') : t('profile.chooseFolder')}
            </Button>
          </Group>
        ) : !inputs.errors.projects && <Loader size="xs" />}
      </Section>
      {firstGuide && (
        <Section icon={<IconBook size={14} />} title={t('profile.guideTitle')} hint={t('profile.guideHint')}>
          <Button size="xs" variant="light" leftSection={<IconBook size={14} />} onClick={() => launchGuide()}>{t('profile.guide')}</Button>
        </Section>
      )}
      {profileDone(inputs) && <Text size="sm" c="teal">{t('profile.done')}</Text>}
    </Stack>
  )
}

/** The installation's projects folder: red while it is only
 *  proposed, green once chosen. The person keeps the proposed one or picks
 *  another; the hub refuses a folder inside the installation, and its reason
 *  shows here. */
function ProjectsFolderSection({ inputs, onDone, t }: { inputs: ReadyInputs; onDone: () => void; t: TFn }) {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const f = inputs.projectsFolder
  const choose = async (folder: string) => {
    setBusy(true)
    setError(null)
    try {
      await marketplaceClient().setProjectsFolder({ folder })
      onDone()
    } catch (e) {
      // The hub's reason alone, without the slot it came through.
      setError(e instanceof KtspError ? e.message.slice(e.slotPath.length + 2) : e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }
  const pick = async () => {
    const r = await PickProjectsFolder()
    if (r.error) setError(r.error)
    else if (r.path) await choose(r.path)
  }
  return (
    <Section icon={<IconFolder size={14} />} title={t('profile.projectsFolderTitle')} hint={t('profile.projectsFolderHint')}>
      <Box data-element="section:ready/projects-folder" data-state={projectsFolderOk(inputs) ? 'ok' : 'todo'}>
        {inputs.errors.projectsFolder && <Alert color="red" variant="light" mb="xs">{t('profile.projectsFolderError', { error: inputs.errors.projectsFolder })}</Alert>}
        {error && <Alert color="red" variant="light" mb="xs">{error}</Alert>}
        {f ? (
          <Group gap="xs" align="center">
            {f.chosen && <ThemeIcon size="sm" radius="xl" color="teal" variant="light"><IconCheck size={12} /></ThemeIcon>}
            <Code>{f.folder}</Code>
            {!f.chosen && <Text size="sm" c="red">{t('profile.projectsFolderProposed')}</Text>}
            {!f.chosen && (
              <Button size="compact-xs" loading={busy} onClick={() => void choose(f.proposed || f.folder)} data-element="action:ready/projects-folder-keep">
                {t('profile.projectsFolderKeep')}
              </Button>
            )}
            <Button size="compact-xs" variant="subtle" disabled={busy} onClick={() => void pick()} data-element="action:ready/projects-folder-pick">
              {t('profile.projectsFolderPick')}
            </Button>
          </Group>
        ) : !inputs.errors.projectsFolder && <Loader size="xs" />}
      </Box>
    </Section>
  )
}

// ── 2. Programmes ────────────────────────────────────────────────────────

function ProgramsTab({ universe, inputs, onDone, t }: { universe: string; inputs: ReadyInputs; onDone: () => void; t: TFn }) {
  const technical = technicalProblems(inputs)
  return (
    <Stack gap="md">
      <Intro text={t('programs.intro')} />
      <ProgramsSection universe={universe} homes={inputs.keys?.universes} plan={inputs.plan} error={inputs.errors.plan} onDone={onDone} t={t} />
      <Accordion variant="contained" defaultValue={technical.length || inputs.errors.doctor ? 'sidecars' : null}>
        <Accordion.Item value="sidecars">
          <Accordion.Control icon={<IconPlugConnected size={16} />}>
            <Group gap="xs">
              <Text size="sm" fw={600}>{t('sidecars.title')}</Text>
              {technical.length > 0 && <Badge size="xs" color="red">{t('sidecars.problems', { count: technical.length })}</Badge>}
              {inputs.errors.doctor && <Badge size="xs" color="red" variant="outline">{t('tabState.unverified')}</Badge>}
            </Group>
          </Accordion.Control>
          <Accordion.Panel>
            <Text size="xs" c="dimmed" mb="sm">{t('sidecars.hint')}</Text>
            <SidecarsList checks={inputs.doctor} error={inputs.errors.doctor} multi={(inputs.keys?.universes?.length ?? 0) > 1} t={t} />
          </Accordion.Panel>
        </Accordion.Item>
      </Accordion>
    </Stack>
  )
}

function Section({ icon, title, hint, children }: { icon: React.ReactNode; title: string; hint: string; children: React.ReactNode }) {
  return (
    <Paper withBorder radius="md" p="md">
      <Group gap="xs" mb={4}>
        <ThemeIcon variant="light" size="sm" radius="sm">{icon}</ThemeIcon>
        <Title order={5}>{title}</Title>
      </Group>
      <Text size="xs" c="dimmed" mb="sm">{hint}</Text>
      {children}
    </Paper>
  )
}

// ── The sidecars' config check, as technical details ─────────────────────

function SidecarsList({ checks, error, multi, t }: { checks?: SidecarCheck[]; error?: string; multi: boolean; t: TFn }) {
  return (
    <>
      {error && <Alert color="red" variant="light" mb="xs">{t('sidecars.error', { error })}</Alert>}
      {checks && checks.length === 0 && <Text size="sm" c="dimmed">{t('sidecars.none')}</Text>}
      <Stack gap={6}>
        {(checks ?? []).map((c) => {
          const ok = sidecarOk(c)
          const problems = sidecarProblems(c)
          return (
            <Paper key={`${c.universeHome}|${c.name}`} p="xs" radius="sm" bg={ok ? undefined : 'var(--mantine-color-orange-light)'} data-guide="ready.sidecar" data-guide-name={c.name}>
              <Group gap="xs" wrap="nowrap">
                <ThemeIcon size="sm" radius="xl" color={ok ? 'teal' : 'orange'} variant={ok ? 'light' : 'filled'}>
                  {ok ? <IconCheck size={12} /> : <IconAlertTriangle size={12} />}
                </ThemeIcon>
                <Text size="sm" fw={600}>{c.name}</Text>
                <Badge size="xs" variant="light" color={ok ? 'teal' : 'orange'}>{ok ? t('sidecars.ok') : t('sidecars.ko')}</Badge>
                {multi && c.universeHome && <Badge size="xs" variant="outline" color="gray">{baseName(c.universeHome)}</Badge>}
                <Text size="xs" c="dimmed" truncate style={{ flex: 1 }}>{c.report.summary}</Text>
              </Group>
              {problems.length > 0 && (
                <Stack gap={2} mt={4} ml={30}>
                  {problems.map((p) => (
                    <Text key={`${p.category}|${p.name}`} size="xs">
                      <Text span fw={600}>{p.name}</Text>
                      <Text span c="dimmed">{' · '}{p.detail || p.status}</Text>
                      {p.hint && <Text span c="dimmed" ff="monospace">{' — '}{p.hint}</Text>}
                    </Text>
                  ))}
                </Stack>
              )}
            </Paper>
          )
        })}
      </Stack>
    </>
  )
}

const baseName = (p: string) => p.replace(/[\\/]+$/, '').split(/[\\/]/).pop() ?? p

// ── The programs ─────────────────────────────────────────────────────────

function ProgramsSection({ universe, homes, plan, error, onDone, t }: {
  universe: string; homes?: string[]; plan?: PlanPrereqsOutput; error?: string; onDone: () => void; t: TFn
}) {
  const pushToast = useAppStore((s) => s.pushToast)
  const [selected, setSelected] = useState<Set<string>>(() => initialSelection(plan))
  const [running, setRunning] = useState<string[] | null>(null)
  const [result, setResult] = useState<InstallPrereqsOutput | null>(null)
  // An install is drawn in the panel the GUI lends its call (its
  // steps, Cancel, its end).
  const [callPanel, setCallPanel] = useState<CallPanel | null>(null)
  const mkt = useMemo(() => installClient(setCallPanel, (slot, input) =>
    slot.endsWith('/install-prereqs') ? t('programs.title') : `${t('programs.reinstall')} ${(input as { name?: string })?.name ?? ''}`.trim(),
  ), [t])
  const inPanel = callPanel !== null
  // A new plan (after an install, or a check again) starts from its own selection.
  useEffect(() => { setSelected(initialSelection(plan)) }, [plan])

  const programs = orderedPrograms(plan)
  const required = programs.filter((p) => p.required)
  const optional = programs.filter((p) => !p.required)
  // A program whose install just succeeded is no longer offered, before the
  // plan is read again.
  const justInstalled = (name: string) => stepOf(result, name)?.status === 'installed'
  const requiredMissing = programsToInstall(plan).map((p) => p.name).filter((n) => !justInstalled(n))
  const chosen = [...selected].filter((n) => !justInstalled(n))
  const elevation = elevationAmong(plan, chosen)
  // An install's progress and result show in each program's own card; a step
  // whose program has no card is listed after them.
  const isInstalling = (name: string) => !inPanel && !!running?.some((n) => n.toLowerCase() === name.toLowerCase())
  const otherSteps = stepsWithoutCard(result, programs)
  const where = { platform: plan?.platform, universe }

  const install = async (names: string[]) => {
    if (!names.length) return
    const { home } = universeTarget(universe)
    setRunning(names)
    setResult(null)
    setCallPanel(null)
    try {
      const out = await mkt.installPrereqs({ universes: homes?.length ? homes : [home], programs: names })
      setResult(out)
    } catch (e) {
      pushToast({ message: errText(e), color: 'red' })
    } finally {
      setRunning(null)
      onDone()
    }
  }

  // A sidecar a universe is wired to whose files are gone: installed again
  // from its marketplace; the plan read after it rewires the universe to it.
  const [reinstalling, setReinstalling] = useState<string | null>(null)
  const reinstall = async (name: string) => {
    setReinstalling(name)
    setCallPanel(null)
    try {
      await mkt.install({ name })
      pushToast({ message: t('programs.reinstalled', { name }), color: 'green' })
    } catch (e) {
      pushToast({ message: errText(e), color: 'red' })
    } finally {
      setReinstalling(null)
      onDone()
    }
  }

  const toggle = (name: string, on: boolean) =>
    setSelected((s) => { const n = new Set(s); if (on) n.add(name); else n.delete(name); return n })

  return (
    <Section icon={<IconPackage size={14} />} title={t('programs.title')} hint={t('programs.hint')}>
      {error && <Alert color="red" variant="light" mb="xs">{t('programs.error', { error })}</Alert>}
      {plan && programs.length === 0 && <Text size="sm" c="dimmed">{t('programs.none')}</Text>}

      {(requiredMissing.length > 0 || chosen.length > 0) && (
        <Group gap="xs" mb="sm">
          <Button size="xs" leftSection={<IconDownload size={14} />} disabled={!requiredMissing.length || !!running}
            onClick={() => void install(requiredMissing)} data-guide="ready.install-all">
            {t('programs.installAll')}
          </Button>
          <Button size="xs" variant="light" disabled={!chosen.length || !!running} onClick={() => void install(chosen)}>
            {t('programs.installSelected', { count: chosen.length })}
          </Button>
        </Group>
      )}

      {callPanel && (
        <Box mb="sm" data-guide="ready.install-panel">
          <DisplayPanel universeName={callPanel.universe} panelId={callPanel.panelId} variant="inline" title={callPanel.title} />
        </Box>
      )}

      {elevation.length > 0 && !running && (
        <Alert color="yellow" variant="light" mb="sm" icon={<IconShieldLock size={16} />}>
          {t('programs.elevation', { programs: elevation.join(', ') })}
        </Alert>
      )}

      <Stack gap={6}>
        {required.map((p) => (
          <ProgramRow key={p.name} p={p} plan={plan} checked={selected.has(p.name)} busy={!!running}
            installing={isInstalling(p.name)} step={stepOf(result, p.name)} rechecked={recheckedOf(result, p.name)} where={where}
            onCheck={(on) => toggle(p.name, on)} onInstall={() => void install([p.name])} t={t} />
        ))}
      </Stack>

      {optional.length > 0 && (
        <Box mt="md">
          <Text size="sm" fw={600}>{t('programs.optionalTitle')}</Text>
          <Text size="xs" c="dimmed" mb={6}>{t('programs.optionalHint')}</Text>
          <Stack gap={6}>
            {optional.map((p) => (
              <ProgramRow key={p.name} p={p} plan={plan} checked={selected.has(p.name)} busy={!!running}
                installing={isInstalling(p.name)} step={stepOf(result, p.name)} rechecked={recheckedOf(result, p.name)} where={where}
                onCheck={(on) => toggle(p.name, on)} onInstall={() => void install([p.name])} t={t} />
            ))}
          </Stack>
        </Box>
      )}

      {otherSteps.length > 0 && (
        <Box mt="md">
          <Text size="sm" fw={600} mb={6}>{t('programs.otherSteps')}</Text>
          <Stack gap={6}>
            {otherSteps.map((s) => (
              <Paper key={s.program} withBorder p="xs" radius="sm">
                <Text size="sm" fw={600}>{s.program}</Text>
                <StepResult step={s} where={where} t={t} />
              </Paper>
            ))}
          </Stack>
        </Box>
      )}

      {plan?.notInstalled?.length ? (
        <Alert color="orange" variant="light" mt="sm" title={t('programs.notInstalledTitle')} data-guide="ready.not-installed">
          <Text size="xs" mb={6}>{t('programs.notInstalledHint')}</Text>
          <Stack gap={4}>
            {plan.notInstalled.map((name) => (
              <Group key={name} gap="xs" justify="space-between">
                <Text size="sm" fw={600}>{name}</Text>
                <Button size="xs" variant="light" leftSection={<IconDownload size={13} />} loading={reinstalling === name}
                  disabled={!!running || (!!reinstalling && reinstalling !== name)} onClick={() => void reinstall(name)}
                  data-guide="ready.reinstall">
                  {t('programs.reinstall')}
                </Button>
              </Group>
            ))}
          </Stack>
        </Alert>
      ) : null}

      {plan?.rewired?.length ? (
        <Text size="xs" c="dimmed" mt="xs">{t('programs.rewired', { members: plan.rewired.join(', ') })}</Text>
      ) : null}

      {plan?.unchecked?.length ? (
        <Text size="xs" c="dimmed" mt="xs">{t('programs.unchecked', { sidecars: plan.unchecked.join(', ') })}</Text>
      ) : null}
    </Section>
  )
}

function ProgramRow({ p, plan, checked, busy, installing, step, rechecked, where, onCheck, onInstall, t }: {
  p: Program; plan?: PlanPrereqsOutput; checked: boolean; busy: boolean
  installing: boolean; step?: InstallStep; rechecked?: Program; where: { platform?: string; universe?: string }
  onCheck: (on: boolean) => void; onInstall: () => void; t: TFn
}) {
  const ok = shownInstalled(p, step)
  const offered = offeredToInstall(p, step)
  const notFound = lookedForAfterInstall(p, step, rechecked)
  const manual = p.state === 'manual'
  const stateBadge = ok
    ? <Badge size="xs" color="teal" variant="light">{p.found ? t('programs.stateOkVersion', { version: p.found }) : t('programs.stateOk')}</Badge>
    : manual
      ? <Badge size="xs" color="red" variant="light">{t('programs.stateManual')}</Badge>
      : <Badge size="xs" color="orange" variant="light">{t('programs.stateInstall')}</Badge>
  return (
    <Paper withBorder p="xs" radius="sm" data-guide="ready.program" data-guide-name={p.name} style={{ opacity: ok ? 0.75 : 1 }}>
      <Group gap="xs" wrap="nowrap" align="flex-start">
        <Checkbox mt={2} size="xs" checked={checked && offered} disabled={!offered || busy} onChange={(e) => onCheck(e.currentTarget.checked)}
          aria-label={p.name} />
        <Box style={{ flex: 1, minWidth: 0 }}>
          <Group gap={6} wrap="wrap">
            <Text size="sm" fw={600}>{p.name}</Text>
            {p.version && <Badge size="xs" variant="outline" color="gray">{t('programs.version', { version: p.version })}</Badge>}
            <Badge size="xs" variant="light" color={p.required ? 'blue' : 'gray'}>{p.required ? t('programs.required') : t('programs.optional')}</Badge>
            {stateBadge}
            {scopeBadge(p, plan, step) === 'admin' && (
              <Tooltip label={t('programs.adminTip')} withArrow multiline w={280}>
                <Badge size="xs" variant="light" color="yellow" tt="none" style={{ flexShrink: 0 }}
                  leftSection={<IconShieldLock size={10} />} data-guide="ready.program-admin">
                  {t('programs.admin')}
                </Badge>
              </Tooltip>
            )}
          </Group>
          {p.neededBy?.length > 0 && (
            <Group gap={4} mt={4} wrap="wrap">
              <Text size="xs" c="dimmed">{t('programs.neededBy')}</Text>
              {p.neededBy.map((n) => <Badge key={n} size="xs" variant="default">{n}</Badge>)}
            </Group>
          )}
          {p.installSkill && <Text size="xs" c="dimmed" mt={2}>{t('programs.skill', { skill: p.installSkill })}</Text>}
          {manual && p.fallback && <Text size="xs" mt={2}>{p.fallback}</Text>}
          {installing && (
            <Box mt={6} data-guide="ready.program-installing">
              <Text size="xs" mb={4}>{t('programs.installingOne')}</Text>
              <Progress value={100} striped animated />
            </Box>
          )}
          {!installing && step && <StepResult step={step} where={where} t={t} />}
          {!installing && notFound && (
            <Text size="xs" c="orange" mt={4} data-guide="ready.program-not-found">
              {t(`programs.${notFound.key}`, { what: notFound.what })}
            </Text>
          )}
        </Box>
        {offered && (
          <Button size="compact-xs" variant="subtle" leftSection={<IconDownload size={12} />} disabled={busy} onClick={onInstall}>
            {t('programs.install')}
          </Button>
        )}
      </Group>
    </Paper>
  )
}

/** What an install did for one program, in its card: installed, or failed
 *  with a short error and a button opening the full error report, which
 *  « Copier » gives as text ready to paste into an assistant's prompt. */
function StepResult({ step, where, t }: { step: InstallStep; where: { platform?: string; universe?: string }; t: TFn }) {
  const [open, setOpen] = useState(false)
  const failed = step.status === 'failed'
  const report = failed ? errorReport(step, where) : ''
  return (
    <Box mt={6} data-guide="ready.program-step" data-guide-status={step.status}>
      <Group gap="xs" wrap="nowrap" align="flex-start">
        <Badge size="xs" color={step.status === 'installed' ? 'teal' : failed ? 'red' : 'gray'} style={{ flexShrink: 0 }}>
          {t(`programs.step.${step.status}`)}
        </Badge>
        {step.detail && <Text size="xs" c={failed ? 'red' : 'dimmed'} style={{ wordBreak: 'break-word' }}>{step.detail}</Text>}
      </Group>
      {failed && (
        <>
          <Button size="compact-xs" variant="light" color="red" mt={6} leftSection={<IconFileAlert size={12} />}
            onClick={() => setOpen(true)} data-guide="ready.program-report">
            {t('programs.reportOpen')}
          </Button>
          <Modal opened={open} onClose={() => setOpen(false)} size="xl" title={t('programs.reportTitle', { program: step.program })}>
            <Text size="xs" c="dimmed" mb="xs">{t('programs.reportHint')}</Text>
            <ScrollArea.Autosize mah="60vh" type="auto">
              <Code block data-guide="ready.program-report-text">{report}</Code>
            </ScrollArea.Autosize>
            <Group justify="flex-end" mt="sm">
              <CopyButton value={report}>
                {({ copied, copy }) => (
                  <Button size="xs" leftSection={copied ? <IconCheck size={14} /> : <IconCopy size={14} />} color={copied ? 'teal' : undefined}
                    onClick={copy} data-guide="ready.program-report-copy">
                    {copied ? t('programs.copied') : t('programs.copy')}
                  </Button>
                )}
              </CopyButton>
            </Group>
          </Modal>
        </>
      )}
    </Box>
  )
}

// ── 3. Clés ──────────────────────────────────────────────────────────────

function KeysSection({ universe, inputs, onWritten, t }: {
  universe: string; inputs: ReturnType<typeof useReadyInputs>; onWritten: () => void; t: TFn
}) {
  const pushToast = useAppStore((s) => s.pushToast)
  const keys = inputs.keys
  const [imported, setImported] = useState(() => keySetsImported(universe))
  const [password, setPassword] = useState('')
  const [passwordError, setPasswordError] = useState('')
  const [values, setValues] = useState<Record<string, string>>({})
  const [busy, setBusy] = useState(false)
  const [fileMode, setFileMode] = useState(false)
  const [file, setFile] = useState('')
  const [filePassword, setFilePassword] = useState('')
  const [filePasswordError, setFilePasswordError] = useState('')
  useEffect(() => { setImported(keySetsImported(universe)) }, [universe])

  const step = keysStep(keys, imported)
  const remaining: RemainingKey[] = keys?.remaining ?? []
  const calls = useMemo(() => saveCalls(remaining, values), [remaining, values])

  const importKeySets = async () => {
    const { home, solution } = universeTarget(universe)
    if (!solution) return
    setBusy(true)
    setPasswordError('')
    try {
      const out = await marketplaceClient().importKeySets({ solution, universeHome: home, password })
      markKeySetsImported(universe)
      setImported(true)
      setPassword('')
      pushToast({ message: t('keys.imported', { count: out.written?.length ?? 0 }), color: 'green' })
      onWritten()
    } catch (e) {
      setPasswordError(errText(e))
    } finally {
      setBusy(false)
    }
  }

  const save = async () => {
    setBusy(true)
    let saved = 0
    try {
      for (const c of calls) {
        const out = await marketplaceClient().applyConfigSet(c)
        saved += out.keysWritten?.length ?? Object.keys(c.values ?? {}).length
      }
      setValues({})
      pushToast({ message: t('keys.saved', { count: saved }), color: 'green' })
    } catch (e) {
      pushToast({ message: errText(e), color: 'red' })
    } finally {
      setBusy(false)
      if (saved) onWritten()
    }
  }

  // A keys file the team gave, opened with its password (ImportKeys).
  const pickFile = async () => {
    try {
      const res = await PickKeysFile()
      if (res.error) pushToast({ message: res.error, color: 'red' })
      else if (res.path) { setFile(res.path); setFileMode(true) }
    } catch (e) {
      pushToast({ message: errText(e), color: 'red' })
    }
  }
  const importFile = async () => {
    const { home } = universeTarget(universe)
    setBusy(true)
    setFilePasswordError('')
    try {
      const out = await marketplaceClient().importKeys({ file, password: filePassword, universeHome: home })
      setFilePassword('')
      setFile('')
      setFileMode(false)
      pushToast({ message: t('keys.imported', { count: out.written?.length ?? 0 }), color: 'green' })
      onWritten()
    } catch (e) {
      setFilePasswordError(errText(e))
    } finally {
      setBusy(false)
    }
  }

  const multi = (keys?.universes?.length ?? 0) > 1
  const blocks = keysBlocks(keys, imported)
  // Before the team password is tried or skipped it stands alone, a keys file
  // behind its link (fileMode); once tried, the file and the form follow it.
  const alone = step === 'password'
  const teamSets = alone && !fileMode

  // A password and its import: the team's (key sets) or a keys file's.
  const passwordField = (forFile: boolean) => {
    const value = forFile ? filePassword : password
    const set = forFile ? setFilePassword : setPassword
    const error = forFile ? filePasswordError : passwordError
    const onImport = () => void (forFile ? importFile() : importKeySets())
    return (
    <Group align="flex-end" gap="xs" wrap="nowrap">
      <PasswordInput size="sm" style={{ flex: 1 }} label={t('keys.password')} value={value} error={error || undefined}
        onChange={(e) => set(e.currentTarget.value)}
        onKeyDown={(e) => { if (e.key === 'Enter' && value) onImport() }} />
      <Button size="sm" leftSection={<IconKey size={14} />} disabled={!value} loading={busy} onClick={onImport}>
        {t('keys.import')}
      </Button>
    </Group>
    )
  }
  const filePicker = (
    <Group gap="xs" mb="xs">
      <Button size="xs" variant="light" leftSection={<IconFile size={14} />} onClick={() => void pickFile()}>{t('keys.pickFile')}</Button>
      {file && <Code>{file}</Code>}
    </Group>
  )

  // The team password: the first block whenever the solution provides key sets.
  const passwordBlock = (
    <Paper key="password" withBorder p="md" radius="md" bg="var(--mantine-color-blue-light)" data-guide="ready.password" data-element="action:ready/import-keys">
      <Group gap="xs" mb={4}>
        <ThemeIcon variant="filled" radius="md"><IconKey size={16} /></ThemeIcon>
        <Title order={4}>{t('keys.importTitle')}</Title>
      </Group>
      <Text size="sm" mb="sm">
        {alone && !teamSets ? t('keys.fileHint') : t('keys.passwordHint', { solution: keys?.solution ?? universe })}
      </Text>
      {(teamSets || !alone) && (
        <Group gap={6} mb="xs">
          {(keys?.keySets ?? []).map((k) => (
            <Badge key={k.name} size="sm" variant="light" color="grape" leftSection={<IconShieldLock size={10} />}>
              {k.name} {k.version}
            </Badge>
          ))}
        </Group>
      )}
      {alone && !teamSets && filePicker}
      {(teamSets || !alone) && passwordField(false)}
      {alone && !teamSets && file && passwordField(true)}
      {alone && (
        <Group gap="md" mt="xs">
          {teamSets && <Anchor component="button" type="button" size="xs" onClick={() => setFileMode(true)}>{t('keys.useFile')}</Anchor>}
          {!teamSets && <Anchor component="button" type="button" size="xs" onClick={() => { setFileMode(false); setFile('') }}>{t('keys.usePassword')}</Anchor>}
          <Anchor component="button" type="button" size="xs" onClick={() => { setImported(true); setFileMode(false) }}>{t('keys.skipPassword')}</Anchor>
        </Group>
      )}
    </Paper>
  )

  // A keys file the team gave: the first block without key sets, an
  // alternative after the team password with them.
  const fileBlock = keys?.passwordNeeded ? (
    <Paper key="file" withBorder p="md" radius="md" data-guide="ready.keys-file" data-element="action:ready/import-keys-file">
      <Text size="sm" fw={600}>{t('keys.fileTitle')}</Text>
      <Text size="xs" c="dimmed" mb="xs">{t('keys.fileHint')}</Text>
      {filePicker}
      {file && passwordField(true)}
    </Paper>
  ) : (
    <Paper key="file" withBorder p="md" radius="md" bg="var(--mantine-color-blue-light)" data-guide="ready.password" data-element="action:ready/import-keys">
      <Group gap="xs" mb={4}>
        <ThemeIcon variant="filled" radius="md"><IconKey size={16} /></ThemeIcon>
        <Title order={4}>{t('keys.importTitle')}</Title>
      </Group>
      <Text size="sm" mb="sm">{t('keys.fileHint')}</Text>
      {filePicker}
      {file && passwordField(true)}
    </Paper>
  )

  const formBlock = (
    <Paper key="form" withBorder p="md" radius="md" data-guide="ready.keys-form" data-element="section:ready/keys-form">
      <Text size="sm" fw={600}>{t('keys.formTitle')}</Text>
      <Text size="xs" c="dimmed" mb="xs">{t('keys.formHint')}</Text>
      <Stack gap="xs">
        {remaining.map((k) => {
          const id = keyId(k)
          const label = (
            <Group gap={6} wrap="nowrap" component="span">
              <Text span size="sm" fw={600} ff="monospace">{k.name}</Text>
              <Badge size="xs" variant="light" color={k.scope === 'global' ? 'indigo' : 'gray'}>
                {k.scope === 'global' ? t('keys.scopeGlobal') : t('keys.scopeUniverse')}
              </Badge>
              {multi && k.scope !== 'global' && <Badge size="xs" variant="outline" color="gray">{baseName(k.universeHome)}</Badge>}
              {k.secret && <Badge size="xs" variant="light" color="grape">{t('keys.secret')}</Badge>}
            </Group>
          )
          const Input = k.secret ? PasswordInput : TextInput
          return (
            <Input key={id} size="xs" label={label} description={k.description} value={values[id] ?? ''}
              onChange={(e) => { const v = e.currentTarget.value; setValues((s) => ({ ...s, [id]: v })) }} />
          )
        })}
      </Stack>
      <Group gap="xs" mt="sm">
        <Button size="xs" leftSection={<IconCheck size={14} />} disabled={!calls.length} loading={busy} onClick={() => void save()}>
          {t('keys.save')}
        </Button>
      </Group>
    </Paper>
  )

  const shown = { password: passwordBlock, file: fileBlock, form: formBlock }
  return (
    <Stack gap="md">
      <Intro text={t('keys.intro')} />
      {inputs.errors.keys && <Alert color="red" variant="light">{t('keys.error', { error: inputs.errors.keys })}</Alert>}

      {keys && step === 'done' && (
        <Group gap="xs"><ThemeIcon size="sm" radius="xl" color="teal" variant="light"><IconCheck size={12} /></ThemeIcon><Text size="sm">{t('keys.done')}</Text></Group>
      )}

      {keys && blocks.map((b) => shown[b])}

      {!keys && !inputs.errors.keys && <Loader size="xs" />}
    </Stack>
  )
}

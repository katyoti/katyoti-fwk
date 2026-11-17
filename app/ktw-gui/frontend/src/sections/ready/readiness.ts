// SPDX-License-Identifier: Apache-2.0
/**
 * readiness — the pure state of the Ready to start screen.
 *
 * The screen reads three hub-owned answers about the universe on screen, all
 * over KTSP through the generated marketplace client:
 *
 *   - the universe's config check (Doctor): each sidecar's status;
 *   - the programs plan (PlanPrereqs): what the sidecars need on this machine,
 *     each with the sidecars it is for and its install skill;
 *   - the keys (KeySets): the solution's sealed key sets (one team password)
 *     and the required keys still without a value (one form);
 *
 * and beside them the person's profile, the installation's projects folder
 * and the universe's project folders, which the first tab (Profil et premiers
 * pas) needs. Each of the screen's
 * three tabs is green or red from these (`tabState`); a part not read is
 * never green.
 *
 * Everything the screen and its badge decide — fully operational or not, which
 * programs are preselected, which step the keys are at, which calls a filled
 * form makes — is computed here from those answers, with no React and no Wails,
 * so the tests read it directly (tests/ready-screen.test.ts). Generic: no
 * sidecar, program or key is named.
 */

import type {
  ApplyConfigSetInput,
  DoctorOutput,
  InstallPrereqsOutput,
  KeySetsOutput,
  PlanPrereqsOutput,
  ProjectsFolderOutput,
} from '../../ktsp/marketplace'
import type { GetOutput as ProfileOutput } from '../../ktsp/profile'

/** One sidecar's config check, tagged with the universe (home) it ran in. */
export type SidecarCheck = DoctorOutput[number] & { universeHome?: string }
/** One program of the plan. */
export type Program = PlanPrereqsOutput['programs'][number]
/** One key the form asks for. */
export type RemainingKey = NonNullable<KeySetsOutput['remaining']>[number]
/** One problem a sidecar's check reports. */
export type CheckItem = SidecarCheck['report']['items'][number]

/** What the screen read about one universe; a part is absent until read, and
 *  carries its error when it could not be read. */
export interface ReadyInputs {
  loading: boolean
  doctor?: SidecarCheck[]
  plan?: PlanPrereqsOutput
  keys?: KeySetsOutput
  /** The person's profile (the admin hub's), and the project folders the
   *  universe attaches — where its first study is created. */
  profile?: ProfileOutput
  /** The installation's projects folder: where every project
   *  of the person lives, chosen outside the installation. */
  projectsFolder?: ProjectsFolderOutput
  projects?: string[]
  errors: { doctor?: string; plan?: string; keys?: string; profile?: string; projectsFolder?: string; projects?: string }
}

/** The screen's (and the badge's) overall state. */
export type ReadyState = 'checking' | 'ready' | 'not-ready' | 'unknown'

/** The counts the badge and the screen's summary show. */
export interface ReadySummary {
  state: ReadyState
  sidecars: { total: number; failing: number }
  programs: { toInstall: number; manual: number; optional: number }
  keys: { missing: number; passwordNeeded: boolean }
}

/** A sidecar check's problems: every item not ok. */
export function sidecarProblems(check: SidecarCheck): CheckItem[] {
  return (check.report.items ?? []).filter((i) => i.status !== 'ok')
}

/** A sidecar is operational when its check says healthy. */
export function sidecarOk(check: SidecarCheck): boolean {
  return !!check.report.healthy
}

/** The required programs a recipe installs on this machine, still missing. */
export function programsToInstall(plan: PlanPrereqsOutput | undefined): Program[] {
  return (plan?.programs ?? []).filter((p) => p.required && p.state === 'install')
}

/** The required programs no recipe installs here: the person installs them by
 *  hand (the plan's fallback says how). */
export function programsManual(plan: PlanPrereqsOutput | undefined): Program[] {
  return (plan?.programs ?? []).filter((p) => p.required && p.state === 'manual')
}

/** The optional programs not on the machine: offered, never preselected. */
export function programsOptional(plan: PlanPrereqsOutput | undefined): Program[] {
  return (plan?.programs ?? []).filter((p) => !p.required && p.state !== 'ok')
}

/** The programs in the order the screen lists them: required before optional,
 *  missing before present, then by name. */
export function orderedPrograms(plan: PlanPrereqsOutput | undefined): Program[] {
  const rank = (p: Program) => (p.required ? 0 : 2) + (p.state === 'ok' ? 1 : 0)
  return [...(plan?.programs ?? [])].sort((a, b) => rank(a) - rank(b) || a.name.localeCompare(b.name))
}

/** The selection the screen starts with: the required programs to install —
 *  an optional program is offered unselected. */
export function initialSelection(plan: PlanPrereqsOutput | undefined): Set<string> {
  return new Set(programsToInstall(plan).map((p) => p.name))
}

/** A program can be ticked when a recipe installs it here and it is missing. */
export function installable(p: Program): boolean {
  return p.state === 'install'
}

/** Whether a program's card shows it installed: the plan found it, or its
 *  install step just succeeded - the card does not wait for the plan read
 *  again. */
export function shownInstalled(p: Program, step: InstallStep | undefined): boolean {
  return p.state === 'ok' || step?.status === 'installed'
}

/** Whether a program's card offers it to install (its checkbox and Installer
 *  button): installable, and not just installed - a failed install keeps it
 *  offered. */
export function offeredToInstall(p: Program, step: InstallStep | undefined): boolean {
  return installable(p) && step?.status !== 'installed'
}

/** Whether a program needs administrator rights here (the plan's elevation list:
 *  those steps run together, after one prompt). */
export function needsElevation(plan: PlanPrereqsOutput | undefined, name: string): boolean {
  return (plan?.elevation ?? []).includes(name)
}

/** The scope badge a program's card carries: « admin » when installing it
 *  needs administrator rights here (its scope, or the plan's elevation list),
 *  while it is still to install - neither found nor just installed; none
 *  otherwise - every other program installs in the person's profile, which the
 *  panel's subtitle already says. */
export function scopeBadge(p: Program, plan: PlanPrereqsOutput | undefined, step?: InstallStep): 'admin' | null {
  if (shownInstalled(p, step)) return null
  return p.scope === 'admin' || needsElevation(plan, p.name) ? 'admin' : null
}

/** The elevation note's programs among those about to be installed. */
export function elevationAmong(plan: PlanPrereqsOutput | undefined, names: Iterable<string>): string[] {
  const set = new Set(names)
  return (plan?.elevation ?? []).filter((n) => set.has(n))
}

/** The step the keys are at:
 *  - `password`: the solution provides sealed key sets, keys are still missing
 *    and the key sets were not imported yet — the team password, once;
 *  - `form`: keys are still missing — the one form;
 *  - `done`: no required key is missing. */
export type KeysStep = 'password' | 'form' | 'done'

export function keysStep(keys: KeySetsOutput | undefined, keySetsImported: boolean): KeysStep {
  const remaining = keys?.remaining ?? []
  if (remaining.length === 0) return 'done'
  if (keys?.passwordNeeded && !keySetsImported) return 'password'
  return 'form'
}

/** A block of the Keys tab: the team password (it opens the solution's key
 *  sets and fills the keys), a keys file, the keys one by one. */
export type KeysBlock = 'password' | 'file' | 'form'

/** The Keys tab's blocks, in the order shown. When the solution provides key
 *  sets, the team password is always first: alone until it is tried or
 *  skipped (a keys file and the keys by hand behind its links), then followed
 *  by a keys file and the keys one by one - the alternatives for what the key
 *  sets do not hold. Without key sets: a keys file, then the keys one by one.
 *  No key missing: none. */
export function keysBlocks(keys: KeySetsOutput | undefined, keySetsImported: boolean): KeysBlock[] {
  switch (keysStep(keys, keySetsImported)) {
    case 'done': return []
    case 'password': return ['password']
    default: return keys?.passwordNeeded ? ['password', 'file', 'form'] : ['file', 'form']
  }
}

/** A key's identity in the form: a global key is one value for every universe;
 *  a universe key is that universe's. */
export function keyId(k: RemainingKey): string {
  return k.scope === 'global' ? `global|${k.name}` : `${k.universeHome}|${k.name}`
}

/** `name@version` → its name and version (no `@`: the name alone). */
export function splitRef(ref: string): { name: string; version?: string } {
  const at = ref.lastIndexOf('@')
  return at > 0 ? { name: ref.slice(0, at), version: ref.slice(at + 1) } : { name: ref }
}

/** The calls a filled form makes: one ApplyConfigSet per config-set and
 *  universe, with the values given for its keys (an empty value is not sent).
 *  A key of no config-set (a sidecar's own check asks for it, the sidecar
 *  declares it) is saved the same way: ApplyConfigSet with an empty name, the
 *  universe's own key set. */
export function saveCalls(remaining: RemainingKey[], values: Record<string, string>): ApplyConfigSetInput[] {
  const calls = new Map<string, ApplyConfigSetInput>()
  for (const k of remaining) {
    const v = (values[keyId(k)] ?? '').trim()
    if (!v) continue
    const id = `${k.configSet ?? ''}|${k.universeHome}`
    let call = calls.get(id)
    if (!call) {
      call = { ...splitRef(k.configSet ?? ''), universeHome: k.universeHome, values: {} }
      calls.set(id, call)
    }
    call.values![k.name] = v
  }
  return [...calls.values()]
}

/** What the screen and its badge say about the universe. */
export function summarize(inp: ReadyInputs): ReadySummary {
  const doctor = inp.doctor ?? []
  const failing = doctor.filter((d) => !sidecarOk(d)).length
  const toInstall = programsToInstall(inp.plan).length
  const manual = programsManual(inp.plan).length
  const optional = programsOptional(inp.plan).length
  const missing = inp.keys?.remaining?.length ?? 0
  const summary: ReadySummary = {
    state: 'ready',
    sidecars: { total: doctor.length, failing },
    programs: { toInstall, manual, optional },
    keys: { missing, passwordNeeded: !!inp.keys?.passwordNeeded },
  }
  const wrong = failing + toInstall + manual + missing > 0
  const unread = !!(inp.errors.doctor || inp.errors.plan || inp.errors.keys)
  if (wrong) summary.state = 'not-ready'
  else if (inp.loading) summary.state = 'checking'
  else if (unread) summary.state = 'unknown'
  else if (!inp.doctor && !inp.plan && !inp.keys) summary.state = 'checking'
  return summary
}

/** What the screen's header says: the universe's summary, except that it is
 *  « everything is in place » only when every tab is green - a red or
 *  unverified tab (the profile, which the summary does not count) leaves it
 *  « tabs-left », naming them. The badge and the log line keep
 *  the summary's state. */
export function headerState(inp: ReadyInputs): { state: ReadySummary['state'] | 'tabs-left'; left: TabKey[] } {
  const state = summarize(inp).state
  if (state !== 'ready') return { state, left: [] }
  const left = TABS.filter((k) => { const s = tabState(inp, k); return s === 'todo' || s === 'unverified' })
  return left.length ? { state: 'tabs-left', left } : { state, left }
}

/** The line the GUI's log gets at the end of each check of a universe: its
 *  state, the counts and whether every part was read. `verified=no` is what
 *  the screen shows as « Non vérifié »: a part that could not be read, an
 *  error, or a check that never finished. The clean-machine proof reads the
 *  last one. */
export function readyLogLine(universe: string, inp: ReadyInputs): string {
  const s = summarize(inp)
  const errs = (['doctor', 'plan', 'keys'] as const)
    .filter((k) => inp.errors[k])
    .map((k) => `${k}: ${String(inp.errors[k]).replace(/\s+/g, ' ')}`)
  const verified = errs.length === 0 && !inp.loading && s.state !== 'checking' && s.state !== 'unknown'
  return `universe=${universe} state=${s.state} verified=${verified ? 'yes' : 'no'}` +
    ` sidecars=${s.sidecars.total - s.sidecars.failing}/${s.sidecars.total}` +
    ` programs=${s.programs.toInstall + s.programs.manual} keys=${s.keys.missing}` +
    (errs.length ? ` errors=${errs.join('; ')}` : '')
}

/** The badge shows whenever the universe is not fully operational: something
 *  is wrong, or a part could not be checked. */
export function badgeShown(s: ReadySummary): boolean {
  return s.state === 'not-ready' || s.state === 'unknown'
}

/** How many things the badge counts: the failing sidecars, the required
 *  programs to install, the missing keys. */
export function badgeCount(s: ReadySummary): number {
  return s.sidecars.failing + s.programs.toInstall + s.programs.manual + s.keys.missing
}

/** The screen's tabs, one per step a first-time person goes through, in order. */
export const TABS = ['profile', 'programs', 'keys'] as const
export type TabKey = (typeof TABS)[number]

/** A tab's state in its title:
 *  - `ok` (green): its step is done;
 *  - `todo` (red): something is left to do there;
 *  - `unverified` (red, « Non vérifié »): a part it shows could not be read —
 *    never green, whatever the counts say (an unread part is not zero problems);
 *  - `checking`: still being read. */
export type TabState = 'ok' | 'todo' | 'unverified' | 'checking'

/** The config-check problems a tab answers: a program to the Programs tab, a
 *  key or a setting to the Keys tab. */
const KEY_CATEGORIES = new Set(['env', 'config', 'key', 'keys'])
const tabOfCategory = (category: string): TabKey | undefined =>
  category === 'program' ? 'programs' : KEY_CATEGORIES.has(category) ? 'keys' : undefined

/** The failing sidecars no other tab shows the cause of: shown as technical
 *  details on the Programs tab (which they keep red), never as a first screen.
 *  While a required program is missing or a key remains, a sidecar whose
 *  problems are all programs or keys is answered by those tabs; once the plan
 *  and the keys are complete, every failing sidecar is technical — so a red
 *  badge always has a red tab to open. */
export function technicalProblems(inp: ReadyInputs): SidecarCheck[] {
  const failing = (inp.doctor ?? []).filter((d) => !sidecarOk(d))
  const shown = programsToInstall(inp.plan).length + programsManual(inp.plan).length + (inp.keys?.remaining?.length ?? 0) > 0
  if (!shown) return failing
  return failing.filter((d) => {
    const problems = sidecarProblems(d)
    return problems.length === 0 || problems.some((p) => !tabOfCategory(p.category))
  })
}

/** The projects folder is in place when the person chose it — a folder only
 *  proposed is not — or when there is no installation to set it for (a
 *  development build: nothing is asked, never blocking). */
export function projectsFolderOk(inp: ReadyInputs): boolean {
  const f = inp.projectsFolder
  return !!f && (f.chosen || !f.installation)
}

/** Whether the screen asks for the projects folder at all: not on a
 *  development build, which has no installation. Unread, it is asked. */
export function projectsFolderAsked(inp: ReadyInputs): boolean {
  return !inp.projectsFolder || !!inp.projectsFolder.installation
}

/** The profile is done when the person said who they are — a real profile,
 *  not the placeholder, with a name and an e-mail — the installation's
 *  projects folder is chosen, and the universe has the project folder its
 *  first study goes into. */
export function profileDone(inp: ReadyInputs): boolean {
  return profileNamed(inp.profile) && projectsFolderOk(inp) && (inp.projects?.length ?? 0) > 0
}

/** The person said who they are: a real profile, not the placeholder, with a
 *  handle, a name and an e-mail (asked in place until then). */
export function profileNamed(p: ProfileOutput | undefined): boolean {
  return !!p && p.source !== 'placeholder' && !!p.id && !!p.email && !!(p.firstName || p.lastName)
}

/** Each tab's state, from what was read. */
export function tabState(inp: ReadyInputs, tab: TabKey): TabState {
  const parts: { read: unknown; error?: string }[] =
    tab === 'profile' ? [{ read: inp.profile, error: inp.errors.profile }, { read: inp.projectsFolder, error: inp.errors.projectsFolder }, { read: inp.projects, error: inp.errors.projects }]
      : tab === 'programs' ? [{ read: inp.plan, error: inp.errors.plan }, { read: inp.doctor, error: inp.errors.doctor }]
        : [{ read: inp.keys, error: inp.errors.keys }]
  if (parts.some((p) => p.error)) return 'unverified'
  if (parts.some((p) => p.read === undefined)) return inp.loading ? 'checking' : 'unverified'
  const done =
    tab === 'profile' ? profileDone(inp)
      : tab === 'programs' ? programsToInstall(inp.plan).length + programsManual(inp.plan).length === 0 && technicalProblems(inp).length === 0
        : (inp.keys?.remaining?.length ?? 0) === 0
  return done ? 'ok' : 'todo'
}

/** The tab the screen opens on: the first one not green (still being read
 *  counts as not green only once it is read); all green, the first. */
export function firstTab(inp: ReadyInputs): TabKey {
  return TABS.find((t) => { const s = tabState(inp, t); return s === 'todo' || s === 'unverified' }) ?? TABS[0]
}

/** The installed version a start runs, what an update or a new install
 *  changes: the installed KaTyoti's version and the version of the universe's
 *  solution (its `name@version` ref), joined; empty when neither is known
 *  (a development build). */
export function installationVersion(katyoti: string | undefined, solutionRef: string | undefined): string {
  return [katyoti ?? '', solutionRef ? splitRef(solutionRef).version ?? '' : ''].filter(Boolean).join('+')
}

/** The record a start leaves once the screen opened by itself: the universe
 *  at the installed version (the universe alone when no version is known). */
export function seenKey(universe: string, version: string): string {
  return version ? `${universe}@${version}` : universe
}

/** The screen opens by itself at the first start of each installed version
 *  in a universe: once per universe and version, never again at that version. */
export function opensAtStart(universe: string | null, seen: ReadonlySet<string>, version = ''): boolean {
  return !!universe && !seen.has(seenKey(universe, version))
}

/** One step an install ran: a program installed, failed or skipped. */
export type InstallStep = NonNullable<InstallPrereqsOutput['steps']>[number]

/** The step an install ran for a program, matched by name whatever its case. */
export function stepOf(result: InstallPrereqsOutput | null | undefined, program: string): InstallStep | undefined {
  return result?.steps?.find((s) => s.program.toLowerCase() === program.toLowerCase())
}

/** The program as the install's own recheck saw it (the sidecars' checks run
 *  again once the steps ended), matched by name whatever its case; undefined
 *  while no recheck answered. */
export function recheckedOf(result: InstallPrereqsOutput | null | undefined, program: string): Program | undefined {
  return result?.after?.programs?.find((p) => p.name.toLowerCase() === program.toLowerCase())
}

/** What a card says when a program's install step succeeded yet its own check,
 *  run again after the install (rechecked), still does not find it: the
 *  message key and what was looked for - a Python package as that package
 *  (never the PATH), else its commands, else its name (a program checked by
 *  its sidecar's own probe). Null when it was found, its install did not
 *  succeed, or its check has not answered yet: the plan the card holds may
 *  still be the one read before the install. */
export function lookedForAfterInstall(p: Program, step: InstallStep | undefined, rechecked: Program | undefined):
  { key: 'notFoundAfterInstall' | 'notFoundAfterInstallPackage'; what: string } | null {
  if (!step || step.status !== 'installed' || p.state === 'ok') return null
  if (!rechecked || rechecked.state === 'ok') return null
  const pkg = rechecked.pythonPackage || p.pythonPackage
  if (pkg) return { key: 'notFoundAfterInstallPackage', what: pkg }
  return { key: 'notFoundAfterInstall', what: rechecked.commands?.length ? rechecked.commands.join(', ') : rechecked.name }
}

/** The steps of an install whose program has no card on the screen: they are
 *  listed apart, so a failure is never lost. */
export function stepsWithoutCard(result: InstallPrereqsOutput | null | undefined, programs: readonly Program[]): InstallStep[] {
  const shown = new Set(programs.map((p) => p.name.toLowerCase()))
  return (result?.steps ?? []).filter((s) => !shown.has(s.program.toLowerCase()))
}

/** A failed step's whole error report as plain text, ready to paste into an
 *  assistant's prompt: what was installed and where, the exact command, its
 *  exit code and what it means, everything it printed and where the
 *  installer's logs are. */
export function errorReport(step: InstallStep, where: { platform?: string; universe?: string } = {}): string {
  const r = step.report
  const lines = [
    `Installing the program "${step.program}" failed in KaTyoti's "Ready to start" screen.`,
    `Platform: ${where.platform || 'unknown'}${where.universe ? ` - universe: ${where.universe}` : ''}`,
    `Installer: ${step.kind || 'unknown'}, scope: ${step.scope || 'user'}`,
    `Error: ${step.detail || 'none given'}`,
  ]
  if (r) {
    lines.push(`Command: ${r.command}`)
    lines.push(`Exit code: ${r.exitCode === -1 ? 'none (the command was stopped or did not start)' : r.exitCode}${r.code && r.code !== String(r.exitCode) ? ` (${r.code})` : ''}`)
    if (r.meaning) lines.push(`Meaning: ${r.meaning}`)
    if (r.logs) lines.push(`Installer logs: ${r.logs}`)
    lines.push('', 'Full output:', '```', r.output?.trim() || '(nothing printed)', '```')
  } else {
    lines.push('No command ran: the error above is all there is.')
  }
  lines.push('', 'Why did it fail, and what should I do to install it?')
  return lines.join('\n')
}

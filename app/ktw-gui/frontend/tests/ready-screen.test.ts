// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import {
  badgeCount,
  badgeShown,
  firstTab,
  initialSelection,
  installationVersion,
  keyId,
  keysBlocks,
  keysStep,
  opensAtStart,
  orderedPrograms,
  projectsFolderAsked,
  scopeBadge,
  saveCalls,
  seenKey,
  summarize,
  tabState,
  technicalProblems,
  type InstallStep,
  type Program,
  type ReadyInputs,
  type RemainingKey,
  type SidecarCheck,
} from '../src/sections/ready/readiness'
import type { KeySetsOutput, PlanPrereqsOutput } from '../src/ktsp/marketplace'

// The Ready to start screen: what the universe on screen still
// needs — each sidecar's status from its config check, the programs to install,
// the keys — decided from the three hub answers, with no sidecar named.

const sidecar = (name: string, healthy: boolean): SidecarCheck => ({
  name,
  universeHome: '/u/main',
  report: {
    healthy, summary: healthy ? 'ok' : '1 problem',
    items: healthy ? [{ category: 'config', name: 'a.yaml', status: 'ok' }] : [{ category: 'env', name: 'A_KEY', status: 'missing', hint: 'set A_KEY' }],
    deps: [], env_keys: [], config_files: [],
  },
})
const program = (name: string, p: Partial<Program>): Program => ({ name, required: true, neededBy: ['s1'], selected: false, state: 'ok', ...p })
const plan = (...programs: Program[]): PlanPrereqsOutput => ({ platform: 'windows', programs })
const key = (name: string, p: Partial<RemainingKey> = {}): RemainingKey => ({ name, scope: 'universe', universeHome: '/u/main', configSet: 'base@1.0.0', ...p })
const keys = (remaining: RemainingKey[], passwordNeeded = false): KeySetsOutput => ({
  passwordNeeded, remaining, universes: ['/u/main'],
  keySets: passwordNeeded ? [{ name: 'team-keys', version: '1', opened: false }] : undefined,
})
const inputs = (p: Partial<ReadyInputs>): ReadyInputs => ({ loading: false, errors: {}, ...p })

describe('the screen state', () => {
  test('all green: every sidecar healthy, no program missing, no key missing — ready, no badge', () => {
    const s = summarize(inputs({ doctor: [sidecar('s1', true), sidecar('s2', true)], plan: plan(program('p', {})), keys: keys([]) }))
    expect(s.state).toBe('ready')
    expect(s.sidecars).toEqual({ total: 2, failing: 0 })
    expect(badgeShown(s)).toBe(false)
  })

  test('a missing required program: not ready, counted on the badge', () => {
    const s = summarize(inputs({ doctor: [sidecar('s1', true)], plan: plan(program('p', { state: 'install', selected: true })), keys: keys([]) }))
    expect(s.state).toBe('not-ready')
    expect(s.programs.toInstall).toBe(1)
    expect(badgeShown(s)).toBe(true)
    expect(badgeCount(s)).toBe(1)
  })

  test('a required program no recipe installs here still makes the universe not ready', () => {
    const s = summarize(inputs({ doctor: [], plan: plan(program('p', { state: 'manual', fallback: 'by hand' })), keys: keys([]) }))
    expect(s.state).toBe('not-ready')
    expect(s.programs.manual).toBe(1)
  })

  test('an optional program missing does not stop the universe', () => {
    const s = summarize(inputs({ doctor: [], plan: plan(program('opt', { required: false, state: 'install' })), keys: keys([]) }))
    expect(s.state).toBe('ready')
    expect(s.programs.optional).toBe(1)
  })

  test('a missing key alone: not ready, counted on the badge', () => {
    const s = summarize(inputs({ doctor: [sidecar('s1', true)], plan: plan(), keys: keys([key('A_KEY')]) }))
    expect(s.state).toBe('not-ready')
    expect(s.keys.missing).toBe(1)
    expect(badgeCount(s)).toBe(1)
  })

  test('a check again over a green universe says checking until it answers', () => {
    expect(summarize(inputs({ loading: true, doctor: [sidecar('s1', true)], plan: plan(), keys: keys([]) })).state).toBe('checking')
  })

  test('a missing key and a failing sidecar: not ready, each counted', () => {
    const s = summarize(inputs({ doctor: [sidecar('s1', false)], plan: plan(), keys: keys([key('A_KEY')]) }))
    expect(s.state).toBe('not-ready')
    expect(badgeCount(s)).toBe(2)
  })

  test('a part that could not be checked shows the badge as unknown', () => {
    const s = summarize(inputs({ doctor: [sidecar('s1', true)], keys: keys([]), errors: { plan: 'hub down' } }))
    expect(s.state).toBe('unknown')
    expect(badgeShown(s)).toBe(true)
  })

  test('while reading, nothing is wrong yet: checking, no badge', () => {
    const s = summarize(inputs({ loading: true }))
    expect(s.state).toBe('checking')
    expect(badgeShown(s)).toBe(false)
  })
})

describe('the programs', () => {
  const p = plan(
    program('zeta', { required: false, state: 'install', selected: false }),
    program('beta', { state: 'ok', found: '2.0' }),
    program('alpha', { state: 'install', selected: true }),
    program('gamma', { state: 'manual' }),
  )
  test('the required programs to install are preselected; an optional one is offered unselected', () => {
    expect([...initialSelection(p)]).toEqual(['alpha'])
    expect(initialSelection(p).has('zeta')).toBe(false)
  })
  test('required before optional, missing before present', () => {
    expect(orderedPrograms(p).map((x) => x.name)).toEqual(['alpha', 'gamma', 'beta', 'zeta'])
  })
})

describe('the keys', () => {
  test('the team password comes first, once, when the solution provides key sets', () => {
    expect(keysStep(keys([key('A')], true), false)).toBe('password')
  })
  test('then the remaining keys in one form', () => {
    expect(keysStep(keys([key('A')], true), true)).toBe('form')
    expect(keysStep(keys([key('A')], false), false)).toBe('form')
  })
  test('no key missing: done, even when the solution has key sets', () => {
    expect(keysStep(keys([], true), false)).toBe('done')
  })

  // With a key set the team password comes first, before a keys
  // file and the keys one by one - also once the key sets were imported and
  // keys they do not hold remain; without a key set the order is unchanged.
  test('with a key set the team password is the first block, the file and the keys by hand after', () => {
    expect(keysBlocks(keys([key('A')], true), false)).toEqual(['password'])
    expect(keysBlocks(keys([key('A')], true), true)).toEqual(['password', 'file', 'form'])
  })
  test('without a key set: a keys file, then the keys one by one', () => {
    expect(keysBlocks(keys([key('A')], false), false)).toEqual(['file', 'form'])
    expect(keysBlocks(keys([key('A')], false), true)).toEqual(['file', 'form'])
  })
  test('no key missing: no block', () => {
    expect(keysBlocks(keys([], true), true)).toEqual([])
    expect(keysBlocks(keys([], false), false)).toEqual([])
  })
  test('the Keys tab renders its blocks in the order keysBlocks gives', () => {
    const src = readFileSync(join(import.meta.dir, '../src/sections/ready/ReadyScreen.tsx'), 'utf8')
    expect(src).toContain('keysBlocks(keys, imported)')
    expect(src).toMatch(/blocks\.map\(/)
  })

  test('the form saves through one ApplyConfigSet per config-set and universe, empty values left out', () => {
    const remaining = [
      key('A', { secret: true }),
      key('B'),
      key('G', { scope: 'global' }),
      key('C', { configSet: 'other@2', universeHome: '/u/second' }),
      key('S', { configSet: undefined, source: 'sidecar:s9' }),
    ]
    const values = { [keyId(remaining[0])]: ' secret ', [keyId(remaining[1])]: '', [keyId(remaining[2])]: 'g', [keyId(remaining[3])]: 'c', [keyId(remaining[4])]: 's' }
    expect(saveCalls(remaining, values)).toEqual([
      { name: 'base', version: '1.0.0', universeHome: '/u/main', values: { A: 'secret', G: 'g' } },
      { name: 'other', version: '2', universeHome: '/u/second', values: { C: 'c' } },
      { name: '', universeHome: '/u/main', values: { S: 's' } },
    ])
  })

  test("a key only a sidecar's own check asks for is saved the same way: the universe's own key set (an empty name)", () => {
    const remaining = [key('A'), key('KEY_X', { configSet: undefined, source: 'sidecar:s9' })]
    const values = { [keyId(remaining[0])]: 'a', [keyId(remaining[1])]: 'x' }
    expect(saveCalls(remaining, values)).toEqual([
      { name: 'base', version: '1.0.0', universeHome: '/u/main', values: { A: 'a' } },
      { name: '', universeHome: '/u/main', values: { KEY_X: 'x' } },
    ])
  })

  test("no key is sent to a sidecar's settings: the screen and its strings have no such fallback", () => {
    const src = join(import.meta.dir, '..', 'src')
    const screen = readFileSync(join(src, 'sections', 'ready', 'ReadyScreen.tsx'), 'utf8')
    expect(screen).not.toMatch(/inSidecar|openSidecars|keySavable/)
    for (const lang of ['en', 'fr']) {
      const ready = JSON.parse(readFileSync(join(src, 'locales', lang, 'ready.json'), 'utf8'))
      expect(ready.keys.inSidecar).toBeUndefined()
      expect(ready.keys.openSidecars).toBeUndefined()
    }
  })

  test('a global key is one value for every universe', () => {
    expect(keyId(key('G', { scope: 'global', universeHome: '/a' }))).toBe(keyId(key('G', { scope: 'global', universeHome: '/b' })))
    expect(keyId(key('U', { universeHome: '/a' }))).not.toBe(keyId(key('U', { universeHome: '/b' })))
  })
})

describe('the first start', () => {
  test('the screen opens by itself the first time a universe is shown, never again', () => {
    expect(opensAtStart('main', new Set())).toBe(true)
    expect(opensAtStart('main', new Set(['main']))).toBe(false)
    expect(opensAtStart(null, new Set())).toBe(false)
  })

  // Seen at version A, it opens again at B, then never again at B.
  test('the screen opens again at the first start of each new installed version', () => {
    const a = installationVersion('2026.10-rc3', 'archifactory@2026.10-rc3')
    const b = installationVersion('2026.10-rc4', 'archifactory@2026.10-rc3')
    expect(a).not.toBe(b)
    const seen = new Set([seenKey('main', a)])
    expect(opensAtStart('main', seen, a)).toBe(false)
    expect(opensAtStart('main', seen, b)).toBe(true)
    seen.add(seenKey('main', b))
    expect(opensAtStart('main', seen, b)).toBe(false)
    // A record kept before the version was known still opens once at a version.
    expect(opensAtStart('main', new Set(['main']), b)).toBe(true)
  })

  test("the installed version is KaTyoti's and the solution's; a solution update alone changes it", () => {
    expect(installationVersion('1.2', 'archifactory@3.4')).toBe('1.2+3.4')
    expect(installationVersion('1.2', 'archifactory@3.5')).not.toBe(installationVersion('1.2', 'archifactory@3.4'))
    expect(installationVersion(undefined, 'archifactory')).toBe('')
    expect(seenKey('main', '')).toBe('main')
  })
})

// One tab per step — Profil et premiers pas, Programmes, Clés —
// each green or red, the screen opening on the first red one; a part that
// could not be read is red « Non vérifié », never green.
describe('the tabs', () => {
  const person = { id: 'ada', firstName: 'Ada', lastName: 'L', email: 'ada@example.com', source: 'file' }
  const chosen = { folder: '/home/ada/Katyoti', chosen: true, proposed: '/home/ada/Katyoti', installation: '/opt/katyoti' }
  const allRead = (p: Partial<ReadyInputs> = {}) => inputs({
    profile: person, projectsFolder: chosen, projects: ['/home/ada/projet'],
    doctor: [sidecar('s1', true)], plan: plan(program('p', {})), keys: keys([]), ...p,
  })

  test('every step done: three green tabs, the screen opens on the first', () => {
    const inp = allRead()
    expect([tabState(inp, 'profile'), tabState(inp, 'programs'), tabState(inp, 'keys')]).toEqual(['ok', 'ok', 'ok'])
    expect(firstTab(inp)).toBe('profile')
  })

  test('the placeholder profile, an e-mail missing or no project folder leave Profil red', () => {
    expect(tabState(allRead({ profile: { ...person, source: 'placeholder' } }), 'profile')).toBe('todo')
    expect(tabState(allRead({ profile: { ...person, email: '' } }), 'profile')).toBe('todo')
    expect(tabState(allRead({ projects: [] }), 'profile')).toBe('todo')
  })

  // The installation's projects folder is a check
  // of Profil et premiers pas.
  test('the projects folder only proposed leaves Profil red; chosen, it is green', () => {
    const proposed = { ...chosen, chosen: false }
    expect(tabState(allRead({ projectsFolder: proposed }), 'profile')).toBe('todo')
    expect(firstTab(allRead({ projectsFolder: proposed }))).toBe('profile')
    expect(projectsFolderAsked(allRead({ projectsFolder: proposed }))).toBe(true)
    expect(tabState(allRead(), 'profile')).toBe('ok')
  })

  test('the projects folder unread is « Non vérifié », never green', () => {
    expect(tabState(allRead({ errors: { projectsFolder: 'timeout' } }), 'profile')).toBe('unverified')
    expect(tabState(allRead({ projectsFolder: undefined }), 'profile')).toBe('unverified')
    expect(tabState(allRead({ projectsFolder: undefined, loading: true }), 'profile')).toBe('checking')
  })

  test('a development build has no installation: the projects folder is not asked and never blocks', () => {
    const dev = { folder: '/home/ada/Katyoti', chosen: false, proposed: '/home/ada/Katyoti' }
    expect(projectsFolderAsked(allRead({ projectsFolder: dev }))).toBe(false)
    expect(tabState(allRead({ projectsFolder: dev }), 'profile')).toBe('ok')
  })

  test('a required program to install makes Programmes red and the screen opens on it', () => {
    const inp = allRead({ plan: plan(program('p', { state: 'install' })) })
    expect(tabState(inp, 'programs')).toBe('todo')
    expect(firstTab(inp)).toBe('programs')
  })

  test('an optional program missing leaves Programmes green', () => {
    expect(tabState(allRead({ plan: plan(program('o', { required: false, state: 'install' })) }), 'programs')).toBe('ok')
  })

  test('a missing key makes Clés red and the screen opens on it', () => {
    const inp = allRead({ keys: keys([key('A_KEY')], true) })
    expect(tabState(inp, 'keys')).toBe('todo')
    expect(firstTab(inp)).toBe('keys')
  })

  test('a part that could not be read is « Non vérifié », never green, and opens its tab', () => {
    // The 2026-10-14 smoke machine: every read timed out, the counts all zero.
    const timedOut = inputs({ errors: { doctor: 'timeout', plan: 'timeout', keys: 'timeout' }, profile: person, projectsFolder: chosen, projects: ['/p'] })
    expect(tabState(timedOut, 'programs')).toBe('unverified')
    expect(tabState(timedOut, 'keys')).toBe('unverified')
    expect(firstTab(timedOut)).toBe('programs')
    expect(tabState(allRead({ errors: { profile: 'timeout' } }), 'profile')).toBe('unverified')
    expect(tabState(allRead({ errors: { projects: 'timeout' } }), 'profile')).toBe('unverified')
    // The config check alone unread still keeps Programmes from green.
    expect(tabState(allRead({ errors: { doctor: 'timeout' } }), 'programs')).toBe('unverified')
  })

  test('a part never read is « Non vérifié » once the check ended, « checking » while it runs', () => {
    expect(tabState(inputs({ profile: person, projects: ['/p'] }), 'keys')).toBe('unverified')
    expect(tabState(inputs({ loading: true }), 'keys')).toBe('checking')
  })

  test('a sidecar problem no program or key answers is technical, and keeps Programmes red', () => {
    const broken: SidecarCheck = { ...sidecar('s2', false), report: { ...sidecar('s2', false).report, items: [{ category: 'technical', name: 'socket', status: 'error' }] } }
    // While a key remains, a sidecar missing only a key is Clés's to show.
    const keyLeft = allRead({ doctor: [broken, sidecar('s3', false)], keys: keys([key('A_KEY')]) })
    expect(technicalProblems(keyLeft).map((d) => d.name)).toEqual(['s2'])
    expect(tabState(keyLeft, 'programs')).toBe('todo')
    expect(tabState(allRead({ doctor: [sidecar('s3', false)], keys: keys([key('A_KEY')]) }), 'programs')).toBe('ok')
  })

  test('a red badge always has a red tab: a failing sidecar no other tab explains keeps Programmes red', () => {
    // Plan and keys complete, a sidecar down with no item, or with only a
    // program the plan does not list (a sidecar it left unchecked).
    const down: SidecarCheck = { ...sidecar('s4', false), report: { ...sidecar('s4', false).report, items: [] } }
    const unplanned: SidecarCheck = { ...sidecar('s5', false), report: { ...sidecar('s5', false).report, items: [{ category: 'program', name: 'rclone', status: 'missing' }] } }
    for (const d of [down, unplanned, sidecar('s3', false)]) {
      const inp = allRead({ doctor: [d] })
      expect(summarize(inp).state).toBe('not-ready')
      expect(tabState(inp, 'programs')).toBe('todo')
      expect(firstTab(inp)).toBe('programs')
    }
  })
})

// A program carries a scope badge only when installing it needs
// administrator rights - every other one installs in the person's profile,
// which the panel says - labelled plainly, its tooltip saying what it means.
describe('the scope badge', () => {
  test('a user-scope program has none; an admin one, or one on the elevation list, has one', () => {
    expect(scopeBadge(program('p', { state: 'install', scope: 'user' }), plan())).toBeNull()
    expect(scopeBadge(program('a', { state: 'install', scope: 'admin' }), plan())).toBe('admin')
    expect(scopeBadge(program('e', { state: 'install', scope: 'user' }), { ...plan(), elevation: ['e'] })).toBe('admin')
  })
  test('a program already installed has none', () => {
    expect(scopeBadge(program('a', { state: 'ok', scope: 'admin' }), plan())).toBeNull()
  })
  test('a program whose install just succeeded has none, before the plan is read again; a failed install keeps it', () => {
    const a = program('a', { state: 'install', scope: 'admin' })
    expect(scopeBadge(a, plan(), { program: 'a', status: 'installed' } as InstallStep)).toBeNull()
    expect(scopeBadge(a, plan(), { program: 'a', status: 'failed' } as InstallStep)).toBe('admin')
    const screen = readFileSync(join(import.meta.dir, '..', 'src', 'sections', 'ready', 'ReadyScreen.tsx'), 'utf8')
    expect(screen).toContain("scopeBadge(p, plan, step) === 'admin'")
  })
  test('the admin badge and its tooltip say what they mean, in French and English; no user-scope label is left', () => {
    for (const [l, label] of [['fr', 'Droits admin requis'], ['en', 'Administrator rights']]) {
      const msgs = JSON.parse(readFileSync(join(import.meta.dir, '..', 'src', 'locales', l, 'ready.json'), 'utf8'))
      expect(msgs.programs.admin).toBe(label)
      expect(msgs.programs.adminTip.length).toBeGreaterThan(40)
      expect(msgs.programs.user).toBeUndefined()
    }
  })
})

// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { errorReport, installable, initialSelection, lookedForAfterInstall, offeredToInstall, recheckedOf, shownInstalled, stepOf, stepsWithoutCard, type InstallStep, type Program } from '../src/sections/ready/readiness'
import type { InstallPrereqsOutput } from '../src/ktsp/marketplace'

// Installing a program from « Prêt à démarrer »: its progress and
// result show in its own card, a failure with a short error and a full error
// report a person copies into an assistant's prompt; nothing at the top of
// the panel.

const failed: InstallStep = {
  program: 'chrome',
  kind: 'winget',
  scope: 'user',
  status: 'failed',
  detail: 'no installer of this package fits this computer for the scope asked (0x8A150010)',
  report: {
    command: 'winget install --id Google.Chrome --exact --source winget --scope user --silent',
    exitCode: 2316632080,
    code: '0x8A150010',
    meaning: 'no installer of this package fits this computer for the scope asked',
    output: 'Found Google Chrome [Google.Chrome]\nNo applicable installer found; see logs for more details.',
    logs: 'C:\\Users\\me\\AppData\\Local\\Packages\\Microsoft.DesktopAppInstaller_8wekyb3d8bbwe\\LocalState\\DiagOutputDir',
  },
}

const result = (steps: InstallStep[]): InstallPrereqsOutput => ({ plan: { platform: 'windows', programs: [] }, steps, elevationPrompts: 0 }) as unknown as InstallPrereqsOutput
const program = (name: string): Program => ({ name, required: true, neededBy: ['ktw-docs'], state: 'install', selected: true }) as Program

describe('the install steps of « Prêt à démarrer »', () => {
  test('each step finds its program\'s card, whatever its case; the others are listed apart', () => {
    const r = result([failed, { program: 'Python', status: 'installed' }, { program: 'bun', status: 'installed' }])
    expect(stepOf(r, 'Chrome')?.status).toBe('failed')
    expect(stepOf(r, 'python')?.status).toBe('installed')
    expect(stepOf(null, 'chrome')).toBeUndefined()
    expect(stepsWithoutCard(r, [program('chrome'), program('python')]).map((s) => s.program)).toEqual(['bun'])
  })

  test('the error report holds everything known, as text for an assistant', () => {
    const text = errorReport(failed, { platform: 'windows', universe: 'archifactory' })
    for (const want of [
      'Installing the program "chrome" failed',
      'Platform: windows - universe: archifactory',
      'Installer: winget, scope: user',
      'Command: winget install --id Google.Chrome --exact --source winget --scope user --silent',
      'Exit code: 2316632080 (0x8A150010)',
      'Meaning: no installer of this package fits this computer',
      'Installer logs: C:\\Users\\me\\AppData',
      'No applicable installer found; see logs for more details.',
      'Why did it fail',
    ]) expect(text).toContain(want)
    // A failure that ran no command says so.
    expect(errorReport({ program: 'bun', status: 'failed', detail: 'python did not install' })).toContain('No command ran')
  })

  test('the progress and the result are in the card, nothing at the top of the panel', () => {
    const screen = readFileSync(join(import.meta.dir, '..', 'src', 'sections', 'ready', 'ReadyScreen.tsx'), 'utf8')
    expect(screen).not.toContain("t('programs.installing'")
    expect(screen).not.toContain('stepsTitle')
    expect(screen).toContain('data-guide="ready.program-installing"')
    expect(screen).toContain('data-guide="ready.program-report-copy"')
    for (const lang of ['en', 'fr']) {
      const ready = JSON.parse(readFileSync(join(import.meta.dir, '..', 'src', 'locales', lang, 'ready.json'), 'utf8'))
      for (const key of ['installingOne', 'reportOpen', 'reportTitle', 'reportHint', 'copy', 'copied', 'otherSteps']) expect(ready.programs[key]).toBeTruthy()
    }
  })
})

// A program the re-plan after an install finds is shown installed,
// with its version, and no longer offered to install - optional ones too.
describe('a program found after its install', () => {
  test('is installed, not installable, not preselected', () => {
    const docling = { ...program('docling'), required: false, state: 'ok', found: '2.55.1' } as Program
    expect(installable(docling)).toBe(false)
    expect([...initialSelection({ platform: 'windows', programs: [docling] } as never)]).toEqual([])
    const screen = readFileSync(join(import.meta.dir, '..', 'src', 'sections', 'ready', 'ReadyScreen.tsx'), 'utf8')
    // The card's badge names the found version; its Install button shows only when installable.
    expect(screen).toContain("t('programs.stateOkVersion', { version: p.found })")
    expect(screen).toMatch(/\{offered && \(\s*<Button/)
  })
})

// A program installed but still not found says, in its card, what
// the check looked for - its commands, else its name (a sidecar's own probe).
describe('installed but not found', () => {
  const p = (state: string, commands?: string[]): Program => ({ name: 'mermaid-cli', required: false, neededBy: ['ktw-acme'], selected: false, state, commands })
  const installed: InstallStep = { program: 'mermaid-cli', status: 'installed', kind: 'bun' } as InstallStep
  test('found after its install: nothing to say', () => {
    expect(lookedForAfterInstall(p('ok'), installed, p('ok'))).toBeNull()
    expect(lookedForAfterInstall(p('install'), installed, p('ok'))).toBeNull()
  })
  test('installed yet its own check, run again, still does not find it: what was looked for', () => {
    expect(lookedForAfterInstall(p('install'), installed, p('install'))).toEqual({ key: 'notFoundAfterInstall', what: 'mermaid-cli' })
    expect(lookedForAfterInstall(p('install'), installed, p('install', ['mmdc', 'mmdc.cmd']))).toEqual({ key: 'notFoundAfterInstall', what: 'mmdc, mmdc.cmd' })
  })
  test('a failed or no install: nothing to say', () => {
    expect(lookedForAfterInstall(p('install'), { ...installed, status: 'failed' }, p('install'))).toBeNull()
    expect(lookedForAfterInstall(p('install'), undefined, p('install'))).toBeNull()
  })
  test('the card says it, in both languages', () => {
    for (const l of ['fr', 'en']) {
      const msgs = JSON.parse(readFileSync(join(import.meta.dir, '..', 'src', 'locales', l, 'ready.json'), 'utf8'))
      expect(msgs.programs.notFoundAfterInstall).toContain('{{what}}')
      expect(msgs.programs.notFoundAfterInstallPackage).toContain('{{what}}')
      expect(msgs.programs.notFoundAfterInstallPackage).not.toContain('PATH')
    }
  })
})

// Right after a pip package's install the card still holds the
// plan read before it (state install) for as long as the plan is read again;
// it judges by the install's own recheck - the package's real check - and,
// for a Python package, never speaks of the PATH.
describe('a Python package just installed', () => {
  const pymupdf = (state: string): Program => ({ name: 'pymupdf', required: true, neededBy: ['ktw-kb'], selected: true, state, pythonPackage: 'pymupdf' }) as Program
  const installed: InstallStep = { program: 'pymupdf', status: 'installed', kind: 'pip' } as InstallStep
  const out = (state?: string): InstallPrereqsOutput => ({
    plan: { platform: 'windows', programs: [] }, steps: [installed], elevationPrompts: 0,
    ...(state ? { after: { platform: 'windows', programs: [pymupdf(state)] } } : {}),
  }) as unknown as InstallPrereqsOutput
  test('its check has not answered: nothing is said, the old plan is no verdict', () => {
    expect(recheckedOf(out(), 'pymupdf')).toBeUndefined()
    expect(lookedForAfterInstall(pymupdf('install'), installed, recheckedOf(out(), 'pymupdf'))).toBeNull()
  })
  test('found by its package check: no « introuvable », while the plan still says to install', () => {
    expect(lookedForAfterInstall(pymupdf('install'), installed, recheckedOf(out('ok'), 'PyMuPDF'))).toBeNull()
  })
  test('not found by its package check: said as the Python package, never the PATH', () => {
    expect(lookedForAfterInstall(pymupdf('install'), installed, recheckedOf(out('install'), 'pymupdf'))).toEqual({ key: 'notFoundAfterInstallPackage', what: 'pymupdf' })
  })
  test('the card reads the recheck, not the plan alone', () => {
    const screen = readFileSync(join(import.meta.dir, '..', 'src', 'sections', 'ready', 'ReadyScreen.tsx'), 'utf8')
    expect(screen).toContain('rechecked={recheckedOf(result, p.name)}')
    expect(screen).toContain('lookedForAfterInstall(p, step, rechecked)')
  })
})

// A program whose install just succeeded is shown installed and no
// longer offered at once - its card does not wait for the plan read again; a
// failed install keeps it offered.
describe('a program just installed', () => {
  const p = { ...program('mermaid-cli'), required: false, selected: false } as Program
  const installed: InstallStep = { program: 'mermaid-cli', status: 'installed', kind: 'bun' } as InstallStep
  test('its install succeeded: installed, no checkbox nor Installer button, even while the plan still says to install', () => {
    expect(shownInstalled(p, installed)).toBe(true)
    expect(offeredToInstall(p, installed)).toBe(false)
  })
  test('its install failed, or none ran: still to install, still offered', () => {
    expect(shownInstalled(p, failed)).toBe(false)
    expect(offeredToInstall(p, { ...failed, program: 'mermaid-cli' })).toBe(true)
    expect(shownInstalled(p, undefined)).toBe(false)
    expect(offeredToInstall(p, undefined)).toBe(true)
  })
  test('the card reads both: its badge, its checkbox and its button; a selection drops what was just installed', () => {
    const screen = readFileSync(join(import.meta.dir, '..', 'src', 'sections', 'ready', 'ReadyScreen.tsx'), 'utf8')
    expect(screen).toContain('const ok = shownInstalled(p, step)')
    expect(screen).toContain('checked={checked && offered} disabled={!offered || busy}')
    expect(screen).toContain('const chosen = [...selected].filter((n) => !justInstalled(n))')
    expect(screen).not.toMatch(/disabled=\{!installable\(p\)/)
  })
})

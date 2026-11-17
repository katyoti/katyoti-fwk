// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { NotServedError, notServedText, samePath, serveChange, waitUntilServed } from '../src/sections/univers/serveWait'

// A switch of the served project waits until the universe serves
// it; a read failing while the member restarts is read again, a wait past its
// bound names what the universe still serves. Never a silent success.
describe('the wait until the universe serves a folder', () => {
  test('the wait ends once the folder is served - a read failing while the member restarts read again', async () => {
    const reads: (() => Promise<string[]>)[] = [
      async () => { throw new Error('no responders') }, async () => ['D:\\demo'], async () => ['d:\\demo\\projects\\a\\'],
    ]
    let i = 0
    await waitUntilServed(() => reads[Math.min(i++, reads.length - 1)](), 'D:/demo/projects/a', { timeoutMs: 2000, everyMs: 1 })
    expect(i).toBe(3)
  })
  test('a wait past its bound throws, naming what the universe still serves', async () => {
    const err = await waitUntilServed(async () => ['D:/demo'],'D:/demo/projects/a', { timeoutMs: 20, everyMs: 5 }).catch((e) => e)
    expect(err).toBeInstanceOf(NotServedError)
    expect(String(err.message)).toContain('still serves D:/demo')
  })
  test('folders compare as the system does', () => {
    expect(samePath('D:\\demo\\a','d:/demo/a/')).toBe(true)
    expect(samePath('/home/x/A', '/home/x/a')).toBe(false)
    expect(samePath('', '')).toBe(false)
  })
  test('the switch no longer deactivates and reactivates the universe: it waits for the folder, and a restart error rejects', () => {
    const src = readFileSync(join(import.meta.dir, '..', 'src', 'sections', 'univers', 'projectMount.ts'), 'utf8')
    const fn = src.slice(src.indexOf('export async function switchServedProject'))
    expect(fn).not.toContain('repointCore')
    expect(fn).toContain('await awaitServed(universe, out, projectDir)')
  })
})

// ATTACHING, DETACHING OR CREATING A PROJECT IN A RUNNING UNIVERSE:
// the hub restarts the member serving the universe's objects, and the GUI waits
// until it serves the change through the one helper the switch uses too; a
// change recorded but not served is shown as it is, with the reason - never the
// old deactivate/activate of the GUI's connection.
describe('an attach, a detach or a creation waits until the universe serves it', () => {
  const tKey = (key: string, vars?: Record<string, unknown>) => key + (vars ? ' ' + JSON.stringify(vars) : '')
  test('a failed restart shows recorded but not served, with the reason; a refusal as it is', async () => {
    const err = await serveChange({ restartError: 'ktw-agents could not be restarted: stop timed out' }, 'D:/p', async () => []).catch((e) => e)
    expect(err).toBeInstanceOf(NotServedError)
    const said = notServedText(err, tKey)
    expect(said).toContain('projects.notServed')
    expect(said).toContain('stop timed out')
    expect(notServedText(new Error('universe "x" already serves project D:/q'), tKey)).toBe('universe "x" already serves project D:/q')
  })
  test('a restart is waited for: the attached folder, or no project after a detach', async () => {
    const reads = [async () => { throw new Error('no responders') }, async () => ['D:/old'], async () => ['d:\\p\\']]
    let i = 0
    await serveChange({ restarted: true }, 'D:/p', () => reads[Math.min(i++, reads.length - 1)](), { timeoutMs: 2000, everyMs: 1 })
    expect(i).toBe(3)
    // A detach of the last member: only a read that answers no project ends the wait - a failed read does not.
    const gone = [async () => { throw new Error('no responders') }, async () => ['D:/p'], async () => [] as string[]]
    let j = 0
    await serveChange({ restarted: true }, { gone: 'D:/p' }, () => gone[Math.min(j++, gone.length - 1)](), { timeoutMs: 2000, everyMs: 1 })
    expect(j).toBe(3)
    const stuck = await waitUntilServed(async () => ['D:/p'], '', { timeoutMs: 20, everyMs: 5 }).catch((e) => e)
    expect(stuck).toBeInstanceOf(NotServedError)
    expect(String(stuck.message)).toContain('still serves D:/p')
  })
  test('detaching one of several members waits for that folder gone, not for no project', async () => {
    // Served [a, b], a detached: the wait ends once a is gone, b served still.
    const reads = [async () => ['D:/a', 'D:/b'], async () => { throw new Error('no responders') }, async () => ['d:\\b\\']]
    let i = 0
    await serveChange({ restarted: true }, { gone: 'D:/a' }, () => reads[Math.min(i++, reads.length - 1)](), { timeoutMs: 2000, everyMs: 1 })
    expect(i).toBe(3)
    // [a, b] unchanged past the bound: the wait throws, naming a.
    const stuck = await waitUntilServed(async () => ['D:/a', 'D:/b'], { gone: 'd:\\a' }, { timeoutMs: 20, everyMs: 5 }).catch((e) => e)
    expect(stuck).toBeInstanceOf(NotServedError)
    expect(String(stuck.message)).toContain('still serves D:/a, D:/b')
    // An attach beside a served member: the new folder anywhere in the list ends the wait.
    await waitUntilServed(async () => ['D:/a', 'D:/c'], 'D:/c', { timeoutMs: 20, everyMs: 5 })
  })
  test('nothing restarted (the universe does not run): nothing to wait for', async () => {
    let read = 0
    await serveChange({}, 'D:/p', async () => { read++; return [] })
    expect(read).toBe(0)
  })
  test('the panels use the shared helper, never repointCore', () => {
    const src = (...p: string[]) => readFileSync(join(import.meta.dir, '..', 'src', ...p), 'utf8')
    const mount = src('sections', 'univers', 'projectMount.ts')
    expect(mount).not.toContain('repointCore')
    const panel = src('pages', 'UniverseProjectsPanel.tsx')
    const shell = src('sections', 'shell', 'ProjectPanel.tsx')
    for (const f of [panel, shell]) {
      expect(f).not.toContain('repointCore')
      expect(f).toContain("await awaitServed(")
      expect(f).toContain('notServedText(err, ')
    }
    expect(panel.match(/await awaitServed\(/g)?.length).toBe(3)
    expect(panel).toContain('await awaitServed(universe, out, { gone: dir })')
    expect(shell).toContain('await awaitServed(displayedUniverseName, out, { gone: dir })')
    // Every served member is read, not only the first.
    expect(mount).toContain('.filter((p) => p.served && p.dir)')
  })
})

// The project-group screens are gone; the waits they shared live in
// sections/univers/serveWait.ts, the one place every panel imports them from.
describe('the serve waits have one home once the group code is gone', () => {
  test('projectMount, UniverseProjectsPanel and ProjectPanel import them from serveWait', () => {
    const src = (...p: string[]) => readFileSync(join(import.meta.dir, '..', 'src', ...p), 'utf8')
    expect(src('sections', 'univers', 'projectMount.ts')).toContain("from './serveWait'")
    expect(src('pages', 'UniverseProjectsPanel.tsx')).toContain("from '../sections/univers/serveWait'")
    expect(src('sections', 'shell', 'ProjectPanel.tsx')).toContain("from '../univers/serveWait'")
  })
})

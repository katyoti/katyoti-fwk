// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { ELEMENTS } from '../src/elements'
import {
  CONVERSION_BADGE,
  CONVERSION_CONFIRM,
  CONVERSION_DIALOG,
  CONVERT_ACTION,
  MIGRATE,
  MIGRATE_COMMIT,
  PROJECTS_KNOWN,
  canConfirm,
  conversionEnd,
  conversionHeader,
  conversionMark,
  knownAt,
  readingLine,
  servedProject,
  stepRows,
  treeLine,
  upToDate,
  type MigrateAnswer,
} from '../src/sections/project/conversion'

// A project whose data is older than KaTyoti offers
// its conversion in the GUI, drawn from cwm's answers alone.
const src = (rel: string) => readFileSync(join(import.meta.dir, '..', 'src', rel), 'utf8')

const dry = (over: Partial<MigrateAnswer> = {}): MigrateAnswer => ({
  from: 30,
  to: 32,
  dry_run: true,
  steps: [
    { from: 30, to: 31, title: 'Rename the field x', changed: ['modules/a/goals/G-001.cwm.yaml', 'modules/b/goals/G-002.cwm.yaml'] },
    { from: 31, to: 32, title: 'Nothing for this project' },
  ],
  tree: { repo: true, clean: true },
  ...over,
})

describe('the mark', () => {
  test('waits, is newer, or is nothing, as ProjectsKnown says', () => {
    expect(conversionMark({ dir: 'd', conversion: 'waiting', data_version: 30 })).toEqual({ kind: 'waiting', from: 30 })
    expect(conversionMark({ dir: 'd', conversion: 'newer', data_version: 40 })).toEqual({ kind: 'newer', from: 40 })
    expect(conversionMark({ dir: 'd', data_version: 34 })).toEqual({ kind: 'none' })
    expect(conversionMark(undefined)).toEqual({ kind: 'none' })
  })
  test('is the served project\'s in the header, the folder\'s in the list', () => {
    const out = { projects: [{ dir: 'D:\\p\\one', served: true, conversion: 'waiting', data_version: 30 }, { dir: 'D:/p/two', conversion: 'newer', data_version: 40 }] }
    expect(servedProject(out)?.dir).toBe('D:\\p\\one')
    expect(servedProject({})).toBeUndefined()
    expect(knownAt(out.projects, 'd:/P/One/')?.served).toBe(true)
    expect(conversionMark(knownAt(out.projects, 'D:\\p\\two')).kind).toBe('newer')
  })
})

describe('the dialog', () => {
  test('lists each step: the files it rewrites, or nothing to rewrite', () => {
    const rows = stepRows(dry())
    expect(rows.map((r) => [r.from, r.to, r.nothing, r.count])).toEqual([[30, 31, false, 2], [31, 32, true, 0]])
    expect(stepRows(null)).toEqual([])
  })
  test('says where git stands: clean, the dirty files named, or no repository', () => {
    expect(treeLine(dry())).toEqual({ kind: 'clean' })
    expect(treeLine(dry({ tree: { repo: true, clean: false, touched_dirty: ['project.cwm.yaml'] } }))).toEqual({ kind: 'dirty', files: ['project.cwm.yaml'] })
    expect(treeLine(dry({ tree: { repo: false, clean: false } }))).toEqual({ kind: 'no-repo' })
    // No tree: the dry run did not ask git (nothing waits) - no line, never « no repository ».
    expect(treeLine(dry({ tree: undefined }))).toEqual({ kind: 'none' })
    expect(treeLine(null)).toEqual({ kind: 'none' })
  })
  test('says the project is up to date when nothing waits, and offers no Confirm', () => {
    // Converted meanwhile by another process: the dry run goes from 49 to 49, no step, no tree.
    const none = dry({ from: 49, to: 49, steps: [], tree: undefined })
    expect(upToDate(none)).toBe(true)
    expect(canConfirm(none)).toBe(false)
    expect(treeLine(none)).toEqual({ kind: 'none' })
    expect(upToDate(dry())).toBe(false)
    expect(upToDate(null)).toBe(false)
    const dialog = src('sections/project/ConversionDialog.tsx')
    expect(dialog).toContain("t('conversion.upToDate'")
    expect(dialog).toContain('{!end && !upToDate(dry) && (')
    // The stale mark is dropped: the views read the project again.
    expect(dialog).toMatch(/if \(upToDate\(out as MigrateAnswer\)\) useAppStore\.setState\(\(s\) => \(\{ universeRevision: s\.universeRevision \+ 1/)
  })
  test('the mark reads ProjectsKnown again when the workspace data changes', () => {
    const mark = src('sections/project/ConversionMark.tsx')
    expect(mark).toContain('s.workspaceTreeRevision')
    expect(mark).toContain('[universe, revision, dataRevision]')
  })
  test('confirms only when something waits and nothing it writes is dirty', () => {
    expect(canConfirm(dry())).toBe(true)
    expect(canConfirm(dry({ tree: { repo: false, clean: false } }))).toBe(true) // keep a copy, then convert
    expect(canConfirm(dry({ tree: { repo: true, clean: false, touched_dirty: ['x'] } }))).toBe(false)
    expect(canConfirm(dry({ from: 32 }))).toBe(false)
    expect(canConfirm(null)).toBe(false)
  })
  test('ends on the version reached and the commit, or says why none was made', () => {
    const run = { ...dry(), dry_run: false, tree: undefined }
    expect(conversionEnd({ ...run, commit: { sha: 'abcdef1234567' } }, true)).toEqual({ kind: 'committed', to: 32, files: 2, sha: 'abcdef1234567' })
    expect(conversionEnd({ ...run, commit: { reason: 'the project is in no git repository' } }, false)).toEqual({ kind: 'no-repo', to: 32, files: 2 })
    expect(conversionEnd({ ...run, commit: { reason: 'secret-scan: refused' } }, true)).toEqual({ kind: 'refused', to: 32, files: 2, reason: 'secret-scan: refused' })
  })
})

describe("the end survives the views' re-read", () => {
  test('the dialog stays on its project when the re-read finds it up to date', () => {
    const before = { dir: 'D:/p/one', served: true, conversion: 'waiting', data_version: 30 }
    const after = { dir: 'D:/p/one', served: true, data_version: 32 }
    // Opened on the waiting project; Confirm converts; the views read it again.
    expect(conversionHeader(before, before)).toEqual({ mark: { kind: 'waiting', from: 30 }, dialog: before })
    expect(conversionHeader(after, before)).toEqual({ mark: { kind: 'none' }, dialog: before })
    expect(conversionHeader(undefined, before).dialog).toBe(before)
    // Closed, nothing is drawn.
    expect(conversionHeader(after, null)).toEqual({ mark: { kind: 'none' }, dialog: null })
  })
  test('the header draws the dialog apart from the mark, and the views re-read after a failure too', () => {
    const mark = src('sections/project/ConversionMark.tsx')
    expect(mark).toContain('conversionHeader(project, converting)')
    expect(mark).toContain('{dialog && <ConversionDialog')
    expect(mark).not.toContain('return null')
    const dialog = src('sections/project/ConversionDialog.tsx')
    // In the finally block: after a failure too, files may be rewritten.
    expect(dialog).toMatch(/\} finally \{[^}]*universeRevision: s\.universeRevision \+ 1/)
  })
})

describe('the methods', () => {
  test('are named in kebab, the name their timeout is declared under', () => {
    expect([PROJECTS_KNOWN, MIGRATE, MIGRATE_COMMIT]).toEqual([
      'ktw-agents:cwm/projects-known',
      'ktw-agents:cwm/migrate',
      'ktw-agents:cwm/migrate-commit',
    ])
    for (const rel of ['sections/project/ConversionDialog.tsx', 'sections/project/ConversionMark.tsx', 'pages/UniverseProjectsPanel.tsx']) {
      expect(src(rel)).not.toMatch(/ktw-agents:cwm\/[A-Z]/)
    }
  })

  test('a long dry run says it is still reading the project, with the seconds', () => {
    expect(readingLine(0)).toEqual({ kind: 'loader' })
    expect(readingLine(1.9)).toEqual({ kind: 'loader' })
    expect(readingLine(16.4)).toEqual({ kind: 'reading', seconds: 16 })
    expect(src('sections/project/ConversionDialog.tsx')).toContain("t('conversion.reading'")
  })
})

describe('the screens', () => {
  test('carry the elements, each in the registry', () => {
    const ids = ELEMENTS.map((e) => e.id)
    for (const id of [CONVERSION_BADGE, CONVERT_ACTION, CONVERSION_DIALOG, CONVERSION_CONFIRM]) expect(ids).toContain(id)
    expect(src('sections/project/ConversionMark.tsx')).toContain('data-element={CONVERT_ACTION}')
    expect(src('sections/project/ConversionDialog.tsx')).toContain('data-element={CONVERSION_CONFIRM}')
    expect(src('sections/project/ConversionDialog.tsx')).toContain('disabled={!canConfirm(dry)}')
    expect(src('sections/shell/ProjectPanel.tsx')).toContain('<ConversionMark universe={displayedUniverseName} />')
    expect(src('pages/UniverseProjectsPanel.tsx')).toContain('conversionMark(knownAt(known, dir))')
  })
  test('say it in English and French alike', () => {
    const en = JSON.parse(src('locales/en/shell.json')).conversion
    const fr = JSON.parse(src('locales/fr/shell.json')).conversion
    expect(Object.keys(fr).sort()).toEqual(Object.keys(en).sort())
    expect(fr.nothing).toBe('rien à réécrire')
  })
})

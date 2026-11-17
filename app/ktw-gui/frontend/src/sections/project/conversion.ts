// SPDX-License-Identifier: Apache-2.0
/**
 * A project whose data is older than KaTyoti offers its conversion:
 * the model of the mark and of the dialog, pure.
 *
 * CWM says it all: projects-known gives each known project its
 * `data_version` and `conversion` (`waiting` older than this KaTyoti, `newer`
 * newer than it knows); the migrate dry run (`cwm/migrate`, `dry_run`) gives
 * each step's title and the files it would change or remove, and the git tree
 * of those files; `cwm/migrate-commit` converts and commits them as one commit,
 * its steps reported into its panel. Nothing here is project-specific: the
 * dialog is drawn from the method's answers.
 */

/** The cwm methods the conversion goes through, every segment kebab (the
 *  URI rule): the name the method is declared and its timeout read under. */
export const PROJECTS_KNOWN = 'ktw-agents:cwm/projects-known'
export const MIGRATE = 'ktw-agents:cwm/migrate'
export const MIGRATE_COMMIT = 'ktw-agents:cwm/migrate-commit'

/** The elements the conversion draws. */
export const CONVERSION_BADGE = 'badge:project/conversion'
export const CONVERT_ACTION = 'action:project/convert'
export const CONVERSION_DIALOG = 'section:project/conversion-dialog'
export const CONVERSION_CONFIRM = 'action:project/conversion-confirm'

/** A project CWM knows, as ProjectsKnown answers it. */
export interface KnownProject {
  dir: string
  id?: string
  name?: string
  served?: boolean
  data_version?: number
  conversion?: string
}

/** Where a project's data stands against this KaTyoti. */
export type ConversionMark =
  | { kind: 'none' }
  | { kind: 'waiting'; from: number }
  | { kind: 'newer'; from: number }

/** The mark a known project carries: none at this KaTyoti's version. */
export function conversionMark(p: KnownProject | undefined | null): ConversionMark {
  const from = p?.data_version ?? 1
  if (p?.conversion === 'waiting') return { kind: 'waiting', from }
  if (p?.conversion === 'newer') return { kind: 'newer', from }
  return { kind: 'none' }
}

/** What the project panel's header draws: the mark, as ProjectsKnown says
 *  it now, and the conversion dialog on the project it was opened on. The
 *  dialog does not hang on the mark: after Confirm the views read the project
 *  again and it comes back up to date (no mark), yet the dialog stays until
 *  the person closes it, so its end - the version reached, the commit, or a
 *  hook's reason and « commit by hand » - is read. */
export function conversionHeader(project: KnownProject | undefined | null, converting: KnownProject | null): { mark: ConversionMark; dialog: KnownProject | null } {
  return { mark: conversionMark(project), dialog: converting }
}

/** The project the universe serves, from ProjectsKnown's answer. */
export function servedProject(out: unknown): KnownProject | undefined {
  const projects = (out as { projects?: KnownProject[] } | null)?.projects
  return Array.isArray(projects) ? projects.find((p) => p.served) : undefined
}

/** A folder as compared: slashes one way, case folded (Windows). */
export const sameDir = (a: string, b: string) => norm(a) === norm(b)
const norm = (d: string) => d.replace(/\\/g, '/').replace(/\/+$/, '').toLowerCase()

/** The known project at a folder. */
export function knownAt(projects: KnownProject[] | undefined, dir: string): KnownProject | undefined {
  return projects?.find((p) => sameDir(p.dir, dir))
}

/** The dry run reads every object of the project: on a large one, or right
 *  after the hub started, it takes seconds. Past this delay the dialog says
 *  it is still reading the project, with the seconds elapsed. */
export const READING_SAYS_AFTER_S = 2

/** What the dialog shows while the dry run runs, `elapsed` seconds in: a bare
 *  loader at first, then that it is still reading the project. */
export function readingLine(elapsed: number): { kind: 'loader' } | { kind: 'reading'; seconds: number } {
  const seconds = Math.max(0, Math.floor(elapsed))
  return seconds < READING_SAYS_AFTER_S ? { kind: 'loader' } : { kind: 'reading', seconds }
}

/** A step of a migrate answer. */
export interface MigrationStep {
  from: number
  to: number
  title: string
  changed?: string[]
  removed?: string[]
}

/** Where git stands on the files a conversion writes. */
export interface MigrateTree {
  repo: boolean
  clean: boolean
  touched_dirty?: string[]
}

/** The migrate dry run's answer (and MigrateCommit's, with its commit). */
export interface MigrateAnswer {
  from: number
  to: number
  dry_run?: boolean
  steps?: MigrationStep[]
  unreadable?: string[]
  tree?: MigrateTree
  commit?: { sha?: string; reason?: string }
}

/** A step as the dialog lists it. */
export interface StepRow {
  from: number
  to: number
  title: string
  /** Nothing to rewrite: the step changes no object. */
  nothing: boolean
  changed: string[]
  removed: string[]
  /** Files it writes, changed and removed. */
  count: number
}

/** The dialog's rows: one per step, in order. */
export function stepRows(out: MigrateAnswer | null | undefined): StepRow[] {
  return (out?.steps ?? []).map((s) => {
    const changed = s.changed ?? []
    const removed = s.removed ?? []
    return { from: s.from, to: s.to, title: s.title, nothing: changed.length + removed.length === 0, changed, removed, count: changed.length + removed.length }
  })
}

/** The git line under the steps. */
export type TreeLine =
  | { kind: 'clean' }
  | { kind: 'dirty'; files: string[] }
  | { kind: 'no-repo' }
  | { kind: 'none' }

/** The git line: clean, the touched files that differ from the last commit,
 *  or no repository (keep a copy first) - only when the dry run asked git:
 *  it carries no tree when nothing waits, and then no line is drawn. */
export function treeLine(out: MigrateAnswer | null | undefined): TreeLine {
  const tree = out?.tree
  if (!tree) return { kind: 'none' }
  if (!tree.repo) return { kind: 'no-repo' }
  const files = tree.touched_dirty ?? []
  if (!tree.clean || files.length > 0) return { kind: 'dirty', files }
  return { kind: 'clean' }
}

/** Something waits: the dry run starts below this KaTyoti's version. */
export const waits = (out: MigrateAnswer | null | undefined) => !!out && out.from < out.to

/** Nothing waits: the project is already at this KaTyoti's version (it was
 *  converted meanwhile, by another window or the command line). The dialog
 *  says so, offers no Confirm, and the mark reads ProjectsKnown again. */
export const upToDate = (out: MigrateAnswer | null | undefined) => !!out && out.from >= out.to

/** Confirm is enabled only when something waits and no file the conversion
 *  writes has uncommitted edits (MigrateCommit would refuse it). */
export function canConfirm(out: MigrateAnswer | null | undefined): boolean {
  return waits(out) && treeLine(out).kind !== 'dirty'
}

/** The end of a conversion, as the dialog says it. */
export type ConversionEnd =
  | { kind: 'committed'; to: number; files: number; sha: string }
  | { kind: 'no-repo'; to: number; files: number }
  | { kind: 'refused'; to: number; files: number; reason: string }

/** The end: the version reached, the files written and the commit; in no
 *  repository, converted with nothing committed; a commit git refused (a
 *  hook - the secret scanner, say) leaves the data converted, the reason
 *  shown and the person commits by hand. `repo` is the dry run's tree: the
 *  conversion's own answer carries none. */
export function conversionEnd(out: MigrateAnswer, repo: boolean): ConversionEnd {
  const files = new Set(stepRows(out).flatMap((r) => [...r.changed, ...r.removed])).size
  const sha = out.commit?.sha ?? ''
  if (sha) return { kind: 'committed', to: out.to, files, sha }
  if (!repo) return { kind: 'no-repo', to: out.to, files }
  return { kind: 'refused', to: out.to, files, reason: out.commit?.reason ?? '' }
}

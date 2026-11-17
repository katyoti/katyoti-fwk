// SPDX-License-Identifier: Apache-2.0
import type { EarlierProjectsOutput, ImportProjectInput, ImportProjectOutput } from '../../ktsp/marketplace'

/** One project an earlier installation serves. */
export type EarlierProject = EarlierProjectsOutput['projects'][number]

/**
 * The earlier installations' projects, offered on « Prêt à démarrer »:
 * each with its installation, folder and last use; none is
 * imported unless the person picks it. Finding them and importing one as a
 * migrated copy - the original never written - is the hub's
 * (marketplace `earlier-projects` / `import-project`).
 */

/** The marketplace calls the offer makes. */
export interface Importer {
  importProject: (input: ImportProjectInput) => Promise<ImportProjectOutput>
}

/** The offer's rows: the newest used first; one imported already says where. */
export function offerRows(out: EarlierProjectsOutput | undefined): EarlierProject[] {
  return [...(out?.projects ?? [])].sort((a, b) => (b.last_used ?? '').localeCompare(a.last_used ?? ''))
}

/** A row can be picked while it is not imported yet. */
export function importable(p: EarlierProject): boolean {
  return !p.imported_to
}

/** The day a project was last used, in the GUI's language; empty when unknown. */
export function lastUseDay(p: EarlierProject, lang: string): string {
  if (!p.last_used) return ''
  const d = new Date(p.last_used)
  return Number.isNaN(d.getTime()) ? '' : d.toLocaleDateString(lang, { year: 'numeric', month: 'short', day: 'numeric' })
}

/** One import's outcome: the copy, or why it failed. */
export interface ImportOutcome {
  folder: string
  copy?: string
  error?: string
}

/** Imports the picked projects - only those, each as a copy, one after the
 *  other; a failure says why and the others still go. Nothing picked: no call. */
export async function importPicked(client: Importer, rows: EarlierProject[], picked: ReadonlySet<string>): Promise<ImportOutcome[]> {
  const out: ImportOutcome[] = []
  for (const p of rows) {
    if (!picked.has(p.folder) || !importable(p)) continue
    try {
      const r = await client.importProject({ folder: p.folder })
      out.push({ folder: p.folder, copy: r.project })
    } catch (err) {
      out.push({ folder: p.folder, error: err instanceof Error ? err.message : String(err) })
    }
  }
  return out
}

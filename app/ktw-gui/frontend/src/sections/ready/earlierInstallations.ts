// SPDX-License-Identifier: Apache-2.0
import type { DeleteInstallationInput, DeleteInstallationOutput, EarlierInstallationsOutput } from '../../ktsp/marketplace'

/**
 * The earlier installations of this machine, on « Prêt à démarrer »:
 * each with its version, folder, size and last use; one is deleted
 * only after the person acknowledged what goes (the plan's folders, sizes and
 * shortcuts), and the most recently used one is kept unless the person unticks
 * it. Listing and deleting by the control plane's rule are the hub's
 * (marketplace `earlier-installations` / `delete-installation`).
 */

/** One installation of this machine. */
export type Installation = EarlierInstallationsOutput['installations'][number]

/** The marketplace call a deletion makes. */
export interface Deleter {
  deleteInstallation: (input: DeleteInstallationInput) => Promise<DeleteInstallationOutput>
}

/** The reasons an installation cannot be deleted (the hub's codes). */
export const REFUSAL_REASONS = ['current', 'last', 'projects-inside', 'shared'] as const

/** The earlier installations: every one but the one answering, the newest used first. */
export function earlierRows(out: EarlierInstallationsOutput | undefined): Installation[] {
  return (out?.installations ?? []).filter((i) => !i.current).sort((a, b) => (b.last_used ?? '').localeCompare(a.last_used ?? ''))
}

/** The one kept unless the person unticks it: the most recently used earlier installation. */
export function keptByDefault(rows: Installation[]): Set<string> {
  return new Set(rows.length ? [rows[0].name] : [])
}

/** A deletion can be asked for an installation the hub does not refuse - the
 *  last one only once the person said to delete it too - and the person does
 *  not keep. */
export function canAskDelete(i: Installation, kept: ReadonlySet<string>): boolean {
  if (kept.has(i.name)) return false
  return !i.reason || i.reason === 'last'
}

/** The plan of deleting it: what goes and the token that acknowledges it - nothing touched. */
export async function planDelete(client: Deleter, i: Installation): Promise<DeleteInstallationOutput> {
  return client.deleteInstallation({ name: i.name, delete_last: i.reason === 'last' })
}

/** Deletes it, only once the person acknowledged the plan: without the
 *  acknowledgement, no call. */
export async function confirmDelete(client: Deleter, plan: DeleteInstallationOutput, acknowledged: boolean, deleteLast: boolean): Promise<DeleteInstallationOutput | undefined> {
  if (!acknowledged || !plan.token) return undefined
  return client.deleteInstallation({ name: plan.name, ack: plan.token, delete_last: deleteLast })
}

/** A size as a person reads it. */
export function humanSize(bytes: number): string {
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let n = bytes
  let u = 0
  while (n >= 1024 && u < units.length - 1) {
    n /= 1024
    u++
  }
  return u === 0 ? `${n} ${units[u]}` : `${n.toFixed(1)} ${units[u]}`
}

/** The locale key of a refusal; undefined when only the hub's sentence says it. */
export function refusalKey(i: Installation): string | undefined {
  return i.reason && (REFUSAL_REASONS as readonly string[]).includes(i.reason) ? `installations.refused.${i.reason}` : undefined
}
